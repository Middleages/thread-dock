// Package workrun contains the narrow orchestration steps used to start a
// task without coupling work execution to a runtime provider.
package workrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

type State interface {
	Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error)
	Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error)
}

type TaskWorktreeGit interface {
	InspectTaskWorktree(context.Context, string, string, string, string) (worktree.TaskWorktreeInspection, error)
	CreateManagedWorktree(context.Context, string, string, string, string) error
}

type PreparationRequest struct {
	WorkID             contractv2.WorkID
	TaskID             contractv2.TaskID
	InvocationID       statev2.InvocationID
	LogicalWorkID      statev2.LogicalWorkID
	RepositoryPath     string
	WorktreePath       string
	Branch             string
	BaseSHA            string
	RuntimeFingerprint string
}

type PreparationService struct {
	state       State
	git         TaskWorktreeGit
	operatorRef string
	now         func() time.Time
}

func NewPreparationService(state State, git TaskWorktreeGit, operatorRef string, now func() time.Time) *PreparationService {
	if now == nil {
		now = time.Now
	}
	return &PreparationService{state: state, git: git, operatorRef: operatorRef, now: now}
}

func (s *PreparationService) Prepare(ctx context.Context, request PreparationRequest) (statev2.WorkSnapshot, error) {
	if s == nil || s.state == nil || s.git == nil {
		return statev2.WorkSnapshot{}, errors.New("preparation dependencies are required")
	}
	if err := validatePreparationRequest(request); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	canonicalRepository, repositoryErr := canonicalPreparationPath(request.RepositoryPath, false)
	canonicalWorktree, worktreeErr := canonicalPreparationPath(request.WorktreePath, true)
	if repositoryErr != nil || worktreeErr != nil || canonicalRepository != request.RepositoryPath || canonicalWorktree != request.WorktreePath {
		return statev2.WorkSnapshot{}, errors.New("preparation request contains a noncanonical path")
	}
	if strings.TrimSpace(s.operatorRef) == "" {
		return statev2.WorkSnapshot{}, errors.New("operator reference is required")
	}
	snapshot, err := s.state.Load(ctx, request.WorkID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	task, _, err := validateSnapshotRequest(snapshot, request)
	if err != nil {
		return snapshot, err
	}
	dependencies := integratedDependencies(snapshot, *task)
	profile := snapshot.Contract.ExecutionProfiles.Builder
	identity := statev2.WorktreeIdentity{CanonicalPath: request.WorktreePath, Branch: request.Branch, BaseSHA: request.BaseSHA, IntegratedDependencies: dependencies}
	if task.Status == statev2.TaskInvocationReserved {
		if task.Invocation == nil || task.Worktree == nil || task.Invocation.LogicalWorkID != request.LogicalWorkID || task.Invocation.RuntimeFingerprint != request.RuntimeFingerprint || task.Worktree.CanonicalPath != request.WorktreePath || task.Worktree.Branch != request.Branch || task.Worktree.BaseSHA != request.BaseSHA || !sameDependencies(task.Worktree.IntegratedDependencies, dependencies) {
			return snapshot, errors.New("reserved task identity does not match preparation request")
		}
		return snapshot, nil
	}
	if task.Status == statev2.TaskWorktreePreparing {
		return s.reconcilePreparing(ctx, snapshot, *task, request)
	}
	if task.Status != statev2.TaskPending {
		return snapshot, fmt.Errorf("task stage %q cannot be prepared", task.Status)
	}
	observation, inspectErr := s.inspect(ctx, request.RepositoryPath, request.WorktreePath, request.Branch, request.BaseSHA)
	if inspectErr != nil || observation.CanonicalPath != request.WorktreePath {
		return s.persistBlocker(ctx, snapshot, *task, request, nil, "worktree evidence could not be established")
	}
	if observation.Exists && !observation.IdentityMatches {
		return s.persistBlocker(ctx, snapshot, *task, request, nil, "worktree identity evidence mismatch")
	}
	if observation.GitCommonDir == "" {
		return s.persistBlocker(ctx, snapshot, *task, request, nil, "worktree evidence could not be established")
	}
	identity.GitCommonDir = observation.GitCommonDir
	begin := statev2.TaskTransition{
		TaskID: request.TaskID, Action: statev2.TaskBeginWorktreePreparation, InvocationID: request.InvocationID,
		LogicalWorkID: request.LogicalWorkID, Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1,
		At: s.timestamp(), Worktree: &identity, Invocation: &statev2.InvocationState{LogicalProfile: profile, RuntimeFingerprint: request.RuntimeFingerprint},
	}
	prepared, err := s.apply(ctx, snapshot, begin)
	if err != nil {
		return snapshot, err
	}
	if observation.IdentityMatches {
		return s.reconcile(ctx, prepared, prepared.TaskStates[request.TaskID], statev2.WorktreePreparationEvidence{OperationTerminated: true, WorktreeExists: true, IdentityMatches: true, Diagnostic: "worktree preparation inspection matched"})
	}
	createErr := s.git.CreateManagedWorktree(ctx, request.RepositoryPath, request.WorktreePath, request.Branch, request.BaseSHA)
	post, postErr := s.inspect(ctx, request.RepositoryPath, request.WorktreePath, request.Branch, request.BaseSHA)
	if postErr == nil && post.CanonicalPath == request.WorktreePath && post.Exists && post.IdentityMatches {
		return s.reconcile(ctx, prepared, prepared.TaskStates[request.TaskID], statev2.WorktreePreparationEvidence{OperationTerminated: true, WorktreeExists: true, IdentityMatches: true, Diagnostic: "worktree preparation inspection matched"})
	}
	if postErr == nil && post.CanonicalPath == request.WorktreePath && !post.Exists {
		settled, settleErr := s.reconcile(ctx, prepared, prepared.TaskStates[request.TaskID], statev2.WorktreePreparationEvidence{OperationTerminated: true, Diagnostic: "worktree preparation target is missing"})
		if settleErr != nil {
			return prepared, settleErr
		}
		if createErr != nil {
			return settled, errors.New("worktree preparation failed")
		}
		return settled, errors.New("worktree preparation completed without a target")
	}
	settled, settleErr := s.persistBlocker(ctx, prepared, prepared.TaskStates[request.TaskID], request, prepared.TaskStates[request.TaskID].Invocation, "worktree identity evidence mismatch")
	if settleErr != nil {
		return prepared, settleErr
	}
	if createErr != nil {
		return settled, errors.New("worktree preparation failed")
	}
	return settled, errors.New("worktree preparation produced untrusted evidence")
}

func (s *PreparationService) reconcilePreparing(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, request PreparationRequest) (statev2.WorkSnapshot, error) {
	if task.Invocation == nil || task.Worktree == nil {
		return s.persistBlocker(ctx, snapshot, task, request, task.Invocation, "worktree preparation identity is incomplete")
	}
	observation, inspectErr := s.inspect(ctx, request.RepositoryPath, task.Worktree.CanonicalPath, task.Worktree.Branch, task.Worktree.BaseSHA)
	if inspectErr == nil && observation.CanonicalPath == task.Worktree.CanonicalPath && observation.Exists && observation.IdentityMatches {
		return s.reconcile(ctx, snapshot, task, statev2.WorktreePreparationEvidence{OperationTerminated: true, WorktreeExists: true, IdentityMatches: true, Diagnostic: "worktree preparation inspection matched"})
	}
	if inspectErr == nil && observation.CanonicalPath == task.Worktree.CanonicalPath && !observation.Exists {
		return s.reconcile(ctx, snapshot, task, statev2.WorktreePreparationEvidence{OperationTerminated: true, Diagnostic: "worktree preparation target is missing"})
	}
	return s.persistBlocker(ctx, snapshot, task, request, task.Invocation, "worktree identity evidence mismatch")
}

func (s *PreparationService) reconcile(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, evidence statev2.WorktreePreparationEvidence) (statev2.WorkSnapshot, error) {
	tr := statev2.TaskTransition{TaskID: task.TaskID, Action: statev2.TaskReconcileWorktreePreparation, InvocationID: task.Invocation.InvocationID, LogicalWorkID: task.Invocation.LogicalWorkID, Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: task.BuilderAttempt, At: s.timestamp(), Preparation: &evidence}
	return s.apply(ctx, snapshot, tr)
}

func (s *PreparationService) persistBlocker(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, request PreparationRequest, invocation *statev2.InvocationState, diagnostic string) (statev2.WorkSnapshot, error) {
	tr := statev2.TaskTransition{TaskID: task.TaskID, Action: statev2.TaskNeedsOperatorAction, Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: task.BuilderAttempt, At: s.timestamp(), Blocker: &statev2.OperatorBlocker{Kind: "evidence_mismatch", OperatorRef: s.operatorRef, TaskID: task.TaskID, Diagnostic: diagnostic}}
	if invocation != nil {
		tr.InvocationID, tr.LogicalWorkID = invocation.InvocationID, invocation.LogicalWorkID
		tr.ReturnStage, tr.BuilderAttempt = invocation.ReturnStage, task.BuilderAttempt
	}
	updated, err := s.apply(ctx, snapshot, tr)
	if err != nil {
		return snapshot, err
	}
	return updated, errors.New("worktree evidence mismatch requires operator action")
}

func (s *PreparationService) inspect(ctx context.Context, repositoryPath, worktreePath, branch, base string) (worktree.TaskWorktreeInspection, error) {
	observation, err := s.git.InspectTaskWorktree(ctx, repositoryPath, worktreePath, branch, base)
	if err != nil {
		return worktree.TaskWorktreeInspection{}, errors.New("unsafe worktree inspection")
	}
	return observation, nil
}

func (s *PreparationService) apply(ctx context.Context, snapshot statev2.WorkSnapshot, transition statev2.TaskTransition) (statev2.WorkSnapshot, error) {
	requestID, err := newRequestID()
	if err != nil {
		return snapshot, err
	}
	req := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Task: &transition}
	req.PayloadHash, err = statev2.TransitionPayloadHash(req)
	if err != nil {
		return snapshot, err
	}
	return s.state.Apply(ctx, req)
}

