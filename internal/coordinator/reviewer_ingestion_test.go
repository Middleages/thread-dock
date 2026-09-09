package coordinator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
)

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
