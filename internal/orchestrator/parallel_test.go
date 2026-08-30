package orchestrator

import (
	"context"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/state"
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
		{name: "protected waits", configure: func(h *parallelHarness) { h.changedFiles = []string{"authentication/policy.go"} }, want: contract.PhaseNeedsOperator},
		{name: "git conflict blocks", configure: func(h *parallelHarness) { h.mergeConflict = true }, want: contract.PhaseBlocked},
		{name: "three recoveries block", configure: func(h *parallelHarness) { h.stallRecoveries = 3 }, want: contract.PhaseBlocked},
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
		})
	}
}

func TestProtectedConfirmationIsIdempotentAndResumesMerge(t *testing.T) {
	h := newParallelHarness(t)
	h.changedFiles = []string{"authentication/policy.go"}
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
	if len(h.parallelGH.projectStatuses) == 0 {
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
