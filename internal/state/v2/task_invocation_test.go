package statev2

import (
	"context"
	"encoding/json"
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
		Worktree:   &WorktreeIdentity{CanonicalPath: "/worktree", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: "0123456789012345678901234567890123456789"},
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
	before := cloneSnapshot(t, s)
	err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Invocation: &InvocationState{ProviderIdentity: "provider"}}})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("mark running before launch error = %v", err)
	}
	if got, want := marshalSnapshot(t, s), marshalSnapshot(t, before); string(got) != string(want) {
		t.Fatalf("invalid mark-running wrote state: before=%s after=%s", want, got)
	}
}

func TestReviewerUsesSharedLifecycleAndPreservesCandidateAndGate(t *testing.T) {
	s := invocationSnapshot()
	state := s.TaskStates["task-1"]
	state.Status = TaskGatePassed
	state.BuilderAttempt = 1
	state.RepairCount = 1
	state.RecoveryCount = 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "builder-work", Role: roleBuilder, BuilderAttempt: 1}
	state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: "candidate"}
	state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: "candidate", Passed: true, ObservedAt: invocationAt(1)}
	s.TaskStates["task-1"] = state
	reserve := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(2), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "main", BaseSHA: "0123456789012345678901234567890123456789"}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatalf("reviewer reserve: %v", err)
	}
	state = s.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.LogicalWork == nil || state.LogicalWork.Role != roleReviewer || state.Candidate == nil || state.Gate == nil {
		t.Fatalf("reviewer reservation did not preserve lifecycle state: %#v", state)
	}
	if state.BuilderAttempt != 1 || state.RepairCount != 1 || state.RecoveryCount != 1 {
		t.Fatalf("reviewer reservation changed builder budgets: attempt=%d repair=%d recovery=%d", state.BuilderAttempt, state.RepairCount, state.RecoveryCount)
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
	reserve := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv-2", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 3, At: invocationAt(2), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "main", BaseSHA: "0123456789012345678901234567890123456789"}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
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

func TestStoreApplyRejectsFreshInvocationWhileInvocationIsActiveWithoutWrite(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	initial := validSnapshot()
	task := initial.TaskStates["task-1"]
	task.Status = TaskInvocationReserved
	task.BuilderAttempt = 1
	task.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical-1", Role: roleBuilder, BuilderAttempt: 1, Purpose: "task invocation"}
	task.Invocation = &InvocationState{
		InvocationID:       "inv-1",
		LogicalWorkID:      "logical-1",
		Role:               roleBuilder,
		ReturnStage:        TaskPending,
		LogicalProfile:     "builder",
		RuntimeFingerprint: "runtime-v1",
	}
	task.InvocationHistory = []InvocationID{"inv-1"}
	initial.TaskStates["task-1"] = task
	if _, err := createPlan(store, ctx, initial); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, initial.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := store.Apply(ctx, transitionRequest(t, snapshot, "approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Load(ctx, approved.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneSnapshot(t, snapshot)
	fresh := builderReserveTransition(invocationAt(2))
	fresh.InvocationID = "inv-2"
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "reserve-inv-2", fresh)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("fresh invocation while active error = %v, want invalid transition", err)
	}
	after, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid fresh reservation changed persisted snapshot: before=%#v after=%#v", before, after)
	}
	if got, want := marshalSnapshot(t, after), marshalSnapshot(t, before); string(got) != string(want) {
		t.Fatalf("invalid fresh reservation changed persisted bytes: before=%s after=%s", want, got)
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
			before := cloneSnapshot(t, s)
			tr := TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Resolution: &tc.proof}
			if err := applyTransition(&s, TransitionRequest{Task: &tr}); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v", err)
			}
			if got, want := marshalSnapshot(t, s), marshalSnapshot(t, before); string(got) != string(want) {
				t.Fatalf("invalid reconcile wrote state: before=%s after=%s", want, got)
			}
		})
	}
}

