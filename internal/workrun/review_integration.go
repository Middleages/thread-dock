package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

type ReviewIntegrationRuntime interface {
	Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error)
	SubmitRuntime(context.Context, contractv2.WorkID, contractv2.TaskID, statev2.InvocationID) <-chan coordinator.CommandResult
	Close(context.Context) error
}

type ReviewIntegrationGit interface {
	CreateManagedWorktree(context.Context, string, string, string, string) error
	ReconcileIntegrationWorktree(context.Context, string, string, string) (bool, error)
	MergeCommitNoFF(context.Context, string, string) error
	AbortMerge(context.Context, string) error
	CurrentCommit(context.Context, string) (string, error)
	IsAncestor(context.Context, string, string) (bool, error)
}

type ReviewIntegrationBinding struct {
	RepositoryPath     string
	IntegrationPath    string
	IntegrationBranch  string
	RuntimeFingerprint string
}

type ReviewIntegrationService struct {
	state       State
	runtime     ReviewIntegrationRuntime
	git         ReviewIntegrationGit
	operatorRef string
	now         func() time.Time
	binding     ReviewIntegrationBinding
}

func NewReviewIntegrationService(state State, runtime ReviewIntegrationRuntime, git ReviewIntegrationGit, operatorRef string, now func() time.Time, binding ReviewIntegrationBinding) *ReviewIntegrationService {
	if now == nil {
		now = time.Now
	}
	return &ReviewIntegrationService{state: state, runtime: runtime, git: git, operatorRef: operatorRef, now: now, binding: binding}
}