func (s *PreparationService) timestamp() time.Time {
	at := s.now()
	if at.IsZero() {
		at = time.Now()
	}
	return at.UTC()
}

func newRequestID() (contractv2.RequestID, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return contractv2.RequestID("prep-" + hex.EncodeToString(data)), nil
}

func validatePreparationRequest(request PreparationRequest) error {
	for name, value := range map[string]string{"work ID": string(request.WorkID), "task ID": string(request.TaskID), "invocation ID": string(request.InvocationID), "logical work ID": string(request.LogicalWorkID), "repository path": request.RepositoryPath, "worktree path": request.WorktreePath, "branch": request.Branch, "base SHA": request.BaseSHA, "runtime fingerprint": request.RuntimeFingerprint} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%s is required", name)
		}
	}
	if !filepath.IsAbs(request.RepositoryPath) || filepath.Clean(request.RepositoryPath) != request.RepositoryPath || !filepath.IsAbs(request.WorktreePath) || filepath.Clean(request.WorktreePath) != request.WorktreePath || len(request.BaseSHA) != 40 || strings.ToLower(request.BaseSHA) != request.BaseSHA || !isHex(request.BaseSHA) || !validBranch(request.Branch) {
		return errors.New("preparation request contains a noncanonical path or invalid Git identity")
	}
	return nil
}

