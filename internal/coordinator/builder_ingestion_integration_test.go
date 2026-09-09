package coordinator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/runner"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

func TestBuilderIngestionUsesRealStoreAndGitEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runBuilderGit(t, "", "init", repo)
	runBuilderGit(t, repo, "config", "user.email", "test@example.com")
	runBuilderGit(t, repo, "config", "user.name", "ThreadDock Test")
	writeBuilderFile(t, filepath.Join(repo, "README.md"), "base\n")
	runBuilderGit(t, repo, "add", "README.md")
	runBuilderGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", "HEAD"))
	runBuilderGit(t, repo, "checkout", "-b", "agent/task-1")
	if err := os.Mkdir(filepath.Join(repo, "internal"), 0700); err != nil {
		t.Fatal(err)
	}
	writeBuilderFile(t, filepath.Join(repo, "internal", "builder.go"), "package internal\n")
	runBuilderGit(t, repo, "add", "internal/builder.go")
	runBuilderGit(t, repo, "commit", "-m", "builder")
	candidate := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", "HEAD"))
	common := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(common) {
		common = filepath.Join(repo, common)
	}

	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-builder", ProjectID: "project-builder", Revision: 1, Request: "build", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", Branch: "agent/task-1", AllowedPaths: []string{"internal/**"}, AcceptanceCriteria: []string{"done"}, Verification: []contractv2.CommandSpec{{Argv: []string{"go", "test"}, CwdRepoKey: "app", TimeoutSeconds: 120}}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	err := contractv2.Write(&encoded, contract)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(sum[:]), Contract: contract, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	store := statev2.NewStore(root)
	if _, err := store.CreatePlan(ctx, snapshot, "plan", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	approved := applyBuilderWork(t, store, snapshot, &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}, "approve")
	worktreeIdentity := &statev2.WorktreeIdentity{CanonicalPath: repo, GitCommonDir: common, Branch: "agent/task-1", BaseSHA: base}
	reserved := applyBuilderTask(t, store, approved, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: worktreeIdentity, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}, "reserve")
	running := applyBuilderTask(t, store, reserved, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC()}, "begin")
	started := time.Now().UTC()
	running = applyBuilderTask(t, store, running, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: started, Invocation: &statev2.InvocationState{ProviderIdentity: "provider-1"}}, "running")
	terminated := applyBuilderTask(t, store, running, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: started.Add(time.Second), Reason: "finished"}, "terminated")

	git := worktree.New(runner.OSRunner{}, "git", root, repo)
	d := &publicationDispatcher{state: store, inspector: git, ownerID: "owner", pid: os.Getpid(), startedAt: time.Now().UTC()}
	resultJSON := []byte(`{"commitSha":"` + candidate + `","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}`)
	artifact := &runtimecontract.ArtifactEnvelope{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, Status: "success", Result: resultJSON}
	result, err := d.ingestBuilderArtifact(ctx, terminated, "task-1", artifact)
	if err != nil {
		t.Fatal(err)
	}
	got := result.Snapshot.TaskStates["task-1"]
	if got.Status != statev2.TaskCandidateReady || got.Candidate == nil || got.Candidate.CandidateSHA != candidate {
		t.Fatalf("task=%#v", got)
	}
	wantTree := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", candidate+"^{tree}"))
	if got.Candidate.TreeSHA != wantTree || len(got.Candidate.ChangedFiles) != 1 || got.Candidate.ChangedFiles[0] != "internal/builder.go" {
		t.Fatalf("candidate=%#v wantTree=%q", got.Candidate, wantTree)
	}
	beforeRevision := result.Snapshot.Revision
	duplicate, err := d.ingestBuilderArtifact(ctx, result.Snapshot, "task-1", artifact)
	if err != nil || duplicate.Snapshot.Revision != beforeRevision || duplicate.Snapshot.TaskStates["task-1"].Candidate.CandidateSHA != candidate {
		t.Fatalf("duplicate result=%#v err=%v", duplicate, err)
	}
}

func TestCoordinatorReconcileRecoversConfirmedBuilderArtifactAfterCrash(t *testing.T) {
	store, git, snapshot, candidate, root := newConfirmedBuilderFixture(t)
	rt := &recoveryArtifactRuntime{artifact: &runtimecontract.ArtifactEnvelope{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, Status: "success", Result: []byte(`{"commitSha":"` + candidate + `","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}`)}}
	c := NewCoordinator(store, rt, nil, git, NewOwnerLocker(root), "owner-recovery", os.Getpid(), time.Now().UTC())
	if _, err := c.Reconcile(context.Background(), snapshot.WorkID); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.TaskStates["task-1"].Status; got != statev2.TaskCandidateReady {
		t.Fatalf("status=%q, want candidate_ready", got)
	}
	if rt.observes != 1 {
		t.Fatalf("observe calls=%d, want 1", rt.observes)
	}
}

