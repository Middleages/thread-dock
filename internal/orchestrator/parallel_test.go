package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
	"thread-dock/internal/testfixture"
)

func TestNewParallelUsesParallelStrategy(t *testing.T) {
	h := newHarness(t)
	h.orchestrator = NewParallel(h.Deps)
	if _, err := h.orchestrator.Start(context.Background(), h.contractPath); err != nil {
		t.Fatal(err)
	}
	runs, err := h.store.ListRecoverable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Strategy != "parallel" {
		t.Fatalf("snapshot strategy = %q, want parallel", runs[0].Strategy)
	}
}

func TestParallelStartPinsOpenCodeAgentSnapshot(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.BuilderOpenCodeAgent = "threaddock-builder"
	h.Deps.ReviewerOpenCodeAgent = "threaddock-reviewer"
	h.orchestrator = NewParallel(h.Deps)

	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	if snapshot.Builder.OpenCodeAgent != "threaddock-builder" || snapshot.Reviewer.OpenCodeAgent != "threaddock-reviewer" {
		t.Fatalf("legacy routing = builder %q reviewer %q", snapshot.Builder.OpenCodeAgent, snapshot.Reviewer.OpenCodeAgent)
	}
	for _, taskID := range snapshot.TaskOrder {
		if got := snapshot.Tasks[taskID].Agent.OpenCodeAgent; got != "threaddock-builder" {
			t.Fatalf("task %s routing = %q, want builder routing", taskID, got)
		}
	}
}

func TestParallelStartUsesPinnedOpenCodeAgentForBuilderAndReviewer(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.BuilderOpenCodeAgent = "threaddock-builder"
	h.Deps.ReviewerOpenCodeAgent = "threaddock-reviewer"
	h.orchestrator = NewParallel(h.Deps)

	if got := h.runToStable(); got != contract.PhaseCompleted {
		t.Fatalf("phase = %s", got)
	}
	if len(h.parallelHD.starts) != 3 {
		t.Fatalf("starts = %#v, want two builders and one reviewer", h.parallelHD.starts)
	}
	for _, request := range h.parallelHD.starts {
		want := "threaddock-builder"
		if strings.HasPrefix(request.Name, "reviewer-") {
			want = "threaddock-reviewer"
		}
		if request.OpenCodeAgent != want {
			t.Fatalf("start request %#v has routing %q, want %q", request, request.OpenCodeAgent, want)
		}
	}
}

func TestTaskAgentNameIsStableDistinctAndHerdrCompatible(t *testing.T) {
	run := contract.RunID("run-1787925632789000929")
	one := taskAgentName("builder", run, "api")
	two := taskAgentName("builder", run, "tests")
	if one == two || one != taskAgentName("builder", run, "api") {
		t.Fatalf("task names are not stable/distinct: %q %q", one, two)
	}
	if len(one) > maxHerdrAgentNameLength || len(two) > maxHerdrAgentNameLength || !validHerdrAgentName(one) || !validHerdrAgentName(two) {
		t.Fatalf("task names are not Herdr-compatible: %q %q", one, two)
	}
}

func TestParallelStories(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*parallelHarness)
		want      contract.RunPhase
	}{
		{name: "ordinary auto merge", want: contract.PhaseCompleted},
		{name: "third repair blocks", configure: func(h *parallelHarness) { h.reviewFailures = 1; h.ciFailures = 2 }, want: contract.PhaseBlocked},
		{name: "protected waits", configure: func(h *parallelHarness) { h.protectedRiskCategories = []string{"authentication"} }, want: contract.PhaseNeedsOperator},
		{name: "git conflict blocks", configure: func(h *parallelHarness) { h.mergeConflict = true }, want: contract.PhaseBlocked},
		{name: "three recoveries block", configure: func(h *parallelHarness) { h.stallRecoveries = 4; h.stallAll = true }, want: contract.PhaseBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newParallelHarness(t)
			if tc.configure != nil {
				tc.configure(h)
			}
			if got := h.runToStable(); got != tc.want {
				t.Fatalf("phase=%s, want %s", got, tc.want)
			}
			if tc.name == "protected waits" && len(h.parallelGH.protectedComments) != 1 {
				t.Fatalf("protected comments=%d, want one", len(h.parallelGH.protectedComments))
			}
			if tc.name == "protected waits" && !strings.Contains(h.parallelGH.protectedComments[0], "<!-- threaddock:") {
				t.Fatalf("protected comment missing unique marker: %q", h.parallelGH.protectedComments[0])
			}
		})
	}
}

func TestParallelDispatchesIndependentAgentsBeforeEitherCompletes(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30 && len(h.parallelHD.starts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.parallelHD.starts) != 2 {
		t.Fatalf("starts=%d, want two independent agents", len(h.parallelHD.starts))
	}
	if h.parallelHD.evidenceReads != 0 {
		t.Fatalf("evidence read before both agents dispatched: %d", h.parallelHD.evidenceReads)
	}
}

