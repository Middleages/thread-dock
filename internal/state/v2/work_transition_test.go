package statev2

import (
	"errors"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func TestWorkApproveRequiresApprovalInputsAndQueues(t *testing.T) {
	base := validSnapshot()
	for _, tc := range []struct {
		name      string
		state     WorkState
		approval  string
		contract  string
		wantError bool
	}{
		{name: "wrong source", state: StateQueued, approval: "ref", contract: base.ContractHash, wantError: true},
		{name: "missing ref", state: StateAwaitingApproval, contract: base.ContractHash, wantError: true},
		{name: "wrong contract", state: StateAwaitingApproval, approval: "ref", contract: "other", wantError: true},
		{name: "valid", state: StateAwaitingApproval, approval: "ref", contract: base.ContractHash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := base
			snapshot.State = tc.state
			next := snapshot
			err := applyWorkTransition(&next, WorkTransition{Action: WorkApprove, ApprovalRef: tc.approval, ContractHash: tc.contract})
			if tc.wantError {
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("error = %v, want invalid transition", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if next.Control.ApprovalRef != "ref" || next.Control.ApprovedContractHash != base.ContractHash || next.State != StateQueued || next.NextAction != "run" {
				t.Fatalf("approved snapshot = %#v", next)
			}
		})
	}
}

func TestWorkPauseResumeAndPreApprovalGuards(t *testing.T) {
	snapshot := validSnapshot()
	if err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkPause}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pre-approval pause error = %v", err)
	}
	if err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkApprove, ApprovalRef: "ref", ContractHash: snapshot.ContractHash}); err != nil {
		t.Fatal(err)
	}
	if err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkPause}); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Control.PauseRequested {
		t.Fatal("pause did not set PauseRequested")
	}
	if snapshot.State != StatePaused || snapshot.NextAction != "resume" {
		t.Fatalf("paused snapshot = %#v", snapshot)
	}
	if err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkResume}); err != nil {
		t.Fatal(err)
	}
	if snapshot.Control.PauseRequested || snapshot.State != StateQueued || snapshot.NextAction != "run" {
		t.Fatalf("resumed snapshot = %#v", snapshot)
	}
}

func TestWorkResumeAndBudgetExtensionGuards(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "ref", PauseRequested: true}
	snapshot.State = StatePaused
	snapshot.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []AttemptSummary{}}
	if err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkResume}); err != nil {
		t.Fatal(err)
	}
	snapshot.State = StateNeedsOperator
	snapshot.Control.Blocker = &OperatorBlocker{Kind: "budget", OperatorRef: "operator-1", TaskID: "task-1", Diagnostic: "budget"}
	for _, tc := range []struct {
		name      string
		payload   *ResolvePayload
		wantError bool
	}{
		{name: "missing operator", payload: &ResolvePayload{Kind: ResolveExtendBudget, TaskID: "task-1", Budget: BudgetKindRepair, NewLimit: 3}, wantError: true},
		{name: "wrong task", payload: &ResolvePayload{Kind: ResolveExtendBudget, OperatorRef: "operator-1", TaskID: "task-2", Budget: BudgetKindRepair, NewLimit: 3}, wantError: true},
		{name: "invalid budget", payload: &ResolvePayload{Kind: ResolveExtendBudget, OperatorRef: "operator-1", TaskID: "task-1", Budget: BudgetKind("other"), NewLimit: 3}, wantError: true},
		{name: "not higher", payload: &ResolvePayload{Kind: ResolveExtendBudget, OperatorRef: "operator-1", TaskID: "task-1", Budget: BudgetKindRepair, NewLimit: 2}, wantError: true},
		{name: "valid repair", payload: &ResolvePayload{Kind: ResolveExtendBudget, OperatorRef: "operator-1", TaskID: "task-1", Budget: BudgetKindRepair, NewLimit: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := snapshot
			next.Control.Blocker = cloneBlocker(snapshot.Control.Blocker)
			err := applyWorkTransition(&next, WorkTransition{Action: WorkResolve, Resolve: tc.payload})
			if tc.wantError {
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if next.TaskStates[contractv2.TaskID("task-1")].RepairLimit != 3 || next.TaskStates["task-1"].RepairCount != 0 || next.Control.Blocker != nil || next.State != StateQueued {
				t.Fatalf("extended snapshot = %#v", next)
			}
		})
	}
}

func cloneBlocker(blocker *OperatorBlocker) *OperatorBlocker {
	if blocker == nil {
		return nil
	}
	clone := *blocker
	return &clone
}