func TestCoordinatorQueueSettlesEndedBuilderArtifactWithRealStoreAndGit(t *testing.T) {
	_, git, terminated, candidate, _ := newConfirmedBuilderFixture(t)
	worktreeIdentity := *terminated.TaskStates["task-1"].Worktree
	initial := terminated
	initial.Revision = 1
	initial.State = statev2.StateAwaitingApproval
	initial.NextAction = "approve"
	initial.Control = statev2.WorkControl{}
	initial.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}
	queueRoot := t.TempDir()
	queueStore := statev2.NewStore(queueRoot)
	if _, err := queueStore.CreatePlan(context.Background(), initial, "queue-plan", "queue-plan-hash"); err != nil {
		t.Fatal(err)
	}
	rt := &recoveryArtifactRuntime{artifact: &runtimecontract.ArtifactEnvelope{RequestID: "queue-inv-1", Role: runtimecontract.RoleBuilder, Status: "success", Result: []byte(`{"commitSha":"` + candidate + `","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}`)}}
	c := NewCoordinator(queueStore, rt, nil, git, NewOwnerLocker(queueRoot), "owner-queue", os.Getpid(), time.Now().UTC())
	if _, err := c.Activate(context.Background(), initial.WorkID); err != nil {
		t.Fatal(err)
	}
	approved := applyBuilderWork(t, queueStore, initial, &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: initial.ContractHash}, "queue-approve")
	reserved := applyBuilderTask(t, queueStore, approved, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "queue-inv-1", LogicalWorkID: "queue-logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: &worktreeIdentity, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}, "queue-reserve")
	applyBuilderTask(t, queueStore, reserved, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "queue-inv-1", LogicalWorkID: "queue-logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC()}, "queue-begin")
	if result := <-c.SubmitRuntime(context.Background(), initial.WorkID, "task-1", "queue-inv-1"); result.Err != nil {
		t.Fatal(result.Err)
	}
	queueAfter, err := queueStore.Load(context.Background(), initial.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if queueAfter.TaskStates["task-1"].Status != statev2.TaskCandidateReady || queueAfter.TaskStates["task-1"].Candidate == nil {
		t.Fatalf("queue task=%#v", queueAfter.TaskStates["task-1"])
	}
	_ = c.Close(context.Background())
}

type recoveryArtifactRuntime struct {
	artifact *runtimecontract.ArtifactEnvelope
	observes int
}

func (r *recoveryArtifactRuntime) Observe(context.Context, statev2.InvocationState, runtimecontract.Invocation) (RuntimeObservation, error) {
	r.observes++
	return RuntimeObservation{State: RuntimeObservationEnded, ProviderIdentity: "provider-1", EndedAt: ptrTime(time.Now().UTC()), Artifact: r.artifact}, nil
}
func (r *recoveryArtifactRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity, runtimecontract.Invocation) (string, error) {
	return "provider-1", nil
}
func (r *recoveryArtifactRuntime) Terminate(context.Context, statev2.InvocationState) error {
	return nil
}

func newConfirmedBuilderFixture(t *testing.T) (statev2.Store, *worktree.Git, statev2.WorkSnapshot, string, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runBuilderGit(t, "", "init", repo)
	runBuilderGit(t, repo, "config", "user.email", "test@example.com")
	runBuilderGit(t, repo, "config", "user.name", "ThreadDock Test")
	writeBuilderFile(t, filepath.Join(repo, "README.md"), "base\n")
	runBuilderGit(t, repo, "add", "README.md")
	runBuilderGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", "HEAD"))
	runBuilderGit(t, repo, "checkout", "-b", "agent/task-1")
	if err := os.Mkdir(filepath.Join(repo, "internal"), 0700); err != nil {
		t.Fatal(err)
	}
	writeBuilderFile(t, filepath.Join(repo, "internal", "builder.go"), "package internal\n")
	runBuilderGit(t, repo, "add", "internal/builder.go")
	runBuilderGit(t, repo, "commit", "-m", "builder")
	candidate := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", "HEAD"))
	common := strings.TrimSpace(runBuilderGit(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(common) {
		common = filepath.Join(repo, common)
	}
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-recovery", ProjectID: "project-recovery", Revision: 1, Request: "build", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", Branch: "agent/task-1", AllowedPaths: []string{"internal/**"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(sum[:]), Contract: contract, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	store := statev2.NewStore(root)
	if _, err := store.CreatePlan(ctx, snapshot, "plan", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	approved := applyBuilderWork(t, store, snapshot, &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}, "approve")
	wt := &statev2.WorktreeIdentity{CanonicalPath: repo, GitCommonDir: common, Branch: "agent/task-1", BaseSHA: base}
	reserved := applyBuilderTask(t, store, approved, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC(), Worktree: wt, Invocation: &statev2.InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}}, "reserve")
	running := applyBuilderTask(t, store, reserved, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Now().UTC()}, "begin")
	started := time.Now().UTC()
	running = applyBuilderTask(t, store, running, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: started, Invocation: &statev2.InvocationState{ProviderIdentity: "provider-1"}}, "running")
	terminated := applyBuilderTask(t, store, running, &statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: started.Add(time.Second), Reason: "finished"}, "terminated")
	return store, worktree.New(runner.OSRunner{}, "git", root, repo), terminated, candidate, root
}

func ptrTime(at time.Time) *time.Time { return &at }

func applyBuilderWork(t *testing.T, store statev2.Store, snapshot statev2.WorkSnapshot, transition *statev2.WorkTransition, id contractv2.RequestID) statev2.WorkSnapshot {
	t.Helper()
	request := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: id, Work: transition}
	var err error
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func applyBuilderTask(t *testing.T, store statev2.Store, snapshot statev2.WorkSnapshot, transition *statev2.TaskTransition, id contractv2.RequestID) statev2.WorkSnapshot {
	t.Helper()
	request := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: id, Task: transition}
	var err error
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func runBuilderGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	result, err := (runner.OSRunner{}).Run(context.Background(), cwd, "git", args...)
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, result.Stderr)
	}
	return result.Stdout
}

func writeBuilderFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}