func TestRepairsRequireFreshCommitAndReuseOnePR(t *testing.T) {
	h := newParallelHarness(t)
	h.reviewFailures = 1
	h.ciFailures = 1
	if got := h.runToStable(); got != contract.PhaseCompleted {
		t.Fatalf("phase=%s", got)
	}
	if h.parallelGH.draftCalls != 1 {
		t.Fatalf("draft PR creates=%d, want one reused PR", h.parallelGH.draftCalls)
	}
	if h.parallelGH.readyCalls != 1 {
		t.Fatalf("ready mutations=%d, want one (reused ready PR is skipped)", h.parallelGH.readyCalls)
	}
	snapshot := h.mustLoadRun()
	if snapshot.PullRequestNodeID != "PR_node" {
		t.Fatalf("pull request node ID=%q, want durable PR_node", snapshot.PullRequestNodeID)
	}
	for _, task := range snapshot.Tasks {
		if task.Agent.CommitSHA == task.PreviousCommitSHA && task.PreviousCommitSHA != "" {
			t.Fatalf("repair reused stale commit %s", task.Agent.CommitSHA)
		}
	}
}

func TestWorkingAgentWaitsThenStaleLiveRequiresOperator(t *testing.T) {
	h := newParallelHarness(t)
	h.stallRecoveries = 1
	// This story exercises live-agent expiry; use the baseline Git fake so its
	// recovery budget is not affected by the independent fingerprint story.
	h.Deps.Worktree, h.Deps.Git = h.parallelGit, h.parallelGit
	h.Deps.WorkingWait = time.Hour
	h.orchestrator = NewParallel(h.Deps)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		snapshot := h.mustLoad(id)
		if snapshot.Summary == "live Agent progress wait: Builder structured evidence is stale or incomplete" {
			if snapshot.RecoveryCount != 0 || snapshot.Phase != contract.PhaseBuilding {
				t.Fatalf("wait snapshot=%+v", snapshot)
			}
			h.parallelHD.harness.stallMode = false
			h.stallRecoveries = 1
			h.parallelHD.forceWorking = true
			break
		}
	}
	h.clock.now = h.clock.now.Add(2 * time.Hour)
	for i := 0; i < 6; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		if h.mustLoad(id).Phase == contract.PhaseNeedsOperator {
			return
		}
	}
	t.Fatal("stale live working agent did not require operator")
}

func TestUnknownMergeabilityIsRereadAndBounded(t *testing.T) {
	h := newParallelHarness(t)
	h.parallelGH.mergeableUnknown = 2
	if got := h.runToStable(); got != contract.PhaseCompleted {
		t.Fatalf("phase=%s", got)
	}
	h = newParallelHarness(t)
	h.parallelGH.mergeableUnknown = 3
	if got := h.runToStable(); got != contract.PhaseBlocked {
		t.Fatalf("unknown mergeability phase=%s", got)
	}
}

func TestProtectedConfirmationIsIdempotentAndResumesMerge(t *testing.T) {
	h := newParallelHarness(t)
	h.protectedRiskCategories = []string{"authentication"}
	if got := h.runToStable(); got != contract.PhaseNeedsOperator {
		t.Fatalf("phase=%s", got)
	}
	runs, err := h.store.ListRecoverable(context.Background())
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	id := runs[0].RunID
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		snapshot := h.mustLoad(id)
		if snapshot.Phase == contract.PhaseCompleted {
			break
		}
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.mustLoad(id); got.Phase != contract.PhaseCompleted {
		t.Fatalf("phase after confirmation=%s", got.Phase)
	}
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatalf("idempotent terminal confirmation: %v", err)
	}
}

func TestProtectedConfirmationRestartsLatestMainBeforeMerge(t *testing.T) {
	h := newParallelHarness(t)
	h.protectedRiskCategories = []string{"authentication"}
	if got := h.runToStable(); got != contract.PhaseNeedsOperator {
		t.Fatalf("phase=%s", got)
	}
	var id contract.RunID
	for runID := range h.orchestrator.runs {
		id = runID
	}
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(id); got.Phase != contract.PhaseCI {
		t.Fatalf("phase after first confirmation=%s, want CI revalidation", got.Phase)
	}
	merges := h.parallelGH.mergeCalls
	if err := h.orchestrator.ConfirmProtectedChange(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.parallelGH.mergeCalls != merges {
		t.Fatalf("repeat confirmation triggered merge: before=%d after=%d", merges, h.parallelGH.mergeCalls)
	}
}

func TestParallelTerminalIdentityNeverEnablesNativeResume(t *testing.T) {
	h := newParallelHarness(t)
	h.herdr.useTerminalIdentity = true
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := h.mustLoad(id)
	for _, task := range snapshot.Tasks {
		if task.Agent.IdentitySource == "terminal" && task.NativeResume {
			t.Fatal("terminal identity enabled native resume")
		}
	}
}

func TestRecoveryFingerprintIsStableIndependentOfCompletionOrder(t *testing.T) {
	checks := []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}}
	left := RecoveryFingerprint(validSHA, "managed", []string{"tests", "api"}, checks)
	right := RecoveryFingerprint(validSHA, "managed", []string{"api", "tests"}, checks)
	if left == "" || left != right {
		t.Fatalf("fingerprint mismatch: %q %q", left, right)
	}
}

