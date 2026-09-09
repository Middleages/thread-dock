package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	statev2 "thread-dock/internal/state/v2"
)

type Preparer interface {
	Prepare(context.Context, PreparationRequest) (statev2.WorkSnapshot, error)
}

type RuntimeCoordinator interface {
	Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error)
	SubmitRuntime(context.Context, contractv2.WorkID, contractv2.TaskID, statev2.InvocationID) <-chan coordinator.CommandResult
	Close(context.Context) error
}

type ForegroundBinding struct {
	RepositoryPath     string
	WorktreeRoot       string
	RuntimeFingerprint string
}

type ForegroundService struct {
	state    State
	preparer Preparer
	runtime  RuntimeCoordinator
	binding  ForegroundBinding
}

func NewForegroundService(state State, preparer Preparer, runtime RuntimeCoordinator, binding ForegroundBinding) *ForegroundService {
	return &ForegroundService{state: state, preparer: preparer, runtime: runtime, binding: binding}
}

func (s *ForegroundService) RunWork(ctx context.Context, workID contractv2.WorkID, expected contractv2.Revision, requestID contractv2.RequestID) (snapshot statev2.WorkSnapshot, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateForegroundDependencies(s); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if err := validateForegroundInput(s.binding, workID, expected, requestID); err != nil {
		return statev2.WorkSnapshot{}, err
	}

	invocationID, logicalWorkID := foregroundIDs(workID, requestID)
	snapshot, err = s.state.Load(ctx, workID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if replayInvocation(snapshot, invocationID) {
		return snapshot, nil
	}
	if err := validateFreshSnapshot(snapshot, workID, expected); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if _, err = s.runtime.Activate(ctx, workID); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	activated := true
	defer func() {
		if !activated {
			return
		}
		closeErr := s.runtime.Close(ctx)
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	snapshot, err = s.state.Load(ctx, workID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if existing := outstandingLifecycle(snapshot); existing {
		return snapshot, nil
	}
	task, plan, err := selectForegroundTask(snapshot)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	worktreePath := filepath.Join(s.binding.WorktreeRoot, foregroundWorktreeID(workID, task.TaskID))
	preparation := PreparationRequest{
		WorkID:             workID,
		TaskID:             task.TaskID,
		InvocationID:       invocationID,
		LogicalWorkID:      logicalWorkID,
		RepositoryPath:     s.binding.RepositoryPath,
		WorktreePath:       worktreePath,
		Branch:             task.Branch,
		BaseSHA:            plan.BaseSHA,
		RuntimeFingerprint: s.binding.RuntimeFingerprint,
	}
	snapshot, err = s.preparer.Prepare(ctx, preparation)
	if err != nil {
		return snapshot, err
	}
	if !reservedPreparationMatches(snapshot, preparation, *task) {
		return snapshot, errors.New("preparation did not return the requested invocation")
	}

	resultChannel := s.runtime.SubmitRuntime(ctx, workID, task.TaskID, invocationID)
	if resultChannel == nil {
		return statev2.WorkSnapshot{}, errors.New("runtime coordinator returned no result")
	}
	result, resultOK := <-resultChannel
	if !resultOK {
		return statev2.WorkSnapshot{}, errors.New("runtime coordinator returned no result")
	}
	if result.Snapshot.WorkID != "" {
		snapshot = result.Snapshot
	}
	if result.Err != nil {
		latest, loadErr := s.state.Load(ctx, workID)
		if loadErr == nil {
			snapshot = latest
		}
		if durableRuntimeBoundary(snapshot, task.TaskID, invocationID) {
			return snapshot, nil
		}
		return statev2.WorkSnapshot{}, result.Err
	}
	latest, loadErr := s.state.Load(ctx, workID)
	if loadErr != nil {
		return snapshot, loadErr
	}
	return latest, nil
}

func validateForegroundDependencies(s *ForegroundService) error {
	if s == nil || s.state == nil || s.preparer == nil || s.runtime == nil {
		return errors.New("foreground work dependencies are required")
	}
	return nil
}

func validateForegroundInput(binding ForegroundBinding, workID contractv2.WorkID, expected contractv2.Revision, requestID contractv2.RequestID) error {
	if !foregroundID(string(workID)) || !foregroundID(string(requestID)) {
		return errors.New("work and request IDs are required")
	}
	if expected == 0 {
		return errors.New("expected revision must be positive")
	}
	for name, value := range map[string]string{"repository path": binding.RepositoryPath, "worktree root": binding.WorktreeRoot, "runtime fingerprint": binding.RuntimeFingerprint} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%s is required", name)
		}
	}
	if !filepath.IsAbs(binding.RepositoryPath) || filepath.Clean(binding.RepositoryPath) != binding.RepositoryPath || !filepath.IsAbs(binding.WorktreeRoot) || filepath.Clean(binding.WorktreeRoot) != binding.WorktreeRoot {
		return errors.New("foreground binding paths must be canonical absolute paths")
	}
	repository, err := canonicalPreparationPath(binding.RepositoryPath, false)
	if err != nil || repository != binding.RepositoryPath {
		return errors.New("repository path is not canonical")
	}
	root, err := canonicalPreparationPath(binding.WorktreeRoot, true)
	if err != nil || root != binding.WorktreeRoot {
		return errors.New("worktree root is not canonical")
	}
	return nil
}

func validateFreshSnapshot(snapshot statev2.WorkSnapshot, workID contractv2.WorkID, expected contractv2.Revision) error {
	if snapshot.WorkID != workID || snapshot.Contract.WorkID != workID {
		return errors.New("work snapshot identity does not match request")
	}
	if snapshot.Revision != expected {
		return errors.New("stale work revision")
	}
	if len(snapshot.Contract.RepositoryPlans) != 1 {
		return errors.New("foreground work requires exactly one repository plan")
	}
	if snapshot.Control.ApprovedContractHash == "" || snapshot.Control.ApprovalRef == "" || snapshot.Control.ApprovedContractHash != snapshot.ContractHash {
		return errors.New("work snapshot is not approved")
	}
	if snapshot.Control.PauseRequested || snapshot.Control.Blocker != nil || snapshot.State == statev2.StatePaused || snapshot.State == statev2.StateNeedsOperator || snapshot.State == statev2.StateAwaitingApproval {
		return errors.New("work is paused or requires operator action")
	}
	for _, task := range snapshot.TaskStates {
		if task.Status == statev2.TaskNeedsOperator {
			return errors.New("work task requires operator action")
		}
	}
	return nil
}

func foregroundID(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, "\r\n") && !strings.ContainsFunc(value, unicode.IsSpace)
}

