package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/testfixture"
)

func TestSingleRunReachesReviewWithIndependentReviewerAndEvidence(t *testing.T) {
	h := newHarness(t)

	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	initial := h.mustLoad(id)
	if initial.Phase != contract.PhaseRegistered {
		t.Fatalf("initial phase = %s", initial.Phase)
	}

	for i := 0; i < 14 && len(h.herdr.prompts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatalf("advance %d: %v", i+1, err)
		}
	}

	got := h.mustLoad(id)
	if got.Phase != contract.PhaseReviewing {
		t.Fatalf("phase = %s, want reviewing", got.Phase)
	}
	if got.Builder.CommitSHA != validSHA || len(got.Builder.Verification) != 1 {
		t.Fatalf("builder evidence = %#v", got.Builder)
	}
	if got.Builder.Name == "" || got.Reviewer.Name == "" || got.Builder.Name == got.Reviewer.Name {
		t.Fatalf("agents = builder %#v reviewer %#v", got.Builder, got.Reviewer)
	}
	if len(h.herdr.prompts) != 2 {
		t.Fatalf("prompt count = %d, want builder and reviewer", len(h.herdr.prompts))
	}
	if !strings.Contains(strings.ToLower(h.herdr.prompts[1]), "review schema") || !strings.Contains(h.herdr.prompts[1], validSHA) {
		t.Fatalf("review packet = %q", h.herdr.prompts[1])
	}
	if strings.Contains(h.herdr.prompts[1], builderTranscriptSecret) {
		t.Fatal("reviewer received builder transcript")
	}
	if h.git.merges != 1 || h.git.mergedBranch == "main" {
		t.Fatalf("merges = %d branch = %q", h.git.merges, h.git.mergedBranch)
	}
}

func TestSingleRunPersistsActualAgentSessionIDsSeparatelyFromNames(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14 && len(h.herdr.prompts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	got := h.mustLoad(id)
	if got.Builder.Name != agentName("builder", id) || got.Reviewer.Name != agentName("reviewer", id) {
		t.Fatalf("agent names=%q/%q", got.Builder.Name, got.Reviewer.Name)
	}
	if got.Builder.SessionID != "session-"+agentName("builder", id) || got.Reviewer.SessionID != "session-"+agentName("reviewer", id) {
		t.Fatalf("agent sessions=%q/%q", got.Builder.SessionID, got.Reviewer.SessionID)
	}
	if got.Builder.SessionID == got.Builder.Name || got.Reviewer.SessionID == got.Reviewer.Name || got.Builder.SessionID == got.Reviewer.SessionID {
		t.Fatalf("session IDs were replaced by names or are not distinct: %#v", got)
	}
}

func TestSingleStartPinsOpenCodeAgentSnapshot(t *testing.T) {
	h := newHarness(t)
	h.Deps.BuilderOpenCodeAgent = "threaddock-builder"
	h.Deps.ReviewerOpenCodeAgent = "threaddock-reviewer"
	h.orchestrator = New(h.Deps)

	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	if snapshot.Builder.OpenCodeAgent != "threaddock-builder" || snapshot.Reviewer.OpenCodeAgent != "threaddock-reviewer" {
		t.Fatalf("initial routing = builder %q reviewer %q", snapshot.Builder.OpenCodeAgent, snapshot.Reviewer.OpenCodeAgent)
	}
}

func TestSingleStartUsesPinnedOpenCodeAgentForBuilderAndReviewer(t *testing.T) {
	h := newHarness(t)
	h.Deps.BuilderOpenCodeAgent = "threaddock-builder"
	h.Deps.ReviewerOpenCodeAgent = "threaddock-reviewer"
	h.orchestrator = New(h.Deps)

	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14 && len(h.herdr.starts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.herdr.starts) != 2 {
		t.Fatalf("starts = %#v, want builder and reviewer", h.herdr.starts)
	}
	if h.herdr.starts[0].OpenCodeAgent != "threaddock-builder" || h.herdr.starts[1].OpenCodeAgent != "threaddock-reviewer" {
		t.Fatalf("start routing = %#v", h.herdr.starts)
	}
}

func TestSingleStartLeavesLegacyOpenCodeAgentEmpty(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14 && len(h.herdr.starts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.herdr.starts) != 2 || h.herdr.starts[0].OpenCodeAgent != "" || h.herdr.starts[1].OpenCodeAgent != "" {
		t.Fatalf("legacy start routing = %#v", h.herdr.starts)
	}
}

func TestSingleRunPersistsDistinctTerminalIdentitiesForSeparateAgentWorkspaces(t *testing.T) {
	h := newHarness(t)
	h.herdr.useTerminalIdentity = true
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14 && len(h.herdr.prompts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}

	got := h.mustLoad(id)
	if got.Builder.SessionID != "herdr-terminal:terminal-builder" || got.Reviewer.SessionID != "herdr-terminal:terminal-reviewer" {
		t.Fatalf("terminal identities=%q/%q", got.Builder.SessionID, got.Reviewer.SessionID)
	}
	if got.Builder.SessionID == got.Reviewer.SessionID || got.BuilderWorktree.WorkspaceID == got.ReviewerWorktree.WorkspaceID {
		t.Fatalf("separate identities/workspaces were not persisted: %#v", got)
	}
}

func TestContainsCredentialRequiresAssignmentForGenericSecretNames(t *testing.T) {
	for _, value := range []string{
		"THREADDOCK_GH_TOKEN=plain-internal-token",
		"token=plain-internal-token",
		"password=hunter2",
		"authorization: Bearer internal-token",
		"secret=internal-secret",
		"apiKey: internal-api-key",
		"private_key = private-material",
		"clientSecret=client-material",
		`{"token":"plain-internal-token"}`,
		`{"password":"hunter2"}`,
		`{"authorization":"Bearer internal-token"}`,
		`'secret' = 'value'`,
		`"clientSecret": "value"`,
		"-----BEGIN PRIVATE KEY-----",
		"ghp_1234567890123456789012345678901234567890",
		"ASIA1234567890ABCDEF",
	} {
		if !containsCredential(value) {
			t.Errorf("containsCredential(%q)=false, want true", value)
		}
	}
	for _, value := range []string{
		"the token field identifies the auth token",
		"privateKey is the configured field name",
		"client secret is never persisted",
		`quoted "token" key is documented`,
		"clientSecret is the configured field name",
	} {
		if containsCredential(value) {
			t.Errorf("containsCredential(%q)=true, want false", value)
		}
	}
}

func TestStartUnreadableContractCreatesNoRunOrIssue(t *testing.T) {
	h := newHarness(t)

	_, err := h.orchestrator.Start(context.Background(), filepath.Join(h.dir, "missing.json"))
	if err == nil {
		t.Fatal("Start succeeded for unreadable contract")
	}
	if h.github.issueCalls != 0 {
		t.Fatalf("issue calls = %d", h.github.issueCalls)
	}
	got, err := h.store.ListRecoverable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("recoverable runs = %#v", got)
	}
}

