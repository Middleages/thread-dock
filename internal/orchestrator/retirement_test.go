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
	infoName      string
	infoSession   string
}

func (h *retirementTestHerdr) GetInfo(ctx context.Context, name string) (herdr.AgentInfo, error) {
	info, err := h.fakeHerdr.GetInfo(ctx, name)
	if err != nil {
		return info, err
	}
	info.Name, info.SessionID = name, "session-"+name
	if h.infoName != "" {
		info.Name = h.infoName
	}
	if h.infoSession != "" {
		info.SessionID = h.infoSession
	}
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

type retirementRecordingStore struct {
	base                *state.Store
	saves               []state.RunSnapshot
	failSessionsRetired bool
	failTerminalSave    bool
}

func (s *retirementRecordingStore) Create(ctx context.Context, snapshot state.RunSnapshot) error {
	return s.base.Create(ctx, snapshot)
}

func (s *retirementRecordingStore) Load(ctx context.Context, id contract.RunID) (state.RunSnapshot, error) {
	return s.base.Load(ctx, id)
}

func (s *retirementRecordingStore) Save(ctx context.Context, snapshot state.RunSnapshot) error {
	if s.failTerminalSave && snapshot.Phase == contract.PhaseCompleted && snapshot.Retirement.Status == "retired" {
		return errors.New("terminal save failed")
	}
	s.saves = append(s.saves, snapshot)
	return s.base.Save(ctx, snapshot)
}

func (s *retirementRecordingStore) Append(ctx context.Context, id contract.RunID, event state.Event) error {
	if s.failSessionsRetired && event.Type == "sessions_retired" {
		return errors.New("sessions-retired audit append failed")
	}
	return s.base.Append(ctx, id, event)
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

func TestProjectDoneHookPersistsRetirementWithoutCompletedIntermediateSave(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.AutoRetireCompletedSessions = true
	recorder := &retirementRecordingStore{base: h.store}
	h.Deps.Store = recorder
	h.orchestrator = NewParallel(h.Deps)
	snapshot := state.RunSnapshot{
		RunID: "project-done-auto", ContractPath: h.contractPath, Phase: contract.PhaseMerging,
		Registration: state.RegistrationState{NodeID: "issue-node"}, PullRequestMerged: true,
		Reviewer: state.AgentEvidence{Name: "reviewer"}, FinalSHA: validSHA,
		ReviewerWorktree: state.WorktreeState{WorkspaceID: "review", PaneID: "review:pane", Path: "/managed/review", Branch: "agent/review"},
		Builder:          state.AgentEvidence{Name: "builder", CommitSHA: validSHA},
		BuilderWorktree:  state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"},
	}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.parallelGH.projectItemID, h.parallelGH.projectStatus = "item-1", "Done"
	loaded := snapshot
	if err := h.orchestrator.reconcileProjectObservation(context.Background(), &loaded, nil, "Done"); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(snapshot.RunID)
	if got.Phase != contract.PhaseRetiring || got.Retirement.Status != "pending" {
		t.Fatalf("snapshot=%#v", got)
	}
	for _, saved := range recorder.saves {
		if saved.Phase == contract.PhaseCompleted {
			t.Fatalf("completed snapshot was saved before retirement: %#v", saved)
		}
		if saved.UpdatedAt.IsZero() {
			t.Fatalf("retirement save omitted UpdatedAt: %#v", saved)
		}
	}
}

func TestProjectDoneResponseLossReconcilesIntoRetirementAtomically(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.AutoRetireCompletedSessions = true
	recorder := &retirementRecordingStore{base: h.store}
	h.Deps.Store = recorder
	h.orchestrator = NewParallel(h.Deps)
	snapshot := state.RunSnapshot{
		RunID: "project-done-response-loss", ContractPath: h.contractPath, Phase: contract.PhaseMerging,
		Registration: state.RegistrationState{NodeID: "issue-node"}, PendingAction: "parallel_project_observe_done", PullRequestMerged: true,
		Reviewer: state.AgentEvidence{Name: "reviewer"}, FinalSHA: validSHA,
		ReviewerWorktree: state.WorktreeState{WorkspaceID: "review", PaneID: "review:pane", Path: "/managed/review", Branch: "agent/review"},
		Builder:          state.AgentEvidence{Name: "builder", CommitSHA: validSHA},
		BuilderWorktree:  state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"},
	}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	h.parallelGH.projectItemID, h.parallelGH.projectStatus = "item-1", "Done"
	loaded := snapshot
	if err := h.orchestrator.reconcileParallelPending(context.Background(), &loaded, nil); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(snapshot.RunID)
	if got.Phase != contract.PhaseRetiring || got.Retirement.Status != "pending" {
		t.Fatalf("snapshot=%#v", got)
	}
	for _, saved := range recorder.saves {
		if saved.Phase == contract.PhaseCompleted {
			t.Fatalf("completed snapshot was saved before retirement: %#v", saved)
		}
	}
}

func TestRetirementAuditAppendFailureNeverPersistsTerminalPhase(t *testing.T) {
	h := newHarness(t)
	hd := &retirementTestHerdr{fakeHerdr: h.herdr, workspaces: map[string]herdr.WorkspaceInfo{}, removeOnClose: true}
	git := &retirementTestGit{fakeGit: h.git}
	recorder := &retirementRecordingStore{base: h.store, failSessionsRetired: true}
	h.Deps.Store, h.Deps.Herdr, h.Deps.Git, h.Deps.Worktree = recorder, hd, git, git
	h.orchestrator = New(h.Deps)
	snapshot := state.RunSnapshot{RunID: "audit-failure", ContractPath: h.contractPath, Phase: contract.PhaseCompleted, RepositoryPath: "/repo", Builder: state.AgentEvidence{Name: "builder", CommitSHA: validSHA}, BuilderWorktree: state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"}}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.BeginRetirement(context.Background(), snapshot.RunID, contract.PhaseCompleted, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("retirement audit append unexpectedly succeeded")
	}
	got := h.mustLoad(snapshot.RunID)
	if got.Phase != contract.PhaseRetiring || got.Retirement.Status == "retired" {
		t.Fatalf("audit failure terminalized snapshot=%#v", got)
	}
	recorder.failSessionsRetired = false
	if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
		t.Fatal(err)
	}
	if got = h.mustLoad(snapshot.RunID); got.Phase != contract.PhaseCompleted || got.Retirement.Status != "retired" {
		t.Fatalf("retry snapshot=%#v", got)
	}
	events := h.events(snapshot.RunID)
	foundAudit := false
	for _, event := range events {
		if event.Type == "sessions_retired" {
			foundAudit = true
			if event.Phase != contract.PhaseRetiring || event.Kind != "sessions_retired" {
				t.Fatalf("retirement audit=%#v", event)
			}
		}
	}
	if !foundAudit {
		t.Fatal("sessions_retired audit is missing")
	}
}

func TestRetirementWorkspaceLifecycleChangeBlocksBeforeClose(t *testing.T) {
	h := newHarness(t)
	hd := &retirementTestHerdr{fakeHerdr: h.herdr, workspaces: map[string]herdr.WorkspaceInfo{
		"builder": {WorkspaceID: "builder", RootPaneID: "builder:pane", Path: "/managed/builder", State: herdr.AgentStateWorking},
	}, removeOnClose: true}
	git := &retirementTestGit{fakeGit: h.git}
	h.Deps.Herdr, h.Deps.Git, h.Deps.Worktree = hd, git, git
	h.orchestrator = New(h.Deps)
	snapshot := state.RunSnapshot{RunID: "retire-state-change", ContractPath: h.contractPath, Phase: contract.PhaseCompleted, RepositoryPath: "/repo", Builder: state.AgentEvidence{Name: "builder", CommitSHA: validSHA}, BuilderWorktree: state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"}}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.BeginRetirement(context.Background(), snapshot.RunID, contract.PhaseCompleted, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
			t.Fatal(err)
		}
	}
	got := h.mustLoad(snapshot.RunID)
	if got.Retirement.Status != "needs_operator" || len(hd.closes) != 0 {
		t.Fatalf("snapshot=%#v closes=%v", got, hd.closes)
	}
}

