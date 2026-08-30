package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

func TestAgentNamePreservesShortRunIDNames(t *testing.T) {
	for _, role := range []string{"builder", "reviewer"} {
		if got, want := agentName(role, "run-184"), role+"-run-184"; got != want {
			t.Fatalf("agentName(%q, run-184) = %q, want %q", role, got, want)
		}
	}
}

func TestAgentNameLongRunIDIsStableDistinctAndHerdrCompatible(t *testing.T) {
	longID := contract.RunID("run-1787925632789000929-1")
	otherID := contract.RunID("run-1787925632789000929-2")
	builder := agentName("builder", longID)
	reviewer := agentName("reviewer", longID)

	for role, name := range map[string]string{"builder": builder, "reviewer": reviewer} {
		if len(name) > 32 {
			t.Errorf("%s name length = %d, want <= 32 (%q)", role, len(name), name)
		}
		if !strings.HasPrefix(name, role+"-") {
			t.Errorf("%s name = %q, want role prefix", role, name)
		}
		for _, char := range name {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
				t.Errorf("%s name = %q contains invalid Herdr character %q", role, name, char)
			}
		}
	}
	if builder != agentName("builder", longID) || reviewer != agentName("reviewer", longID) {
		t.Fatalf("agent names are not stable: %q / %q", builder, reviewer)
	}
	if builder == reviewer || builder == agentName("builder", otherID) || reviewer == agentName("reviewer", otherID) {
		t.Fatalf("agent names are not collision-resistant: builder=%q reviewer=%q", builder, reviewer)
	}
}

func TestTaskAgentNamesCompareFullExecutionIdentity(t *testing.T) {
	name := taskAgentName("builder", "run-184", "api")
	work := state.WorktreeState{Path: "/managed/api", WorkspaceID: "workspace-api", PaneID: "pane-api"}
	base := herdr.AgentInfo{Name: name, SessionID: "provider-session", WorkspaceID: work.WorkspaceID, PaneID: work.PaneID, Path: work.Path}
	if !matchesParallelAgentIdentity(base, name, work) {
		t.Fatal("complete provider identity did not reconcile")
	}
	for field := range map[string]struct{}{"name": {}, "session": {}, "workspace": {}, "pane": {}, "path": {}} {
		got := base
		switch field {
		case "name":
			got.Name = "other"
		case "session":
			got.SessionID = ""
		case "workspace":
			got.WorkspaceID = "other"
		case "pane":
			got.PaneID = "other"
		case "path":
			got.Path = "/managed/other"
		}
		if matchesParallelAgentIdentity(got, name, work) {
			t.Fatalf("identity mismatch %s was accepted", field)
		}
	}
}

func TestPendingStartMigratesLegacyLongAgentNameBeforeRetry(t *testing.T) {
	longID := contract.RunID("run-1787925632789000929-1")
	for _, tc := range []struct {
		role       string
		phase      contract.RunPhase
		cursor     int
		legacyName func(contract.RunID) string
		set        func(*state.RunSnapshot, string)
		name       func(*state.RunSnapshot) string
	}{
		{
			role:       "builder",
			phase:      contract.PhaseBuilding,
			cursor:     0,
			legacyName: func(id contract.RunID) string { return "builder-" + string(id) },
			set: func(snapshot *state.RunSnapshot, name string) {
				snapshot.BuilderWorktree = state.WorktreeState{Path: "/tmp/builder", WorkspaceID: "workspace-184", PaneID: "pane-184", Branch: "agent/api"}
				snapshot.Builder = state.AgentEvidence{Name: name}
			},
			name: func(snapshot *state.RunSnapshot) string { return snapshot.Builder.Name },
		},
		{
			role:       "reviewer",
			phase:      contract.PhaseReviewing,
			cursor:     1,
			legacyName: func(id contract.RunID) string { return "reviewer-" + string(id) },
			set: func(snapshot *state.RunSnapshot, name string) {
				snapshot.ReviewerWorktree = state.WorktreeState{Path: "/tmp/review", WorkspaceID: "workspace-review-184", PaneID: "pane-review-184", Branch: "agent/api-integration"}
				snapshot.Reviewer = state.AgentEvidence{Name: name}
			},
			name: func(snapshot *state.RunSnapshot) string { return snapshot.Reviewer.Name },
		},
	} {
		t.Run(tc.role, func(t *testing.T) {
			h := newHarness(t)
			h.Deps.RunID = func(time.Time) contract.RunID { return longID }
			h.orchestrator = New(h.Deps)
			id, err := h.orchestrator.Start(context.Background(), h.contractPath)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := h.mustLoad(id)
			snapshot.Phase = tc.phase
			snapshot.ActionCursor = tc.cursor
			snapshot.PendingAction = "start_" + tc.role
			legacyName := tc.legacyName(id)
			tc.set(&snapshot, legacyName)
			if err := h.store.Save(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}

			if err := New(h.Deps).Advance(context.Background(), id); err != nil {
				t.Fatalf("legacy name reconcile error = %v", err)
			}
			got := h.mustLoad(id)
			wantName := agentName(tc.role, id)
			if tc.name(&got) != wantName || got.PendingAction != "" || len(h.herdr.starts) != 0 {
				t.Fatalf("migration state = %#v, starts=%v, want name=%q and no start", got, h.herdr.starts, wantName)
			}

			if err := New(h.Deps).Advance(context.Background(), id); err != nil {
				t.Fatalf("retry start error = %v", err)
			}
			if len(h.herdr.starts) != 1 || h.herdr.starts[0].Name != wantName {
				t.Fatalf("retry starts = %v, want one start for %q", h.herdr.starts, wantName)
			}
		})
	}
}
