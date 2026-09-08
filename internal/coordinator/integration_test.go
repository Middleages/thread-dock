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
	store, contract := setupRunningOwnerWork(t, root)
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
	defer func() {
		if ownerHelper.ProcessState == nil {
			_ = ownerHelper.Process.Kill()
		}
		_ = ownerHelper.Wait()
	}()
	readyCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		ready, readErr := bufio.NewReader(stdout).ReadString('\n')
		if readErr != nil {
			errCh <- readErr
			return
		}
		readyCh <- ready
	}()
	select {
	case ready := <-readyCh:
		if ready != "ready\n" {
			t.Fatalf("helper ready=%q", ready)
		}
	case readErr := <-errCh:
		t.Fatalf("helper readiness: %v", readErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for owner helper")
	}
	if err := ownerHelper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := ownerHelper.Wait(); err == nil {
		t.Fatal("owner helper unexpectedly exited cleanly")
	}
	_ = stdin.Close()
	rt := &integrationRuntime{identity: "provider-1", phase: "reconcile", events: &[]string{}}
	c := coordinator.NewCoordinator(store, rt, nil, coordinator.NewOwnerLocker(root), "successor", 43, reconcileAt)
	result, err := c.Reconcile(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkID != contract.WorkID || rt.observes != 1 || rt.launches != 0 || rt.terminates != 0 {
		t.Fatalf("result=%#v runtime=%d/%d/%d", result, rt.observes, rt.launches, rt.terminates)
	}
	rt.phase = "dispatcher"
	dispatcher := coordinator.NewRuntimeDispatcher(store, rt, coordinator.NewOwnerLocker(root), "queue", 44, reconcileAt)
	if result := <-dispatcher.SubmitRuntime(context.Background(), contract.WorkID, "task-1", "inv-1"); result.Err != nil {
		t.Fatal(result.Err)
	}
	if rt.observes != 2 || len(*rt.events) != 2 || (*rt.events)[0] != "reconcile-observe" || (*rt.events)[1] != "dispatcher-observe" {
		t.Fatalf("event order=%#v observes=%d", *rt.events, rt.observes)
	}
	if err := dispatcher.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	lease, err := coordinator.NewOwnerLocker(root).Acquire(context.Background(), contract.WorkID, "after", 44, reconcileAt)
	if err != nil {
		t.Fatal(err)
	}
	_ = lease.Release()
}

func setupRunningOwnerWork(t *testing.T, root string) (statev2.Store, contractv2.WorkItemContract) {
	t.Helper()
	projects, works := registry.NewStore(root), statev2.NewStore(root)
	project := registry.Project{ProjectID: "project-owner-loss", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	payload, _ := json.Marshal(project)
	sum := sha256.Sum256(payload)
	if _, err := projects.Create(context.Background(), project, 0, "project-owner", hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-owner-loss", ProjectID: project.ProjectID, Revision: 1, Request: "owner", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: strings.Repeat("0", 40), TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var input strings.Builder
	if err := contractv2.Write(&input, contract); err != nil {
		t.Fatal(err)
	}
	service := workflow.New(projects, works)
	if _, err := service.PlanWork(context.Background(), "owner.json", strings.NewReader(input.String()), 0, "plan-owner"); err != nil {
		t.Fatal(err)
	}
	approved, err := service.ApproveWork(context.Background(), contract.WorkID, 1, "approve-owner")
	if err != nil {
		t.Fatal(err)
	}
	worktree := &statev2.WorktreeIdentity{CanonicalPath: "/work", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: strings.Repeat("0", 40)}
	apply := func(snapshot statev2.WorkSnapshot, request statev2.TransitionRequest) statev2.WorkSnapshot {
		t.Helper()
		request.WorkID = contract.WorkID
		request.ExpectedRevision = snapshot.Revision
		var hashErr error
		request.PayloadHash, hashErr = statev2.TransitionPayloadHash(request)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		updated, applyErr := works.Apply(context.Background(), request)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		return updated
	}
	reserved := apply(approved, statev2.TransitionRequest{RequestID: "reserve-owner", Task: &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Worktree: worktree, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}})
	launched := apply(reserved, statev2.TransitionRequest{RequestID: "begin-owner", Task: &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt}})
	apply(launched, statev2.TransitionRequest{RequestID: "running-owner", Task: &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Invocation: &statev2.InvocationState{ProviderIdentity: "provider-1"}}})
	return works, contract
}

