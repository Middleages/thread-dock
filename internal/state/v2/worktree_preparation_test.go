package statev2

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func preparationTransition(invocationID InvocationID) TaskTransition {
	return TaskTransition{
		TaskID: "task-1", Action: TaskBeginWorktreePreparation, InvocationID: invocationID,
		LogicalWorkID: "logical-preparation-1", Role: roleBuilder, ReturnStage: TaskPending,
		BuilderAttempt: 1, At: invocationAt(1),
		Worktree: &WorktreeIdentity{
			CanonicalPath: "/worktree", GitCommonDir: "/repo/.git", Branch: "agent/task-1",
			BaseSHA: "0123456789012345678901234567890123456789",
		},
		Invocation: &InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"},
	}
}

func approvedPreparationStore(t *testing.T) (Store, WorkSnapshot) {
	t.Helper()
	ctx := context.Background()
	store := NewStore(t.TempDir())
	snapshot := validSnapshot()
	if _, err := createPlan(store, ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Apply(ctx, transitionRequest(t, snapshot, "approve-preparation", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}))
	if err != nil {
		t.Fatal(err)
	}
	return store, snapshot
}

func TestWorktreePreparationBeginIsDurableBeforeSideEffect(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedPreparationStore(t)
	transition := preparationTransition("prepare-1")
	prepared, err := store.Apply(ctx, taskRequest(t, snapshot, "begin-preparation", transition))
	if err != nil {
		t.Fatalf("begin preparation: %v", err)
	}
	state := prepared.TaskStates["task-1"]
	if state.Status != TaskWorktreePreparing || state.BuilderAttempt != 1 || state.LogicalWork == nil || state.Invocation == nil || state.Worktree == nil {
		t.Fatalf("prepared state = %#v", state)
	}
	if state.LogicalWork.LogicalWorkID != transition.LogicalWorkID || state.LogicalWork.Role != roleBuilder || state.LogicalWork.BuilderAttempt != 1 || !worktreesEqual(state.LogicalWork.Worktree, transition.Worktree) {
		t.Fatalf("logical work = %#v", state.LogicalWork)
	}
	if state.Invocation.InvocationID != transition.InvocationID || state.Invocation.TransitionRequestID != "begin-preparation" || state.Invocation.ProviderIdentity != "" || state.Invocation.LaunchRequested || state.Invocation.StartedAt != nil || state.Invocation.EndedAt != nil || state.Invocation.TerminationConfirmed || state.Invocation.TransientFailure {
		t.Fatalf("preparing invocation = %#v", state.Invocation)
	}
	loaded, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.TaskStates["task-1"], state) || loaded.Revision != prepared.Revision {
		t.Fatalf("preparation was not durably loaded: loaded=%#v prepared=%#v", loaded.TaskStates["task-1"], state)
	}
}

func TestWorktreePreparationReconcilePresentPromotesReserved(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedPreparationStore(t)
	prepared, err := store.Apply(ctx, taskRequest(t, snapshot, "begin-present", preparationTransition("prepare-present")))
	if err != nil {
		t.Fatal(err)
	}
	before := prepared.TaskStates["task-1"]
	reconcile := preparationTransition("prepare-present")
	reconcile.Action = TaskReconcileWorktreePreparation
	reconcile.Preparation = &WorktreePreparationEvidence{OperationTerminated: true, WorktreeExists: true, IdentityMatches: true, Diagnostic: "git worktree preparation completed"}
	reconciled, err := store.Apply(ctx, taskRequest(t, prepared, "reconcile-present", reconcile))
	if err != nil {
		t.Fatalf("reconcile present: %v", err)
	}
	state := reconciled.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.BuilderAttempt != before.BuilderAttempt || !reflect.DeepEqual(state.LogicalWork, before.LogicalWork) || !reflect.DeepEqual(state.Worktree, before.Worktree) || !reflect.DeepEqual(state.Invocation, before.Invocation) || len(state.InvocationHistory) != 1 {
		t.Fatalf("present reconciliation changed identity: before=%#v after=%#v", before, state)
	}
}

