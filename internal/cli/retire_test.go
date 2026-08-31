package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

type fakeRetireService struct {
	calls []struct {
		id      contract.RunID
		blocked bool
	}
	err error
}

func (f *fakeRetireService) Retire(_ context.Context, id contract.RunID, blocked bool) error {
	f.calls = append(f.calls, struct {
		id      contract.RunID
		blocked bool
	}{id: id, blocked: blocked})
	return f.err
}

func TestRetireRequiresBlockedFlagAndRoutesValidRequests(t *testing.T) {
	service := &fakeRetireService{}
	var out, errOut bytes.Buffer
	if code := RunWithDependencies(context.Background(), []string{"retire", "blocked-run", "--wrong"}, &out, &errOut, Dependencies{Retirement: service}); code == 0 {
		t.Fatal("malformed blocked request was accepted")
	}
	if len(service.calls) != 0 {
		t.Fatalf("invalid request called service: %#v", service.calls)
	}
	if code := RunWithDependencies(context.Background(), []string{"retire", "blocked-run", "--blocked"}, &out, &errOut, Dependencies{Retirement: service}); code != 0 {
		t.Fatalf("blocked retire code=%d stderr=%q", code, errOut.String())
	}
	if code := RunWithDependencies(context.Background(), []string{"retire", "completed-run"}, &out, &errOut, Dependencies{Retirement: service}); code != 0 {
		t.Fatalf("completed retire code=%d stderr=%q", code, errOut.String())
	}
	if len(service.calls) != 2 || service.calls[0].id != "blocked-run" || !service.calls[0].blocked || service.calls[1].id != "completed-run" || service.calls[1].blocked {
		t.Fatalf("calls=%#v", service.calls)
	}
}

func TestRetireReportsGenericErrorWithoutProviderOutput(t *testing.T) {
	service := &fakeRetireService{err: errors.New("provider token=secret")}
	var out, errOut bytes.Buffer
	if code := RunWithDependencies(context.Background(), []string{"retire", "run-184"}, &out, &errOut, Dependencies{Retirement: service}); code == 0 {
		t.Fatal("retire unexpectedly succeeded")
	}
	if strings.Contains(errOut.String(), "secret") || strings.Contains(errOut.String(), "provider") {
		t.Fatalf("provider detail leaked: %q", errOut.String())
	}
}

type retireCoordinator struct {
	store    *fakeStateStore
	begin    int
	advances int
}

func (c *retireCoordinator) Start(context.Context, string) (contract.RunID, error) {
	return "", errors.New("unused")
}

func (c *retireCoordinator) Stop(context.Context, contract.RunID) error { return nil }

func (c *retireCoordinator) Advance(_ context.Context, _ contract.RunID) error {
	c.advances++
	snapshot := c.store.snapshot
	snapshot.Retirement.Status = "retired"
	snapshot.Phase = snapshot.Retirement.TargetPhase
	c.store.snapshot = snapshot
	return nil
}

func (c *retireCoordinator) BeginRetirement(_ context.Context, _ contract.RunID, target contract.RunPhase, _ bool) error {
	c.begin++
	snapshot := c.store.snapshot
	snapshot.Phase = contract.PhaseRetiring
	snapshot.Retirement = state.RetirementState{Status: "pending", TargetPhase: target}
	c.store.snapshot = snapshot
	return nil
}

func TestRetireCompletesBoundedLoopAndIsIdempotent(t *testing.T) {
	store := &fakeStateStore{snapshot: state.RunSnapshot{RunID: "completed", Phase: contract.PhaseCompleted}}
	coordinator := &retireCoordinator{store: store}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, nil, "/managed")
	if err := service.Retire(context.Background(), "completed", false); err != nil {
		t.Fatal(err)
	}
	if coordinator.begin != 1 || coordinator.advances != 1 || store.snapshot.Retirement.Status != "retired" {
		t.Fatalf("begin=%d advances=%d snapshot=%#v", coordinator.begin, coordinator.advances, store.snapshot)
	}
	if err := service.Retire(context.Background(), "completed", false); err != nil {
		t.Fatal(err)
	}
	if coordinator.begin != 1 || coordinator.advances != 1 {
		t.Fatalf("idempotent retire called coordinator: begin=%d advances=%d", coordinator.begin, coordinator.advances)
	}
}