func setupReservedOwnerWork(t *testing.T, root string) (statev2.Store, contractv2.WorkItemContract) {
	t.Helper()
	projects, works := registry.NewStore(root), statev2.NewStore(root)
	project := registry.Project{ProjectID: "project-reserved", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	payload, _ := json.Marshal(project)
	sum := sha256.Sum256(payload)
	if _, err := projects.Create(context.Background(), project, 0, "project-reserved", hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-reserved", ProjectID: project.ProjectID, Revision: 1, Request: "reserved", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: strings.Repeat("0", 40), TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var input strings.Builder
	if err := contractv2.Write(&input, contract); err != nil {
		t.Fatal(err)
	}
	service := workflow.New(projects, works)
	if _, err := service.PlanWork(context.Background(), "reserved.json", strings.NewReader(input.String()), 0, "plan-reserved"); err != nil {
		t.Fatal(err)
	}
	approved, err := service.ApproveWork(context.Background(), contract.WorkID, 1, "approve-reserved")
	if err != nil {
		t.Fatal(err)
	}
	worktree := &statev2.WorktreeIdentity{CanonicalPath: "/work", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: strings.Repeat("0", 40)}
	_ = contractAfterReserve(t, works, contract, approved, worktree)
	return works, contract
}

func contractAfterReserve(t *testing.T, works statev2.Store, contract contractv2.WorkItemContract, approved statev2.WorkSnapshot, worktree *statev2.WorktreeIdentity) statev2.WorkSnapshot {
	t.Helper()
	reserve := statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Worktree: worktree, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}
	request := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: approved.Revision, RequestID: "reserve-reserved", Task: &reserve}
	var err error
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := works.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return reserved
}