func cloneSnapshot(t *testing.T, snapshot WorkSnapshot) WorkSnapshot {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var clone WorkSnapshot
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestReserveRejectsLifecycleMatrixWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*WorkSnapshot, *TaskTransition)
	}{
		{name: "role mismatch", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Role = "scout" }},
		{name: "return stage mismatch", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.ReturnStage = TaskGateFailed }},
		{name: "foreign task", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.TaskID = "foreign" }},
		{name: "pre approval", mutate: func(s *WorkSnapshot, _ *TaskTransition) { s.Control = WorkControl{}; s.State = StateAwaitingApproval }},
		{name: "paused", mutate: func(s *WorkSnapshot, _ *TaskTransition) { s.Control.PauseRequested = true; s.State = StatePaused }},
		{name: "blocked", mutate: func(s *WorkSnapshot, _ *TaskTransition) {
			s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "blocked"}
			s.State = StateNeedsOperator
		}},
		{name: "completed", mutate: func(s *WorkSnapshot, _ *TaskTransition) { s.State = StateCompleted }},
		{name: "whitespace canonical path", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Worktree.CanonicalPath = " \t" }},
		{name: "whitespace git common dir", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Worktree.GitCommonDir = " " }},
		{name: "whitespace branch", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Worktree.Branch = "\t" }},
		{name: "whitespace base SHA", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Worktree.BaseSHA = " \n" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := cloneSnapshot(t, invocationSnapshot())
			reserve := builderReserveTransition(invocationAt(1))
			tc.mutate(&snapshot, &reserve)
			before := cloneSnapshot(t, snapshot)
			if err := applyTransition(&snapshot, TransitionRequest{Task: &reserve}); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want invalid transition", err)
			}
			if got, want := marshalSnapshot(t, snapshot), marshalSnapshot(t, before); string(got) != string(want) {
				t.Fatalf("invalid reservation wrote state: before=%s after=%s", want, got)
			}
		})
	}
}

func TestReserveRejectsDuplicateAndCandidateBeforeTerminationWithoutWrite(t *testing.T) {
	snapshot := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&snapshot, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	before := cloneSnapshot(t, snapshot)
	if err := applyTransition(&snapshot, TransitionRequest{Task: &reserve}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("duplicate reserve error = %v", err)
	}
	candidate := TaskTransition{TaskID: "task-1", Action: TaskRecordCandidate, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
	if err := applyTransition(&snapshot, TransitionRequest{Task: &candidate}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("candidate before termination error = %v", err)
	}
	if got, want := marshalSnapshot(t, snapshot), marshalSnapshot(t, before); string(got) != string(want) {
		t.Fatalf("invalid duplicate/candidate transition wrote state: before=%s after=%s", want, got)
	}
}

func marshalSnapshot(t *testing.T, snapshot WorkSnapshot) []byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLifecycleIdentityGuardsReachEachActionSourceStage(t *testing.T) {
	for _, action := range []TaskAction{TaskBeginLaunch, TaskMarkRunning, TaskRequestTermination, TaskConfirmTermination, TaskReconcileNotStarted} {
		for _, identity := range []struct {
			name   string
			mutate func(*TaskTransition)
		}{
			{name: "invocation", mutate: func(tr *TaskTransition) { tr.InvocationID = "other" }},
			{name: "logical work", mutate: func(tr *TaskTransition) { tr.LogicalWorkID = "other" }},
			{name: "attempt", mutate: func(tr *TaskTransition) { tr.BuilderAttempt = 2 }},
		} {
			t.Run(string(action)+"/"+identity.name, func(t *testing.T) {
				store, source, transition := setupLifecycleStage(t, action)
				identity.mutate(&transition)
				before := marshalSnapshot(t, source)
				request := taskRequest(t, source, "invalid", transition)
				if _, err := store.Apply(context.Background(), request); !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("error = %v, want invalid transition", err)
				}
				after, err := store.Load(context.Background(), source.WorkID)
				if err != nil {
					t.Fatal(err)
				}
				if got := marshalSnapshot(t, after); string(got) != string(before) {
					t.Fatalf("invalid lifecycle wrote state: before=%s after=%s", before, got)
				}
			})
		}
	}
}

func setupLifecycleStage(t *testing.T, action TaskAction) (Store, WorkSnapshot, TaskTransition) {
	t.Helper()
	ctx := context.Background()
	store := NewStore(t.TempDir())
	if _, err := createPlan(store, ctx, validSnapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, transitionRequest(t, snapshot, "approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash})); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Load(ctx, "work-1")
	reserve := builderReserveTransition(invocationAt(1))
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "reserve", reserve)); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Load(ctx, "work-1")
	if action == TaskReconcileNotStarted {
		return store, snapshot, TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(6), Resolution: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true}}
	}
	begin := TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
	if action == TaskBeginLaunch {
		return store, snapshot, begin
	}
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "begin", begin)); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Load(ctx, "work-1")
	running := TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Invocation: &InvocationState{ProviderProcess: "pid"}}
	if action == TaskMarkRunning {
		return store, snapshot, running
	}
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "running", running)); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Load(ctx, "work-1")
	requestTermination := TaskTransition{TaskID: "task-1", Action: TaskRequestTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Reason: "done"}
	if action == TaskRequestTermination {
		return store, snapshot, requestTermination
	}
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "request-termination", requestTermination)); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Load(ctx, "work-1")
	confirm := TaskTransition{TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(5)}
	if action == TaskConfirmTermination {
		return store, snapshot, confirm
	}
	return store, snapshot, confirm
}
