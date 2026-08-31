package orchestrator

import (
	"context"
	"testing"

	"thread-dock/internal/contract"
	retirementpolicy "thread-dock/internal/retirement"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

type round3RoleInspector struct {
	roles  []string
	legacy int
}

func (i *round3RoleInspector) InspectRetirementTarget(_ context.Context, _, path, branch, sha string) (worktree.RetirementProof, error) {
	i.legacy++
	return worktree.RetirementProof{RepositoryCommonDir: "/repo/.git", Path: path, Branch: branch, HeadSHA: sha}, nil
}

func (i *round3RoleInspector) InspectRetirementTargetForRole(_ context.Context, _, role, path, branch, sha string) (worktree.RetirementProof, error) {
	i.roles = append(i.roles, role)
	return worktree.RetirementProof{RepositoryCommonDir: "/repo/.git", Path: path, Branch: branch, HeadSHA: sha}, nil
}

func TestRetirementProofRoutesReviewerAndBuilderThroughRoleAwareInspector(t *testing.T) {
	for _, test := range []struct {
		name, role, path string
	}{
		{name: "reviewer uses managed role", role: "reviewer", path: "/managed/review"},
		{name: "builder uses Herdr role", role: "builder", path: "/herdr/builder"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			inspector := &round3RoleInspector{}
			h.Deps.RetirementGitInspector = inspector
			h.orchestrator = New(h.Deps)
			target := state.RetirementTarget{Key: test.role, Role: test.role, Path: test.path, Branch: "agent/" + test.role, HeadSHA: validSHA, Status: "agent_observed"}
			snapshot := state.RunSnapshot{RunID: contract.RunID("role-aware-" + test.role), Phase: contract.PhaseRetiring, RepositoryPath: "/repo", Retirement: state.RetirementState{Status: "pending", TargetPhase: contract.PhaseCompleted, Targets: []state.RetirementTarget{target}}}
			if err := h.store.Create(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}
			loaded := h.mustLoad(snapshot.RunID)
			if err := h.orchestrator.proveRetirementGit(context.Background(), &loaded, retirementpolicy.Decision{TargetKey: target.Key}); err != nil {
				t.Fatal(err)
			}
			if len(inspector.roles) != 1 || inspector.roles[0] != test.role || inspector.legacy != 0 {
				t.Fatalf("roles=%v legacy=%d", inspector.roles, inspector.legacy)
			}
		})
	}
}