func validateSnapshotRequest(snapshot statev2.WorkSnapshot, request PreparationRequest) (*statev2.TaskExecutionState, *contractv2.RepositoryPlan, error) {
	if snapshot.WorkID != request.WorkID || snapshot.Contract.WorkID != request.WorkID {
		return nil, nil, errors.New("work snapshot identity does not match request")
	}
	if snapshot.Control.ApprovedContractHash == "" || snapshot.Control.ApprovalRef == "" || snapshot.Control.ApprovedContractHash != snapshot.ContractHash {
		return nil, nil, errors.New("work snapshot is not approved")
	}
	task, ok := snapshot.TaskStates[request.TaskID]
	if !ok || task.TaskID != request.TaskID {
		return nil, nil, errors.New("task is not in work snapshot")
	}
	if task.Status != statev2.TaskPending && task.Status != statev2.TaskWorktreePreparing && task.Status != statev2.TaskInvocationReserved {
		return nil, nil, fmt.Errorf("task stage %q cannot be prepared", task.Status)
	}
	contractTask := findTask(snapshot, request.TaskID)
	if contractTask == nil || contractTask.RepoKey == "" {
		return nil, nil, errors.New("task repository plan is required")
	}
	var plan *contractv2.RepositoryPlan
	for i := range snapshot.Contract.RepositoryPlans {
		if snapshot.Contract.RepositoryPlans[i].RepoKey == contractTask.RepoKey {
			plan = &snapshot.Contract.RepositoryPlans[i]
			break
		}
	}
	if plan == nil || plan.BaseSHA != request.BaseSHA || plan.TargetBranch != request.Branch {
		return nil, nil, errors.New("task repository plan does not match preparation request")
	}
	if contractTask.Branch != "" && contractTask.Branch != request.Branch {
		return nil, nil, errors.New("task branch does not match preparation request")
	}
	return &task, plan, nil
}