func (s *ReviewIntegrationService) Advance(ctx context.Context, supplied statev2.WorkSnapshot, taskID contractv2.TaskID, requestID contractv2.RequestID) (statev2.WorkSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.state == nil || s.runtime == nil || s.git == nil {
		return statev2.WorkSnapshot{}, errors.New("review integration dependencies are required")
	}
	if strings.TrimSpace(string(taskID)) == "" || strings.TrimSpace(string(requestID)) == "" || strings.TrimSpace(s.operatorRef) == "" {
		return statev2.WorkSnapshot{}, errors.New("review integration identity is required")
	}
	if strings.TrimSpace(s.binding.RepositoryPath) == "" || strings.TrimSpace(s.binding.IntegrationPath) == "" || strings.TrimSpace(s.binding.IntegrationBranch) == "" || strings.TrimSpace(s.binding.RuntimeFingerprint) == "" {
		return statev2.WorkSnapshot{}, errors.New("review integration binding is required")
	}
	if err := ctx.Err(); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	current, err := s.state.Load(ctx, supplied.WorkID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if err := validateReviewIntegrationSnapshot(supplied, current, taskID); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	task, ok := current.TaskStates[taskID]
	if !ok {
		return current, errors.New("task is not in contract")
	}
	if task.Status == statev2.TaskIntegrated {
		return current, nil
	}
	if task.Status == statev2.TaskGatePassed && task.Review == nil {
		current, err = s.launchReviewer(ctx, current, taskID, requestID)
		if err != nil {
			return current, err
		}
	}
	current, err = s.state.Load(ctx, current.WorkID)
	if err != nil {
		return current, err
	}
	task = current.TaskStates[taskID]
	if task.Status == statev2.TaskIntegrated {
		return current, nil
	}
	if task.Status != statev2.TaskAccepted || task.Review == nil || !task.Review.Accepted || task.Candidate == nil {
		return current, fmt.Errorf("task is not accepted for integration")
	}
	return s.integrate(ctx, current, taskID)
}

func validateReviewIntegrationSnapshot(supplied, current statev2.WorkSnapshot, taskID contractv2.TaskID) error {
	if supplied.WorkID == "" || supplied.WorkID != current.WorkID || supplied.Revision != current.Revision {
		return errors.New("review integration snapshot is stale")
	}
	suppliedTask, suppliedOK := supplied.TaskStates[taskID]
	currentTask, currentOK := current.TaskStates[taskID]
	if !suppliedOK || !currentOK || !reflect.DeepEqual(suppliedTask, currentTask) {
		return errors.New("review integration task snapshot is stale")
	}
	return nil
}

func (s *ReviewIntegrationService) launchReviewer(ctx context.Context, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, callerRequestID contractv2.RequestID) (resultSnapshot statev2.WorkSnapshot, resultErr error) {
	resultSnapshot = snapshot
	activated := false
	defer func() {
		if activated {
			if closeErr := s.runtime.Close(ctx); resultErr == nil && closeErr != nil {
				resultErr = closeErr
			}
		}
	}()
	task := snapshot.TaskStates[taskID]
	if task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Worktree == nil {
		return snapshot, errors.New("gate-passed candidate evidence is required")
	}
	if task.Invocation != nil && task.Invocation.Role != "reviewer" {
		return snapshot, errors.New("reviewer invocation identity is invalid")
	}
	invocationID, logicalWorkID := reviewIDs(snapshot.WorkID, taskID, callerRequestID)
	if task.Invocation == nil {
		contractTask, found := contractTaskByID(snapshot.Contract, taskID)
		if !found {
			return snapshot, errors.New("task is not in contract")
		}
		transition := statev2.TaskTransition{
			TaskID: taskID, Action: statev2.TaskReserveInvocation, InvocationID: invocationID,
			LogicalWorkID: logicalWorkID, Role: "reviewer", ReturnStage: statev2.TaskGatePassed,
			BuilderAttempt: task.BuilderAttempt, At: s.timestamp(), Worktree: cloneReviewWorktree(task.Worktree),
			Invocation: &statev2.InvocationState{LogicalProfile: snapshot.Contract.ExecutionProfiles.Reviewer, RuntimeFingerprint: s.binding.RuntimeFingerprint},
		}
		if strings.TrimSpace(string(contractTask.TaskID)) == "" || strings.TrimSpace(transition.Invocation.LogicalProfile) == "" {
			return snapshot, errors.New("reviewer contract profile is required")
		}
		var err error
		snapshot, err = s.applyReviewTransition(ctx, snapshot, transition, "review-reserve", callerRequestID)
		if err != nil {
			return snapshot, err
		}
	}
	if task := snapshot.TaskStates[taskID]; task.Invocation == nil || task.Invocation.Role != "reviewer" {
		return snapshot, errors.New("reviewer invocation was not reserved")
	} else {
		if _, activateErr := s.runtime.Activate(ctx, snapshot.WorkID); activateErr != nil {
			return snapshot, activateErr
		}
		activated = true
		resultChannel := s.runtime.SubmitRuntime(ctx, snapshot.WorkID, taskID, task.Invocation.InvocationID)
		if resultChannel == nil {
			return snapshot, errors.New("runtime coordinator returned no result")
		}
		result, ok := <-resultChannel
		if !ok {
			return snapshot, errors.New("runtime coordinator returned no result")
		}
		if result.Snapshot.WorkID != "" {
			snapshot = result.Snapshot
		}
		if result.Err != nil {
			latest, loadErr := s.state.Load(ctx, snapshot.WorkID)
			if loadErr == nil {
				snapshot = latest
			}
			return snapshot, result.Err
		}
	}
	return snapshot, nil
}

func (s *ReviewIntegrationService) integrate(ctx context.Context, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID) (statev2.WorkSnapshot, error) {
	task := snapshot.TaskStates[taskID]
	base, err := repositoryBase(snapshot.Contract, taskID)
	if err != nil {
		return snapshot, err
	}
	ready, reconcileErr := s.git.ReconcileIntegrationWorktree(ctx, s.binding.IntegrationPath, s.binding.IntegrationBranch, base)
	if reconcileErr != nil {
		return snapshot, reconcileErr
	}
	if !ready {
		if err := s.git.CreateManagedWorktree(ctx, s.binding.RepositoryPath, s.binding.IntegrationPath, s.binding.IntegrationBranch, base); err != nil {
			return snapshot, err
		}
	}
	if err := s.git.MergeCommitNoFF(ctx, s.binding.IntegrationPath, task.Candidate.CandidateSHA); err != nil {
		if errors.Is(err, worktree.ErrConflict) {
			_ = s.git.AbortMerge(ctx, s.binding.IntegrationPath)
			return s.recordIntegrationBlocker(ctx, snapshot, task, "integration merge conflict")
		}
		return snapshot, err
	}
	head, err := s.git.CurrentCommit(ctx, s.binding.IntegrationPath)
	if err != nil {
		return snapshot, err
	}
	if head == task.Candidate.CandidateSHA {
		return snapshot, errors.New("integration head must differ from candidate")
	}
	relation, err := s.git.IsAncestor(ctx, s.binding.IntegrationPath, task.Candidate.CandidateSHA)
	if err != nil || !relation {
		if err == nil {
			err = errors.New("candidate is not an ancestor of integration head")
		}
		return snapshot, err
	}
	at := s.timestamp()
	transition := statev2.TaskTransition{TaskID: taskID, Action: statev2.TaskRecordIntegration, InvocationID: task.Invocation.InvocationID, LogicalWorkID: task.Invocation.LogicalWorkID, Role: "builder", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: task.BuilderAttempt, At: at, Integration: &statev2.IntegrationEvidence{BuilderAttempt: task.BuilderAttempt, CandidateSHA: task.Candidate.CandidateSHA, IntegrationHEAD: head, RelationVerified: true, ObservedAt: at}}
	updated, err := s.applyReviewTransition(ctx, snapshot, transition, "review-integration", contractv2.RequestID(task.Invocation.InvocationID))
	if err != nil {
		return snapshot, err
	}
	return updated, nil
}

func (s *ReviewIntegrationService) recordIntegrationBlocker(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, diagnostic string) (statev2.WorkSnapshot, error) {
	if task.Invocation == nil {
		return snapshot, errors.New("reviewer invocation is required for integration blocker")
	}
	at := s.timestamp()
	transition := statev2.TaskTransition{TaskID: task.TaskID, Action: statev2.TaskNeedsOperatorAction, InvocationID: task.Invocation.InvocationID, LogicalWorkID: task.Invocation.LogicalWorkID, Role: task.Invocation.Role, ReturnStage: task.Invocation.ReturnStage, BuilderAttempt: task.BuilderAttempt, At: at, Blocker: &statev2.OperatorBlocker{Kind: statev2.BlockerKindEvidenceMismatch, OperatorRef: s.operatorRef, TaskID: task.TaskID, InvocationID: task.Invocation.InvocationID, Diagnostic: diagnostic}}
	return s.applyReviewTransition(ctx, snapshot, transition, "review-integration-conflict", contractv2.RequestID(task.Invocation.InvocationID))
}

func (s *ReviewIntegrationService) applyReviewTransition(ctx context.Context, snapshot statev2.WorkSnapshot, transition statev2.TaskTransition, prefix string, seed contractv2.RequestID) (statev2.WorkSnapshot, error) {
	requestID := deterministicReviewRequest(prefix, snapshot.WorkID, transition.TaskID, seed, transition.Action)
	request := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Task: &transition}
	hash, err := statev2.TransitionPayloadHash(request)
	if err != nil {
		return snapshot, err
	}
	request.PayloadHash = hash
	return s.state.Apply(ctx, request)
}

