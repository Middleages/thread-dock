package retirement

import (
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/state"
)

const (
	retirementTestSHA  = "0123456789abcdef0123456789abcdef01234567"
	retirementAlphaSHA = "abcdef0123456789abcdef0123456789abcdef01"
	retirementBetaSHA  = "fedcba9876543210fedcba9876543210fedcba98"
)

func TestBuildOrdersReviewerThenBuildersInReverseAndDeduplicatesWorkspace(t *testing.T) {
	snapshot := state.RunSnapshot{TaskOrder: []string{"alpha", "beta"}, FinalSHA: retirementTestSHA}
	snapshot.Reviewer = state.AgentEvidence{Name: "reviewer"}
	snapshot.ReviewerWorktree = state.WorktreeState{WorkspaceID: "review", PaneID: "review:p1", Path: "/managed/integration", Branch: "agent/integration"}
	snapshot.Tasks = map[string]state.TaskRunState{
		"alpha": {Agent: state.AgentEvidence{Name: "alpha", CommitSHA: retirementAlphaSHA}, Worktree: state.WorktreeState{WorkspaceID: "a", PaneID: "a:p1", Path: "/herdr/a", Branch: "agent/a"}},
		"beta":  {Agent: state.AgentEvidence{Name: "beta", CommitSHA: retirementBetaSHA}, Worktree: state.WorktreeState{WorkspaceID: "b", PaneID: "b:p1", Path: "/herdr/b", Branch: "agent/b"}},
	}
	got, err := Build(snapshot, true, contract.PhaseCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if ids := targetKeys(got.Targets); !reflect.DeepEqual(ids, []string{"reviewer", "builder:beta", "builder:alpha"}) {
		t.Fatalf("targets=%v", ids)
	}
	if got.Status != "pending" || !got.Automatic || got.TargetPhase != contract.PhaseCompleted {
		t.Fatalf("state=%#v", got)
	}
}

func TestBuildRejectsMissingIdentityOrExpectedHead(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*state.RunSnapshot)
	}{
		{name: "reviewer workspace", mutate: func(s *state.RunSnapshot) { s.ReviewerWorktree.WorkspaceID = "" }},
		{name: "reviewer pane", mutate: func(s *state.RunSnapshot) { s.ReviewerWorktree.PaneID = "" }},
		{name: "reviewer path", mutate: func(s *state.RunSnapshot) { s.ReviewerWorktree.Path = "" }},
		{name: "reviewer branch", mutate: func(s *state.RunSnapshot) { s.ReviewerWorktree.Branch = "" }},
		{name: "reviewer head", mutate: func(s *state.RunSnapshot) { s.FinalSHA = "" }},
		{name: "builder head", mutate: func(s *state.RunSnapshot) {
			task := s.Tasks["alpha"]
			task.Agent.CommitSHA = ""
			s.Tasks["alpha"] = task
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := retirementSnapshot()
			tt.mutate(&snapshot)
			if _, err := Build(snapshot, false, contract.PhaseCompleted); err == nil {
				t.Fatal("Build accepted incomplete retirement identity")
			}
		})
	}
}

func TestNextRequiresOperatorForWorkingBlockedOrUnknownAgent(t *testing.T) {
	for _, agentState := range []string{"working", "blocked", "unknown", ""} {
		t.Run(agentState, func(t *testing.T) {
			got := Next(pendingRetirement(), &Observation{TargetKey: "reviewer", AgentState: agentState})
			if got.Kind != NeedsOperator {
				t.Fatalf("decision=%#v", got)
			}
		})
	}
}

func TestNextAdvancesIdleOrDoneThroughProofAndClose(t *testing.T) {
	for _, agentState := range []string{"idle", "done"} {
		t.Run(agentState, func(t *testing.T) {
			plan := pendingRetirement()
			if got := Next(plan, &Observation{TargetKey: "reviewer", AgentState: agentState}); got.Kind != ProveGit {
				t.Fatalf("agent decision=%#v", got)
			}
			plan.Targets[0].Status = "agent_observed"
			if got := Next(plan, &Observation{TargetKey: "reviewer", WorkspaceFound: true, WorkspaceObserved: true, GitProven: true, RepositoryCommonDir: "/repo/.git", Path: "/managed/integration", Branch: "agent/integration", HeadSHA: retirementTestSHA}); got.Kind != CloseWorkspace {
				t.Fatalf("proof decision=%#v", got)
			}
		})
	}
}

func TestNextMissingWorkspaceCompletesTargetAndAllTargetsComplete(t *testing.T) {
	plan := pendingRetirement()
	plan.Targets[0].Status = "closing"
	plan.Targets[1].Status = "retired"
	got := Next(plan, &Observation{TargetKey: "reviewer", WorkspaceFound: false})
	if got.Kind != Complete || got.TargetKey != "reviewer" {
		t.Fatalf("missing workspace decision=%#v", got)
	}
	plan.Targets[0].Status = "retired"
	if got := Next(plan, nil); got.Kind != Complete {
		t.Fatalf("all retired decision=%#v", got)
	}
}

func TestNextAllowsAlreadyClosedWorkspaceAfterGitProof(t *testing.T) {
	plan := pendingRetirement()
	plan.Targets[0].Status = "agent_observed"
	got := Next(plan, &Observation{TargetKey: "reviewer", WorkspaceFound: false, WorkspaceObserved: true, GitProven: true, Path: "/managed/integration", Branch: "agent/integration", HeadSHA: retirementTestSHA})
	if got.Kind != Complete {
		t.Fatalf("already-closed decision=%#v", got)
	}
}

func TestNextRejectsConflictingWorkspaceIdentity(t *testing.T) {
	plan := pendingRetirement()
	plan.Targets[0].Status = "closing"
	got := Next(plan, &Observation{TargetKey: "reviewer", WorkspaceFound: true, WorkspaceID: "other", PaneID: "wrong"})
	if got.Kind != NeedsOperator {
		t.Fatalf("decision=%#v", got)
	}
	if !strings.Contains(got.Reason, "identity") {
		t.Fatalf("reason=%q", got.Reason)
	}
}

func retirementSnapshot() state.RunSnapshot {
	return state.RunSnapshot{
		TaskOrder: []string{"alpha"}, FinalSHA: retirementTestSHA,
		Reviewer:         state.AgentEvidence{Name: "reviewer"},
		ReviewerWorktree: state.WorktreeState{WorkspaceID: "review", PaneID: "review:p1", Path: "/managed/integration", Branch: "agent/integration"},
		Tasks: map[string]state.TaskRunState{
			"alpha": {Agent: state.AgentEvidence{Name: "alpha", CommitSHA: retirementAlphaSHA}, Worktree: state.WorktreeState{WorkspaceID: "a", PaneID: "a:p1", Path: "/herdr/a", Branch: "agent/a"}},
		},
	}
}

func pendingRetirement() state.RetirementState {
	return state.RetirementState{Status: "pending", Targets: []state.RetirementTarget{
		{Key: "reviewer", Role: "reviewer", WorkspaceID: "review", PaneID: "review:p1", Path: "/managed/integration", Branch: "agent/integration", HeadSHA: retirementTestSHA, Status: "pending"},
		{Key: "builder:alpha", Role: "builder", TaskID: "alpha", WorkspaceID: "a", PaneID: "a:p1", Path: "/herdr/a", Branch: "agent/a", HeadSHA: retirementAlphaSHA, Status: "pending"},
	}}
}

func targetKeys(targets []state.RetirementTarget) []string {
	keys := make([]string, len(targets))
	for i, target := range targets {
		keys[i] = target.Key
	}
	return keys
}
