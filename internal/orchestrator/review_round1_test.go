package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

func TestRound1CreatesAndPersistsIntegrationWorktree(t *testing.T) {
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
	got := h.mustLoad(id)
	if h.git.creates != 1 || h.git.integrationPath != got.IntegrationPath {
		t.Fatalf("creates=%d path=%q snapshot=%q", h.git.creates, h.git.integrationPath, got.IntegrationPath)
	}
	if got.Integration.Path == "" || got.Integration.Branch == "" {
		t.Fatalf("integration state = %#v", got.Integration)
	}
	if got.BuilderWorktree.Path == "" || got.BuilderWorktree.PaneID == "" {
		t.Fatalf("builder worktree state = %#v", got.BuilderWorktree)
	}
}

func TestRound1RestartReconcilesPersistedResourcesAndCursor(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	first := h.mustLoad(id)
	if first.PendingAction != "" || first.ActionCursor == 0 {
		t.Fatalf("first durable cursor = %#v", first)
	}

	resumed := New(h.Deps)
	if err := resumed.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseBuilding || got.BuilderWorktree.WorkspaceID == "" || got.BuilderWorktree.Path == "" {
		t.Fatalf("resumed snapshot = %#v", got)
	}
	if h.herdr.worktrees != 1 {
		t.Fatalf("duplicate Herdr worktree creation: %d", h.herdr.worktrees)
	}
}

func TestRound1IntegrationPendingReconcileUsesGitNotHerdr(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.PendingAction = "create_integration_worktree"
	snapshot.Integration.Path = snapshot.IntegrationPath
	snapshot.Integration.Branch = "agent/parent-integration"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.git.reconcileCalls != 1 || h.herdr.findWorktreeCalls != 0 {
		t.Fatalf("git reconcile=%d Herdr reconcile=%d", h.git.reconcileCalls, h.herdr.findWorktreeCalls)
	}
}

func TestRound1ReviewerReceivesBoundedPatchAndStructuredVerificationOnly(t *testing.T) {
	h := newHarness(t)
	h.herdr.recent = builderTranscriptSecret + " arbitrary transcript"
	h.herdr.evidence = herdr.Evidence{
		CommitSHA: validSHA,
		Verification: []herdr.VerificationCheck{
			{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1.2s"},
			{Command: "go vet ./internal/payments", Outcome: "passed", Duration: "0.4s"},
		},
	}
	h.git.inspection = worktree.CommitInspection{
		CommitSHA:    validSHA,
		Branch:       "agent/api",
		ChangedFiles: []string{"src/payments/retry.go"},
		Patch:        "diff --git a/src/payments/retry.go b/src/payments/retry.go\n+bounded patch\n",
	}
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12 && len(h.herdr.prompts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.herdr.prompts) < 2 {
		t.Fatalf("reviewer prompt was not sent; phase=%s", h.mustLoad(id).Phase)
	}
	packet := h.herdr.prompts[len(h.herdr.prompts)-1]
	if !strings.Contains(packet, h.git.inspection.Patch) || !strings.Contains(packet, "Outcome: passed") {
		t.Fatalf("packet lacks bounded evidence: %q", packet)
	}
	if strings.Contains(packet, builderTranscriptSecret) || strings.Contains(packet, "arbitrary transcript") {
		t.Fatalf("raw transcript leaked into reviewer packet: %q", packet)
	}
}

func TestRound1RejectsUnverifiedCommitAndDoesNotMerge(t *testing.T) {
	h := newHarness(t)
	h.herdr.evidence = herdr.Evidence{CommitSHA: validSHA, Verification: []herdr.VerificationCheck{{Command: "go test ./internal/payments", Outcome: "failed"}}}
	h.git.inspection = worktree.CommitInspection{CommitSHA: validSHA, Branch: "agent/api", ChangedFiles: []string{"src/payments/retry.go"}, Patch: "patch"}
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.orchestrator.Advance(context.Background(), id); !errors.Is(err, ErrBuilderEvidence) {
		t.Fatalf("error=%v want missing duration evidence", err)
	}
	if h.git.merges != 0 {
		t.Fatal("unverified Builder was merged")
	}
	if got := h.mustLoad(id).Phase; got != contract.PhaseBuilding {
		t.Fatalf("phase=%s want building", got)
	}
}

func TestRound1VerificationEvidenceRequiresDuration(t *testing.T) {
	h := newHarness(t)
	h.herdr.evidence = herdr.Evidence{CommitSHA: validSHA, Verification: []herdr.VerificationCheck{{Command: "go test ./internal/payments", Outcome: "passed"}}}
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil && !errors.Is(err, ErrBuilderEvidence) {
			t.Fatal(err)
		}
	}
	if got := h.mustLoad(id).Phase; got != contract.PhaseBuilding {
		t.Fatalf("phase=%s want building", got)
	}
}

func TestRound1ConcurrentAdvanceSerializesOneAction(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	h.herdr.startEntered = make(chan struct{})
	h.herdr.releaseStart = make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = h.orchestrator.Advance(context.Background(), id)
		}(i)
	}
	select {
	case <-h.herdr.startEntered:
	case <-time.After(time.Second):
		t.Fatal("first action did not start")
	}
	close(h.herdr.releaseStart)
	wg.Wait()
	if h.herdr.startCount != 1 {
		t.Fatalf("StartAgent calls=%d", h.herdr.startCount)
	}
	if !errors.Is(errs[0], ErrRunBusy) && !errors.Is(errs[1], ErrRunBusy) {
		t.Fatalf("concurrent errors=%v", errs)
	}
}

func TestRound1GHESFailureIsDurableAndBlocksFollowupAction(t *testing.T) {
	h := newHarness(t)
	h.github.temporaryFailures = 4
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err == nil {
		t.Fatal("Start unexpectedly succeeded")
	}
	got := h.mustLoad(id)
	if got.Summary != connectionProblem || got.PendingAction != "register_issue_bundle" || got.Registration.Status != "pending" {
		t.Fatalf("durable failure = %#v", got)
	}
	h.github.issueErr = nil
	if err := h.orchestrator.Advance(context.Background(), id); err == nil {
		t.Fatal("Advance bypassed pending GHES reconciliation")
	}
	if h.herdr.worktrees != 0 || h.git.creates != 0 {
		t.Fatal("follow-up action ran before registration reconciliation")
	}
}

func TestRound1StateLockIsProcessSafe(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := h.store.Acquire(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if err := h.orchestrator.Advance(context.Background(), id); !errors.Is(err, ErrRunBusy) {
		t.Fatalf("Advance error=%v want busy", err)
	}
}

var _ state.RunSnapshot
