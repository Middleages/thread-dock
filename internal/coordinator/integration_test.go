package coordinator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

func TestCoordinatorIntegrationUsesDurableStateForNoLaunchProof(t *testing.T) {
	root := t.TempDir()
	store := statev2.NewStore(root)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-actual", ProjectID: "project-actual", Revision: 1, Request: "run", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-actual", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, contract); err != nil {
		t.Fatal(err)
	}
	contractSum := sha256.Sum256(encoded.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(contractSum[:]), Contract: contract, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-actual": {TaskID: "task-actual", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	if _, err := store.CreatePlan(context.Background(), snapshot, "plan", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	var err error
	approveRequest := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: 1, RequestID: "approve", Work: &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}}
	approveRequest.PayloadHash, err = statev2.TransitionPayloadHash(approveRequest)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := store.Apply(context.Background(), approveRequest)
	if err != nil {
		t.Fatal(err)
	}
	worktree := &statev2.WorktreeIdentity{CanonicalPath: "/work", GitCommonDir: "/repo/.git", Branch: "agent/task-actual", BaseSHA: "0123456789012345678901234567890123456789"}
	reserve := statev2.TaskTransition{TaskID: "task-actual", Action: statev2.TaskReserveInvocation, InvocationID: "inv-actual", LogicalWorkID: "logical-actual", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Worktree: worktree, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}
	request := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: approved.Revision, RequestID: "reserve", Task: &reserve}
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	locker := NewOwnerLocker(root)
	c := NewCoordinator(store, &reconcileTestRuntime{}, nil, locker, "owner-actual", 41, reconcileAt)
	result, err := c.Reconcile(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if result.NextAction != "run" {
		t.Fatalf("result = %#v", result)
	}
	after, err := store.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.TaskStates["task-actual"].Status; got != statev2.TaskPending {
		t.Fatalf("task status = %q", got)
	}
}

// This fake-backed flow intentionally performs reserve/candidate/publication
// setup as durable state transitions owned by the state layer. Coordinator is
// responsible only for restart settlement and provider call cardinality.
func TestCoordinatorIntegrationLaunchTerminateCandidatePublication(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcileSnapshot(statev2.TaskInvocationReserved)}
	task := st.snapshot.TaskStates["task-1"]
	task.Invocation.LaunchRequested = true
	st.snapshot.TaskStates["task-1"] = task
	rt := &reconcileTestRuntime{observation: RuntimeObservation{State: RuntimeObservationActive, ProviderIdentity: "provider-1"}}
	pub := &reconcileTestPublisher{observation: PublicationObservation{State: PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: reconcileAt}}}
	locker := &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}
	c := NewCoordinator(st, rt, pub, locker, "owner-1", 41, reconcileAt)

	if _, err := c.Reconcile(context.Background(), contractv2.WorkID("work-1")); err != nil {
		t.Fatal(err)
	}
	if st.snapshot.TaskStates["task-1"].Status != statev2.TaskRunning || rt.launches != 0 || rt.observes != 1 {
		t.Fatalf("launch settlement status=%q runtime=%d/%d", st.snapshot.TaskStates["task-1"].Status, rt.launches, rt.observes)
	}

	task = st.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	st.snapshot.TaskStates["task-1"] = task
	rt.observation = RuntimeObservation{State: RuntimeObservationEnded, ProviderIdentity: "provider-1", EndedAt: func() *time.Time { at := reconcileAt.Add(time.Minute); return &at }()}
	if _, err := c.Reconcile(context.Background(), "work-1"); err != nil {
		t.Fatal(err)
	}
	if st.snapshot.TaskStates["task-1"].Status != statev2.TaskTerminated || rt.terminates != 0 {
		t.Fatalf("termination status=%q terminate calls=%d", st.snapshot.TaskStates["task-1"].Status, rt.terminates)
	}

	task = st.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskIntegrated
	task.Invocation = nil
	st.snapshot.TaskStates["task-1"] = task
	st.snapshot.Publications["intent-1"] = statev2.PublicationState{IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, Status: statev2.PublicationPending, PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PayloadRef: "artifact://one", Target: statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: true}
	if _, err := c.Reconcile(context.Background(), "work-1"); err != nil {
		t.Fatal(err)
	}
	if pub.observes != 1 || pub.publishes != 0 || st.snapshot.Publications["intent-1"].Status != statev2.PublicationCompleted {
		t.Fatalf("publication calls=%d/%d status=%q", pub.observes, pub.publishes, st.snapshot.Publications["intent-1"].Status)
	}
}
