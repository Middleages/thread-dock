package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

func TestParallelMapOnlyStatusEnumeratesEverySessionLifecycle(t *testing.T) {
	snapshot := state.RunSnapshot{
		RunID: "parallel-map-only", Strategy: "parallel", Phase: contract.PhaseRetiring, Summary: "retiring",
		Reviewer: state.AgentEvidence{Name: "reviewer"}, ReviewerWorktree: state.WorktreeState{Path: "/managed/integration"},
		IntegrationPath: "/managed/integration", Integration: state.WorktreeState{Path: "/managed/integration"},
		Tasks: map[string]state.TaskRunState{
			"zeta":  {State: "completed", Agent: state.AgentEvidence{Name: "builder-zeta"}, Worktree: state.WorktreeState{Path: "/herdr/zeta"}},
			"alpha": {State: "running", Agent: state.AgentEvidence{Name: "builder-alpha"}, Worktree: state.WorktreeState{Path: "/herdr/alpha"}},
		},
		Retirement: state.RetirementState{Status: "pending", Targets: []state.RetirementTarget{
			{Role: "reviewer", Path: "/managed/integration", Status: "retired"},
			{Role: "builder", TaskID: "alpha", Path: "/herdr/alpha", Status: "pending"},
			{Role: "builder", TaskID: "zeta", Path: "/herdr/zeta", Status: "retired"},
		}},
		UpdatedAt: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC),
	}
	view := statusView(snapshot)
	if len(view.Agents) != 3 {
		t.Fatalf("agents=%#v", view.Agents)
	}
	want := map[string]string{"reviewer": "retired", "builder-alpha": "retiring", "builder-zeta": "retired"}
	for _, agent := range view.Agents {
		if want[agent.Name] != agent.Lifecycle {
			t.Fatalf("agent=%#v want lifecycle=%q", agent, want[agent.Name])
		}
		delete(want, agent.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing agents=%v", want)
	}
	var jsonOut bytes.Buffer
	if err := writeStatusJSON(&jsonOut, view); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Agents []struct {
			Name      string `json:"name"`
			Lifecycle string `json:"lifecycle"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Agents) != 3 {
		t.Fatalf("json agents=%#v", wire.Agents)
	}
	var human bytes.Buffer
	writeHumanStatus(&human, view)
	for name, lifecycle := range map[string]string{"reviewer": "retired", "builder-alpha": "retiring", "builder-zeta": "retired"} {
		if !strings.Contains(human.String(), "세션 "+name+": "+lifecycle) {
			t.Fatalf("human=%q missing %s=%s", human.String(), name, lifecycle)
		}
	}
}

func TestParallelMapOnlyCleanupPreflightsAndRemovesEveryBuilder(t *testing.T) {
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	snapshot := round1OldSnapshot()
	snapshot.RunID, snapshot.Strategy, snapshot.UpdatedAt = "parallel-cleanup", "parallel", now.Add(-8*24*time.Hour)
	snapshot.Tasks = map[string]state.TaskRunState{
		"zeta":  {State: "completed", Agent: state.AgentEvidence{Name: "builder-zeta"}, Worktree: state.WorktreeState{Path: "/herdr/zeta", WorkspaceID: "zeta-ws", PaneID: "zeta-pane"}},
		"alpha": {State: "completed", Agent: state.AgentEvidence{Name: "builder-alpha"}, Worktree: state.WorktreeState{Path: "/herdr/alpha", WorkspaceID: "alpha-ws", PaneID: "alpha-pane"}},
	}
	cleanup := &round1Cleanup{identities: map[string]herdr.Worktree{
		"/herdr/alpha": {Path: "/herdr/alpha", WorkspaceID: "alpha-ws", PaneID: "alpha-pane"},
		"/herdr/zeta":  {Path: "/herdr/zeta", WorkspaceID: "zeta-ws", PaneID: "zeta-pane"},
	}}
	store := &fakeStateStore{snapshot: snapshot}
	remover := &fakeStateRemover{}
	service := NewOrchestratorRunService(nil, store, cleanup, remover, func() time.Time { return now }, "/managed", "/herdr")
	if err := service.Cleanup(context.Background(), snapshot.RunID); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.herdrRemoved) != 2 || len(remover.calls) != 1 {
		t.Fatalf("cleanup=%#v state=%v", cleanup, remover.calls)
	}
}
