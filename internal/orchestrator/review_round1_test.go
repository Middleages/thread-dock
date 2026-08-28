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

func TestRound2NotFoundReconcileDefersCreateToNextAdvance(t *testing.T) {
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
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.git.reconcileExists = false
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatalf("reconcile error=%v", err)
	}
	if got := h.mustLoad(id); got.PendingAction != "" || h.git.creates != 0 {
		t.Fatalf("not-found reconcile state=%#v creates=%d", got, h.git.creates)
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.git.creates != 1 || h.mustLoad(id).PendingAction != "" {
		t.Fatalf("deferred create state=%#v creates=%d", h.mustLoad(id), h.git.creates)
	}
}

func TestRound2FoundBuilderReconcileEntersBuildingCursorZero(t *testing.T) {
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
	snapshot := h.mustLoad(id)
	snapshot.PendingAction = "create_builder_worktree"
	snapshot.BuilderWorktree.Branch = "agent/api"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseBuilding || got.ActionCursor != 0 || got.BuilderWorktree.PaneID == "" {
		t.Fatalf("builder reconcile state=%#v", got)
	}
	if h.herdr.worktrees != 0 {
		t.Fatalf("reconcile created duplicate worktree: %d", h.herdr.worktrees)
	}
	if h.herdr.findWorktreeCWD != snapshot.RepositoryPath {
		t.Fatalf("reconcile cwd=%q want %q", h.herdr.findWorktreeCWD, snapshot.RepositoryPath)
	}
}

func TestRound2NotFoundBuilderReconcileDefersCreateToNextAdvance(t *testing.T) {
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
	snapshot := h.mustLoad(id)
	snapshot.PendingAction = "create_builder_worktree"
	snapshot.BuilderWorktree.Branch = "agent/api"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.herdr.findWorktreeExists = false
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(id); got.PendingAction != "" || h.herdr.worktrees != 0 {
		t.Fatalf("not-found state=%#v worktrees=%d", got, h.herdr.worktrees)
	}
	h.herdr.findWorktreeExists = true
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.herdr.worktrees != 1 || h.mustLoad(id).Phase != contract.PhaseBuilding || h.mustLoad(id).ActionCursor != 0 {
		t.Fatalf("deferred state=%#v worktrees=%d", h.mustLoad(id), h.herdr.worktrees)
	}
}

func TestRound2PromptPendingSequenceDefersOrConfirmsExactlyOnce(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	snapshot.ActionCursor = 2
	snapshot.Builder = state.AgentEvidence{Name: agentName("builder", id), SessionID: "builder-session"}
	snapshot.BuilderPrompt = state.PromptReceipt{RequestID: string(id) + ":builder-prompt", BaselineSeq: 42}
	snapshot.PendingAction = "prompt_builder"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.herdr.agentSeq = 42
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatalf("same-seq error=%v", err)
	}
	if got := h.mustLoad(id); got.PendingAction != "" || got.ActionCursor != 2 || len(h.herdr.prompts) != 0 || h.herdr.promptReceiptReads != 1 {
		t.Fatalf("same-seq state=%#v prompts=%d", got, len(h.herdr.prompts))
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(h.herdr.prompts) != 1 {
		t.Fatalf("prompt count=%d", len(h.herdr.prompts))
	}
	snapshot = h.mustLoad(id)
	snapshot.PendingAction = "prompt_builder"
	snapshot.BuilderPrompt.BaselineSeq = 42
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.herdr.agentSeq = 43
	if err := New(h.Deps).Advance(context.Background(), id); !errors.Is(err, ErrPendingReconcile) {
		t.Fatalf("error=%v want pending reconcile", err)
	}
	if got := h.mustLoad(id); got.PendingAction != "prompt_builder" || got.ActionCursor != 3 || len(h.herdr.prompts) != 1 || h.herdr.promptReceiptReads != 2 {
		t.Fatalf("increased-seq state=%#v prompts=%d", got, len(h.herdr.prompts))
	}

	snapshot = h.mustLoad(id)
	snapshot.PendingAction = "prompt_builder"
	snapshot.BuilderPrompt.BaselineSeq = 42
	h.herdr.recent = "receipt=" + snapshot.BuilderPrompt.RequestID
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(id); got.PendingAction != "" || got.ActionCursor != 4 || len(h.herdr.prompts) != 1 || h.herdr.promptReceiptReads != 3 {
		t.Fatalf("observed state=%#v prompts=%d", got, len(h.herdr.prompts))
	}
}