func TestWorktreePreparationReconcileMissingRetainsLogicalIdentityForRetry(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedPreparationStore(t)
	prepared, err := store.Apply(ctx, taskRequest(t, snapshot, "begin-missing", preparationTransition("prepare-missing")))
	if err != nil {
		t.Fatal(err)
	}
	before := prepared.TaskStates["task-1"]
	reconcile := preparationTransition("prepare-missing")
	reconcile.Action = TaskReconcileWorktreePreparation
	reconcile.Preparation = &WorktreePreparationEvidence{OperationTerminated: true, Diagnostic: "worktree operation ended before the path appeared"}
	missing, err := store.Apply(ctx, taskRequest(t, prepared, "reconcile-missing", reconcile))
	if err != nil {
		t.Fatalf("reconcile missing: %v", err)
	}
	state := missing.TaskStates["task-1"]
	if state.Status != TaskPending || state.Invocation != nil || state.Worktree != nil || state.LogicalWork == nil || !worktreesEqual(state.LogicalWork.Worktree, before.Worktree) || state.LogicalWork.LogicalWorkID != before.LogicalWork.LogicalWorkID || state.BuilderAttempt != 1 || len(state.InvocationHistory) != 1 || state.RepairCount != before.RepairCount || state.RecoveryCount != before.RecoveryCount {
		t.Fatalf("missing reconciliation lost identity: before=%#v after=%#v", before, state)
	}
	retry := preparationTransition("prepare-retry")
	retry.LogicalWorkID = state.LogicalWork.LogicalWorkID
	retried, err := store.Apply(ctx, taskRequest(t, missing, "retry-preparation", retry))
	if err != nil {
		t.Fatalf("retry preparation: %v", err)
	}
	retryState := retried.TaskStates["task-1"]
	if retryState.Status != TaskWorktreePreparing || retryState.Invocation.InvocationID != "prepare-retry" || retryState.LogicalWork.LogicalWorkID != before.LogicalWork.LogicalWorkID || !worktreesEqual(retryState.Worktree, before.Worktree) || len(retryState.InvocationHistory) != 2 {
		t.Fatalf("retry changed logical identity: %#v", retryState)
	}
}

func TestWorktreePreparationReducerProjectsReconcileWhilePaused(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval"}
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskWorktreePreparing
	state.BuilderAttempt = 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: roleBuilder, BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1", Worktree: &WorktreeIdentity{CanonicalPath: "/worktree", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: "0123456789012345678901234567890123456789"}}
	state.Worktree = cloneWorktree(state.LogicalWork.Worktree)
	state.Invocation = &InvocationState{InvocationID: "inv", LogicalWorkID: "logical", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}
	state.InvocationHistory = []InvocationID{"inv"}
	snapshot.TaskStates["task-1"] = state
	Reduce(&snapshot)
	if snapshot.State != StateRunning || snapshot.NextAction != "reconcile" {
		t.Fatalf("unpaused projection = %q/%q", snapshot.State, snapshot.NextAction)
	}
	snapshot.Control.PauseRequested = true
	Reduce(&snapshot)
	if snapshot.State != StateRunning || snapshot.NextAction != "reconcile" {
		t.Fatalf("paused projection = %q/%q", snapshot.State, snapshot.NextAction)
	}
}

func TestWorktreePreparationStoreReplayIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedPreparationStore(t)
	request := taskRequest(t, snapshot, "begin-replay", preparationTransition("prepare-replay"))
	first, err := store.Apply(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.Apply(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replay) || len(replay.TaskStates["task-1"].InvocationHistory) != 1 || replay.Revision != first.Revision {
		t.Fatalf("replay changed preparation: first=%#v replay=%#v", first, replay)
	}
}

