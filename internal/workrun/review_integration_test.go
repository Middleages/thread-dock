package workrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/runner"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

func TestReviewIntegrationServiceRequiresDependencies(t *testing.T) {
	service := NewReviewIntegrationService(nil, nil, nil, "operator", nil, ReviewIntegrationBinding{})
	_, err := service.Advance(context.Background(), statev2.WorkSnapshot{WorkID: contractv2.WorkID("work-1")}, "task-1", "request-1")
	if err == nil {
		t.Fatal("missing integration dependencies unexpectedly accepted")
	}
}

func TestReviewIntegrationRealStoreReviewerReplayAccept(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerEvidenceFixture(t)
	git := worktree.New(runner.OSRunner{}, "git", root, repo)
	runtime := &realReviewerRuntime{candidateSHA: candidate}
	coordinatorRuntime := coordinator.NewCoordinator(store, runtime, nil, git, coordinator.NewOwnerLocker(root), "evidence-owner", 0, time.Now().UTC())
	service := NewReviewIntegrationService(store, coordinatorRuntime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	first, firstErr := service.Advance(context.Background(), snapshot, "task-1", "caller-1")
	if firstErr != nil || first.TaskStates["task-1"].Status != statev2.TaskRunning || runtime.launches != 1 {
		t.Fatalf("first advance status=%q err=%v launches=%d", first.TaskStates["task-1"].Status, firstErr, runtime.launches)
	}
	secondInput, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	second, secondErr := service.Advance(context.Background(), secondInput, "task-1", "caller-2")
	if secondErr != nil || second.TaskStates["task-1"].Status != statev2.TaskAccepted || runtime.launches != 1 || runtime.observes != 1 {
		t.Fatalf("second advance status=%q err=%v launches/observes=%d/%d", second.TaskStates["task-1"].Status, secondErr, runtime.launches, runtime.observes)
	}
	if second.TaskStates["task-1"].Review == nil || second.TaskStates["task-1"].Review.CandidateSHA != candidate {
		t.Fatalf("review evidence=%#v", second.TaskStates["task-1"].Review)
	}
	_ = coordinatorRuntime.Close(context.Background())
}

func TestReviewIntegrationRealStoreRestartActivateConvergesBeforeSubmit(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerTerminatedFixture(t)
	inspector := worktree.New(runner.OSRunner{}, "git", root, repo)
	runtime := &realReviewerRuntime{candidateSHA: candidate}
	coord := coordinator.NewCoordinator(store, runtime, nil, inspector, coordinator.NewOwnerLocker(root), "restart-owner", 0, time.Now().UTC())
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40)}
	service := NewReviewIntegrationService(store, coord, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "restart-caller")
	if err != nil || got.TaskStates["task-1"].Status != statev2.TaskIntegrated || runtime.launches != 0 || runtime.observes != 1 || git.mergedSHA != candidate {
		t.Fatalf("status=%q err=%v launches/observes=%d/%d merge=%q", got.TaskStates["task-1"].Status, err, runtime.launches, runtime.observes, git.mergedSHA)
	}
}

func TestReviewerActiveArtifactDoesNotMutateRealStore(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerGateFixture(t)
	state := applyForegroundTransition(t, store, snapshot, "review-reserve", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: snapshot.TaskStates["task-1"].Worktree, Invocation: &statev2.InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime"}})
	state = applyForegroundTransition(t, store, state, "review-begin", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC()})
	state = applyForegroundTransition(t, store, state, "review-running", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Invocation: &statev2.InvocationState{ProviderIdentity: "review-provider"}})
	before, err := store.Load(context.Background(), state.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &realReviewerRuntime{candidateSHA: candidate, active: true}
	git := worktree.New(runner.OSRunner{}, "git", root, repo)
	coord := coordinator.NewCoordinator(store, runtime, nil, git, coordinator.NewOwnerLocker(root), "active-owner", 0, time.Now().UTC())
	if _, err := coord.Activate(context.Background(), state.WorkID); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load(context.Background(), state.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("active artifact mutated durable store\nbefore=%#v\nafter=%#v", before, after)
	}
	runtime.active = false
	result := <-coord.SubmitRuntime(context.Background(), state.WorkID, "task-1", "review-inv")
	if result.Err != nil || result.Snapshot.TaskStates["task-1"].Status != statev2.TaskAccepted || result.Snapshot.TaskStates["task-1"].Review == nil {
		t.Fatalf("ended result=%#v", result)
	}
	_ = coord.Close(context.Background())
}

