package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

func TestBeginRetirementPersistsOrderedPlanAndRetiringPhase(t *testing.T) {
	h := newHarness(t)
	snapshot := state.RunSnapshot{
		RunID:        "completed-run",
		ContractPath: h.contractPath,
		Strategy:     "parallel",
		Phase:        contract.PhaseCompleted,
		FinalSHA:     validSHA,
		Reviewer:     state.AgentEvidence{Name: "reviewer"},
		ReviewerWorktree: state.WorktreeState{
			WorkspaceID: "review", PaneID: "review:pane", Path: "/managed/review", Branch: "agent/review",
		},
		TaskOrder: []string{"alpha", "beta"},
		Tasks: map[string]state.TaskRunState{
			"alpha": {Agent: state.AgentEvidence{Name: "alpha", CommitSHA: validSHA}, Worktree: state.WorktreeState{WorkspaceID: "alpha", PaneID: "alpha:pane", Path: "/managed/alpha", Branch: "agent/alpha"}},
			"beta":  {Agent: state.AgentEvidence{Name: "beta", CommitSHA: validSHA}, Worktree: state.WorktreeState{WorkspaceID: "beta", PaneID: "beta:pane", Path: "/managed/beta", Branch: "agent/beta"}},
		},
		UpdatedAt: time.Now(),
	}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}

	if err := h.orchestrator.BeginRetirement(context.Background(), snapshot.RunID, contract.PhaseCompleted, true); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(snapshot.RunID)
	if got.Phase != contract.PhaseRetiring || got.Retirement.Status != "pending" || !got.Retirement.Automatic {
		t.Fatalf("snapshot=%#v", got)
	}
	if got.Retirement.Targets[0].Key != "reviewer" || got.Retirement.Targets[1].Key != "builder:beta" || got.Retirement.Targets[2].Key != "builder:alpha" {
		t.Fatalf("targets=%#v", got.Retirement.Targets)
	}
}

type retirementTestHerdr struct {
	*fakeHerdr
	workspaces    map[string]herdr.WorkspaceInfo
	reads         int
	closes        []string
	removeOnClose bool
}

func (h *retirementTestHerdr) GetInfo(ctx context.Context, name string) (herdr.AgentInfo, error) {
	info, err := h.fakeHerdr.GetInfo(ctx, name)
	if err != nil {
		return info, err
	}
	info.Name, info.SessionID = name, "session-"+name
	info.WorkspaceID, info.PaneID, info.Path = "builder", "builder:pane", "/managed/builder"
	info.State = herdr.AgentStateDone
	return info, nil
}

func (h *retirementTestHerdr) GetWorkspace(_ context.Context, id string) (herdr.WorkspaceInfo, bool, error) {
	h.reads++
	info, ok := h.workspaces[id]
	return info, ok, nil
}

func (h *retirementTestHerdr) CloseWorkspace(_ context.Context, id string) error {
	h.closes = append(h.closes, id)
	if h.removeOnClose {
		delete(h.workspaces, id)
	}
	return nil
}

type retirementTestGit struct {
	*fakeGit
	inspects int
}

func (g *retirementTestGit) InspectRetirementTarget(_ context.Context, _, path, branch, sha string) (worktree.RetirementProof, error) {
	g.inspects++
	return worktree.RetirementProof{RepositoryCommonDir: "/repo/.git", Path: path, Branch: branch, HeadSHA: sha}, nil
}

func TestRetirementAdvancesOneExternalActionAndReconcilesMissingClose(t *testing.T) {
	h := newHarness(t)
	hd := &retirementTestHerdr{fakeHerdr: h.herdr, workspaces: map[string]herdr.WorkspaceInfo{
		"builder": {WorkspaceID: "builder", RootPaneID: "builder:pane", Path: "/managed/builder", State: herdr.AgentStateDone},
	}, removeOnClose: true}
	git := &retirementTestGit{fakeGit: h.git}
	h.Deps.Herdr, h.Deps.Git, h.Deps.Worktree = hd, git, git
	h.orchestrator = New(h.Deps)
	snapshot := state.RunSnapshot{
		RunID: "retire-one", ContractPath: h.contractPath, Phase: contract.PhaseCompleted, RepositoryPath: "/repo",
		Builder:         state.AgentEvidence{Name: "builder", SessionID: "session-builder", CommitSHA: validSHA},
		BuilderWorktree: state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"},
	}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.BeginRetirement(context.Background(), snapshot.RunID, contract.PhaseCompleted, false); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 6; step++ {
		beforeAgent, beforeGit, beforeReads, beforeClose := hd.getInfoCalls, git.inspects, hd.reads, len(hd.closes)
		err := h.orchestrator.Advance(context.Background(), snapshot.RunID)
		if err != nil && !errors.Is(err, ErrRunFinished) {
			t.Fatal(err)
		}
		calls := (hd.getInfoCalls - beforeAgent) + (git.inspects - beforeGit) + (hd.reads - beforeReads) + (len(hd.closes) - beforeClose)
		if calls > 1 {
			t.Fatalf("advance %d made %d external calls", step, calls)
		}
	}
	got := h.mustLoad(snapshot.RunID)
	if got.Phase != contract.PhaseCompleted || got.Retirement.Status != "retired" {
		t.Fatalf("snapshot=%#v", got)
	}
	if !reflect.DeepEqual(hd.closes, []string{"builder"}) {
		t.Fatalf("closes=%v", hd.closes)
	}
}

func TestParallelTerminalHookStartsAutomaticRetirementBeforeCompletion(t *testing.T) {
	h := newHarness(t)
	h.Deps.AutoRetireCompletedSessions = true
	h.orchestrator = NewParallel(h.Deps)
	snapshot := state.RunSnapshot{RunID: "auto-terminal", ContractPath: h.contractPath, Phase: contract.PhaseMerging, MergeSHA: validSHA, PullRequestMerged: true}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	loaded := h.mustLoad(snapshot.RunID)
	if err := h.orchestrator.finishParallelMainMerge(context.Background(), &loaded); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(snapshot.RunID); got.Phase != contract.PhaseRetiring || got.Retirement.Status != "pending" {
		t.Fatalf("terminal snapshot=%#v", got)
	}
	if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(snapshot.RunID); got.Phase != contract.PhaseCompleted || got.Retirement.Status != "retired" {
		t.Fatalf("retired snapshot=%#v", got)
	}
}

func TestParallelTerminalHookKeepsSessionsActiveWhenAutoRetirementDisabled(t *testing.T) {
	h := newHarness(t)
	h.orchestrator = NewParallel(h.Deps)
	snapshot := state.RunSnapshot{RunID: "manual-terminal", ContractPath: h.contractPath, Phase: contract.PhaseMerging, MergeSHA: validSHA, PullRequestMerged: true}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	loaded := h.mustLoad(snapshot.RunID)
	if err := h.orchestrator.finishParallelMainMerge(context.Background(), &loaded); err != nil {
		t.Fatal(err)
	}
	if got := h.mustLoad(snapshot.RunID); got.Phase != contract.PhaseCompleted || got.Retirement.Status != "active" {
		t.Fatalf("terminal snapshot=%#v", got)
	}
}