func TestRetireRejectsBlockedWithoutFlagBeforeCoordinator(t *testing.T) {
	store := &fakeStateStore{snapshot: state.RunSnapshot{RunID: "blocked", Phase: contract.PhaseBlocked}}
	coordinator := &retireCoordinator{store: store}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, nil, "/managed")
	if err := service.Retire(context.Background(), "blocked", false); err == nil {
		t.Fatal("blocked run retired without --blocked")
	}
	if coordinator.begin != 0 || coordinator.advances != 0 || store.snapshot.Phase != contract.PhaseBlocked {
		t.Fatalf("blocked guard mutated run: begin=%d advances=%d snapshot=%#v", coordinator.begin, coordinator.advances, store.snapshot)
	}
}

type mixedRetirementCleanup struct {
	retiredInspectErr error
	retiredInspected  int
	retiredRemoved    int
	activeRemoved     int
	herdrRemoved      int
	stateRemoved      int
}

func (c *mixedRetirementCleanup) Status(context.Context, string) (string, error) { return "", nil }
func (c *mixedRetirementCleanup) RemoveSafe(context.Context, string, string, string) error {
	c.activeRemoved++
	return nil
}
func (c *mixedRetirementCleanup) RemoveHerdr(context.Context, string) error {
	c.herdrRemoved++
	return nil
}
func (c *mixedRetirementCleanup) FindWorktree(_ context.Context, _ string, path, _ string) (herdr.Worktree, bool, error) {
	return herdr.Worktree{Path: path, WorkspaceID: "builder-ws", PaneID: "builder-pane"}, true, nil
}
func (c *mixedRetirementCleanup) InspectRetired(context.Context, string, string, state.RetirementTarget) error {
	c.retiredInspected++
	return c.retiredInspectErr
}
func (c *mixedRetirementCleanup) RemoveRetired(context.Context, string, string, state.RetirementTarget) error {
	c.retiredRemoved++
	return nil
}

func TestCleanupUsesGitPathForRetiredAndHerdrForActiveTargets(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	store := &fakeStateStore{snapshot: state.RunSnapshot{
		RunID: "old-completed", Phase: contract.PhaseCompleted,
		UpdatedAt: now.Add(-8 * 24 * time.Hour), RepositoryPath: "/repo", IntegrationPath: "/herdr/integration",
		Integration:      state.WorktreeState{Path: "/herdr/integration"},
		BuilderWorktree:  state.WorktreeState{Path: "/herdr/builder", WorkspaceID: "builder-ws", PaneID: "builder-pane"},
		ReviewerWorktree: state.WorktreeState{Path: "/herdr/integration", WorkspaceID: "reviewer-ws", PaneID: "reviewer-pane"},
		Retirement:       state.RetirementState{Targets: []state.RetirementTarget{{Role: "reviewer", Path: "/herdr/integration", Status: "retired", RepositoryCommonDir: "/repo/.git", Branch: "agent/reviewer", HeadSHA: "0123456789abcdef0123456789abcdef01234567"}}},
	}}
	cleanup := &mixedRetirementCleanup{}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed", "/herdr")
	if err := service.Cleanup(context.Background(), "old-completed"); err != nil {
		t.Fatal(err)
	}
	if cleanup.retiredInspected != 1 || cleanup.retiredRemoved != 1 || cleanup.herdrRemoved != 1 || cleanup.activeRemoved != 0 || len(remover.calls) != 1 {
		t.Fatalf("cleanup=%#v state=%v", cleanup, remover.calls)
	}
}