func TestRetirementAgentIdentityRequiresByteExactCanonicalValues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*retirementTestHerdr)
	}{
		{name: "name", configure: func(h *retirementTestHerdr) { h.infoName = " builder" }},
		{name: "session", configure: func(h *retirementTestHerdr) { h.infoSession = "session-other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			hd := &retirementTestHerdr{fakeHerdr: h.herdr, workspaces: map[string]herdr.WorkspaceInfo{"builder": {WorkspaceID: "builder", RootPaneID: "builder:pane", Path: "/managed/builder", State: herdr.AgentStateDone}}}
			tc.configure(hd)
			git := &retirementTestGit{fakeGit: h.git}
			h.Deps.Herdr, h.Deps.Git, h.Deps.Worktree = hd, git, git
			h.orchestrator = New(h.Deps)
			snapshot := state.RunSnapshot{RunID: contract.RunID("agent-identity-" + tc.name), ContractPath: h.contractPath, Phase: contract.PhaseCompleted, RepositoryPath: "/repo", Builder: state.AgentEvidence{Name: "builder", SessionID: "session-builder", CommitSHA: validSHA}, BuilderWorktree: state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"}}
			if err := h.store.Create(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}
			if err := h.orchestrator.BeginRetirement(context.Background(), snapshot.RunID, contract.PhaseCompleted, true); err != nil {
				t.Fatal(err)
			}
			if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
				t.Fatal(err)
			}
			got := h.mustLoad(snapshot.RunID)
			if got.Retirement.Status != "needs_operator" || len(hd.closes) != 0 {
				t.Fatalf("snapshot=%#v closes=%v", got, hd.closes)
			}
		})
	}
}

func TestCloseResponseLossWithExactWorkspaceClearsIntentBeforeRetry(t *testing.T) {
	h := newHarness(t)
	hd := &retirementTestHerdr{fakeHerdr: h.herdr, workspaces: map[string]herdr.WorkspaceInfo{
		"builder": {WorkspaceID: "builder", RootPaneID: "builder:pane", Path: "/managed/builder", State: herdr.AgentStateDone},
	}, removeOnClose: false}
	git := &retirementTestGit{fakeGit: h.git}
	h.Deps.Herdr, h.Deps.Git, h.Deps.Worktree = hd, git, git
	h.orchestrator = New(h.Deps)
	snapshot := state.RunSnapshot{RunID: "close-response-loss", ContractPath: h.contractPath, Phase: contract.PhaseCompleted, RepositoryPath: "/repo", Builder: state.AgentEvidence{Name: "builder", CommitSHA: validSHA}, BuilderWorktree: state.WorktreeState{WorkspaceID: "builder", PaneID: "builder:pane", Path: "/managed/builder", Branch: "agent/builder"}}
	if err := h.store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.BeginRetirement(context.Background(), snapshot.RunID, contract.PhaseCompleted, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hd.closes, []string{"builder"}) {
		t.Fatalf("initial close calls=%v", hd.closes)
	}
	if err := h.orchestrator.Advance(context.Background(), snapshot.RunID); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(snapshot.RunID)
	if got.PendingAction != "" || got.Retirement.Targets[0].Status != "workspace_observed" || len(hd.closes) != 1 {
		t.Fatalf("reconciled snapshot=%#v closes=%v", got, hd.closes)
	}
}
