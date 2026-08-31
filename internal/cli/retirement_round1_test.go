package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

type round1Cleanup struct {
	inspectErrByPath map[string]error
	identities       map[string]herdr.Worktree
	inspected        []string
	retiredRemoved   []string
	herdrRemoved     []string
	stateRemoved     bool
}

func (c *round1Cleanup) Status(context.Context, string) (string, error)           { return "", nil }
func (c *round1Cleanup) RemoveSafe(context.Context, string, string, string) error { return nil }
func (c *round1Cleanup) RemoveHerdr(_ context.Context, id string) error {
	c.herdrRemoved = append(c.herdrRemoved, id)
	return nil
}
func (c *round1Cleanup) FindWorktree(_ context.Context, _ string, path, _ string) (herdr.Worktree, bool, error) {
	if c.identities != nil {
		identity, ok := c.identities[path]
		return identity, ok, nil
	}
	return herdr.Worktree{Path: path, WorkspaceID: filepath.Base(path) + "-workspace", PaneID: filepath.Base(path) + "-pane"}, true, nil
}
func (c *round1Cleanup) InspectRetired(_ context.Context, _ string, _ string, target state.RetirementTarget) error {
	c.inspected = append(c.inspected, target.Path)
	if c.inspectErrByPath != nil {
		return c.inspectErrByPath[target.Path]
	}
	return nil
}
func (c *round1Cleanup) RemoveRetired(_ context.Context, _ string, _ string, target state.RetirementTarget) error {
	c.retiredRemoved = append(c.retiredRemoved, target.Path)
	return nil
}

func round1OldSnapshot() state.RunSnapshot {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	return state.RunSnapshot{
		RunID: "old", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour),
		RepositoryPath: "/repo", IntegrationPath: "/managed/integration",
		Integration: state.WorktreeState{Path: "/managed/integration", Branch: "agent/integration"},
	}
}

func TestCleanupRejectsParallelSnapshotWithOmittedBuilderTarget(t *testing.T) {
	snapshot := round1OldSnapshot()
	snapshot.Strategy = "parallel"
	snapshot.TaskOrder = []string{"alpha", "beta"}
	snapshot.Tasks = map[string]state.TaskRunState{
		"alpha": {Agent: state.AgentEvidence{Name: "builder-alpha"}, Worktree: state.WorktreeState{Path: "/herdr/alpha", WorkspaceID: "alpha-workspace", PaneID: "alpha-pane"}},
		"beta":  {Agent: state.AgentEvidence{Name: "builder-beta"}},
	}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed", "/herdr")

	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted an omitted parallel builder target")
	}
	if len(remover.calls) != 0 || len(cleanup.herdrRemoved) != 0 || len(cleanup.retiredRemoved) != 0 {
		t.Fatalf("cleanup mutated after omitted target: remover=%v cleanup=%#v", remover.calls, cleanup)
	}
}

func retiredRound1Target(path, role, workspace string) state.RetirementTarget {
	return state.RetirementTarget{
		Key: role + ":" + path, Role: role, WorkspaceID: workspace, PaneID: workspace + "-pane",
		Path: path, Branch: "agent/" + role, HeadSHA: "0123456789abcdef0123456789abcdef01234567",
		Status: "retired", RepositoryCommonDir: "/repo/.git",
	}
}

func TestCleanupRequiresRetiredInspectorBeforeAnyRemoval(t *testing.T) {
	snapshot := round1OldSnapshot()
	snapshot.Retirement.Targets = []state.RetirementTarget{retiredRound1Target("/herdr/retired", "builder", "retired-workspace")}
	store := &fakeStateStore{snapshot: snapshot}
	// RemoveRetired is present, but the read-only proof seam is deliberately
	// absent. The cleanup must reject before status or removal.
	cleanup := &retiredOnlyRound1Cleanup{}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed", "/herdr")

	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted retired targets without an inspector")
	}
	if len(cleanup.removed) != 0 || len(remover.calls) != 0 {
		t.Fatalf("retired cleanup mutated without inspector: %#v state=%v", cleanup, remover.calls)
	}
}

type retiredOnlyRound1Cleanup struct{ removed []string }

func (c *retiredOnlyRound1Cleanup) Status(context.Context, string) (string, error) { return "", nil }
func (c *retiredOnlyRound1Cleanup) RemoveSafe(context.Context, string, string, string) error {
	return nil
}
func (c *retiredOnlyRound1Cleanup) RemoveRetired(_ context.Context, _, _ string, target state.RetirementTarget) error {
	c.removed = append(c.removed, target.Path)
	return nil
}

