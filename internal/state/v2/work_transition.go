package statev2

import (
	"strings"

	contractv2 "thread-dock/internal/contract/v2"
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
		if snapshot.Control.ApprovedContractHash == "" || snapshot.State == StateAwaitingApproval || snapshot.State == StatePaused || snapshot.State == StateNeedsOperator || snapshot.State == StateCompleted || snapshot.State == StateDraft {
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
		return applyWorkResolve(snapshot, transition.Resolve)
	default:
		return invalidTransition("unsupported work action %q", transition.Action)
	}
}

func applyWorkResolve(snapshot *WorkSnapshot, payload *ResolvePayload) error {
	if snapshot.State != StateNeedsOperator || payload == nil || payload.Kind != ResolveExtendBudget {
		return invalidTransition("unsupported work resolve transition")
	}
	if strings.TrimSpace(payload.OperatorRef) == "" || payload.TaskID == "" || !validBudgetKind(payload.Budget) {
		return invalidTransition("invalid budget resolution")
	}
	blocker := snapshot.Control.Blocker
	if blocker == nil || blocker.TaskID != payload.TaskID || blocker.OperatorRef != payload.OperatorRef {
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
		task.RepairLimit = payload.NewLimit
	case BudgetRecovery:
		if payload.NewLimit <= task.RecoveryCount || payload.NewLimit <= task.RecoveryLimit {
			return invalidTransition("new recovery budget must be higher than current limit and usage")
		}
		task.RecoveryLimit = payload.NewLimit
	}
	snapshot.TaskStates[contractv2.TaskID(payload.TaskID)] = task
	snapshot.Control.Blocker = nil
	reduce(snapshot)
	return nil
}

func validBudgetKind(kind BudgetKind) bool {
	return kind == BudgetRepair || kind == BudgetRecovery
}