func applyOwnerTransition(t *testing.T, works statev2.Store, workID contractv2.WorkID, snapshot statev2.WorkSnapshot, requestID contractv2.RequestID, task *statev2.TaskTransition, publication *statev2.PublicationTransition) statev2.WorkSnapshot {
	t.Helper()
	request := statev2.TransitionRequest{WorkID: workID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Task: task, Publication: publication}
	var err error
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := works.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return updated
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

func TestCoordinatorRealStoreRuntimeUnknownMatrixPersistsOperatorBlocker(t *testing.T) {
	for _, observation := range []coordinator.RuntimeObservation{{State: coordinator.RuntimeObservationUnknown}, {State: coordinator.RuntimeObservationNotStarted}, {}, {State: coordinator.RuntimeObservationActive, ProviderIdentity: "provider-other"}} {
		t.Run(observation.State+observation.ProviderIdentity, func(t *testing.T) {
			root := t.TempDir()
			works, contract := setupRunningOwnerWork(t, root)
			rt := &integrationRuntime{identity: "provider-1", observation: observation, observationSet: true}
			c := coordinator.NewCoordinator(works, rt, nil, coordinator.NewOwnerLocker(root), "owner", 43, reconcileAt)
			if _, err := c.Reconcile(context.Background(), contract.WorkID); err == nil {
				t.Fatal("unknown runtime unexpectedly reconciled")
			}
			after, err := works.Load(context.Background(), contract.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			task := after.TaskStates["task-1"]
			if after.State != statev2.StateNeedsOperator || task.Status != statev2.TaskNeedsOperator || rt.terminates != 0 {
				t.Fatalf("state=%q task=%q terminate=%d", after.State, task.Status, rt.terminates)
			}
			blocker := after.Control.Blocker
			if blocker == nil || blocker.Kind != statev2.BlockerKindRuntimeUnknown || blocker.OperatorRef != "owner" || blocker.TaskID != "task-1" || blocker.InvocationID != "inv-1" || blocker.Diagnostic == "" {
				t.Fatalf("blocker=%#v", blocker)
			}
		})
	}
}

func TestCoordinatorRealStoreTerminationPendingUnknownDoesNotTerminate(t *testing.T) {
	root := t.TempDir()
	works, contract := setupRunningOwnerWork(t, root)
	snapshot, err := works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	task := snapshot.TaskStates["task-1"]
	snapshot = applyOwnerTransition(t, works, contract.WorkID, snapshot, "request-termination", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRequestTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Reason: "operator pause"}, nil)
	rt := &integrationRuntime{identity: "provider-1", observation: coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown}, observationSet: true}
	c := coordinator.NewCoordinator(works, rt, nil, coordinator.NewOwnerLocker(root), "owner", 43, reconcileAt)
	if _, err := c.Reconcile(context.Background(), contract.WorkID); err == nil {
		t.Fatal("unknown termination unexpectedly reconciled")
	}
	after, err := works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != statev2.StateNeedsOperator || after.TaskStates[task.TaskID].Status != statev2.TaskNeedsOperator || rt.terminates != 0 {
		t.Fatalf("state=%q task=%q terminate=%d", after.State, after.TaskStates[task.TaskID].Status, rt.terminates)
	}
	if blocker := after.Control.Blocker; blocker == nil || blocker.Kind != statev2.BlockerKindRuntimeUnknown || blocker.OperatorRef != "owner" || blocker.TaskID != task.TaskID || blocker.InvocationID != task.Invocation.InvocationID {
		t.Fatalf("blocker=%#v", blocker)
	}
}

func TestCoordinatorRealStoreHistoricalInvocationSkippedAndPublicationSettles(t *testing.T) {
	root := t.TempDir()
	works, contract := setupRunningOwnerWork(t, root)
	snapshot, err := works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	ended := reconcileAt.Add(time.Minute)
	snapshot = applyOwnerTransition(t, works, contract.WorkID, snapshot, "confirm-history", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: ended, Reason: "finished"}, nil)
	snapshot, err = works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	publication := &statev2.PublicationTransition{Action: statev2.PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://history", Target: &statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: func() *bool { value := true; return &value }()}
	applyOwnerTransition(t, works, contract.WorkID, snapshot, "begin-history-publication", nil, publication)
	rt := &integrationRuntime{identity: "provider-1", observation: coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown}}
	pub := &integrationPublisher{observation: integrationMatch}
	c := coordinator.NewCoordinator(works, rt, pub, coordinator.NewOwnerLocker(root), "owner", 43, reconcileAt)
	if _, err := c.Reconcile(context.Background(), contract.WorkID); err != nil {
		t.Fatal(err)
	}
	after, err := works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if rt.observes != 0 || pub.observes != 1 || after.Publications["intent-1"].Status != statev2.PublicationCompleted {
		t.Fatalf("runtime observe=%d publication observe=%d status=%q", rt.observes, pub.observes, after.Publications["intent-1"].Status)
	}
}