func findTask(snapshot statev2.WorkSnapshot, id contractv2.TaskID) *contractv2.Task {
	for i := range snapshot.Contract.Tasks {
		if snapshot.Contract.Tasks[i].TaskID == id {
			return &snapshot.Contract.Tasks[i]
		}
	}
	return nil
}

func integratedDependencies(snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState) map[contractv2.TaskID]string {
	result := map[contractv2.TaskID]string{}
	contractTask := findTask(snapshot, task.TaskID)
	if contractTask == nil {
		return result
	}
	for _, id := range contractTask.DependsOn {
		if dep, ok := snapshot.TaskStates[id]; ok && dep.Integration != nil {
			result[id] = dep.Integration.IntegrationHEAD
		}
	}
	return result
}

func sameDependencies(left, right map[contractv2.TaskID]string) bool {
	if len(left) != len(right) {
		return false
	}
	for id, head := range left {
		if right[id] != head {
			return false
		}
	}
	return true
}

func isHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func validBranch(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, " ~^:?*[\\\x00\r\n") && !strings.Contains(value, "..") && !strings.HasSuffix(value, "/")
}

func canonicalPreparationPath(path string, allowMissingLeaf bool) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("path is not canonical")
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("path is a symlink")
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil {
			return "", resolveErr
		}
		return filepath.Clean(resolved), nil
	}
	if !allowMissingLeaf || !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	suffix := []string{filepath.Base(path)}
	for {
		parentInfo, statErr := os.Lstat(parent)
		if statErr == nil {
			if parentInfo.Mode()&os.ModeSymlink != 0 {
				resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
				if resolveErr != nil {
					return "", resolveErr
				}
				parent = resolvedParent
			} else {
				resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
				if resolveErr != nil {
					return "", resolveErr
				}
				parent = resolvedParent
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				parent = filepath.Join(parent, suffix[i])
			}
			return filepath.Clean(parent), nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		suffix = append(suffix, filepath.Base(parent))
		next := filepath.Dir(parent)
		if next == parent {
			return "", os.ErrNotExist
		}
		parent = next
	}
}