func TestReviewIntegrationRealStoreReviewerReplayBlockNoGit(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerEvidenceFixture(t)
	git := worktree.New(runner.OSRunner{}, "git", root, repo)
	runtime := &realReviewerRuntime{candidateSHA: candidate, decision: "block"}
	coord := coordinator.NewCoordinator(store, runtime, nil, git, coordinator.NewOwnerLocker(root), "block-owner", 0, time.Now().UTC())
	countingGit := &countingReviewGit{}
	service := NewReviewIntegrationService(store, coord, countingGit, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	first, err := service.Advance(context.Background(), snapshot, "task-1", "caller-1")
	if err != nil || first.TaskStates["task-1"].Status != statev2.TaskRunning {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	secondInput, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Advance(context.Background(), secondInput, "task-1", "caller-2")
	if err != nil || second.TaskStates["task-1"].Status != statev2.TaskReviewBlocked || second.TaskStates["task-1"].Review == nil || second.TaskStates["task-1"].Review.Accepted || len(second.TaskStates["task-1"].Review.Findings) != 1 || second.TaskStates["task-1"].Integration != nil || countingGit.calls != 0 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestReviewIntegrationRealStoreAcceptedCandidateRealGitIntegration(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerEvidenceFixture(t)
	git := worktree.New(runner.OSRunner{}, "git", root, repo)
	runtime := &realReviewerRuntime{candidateSHA: candidate}
	coord := coordinator.NewCoordinator(store, runtime, nil, git, coordinator.NewOwnerLocker(root), "integration-owner", 0, time.Now().UTC())
	binding := ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"}
	service := NewReviewIntegrationService(store, coord, git, "operator", time.Now, binding)
	first, err := service.Advance(context.Background(), snapshot, "task-1", "caller-1")
	if err != nil || first.TaskStates["task-1"].Status != statev2.TaskRunning {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	current, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := service.Advance(context.Background(), current, "task-1", "caller-2")
	if err != nil || accepted.TaskStates["task-1"].Status != statev2.TaskAccepted {
		t.Fatalf("accepted=%#v err=%v", accepted, err)
	}
	integrated, err := service.Advance(context.Background(), accepted, "task-1", "caller-3")
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	evidence := persisted.TaskStates["task-1"].Integration
	if integrated.TaskStates["task-1"].Status != statev2.TaskIntegrated || evidence == nil || evidence.CandidateSHA != candidate || evidence.IntegrationHEAD == candidate || !evidence.RelationVerified {
		t.Fatalf("integrated=%#v evidence=%#v", integrated.TaskStates["task-1"], evidence)
	}
	isAncestor, err := git.IsAncestor(context.Background(), binding.IntegrationPath, candidate)
	if err != nil || !isAncestor {
		t.Fatalf("ancestor=%v err=%v", isAncestor, err)
	}
}

func TestReviewIntegrationRealStoreConflictAbortsAndPersistsBlocker(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerEvidenceFixture(t)
	accepted, err := applyRealReviewerAccept(t, store, snapshot, candidate, repo, root, "conflict", "accept")
	if err != nil {
		t.Fatal(err)
	}
	git := &countingReviewGit{mergeErr: worktree.ErrConflict}
	service := NewReviewIntegrationService(store, &countingReviewRuntime{}, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), accepted, "task-1", "conflict-caller")
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	task := persisted.TaskStates["task-1"]
	if git.abort != 1 || task.Status != statev2.TaskNeedsOperator || persisted.Control.Blocker == nil || persisted.Control.Blocker.TaskID != "task-1" || task.Integration != nil || got.TaskStates["task-1"].Status != statev2.TaskNeedsOperator {
		t.Fatalf("task=%#v blocker=%#v abort=%d", task, persisted.Control.Blocker, git.abort)
	}
}

func TestReviewIntegrationRealStoreRejectsStaleAndIntegratedReplayWithoutIO(t *testing.T) {
	store, snapshot, candidate, repo, root := realReviewerEvidenceFixture(t)
	countingRuntime := &countingReviewRuntime{}
	countingGit := &countingReviewGit{}
	service := NewReviewIntegrationService(store, countingRuntime, countingGit, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	stale := snapshot
	stale.Revision = 0
	if _, err := service.Advance(context.Background(), stale, "task-1", "stale"); err == nil || countingRuntime.calls != 0 || countingGit.calls != 0 {
		t.Fatalf("stale err=%v runtime=%d git=%d", err, countingRuntime.calls, countingGit.calls)
	}
	accepted, err := applyRealReviewerAccept(t, store, snapshot, candidate, repo, root, "integrated", "accept")
	if err != nil {
		t.Fatal(err)
	}
	// Persist integration through the real service and real reducer before the idempotency replay.
	realGit := worktree.New(runner.OSRunner{}, "git", root, repo)
	integration := NewReviewIntegrationService(store, &countingReviewRuntime{}, realGit, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	integrated, err := integration.Advance(context.Background(), accepted, "task-1", "integrated-run")
	if err != nil {
		t.Fatal(err)
	}
	if integrated.TaskStates["task-1"].Status != statev2.TaskIntegrated {
		t.Fatalf("status=%q", integrated.TaskStates["task-1"].Status)
	}
	beforeRuntime, beforeGit := countingRuntime.calls, countingGit.calls
	current, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Advance(context.Background(), current, "task-1", "replay"); err != nil {
		t.Fatal(err)
	}
	if countingRuntime.calls != beforeRuntime || countingGit.calls != beforeGit {
		t.Fatalf("integrated replay performed I/O runtime=%d/%d git=%d/%d", beforeRuntime, countingRuntime.calls, beforeGit, countingGit.calls)
	}
}

type realReviewerRuntime struct {
	candidateSHA       string
	launches, observes int
	decision           string
	active             bool
}

func (r *realReviewerRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity, runtimecontract.Invocation) (string, error) {
	r.launches++
	return "review-provider", nil
}
func (r *realReviewerRuntime) Observe(_ context.Context, invocation statev2.InvocationState, _ runtimecontract.Invocation) (coordinator.RuntimeObservation, error) {
	r.observes++
	decision := "accept"
	findings := []runtimecontract.ReviewerFinding{}
	if r.decision == "block" {
		decision = "block"
		findings = []runtimecontract.ReviewerFinding{{Code: "unsafe", Diagnostic: "unsafe change"}}
	}
	result, _ := json.Marshal(runtimecontract.ReviewerResult{ReviewedSHA: r.candidateSHA, Decision: decision, BlockingFindings: findings})
	at := time.Now().UTC()
	if r.active {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationActive, ProviderIdentity: "review-provider", Artifact: &runtimecontract.ArtifactEnvelope{RequestID: contractv2.RequestID(invocation.InvocationID), Role: runtimecontract.RoleReviewer, Status: "success", Result: result}}, nil
	}
	return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationEnded, ProviderIdentity: "review-provider", EndedAt: &at, Artifact: &runtimecontract.ArtifactEnvelope{RequestID: contractv2.RequestID(invocation.InvocationID), Role: runtimecontract.RoleReviewer, Status: "success", Result: result}}, nil
}
func (r *realReviewerRuntime) Terminate(context.Context, statev2.InvocationState) error { return nil }

func applyRealReviewerAccept(t *testing.T, store statev2.Store, snapshot statev2.WorkSnapshot, candidate, repo, root, owner, decision string) (statev2.WorkSnapshot, error) {
	t.Helper()
	git := worktree.New(runner.OSRunner{}, "git", root, repo)
	runtime := &realReviewerRuntime{candidateSHA: candidate, decision: decision}
	coord := coordinator.NewCoordinator(store, runtime, nil, git, coordinator.NewOwnerLocker(root), coordinator.OwnerID(owner), 0, time.Now().UTC())
	service := NewReviewIntegrationService(store, coord, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	first, err := service.Advance(context.Background(), snapshot, "task-1", contractv2.RequestID(owner+"-first"))
	if err != nil {
		return first, err
	}
	current, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		return current, err
	}
	second, err := service.Advance(context.Background(), current, "task-1", contractv2.RequestID(owner+"-second"))
	return second, err
}

type countingReviewRuntime struct{ calls int }

func (r *countingReviewRuntime) Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error) {
	r.calls++
	return coordinator.ReconcileResult{}, nil
}
func (r *countingReviewRuntime) SubmitRuntime(context.Context, contractv2.WorkID, contractv2.TaskID, statev2.InvocationID) <-chan coordinator.CommandResult {
	r.calls++
	return nil
}
func (r *countingReviewRuntime) Close(context.Context) error { r.calls++; return nil }

type countingReviewGit struct {
	calls, abort int
	mergeErr     error
}

func (g *countingReviewGit) CreateManagedWorktree(context.Context, string, string, string, string) error {
	g.calls++
	return nil
}
func (g *countingReviewGit) ReconcileIntegrationWorktree(context.Context, string, string, string) (bool, error) {
	g.calls++
	return false, nil
}
func (g *countingReviewGit) MergeCommitNoFF(context.Context, string, string) error {
	g.calls++
	return g.mergeErr
}
func (g *countingReviewGit) AbortMerge(context.Context, string) error {
	g.calls++
	g.abort++
	return nil
}
func (g *countingReviewGit) CurrentCommit(context.Context, string) (string, error) {
	g.calls++
	return strings.Repeat("3", 40), nil
}
func (g *countingReviewGit) IsAncestor(context.Context, string, string) (bool, error) {
	g.calls++
	return true, nil
}

func realReviewerEvidenceFixture(t *testing.T) (statev2.Store, statev2.WorkSnapshot, string, string, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGit(t, "", "init", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "ThreadDock Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	runGit(t, repo, "checkout", "-b", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "change.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "change.txt")
	runGit(t, repo, "commit", "-m", "candidate")
	candidate := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runGit(t, repo, "rev-parse", candidate+"^{tree}"))
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-review-evidence", ProjectID: "project", Revision: 1, Request: "review", AcceptanceCriteria: []string{"work acceptance"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", Branch: "candidate", AllowedPaths: []string{"change.txt"}, AcceptanceCriteria: []string{"task acceptance"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	if violations := contractv2.Validate(contract); len(violations) != 0 {
		t.Fatalf("contract violations: %#v", violations)
	}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(sum[:]), Contract: contract, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	store := statev2.NewStore(root)
	if _, err := store.CreatePlan(context.Background(), snapshot, "plan", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	snapshot = applyForegroundTransition(t, store, snapshot, "approve", &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash})
	wt := &statev2.WorktreeIdentity{CanonicalPath: repo, GitCommonDir: filepath.Join(repo, ".git"), Branch: "candidate", BaseSHA: base}
	snapshot = applyForegroundTransition(t, store, snapshot, "reserve", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "builder-inv", LogicalWorkID: "builder-logical", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: wt, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "builder-runtime"}})
	snapshot = applyForegroundTransition(t, store, snapshot, "begin", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "builder-inv", LogicalWorkID: "builder-logical", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC()})
	snapshot = applyForegroundTransition(t, store, snapshot, "running", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "builder-inv", LogicalWorkID: "builder-logical", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Invocation: &statev2.InvocationState{ProviderIdentity: "builder-provider"}})
	snapshot = applyForegroundTransition(t, store, snapshot, "terminated", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "builder-inv", LogicalWorkID: "builder-logical", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Reason: "finished"})
	snapshot = applyForegroundTransition(t, store, snapshot, "candidate", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRecordCandidate, InvocationID: "builder-inv", LogicalWorkID: "builder-logical", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Candidate: &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, TreeSHA: tree, ChangedFiles: []string{"change.txt"}}})
	snapshot = applyForegroundTransition(t, store, snapshot, "gate", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskRecordGate, Role: "builder", BuilderAttempt: 1, At: time.Now().UTC(), Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: time.Now().UTC()}})
	return store, snapshot, candidate, repo, root
}

