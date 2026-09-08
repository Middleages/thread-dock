package statev2

import (
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

const (
	BlockerKindRepairBudgetExhausted   = "repair_budget_exhausted"
	BlockerKindRecoveryBudgetExhausted = "recovery_budget_exhausted"
	BlockerKindRuntimeUnknown          = "runtime_unknown"
	BlockerKindEvidenceMismatch        = "evidence_mismatch"
	BlockerKindRetryVerifiedStage      = "retry_verified_stage"
	BlockerKindPublicationConflict     = "publication_conflict"
)

func applyWorkTransition(snapshot *WorkSnapshot, transition WorkTransition) error {
	switch transition.Action {
	case WorkApprove:
		if snapshot.State != StateAwaitingApproval || snapshot.Control.ApprovedContractHash != "" || snapshot.Control.ApprovalRef != "" {
			return invalidTransition("work is not awaiting approval")
		}
		if strings.TrimSpace(transition.ApprovalRef) == "" {
			return invalidTransition("approval reference is required")
		}
		if transition.ContractHash == "" || transition.ContractHash != snapshot.ContractHash {
			return invalidTransition("approval contract hash does not match current contract")
		}
		snapshot.Control.ApprovedContractHash = transition.ContractHash
		snapshot.Control.ApprovalRef = transition.ApprovalRef
		reduce(snapshot)
		return nil
	case WorkPause:
		if snapshot.Control.ApprovedContractHash == "" || snapshot.State == StateAwaitingApproval || snapshot.State == StatePaused || snapshot.State == StateCompleted || snapshot.State == StateDraft {
			return invalidTransition("work cannot be paused in state %q", snapshot.State)
		}
		if snapshot.Control.PauseRequested {
			return invalidTransition("work is already paused")
		}
		snapshot.Control.PauseRequested = true
		reduce(snapshot)
		return nil
	case WorkResume:
		if snapshot.State != StatePaused || !snapshot.Control.PauseRequested {
			return invalidTransition("work is not paused")
		}
		if snapshot.Control.Blocker != nil {
			return invalidTransition("work has an operator blocker")
		}
		if hasActiveOrUnknownInvocation(snapshot) {
			return invalidTransition("work has an active or unknown invocation")
		}
		snapshot.Control.PauseRequested = false
		reduce(snapshot)
		return nil
	case WorkResolve:
		return applyWorkResolve(snapshot, transition)
	default:
		return invalidTransition("unsupported work action %q", transition.Action)
	}
}

func applyWorkResolve(snapshot *WorkSnapshot, transition WorkTransition) error {
	payload := transition.Resolve
	if snapshot.State != StateNeedsOperator || payload == nil {
		return invalidTransition("unsupported work resolve transition")
	}
	if payload.Kind == ResolvePublicationReconciled {
		return applyPublicationReconcile(snapshot, transition)
	}
	if payload.Kind == ResolveRuntimeNotStarted || payload.Kind == ResolveRuntimeTerminated {
		return applyRuntimeResolve(snapshot, transition)
	}
	if payload.Kind == ResolveRetryVerifiedStage {
		return applyRetryVerifiedStage(snapshot, transition)
	}
	if payload.Kind != ResolveExtendBudget {
		return invalidTransition("unsupported work resolve transition")
	}
	if strings.TrimSpace(payload.OperatorRef) == "" || payload.TaskID == "" || !validBudgetKind(payload.Budget) {
		return invalidTransition("invalid budget resolution")
	}
	blocker := snapshot.Control.Blocker
	if blocker == nil || blocker.TaskID != payload.TaskID || blocker.OperatorRef != payload.OperatorRef || blocker.Kind != budgetBlockerKind(payload.Budget) {
		return invalidTransition("budget resolution does not match blocker")
	}
	task, ok := snapshot.TaskStates[payload.TaskID]
	if !ok {
		return invalidTransition("budget resolution task is not in contract")
	}
	switch payload.Budget {
	case BudgetRepair:
		if payload.NewLimit <= task.RepairCount || payload.NewLimit <= task.RepairLimit {
			return invalidTransition("new repair budget must be higher than current limit and usage")
		}
		if task.Review != nil && !task.Review.Accepted && task.Gate != nil && task.Gate.Passed {
			task.Status = TaskReviewBlocked
		} else if task.Gate != nil && !task.Gate.Passed {
			task.Status = TaskGateFailed
		} else {
			return invalidTransition("repair blocker has no coherent evidence source")
		}
		task.RepairLimit = payload.NewLimit
	case BudgetRecovery:
		if payload.NewLimit <= task.RecoveryCount || payload.NewLimit <= task.RecoveryLimit {
			return invalidTransition("new recovery budget must be higher than current limit and usage")
		}
		if task.Invocation == nil || task.Status != TaskNeedsOperator || !task.Invocation.TerminationConfirmed || task.Invocation.EndedAt == nil || !task.Invocation.TransientFailure {
			return invalidTransition("recovery blocker has no coherent terminated invocation")
		}
		task.Status = TaskTerminated
		task.RecoveryLimit = payload.NewLimit
	}
	snapshot.TaskStates[contractv2.TaskID(payload.TaskID)] = task
	snapshot.Control.Blocker = nil
	reduce(snapshot)
	return nil
}

func applyRetryVerifiedStage(snapshot *WorkSnapshot, transition WorkTransition) error {
	payload := transition.Resolve
	if payload == nil || strings.TrimSpace(payload.OperatorRef) == "" || payload.TaskID == "" || payload.Evidence == nil {
		return invalidTransition("invalid evidence resolution")
	}
	blocker := snapshot.Control.Blocker
	if blocker == nil || blocker.Kind != BlockerKindRetryVerifiedStage || blocker.OperatorRef != payload.OperatorRef || blocker.TaskID != payload.TaskID {
		return invalidTransition("evidence resolution does not match blocker")
	}
	task, ok := snapshot.TaskStates[payload.TaskID]
	if !ok {
		return invalidTransition("evidence resolution task is not in contract")
	}
	if strings.TrimSpace(payload.Evidence.Diagnostic) == "" {
		return invalidTransition("evidence resolution diagnostic is required")
	}
	if err := validateDiagnostic(payload.Evidence.Diagnostic); err != nil {
		return invalidTransition("resolution diagnostic: %v", err)
	}
	if payload.Evidence.BuilderAttempt != task.BuilderAttempt {
		return invalidTransition("evidence resolution attempt does not match task")
	}
	if task.Candidate != nil && payload.Evidence.CandidateSHA != task.Candidate.CandidateSHA {
		return invalidTransition("evidence resolution candidate SHA does not match task")
	}
	if task.Candidate == nil && payload.Evidence.CandidateSHA != "" {
		return invalidTransition("evidence resolution candidate SHA is unexpected")
	}
	if task.Review != nil && payload.Evidence.ReviewSHA != task.Review.ReviewSHA {
		return invalidTransition("evidence resolution review SHA does not match task")
	}
	if task.Review == nil && payload.Evidence.ReviewSHA != "" {
		return invalidTransition("evidence resolution review SHA is unexpected")
	}
	switch {
	case task.Integration != nil && task.Integration.BuilderAttempt == task.BuilderAttempt && task.Integration.CandidateSHA == candidateSHAOf(task):
		task.Status = TaskIntegrated
	case task.Review != nil && task.Review.BuilderAttempt == task.BuilderAttempt && task.Review.CandidateSHA == candidateSHAOf(task) && task.Review.Accepted:
		task.Status = TaskAccepted
	case task.Review != nil && task.Review.BuilderAttempt == task.BuilderAttempt && task.Review.CandidateSHA == candidateSHAOf(task) && !task.Review.Accepted:
		task.Status = TaskReviewBlocked
	case task.Gate != nil && task.Gate.BuilderAttempt == task.BuilderAttempt && task.Gate.CandidateSHA == candidateSHAOf(task) && task.Gate.Passed:
		task.Status = TaskGatePassed
	case task.Gate != nil && task.Gate.BuilderAttempt == task.BuilderAttempt && task.Gate.CandidateSHA == candidateSHAOf(task) && !task.Gate.Passed:
		task.Status = TaskGateFailed
	case task.Candidate != nil && task.Candidate.BuilderAttempt == task.BuilderAttempt:
		task.Status = TaskCandidateReady
	case task.Invocation != nil && task.Invocation.TerminationConfirmed && task.Invocation.EndedAt != nil:
		task.Status = TaskTerminated
	default:
		task.Status = TaskPending
	}
	snapshot.TaskStates[payload.TaskID] = task
	snapshot.Control.Blocker = nil
	reduce(snapshot)
	return nil
}

func candidateSHAOf(task TaskExecutionState) string {
	if task.Candidate == nil {
		return ""
	}
	return task.Candidate.CandidateSHA
}

func applyRuntimeResolve(snapshot *WorkSnapshot, transition WorkTransition) error {
	payload := transition.Resolve
	if payload == nil || strings.TrimSpace(payload.OperatorRef) == "" || payload.TaskID == "" || payload.InvocationID == "" || payload.Evidence == nil {
		return invalidTransition("invalid runtime resolution")
	}
	blocker := snapshot.Control.Blocker
	if blocker == nil || blocker.Kind != BlockerKindRuntimeUnknown || blocker.OperatorRef != payload.OperatorRef || blocker.TaskID != payload.TaskID || blocker.InvocationID != payload.InvocationID {
		return invalidTransition("runtime resolution does not match blocker")
	}
	task, ok := snapshot.TaskStates[payload.TaskID]
	if !ok || task.Invocation == nil || task.Invocation.InvocationID != payload.InvocationID {
		return invalidTransition("runtime resolution task does not match invocation")
	}
	at := transition.At
	if at.IsZero() || at.Location() != time.UTC {
		return invalidTransition("runtime resolution time must be UTC")
	}
	switch payload.Kind {
	case ResolveRuntimeNotStarted:
		if task.Status != TaskInvocationReserved && task.Status != TaskNeedsOperator || task.Invocation.LaunchRequested || hasProviderIdentity(task.Invocation) || !payload.Evidence.OwnerTerminated || payload.Evidence.LaunchRequested || !payload.Evidence.ProviderAbsent {
			return invalidTransition("positive no-launch proof is required")
		}
		if err := validateDiagnostic(payload.Evidence.Diagnostic); err != nil {
			return invalidTransition("resolution diagnostic: %v", err)
		}
		if len(task.PriorAttempts) >= MaxPriorAttempts {
			return invalidTransition("prior attempt summary limit reached")
		}
		task.PriorAttempts = append(task.PriorAttempts, AttemptSummary{BuilderAttempt: task.BuilderAttempt, Outcome: "abandoned_not_started", Diagnostic: payload.Evidence.Diagnostic})
		if err := restoreAfterReconcile(&task); err != nil {
			return err
		}
	case ResolveRuntimeTerminated:
		if task.Status != TaskRunning && task.Status != TaskTerminationPending && task.Status != TaskNeedsOperator || !payload.Evidence.OwnerTerminated {
			return invalidTransition("positive termination proof is required")
		}
		if payload.Evidence.Diagnostic != "" && len([]byte(payload.Evidence.Diagnostic)) > MaxDiagnosticBytes {
			return invalidTransition("termination diagnostic exceeds limit")
		}
		if task.Status == TaskRunning || task.Status == TaskTerminationPending {
			tr := TaskTransition{TaskID: payload.TaskID, Action: TaskConfirmTermination, InvocationID: payload.InvocationID, LogicalWorkID: task.Invocation.LogicalWorkID, Role: task.Invocation.Role, BuilderAttempt: task.BuilderAttempt, At: at, Reason: payload.Evidence.Diagnostic}
			if err := confirmTermination(&task, tr); err != nil {
				return err
			}
		} else {
			task.Invocation.EndedAt = &at
			task.Invocation.TerminationConfirmed = true
			if payload.Evidence.Diagnostic != "" {
				task.Invocation.TerminationReason = payload.Evidence.Diagnostic
			}
			task.Status = TaskTerminated
		}
	default:
		return invalidTransition("unsupported runtime resolution")
	}
	snapshot.TaskStates[payload.TaskID] = task
	snapshot.Control.Blocker = nil
	reduce(snapshot)
	return nil
}

func validBudgetKind(kind BudgetKind) bool {
	return kind == BudgetRepair || kind == BudgetRecovery
}

func budgetBlockerKind(kind BudgetKind) string {
	if kind == BudgetRecovery {
		return BlockerKindRecoveryBudgetExhausted
	}
	return BlockerKindRepairBudgetExhausted
}