func reviewIDs(workID contractv2.WorkID, taskID contractv2.TaskID, requestID contractv2.RequestID) (statev2.InvocationID, statev2.LogicalWorkID) {
	sum := sha256.Sum256([]byte(string(workID) + "\x00" + string(taskID) + "\x00" + string(requestID)))
	encoded := hex.EncodeToString(sum[:])
	return statev2.InvocationID("review-" + encoded), statev2.LogicalWorkID("review-work-" + encoded)
}

func deterministicReviewRequest(prefix string, workID contractv2.WorkID, taskID contractv2.TaskID, seed contractv2.RequestID, action statev2.TaskAction) contractv2.RequestID {
	sum := sha256.Sum256([]byte(prefix + "\x00" + string(workID) + "\x00" + string(taskID) + "\x00" + string(seed) + "\x00" + string(action)))
	return contractv2.RequestID(prefix + "-" + hex.EncodeToString(sum[:]))
}

func (s *ReviewIntegrationService) timestamp() time.Time {
	at := s.now()
	if at.IsZero() {
		at = time.Now()
	}
	return at.UTC()
}

func cloneReviewWorktree(worktree *statev2.WorktreeIdentity) *statev2.WorktreeIdentity {
	if worktree == nil {
		return nil
	}
	clone := *worktree
	if worktree.IntegratedDependencies != nil {
		clone.IntegratedDependencies = make(map[contractv2.TaskID]string, len(worktree.IntegratedDependencies))
		for id, head := range worktree.IntegratedDependencies {
			clone.IntegratedDependencies[id] = head
		}
	}
	return &clone
}

func contractTaskByID(contract contractv2.WorkItemContract, taskID contractv2.TaskID) (contractv2.Task, bool) {
	for _, task := range contract.Tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return contractv2.Task{}, false
}

func repositoryBase(contract contractv2.WorkItemContract, taskID contractv2.TaskID) (string, error) {
	task, ok := contractTaskByID(contract, taskID)
	if !ok {
		return "", errors.New("task is not in contract")
	}
	for _, plan := range contract.RepositoryPlans {
		if plan.RepoKey == task.RepoKey {
			return plan.BaseSHA, nil
		}
	}
	return "", errors.New("task repository plan is required")
}
