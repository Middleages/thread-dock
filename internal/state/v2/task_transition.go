package statev2

import (
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

const (
	roleBuilder  = "builder"
	roleReviewer = "reviewer"
)

func applyTaskTransition(snapshot *WorkSnapshot, transition TaskTransition, requestID contractv2.RequestID) error {
	if transition.At.IsZero() || transition.At.Location() != time.UTC {
		return invalidTransition("task transition time must be a nonzero UTC time")
	}
	if transition.TaskID == "" {
		return invalidTransition("task ID is required")
	}
	task, ok := snapshot.TaskStates[transition.TaskID]
	if !ok {
		return invalidTransition("task is not in contract")
	}
	if snapshot.Contract.WorkID != snapshot.WorkID || task.TaskID != transition.TaskID {
		return invalidTransition("task identity does not match contract")
	}
	if transition.Role != roleBuilder && transition.Role != roleReviewer {
		return invalidTransition("invalid invocation role")
	}
	if snapshot.Control.ApprovedContractHash == "" {
		return invalidTransition("work is not approved")
	}
	if snapshot.Control.Blocker != nil {
		return invalidTransition("work has an operator blocker")
	}
	switch transition.Action {
	case TaskReserveInvocation:
		return reserveInvocation(snapshot, &task, transition, requestID)
	case TaskBeginLaunch:
		if err := beginLaunch(&task, transition); err != nil {
			return err
		}
	case TaskMarkRunning:
		if err := markRunning(&task, transition); err != nil {
			return err
		}
	case TaskRequestTermination:
		if err := requestTermination(&task, transition); err != nil {
			return err
		}
	case TaskConfirmTermination:
		if err := confirmTermination(&task, transition); err != nil {
			return err
		}
	case TaskReconcileNotStarted:
		if err := reconcileNotStarted(&task, transition); err != nil {
			return err
		}
	case TaskNeedsOperatorAction:
		return needsOperator(snapshot, &task, transition)
	default:
		return invalidTransition("unsupported task action %q", transition.Action)
	}
	snapshot.TaskStates[task.TaskID] = task
	reduce(snapshot)
	return nil
}

func reserveInvocation(snapshot *WorkSnapshot, task *TaskExecutionState, transition TaskTransition, requestID contractv2.RequestID) error {
	if snapshot.State == StateAwaitingApproval || snapshot.State == StateDraft || snapshot.State == StatePaused || snapshot.State == StateNeedsOperator || snapshot.State == StateCompleted {
		return invalidTransition("work cannot dispatch in state %q", snapshot.State)
	}
	if snapshot.Control.PauseRequested {
		return invalidTransition("work pause is requested")
	}
	if transition.InvocationID == "" || transition.LogicalWorkID == "" || transition.Invocation == nil || transition.Worktree == nil {
		return invalidTransition("invocation identity and launch inputs are required")
	}
	if strings.TrimSpace(string(transition.InvocationID)) == "" || strings.TrimSpace(string(transition.LogicalWorkID)) == "" {
		return invalidTransition("invocation identity and launch inputs are required")
	}
	for _, invocationID := range task.InvocationHistory {
		if invocationID == transition.InvocationID {
			return invalidTransition("invocation ID was already used")
		}
	}
	if task.Invocation != nil {
		return invalidTransition("task already has an invocation")
	}
	if strings.TrimSpace(transition.Invocation.LogicalProfile) == "" || strings.TrimSpace(transition.Invocation.RuntimeFingerprint) == "" {
		return invalidTransition("logical profile and runtime fingerprint are required")
	}
	if strings.TrimSpace(transition.Worktree.CanonicalPath) == "" || strings.TrimSpace(transition.Worktree.GitCommonDir) == "" || strings.TrimSpace(transition.Worktree.Branch) == "" || strings.TrimSpace(transition.Worktree.BaseSHA) == "" {
		return invalidTransition("complete worktree identity is required")
	}
	if transition.Invocation.ProviderIdentity != "" || transition.Invocation.ProviderSession != "" || transition.Invocation.ProviderPane != "" || transition.Invocation.ProviderProcess != "" {
		return invalidTransition("provider identity is not accepted when reserving")
	}
	if transition.Invocation.StartedAt != nil || transition.Invocation.EndedAt != nil || transition.Invocation.LaunchRequested || transition.Invocation.TerminationConfirmed {
		return invalidTransition("invalid initial invocation state")
	}
	if task.Status == TaskPending && task.LogicalWork == nil {
		if transition.Role != roleBuilder || transition.ReturnStage != TaskPending || transition.BuilderAttempt != 1 || task.BuilderAttempt != 0 || task.LogicalWork != nil {
			return invalidTransition("invalid initial builder reservation")
		}
		for _, dep := range taskDependencies(snapshot, task.TaskID) {
			if dep.Status != TaskIntegrated {
				return invalidTransition("task dependencies are not integrated")
			}
		}
		task.BuilderAttempt = 1
		task.LogicalWork = &LogicalWorkState{LogicalWorkID: transition.LogicalWorkID, Role: transition.Role, BuilderAttempt: 1, Purpose: "task invocation"}
	} else if task.Status == TaskGatePassed && transition.Role == roleReviewer {
		if transition.ReturnStage != TaskGatePassed || transition.BuilderAttempt != task.BuilderAttempt || task.BuilderAttempt == 0 || task.Candidate == nil || task.Gate == nil || task.LogicalWork == nil {
			return invalidTransition("invalid reviewer reservation")
		}
		if task.LogicalWork.Role == roleReviewer {
			if task.LogicalWork.LogicalWorkID != transition.LogicalWorkID || task.LogicalWork.BuilderAttempt != transition.BuilderAttempt {
				return invalidTransition("reviewer reservation does not match logical work")
			}
		} else {
			if task.LogicalWork.Role != roleBuilder && task.LogicalWork.Role != "" {
				return invalidTransition("reviewer reservation has invalid prior logical work")
			}
			if task.LogicalWork.LogicalWorkID == transition.LogicalWorkID {
				return invalidTransition("reviewer reservation must use a new logical work")
			}
			// Initial reviewer work is distinct while candidate and gate evidence
			// remain attached to the task.
			task.LogicalWork = &LogicalWorkState{LogicalWorkID: transition.LogicalWorkID, Role: transition.Role, BuilderAttempt: task.BuilderAttempt, Purpose: "task invocation"}
		}
	} else {
		if task.LogicalWork == nil || task.LogicalWork.LogicalWorkID != transition.LogicalWorkID || task.LogicalWork.Role != transition.Role || task.LogicalWork.BuilderAttempt != transition.BuilderAttempt || task.BuilderAttempt != transition.BuilderAttempt {
			return invalidTransition("reservation does not match logical work")
		}
		if transition.ReturnStage != task.LogicalWorkReturnStage() {
			return invalidTransition("reservation return stage does not match task")
		}
		if !reservationStage(task.Status, transition.Role, transition.ReturnStage) {
			return invalidTransition("task is not at a reservable stage")
		}
	}
	if transition.Role == roleReviewer && (task.Status != TaskGatePassed || transition.ReturnStage != TaskGatePassed) {
		return invalidTransition("reviewer requires gate_passed task")
	}
	if transition.Role == roleBuilder && task.Status != TaskPending && task.Status != TaskGateFailed && task.Status != TaskReviewBlocked {
		return invalidTransition("builder task is not at a reservable stage")
	}
	invocation := *transition.Invocation
	invocation.InvocationID = transition.InvocationID
	invocation.LogicalWorkID = transition.LogicalWorkID
	invocation.Role = transition.Role
	invocation.ReturnStage = transition.ReturnStage
	invocation.TransitionRequestID = requestID
	task.Worktree = cloneWorktree(transition.Worktree)
	task.Invocation = &invocation
	task.InvocationHistory = append(task.InvocationHistory, transition.InvocationID)
	task.Status = TaskInvocationReserved
	snapshot.TaskStates[task.TaskID] = *task
	reduce(snapshot)
	return nil
}

func taskDependencies(snapshot *WorkSnapshot, id contractv2.TaskID) []TaskExecutionState {
	for _, contractTask := range snapshot.Contract.Tasks {
		if contractTask.TaskID != id {
			continue
		}
		deps := make([]TaskExecutionState, 0, len(contractTask.DependsOn))
		for _, depID := range contractTask.DependsOn {
			if dep, ok := snapshot.TaskStates[depID]; ok {
				deps = append(deps, dep)
			}
		}
		return deps
	}
	return nil
}

func reservationStage(status TaskStatus, role string, stage TaskStatus) bool {
	if role == roleReviewer {
		return status == TaskGatePassed && stage == TaskGatePassed
	}
	return (status == TaskPending && stage == TaskPending) ||
		(status == TaskGateFailed && stage == TaskGateFailed) ||
		(status == TaskReviewBlocked && stage == TaskReviewBlocked)
}

func (task *TaskExecutionState) LogicalWorkReturnStage() TaskStatus {
	if task.LogicalWork == nil {
		return ""
	}
	switch task.Status {
	case TaskPending:
		return TaskPending
	case TaskGateFailed:
		return TaskGateFailed
	case TaskReviewBlocked:
		return TaskReviewBlocked
	case TaskGatePassed:
		return TaskGatePassed
	default:
		return task.InvocationReturnStage()
	}
}

func (task *TaskExecutionState) InvocationReturnStage() TaskStatus {
	if task.Invocation == nil {
		return ""
	}
	return task.Invocation.ReturnStage
}

func beginLaunch(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskInvocationReserved); err != nil {
		return err
	}
	if task.Invocation.LaunchRequested {
		return invalidTransition("launch was already requested")
	}
	task.Invocation.LaunchRequested = true
	return nil
}

