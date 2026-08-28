package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/runner"
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
	var wire map[string]any
	if err := json.Unmarshal(out.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	next, ok := wire["nextAction"].(map[string]any)
	if !ok || next["kind"] != "review" || next["label"] != "Reviewer 결과 확인" {
		t.Fatalf("nextAction wire=%v", wire["nextAction"])
	}
	if _, upper := next["Kind"]; upper {
		t.Fatalf("uppercase nested key leaked: %v", next)
	}
	agents, ok := wire["agents"].([]any)
	if !ok || len(agents) != 1 {
		t.Fatalf("agents wire=%v", wire["agents"])
	}
	agent := agents[0].(map[string]any)
	if agent["name"] != "reviewer-184" || agent["role"] != "reviewer" || agent["state"] != "working" || agent["summary"] != "확인 중" {
		t.Fatalf("agent wire=%v", agent)
	}
	githubWire, ok := wire["github"].(map[string]any)
	if !ok || githubWire["parentIssue"] != float64(184) || githubWire["pullRequest"] != float64(185) || githubWire["url"] != "https://github.example/184" || githubWire["ci"] != "pending" {
		t.Fatalf("github wire=%v", wire["github"])
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
	for _, want := range []string{"단계: reviewing", "요약: 독립 확인 중", "최근 갱신:", "다음 작업: Reviewer 결과 확인"} {
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

func TestStartPreservesRunIDWhenServiceReturnsError(t *testing.T) {
	h := newCLIHarness(t)
	h.service.startID = "run-outage-184"
	h.service.startErr = errFakeRun
	var out, errOut bytes.Buffer
	if got := h.Run([]string{"start", "contract.json"}, &out, &errOut); got != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", got, out.String(), errOut.String())
	}
	if first := strings.SplitN(out.String(), "\n", 2)[0]; first != "run-outage-184" {
		t.Fatalf("first stdout line=%q", first)
	}
	if !strings.Contains(errOut.String(), "실행") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}

func TestRunWithDependenciesAcceptsNilContext(t *testing.T) {
	h := newCLIHarness(t)
	var out, errOut bytes.Buffer
	if got := RunWithDependencies(nil, []string{"status", "run-184", "--json"}, &out, &errOut, Dependencies{Runs: h.service}); got != 0 {
		t.Fatalf("code=%d stderr=%q", got, errOut.String())
	}
}

func TestNeedsProductionDependenciesOnlyForValidRunInvocations(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "unknown", args: []string{"wat"}, want: false},
		{name: "version", args: []string{"version"}, want: false},
		{name: "contract", args: []string{"contract", "validate", "contract.json"}, want: false},
		{name: "malformed start", args: []string{"start"}, want: false},
		{name: "malformed stop", args: []string{"stop"}, want: false},
		{name: "malformed status", args: []string{"status", "--unknown"}, want: false},
		{name: "valid start", args: []string{"start", "contract.json"}, want: true},
		{name: "valid status", args: []string{"status", "run-184", "--json"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsProductionDependencies(tt.args); got != tt.want {
				t.Fatalf("NeedsProductionDependencies(%v)=%v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestDiscoverRepositoryPathUsesExplicitGitArguments(t *testing.T) {
	r := &recordedProcessRunner{result: runner.Result{Stdout: "/workspace/payments-api\n", ExitCode: 0}}
	got, err := DiscoverRepositoryPath(context.Background(), r, "git-test")
	if err != nil || got != "/workspace/payments-api" {
		t.Fatalf("path=%q err=%v", got, err)
	}
	if len(r.calls) != 1 || r.calls[0].executable != "git-test" || strings.Join(r.calls[0].args, " ") != "rev-parse --show-toplevel" {
		t.Fatalf("calls=%v", r.calls)
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
	service := NewOrchestratorRunService(coordinator, store, nil, nil, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed")

	if err := service.Resume(context.Background(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if coordinator.advanceCalls != 1 {
		t.Fatalf("advance calls=%d, want 1", coordinator.advanceCalls)
	}
	if got := store.snapshot; got.Phase != contract.PhaseBuilding || got.PreviousPhase != contract.PhaseBuilding || got.PendingAction != "prompt_builder" || got.ActionCursor != 2 || got.BuilderPrompt.RequestID != "run-184:builder-prompt" || got.BuilderWorktree.PaneID != "pane-1" {
		t.Fatalf("resumed snapshot=%+v", got)
	}
	if len(store.events) != 1 || store.events[0].Type != "intent" || store.events[0].Kind != "resume" || store.events[0].Phase != contract.PhaseBuilding {
		t.Fatalf("events=%+v", store.events)
	}
}

func TestResumeAppendFailureLeavesPausedSnapshotUntouched(t *testing.T) {
	store := &fakeStateStore{
		snapshot:  state.RunSnapshot{RunID: "run-184", Phase: contract.PhasePaused, PreviousPhase: contract.PhaseBuilding},
		appendErr: errFakeRun,
	}
	coordinator := &fakeCoordinator{}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed")

	if err := service.Resume(context.Background(), "run-184"); err == nil {
		t.Fatal("expected resume intent append failure")
	}
	if store.snapshot.Phase != contract.PhasePaused || len(store.saves) != 0 || coordinator.advanceCalls != 0 {
		t.Fatalf("snapshot=%+v saves=%v advance=%d", store.snapshot, store.saves, coordinator.advanceCalls)
	}
}

func TestResumeContinuesActiveRunWithOneAdvance(t *testing.T) {
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseBuilding, PreviousPhase: contract.PhaseAnalyzing,
		PendingAction: "prompt_builder", ActionCursor: 2,
	}}
	coordinator := &fakeCoordinator{}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, nil, "/managed")

	if err := service.Resume(context.Background(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if coordinator.advanceCalls != 1 || len(store.saves) != 0 || len(store.events) != 0 || store.snapshot.PreviousPhase != contract.PhaseAnalyzing {
		t.Fatalf("advance=%d saves=%v events=%v snapshot=%+v", coordinator.advanceCalls, store.saves, store.events, store.snapshot)
	}
}

func TestResumeAdvanceFailureCanBeRetriedOnActiveRun(t *testing.T) {
	store := &fakeStateStore{snapshot: state.RunSnapshot{RunID: "run-184", Phase: contract.PhaseIntegrating}}
	coordinator := &fakeCoordinator{advanceErr: errFakeRun}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, nil, "/managed")

	if err := service.Resume(context.Background(), "run-184"); err == nil {
		t.Fatal("expected first advance failure")
	}
	if err := service.Resume(context.Background(), "run-184"); err == nil {
		t.Fatal("expected second advance failure")
	}
	if coordinator.advanceCalls != 2 || store.snapshot.Phase != contract.PhaseIntegrating {
		t.Fatalf("advance=%d snapshot=%+v", coordinator.advanceCalls, store.snapshot)
	}
}

func validHerdrCleanupIdentities() map[string]herdr.Worktree {
	return map[string]herdr.Worktree{
		"/home/operator/.herdr/worktrees/builder-184":  {Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-builder-184", PaneID: "pane-builder-184"},
		"/home/operator/.herdr/worktrees/reviewer-184": {Path: "/home/operator/.herdr/worktrees/reviewer-184", WorkspaceID: "workspace-reviewer-184", PaneID: "pane-reviewer-184"},
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
	cleanup := &fakeCleanup{statuses: map[string]string{"/home/operator/.herdr/worktrees/builder-184": " M file.go"}, foundWorktrees: validHerdrCleanupIdentities()}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed")

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
	cleanup := &fakeCleanup{statuses: map[string]string{}, foundWorktrees: validHerdrCleanupIdentities()}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed")

	if err := service.Cleanup(context.Background(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.statusCalls) != 3 || len(cleanup.removed) != 1 || len(cleanup.herdrRemoved) != 2 || len(remover.calls) != 1 || remover.calls[0] != "run-184" {
		t.Fatalf("status=%v integration=%v herdr=%v state=%v", cleanup.statusCalls, cleanup.removed, cleanup.herdrRemoved, remover.calls)
	}
	wantPrefix := []string{"status:/managed/integration/run-184", "find:/home/operator/.herdr/worktrees/builder-184", "status:/home/operator/.herdr/worktrees/builder-184", "find:/home/operator/.herdr/worktrees/reviewer-184", "status:/home/operator/.herdr/worktrees/reviewer-184"}
	for i, want := range wantPrefix {
		if cleanup.events[i] != want {
			t.Fatalf("events=%v, want preflight event %q at %d", cleanup.events, want, i)
		}
	}
	if len(cleanup.findCalls) != 2 || cleanup.findCalls[0] != (findWorktreeCall{cwd: "/repo", path: "/home/operator/.herdr/worktrees/builder-184", label: ""}) || cleanup.findCalls[1] != (findWorktreeCall{cwd: "/repo", path: "/home/operator/.herdr/worktrees/reviewer-184", label: ""}) {
		t.Fatalf("find calls=%v", cleanup.findCalls)
	}
}

func TestCleanupUsesTrustedManagedRootNotSnapshotParent(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/snapshot-derived/run-184",
		Integration: state.WorktreeState{Path: "/snapshot-derived/run-184"},
	}}
	cleanup := &fakeCleanup{statuses: map[string]string{}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/trusted/worktrees")

	if err := service.Cleanup(context.Background(), "run-184"); err == nil || !strings.Contains(err.Error(), "관리 대상 루트 밖") {
		t.Fatalf("err=%v", err)
	}
	if len(cleanup.statusCalls) != 0 || len(cleanup.removed) != 0 || len(remover.calls) != 0 {
		t.Fatalf("cleanup mutated: status=%v removed=%v state=%v", cleanup.statusCalls, cleanup.removed, remover.calls)
	}
}

func TestCleanupRejectsMismatchedHerdrIdentityBeforeStatusOrRemoval(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration/run-184",
		Integration:     state.WorktreeState{Path: "/managed/integration/run-184"},
		BuilderWorktree: state.WorktreeState{Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-builder-184", PaneID: "pane-builder-184"},
	}}
	found := validHerdrCleanupIdentities()
	found["/home/operator/.herdr/worktrees/builder-184"] = herdr.Worktree{Path: "/home/operator/.herdr/worktrees/builder-184", WorkspaceID: "workspace-other", PaneID: "pane-builder-184"}
	cleanup := &fakeCleanup{statuses: map[string]string{}, foundWorktrees: found}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed")

	if err := service.Cleanup(context.Background(), "run-184"); err == nil || !strings.Contains(err.Error(), "식별자") {
		t.Fatalf("err=%v", err)
	}
	if len(cleanup.statusCalls) != 1 || cleanup.statusCalls[0] != "/managed/integration/run-184" || len(cleanup.removed) != 0 || len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("cleanup mutated: status=%v removed=%v herdr=%v state=%v", cleanup.statusCalls, cleanup.removed, cleanup.herdrRemoved, remover.calls)
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
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed")

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
	cleanup := &fakeCleanup{statuses: map[string]string{}, foundWorktrees: validHerdrCleanupIdentities(), herdrRemoveErr: errors.New("raw Herdr stderr token=secret")}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed")

	err := service.Cleanup(context.Background(), "run-184")
	if err == nil || !strings.Contains(err.Error(), "Worktree 정리") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err=%v", err)
	}
	if len(cleanup.statusCalls) != 3 || len(cleanup.events) < 5 || cleanup.events[4] != "status:/home/operator/.herdr/worktrees/reviewer-184" {
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

func TestRunStateRemoverRejectsUnsafeRunIDs(t *testing.T) {
	for _, id := range []contract.RunID{"", ".", "..", "/absolute", "nested/run", `nested\run`} {
		t.Run(string(id), func(t *testing.T) {
			root := newStateRoot(t)
			if err := NewRunStateRemover(root).Remove(context.Background(), id); err == nil {
				t.Fatalf("run ID %q was accepted", id)
			}
		})
	}
}

func TestRunStateRemoverRejectsRootAndRunsSymlinks(t *testing.T) {
	root := newStateRoot(t)
	linkedRoot := filepath.Join(t.TempDir(), "linked-state")
	if err := os.Symlink(root, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if err := NewRunStateRemover(linkedRoot).Remove(context.Background(), "run-184"); err == nil {
		t.Fatal("intermediate root symlink was accepted")
	}

	runsTarget := filepath.Join(t.TempDir(), "runs-target")
	if err := os.MkdirAll(runsTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(root, "runs")
	if err := os.Remove(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(runsTarget, runs); err != nil {
		t.Fatal(err)
	}
	if err := NewRunStateRemover(root).Remove(context.Background(), "run-184"); err == nil {
		t.Fatal("runs symlink was accepted")
	}
}

func TestRunStateRemoverRejectsTargetSymlink(t *testing.T) {
	root := newStateRoot(t)
	external := filepath.Join(t.TempDir(), "external-run")
	if err := os.MkdirAll(external, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "runs", "run-184")
	if err := os.Symlink(external, target); err != nil {
		t.Fatal(err)
	}
	if err := NewRunStateRemover(root).Remove(context.Background(), "run-184"); err == nil {
		t.Fatal("target symlink was accepted")
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("external target changed: %v", err)
	}
}

func TestRunStateRemoverRemovesOnlyValidDirectChild(t *testing.T) {
	root := newStateRoot(t)
	runs := filepath.Join(root, "runs")
	target := filepath.Join(runs, "run-184")
	sibling := filepath.Join(runs, "run-185")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := NewRunStateRemover(root).Remove(context.Background(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("sibling was removed: %v", err)
	}
}

func newStateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

var _ io.Writer = (*bytes.Buffer)(nil)
