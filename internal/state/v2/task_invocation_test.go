package statev2

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func invocationSnapshot() WorkSnapshot {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval"}
	s.State = StateQueued
	return s
}

func invocationAt(n int) time.Time {
	return time.Date(2026, time.January, 2, 3, 4, n, 0, time.UTC)
}

func builderReserveTransition(at time.Time) TaskTransition {
	return TaskTransition{
		TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "inv-1",
		LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending,
		BuilderAttempt: 1, At: at,
		Worktree:   &WorktreeIdentity{CanonicalPath: "/worktree", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: "base"},
		Invocation: &InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"},
	}
}

func TestBuilderInvocationLifecycleCanReserveLaunchRunRequestAndConfirmTermination(t *testing.T) {
	s := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := s.TaskStates["task-1"].Status; got != TaskInvocationReserved {
		t.Fatalf("reserved status = %q", got)
	}

	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}}); err != nil {
		t.Fatalf("begin launch: %v", err)
	}
	if !s.TaskStates["task-1"].Invocation.LaunchRequested {
		t.Fatal("launch was not requested")
	}

	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Invocation: &InvocationState{ProviderIdentity: "provider", ProviderSession: "session"}}}); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if got := s.TaskStates["task-1"].Status; got != TaskRunning {
		t.Fatalf("running status = %q", got)
	}

	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskRequestTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Reason: "completed"}}); err != nil {
		t.Fatalf("request termination: %v", err)
	}
	if got := s.TaskStates["task-1"].Status; got != TaskTerminationPending {
		t.Fatalf("termination-pending status = %q", got)
	}

	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(5)}}); err != nil {
		t.Fatalf("confirm termination: %v", err)
	}
	state := s.TaskStates["task-1"]
	if state.Status != TaskTerminated || state.Invocation == nil || !state.Invocation.TerminationConfirmed || state.Invocation.EndedAt == nil {
		t.Fatalf("terminated state = %#v", state)
	}
}

func TestInvocationLifecycleRejectsMarkRunningBeforeLaunch(t *testing.T) {
	s := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Invocation: &InvocationState{ProviderIdentity: "provider"}}})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("mark running before launch error = %v", err)
	}
}

func TestReviewerUsesSharedLifecycleAndPreservesCandidateAndGate(t *testing.T) {
	s := invocationSnapshot()
	state := s.TaskStates["task-1"]
	state.Status = TaskGatePassed
	state.BuilderAttempt = 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "builder-work", Role: roleBuilder, BuilderAttempt: 1}
	state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: "candidate"}
	state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: "candidate", Passed: true, ObservedAt: invocationAt(1)}
	s.TaskStates["task-1"] = state
	reserve := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(2), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "main", BaseSHA: "base"}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatalf("reviewer reserve: %v", err)
	}
	state = s.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.LogicalWork == nil || state.LogicalWork.Role != roleReviewer || state.Candidate == nil || state.Gate == nil {
		t.Fatalf("reviewer reservation did not preserve lifecycle state: %#v", state)
	}
}

func TestReconcileNotStartedRestoresStageAndPreservesLogicalWork(t *testing.T) {
	s := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	before := s.TaskStates["task-1"]
	tr := TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Resolution: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true, Diagnostic: "owner exited before launch"}}
	if err := applyTransition(&s, TransitionRequest{Task: &tr}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	state := s.TaskStates["task-1"]
	if state.Status != TaskPending || state.Invocation != nil || state.LogicalWork == nil || state.LogicalWork.LogicalWorkID != before.LogicalWork.LogicalWorkID || state.BuilderAttempt != before.BuilderAttempt || len(state.PriorAttempts) != 1 || state.PriorAttempts[0].Outcome != "abandoned_not_started" {
		t.Fatalf("reconciled state = %#v", state)
	}

	reserve.InvocationID = "inv-2"
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	state = s.TaskStates["task-1"]
	if state.LogicalWork.LogicalWorkID != "logical-1" || state.BuilderAttempt != 1 || len(state.PriorAttempts) != 1 {
		t.Fatalf("re-reserved state changed logical work/counts: %#v", state)
	}
}

func TestReviewerRereservationPreservesExistingLogicalWork(t *testing.T) {
	s := invocationSnapshot()
	state := s.TaskStates["task-1"]
	state.Status = TaskGatePassed
	state.BuilderAttempt = 3
	state.Candidate = &CandidateEvidence{BuilderAttempt: 3, CandidateSHA: "candidate"}
	state.Gate = &GateEvidence{BuilderAttempt: 3, CandidateSHA: "candidate", Passed: true, ObservedAt: invocationAt(1)}
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "review-work", Role: roleReviewer, BuilderAttempt: 3, RepairCount: 2, RecoveryCount: 1, RepairBudgetDebited: true, RecoveryBudgetDebited: true}
	s.TaskStates["task-1"] = state
	reserve := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv-2", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 3, At: invocationAt(2), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "main", BaseSHA: "base"}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatalf("reviewer re-reserve: %v", err)
	}
	got := s.TaskStates["task-1"].LogicalWork
	if *got != *state.LogicalWork {
		t.Fatalf("reviewer re-reservation replaced logical work: got=%#v want=%#v", got, state.LogicalWork)
	}
}

func TestReconcileRejectsInvocationIDReuse(t *testing.T) {
	s := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	reconcile := TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Resolution: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true}}
	if err := applyTransition(&s, TransitionRequest{Task: &reconcile}); err != nil {
		t.Fatal(err)
	}
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reused invocation error = %v, want invalid transition", err)
	}
}

func TestSameRequestApplyReplayDoesNotDoubleReserveInvocation(t *testing.T) {
	ctx := context.Background()
	s := NewStore(t.TempDir())
	if _, err := createPlan(s, ctx, validSnapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot, err = s.Apply(ctx, transitionRequest(t, snapshot, "approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash})); err != nil {
		t.Fatal(err)
	}
	reserve := builderReserveTransition(invocationAt(1))
	snapshot, err = s.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	request := taskRequest(t, snapshot, "reserve", reserve)
	first, err := s.Apply(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Apply(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replay) || len(replay.TaskStates["task-1"].InvocationHistory) != 1 || replay.Revision != first.Revision {
		t.Fatalf("replay mutated lifecycle: first=%#v replay=%#v", first, replay)
	}
}

func TestReconcileNotStartedRejectsLaunchRequestedOrInsufficientProof(t *testing.T) {
	for _, tc := range []struct {
		name   string
		launch bool
		proof  ResolutionEvidence
	}{
		{name: "launch requested", launch: true, proof: ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true}},
		{name: "ttl only", proof: ResolutionEvidence{ProviderAbsent: true}},
		{name: "missing provider proof", proof: ResolutionEvidence{OwnerTerminated: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := invocationSnapshot()
			reserve := builderReserveTransition(invocationAt(1))
			if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
				t.Fatal(err)
			}
			if tc.launch {
				launch := TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
				if err := applyTransition(&s, TransitionRequest{Task: &launch}); err != nil {
					t.Fatal(err)
				}
			}
			tr := TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Resolution: &tc.proof}
			if err := applyTransition(&s, TransitionRequest{Task: &tr}); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