func TestCoordinatorRealStorePausedReservedFalseReconcilesLocally(t *testing.T) {
	root := t.TempDir()
	works, contract := setupReservedOwnerWork(t, root)
	service := workflow.New(registry.NewStore(root), works)
	snapshot, err := works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PauseWork(context.Background(), contract.WorkID, snapshot.Revision, "pause-reserved"); err != nil {
		t.Fatal(err)
	}
	rt := &integrationRuntime{identity: "provider-1"}
	pub := &integrationPublisher{}
	c := coordinator.NewCoordinator(works, rt, pub, coordinator.NewOwnerLocker(root), "owner", 43, reconcileAt)
	if _, err := c.Reconcile(context.Background(), contract.WorkID); err != nil {
		t.Fatal(err)
	}
	after, err := works.Load(context.Background(), contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.TaskStates["task-1"].Status != statev2.TaskPending || rt.observes != 0 || rt.launches != 0 || pub.publishes != 0 {
		t.Fatalf("task=%q runtime=%d/%d publication=%d", after.TaskStates["task-1"].Status, rt.observes, rt.launches, pub.publishes)
	}
}

func TestCoordinatorRealStorePausedPublicationMatchOrAbsent(t *testing.T) {
	for _, match := range []bool{true, false} {
		t.Run(fmt.Sprintf("match=%t", match), func(t *testing.T) {
			root := t.TempDir()
			works, contract := setupRunningOwnerWork(t, root)
			service := workflow.New(registry.NewStore(root), works)
			snapshot, err := works.Load(context.Background(), contract.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			snapshot = applyOwnerTransition(t, works, contract.WorkID, snapshot, "term-paused", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRequestTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt, Reason: "finished"}, nil)
			snapshot = applyOwnerTransition(t, works, contract.WorkID, snapshot, "confirm-paused", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: reconcileAt.Add(time.Minute), Reason: "finished"}, nil)
			publication := &statev2.PublicationTransition{Action: statev2.PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://paused", Target: &statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: func() *bool { value := true; return &value }()}
			snapshot = applyOwnerTransition(t, works, contract.WorkID, snapshot, "begin-paused", nil, publication)
			if _, err := service.PauseWork(context.Background(), contract.WorkID, snapshot.Revision, "pause-publication"); err != nil {
				t.Fatal(err)
			}
			observation := coordinator.PublicationObservation{State: coordinator.PublicationObservationAbsent}
			if match {
				observation = integrationMatch
			}
			pub := &integrationPublisher{observation: observation}
			c := coordinator.NewCoordinator(works, nil, pub, coordinator.NewOwnerLocker(root), "owner", 43, reconcileAt)
			if _, err := c.Reconcile(context.Background(), contract.WorkID); err != nil {
				t.Fatal(err)
			}
			after, err := works.Load(context.Background(), contract.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			want := statev2.PublicationPending
			if match {
				want = statev2.PublicationCompleted
			}
			if after.Publications["intent-1"].Status != want || pub.observes != 1 || pub.publishes != 0 {
				t.Fatalf("match=%t status=%q observe/publish=%d/%d", match, after.Publications["intent-1"].Status, pub.observes, pub.publishes)
			}
		})
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
	observation                    coordinator.RuntimeObservation
	observationSet                 bool
	launches, observes, terminates int
	phase                          string
	events                         *[]string
}

func (r *integrationRuntime) Observe(context.Context, statev2.InvocationState) (coordinator.RuntimeObservation, error) {
	r.observes++
	if r.events != nil {
		*r.events = append(*r.events, r.phase+"-observe")
	}
	if r.observationSet {
		return r.observation, nil
	}
	return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationActive, ProviderIdentity: r.identity}, nil
}
func (r *integrationRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity) (string, error) {
	r.launches++
	if r.events != nil {
		*r.events = append(*r.events, "dispatcher-launch")
	}
	return r.identity, nil
}
func (r *integrationRuntime) Terminate(context.Context, statev2.InvocationState) error {
	r.terminates++
	return nil
}

type integrationPublisher struct {
	observation         coordinator.PublicationObservation
	observes, publishes int
}

var integrationMatch = coordinator.PublicationObservation{State: coordinator.PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{NodeID: "node-match", PublishedAt: reconcileAt}}

func (p *integrationPublisher) Observe(context.Context, statev2.PublicationState) (coordinator.PublicationObservation, error) {
	p.observes++
	if p.observation.State != "" {
		return p.observation, nil
	}
	return coordinator.PublicationObservation{State: coordinator.PublicationObservationAbsent}, nil
}
func (p *integrationPublisher) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	p.publishes++
	if p.observation.Receipt != nil {
		return *p.observation.Receipt, nil
	}
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