func markRunning(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskInvocationReserved); err != nil {
		return err
	}
	if !task.Invocation.LaunchRequested {
		return invalidTransition("launch has not been requested")
	}
	if task.Invocation.StartedAt != nil || task.Invocation.EndedAt != nil {
		return invalidTransition("invocation timing is already recorded")
	}
	if transition.Invocation == nil || transition.Invocation.ProviderIdentity == "" && transition.Invocation.ProviderSession == "" && transition.Invocation.ProviderPane == "" && transition.Invocation.ProviderProcess == "" {
		return invalidTransition("provider identity is required")
	}
	provider := transition.Invocation
	task.Invocation.ProviderIdentity = provider.ProviderIdentity
	task.Invocation.ProviderSession = provider.ProviderSession
	task.Invocation.ProviderPane = provider.ProviderPane
	task.Invocation.ProviderProcess = provider.ProviderProcess
	at := transition.At
	task.Invocation.StartedAt = &at
	task.Status = TaskRunning
	return nil
}

func requestTermination(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskRunning); err != nil {
		return err
	}
	if strings.TrimSpace(transition.Reason) == "" || len([]byte(transition.Reason)) > MaxDiagnosticBytes {
		return invalidTransition("bounded termination reason is required")
	}
	task.Invocation.TerminationReason = transition.Reason
	task.Status = TaskTerminationPending
	return nil
}