func TestRound2ReviewerPromptPendingWithMissingReceiptStaysUncertain(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseReviewing
	snapshot.ActionCursor = 3
	snapshot.Reviewer = state.AgentEvidence{Name: agentName("reviewer", id), SessionID: "reviewer-session"}
	snapshot.ReviewerPrompt = state.PromptReceipt{RequestID: string(id) + ":reviewer-prompt", BaselineSeq: 42}
	snapshot.PendingAction = "prompt_reviewer"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.herdr.agentSeq = 43
	h.herdr.recent = "reviewer is still working without a receipt"
	if err := New(h.Deps).Advance(context.Background(), id); !errors.Is(err, ErrPendingReconcile) {
		t.Fatalf("error=%v want pending reconcile", err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "prompt_reviewer" || got.ActionCursor != 3 || len(h.herdr.prompts) != 0 || h.herdr.promptReceiptReads != 1 {
		t.Fatalf("reviewer uncertain state=%#v prompts=%d", got, len(h.herdr.prompts))
	}
}

func TestRound2PendingMergeRequiresVerifiedCommitAtHead(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseIntegrating
	snapshot.ActionCursor = 1
	snapshot.PendingAction = "merge_verified_commit"
	snapshot.Builder = state.AgentEvidence{RequestID: string(id) + ":builder-prompt", CommitSHA: validSHA, Branch: "agent/api", Patch: "bounded patch", VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1s"}}}
	snapshot.BuilderPrompt.RequestID = snapshot.Builder.RequestID
	snapshot.Integration.Path = "/tmp/integration"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.git.currentCommit = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if err := New(h.Deps).Advance(context.Background(), id); !errors.Is(err, ErrPendingReconcile) {
		t.Fatalf("error=%v want pending reconcile", err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "merge_verified_commit" || got.Phase != contract.PhaseIntegrating || h.git.merges != 0 {
		t.Fatalf("merge mismatch state=%#v merges=%d", got, h.git.merges)
	}
}

func TestRound2AgentStartNotFoundReconcileDefersStartToNextAdvance(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	snapshot.ActionCursor = 0
	snapshot.BuilderWorktree = state.WorktreeState{Path: "/tmp/builder", WorkspaceID: "workspace-184", PaneID: "pane-184", Branch: "agent/api"}
	snapshot.Builder = state.AgentEvidence{Name: agentName("builder", id)}
	snapshot.PendingAction = "start_builder"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.herdr.agentInfoErr = herdr.ErrAgentNotFound
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatalf("not-found reconcile error=%v", err)
	}
	if got := h.mustLoad(id); got.PendingAction != "" || h.herdr.startCount != 0 {
		t.Fatalf("not-found state=%#v starts=%d", got, h.herdr.startCount)
	}
	h.herdr.agentInfoErr = nil
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.herdr.startCount != 1 || h.mustLoad(id).PendingAction != "" {
		t.Fatalf("deferred start state=%#v starts=%d", h.mustLoad(id), h.herdr.startCount)
	}
}

func TestRound1ReviewerReceivesBoundedPatchAndStructuredVerificationOnly(t *testing.T) {
	h := newHarness(t)
	h.herdr.recent = builderTranscriptSecret + " arbitrary transcript"
	h.herdr.evidence = herdr.Evidence{
		CommitSHA: validSHA,
		Verification: []herdr.VerificationCheck{
			{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1.2s"},
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
	for i := 0; i < 14 && len(h.herdr.prompts) < 2; i++ {
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
	for i := 0; i < 7; i++ {
		err = h.orchestrator.Advance(context.Background(), id)
		if i < 5 && err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(err, ErrBuilderEvidence) {
		t.Fatalf("error=%v want failed verification evidence", err)
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
	for i := 0; i < 7; i++ {
		err = h.orchestrator.Advance(context.Background(), id)
		if i < 5 && err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(err, ErrBuilderEvidence) {
		t.Fatalf("error=%v want missing duration evidence", err)
	}
	if got := h.mustLoad(id).Phase; got != contract.PhaseBuilding {
		t.Fatalf("phase=%s want building", got)
	}
}

func TestRound2PromptReceiptUsesBaselineSequenceAndRequestID(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20 && len(h.herdr.prompts) < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	got := h.mustLoad(id)
	if got.BuilderPrompt.RequestID == "" || got.BuilderPrompt.BaselineSeq == 0 || got.ReviewerPrompt.RequestID == "" || got.ReviewerPrompt.BaselineSeq == 0 {
		t.Fatalf("prompt receipts = builder %#v reviewer %#v", got.BuilderPrompt, got.ReviewerPrompt)
	}
	if !strings.Contains(h.herdr.prompts[0], got.BuilderPrompt.RequestID) || !strings.Contains(h.herdr.prompts[1], got.ReviewerPrompt.RequestID) {
		t.Fatalf("request IDs missing from packets: %#v", h.herdr.prompts)
	}
	if !strings.Contains(h.herdr.prompts[0], herdr.EvidenceSchemaExample) || strings.Contains(h.herdr.prompts[0], "Markdown") == false {
		t.Fatalf("Builder packet lacks strict shared schema: %q", h.herdr.prompts[0])
	}
	if h.herdr.evidence.RequestID != got.BuilderPrompt.RequestID {
		t.Fatalf("evidence requestID=%q want %q", h.herdr.evidence.RequestID, got.BuilderPrompt.RequestID)
	}
}

func TestRound2BaselineCrashReconcileStoresSeqBeforePrompt(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	snapshot.ActionCursor = 1
	snapshot.BuilderWorktree = state.WorktreeState{WorkspaceID: "workspace-184", PaneID: "pane-184"}
	snapshot.Builder = state.AgentEvidence{Name: agentName("builder", id)}
	snapshot.BuilderPrompt = state.PromptReceipt{RequestID: string(id) + ":builder-prompt"}
	snapshot.PendingAction = "baseline_builder_prompt"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "" || got.ActionCursor != 2 || got.Builder.SessionID != "session-"+agentName("builder", id) || got.BuilderPrompt.BaselineSeq != 42 || len(h.herdr.prompts) != 0 {
		t.Fatalf("baseline reconcile state=%#v prompts=%d", got, len(h.herdr.prompts))
	}
}

func TestRound2ReviewerBaselineCrashReconcileStoresActualSessionAndSeq(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseReviewing
	snapshot.ActionCursor = 3
	snapshot.Reviewer = state.AgentEvidence{Name: agentName("reviewer", id)}
	snapshot.ReviewerWorktree = state.WorktreeState{Path: "/tmp/review", WorkspaceID: "workspace-review-184", PaneID: "pane-review-184"}
	snapshot.ReviewerPrompt = state.PromptReceipt{RequestID: string(id) + ":reviewer-prompt"}
	snapshot.PendingAction = "baseline_reviewer_prompt"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := New(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "" || got.ActionCursor != 4 || got.Reviewer.SessionID != "session-"+agentName("reviewer", id) || got.ReviewerPrompt.BaselineSeq != 42 || len(h.herdr.prompts) != 0 {
		t.Fatalf("baseline reconcile state=%#v prompts=%d", got, len(h.herdr.prompts))
	}
}

func TestRound2BaselineCrashReconcileRejectsWrongIdentityForBuilderAndReviewer(t *testing.T) {
	cases := []struct {
		name      string
		pending   string
		reviewer  bool
		workspace string
		pane      string
		session   string
	}{
		{name: "builder workspace", pending: "baseline_builder_prompt", workspace: "wrong-workspace", pane: "pane-184", session: "session-builder"},
		{name: "builder pane", pending: "baseline_builder_prompt", workspace: "workspace-184", pane: "wrong-pane", session: "session-builder"},
		{name: "builder empty session", pending: "baseline_builder_prompt", workspace: "workspace-184", pane: "pane-184"},
		{name: "reviewer workspace", pending: "baseline_reviewer_prompt", reviewer: true, workspace: "wrong-workspace", pane: "pane-review-184", session: "session-reviewer"},
		{name: "reviewer pane", pending: "baseline_reviewer_prompt", reviewer: true, workspace: "workspace-review-184", pane: "wrong-pane", session: "session-reviewer"},
		{name: "reviewer empty session", pending: "baseline_reviewer_prompt", reviewer: true, workspace: "workspace-review-184", pane: "pane-review-184"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			id, err := h.orchestrator.Start(context.Background(), h.contractPath)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := h.mustLoad(id)
			snapshot.Phase = contract.PhaseBuilding
			snapshot.ActionCursor = 1
			snapshot.BuilderWorktree = state.WorktreeState{WorkspaceID: "workspace-184", PaneID: "pane-184"}
			snapshot.ReviewerWorktree = state.WorktreeState{WorkspaceID: "workspace-review-184", PaneID: "pane-review-184"}
			name := agentName("builder", id)
			if tc.reviewer {
				snapshot.Phase = contract.PhaseReviewing
				snapshot.ActionCursor = 3
				name = agentName("reviewer", id)
				snapshot.Reviewer = state.AgentEvidence{Name: name}
				snapshot.ReviewerPrompt = state.PromptReceipt{RequestID: string(id) + ":reviewer-prompt"}
			} else {
				snapshot.Builder = state.AgentEvidence{Name: name}
				snapshot.BuilderPrompt = state.PromptReceipt{RequestID: string(id) + ":builder-prompt"}
			}
			snapshot.PendingAction = tc.pending
			if err := h.store.Save(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}
			h.herdr.agentInfoOverride = &herdr.AgentInfo{Name: name, SessionID: tc.session, WorkspaceID: tc.workspace, PaneID: tc.pane, StateChangeSeq: 42}
			if err := New(h.Deps).Advance(context.Background(), id); !errors.Is(err, ErrPendingReconcile) {
				t.Fatalf("error=%v want pending reconcile", err)
			}
			got := h.mustLoad(id)
			if got.PendingAction != tc.pending || got.ActionCursor != snapshot.ActionCursor {
				t.Fatalf("state changed after identity mismatch: %#v", got)
			}
		})
	}
}

func TestRound2StopPreservesRecoveryCursorAndPendingState(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	snapshot.PreviousPhase = contract.PhaseAnalyzing
	snapshot.PendingAction = "prompt_builder"
	snapshot.ActionCursor = 2
	snapshot.BuilderPrompt.RequestID = "prompt-1"
	snapshot.BuilderPrompt.BaselineSeq = 9
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Stop(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhasePaused || got.PreviousPhase != contract.PhaseBuilding || got.PendingAction != "prompt_builder" || got.ActionCursor != 2 || got.BuilderPrompt.RequestID != "prompt-1" || got.BuilderPrompt.BaselineSeq != 9 {
		t.Fatalf("stop lost recovery state: %#v", got)
	}
}

func TestRound2RejectsExactVerificationCommandMismatch(t *testing.T) {
	h := newHarness(t)
	h.herdr.evidence.Verification = []herdr.VerificationCheck{{Command: "true", Outcome: "passed", Duration: "1s"}}
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for i := 0; i < 7; i++ {
		last = h.orchestrator.Advance(context.Background(), id)
		if i < 6 && last != nil {
			t.Fatal(last)
		}
	}
	if !errors.Is(last, ErrBuilderEvidence) {
		t.Fatalf("error=%v want exact verification mismatch", last)
	}
	if got := h.mustLoad(id).Phase; got != contract.PhaseBuilding {
		t.Fatalf("phase=%s want building", got)
	}
}

func TestVerificationMatchesTaskRejectsOneCheckForMultipleRequiredCommands(t *testing.T) {
	checks := []herdr.VerificationCheck{{Command: "test -f pilot-result.txt", Outcome: "passed", Duration: "1ms"}}
	required := []string{"test -f pilot-result.txt", "go test ./internal/herdr"}
	if verificationMatchesTask(checks, required) {
		t.Fatal("accepted one normalized verification check for multiple required commands")
	}
}

func TestRound2BlocksCredentialInGitPatchBeforeReviewerPrompt(t *testing.T) {
	h := newHarness(t)
	h.git.inspection.Patch = "diff --git a/config b/config\n+apiKey = \\\"value\\\"\n"
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		err = h.orchestrator.Advance(context.Background(), id)
		if errors.Is(err, ErrSensitivePatch) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(err, ErrSensitivePatch) {
		t.Fatalf("error=%v want sensitive patch block", err)
	}
	if len(h.herdr.prompts) > 1 || h.mustLoad(id).Phase != contract.PhaseIntegrating || h.mustLoad(id).Builder.Patch != "" {
		t.Fatalf("reviewer prompt/state = %d/%s", len(h.herdr.prompts), h.mustLoad(id).Phase)
	}
}

func TestRound2BlocksCredentialPatchBeforePersistingNormalInspection(t *testing.T) {
	h := newHarness(t)
	h.git.inspection.Patch = "diff --git a/config b/config\n+ghp_1234567890123456789012345678901234567890\n"
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		err = h.orchestrator.Advance(context.Background(), id)
		if i < 7 && err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(err, ErrSensitivePatch) {
		t.Fatalf("error=%v want sensitive patch block", err)
	}
	got := h.mustLoad(id)
	if got.Builder.Patch != "" || got.Phase != contract.PhaseIntegrating {
		t.Fatalf("snapshot persisted sensitive patch/state=%#v", got)
	}
}

func TestRound2BlocksQuotedCredentialPatchBeforePersistingNormalInspection(t *testing.T) {
	for _, patch := range []string{
		`{"token":"plain-internal-token"}`,
		`{"password":"hunter2"}`,
		`{"authorization":"Bearer internal-token"}`,
		`'secret' = 'value'`,
		`"clientSecret": "value"`,
	} {
		t.Run(patch, func(t *testing.T) {
			h := newHarness(t)
			h.git.inspection.Patch = patch
			id, err := h.orchestrator.Start(context.Background(), h.contractPath)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 8; i++ {
				err = h.orchestrator.Advance(context.Background(), id)
				if i < 7 && err != nil {
					t.Fatal(err)
				}
			}
			if !errors.Is(err, ErrSensitivePatch) {
				t.Fatalf("error=%v want sensitive patch block", err)
			}
			got := h.mustLoad(id)
			if got.Builder.Patch != "" || got.Phase != contract.PhaseIntegrating {
				t.Fatalf("snapshot persisted sensitive patch/state=%#v", got)
			}
		})
	}
}

func TestRound2BlocksCredentialPatchAfterPendingInspectionReconcile(t *testing.T) {
	h := newHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := h.mustLoad(id)
	snapshot.PendingAction = "inspect_builder_commit"
	snapshot.ActionCursor = 0
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.git.inspection.Patch = "diff --git a/config b/config\n+apiKey: leaked\n"
	if err := New(h.Deps).Advance(context.Background(), id); !errors.Is(err, ErrSensitivePatch) {
		t.Fatalf("error=%v want sensitive patch block", err)
	}
	if got := h.mustLoad(id); got.Builder.Patch != "" {
		t.Fatalf("pending reconcile persisted sensitive patch: %#v", got.Builder)
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
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatalf("missing-marker reconcile error=%v", err)
	}
	if got := h.mustLoad(id); got.PendingAction != "" || got.Registration.Status != "pending" {
		t.Fatalf("reconcile state=%#v", got)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatalf("deferred registration error=%v", err)
	}
	if h.github.issueCalls != 5 || h.mustLoad(id).Registration.Status != "registered" {
		t.Fatalf("registration calls=%d state=%#v", h.github.issueCalls, h.mustLoad(id))
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
