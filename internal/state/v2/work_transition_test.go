package statev2

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

func taskRequest(t *testing.T, snapshot WorkSnapshot, requestID contractv2.RequestID, transition TaskTransition) TransitionRequest {
	t.Helper()
	req := TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Task: &transition}
	hash, err := TransitionPayloadHash(req)
	if err != nil {
		t.Fatal(err)
	}
	req.PayloadHash = hash
	return req
}

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
	snapshot.Control.Blocker = &OperatorBlocker{Kind: BlockerKindRepairBudgetExhausted, OperatorRef: "operator-1", TaskID: "task-1", Diagnostic: "budget"}
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

func TestExtendBudgetRejectsUnrelatedBlockerWithoutWrite(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.State = StateNeedsOperator
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval", Blocker: &OperatorBlocker{
		Kind: BlockerKindRecoveryBudgetExhausted, OperatorRef: "operator-1", TaskID: "task-1", Diagnostic: "recovery budget exhausted",
	}}
	before := snapshot
	err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{
		Kind: ResolveExtendBudget, OperatorRef: "operator-1", TaskID: "task-1", Budget: BudgetKindRepair, NewLimit: 3,
	}})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, want invalid transition", err)
	}
	if !reflect.DeepEqual(snapshot, before) {
		t.Fatalf("unrelated blocker resolution wrote state: before=%#v after=%#v", before, snapshot)
	}
}

func TestExtendBudgetRequiresMatchingBudgetBlockerKind(t *testing.T) {
	for _, tc := range []struct {
		name        string
		blockerKind string
		budget      BudgetKind
	}{
		{name: "repair request against recovery blocker", blockerKind: BlockerKindRecoveryBudgetExhausted, budget: BudgetKindRepair},
		{name: "recovery request against repair blocker", blockerKind: BlockerKindRepairBudgetExhausted, budget: BudgetKindRecovery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validSnapshot()
			snapshot.State = StateNeedsOperator
			snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval", Blocker: &OperatorBlocker{
				Kind: tc.blockerKind, OperatorRef: "operator-1", TaskID: "task-1", Diagnostic: "budget exhausted",
			}}
			err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{
				Kind: ResolveExtendBudget, OperatorRef: "operator-1", TaskID: "task-1", Budget: tc.budget, NewLimit: 3,
			}})
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want invalid transition", err)
			}
			if snapshot.Control.Blocker == nil || snapshot.Control.Blocker.Kind != tc.blockerKind {
				t.Fatalf("blocker changed on mismatch: %#v", snapshot.Control.Blocker)
			}
		})
	}
}

func TestPauseNeedsOperatorPersistsPauseRequest(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.State = StateNeedsOperator
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval", Blocker: &OperatorBlocker{
		Kind: "runtime_unknown", OperatorRef: "operator-1", TaskID: "task-1", Diagnostic: "runtime requires inspection",
	}}
	if err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkPause}); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Control.PauseRequested || snapshot.State != StateNeedsOperator || snapshot.NextAction != "resolve" {
		t.Fatalf("paused needs-operator snapshot = %#v", snapshot)
	}
}

func cloneBlocker(blocker *OperatorBlocker) *OperatorBlocker {
	if blocker == nil {
		return nil
	}
	clone := *blocker
	return &clone
}

func TestRuntimeNotStartedResolutionRestoresTaskInOneApply(t *testing.T) {
	ctx := context.Background()
	s := NewStore(t.TempDir())
	if _, err := createPlan(s, ctx, validSnapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	approved := transitionRequest(t, snapshot, "approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash})
	if snapshot, err = s.Apply(ctx, approved); err != nil {
		t.Fatal(err)
	}
	reserve := builderReserveTransition(time.Date(2026, time.January, 2, 3, 4, 1, 0, time.UTC))
	if snapshot, err = s.Apply(ctx, taskRequest(t, snapshot, "reserve", reserve)); err != nil {
		t.Fatal(err)
	}
	block := TaskTransition{TaskID: "task-1", Action: TaskNeedsOperatorAction, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Blocker: &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "operator-1", TaskID: "task-1", InvocationID: "inv-1", Diagnostic: "owner state unknown"}}
	if snapshot, err = s.Apply(ctx, taskRequest(t, snapshot, "block", block)); err != nil {
		t.Fatal(err)
	}
	resolve := WorkTransition{Action: WorkResolve, At: invocationAt(3), Resolve: &ResolvePayload{Kind: ResolveRuntimeNotStarted, OperatorRef: "operator-1", TaskID: "task-1", InvocationID: "inv-1", Evidence: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true, Diagnostic: "confirmed owner exit before launch"}}}
	if snapshot, err = s.Apply(ctx, transitionRequest(t, snapshot, "resolve", resolve)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	state := snapshot.TaskStates["task-1"]
	if state.Status != TaskPending || state.Invocation != nil || snapshot.Control.Blocker != nil || len(state.PriorAttempts) != 1 {
		t.Fatalf("resolved snapshot = %#v", snapshot)
	}
}

func TestRuntimeTerminatedResolutionConfirmsOnlyMatchingRuntimeBlocker(t *testing.T) {
	ctx := context.Background()
	s := NewStore(t.TempDir())
	if _, err := createPlan(s, ctx, validSnapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Load(ctx, "work-1")
	if snapshot, err := s.Apply(ctx, transitionRequest(t, snapshot, "approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash})); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = s.Load(ctx, "work-1")
	reserve := builderReserveTransition(invocationAt(1))
	if snapshot, err := s.Apply(ctx, taskRequest(t, snapshot, "reserve", reserve)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = s.Load(ctx, "work-1")
	launch := TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
	if snapshot, err := s.Apply(ctx, taskRequest(t, snapshot, "launch", launch)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = s.Load(ctx, "work-1")
	running := TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Invocation: &InvocationState{ProviderProcess: "pid"}}
	if snapshot, err := s.Apply(ctx, taskRequest(t, snapshot, "running", running)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = s.Load(ctx, "work-1")
	block := TaskTransition{TaskID: "task-1", Action: TaskNeedsOperatorAction, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Blocker: &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "operator-1", TaskID: "task-1", InvocationID: "inv-1", Diagnostic: "owner state unknown"}}
	if snapshot, err := s.Apply(ctx, taskRequest(t, snapshot, "block", block)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = s.Load(ctx, "work-1")
	resolve := WorkTransition{Action: WorkResolve, At: invocationAt(5), Resolve: &ResolvePayload{Kind: ResolveRuntimeTerminated, OperatorRef: "operator-1", TaskID: "task-1", InvocationID: "inv-1", Evidence: &ResolutionEvidence{OwnerTerminated: true, Diagnostic: "owner exit confirmed"}}}
	resolved, err := s.Apply(ctx, transitionRequest(t, snapshot, "resolve", resolve))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	state := resolved.TaskStates["task-1"]
	if state.Status != TaskTerminated || state.Invocation == nil || !state.Invocation.TerminationConfirmed || resolved.Control.Blocker != nil {
		t.Fatalf("resolved snapshot = %#v", resolved)
	}
}