func TestProjectAutomationDisabledDoesNotCallProjectPort(t *testing.T) {
	h := newParallelHarness(t)
	if got := h.runToStable(); got != contract.PhaseCompleted {
		t.Fatalf("phase=%s", got)
	}
	if len(h.parallelGH.projectStatuses) != 0 {
		t.Fatalf("project calls=%d, want disabled", len(h.parallelGH.projectStatuses))
	}
	var id contract.RunID
	for runID := range h.orchestrator.runs {
		id = runID
	}
	if id == "" {
		t.Fatal("parallel run ID not retained")
	}
	if !hasEventMessage(h.events(id), "Project automation disabled") {
		t.Fatal("missing disabled project audit")
	}
}

func TestProjectAutomationEnabledUsesProjectPort(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.ProjectAutomationEnabled = true
	h.Deps.Project = github.ProjectRef{ID: "PVT_1", StatusFieldID: "F_1", StatusOptions: map[string]string{"Ready": "ready", "In Progress": "progress", "Review": "review", "Done": "done"}}
	h.orchestrator = NewParallel(h.Deps)
	if got := h.runToStable(); got != contract.PhaseCompleted {
		t.Fatalf("phase=%s", got)
	}
	if h.parallelGH.projectItemID == "" || h.parallelGH.projectStatus != "Done" {
		t.Fatal("project automation did not call project port")
	}
}

func TestParallelPendingDraftPRReconcilesWithoutDuplicateCreate(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseReviewing
	snapshot.ProjectStatus = "Review"
	snapshot.ActionCursor = 6
	snapshot.Integration.Branch = "agent/parent-integration"
	snapshot.IntegrationSHA = validSHA
	snapshot.ParentIssue = 184
	snapshot.PendingAction = "parallel_create_draft_pr"
	h.parallelGH.pr = github.PullRequest{Number: 185, HTMLURL: "https://github.example/185", State: "open", Draft: true, Head: snapshot.Integration.Branch, HeadSHA: validSHA, Base: "main"}
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "" || got.PullRequest != 185 || h.parallelGH.draftCalls != 0 {
		t.Fatalf("snapshot=%+v draftCalls=%d", got, h.parallelGH.draftCalls)
	}
}

func TestMainMergeResponseLossReconcilesMergedFlagAndSHA(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase, snapshot.ProjectStatus, snapshot.PendingAction = contract.PhaseMerging, "Review", "parallel_merge_main"
	snapshot.PullRequest, snapshot.PullRequestHeadSHA, snapshot.FinalSHA = 185, validSHA, validSHA
	mergeSHA := "2222222222222222222222222222222222222222"
	h.parallelGH.pr = github.PullRequest{Number: 185, State: "closed", Merged: true, Head: "agent/parent-integration", HeadSHA: validSHA, Base: "main", MergeCommitSHA: mergeSHA}
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseCompleted || got.MergeSHA != mergeSHA || !got.PullRequestMerged {
		t.Fatalf("reconciled snapshot=%+v", got)
	}
	if h.parallelGH.mergeCalls != 0 {
		t.Fatalf("merge duplicated after response loss: %d", h.parallelGH.mergeCalls)
	}
}

func TestParallelReviewerPacketUsesStrictEnvelopeAndAllTaskEvidence(t *testing.T) {
	snapshot := state.RunSnapshot{
		RunID: "run-184", IntegrationSHA: validSHA,
		Integration:    state.WorktreeState{Branch: "agent/parent-integration"},
		ReviewerPrompt: state.PromptReceipt{RequestID: "run-184:reviewer-prompt"},
		TaskOrder:      []string{"api", "tests"},
		Tasks: map[string]state.TaskRunState{
			"api":   {State: "completed", Agent: state.AgentEvidence{CommitSHA: validSHA, Patch: "api patch", VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1s"}}}},
			"tests": {State: "completed", Agent: state.AgentEvidence{CommitSHA: validSHA, Patch: "tests patch", VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./tests/payments", Outcome: "passed", Duration: "1s"}}}},
		},
		IntegrationVerification: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}},
	}
	runtime := &runRuntime{contract: testfixture.ValidContract()}
	runtime.contract.Parent.AcceptanceCriteria = []string{"parent criterion"}
	packet := parallelReviewerPacket(runtime.contract, &snapshot)
	for _, want := range []string{herdr.THREADDOCK_REVIEW_BEGIN, herdr.THREADDOCK_REVIEW_END, "requestId=run-184:reviewer-prompt", "parent criterion", "api patch", "tests patch", "go test ./...", herdr.ReviewEvidenceSchemaExample} {
		if !strings.Contains(packet, want) {
			t.Fatalf("review packet missing %q:\n%s", want, packet)
		}
	}
}
