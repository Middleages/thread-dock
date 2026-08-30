package orchestrator

import (
	"bytes"
	"context"
	"os"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

func TestTerminalFallbackRequiresExactPrefix(t *testing.T) {
	for _, tc := range []struct {
		identity string
		want     bool
	}{
		{"herdr-terminal:abc", true},
		{"provider-terminal-session", false},
		{"xherdr-terminal:abc", false},
		{"Herdr-terminal:abc", false},
	} {
		if got := isTerminalIdentity(tc.identity); got != tc.want {
			t.Fatalf("identity=%q got=%v want=%v", tc.identity, got, tc.want)
		}
	}
}

func TestCIRepairAttributionRequiresUniqueRequiredCommand(t *testing.T) {
	c := contract.TaskContract{Tasks: []contract.Task{
		{ID: "api", Verification: []string{"go test ./internal/payments"}},
		{ID: "tests", Verification: []string{"go test ./tests/payments"}},
	}}
	if id, ok := uniqueCIRepairTask(c, github.CheckState{Name: "go test ./internal/payments", State: "failure"}); !ok || id != "api" {
		t.Fatalf("unique attribution=%q,%v", id, ok)
	}
	if id, ok := uniqueCIRepairTask(c, github.CheckState{Name: "ci", State: "failure"}); ok || id != "" {
		t.Fatalf("ambiguous attribution=%q,%v", id, ok)
	}
}

type fullSuiteFailureGit struct {
	*parallelGit
	checksCalls int
}

type nodeObservationGitHub struct {
	*parallelGitHub
	observations []github.PullRequest
}

func (g *nodeObservationGitHub) GetPullRequest(context.Context, github.Repository, int) (github.PullRequest, error) {
	if len(g.observations) == 0 {
		return g.parallelGitHub.pr, nil
	}
	pr := g.observations[0]
	g.observations = g.observations[1:]
	return pr, nil
}

func (g *nodeObservationGitHub) MarkReadyForReview(context.Context, github.Repository, string) (github.PullRequest, error) {
	g.readyCalls++
	pr := g.parallelGitHub.pr
	pr.Draft = false
	return pr, nil
}

func TestMissingPRNodeObservationKeepsReadyCursorForNextAdvance(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase, snapshot.ActionCursor = contract.PhaseCI, 5
	snapshot.PullRequest, snapshot.FinalSHA, snapshot.PullRequestHeadSHA = 185, validSHA, validSHA
	snapshot.PullRequestDraft = true
	prWithoutNode := github.PullRequest{Number: 185, State: "open", Draft: true, Head: snapshot.Integration.Branch, HeadSHA: validSHA, Base: snapshot.Repository.DefaultBranch}
	prWithNode := prWithoutNode
	prWithNode.NodeID = "PR_node"
	h.parallelGH.pr = prWithNode
	gh := &nodeObservationGitHub{parallelGitHub: h.parallelGH, observations: []github.PullRequest{prWithoutNode, prWithNode}}
	h.orchestrator.deps.GitHub = gh
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	runtime := h.orchestrator.runs[id]
	if err := h.orchestrator.advanceParallelCI(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if snapshot.PullRequestNodeID != "" || snapshot.ActionCursor != 5 || gh.readyCalls != 0 {
		t.Fatalf("missing node skipped ready: node=%q cursor=%d ready=%d", snapshot.PullRequestNodeID, snapshot.ActionCursor, gh.readyCalls)
	}
	if err := h.orchestrator.advanceParallelCI(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if snapshot.PullRequestNodeID != "PR_node" || snapshot.ActionCursor != 5 || gh.readyCalls != 0 {
		t.Fatalf("node observation did not preserve ready cursor: snapshot=%+v ready=%d", snapshot, gh.readyCalls)
	}
	if err := h.orchestrator.advanceParallelCI(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if gh.readyCalls != 1 || snapshot.PullRequestDraft || snapshot.ActionCursor != 6 {
		t.Fatalf("ready mutation count/state=%d/%+v", gh.readyCalls, snapshot)
	}
}

func TestPendingMissingPRNodeReconcilesThenInvokesReadyOnce(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase, snapshot.ActionCursor = contract.PhaseCI, 5
	snapshot.PullRequest, snapshot.FinalSHA, snapshot.PullRequestHeadSHA = 185, validSHA, validSHA
	snapshot.PullRequestDraft = true
	withoutNode := github.PullRequest{Number: 185, State: "open", Draft: true, HeadSHA: validSHA, Base: snapshot.Repository.DefaultBranch}
	withNode := withoutNode
	withNode.NodeID = "PR_node"
	h.parallelGH.pr = withNode
	gh := &nodeObservationGitHub{parallelGitHub: h.parallelGH, observations: []github.PullRequest{withoutNode, withNode}}
	h.orchestrator.deps.GitHub = gh
	snapshot.PendingAction = "parallel_read_pr_node"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	runtime := h.orchestrator.runs[id]
	if err := h.orchestrator.reconcileParallelPending(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if snapshot.PendingAction != "" || snapshot.ActionCursor != 5 || snapshot.PullRequestNodeID != "" {
		t.Fatalf("missing node pending reconcile=%+v", snapshot)
	}
	snapshot.PendingAction = "parallel_read_pr_node"
	if err := h.orchestrator.reconcileParallelPending(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if snapshot.PullRequestNodeID != "PR_node" || snapshot.ActionCursor != 5 || gh.readyCalls != 0 {
		t.Fatalf("pending node reconcile skipped cursor=%d node=%q ready=%d", snapshot.ActionCursor, snapshot.PullRequestNodeID, gh.readyCalls)
	}
	if err := h.orchestrator.advanceParallelCI(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if gh.readyCalls != 1 {
		t.Fatalf("pending node path ready calls=%d, want one", gh.readyCalls)
	}
}

func TestParallelRestartBlocksContractRepositoryMutationBeforeProviderCall(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase, snapshot.ActionCursor = contract.PhaseCI, 7
	snapshot.FinalSHA = validSHA
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	c, err := contract.Read(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	c.Repository.Owner, c.Repository.Name, c.Repository.DefaultBranch = "attacker", "other", "trunk"
	var encoded bytes.Buffer
	if err := contract.Write(&encoded, c); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.contractPath, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	before := h.parallelGH.checkCalls
	if err := h.orchestrator.Advance(context.Background(), id); err == nil {
		t.Fatal("mutated contract repository was accepted")
	}
	if h.parallelGH.checkCalls != before {
		t.Fatalf("provider call occurred after repository mutation: before=%d after=%d", before, h.parallelGH.checkCalls)
	}
}

func (g *fullSuiteFailureGit) RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error) {
	g.checksCalls++
	return []worktree.VerificationCheck{{Command: "go test ./internal/payments", Outcome: "failed", Duration: "1s", ExitCode: 1}}, nil
}

func TestFullSuitePersistsFinalSHABeforeFailureSchedulesRepair(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseCI
	snapshot.ActionCursor = 2
	snapshot.IntegrationSHA = validSHA
	snapshot.Integration.Path = "/tmp/integration"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	git := &fullSuiteFailureGit{parallelGit: h.parallelGit}
	h.orchestrator.deps.Worktree = git
	runtime := h.orchestrator.runs[id]
	if err := h.orchestrator.advanceParallelCI(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if snapshot.FinalSHA != validSHA || snapshot.ActionCursor != 3 || git.checksCalls != 0 {
		t.Fatalf("final SHA action snapshot=%+v checks=%d", snapshot, git.checksCalls)
	}
	if err := h.orchestrator.advanceParallelCI(context.Background(), &snapshot, runtime); err != nil {
		t.Fatal(err)
	}
	if git.checksCalls != 1 || snapshot.Phase != contract.PhaseBuilding || snapshot.RepairBaseSHA != validSHA {
		t.Fatalf("full suite failure did not schedule unique repair: checks=%d phase=%s repairBase=%q", git.checksCalls, snapshot.Phase, snapshot.RepairBaseSHA)
	}
}

type recordingFingerprintReader struct {
	*fakeGit
	path      string
	newPath   string
	oldCalled bool
}

func (r *recordingFingerprintReader) Fingerprint(_ context.Context, path string) (string, error) {
	r.oldCalled = true
	r.path = path
	return "managed", nil
}

func (r *recordingFingerprintReader) FingerprintWorktree(_ context.Context, path string) (string, error) {
	r.newPath = path
	return "herdr", nil
}

func TestRecoveryFingerprintReadsBuilderWorktree(t *testing.T) {
	reader := &recordingFingerprintReader{fakeGit: &fakeGit{}}
	o := &Orchestrator{deps: Dependencies{Worktree: reader}}
	_, err := o.builderManagedFingerprint(context.Background(), state.TaskRunState{Worktree: state.WorktreeState{Path: "/builder/worktree"}})
	if err != nil || reader.newPath != "/builder/worktree" || reader.oldCalled {
		t.Fatalf("newPath=%q oldPath=%q oldCalled=%v err=%v", reader.newPath, reader.path, reader.oldCalled, err)
	}
}

func TestRecoveryFingerprintProgressAcknowledgesCurrentValueAndReturnsToEvidence(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	task := snapshot.Tasks["api"]
	task.State, task.Stage = "running", "recovery_observe"
	task.Agent.CommitSHA = validSHA
	task.Agent.VerificationEvidence = []state.VerificationEvidence{{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1s"}}
	task.Worktree.Path = "/herdr/api"
	task.ProgressFingerprint = "old"
	task.PreviousFingerprint = "old"
	task.RecoveryCount = 2
	snapshot.Tasks["api"] = task
	reader := &recordingFingerprintReader{fakeGit: h.parallelGit.fakeGit}
	h.orchestrator.deps.Worktree = reader
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.parallelRecoveryFingerprint(context.Background(), &snapshot, "api", task); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id).Tasks["api"]
	if got.ProgressFingerprint == "old" || got.PreviousFingerprint != got.ProgressFingerprint || got.RecoveryCount != 0 || got.Stage != "evidence" || got.LastProgressAt.IsZero() {
		t.Fatalf("progress was not acknowledged: %+v", got)
	}
}

func TestCompleteStoryFingerprintChangeResetsRecoveryBudget(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	task := snapshot.Tasks["api"]
	task.State, task.Stage = "running", "recovery_observe"
	task.Agent.CommitSHA = validSHA
	task.Agent.VerificationEvidence = []state.VerificationEvidence{{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1s"}}
	task.Worktree.Path = "/herdr/api"
	oldFingerprint := RecoveryFingerprint(validSHA, "baseline", nil, task.Agent.VerificationEvidence)
	task.ProgressFingerprint = oldFingerprint
	task.PreviousFingerprint = task.ProgressFingerprint
	task.RecoveryCount = 3
	snapshot.Tasks["api"] = task
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	fingerprintGit := &fingerprintParallelGit{parallelGit: h.parallelGit, fingerprintBaseline: "baseline", fingerprintReads: 1, fingerprintChange: "changed"}
	h.orchestrator.deps.Worktree = fingerprintGit
	if err := h.orchestrator.parallelRecoveryFingerprint(context.Background(), &snapshot, "api", task); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id).Tasks["api"]
	if got.RecoveryCount != 0 || got.Stage != "evidence" || got.ProgressFingerprint == oldFingerprint || got.PreviousFingerprint != got.ProgressFingerprint {
		t.Fatalf("changed fingerprint did not reset recovery budget: %+v", got)
	}
}

func TestProtectedConfirmationInvalidatesEvidenceBeforeResume(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseNeedsOperator
	snapshot.ProtectedReasons = []string{"authentication"}
	snapshot.FinalSHA = validSHA
	snapshot.FinalChecksSHA = validSHA
	snapshot.FinalChecks = []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}}
	snapshot.MainSHA = validSHA
	snapshot.MergeabilityKnown, snapshot.Mergeable = true, true
	snapshot.ActionCursor = 9
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseCI || got.FinalSHA != "" || got.FinalChecksSHA != "" || got.MainSHA != "" || got.PullRequestHeadSHA != "" || got.MergeabilityKnown || got.MergePreflightReady || got.CIState != "" {
		t.Fatalf("confirmation did not invalidate evidence: %+v", got)
	}
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got2 := h.mustLoad(id); got2.UpdatedAt != got.UpdatedAt {
		t.Fatalf("repeated confirmation changed snapshot: before=%+v after=%+v", got, got2)
	}
}

func TestConflictAbortIsAttemptedBeforeBlocking(t *testing.T) {
	h := newParallelHarness(t)
	h.mergeConflict = true
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		if h.mustLoad(id).Phase == contract.PhaseBlocked {
			break
		}
	}
	if h.parallelGit.abortMerges == 0 {
		t.Fatal("confirmed conflict was blocked without merge abort")
	}
}
