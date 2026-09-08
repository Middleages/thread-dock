package statev2

import (
	"context"
	"errors"
	"testing"
)

func TestRepairReservationDebitsAndInvalidatesEvidenceAtomically(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "candidate", candidateTransition())); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	gate := TaskTransition{TaskID: "task-1", Action: TaskRecordGate, Role: roleBuilder, BuilderAttempt: 1, At: invocationAt(6), Gate: &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"fail"}, Passed: false, ObservedAt: invocationAt(6)}}
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "gate", gate)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	reserve := builderReserveTransition(invocationAt(7))
	reserve.InvocationID = "repair-inv"
	reserve.LogicalWorkID = "repair-work"
	reserve.BuilderAttempt = 2
	reserve.ReturnStage = TaskGateFailed
	snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "repair", reserve))
	if err != nil {
		t.Fatal(err)
	}
	state := snapshot.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.RepairCount != 1 || state.BuilderAttempt != 2 || state.Candidate != nil || state.Gate != nil || state.Invocation == nil || state.Invocation.InvocationID != "repair-inv" || len(state.PriorAttempts) != 1 {
		t.Fatalf("repair state = %#v", state)
	}
	if state.PriorAttempts[0].CandidateSHA != candidateSHA || state.PriorAttempts[0].Outcome != "gate_failed" {
		t.Fatalf("repair summary = %#v", state.PriorAttempts[0])
	}
}

func TestRepairBudgetExhaustionNeedsOperatorWithoutReservation(t *testing.T) {
	_, snapshot := approvedStore(t)
	state := snapshot.TaskStates["task-1"]
	state.Status, state.BuilderAttempt, state.RepairCount = TaskGateFailed, 2, 2
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical-1", Role: roleBuilder, BuilderAttempt: 2}
	state.Candidate = &CandidateEvidence{BuilderAttempt: 2, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"x"}}
	state.Gate = &GateEvidence{BuilderAttempt: 2, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"fail"}, ObservedAt: invocationAt(1)}
	snapshot.TaskStates["task-1"] = state
	if err := validateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	reserve := builderReserveTransition(invocationAt(2))
	reserve.InvocationID = "repair-inv"
	reserve.LogicalWorkID = "repair-work"
	reserve.BuilderAttempt = 3
	reserve.ReturnStage = TaskGateFailed
	if err := applyTaskTransition(&snapshot, reserve, "repair-exhausted"); err != nil {
		t.Fatal(err)
	}
	got := snapshot
	state = got.TaskStates["task-1"]
	if state.Status != TaskNeedsOperator || state.RepairCount != 2 || state.Invocation != nil || got.Control.Blocker == nil || got.Control.Blocker.Kind != BlockerKindRepairBudgetExhausted {
		t.Fatalf("exhausted state = %#v blocker=%#v", state, got.Control.Blocker)
	}
}

func TestRecoveryReservationUsesTransientTerminatedInvocationAndBudget(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, true)
	tr := builderReserveTransition(invocationAt(5))
	tr.InvocationID = "recovery-inv"
	tr.Transient = true
	tr.Invocation.TransientFailure = true
	got, err := store.Apply(ctx, taskRequest(t, snapshot, "recovery", tr))
	if err != nil {
		t.Fatal(err)
	}
	state := got.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.RecoveryCount != 1 || state.BuilderAttempt != 1 || state.Invocation.InvocationID != "recovery-inv" {
		t.Fatalf("recovery state = %#v", state)
	}
	if state.Candidate != nil || state.Gate != nil {
		t.Fatalf("recovery changed evidence: %#v %#v", state.Candidate, state.Gate)
	}
}

func TestRetryVerifiedStageDerivesTaskStatusFromEvidence(t *testing.T) {
	_, snapshot := approvedStore(t)
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskNeedsOperator
	state.BuilderAttempt = 1
	state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"x"}}
	state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)}
	snapshot.TaskStates["task-1"] = state
	snapshot.State = StateNeedsOperator
	snapshot.Control.Blocker = &OperatorBlocker{Kind: "evidence_mismatch", OperatorRef: "operator", TaskID: "task-1", Diagnostic: "repair"}
	if err := validateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkResolve, At: invocationAt(2), Resolve: &ResolvePayload{Kind: ResolveRetryVerifiedStage, OperatorRef: "operator", TaskID: "task-1", Evidence: &ResolutionEvidence{Diagnostic: "verified"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot
	if got.TaskStates["task-1"].Status != TaskGatePassed || got.Control.Blocker != nil {
		t.Fatalf("resolved state = %#v blocker=%#v", got.TaskStates["task-1"], got.Control.Blocker)
	}
}

func TestUnsupportedEvidenceActionIsRejected(t *testing.T) {
	tr := TaskTransition{TaskID: "task-1", Action: TaskRecordCandidate}
	if _, err := TransitionPayloadHash(TransitionRequest{Task: &tr}); err != nil {
		t.Fatal(err)
	}
	if err := applyTransition(&WorkSnapshot{}, TransitionRequest{Task: &tr}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v", err)
	}
}
