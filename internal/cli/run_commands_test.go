package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/state"
)

func TestStatusJSONContract(t *testing.T) {
	h := newCLIHarness(t)
	var out, errOut bytes.Buffer
	code := h.Run([]string{"status", "run-184", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	var view contract.StatusView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ContractVersion != 1 || view.RunID != "run-184" {
		t.Fatalf("view=%#v", view)
	}
	if errOut.Len() != 0 || !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
	if got := h.service.calls; len(got) != 1 || got[0] != "status:run-184" {
		t.Fatalf("calls=%v", got)
	}
}

func TestStatusHumanShowsProgressAndNextAction(t *testing.T) {
	h := newCLIHarness(t)
	var out, errOut bytes.Buffer
	code := h.Run([]string{"status", "run-184"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"단계: reviewing", "요약: 독립 확인 중", "최근 진행: 독립 확인 중", "다음 작업: Reviewer 결과 확인"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout=%q missing %q", out.String(), want)
		}
	}
}

func TestRunCommandsRouteStableExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
		call string
	}{
		{name: "start", args: []string{"start", "contract.json"}, want: 0, call: "start:contract.json"},
		{name: "stop", args: []string{"stop", "run-184"}, want: 0, call: "stop:run-184"},
		{name: "resume", args: []string{"resume", "run-184"}, want: 0, call: "resume:run-184"},
		{name: "cleanup", args: []string{"cleanup", "run-184"}, want: 0, call: "cleanup:run-184"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCLIHarness(t)
			var out, errOut bytes.Buffer
			if got := h.Run(tt.args, &out, &errOut); got != tt.want {
				t.Fatalf("code=%d stdout=%q stderr=%q", got, out.String(), errOut.String())
			}
			if len(h.service.calls) != 1 || h.service.calls[0] != tt.call {
				t.Fatalf("calls=%v", h.service.calls)
			}
		})
	}
}

func TestRunCommandsRejectMisuseWithExitCodeTwo(t *testing.T) {
	h := newCLIHarness(t)
	var out, errOut bytes.Buffer
	if got := h.Run([]string{"stop"}, &out, &errOut); got != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", got, out.String(), errOut.String())
	}
	if len(h.service.calls) != 0 || errOut.Len() == 0 {
		t.Fatalf("calls=%v stderr=%q", h.service.calls, errOut.String())
	}
}

func TestStatusJSONWritesNoHumanOutputOnServiceFailure(t *testing.T) {
	h := newCLIHarness(t)
	h.service.statusErr = errFakeRun
	var out, errOut bytes.Buffer
	if got := h.Run([]string{"status", "run-184", "--json"}, &out, &errOut); got != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", got, out.String(), errOut.String())
	}
	if out.Len() != 0 || !strings.Contains(errOut.String(), "실행") {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestRunWithDependenciesAcceptsNilContext(t *testing.T) {
	h := newCLIHarness(t)
	var out, errOut bytes.Buffer
	if got := RunWithDependencies(nil, []string{"status", "run-184", "--json"}, &out, &errOut, Dependencies{Runs: h.service}); got != 0 {
		t.Fatalf("code=%d stderr=%q", got, errOut.String())
	}
}

func TestResumeRestoresPreviousPhasePreservingRecoveryCursorAndIDs(t *testing.T) {
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID:           "run-184",
		Phase:           contract.PhasePaused,
		PreviousPhase:   contract.PhaseBuilding,
		PendingAction:   "prompt_builder",
		ActionCursor:    2,
		BuilderPrompt:   state.PromptReceipt{RequestID: "run-184:builder-prompt", BaselineSeq: 42},
		BuilderWorktree: state.WorktreeState{Path: "/managed/builder", WorkspaceID: "ws-1", PaneID: "pane-1"},
	}}
	coordinator := &fakeCoordinator{}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) })

	if err := service.Resume(context.Background(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if coordinator.advanceCalls != 1 {
		t.Fatalf("advance calls=%d, want 1", coordinator.advanceCalls)
	}
	if got := store.snapshot; got.Phase != contract.PhaseBuilding || got.PendingAction != "prompt_builder" || got.ActionCursor != 2 || got.BuilderPrompt.RequestID != "run-184:builder-prompt" || got.BuilderWorktree.PaneID != "pane-1" {
		t.Fatalf("resumed snapshot=%+v", got)
	}
	if len(store.events) != 1 || store.events[0].Type != "resumed" {
		t.Fatalf("events=%+v", store.events)
	}
}

func TestCleanupRejectsDirtyWorktreeBeforeAnyMutation(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration/run-184",
		Integration:      state.WorktreeState{Path: "/managed/integration/run-184"},
		BuilderWorktree:  state.WorktreeState{Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-builder-184", PaneID: "pane-builder-184"},
		ReviewerWorktree: state.WorktreeState{Path: "/home/operator/.herdr/worktrees/reviewer-184", WorkspaceID: "workspace-reviewer-184", PaneID: "pane-reviewer-184"},
	}}
	cleanup := &fakeCleanup{statuses: map[string]string{"/home/operator/.herdr/worktrees/builder-184": " M file.go"}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now })

	if err := service.Cleanup(context.Background(), "run-184"); err == nil || !strings.Contains(err.Error(), "깨끗하지 않아") {
		t.Fatalf("err=%v", err)
	}
	if len(cleanup.removed) != 0 || len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("cleanup mutated: integration=%v herdr=%v state=%v", cleanup.removed, cleanup.herdrRemoved, remover.calls)
	}
}