func confirmTermination(task *TaskExecutionState, transition TaskTransition) error {
	if task.Status != TaskRunning && task.Status != TaskTerminationPending {
		return invalidTransition("task is not running")
	}
	if err := matchInvocation(task, transition, task.Status); err != nil {
		return err
	}
	if task.Invocation.TerminationConfirmed || task.Invocation.EndedAt != nil {
		return invalidTransition("invocation termination is already confirmed")
	}
	if transition.Reason != "" && len([]byte(transition.Reason)) > MaxDiagnosticBytes {
		return invalidTransition("termination reason exceeds limit")
	}
	at := transition.At
	task.Invocation.EndedAt = &at
	task.Invocation.TerminationConfirmed = true
	if transition.Reason != "" {
		task.Invocation.TerminationReason = transition.Reason
	}
	task.Status = TaskTerminated
	return nil
}

func reconcileNotStarted(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskInvocationReserved); err != nil {
		return err
	}
	if task.Invocation.LaunchRequested || hasProviderIdentity(task.Invocation) || transition.Resolution == nil || !transition.Resolution.OwnerTerminated || transition.Resolution.LaunchRequested || !transition.Resolution.ProviderAbsent {
		return invalidTransition("positive no-launch proof is required")
	}
	if err := validateDiagnostic(transition.Resolution.Diagnostic); err != nil {
		return invalidTransition("resolution diagnostic: %v", err)
	}
	if len(task.PriorAttempts) >= MaxPriorAttempts {
		return invalidTransition("prior attempt summary limit reached")
	}
	task.PriorAttempts = append(task.PriorAttempts, AttemptSummary{BuilderAttempt: task.BuilderAttempt, Outcome: "abandoned_not_started", FailureReason: transition.Reason, Diagnostic: transition.Resolution.Diagnostic})
	return restoreAfterReconcile(task)
}