func realReviewerGateFixture(t *testing.T) (statev2.Store, statev2.WorkSnapshot, string, string, string) {
	return realReviewerEvidenceFixture(t)
}

func realReviewerTerminatedFixture(t *testing.T) (statev2.Store, statev2.WorkSnapshot, string, string, string) {
	store, snapshot, candidate, repo, root := realReviewerEvidenceFixture(t)
	wt := snapshot.TaskStates["task-1"].Worktree
	snapshot = applyForegroundTransition(t, store, snapshot, "review-reserve", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: wt, Invocation: &statev2.InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime"}})
	snapshot = applyForegroundTransition(t, store, snapshot, "review-begin", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC()})
	snapshot = applyForegroundTransition(t, store, snapshot, "review-running", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Invocation: &statev2.InvocationState{ProviderIdentity: "review-provider"}})
	snapshot = applyForegroundTransition(t, store, snapshot, "review-terminated", &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, BuilderAttempt: 1, At: time.Now().UTC(), Reason: "finished"})
	return store, snapshot, candidate, repo, root
}

func TestReviewIntegrationLaunchesReviewerAndMergesExactCandidate(t *testing.T) {
	const candidate = "0123456789abcdef0123456789abcdef01234567"
	const tree = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	const base = "1111111111111111111111111111111111111111"
	snapshot := statev2.WorkSnapshot{WorkID: "work-1", Revision: 3, Contract: contractv2.WorkItemContract{
		WorkID: "work-1", AcceptanceCriteria: []string{"work"},
		RepositoryPlans:   []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base}},
		Tasks:             []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", AcceptanceCriteria: []string{"task"}}},
		ExecutionProfiles: contractv2.ExecutionProfiles{Reviewer: "reviewer"},
	}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	wt := &statev2.WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: base}
	snapshot.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskGatePassed, BuilderAttempt: 1, Worktree: wt, LogicalWork: &statev2.LogicalWorkState{LogicalWorkID: "builder", Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "builder-runtime", Worktree: wt}, Candidate: &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, TreeSHA: tree, ChangedFiles: []string{"internal/x.go"}}, Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: time.Now().UTC()}, InvocationHistory: []statev2.InvocationID{}}
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state}
	git := &reviewIntegrationGitFake{head: "2222222222222222222222222222222222222222"}
	service := NewReviewIntegrationService(state, runtime, git, "operator", func() time.Time { return time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC) }, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "caller-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskStates["task-1"].Status != statev2.TaskAccepted || runtime.activate != 1 || runtime.submit != 1 || git.mergedSHA != "" || git.create != 0 || git.abort != 0 {
		t.Fatalf("status=%q runtime=%d/%d git create/merge/abort=%d/%s/%d", got.TaskStates["task-1"].Status, runtime.activate, runtime.submit, git.create, git.mergedSHA, git.abort)
	}
	got, err = service.Advance(context.Background(), got, "task-1", "caller-1")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.activate != 1 || runtime.submit != 1 || git.create != 1 || git.mergedSHA != candidate {
		t.Fatalf("integrated replay mutated runtime/git: runtime=%d/%d git=%d/%s", runtime.activate, runtime.submit, git.create, git.mergedSHA)
	}
}