func replayInvocation(snapshot statev2.WorkSnapshot, invocationID statev2.InvocationID) bool {
	for _, task := range snapshot.TaskStates {
		if task.Invocation != nil && task.Invocation.InvocationID == invocationID {
			return true
		}
		for _, historical := range task.InvocationHistory {
			if historical == invocationID {
				return true
			}
		}
	}
	return false
}

func outstandingLifecycle(snapshot statev2.WorkSnapshot) bool {
	for _, task := range snapshot.TaskStates {
		switch task.Status {
		case statev2.TaskWorktreePreparing, statev2.TaskInvocationReserved, statev2.TaskRunning, statev2.TaskTerminationPending, statev2.TaskTerminated, statev2.TaskCandidateReady, statev2.TaskNeedsOperator:
			return true
		}
		if task.Candidate != nil || task.Invocation != nil && task.Status != statev2.TaskPending {
			return true
		}
	}
	return false
}

func selectForegroundTask(snapshot statev2.WorkSnapshot) (*contractv2.Task, *contractv2.RepositoryPlan, error) {
	plan := &snapshot.Contract.RepositoryPlans[0]
	for i := range snapshot.Contract.Tasks {
		candidate := &snapshot.Contract.Tasks[i]
		if candidate.RepoKey != plan.RepoKey || !validBranch(candidate.Branch) {
			if candidate.RepoKey == plan.RepoKey && snapshot.TaskStates[candidate.TaskID].Status == statev2.TaskPending {
				return nil, nil, errors.New("pending task branch is missing or invalid")
			}
			continue
		}
		state, ok := snapshot.TaskStates[candidate.TaskID]
		if !ok || state.Status != statev2.TaskPending {
			continue
		}
		ready := true
		for _, dependencyID := range candidate.DependsOn {
			dependency, ok := snapshot.TaskStates[dependencyID]
			if !ok || dependency.Status != statev2.TaskIntegrated {
				ready = false
				break
			}
		}
		if ready {
			return candidate, plan, nil
		}
	}
	return nil, nil, errors.New("no dependency-ready pending task")
}

func reservedPreparationMatches(snapshot statev2.WorkSnapshot, request PreparationRequest, task contractv2.Task) bool {
	state, ok := snapshot.TaskStates[request.TaskID]
	if !ok || state.Status != statev2.TaskInvocationReserved || state.Invocation == nil || state.Worktree == nil || state.LogicalWork == nil {
		return false
	}
	return state.Invocation.InvocationID == request.InvocationID && state.Invocation.LogicalWorkID == request.LogicalWorkID && state.Invocation.Role == "builder" && state.Invocation.ReturnStage == statev2.TaskPending && state.Invocation.LogicalProfile == snapshot.Contract.ExecutionProfiles.Builder && state.Invocation.RuntimeFingerprint == request.RuntimeFingerprint && state.LogicalWork.LogicalWorkID == request.LogicalWorkID && state.LogicalWork.Role == "builder" && state.LogicalWork.LogicalProfile == snapshot.Contract.ExecutionProfiles.Builder && state.LogicalWork.RuntimeFingerprint == request.RuntimeFingerprint && state.Worktree.CanonicalPath == request.WorktreePath && state.Worktree.Branch == request.Branch && state.Worktree.BaseSHA == request.BaseSHA && task.Branch == request.Branch
}

func durableRuntimeBoundary(snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, invocationID statev2.InvocationID) bool {
	task, ok := snapshot.TaskStates[taskID]
	if !ok || task.Invocation == nil || task.Invocation.InvocationID != invocationID {
		return false
	}
	switch task.Status {
	case statev2.TaskInvocationReserved, statev2.TaskRunning, statev2.TaskCandidateReady, statev2.TaskNeedsOperator:
		return true
	default:
		return false
	}
}

func foregroundIDs(workID contractv2.WorkID, requestID contractv2.RequestID) (statev2.InvocationID, statev2.LogicalWorkID) {
	invocation := foregroundHash("run", string(workID), string(requestID))
	logical := foregroundHash("logical", string(workID), string(requestID))
	return statev2.InvocationID(invocation), statev2.LogicalWorkID(logical)
}

func foregroundWorktreeID(workID contractv2.WorkID, taskID contractv2.TaskID) string {
	return foregroundHash("worktree", string(workID), string(taskID))
}

func foregroundHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}
