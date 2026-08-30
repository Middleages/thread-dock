package cli

import (
	"context"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/revert"
	"thread-dock/internal/state"
)

type captureRevertCreator struct {
	request revert.Request
}

func (c *captureRevertCreator) Create(_ context.Context, request revert.Request) (github.PullRequest, error) {
	c.request = request
	return github.PullRequest{Number: 1}, nil
}

func TestCreateRevertUsesImmutableSnapshotIdentityAndMergeSHA(t *testing.T) {
	store := state.NewStore(t.TempDir())
	snapshot := state.RunSnapshot{
		RunID: "run-immutable-revert", Phase: contract.PhaseCompleted, ContractPath: "/mutable/contract.json",
		Repository:     contract.RepositoryRef{Owner: "frozen-owner", Name: "frozen-repo", DefaultBranch: "frozen-main"},
		RepositoryPath: "/repo", ParentIssue: 184, PullRequestMerged: true,
		MergeSHA: "0123456789abcdef0123456789abcdef01234567",
	}
	if err := store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	creator := &captureRevertCreator{}
	service := NewRevertRunService(store, creator, t.TempDir())
	if _, err := service.CreateRevert(context.Background(), snapshot.RunID, "regression"); err != nil {
		t.Fatal(err)
	}
	if creator.request.Repository != (github.Repository{Owner: "frozen-owner", Name: "frozen-repo"}) || creator.request.DefaultBranch != "frozen-main" || creator.request.BaseCommit != snapshot.MergeSHA || creator.request.MergeSHA != snapshot.MergeSHA {
		t.Fatalf("revert request=%+v", creator.request)
	}
}
