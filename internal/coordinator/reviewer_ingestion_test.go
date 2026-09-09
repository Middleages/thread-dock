package coordinator

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
)

func TestSubmitRuntimeActiveReviewerArtifactLeavesRealStoreUnchanged(t *testing.T) {
	store, coord, snapshot, artifact := asyncReviewerFixture(t)
	before, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	runtime := coord.Runtime.(*sequenceRuntime)
	runtime.observations = []RuntimeObservation{{State: RuntimeObservationActive, ProviderIdentity: "sequence-provider", Artifact: artifact}}
	result := <-coord.SubmitRuntime(context.Background(), snapshot.WorkID, "task-1", "review-inv")
	after, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("result=%#v\nbefore=%#v\nafter=%#v", result, before, after)
	}
	_ = coord.Close(context.Background())
}

func TestSubmitRuntimeEndedReviewerArtifactAfterActiveIsIngested(t *testing.T) {
	store, coord, snapshot, artifact := asyncReviewerFixture(t)
	runtime := coord.Runtime.(*sequenceRuntime)
	runtime.observations = []RuntimeObservation{{State: RuntimeObservationActive, ProviderIdentity: "sequence-provider", Artifact: artifact}, {State: RuntimeObservationEnded, ProviderIdentity: "sequence-provider", EndedAt: ptrTime(time.Now().UTC()), Artifact: artifact}}
	first := <-coord.SubmitRuntime(context.Background(), snapshot.WorkID, "task-1", "review-inv")
	if first.Err != nil || first.Snapshot.TaskStates["task-1"].Status != statev2.TaskRunning {
		t.Fatalf("active result=%#v", first)
	}
	second := <-coord.SubmitRuntime(context.Background(), snapshot.WorkID, "task-1", "review-inv")
	task := second.Snapshot.TaskStates["task-1"]
	if second.Err != nil || task.Status != statev2.TaskAccepted || task.Review == nil || task.Review.CandidateSHA != snapshot.TaskStates["task-1"].Candidate.CandidateSHA || runtime.launches != 0 || len(task.InvocationHistory) != len(snapshot.TaskStates["task-1"].InvocationHistory) {
		t.Fatalf("ended result=%#v task=%#v", second, task)
	}
	persisted, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.TaskStates["task-1"].Review == nil || persisted.TaskStates["task-1"].Status != statev2.TaskAccepted {
		t.Fatalf("persisted=%#v", persisted.TaskStates["task-1"])
	}
	_ = coord.Close(context.Background())
}

func TestSubmitRuntimeActiveBuilderArtifactStillNeedsOperator(t *testing.T) {
	store, git, snapshot, candidate, root := newRunningBuilderFixture(t)
	artifact := &runtimecontract.ArtifactEnvelope{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, Status: "success", Result: []byte(`{"commitSha":"` + candidate + `","verification":[{"command":"check","outcome":"passed","duration":"1s"}]}`)}
	runtime := &sequenceRuntime{observations: []RuntimeObservation{{State: RuntimeObservationActive, ProviderIdentity: "provider-1"}, {State: RuntimeObservationActive, ProviderIdentity: "provider-1", Artifact: artifact}}}
	coord := NewCoordinator(store, runtime, nil, git, NewOwnerLocker(root), "builder-active-owner", 0, time.Now().UTC())
	if _, err := coord.Activate(context.Background(), snapshot.WorkID); err != nil {
		t.Fatal(err)
	}
	result := <-coord.SubmitRuntime(context.Background(), snapshot.WorkID, "task-1", "inv-1")
	persisted, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	task := persisted.TaskStates["task-1"]
	if result.Err == nil || task.Status != statev2.TaskNeedsOperator || persisted.Control.Blocker == nil || persisted.Control.Blocker.TaskID != "task-1" || task.Candidate != nil || task.Review != nil || task.Integration != nil {
		t.Fatalf("result=%#v task=%#v blocker=%#v", result, task, persisted.Control.Blocker)
	}
	_ = coord.Close(context.Background())
}