func TestCleanupRejectsActiveBuilderOutsideHerdrRoot(t *testing.T) {
	snapshot := round1OldSnapshot()
	snapshot.BuilderWorktree = state.WorktreeState{Path: "/outside/builder", WorkspaceID: "builder-workspace", PaneID: "builder-pane"}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{identities: map[string]herdr.Worktree{"/outside/builder": {Path: "/outside/builder", WorkspaceID: "builder-workspace", PaneID: "builder-pane"}}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed", "/herdr")

	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted active builder outside Herdr root")
	}
	if len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("outside builder mutated: cleanup=%#v state=%v", cleanup, remover.calls)
	}
}

func TestCleanupRejectsConflictingSharedWorktreeOwners(t *testing.T) {
	snapshot := round1OldSnapshot()
	snapshot.BuilderWorktree = state.WorktreeState{Path: "/herdr/shared", WorkspaceID: "shared-builder-workspace", PaneID: "shared-builder-pane", Branch: "agent/builder"}
	snapshot.ReviewerWorktree = state.WorktreeState{Path: "/herdr/shared", WorkspaceID: "shared-reviewer-workspace", PaneID: "shared-reviewer-pane", Branch: "agent/reviewer"}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{identities: map[string]herdr.Worktree{"/herdr/shared": {Path: "/herdr/shared", WorkspaceID: "shared-builder-workspace", PaneID: "shared-builder-pane"}}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed", "/herdr")

	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted conflicting shared owners")
	}
	if len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("conflicting owners mutated: cleanup=%#v state=%v", cleanup, remover.calls)
	}
}

func TestCleanupRunsAllRetiredProofsBeforeRemovingAny(t *testing.T) {
	snapshot := round1OldSnapshot()
	snapshot.Retirement.Targets = []state.RetirementTarget{
		retiredRound1Target("/herdr/alpha", "builder", "alpha-workspace"),
		retiredRound1Target("/herdr/beta", "builder", "beta-workspace"),
	}
	store := &fakeStateStore{snapshot: snapshot}
	cleanup := &round1Cleanup{inspectErrByPath: map[string]error{"/herdr/beta": errors.New("dirty")}}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC) }, "/managed", "/herdr")

	if err := service.Cleanup(context.Background(), snapshot.RunID); err == nil {
		t.Fatal("cleanup accepted a failed later retired proof")
	}
	if len(cleanup.retiredRemoved) != 0 || len(cleanup.herdrRemoved) != 0 || len(remover.calls) != 0 {
		t.Fatalf("partial cleanup occurred after proof failure: cleanup=%#v state=%v", cleanup, remover.calls)
	}
}

func TestStatusDistinguishesPerSessionLifecycle(t *testing.T) {
	snapshot := state.RunSnapshot{
		RunID: "status-retirement", Phase: contract.PhaseRetiring, Summary: "retiring",
		Builder: state.AgentEvidence{Name: "builder-alpha"}, BuilderWorktree: state.WorktreeState{Path: "/herdr/alpha"},
		Reviewer: state.AgentEvidence{Name: "reviewer"}, ReviewerWorktree: state.WorktreeState{Path: "/managed/integration"},
		IntegrationPath: "/managed/integration", Integration: state.WorktreeState{Path: "/managed/integration"},
		Retirement: state.RetirementState{Status: "pending", Targets: []state.RetirementTarget{
			{Role: "builder", Path: "/herdr/alpha", Status: "retired"},
			{Role: "reviewer", Path: "/managed/integration", Status: "pending"},
		}},
	}
	view := statusView(snapshot)
	if len(view.Agents) != 2 || view.Agents[0].Lifecycle != "retired" || view.Agents[1].Lifecycle != "retiring" {
		t.Fatalf("view=%#v", view)
	}
	var out bytes.Buffer
	if err := writeStatusJSON(&out, view); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(out.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Agents[0]["lifecycle"] != "retired" || wire.Agents[1]["lifecycle"] != "retiring" {
		t.Fatalf("agents=%v", wire.Agents)
	}
	var human bytes.Buffer
	writeHumanStatus(&human, view)
	if !strings.Contains(human.String(), "세션 builder-alpha: retired") || !strings.Contains(human.String(), "세션 reviewer: retiring") {
		t.Fatalf("human=%q", human.String())
	}
}

func TestResumeRetiringAdvancesOneAction(t *testing.T) {
	store := &fakeStateStore{snapshot: state.RunSnapshot{RunID: "resume-retiring", Phase: contract.PhaseRetiring, Retirement: state.RetirementState{Status: "pending", TargetPhase: contract.PhaseCompleted}}}
	coordinator := &fakeCoordinator{}
	service := NewOrchestratorRunService(coordinator, store, nil, nil, nil, "/managed")
	if err := service.Resume(context.Background(), "resume-retiring"); err != nil {
		t.Fatal(err)
	}
	if coordinator.advanceCalls != 1 {
		t.Fatalf("advance calls=%d", coordinator.advanceCalls)
	}
}