func TestCleanupRemovesOnlyAfterAllWorktreesAreClean(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration/run-184",
		Integration:      state.WorktreeState{Path: "/managed/integration/run-184"},
		BuilderWorktree:  state.WorktreeState{Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-builder-184", PaneID: "pane-builder-184"},
		ReviewerWorktree: state.WorktreeState{Path: "/home/operator/.herdr/worktrees/reviewer-184", WorkspaceID: "workspace-reviewer-184", PaneID: "pane-reviewer-184"},
	}}
	cleanup := &fakeCleanup{statuses: map[string]string{}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now })

	if err := service.Cleanup(context.Background(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.statusCalls) != 3 || len(cleanup.removed) != 1 || len(cleanup.herdrRemoved) != 2 || len(remover.calls) != 1 || remover.calls[0] != "run-184" {
		t.Fatalf("status=%v integration=%v herdr=%v state=%v", cleanup.statusCalls, cleanup.removed, cleanup.herdrRemoved, remover.calls)
	}
	wantPrefix := []string{"status:/managed/integration/run-184", "status:/home/operator/.herdr/worktrees/builder-184", "status:/home/operator/.herdr/worktrees/reviewer-184"}
	for i, want := range wantPrefix {
		if cleanup.events[i] != want {
			t.Fatalf("events=%v, want preflight event %q at %d", cleanup.events, want, i)
		}
	}
}

func TestCleanupRejectsIncompleteHerdrIdentityWithoutMutation(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration/run-184",
		Integration:     state.WorktreeState{Path: "/managed/integration/run-184"},
		BuilderWorktree: state.WorktreeState{Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-builder-184"},
	}}
	cleanup := &fakeCleanup{statuses: map[string]string{}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now })

	if err := service.Cleanup(context.Background(), "run-184"); err == nil || !strings.Contains(err.Error(), "식별자") {
		t.Fatalf("err=%v", err)
	}
	if len(cleanup.statusCalls) != 0 || len(cleanup.removed) != 0 || len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("cleanup mutated: status=%v integration=%v herdr=%v state=%v", cleanup.statusCalls, cleanup.removed, cleanup.herdrRemoved, remover.calls)
	}
}

func TestCleanupPreservesStateAfterHerdrRemovalFailure(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration/run-184",
		Integration:      state.WorktreeState{Path: "/managed/integration/run-184"},
		BuilderWorktree:  state.WorktreeState{Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-builder-184", PaneID: "pane-builder-184"},
		ReviewerWorktree: state.WorktreeState{Path: "/home/operator/.herdr/worktrees/reviewer-184", WorkspaceID: "workspace-reviewer-184", PaneID: "pane-reviewer-184"},
	}}
	cleanup := &fakeCleanup{statuses: map[string]string{}, herdrRemoveErr: errors.New("raw Herdr stderr token=secret")}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now })

	err := service.Cleanup(context.Background(), "run-184")
	if err == nil || !strings.Contains(err.Error(), "Worktree 정리") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err=%v", err)
	}
	if len(cleanup.statusCalls) != 3 || len(cleanup.events) < 3 || cleanup.events[2] != "status:/home/operator/.herdr/worktrees/reviewer-184" {
		t.Fatalf("preflight incomplete: statuses=%v events=%v", cleanup.statusCalls, cleanup.events)
	}
	if len(remover.calls) != 0 {
		t.Fatalf("state removed after partial cleanup: %v", remover.calls)
	}
}

func TestHerdrCleanupUsesWorkspaceIDWithoutForce(t *testing.T) {
	runner := &recordedProcessRunner{}
	cleanup := SafeWorktreeCleanup{Runner: runner, HerdrBinary: "herdr-test"}
	if err := cleanup.RemoveHerdr(context.Background(), "workspace-builder-184"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls=%v", runner.calls)
	}
	call := runner.calls[0]
	if call.executable != "herdr-test" || strings.Join(call.args, " ") != "worktree remove --workspace workspace-builder-184" {
		t.Fatalf("call=%+v", call)
	}
	for _, arg := range call.args {
		if arg == "--force" {
			t.Fatal("Herdr cleanup must not force removal")
		}
	}
}

var _ io.Writer = (*bytes.Buffer)(nil)