func restoreAfterReconcile(task *TaskExecutionState) error {
	if task.Invocation == nil || task.LogicalWork == nil {
		return invalidTransition("invocation logical work is missing")
	}
	stage := task.Invocation.ReturnStage
	if !validReturnStage(task.Invocation.Role, stage) {
		return invalidTransition("invalid invocation return stage")
	}
	task.Status = stage
	task.Invocation = nil
	return nil
}

func needsOperator(snapshot *WorkSnapshot, task *TaskExecutionState, transition TaskTransition) error {
	blocker := transition.Blocker
	if blocker == nil || strings.TrimSpace(blocker.Kind) == "" || strings.TrimSpace(blocker.OperatorRef) == "" || strings.TrimSpace(blocker.Diagnostic) == "" || len([]byte(blocker.Diagnostic)) > MaxDiagnosticBytes || blocker.TaskID != task.TaskID {
		return invalidTransition("invalid task operator blocker")
	}
	if task.Invocation == nil {
		if blocker.InvocationID != "" || transition.InvocationID != "" {
			return invalidTransition("operator blocker invocation does not match task")
		}
	} else if blocker.InvocationID != task.Invocation.InvocationID || transition.InvocationID != task.Invocation.InvocationID {
		return invalidTransition("operator blocker invocation does not match task")
	}
	if task.Invocation != nil {
		if transition.ReturnStage != task.Invocation.ReturnStage || transition.BuilderAttempt != task.BuilderAttempt {
			return invalidTransition("operator blocker stage does not match task")
		}
	} else if !validReturnStage(transition.Role, transition.ReturnStage) || transition.BuilderAttempt != task.BuilderAttempt {
		return invalidTransition("operator blocker stage does not match task")
	}
	copy := *blocker
	snapshot.Control.Blocker = &copy
	task.Status = TaskNeedsOperator
	snapshot.TaskStates[task.TaskID] = *task
	reduce(snapshot)
	return nil
}

func matchInvocation(task *TaskExecutionState, transition TaskTransition, status TaskStatus) error {
	if task.Status != status {
		return invalidTransition("task status %q does not allow action", task.Status)
	}
	if task.Invocation == nil || transition.InvocationID == "" || task.Invocation.InvocationID != transition.InvocationID || task.Invocation.LogicalWorkID != transition.LogicalWorkID || task.Invocation.Role != transition.Role || task.BuilderAttempt != transition.BuilderAttempt {
		return invalidTransition("invocation identity does not match task")
	}
	if !validReturnStage(task.Invocation.Role, task.Invocation.ReturnStage) {
		return invalidTransition("invalid invocation return stage")
	}
	if transition.ReturnStage != task.Invocation.ReturnStage {
		return invalidTransition("invocation return stage does not match task")
	}
	return nil
}

func validReturnStage(role string, stage TaskStatus) bool {
	if role == roleReviewer {
		return stage == TaskGatePassed
	}
	return role == roleBuilder && (stage == TaskPending || stage == TaskGateFailed || stage == TaskReviewBlocked)
}

func hasProviderIdentity(invocation *InvocationState) bool {
	return invocation != nil && (invocation.ProviderIdentity != "" || invocation.ProviderSession != "" || invocation.ProviderPane != "" || invocation.ProviderProcess != "")
}

func cloneWorktree(worktree *WorktreeIdentity) *WorktreeIdentity {
	if worktree == nil {
		return nil
	}
	clone := *worktree
	return &clone
}
