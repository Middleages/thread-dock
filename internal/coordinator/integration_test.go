package coordinator_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	coordinator "thread-dock/internal/coordinator"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/workflow"
)

var reconcileAt = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

func TestCoordinatorOwnerLossAcquiresBeforeAnyRuntimeCall(t *testing.T) {
	if os.Getenv("THREADDOCK_COORDINATOR_OWNER_HELPER") == "1" {
		root := os.Getenv("THREADDOCK_COORDINATOR_OWNER_ROOT")
		lease, err := coordinator.NewOwnerLocker(root).Acquire(context.Background(), "work-owner-loss", "helper", os.Getpid(), reconcileAt)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, "ready")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		_ = lease.Release()
		return
	}
	root := t.TempDir()
	store := statev2.NewStore(root)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-owner-loss", ProjectID: "project-owner-loss", Revision: 1, Request: "owner", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(sum[:]), Contract: contract, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	if _, err := store.CreatePlan(context.Background(), snapshot, "plan", "hash"); err != nil {
		t.Fatal(err)
	}
	ownerHelper := exec.Command(os.Args[0], "-test.run", "^TestCoordinatorOwnerLossAcquiresBeforeAnyRuntimeCall$")
	ownerHelper.Env = append(os.Environ(), "THREADDOCK_COORDINATOR_OWNER_HELPER=1", "THREADDOCK_COORDINATOR_OWNER_ROOT="+root)
	stdout, err := ownerHelper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := ownerHelper.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := ownerHelper.Start(); err != nil {
		t.Fatal(err)
	}
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatalf("helper ready=%q err=%v", ready, err)
	}
	if err := ownerHelper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := ownerHelper.Wait(); err == nil {
		t.Fatal("owner helper unexpectedly exited cleanly")
	}
	_ = stdin.Close()
	rt := &integrationRuntime{}
	c := coordinator.NewCoordinator(store, rt, nil, coordinator.NewOwnerLocker(root), "successor", 43, reconcileAt)
	result, err := c.Reconcile(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkID != contract.WorkID || rt.observes != 0 || rt.launches != 0 || rt.terminates != 0 {
		t.Fatalf("result=%#v runtime=%d/%d/%d", result, rt.observes, rt.launches, rt.terminates)
	}
	lease, err := coordinator.NewOwnerLocker(root).Acquire(context.Background(), contract.WorkID, "after", 44, reconcileAt)
	if err != nil {
		t.Fatal(err)
	}
	_ = lease.Release()
}

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
	locker := coordinator.NewOwnerLocker(root)
	c := coordinator.NewCoordinator(store, &integrationRuntime{}, nil, locker, "owner-actual", 41, reconcileAt)
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
	root := t.TempDir()
	projects, works := registry.NewStore(root), statev2.NewStore(root)
	project := registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	projectBytes, _ := json.Marshal(project)
	projectSum := sha256.Sum256(projectBytes)
	if _, err := projects.Create(context.Background(), project, 0, "project", hex.EncodeToString(projectSum[:])); err != nil {
		t.Fatal(err)
	}
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-flow", ProjectID: "project-1", Revision: 1, Request: "flow", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "not required"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var input strings.Builder
	if err := contractv2.Write(&input, contract); err != nil {
		t.Fatal(err)
	}
	service := workflow.New(projects, works)
	if _, err := service.PlanWork(context.Background(), "flow.json", strings.NewReader(input.String()), 0, "plan-flow"); err != nil {
		t.Fatal(err)
	}
	approved, err := service.ApproveWork(context.Background(), contract.WorkID, 1, "approve-flow")
	if err != nil {
		t.Fatal(err)
	}
	worktree := &statev2.WorktreeIdentity{CanonicalPath: "/work", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: "0123456789012345678901234567890123456789"}
	reserve := statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Worktree: worktree, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}
	reserveRequest := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: approved.Revision, RequestID: "reserve-flow", Task: &reserve}
	reserveRequest.PayloadHash, err = statev2.TransitionPayloadHash(reserveRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := works.Apply(context.Background(), reserveRequest); err != nil {
		t.Fatal(err)
	}
	rt := &integrationRuntime{identity: "provider-1"}
	locker := &integrationLocker{}
	dispatcher := coordinator.NewRuntimeDispatcher(works, rt, locker, "owner-dispatch", 41, reconcileAt)
	if result := <-dispatcher.SubmitRuntime(context.Background(), contract.WorkID, "task-1", "inv-1"); result.Err != nil {
		t.Fatal(result.Err)
	}
	if rt.launches != 1 {
		t.Fatalf("launch calls = %d, want 1", rt.launches)
	}
	beforePause, err := service.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PauseWork(context.Background(), contract.WorkID, beforePause.Revision, "pause-flow"); err != nil {
		t.Fatal(err)
	}
	if result := <-dispatcher.SubmitRuntime(context.Background(), contract.WorkID, "task-1", "inv-1"); result.Err != nil {
		t.Fatal(result.Err)
	}
	if rt.terminates != 1 {
		t.Fatalf("terminate calls = %d, want 1", rt.terminates)
	}
	if locker.acquires != 1 || locker.releases != 0 {
		t.Fatalf("dispatcher owner calls before close = acquire %d release %d", locker.acquires, locker.releases)
	}
	if err := dispatcher.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if locker.releases != 1 {
		t.Fatalf("dispatcher owner releases = %d, want 1", locker.releases)
	}
	paused, err := service.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.TaskStates["task-1"].Status != statev2.TaskTerminated {
		t.Fatalf("task after pause = %q", paused.TaskStates["task-1"].Status)
	}
	if _, err := service.ResumeWork(context.Background(), contract.WorkID, paused.Revision, "resume-flow"); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paused.TaskStates, resumed.TaskStates) || !reflect.DeepEqual(paused.EvidenceRefs, resumed.EvidenceRefs) {
		t.Fatalf("pause/resume changed durable task evidence: paused=%#v resumed=%#v", paused.TaskStates, resumed.TaskStates)
	}
	candidateSHA, treeSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	candidate := statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRecordCandidate, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt.Add(2 * time.Minute), Candidate: &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"internal/example.go"}}}
	candidateRequest := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: resumed.Revision, RequestID: "candidate-flow", Task: &candidate}
	candidateRequest.PayloadHash, err = statev2.TransitionPayloadHash(candidateRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := works.Apply(context.Background(), candidateRequest); err != nil {
		t.Fatal(err)
	}
	ready, err := service.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	payloadHash := strings.Repeat("c", 64)
	completionRequired := true
	publication := statev2.PublicationTransition{Action: statev2.PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, PayloadHash: payloadHash, PayloadRef: "artifact://flow", Target: &statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: &completionRequired}
	publicationRequest := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: ready.Revision, RequestID: "publication-flow", Publication: &publication}
	publicationRequest.PayloadHash, err = statev2.TransitionPayloadHash(publicationRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := works.Apply(context.Background(), publicationRequest); err != nil {
		t.Fatal(err)
	}
	pub := &integrationPublisher{}
	reconcileLocker := &integrationLocker{}
	reconciler := coordinator.NewCoordinator(works, nil, pub, reconcileLocker, "owner-reconcile", 42, reconcileAt)
	workflowWithCoordinator := workflow.NewWithCoordinator(projects, works, reconciler)
	statusBefore, err := workflowWithCoordinator.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflowWithCoordinator.ReconcileWork(context.Background(), contract.WorkID); err != nil {
		t.Fatal(err)
	}
	statusAfter, err := workflowWithCoordinator.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if statusAfter.Revision == statusBefore.Revision || pub.observes != 1 || pub.publishes != 1 || statusAfter.Publications["intent-1"].Status != statev2.PublicationCompleted {
		t.Fatalf("status revision=%d/%d publication calls=%d/%d status=%q", statusBefore.Revision, statusAfter.Revision, pub.observes, pub.publishes, statusAfter.Publications["intent-1"].Status)
	}
	if reconcileLocker.acquires != 1 || reconcileLocker.releases != 1 {
		t.Fatalf("reconcile owner calls = acquire %d release %d", reconcileLocker.acquires, reconcileLocker.releases)
	}
	stable, err := workflowWithCoordinator.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if stable.Revision != statusAfter.Revision {
		t.Fatalf("repeated status mutated revision %d -> %d", statusAfter.Revision, stable.Revision)
	}
	beforeSnapshot, err := workflowWithCoordinator.Snapshot(context.Background(), reconcileAt.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	afterSnapshot, err := workflowWithCoordinator.Snapshot(context.Background(), reconcileAt.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeSnapshot, afterSnapshot) {
		t.Fatal("repeated Snapshot mutated its result")
	}
	finalStatus, err := workflowWithCoordinator.Status(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if finalStatus.Revision != statusAfter.Revision {
		t.Fatalf("Snapshot reads mutated durable revision %d -> %d", statusAfter.Revision, finalStatus.Revision)
	}
}

type integrationRuntime struct {
	identity                       string
	launches, observes, terminates int
}

func (r *integrationRuntime) Observe(context.Context, statev2.InvocationState) (coordinator.RuntimeObservation, error) {
	r.observes++
	return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationActive, ProviderIdentity: r.identity}, nil
}
func (r *integrationRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity) (string, error) {
	r.launches++
	return r.identity, nil
}
func (r *integrationRuntime) Terminate(context.Context, statev2.InvocationState) error {
	r.terminates++
	return nil
}

type integrationPublisher struct{ observes, publishes int }

func (p *integrationPublisher) Observe(context.Context, statev2.PublicationState) (coordinator.PublicationObservation, error) {
	p.observes++
	return coordinator.PublicationObservation{State: coordinator.PublicationObservationAbsent}, nil
}
func (p *integrationPublisher) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	p.publishes++
	return statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: reconcileAt}, nil
}

type integrationLocker struct{ acquires, releases int }

func (l *integrationLocker) Acquire(_ context.Context, workID contractv2.WorkID, ownerID coordinator.OwnerID, pid int, startedAt time.Time) (coordinator.OwnerLease, error) {
	l.acquires++
	return &integrationLease{record: coordinator.OwnerRecord{WorkID: workID, OwnerID: ownerID, PID: pid, StartedAt: startedAt}, locker: l}, nil
}

type integrationLease struct {
	record coordinator.OwnerRecord
	locker *integrationLocker
}

func (l *integrationLease) Record() coordinator.OwnerRecord { return l.record }
func (l *integrationLease) Release() error {
	l.locker.releases++
	return nil
}