func TestReviewIntegrationBlockDoesNotMerge(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	task := snapshot.TaskStates["task-1"]
	task.Status, task.Review, task.Integration = statev2.TaskGatePassed, nil, nil
	snapshot.TaskStates["task-1"] = task
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state, decision: "block"}
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40)}
	service := NewReviewIntegrationService(state, runtime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "caller")
	if err != nil || got.TaskStates["task-1"].Status != statev2.TaskReviewBlocked || git.mergedSHA != "" {
		t.Fatalf("status=%q err=%v merge=%q", got.TaskStates["task-1"].Status, err, git.mergedSHA)
	}
}

func TestReviewIntegrationConflictAbortsAndPersistsBlocker(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	state := &reviewIntegrationState{snapshot: snapshot}
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40), mergeErr: worktree.ErrConflict}
	service := NewReviewIntegrationService(state, &reviewIntegrationRuntimeFake{state: state}, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "caller")
	if git.abort != 1 || got.TaskStates["task-1"].Status != statev2.TaskNeedsOperator || got.Control.Blocker == nil || got.TaskStates["task-1"].Integration != nil {
		t.Fatalf("status=%q err=%v abort=%d blocker=%#v", got.TaskStates["task-1"].Status, err, git.abort, got.Control.Blocker)
	}
}

