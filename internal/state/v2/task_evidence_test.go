package statev2

import (
	"context"
	"errors"
	"testing"
	contractv2 "thread-dock/internal/contract/v2"
)

const (
	candidateSHA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	treeSHA        = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	integrationSHA = "dddddddddddddddddddddddddddddddddddddddd"
)

func advanceBuilderToTerminated(t *testing.T, store Store, snapshot WorkSnapshot, transient bool) WorkSnapshot {
	t.Helper()
	ctx := context.Background()
	reserve := builderReserveTransition(invocationAt(1))
	var err error
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "reserve", reserve)); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id string
		tr TaskTransition
	}{
		{"launch", TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}},
		{"running", TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Invocation: &InvocationState{ProviderProcess: "pid"}}},
		{"terminated", TaskTransition{TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Transient: transient}},
	} {
		id, tr := item.id, item.tr
		if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID(id), tr)); err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}

func approvedStore(t *testing.T) (Store, WorkSnapshot) {
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
	snapshot, err = store.Apply(ctx, transitionRequest(t, snapshot, "approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}))
	if err != nil {
		t.Fatal(err)
	}
	return store, snapshot
}

func candidateTransition() TaskTransition {
	return TaskTransition{TaskID: "task-1", Action: TaskRecordCandidate, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(5), Candidate: &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"internal/state/v2/task_transition.go"}}}
}

func TestStoreApplyRecordsEvidenceChainWithExactSHA(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	var err error
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "candidate", candidateTransition())); err != nil {
		t.Fatal(err)
	}
	if snapshot.TaskStates["task-1"].Status != TaskCandidateReady {
		t.Fatalf("candidate status = %q", snapshot.TaskStates["task-1"].Status)
	}
	gate := TaskTransition{TaskID: "task-1", Action: TaskRecordGate, Role: roleBuilder, BuilderAttempt: 1, At: invocationAt(6), Gate: &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"go test ./..."}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(6)}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "gate", gate)); err != nil {
		t.Fatal(err)
	}
	if snapshot.TaskStates["task-1"].Status != TaskGatePassed {
		t.Fatalf("gate status = %q", snapshot.TaskStates["task-1"].Status)
	}
	reviewer := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(7), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "review", BaseSHA: treeSHA}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "review-reserve", reviewer)); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id string
		tr TaskTransition
	}{
		{"review-launch", TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(8)}},
		{"review-running", TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(9), Invocation: &InvocationState{ProviderProcess: "review-pid"}}},
		{"review-term", TaskTransition{TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(10)}},
	} {
		id, tr := item.id, item.tr
		if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID(id), tr)); err != nil {
			t.Fatal(err)
		}
	}
	review := TaskTransition{TaskID: "task-1", Action: TaskRecordReview, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(11), Review: &ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: true, Findings: []ReviewFinding{}, ObservedAt: invocationAt(11)}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "review", review)); err != nil {
		t.Fatal(err)
	}
	integration := TaskTransition{TaskID: "task-1", Action: TaskRecordIntegration, Role: roleBuilder, BuilderAttempt: 1, At: invocationAt(12), Integration: &IntegrationEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, IntegrationHEAD: integrationSHA, RelationVerified: true, ObservedAt: invocationAt(12)}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "integration", integration)); err != nil {
		t.Fatal(err)
	}
	state := snapshot.TaskStates["task-1"]
	if state.Status != TaskIntegrated || state.Integration == nil || !state.Integration.RelationVerified || state.Integration.IntegrationHEAD != integrationSHA {
		t.Fatalf("integration state = %#v", state)
	}
}

func TestRecordEvidenceRejectsMismatchedSHAWithoutWrite(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	tr := candidateTransition()
	tr.Candidate.CandidateSHA = "not-a-sha"
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "bad-candidate", tr)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v", err)
	}
	after, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.TaskStates["task-1"].Status != TaskTerminated || after.TaskStates["task-1"].Candidate != nil {
		t.Fatalf("invalid evidence wrote state: after=%#v", after)
	}
}

func TestRecordCandidateAcceptsEmptyNonNilChangedFiles(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	tr := candidateTransition()
	tr.Candidate.ChangedFiles = []string{}
	got, err := store.Apply(ctx, taskRequest(t, snapshot, "empty-files", tr))
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskStates["task-1"].Status != TaskCandidateReady || got.TaskStates["task-1"].Candidate.ChangedFiles == nil {
		t.Fatalf("candidate = %#v", got.TaskStates["task-1"].Candidate)
	}
}