func TestValidateWorktreePreparingSnapshotRequiresCompleteMatchingShape(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval"}
	snapshot.State = StateQueued
	transition := preparationTransition("prepare-validation")
	if err := applyTransition(&snapshot, TransitionRequest{RequestID: "begin-validation", Task: &transition}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*TaskExecutionState)
	}{
		{name: "missing invocation", mutate: func(task *TaskExecutionState) { task.Invocation = nil }},
		{name: "missing worktree", mutate: func(task *TaskExecutionState) { task.Worktree = nil }},
		{name: "mismatched logical work", mutate: func(task *TaskExecutionState) { task.LogicalWork.Worktree.Branch = "other" }},
		{name: "provider identity", mutate: func(task *TaskExecutionState) { task.Invocation.ProviderIdentity = "provider" }},
		{name: "candidate evidence", mutate: func(task *TaskExecutionState) { task.Candidate = &CandidateEvidence{} }},
		{name: "reviewer invocation", mutate: func(task *TaskExecutionState) { task.Invocation.Role = roleReviewer }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := cloneSnapshot(t, snapshot)
			task := invalid.TaskStates["task-1"]
			tc.mutate(&task)
			invalid.TaskStates["task-1"] = task
			if err := validateSnapshot(invalid); err == nil {
				t.Fatal("validateSnapshot accepted invalid preparing shape")
			}
		})
	}
}

func TestWorktreePreparationRejectsInvalidEvidenceAndPreconditionsWithoutMutation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*WorkSnapshot, *TaskTransition)
	}{
		{name: "wrong role", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Role = roleReviewer }},
		{name: "wrong return stage", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.ReturnStage = TaskGatePassed }},
		{name: "provider prepopulated", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Invocation.ProviderIdentity = "provider" }},
		{name: "incomplete worktree", mutate: func(_ *WorkSnapshot, tr *TaskTransition) { tr.Worktree.Branch = " " }},
		{name: "paused", mutate: func(s *WorkSnapshot, _ *TaskTransition) { s.Control.PauseRequested = true; s.State = StatePaused }},
		{name: "completed", mutate: func(s *WorkSnapshot, _ *TaskTransition) { s.State = StateCompleted }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, snapshot := approvedPreparationStore(t)
			requestTransition := preparationTransition(InvocationID("invalid-" + strings.ReplaceAll(tc.name, " ", "-")))
			tc.mutate(&snapshot, &requestTransition)
			before := cloneSnapshot(t, snapshot)
			if err := applyTransition(&snapshot, TransitionRequest{RequestID: "invalid-request", Task: &requestTransition}); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want ErrInvalidTransition", err)
			}
			if !reflect.DeepEqual(before, snapshot) {
				t.Fatalf("invalid preparation mutated snapshot: before=%#v after=%#v", before, snapshot)
			}
		})
	}

	store, snapshot := approvedPreparationStore(t)
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "begin-invalid-evidence", preparationTransition("prepare-invalid-evidence"))); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []*WorktreePreparationEvidence{
		nil,
		{OperationTerminated: false, WorktreeExists: true, IdentityMatches: true, Diagnostic: "not done"},
		{OperationTerminated: true, WorktreeExists: true, IdentityMatches: false, Diagnostic: "identity mismatch"},
		{OperationTerminated: true, WorktreeExists: false, IdentityMatches: true, Diagnostic: "inconsistent"},
	} {
		reconcile := preparationTransition("prepare-invalid-evidence")
		reconcile.Action = TaskReconcileWorktreePreparation
		reconcile.Preparation = evidence
		before, err := store.Load(ctx, snapshot.WorkID)
		if err != nil {
			t.Fatal(err)
		}
		requestID := contractv2.RequestID("invalid-evidence-request-" + strings.ReplaceAll(strings.ToLower(evidenceDiagnostic(evidence)), " ", "-"))
		if _, err := store.Apply(ctx, taskRequest(t, before, requestID, reconcile)); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("evidence %#v error = %v, want ErrInvalidTransition", evidence, err)
		}
		after, err := store.Load(ctx, snapshot.WorkID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("invalid evidence mutated snapshot: before=%#v after=%#v", before, after)
		}
	}
}

func evidenceDiagnostic(evidence *WorktreePreparationEvidence) string {
	if evidence == nil {
		return "nil"
	}
	return evidence.Diagnostic
}