func TestReviewIntegrationRejectsStaleSuppliedSnapshotBeforeRuntimeOrGit(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state}
	git := &reviewIntegrationGitFake{}
	service := NewReviewIntegrationService(state, runtime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	stale := snapshot
	stale.Revision = 0
	if _, err := service.Advance(context.Background(), stale, "task-1", "caller"); err == nil || runtime.activate != 0 || runtime.submit != 0 || git.create != 0 || git.mergedSHA != "" {
		t.Fatalf("stale advance err=%v runtime=%d/%d git=%d/%s", err, runtime.activate, runtime.submit, git.create, git.mergedSHA)
	}
}

func TestReviewIntegrationReplayUsesExistingReviewerInvocation(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	task := snapshot.TaskStates["task-1"]
	task.Status, task.Review, task.Integration = statev2.TaskInvocationReserved, nil, nil
	snapshot.TaskStates["task-1"] = task
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state}
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40)}
	service := NewReviewIntegrationService(state, runtime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "fresh-caller")
	if err != nil || runtime.lastInvocation != "review-inv" || state.reserves != 0 || got.TaskStates["task-1"].Status != statev2.TaskAccepted {
		t.Fatalf("status=%q err=%v invocation=%q reserves=%d", got.TaskStates["task-1"].Status, err, runtime.lastInvocation, state.reserves)
	}
}

