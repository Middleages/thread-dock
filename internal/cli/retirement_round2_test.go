package cli

import (
	"context"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

func TestCleanupSharedReviewerUsesFinalSHAOverStaleIntegrationSHA(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	snapshot := state.RunSnapshot{
		RunID: "final-sha", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", Strategy: "parallel", IntegrationPath: "/managed/integration",
		Integration:      state.WorktreeState{Path: "/managed/integration", Branch: "agent/integration"},
		IntegrationSHA:   "0123456789abcdef0123456789abcdef01234567",
		FinalSHA:         "abcdef0123456789abcdef0123456789abcdef01",
		ReviewerWorktree: state.WorktreeState{Path: "/managed/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane", Branch: "agent/integration"},
		Retirement: state.RetirementState{Targets: []state.RetirementTarget{{
			Key: "reviewer", Role: "reviewer", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane",
			Path: "/managed/integration", Branch: "agent/integration", HeadSHA: "abcdef0123456789abcdef0123456789abcdef01", Status: "active",
		}}},
	}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{identities: map[string]herdr.Worktree{"/managed/integration": {Path: "/managed/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane"}}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed", "/herdr")
	if err := service.Cleanup(context.Background(), snapshot.RunID); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.herdrRemoved) != 1 || cleanup.herdrRemoved[0] != "reviewer-ws" || len(remover.calls) != 1 {
		t.Fatalf("cleanup=%#v state=%v", cleanup, remover.calls)
	}
}

func TestCleanupRejectsActiveBuilderWhenHerdrRootIsEmptyBeforeProviderCalls(t *testing.T) {
	snapshot := round1OldSnapshot()
	snapshot.BuilderWorktree = state.WorktreeState{Path: "/herdr/builder", WorkspaceID: "builder-ws", PaneID: "builder-pane"}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{identities: map[string]herdr.Worktree{"/herdr/builder": {Path: "/herdr/builder", WorkspaceID: "builder-ws", PaneID: "builder-pane"}}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed")
	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted active builder without Herdr root")
	}
	if len(cleanup.inspected) != 0 || len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("provider or removal call before root guard: cleanup=%#v state=%v", cleanup, remover.calls)
	}
}

func TestRetirePhaseGuardMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		phase   contract.RunPhase
		blocked bool
	}{
		{name: "needs operator", phase: contract.PhaseNeedsOperator},
		{name: "active", phase: contract.PhaseBuilding},
		{name: "paused", phase: contract.PhasePaused},
		{name: "completed with blocked flag", phase: contract.PhaseCompleted, blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStateStore{snapshot: state.RunSnapshot{RunID: "guard", Phase: test.phase}}
			coordinator := &retireCoordinator{store: store}
			service := NewOrchestratorRunService(coordinator, store, nil, nil, nil, "/managed")
			if err := service.Retire(context.Background(), "guard", test.blocked); err == nil {
				t.Fatal("retire accepted guarded phase")
			}
			if coordinator.begin != 0 || coordinator.advances != 0 {
				t.Fatalf("guard called coordinator: begin=%d advances=%d", coordinator.begin, coordinator.advances)
			}
		})
	}
}

func TestCleanupRejectsRetiredDuplicateWithDifferentRepositoryProof(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	snapshot := state.RunSnapshot{
		RunID: "proof-conflict", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration",
		Integration:      state.WorktreeState{Path: "/managed/integration"},
		ReviewerWorktree: state.WorktreeState{Path: "/managed/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane"},
		Retirement: state.RetirementState{Targets: []state.RetirementTarget{
			{Role: "reviewer", Path: "/managed/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane", Branch: "agent/reviewer", HeadSHA: "0123456789abcdef0123456789abcdef01234567", RepositoryCommonDir: "/repo/.git", Status: "retired"},
			{Role: "reviewer", Path: "/managed/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane", Branch: "agent/reviewer", HeadSHA: "0123456789abcdef0123456789abcdef01234567", RepositoryCommonDir: "/other/.git", Status: "retired"},
		}},
	}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{identities: map[string]herdr.Worktree{"/managed/integration": {Path: "/managed/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane"}}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed", "/herdr")
	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted conflicting retired proof")
	}
	if len(cleanup.retiredRemoved) != 0 || len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("conflicting proof mutated cleanup=%#v state=%v", cleanup, remover.calls)
	}
}