func asyncReviewerFixture(t *testing.T) (statev2.Store, *Coordinator, statev2.WorkSnapshot, *runtimecontract.ArtifactEnvelope) {
	store, git, snapshot, candidate, root := newConfirmedBuilderFixture(t)
	d := &publicationDispatcher{state: store, inspector: git, ownerID: "fixture-owner", pid: 1, startedAt: time.Now().UTC()}
	result, err := d.ingestBuilderArtifact(context.Background(), snapshot, "task-1", &runtimecontract.ArtifactEnvelope{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, Status: "success", Result: []byte(`{"commitSha":"` + candidate + `","verification":[{"command":"check","outcome":"passed","duration":"1s"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot = result.Snapshot
	at := time.Now().UTC()
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRecordGate, Role: "builder", BuilderAttempt: 1, At: at, Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: at}}, "gate")
	runtime := &sequenceRuntime{}
	coord := NewCoordinator(store, runtime, nil, git, NewOwnerLocker(root), "async-review-owner", 0, at)
	if _, err := coord.Activate(context.Background(), snapshot.WorkID); err != nil {
		t.Fatal(err)
	}
	wt := snapshot.TaskStates["task-1"].Worktree
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: at, Worktree: wt, Invocation: &statev2.InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}, "review-reserve")
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: at}, "review-begin")
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: at, Invocation: &statev2.InvocationState{ProviderIdentity: "sequence-provider"}}, "review-running")
	snapshot, err = store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	reviewJSON, err := json.Marshal(runtimecontract.ReviewerResult{ReviewedSHA: candidate, Decision: "accept", BlockingFindings: []runtimecontract.ReviewerFinding{}})
	if err != nil {
		t.Fatal(err)
	}
	return store, coord, snapshot, &runtimecontract.ArtifactEnvelope{RequestID: "review-inv", Role: runtimecontract.RoleReviewer, Status: "success", Result: reviewJSON}
}

func TestReviewerIngestionRealStoreAcceptAndBlockMatrix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision string
		findings []runtimecontract.ReviewerFinding
		want     statev2.TaskStatus
	}{
		{name: "accept", decision: "accept", findings: []runtimecontract.ReviewerFinding{}, want: statev2.TaskAccepted},
		{name: "block", decision: "block", findings: []runtimecontract.ReviewerFinding{{Code: "unsafe", Diagnostic: "unsafe change"}}, want: statev2.TaskReviewBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, dispatcher, snapshot := terminatedReviewerFixture(t)
			candidateSHA := snapshot.TaskStates["task-1"].Candidate.CandidateSHA
			resultData, err := json.Marshal(runtimecontract.ReviewerResult{ReviewedSHA: candidateSHA, Decision: tc.decision, BlockingFindings: tc.findings})
			if err != nil {
				t.Fatal(err)
			}
			result, ingestErr := dispatcher.ingestReviewerArtifact(context.Background(), snapshot, "task-1", &runtimecontract.ArtifactEnvelope{RequestID: "review-inv", Role: runtimecontract.RoleReviewer, Status: "success", Result: resultData})
			if ingestErr != nil {
				t.Fatal(ingestErr)
			}
			if result.Snapshot.TaskStates["task-1"].Status != tc.want || result.Snapshot.TaskStates["task-1"].Review == nil || result.Snapshot.TaskStates["task-1"].Review.CandidateSHA != candidateSHA {
				t.Fatalf("task=%#v", result.Snapshot.TaskStates["task-1"])
			}
		})
	}
}

func TestReviewerArtifactBeforeTerminationDoesNotRecordReview(t *testing.T) {
	_, dispatcher, snapshot := terminatedReviewerFixture(t)
	task := snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	task.Invocation.TerminationConfirmed = false
	task.Invocation.EndedAt = nil
	snapshot.TaskStates["task-1"] = task
	result, _ := dispatcher.ingestReviewerArtifact(context.Background(), snapshot, "task-1", &runtimecontract.ArtifactEnvelope{RequestID: "review-inv", Role: runtimecontract.RoleReviewer, Status: "success", Result: []byte(`{"reviewedSha":"0123456789abcdef0123456789abcdef01234567","decision":"accept","blockingFindings":[]}`)})
	if result.Snapshot.TaskStates["task-1"].Review != nil {
		t.Fatal("review evidence was recorded before termination")
	}
}

func TestReviewerIngestionRejectsStaleIdentityAndMalformedArtifacts(t *testing.T) {
	validSHA := "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct {
		name   string
		mutate func(*runtimecontract.ArtifactEnvelope)
	}{
		{name: "wrong request", mutate: func(a *runtimecontract.ArtifactEnvelope) { a.RequestID = "old-review" }},
		{name: "wrong role", mutate: func(a *runtimecontract.ArtifactEnvelope) { a.Role = runtimecontract.RoleBuilder }},
		{name: "wrong reviewed sha", mutate: func(a *runtimecontract.ArtifactEnvelope) {
			a.Result = []byte(`{"reviewedSha":"` + validSHA + `","decision":"accept","blockingFindings":[]}`)
		}},
		{name: "malformed result", mutate: func(a *runtimecontract.ArtifactEnvelope) { a.Result = []byte(`{"reviewedSha":`) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, dispatcher, snapshot := terminatedReviewerFixture(t)
			candidateSHA := snapshot.TaskStates["task-1"].Candidate.CandidateSHA
			artifact := &runtimecontract.ArtifactEnvelope{RequestID: "review-inv", Role: runtimecontract.RoleReviewer, Status: "success", Result: []byte(`{"reviewedSha":"` + candidateSHA + `","decision":"accept","blockingFindings":[]}`)}
			tc.mutate(artifact)
			result, _ := dispatcher.ingestReviewerArtifact(context.Background(), snapshot, "task-1", artifact)
			if result.Snapshot.TaskStates["task-1"].Status != statev2.TaskNeedsOperator || result.Snapshot.TaskStates["task-1"].Review != nil {
				t.Fatalf("task=%#v", result.Snapshot.TaskStates["task-1"])
			}
		})
	}
}

func terminatedReviewerFixture(t *testing.T) (statev2.Store, *publicationDispatcher, statev2.WorkSnapshot) {
	t.Helper()
	store, git, snapshot, candidate, _ := newConfirmedBuilderFixture(t)
	d := &publicationDispatcher{state: store, inspector: git, ownerID: "review-owner", pid: 1, startedAt: time.Now().UTC()}
	artifact := &runtimecontract.ArtifactEnvelope{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, Status: "success", Result: []byte(`{"commitSha":"` + candidate + `","verification":[{"command":"check","outcome":"passed","duration":"1s"}]}`)}
	result, err := d.ingestBuilderArtifact(context.Background(), snapshot, "task-1", artifact)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = result.Snapshot
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRecordGate, Role: "builder", BuilderAttempt: 1, At: time.Now().UTC(), Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: time.Now().UTC()}}, "gate")
	wt := snapshot.TaskStates["task-1"].Worktree
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: wt, Invocation: &statev2.InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}, "review-reserve")
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC()}, "review-begin")
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Invocation: &statev2.InvocationState{ProviderIdentity: "review-provider"}}, "review-running")
	snapshot = applyBuilderTask(t, store, snapshot, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Reason: "finished"}, "review-terminated")
	return store, d, snapshot
}