func TestReviewIntegrationAcceptUsesRealManagedGitAndPersistsEvidence(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGit(t, "", "init", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "ThreadDock Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	runGit(t, repo, "checkout", "-b", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "change.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "change.txt")
	runGit(t, repo, "commit", "-m", "candidate")
	candidate := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	state := &reviewIntegrationState{snapshot: acceptedReviewSnapshot(base, candidate)}
	realGit := worktree.New(runner.OSRunner{}, "git", root, repo)
	service := NewReviewIntegrationService(state, &reviewIntegrationRuntimeFake{state: state}, realGit, "operator", func() time.Time { return time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC) }, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), state.snapshot, "task-1", "caller")
	if err != nil {
		t.Fatal(err)
	}
	task := got.TaskStates["task-1"]
	if task.Status != statev2.TaskIntegrated || task.Integration == nil || task.Integration.CandidateSHA != candidate || task.Integration.IntegrationHEAD == candidate || !task.Integration.RelationVerified {
		t.Fatalf("task=%#v", task)
	}
	if !strings.Contains(runGit(t, filepath.Join(root, "integration"), "log", "--format=%s"), "Merge") {
		t.Fatal("integration branch did not record a merge commit")
	}
}

func acceptedReviewSnapshot(base, candidate string) statev2.WorkSnapshot {
	wt := &statev2.WorktreeIdentity{CanonicalPath: "/candidate", GitCommonDir: "/repo/.git", Branch: "candidate", BaseSHA: base}
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	return statev2.WorkSnapshot{WorkID: "work-1", Revision: 1, Contract: contractv2.WorkItemContract{WorkID: "work-1", RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo"}}}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskAccepted, BuilderAttempt: 1, Worktree: wt, Invocation: &statev2.InvocationState{InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, TerminationConfirmed: true, EndedAt: &at}, Candidate: &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, TreeSHA: strings.Repeat("a", 40), ChangedFiles: []string{"change.txt"}}, Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: at}, Review: &statev2.ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidate, ReviewSHA: candidate, Accepted: true, Findings: []statev2.ReviewFinding{}, ObservedAt: at}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
}

type reviewIntegrationState struct {
	snapshot statev2.WorkSnapshot
	reserves int
}

func (s *reviewIntegrationState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}
func (s *reviewIntegrationState) Apply(_ context.Context, request statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	if request.Task == nil {
		return s.snapshot, errors.New("task transition required")
	}
	task := s.snapshot.TaskStates[request.Task.TaskID]
	switch request.Task.Action {
	case statev2.TaskReserveInvocation:
		s.reserves++
		task.Status = statev2.TaskInvocationReserved
		task.Invocation = &statev2.InvocationState{InvocationID: request.Task.InvocationID, LogicalWorkID: request.Task.LogicalWorkID, Role: "reviewer", ReturnStage: statev2.TaskGatePassed, LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime"}
		task.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: request.Task.LogicalWorkID, Role: "reviewer", BuilderAttempt: 1, LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime", Worktree: request.Task.Worktree}
		task.Worktree = request.Task.Worktree
	case statev2.TaskRecordIntegration:
		task.Status = statev2.TaskIntegrated
		task.Integration = request.Task.Integration
	case statev2.TaskNeedsOperatorAction:
		task.Status = statev2.TaskNeedsOperator
		s.snapshot.Control.Blocker = request.Task.Blocker
	default:
		return s.snapshot, errors.New("unexpected transition")
	}
	s.snapshot.TaskStates[request.Task.TaskID] = task
	s.snapshot.Revision++
	return s.snapshot, nil
}

type reviewIntegrationRuntimeFake struct {
	state            *reviewIntegrationState
	activate, submit int
	decision         string
	lastInvocation   statev2.InvocationID
}

func (r *reviewIntegrationRuntimeFake) Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error) {
	if invocation := r.state.snapshot.TaskStates["task-1"].Invocation; invocation != nil && invocation.Role != "reviewer" {
		return coordinator.ReconcileResult{}, errors.New("reviewer was reserved before activation")
	}
	r.activate++
	return coordinator.ReconcileResult{}, nil
}
func (r *reviewIntegrationRuntimeFake) SubmitRuntime(_ context.Context, _ contractv2.WorkID, taskID contractv2.TaskID, _ statev2.InvocationID) <-chan coordinator.CommandResult {
	r.submit++
	task := r.state.snapshot.TaskStates[taskID]
	r.lastInvocation = task.Invocation.InvocationID
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	task.Status = statev2.TaskAccepted
	if r.decision == "block" {
		task.Status = statev2.TaskReviewBlocked
	}
	task.Invocation.TerminationConfirmed = true
	task.Invocation.EndedAt = &at
	task.Review = &statev2.ReviewEvidence{ReviewerInvocationID: task.Invocation.InvocationID, BuilderAttempt: 1, CandidateSHA: task.Candidate.CandidateSHA, ReviewSHA: task.Candidate.CandidateSHA, Accepted: r.decision != "block", Findings: []statev2.ReviewFinding{}, ObservedAt: at}
	if r.decision == "block" {
		task.Review.Findings = []statev2.ReviewFinding{{Code: "blocked", Severity: "blocking", Diagnostic: "blocked"}}
	}
	r.state.snapshot.TaskStates[taskID] = task
	result := make(chan coordinator.CommandResult, 1)
	result <- coordinator.CommandResult{Snapshot: r.state.snapshot}
	return result
}
func (r *reviewIntegrationRuntimeFake) Close(context.Context) error { return nil }

type reviewIntegrationGitFake struct {
	create, abort   int
	mergeErr        error
	mergedSHA, head string
}

func (g *reviewIntegrationGitFake) CreateManagedWorktree(context.Context, string, string, string, string) error {
	g.create++
	return nil
}
func (g *reviewIntegrationGitFake) ReconcileIntegrationWorktree(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (g *reviewIntegrationGitFake) MergeCommitNoFF(_ context.Context, _ string, sha string) error {
	g.mergedSHA = sha
	return g.mergeErr
}
func (g *reviewIntegrationGitFake) AbortMerge(context.Context, string) error { g.abort++; return nil }
func (g *reviewIntegrationGitFake) CurrentCommit(context.Context, string) (string, error) {
	return g.head, nil
}
func (g *reviewIntegrationGitFake) IsAncestor(context.Context, string, string) (bool, error) {
	return true, nil
}
