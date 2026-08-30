package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/revert"
)

// RevertRunService is the narrow command service. It owns only run-state
// lookup and request composition; reviewed revert semantics remain in
// revert.Service.
type RevertRunService interface {
	CreateRevert(context.Context, contract.RunID, string) (github.PullRequest, error)
}

func runCreateRevert(ctx context.Context, args []string, stdout, stderr interface{ Write([]byte) (int, error) }, service RevertRunService) int {
	if len(args) != 3 || strings.TrimSpace(args[0]) == "" || args[1] != "--reason" || args[2] == "" {
		printUsage(stderr)
		return 2
	}
	if service == nil {
		return reportRunError(stderr, errors.New("revert 서비스가 구성되지 않았습니다"))
	}
	pr, err := service.CreateRevert(ctx, contract.RunID(args[0]), args[2])
	if err != nil {
		return reportRunError(stderr, err)
	}
	if pr.Number <= 0 {
		return reportRunError(stderr, errors.New("revert draft PR 번호가 반환되지 않았습니다"))
	}
	fmt.Fprintf(stdout, "%d\n", pr.Number)
	return 0
}

// RevertRunServiceImpl composes a reviewed revert.Service with persisted
// run metadata. It deliberately rejects non-completed or non-immutable runs.
type RevertRunServiceImpl struct {
	store   RunStateStore
	creator interface {
		Create(context.Context, revert.Request) (github.PullRequest, error)
	}
	managedRoot string
}

func NewRevertRunService(store RunStateStore, creator interface {
	Create(context.Context, revert.Request) (github.PullRequest, error)
}, managedRoot string) *RevertRunServiceImpl {
	return &RevertRunServiceImpl{store: store, creator: creator, managedRoot: managedRoot}
}

func (s *RevertRunServiceImpl) CreateRevert(ctx context.Context, id contract.RunID, reason string) (github.PullRequest, error) {
	if s == nil || s.store == nil || s.creator == nil {
		return github.PullRequest{}, errors.New("revert service dependencies are incomplete")
	}
	snapshot, err := s.store.Load(ctx, id)
	if err != nil {
		return github.PullRequest{}, err
	}
	if snapshot.Phase != contract.PhaseCompleted || !snapshot.PullRequestMerged || strings.TrimSpace(snapshot.MergeSHA) == "" || strings.TrimSpace(snapshot.ContractPath) == "" || strings.TrimSpace(snapshot.Repository.Owner) == "" || strings.TrimSpace(snapshot.Repository.Name) == "" || strings.TrimSpace(snapshot.Repository.DefaultBranch) == "" {
		return github.PullRequest{}, errors.New("revert requires an exact completed merge state")
	}
	return s.creator.Create(ctx, revert.Request{
		Repository:     github.Repository{Owner: snapshot.Repository.Owner, Name: snapshot.Repository.Name},
		RepositoryPath: snapshot.RepositoryPath, ManagedRoot: s.managedRoot,
		Parent: github.Issue{Number: snapshot.ParentIssue}, DefaultBranch: snapshot.Repository.DefaultBranch,
		BaseCommit: snapshot.MergeSHA, MergeSHA: snapshot.MergeSHA, Remote: "origin", Reason: reason,
	})
}

var _ RevertRunService = (*RevertRunServiceImpl)(nil)