func TestStartIssueFailurePreservesRegisteredPhaseAndEvent(t *testing.T) {
	h := newHarness(t)
	h.github.issueErr = errors.New("issue service unavailable")

	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err == nil {
		t.Fatal("Start succeeded despite issue failure")
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseRegistered {
		t.Fatalf("phase = %s, want registered", got.Phase)
	}
	events := h.events(id)
	if !hasEventMessage(events, "Issue bundle") {
		t.Fatalf("events = %#v", events)
	}
}

func TestStopPausesWithoutDeletingWorktree(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.herdr.worktrees != 1 {
		t.Fatalf("worktrees = %d", h.herdr.worktrees)
	}
	if err := h.orchestrator.Stop(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(id).Phase; got != contract.PhasePaused {
		t.Fatalf("phase = %s, want paused", got)
	}
	if h.herdr.worktrees != 1 {
		t.Fatal("Stop deleted the worktree")
	}
}

func TestTemporaryGitHubFailureUsesBoundedBackoffWithoutDuplicateRuntimeObjects(t *testing.T) {
	h := newHarness(t)
	h.github.temporaryFailures = 4

	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err == nil {
		t.Fatal("Start succeeded despite repeated temporary failures")
	}
	if h.github.issueCalls != 4 {
		t.Fatalf("issue calls = %d, want 4 total attempts", h.github.issueCalls)
	}
	if got, want := h.clock.sleeps, []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}; !sameDurations(got, want) {
		t.Fatalf("sleeps = %v, want %v", got, want)
	}
	if h.herdr.worktrees != 0 {
		t.Fatalf("worktrees = %d, want no worktree after failed registration", h.herdr.worktrees)
	}
	if got := h.mustLoad(id).Phase; got != contract.PhaseRegistered {
		t.Fatalf("phase = %s, want registered", got)
	}
	if !hasEventMessage(h.events(id), "GitHub 연결 문제") {
		t.Fatalf("events = %#v", h.events(id))
	}
}

func TestBuilderEvidenceIsRequiredBeforeIntegration(t *testing.T) {
	h := newHarness(t)
	h.herdr.recent = "changed_file: internal/payments/retry.go\nverification: go test ./internal/payments\n"
	h.herdr.evidence = herdr.Evidence{Verification: []herdr.VerificationCheck{{Command: "go test ./internal/payments", Outcome: "failed"}}}
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatalf("advance %d: %v", i+1, err)
		}
	}
	if err := h.orchestrator.Advance(context.Background(), id); !errors.Is(err, ErrBuilderEvidence) {
		t.Fatalf("error = %v, want builder evidence error", err)
	}
	if got := h.mustLoad(id).Phase; got != contract.PhaseBuilding {
		t.Fatalf("phase = %s, want building", got)
	}
	if h.git.merges != 0 {
		t.Fatal("integration ran without builder evidence")
	}
}

func TestAdvancePersistsIntentBeforeEachExternalAction(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		before := len(h.events(id))
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatalf("advance %d: %v", i+1, err)
		}
		after := h.events(id)
		if len(after) <= before || !strings.Contains(after[before].Type, "intent") && !strings.Contains(after[before].Kind, "intent") {
			t.Fatalf("advance %d did not append intent first: %#v", i+1, after)
		}
	}
}

func TestStopRejectsUnknownRun(t *testing.T) {
	h := newHarness(t)
	if err := h.orchestrator.Stop(context.Background(), "missing"); err == nil {
		t.Fatal("Stop succeeded for unknown run")
	}
}

const (
	validSHA                = "0123456789abcdef0123456789abcdef01234567"
	builderTranscriptSecret = "BUILDER-PRIVATE-TRANSCRIPT"
)

func writeFixtureContract(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.Write(f, testfixture.ValidContract()); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
