package workrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func TestForegroundIdentifiersAreStableAndBounded(t *testing.T) {
	firstInvocation, firstLogical := foregroundIDs("work-1", "request-1")
	secondInvocation, secondLogical := foregroundIDs("work-1", "request-1")
	if firstInvocation != secondInvocation || firstLogical != secondLogical {
		t.Fatalf("foreground IDs are not deterministic: %q/%q vs %q/%q", firstInvocation, firstLogical, secondInvocation, secondLogical)
	}
	if len(firstInvocation) != 32 || len(firstLogical) != 32 || !isLowerHex(string(firstInvocation)) || !isLowerHex(string(firstLogical)) {
		t.Fatalf("foreground IDs are not bounded lowercase hex: %q/%q", firstInvocation, firstLogical)
	}
	if got := foregroundWorktreeID("work/with/secrets", "task/with/secrets"); len(got) != 32 || !isLowerHex(got) {
		t.Fatalf("worktree ID=%q", got)
	}
}

type scriptedForegroundRuntime struct {
	launches, observes int
	endedCommit        string
}

func (r *scriptedForegroundRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity, runtimecontract.Invocation) (string, error) {
	r.launches++
	return "provider-foreground", nil
}
func (r *scriptedForegroundRuntime) Observe(_ context.Context, _ statev2.InvocationState, invocation runtimecontract.Invocation) (coordinator.RuntimeObservation, error) {
	r.observes++
	if r.endedCommit != "" {
		result, err := json.Marshal(runtimecontract.BuilderResult{CommitSHA: r.endedCommit, Verification: []runtimecontract.VerificationResult{{Command: "scripted-check", Outcome: "passed", Duration: "1ms"}}})
		if err != nil {
			return coordinator.RuntimeObservation{}, err
		}
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationEnded, ProviderIdentity: "provider-foreground", Artifact: &runtimecontract.ArtifactEnvelope{RequestID: invocation.RequestID, Role: runtimecontract.RoleBuilder, Status: "success", Result: result}}, nil
	}
	return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationActive, ProviderIdentity: "provider-foreground"}, nil
}
func (r *scriptedForegroundRuntime) Terminate(context.Context, statev2.InvocationState) error {
	return nil
}

func TestForegroundRunRealStoreGitAndCoordinatorReplaysWithoutLaunch(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init")
	runGit(t, repository, "config", "user.email", "test@example.com")
	runGit(t, repository, "config", "user.name", "ThreadDock Test")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README.md")
	runGit(t, repository, "commit", "-m", "base")
	base := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-real", ProjectID: "project-real", Revision: 1, Request: "run", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-real", RepoKey: "repo", Branch: "agent/task-real", AllowedPaths: []string{"internal/**"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: contract, ContractHash: hex.EncodeToString(sum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-real": {TaskID: "task-real", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	store := statev2.NewStore(filepath.Join(root, "state"))
	if _, err := store.CreatePlan(ctx, snapshot, "plan-real", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	approved, err := store.Apply(ctx, transitionFor(contract.WorkID, 1, "approve-real", &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}))
	if err != nil {
		t.Fatal(err)
	}
	if approved.Revision != 2 {
		t.Fatalf("approved revision=%d", approved.Revision)
	}
	managed := filepath.Join(root, "worktrees")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}
	git := worktree.New(runner.OSRunner{}, "git", managed, repository)
	rt := &scriptedForegroundRuntime{}
	coord := coordinator.NewCoordinator(store, rt, nil, git, coordinator.NewOwnerLocker(filepath.Join(root, "state")), "foreground-test", 0, timeNowUTC())
	prep := NewPreparationService(store, git, "foreground-test", nil)
	service := NewForegroundService(store, prep, git, coord, foregroundCandidateVerifierFake{}, ForegroundBinding{RepositoryPath: repository, WorktreeRoot: managed, RuntimeFingerprint: "runtime-1"})
	first, err := service.RunWork(ctx, contract.WorkID, approved.Revision, "request-real")
	if err != nil || first.TaskStates["task-real"].Status != statev2.TaskRunning || rt.launches != 1 {
		t.Fatalf("first status=%q launch=%d observe=%d err=%v", first.TaskStates["task-real"].Status, rt.launches, rt.observes, err)
	}
	second, err := service.RunWork(ctx, contract.WorkID, 999, "request-real")
	if err != nil || second.TaskStates["task-real"].Status != statev2.TaskRunning || rt.launches != 1 {
		t.Fatalf("replay status=%q launch=%d observe=%d err=%v", second.TaskStates["task-real"].Status, rt.launches, rt.observes, err)
	}
	worktreePath := first.TaskStates["task-real"].Worktree.CanonicalPath
	if err := os.Mkdir(filepath.Join(worktreePath, "internal"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "internal", "foreground.go"), []byte("package internal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "add", "internal/foreground.go")
	runGit(t, worktreePath, "commit", "-m", "scripted candidate")
	candidate := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD"))
	if inspection, inspectErr := git.InspectCommit(ctx, worktreePath, base, "agent/task-real", candidate); inspectErr != nil {
		t.Fatalf("pre-reconcile candidate inspect err=%v inspection=%#v", inspectErr, inspection)
	}
	rt.endedCommit = candidate
	separate := coordinator.NewCoordinator(store, rt, nil, git, coordinator.NewOwnerLocker(filepath.Join(root, "state")), "reconcile-test", 0, timeNowUTC())
	if _, err := separate.Reconcile(ctx, contract.WorkID); err != nil {
		failed, loadErr := store.Load(ctx, contract.WorkID)
		t.Fatalf("scripted reconcile err=%v loadErr=%v task=%#v", err, loadErr, failed.TaskStates["task-real"])
	}
	final, err := store.Load(ctx, contract.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	task := final.TaskStates["task-real"]
	tree := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", candidate+"^{tree}"))
	if rt.launches != 1 || rt.observes != 1 || task.Status != statev2.TaskCandidateReady || task.Candidate == nil || task.Candidate.CandidateSHA != candidate || task.Candidate.TreeSHA != tree || !reflect.DeepEqual(task.Candidate.ChangedFiles, []string{"internal/foreground.go"}) || task.Gate != nil {
		t.Fatalf("reconcile launch/observe=%d/%d task=%#v tree=%q", rt.launches, rt.observes, task, tree)
	}
}

func transitionFor(workID contractv2.WorkID, revision contractv2.Revision, requestID contractv2.RequestID, work *statev2.WorkTransition) statev2.TransitionRequest {
	request := statev2.TransitionRequest{WorkID: workID, ExpectedRevision: revision, RequestID: requestID, Work: work}
	request.PayloadHash, _ = statev2.TransitionPayloadHash(request)
	return request
}

func runGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	result, err := (runner.OSRunner{}).Run(context.Background(), cwd, "git", args...)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("git %v exit=%d err=%v stderr=%q", args, result.ExitCode, err, result.Stderr)
	}
	return result.Stdout
}

func timeNowUTC() time.Time { return time.Now().UTC() }

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
