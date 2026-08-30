package revert

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/github"
)

type fakeGit struct {
	calls     []string
	revertErr error
	abortErr  error
	createErr error
	pushErr   error
}

func (f *fakeGit) CreateManagedWorktree(_ context.Context, repositoryPath, worktreePath, branch, base string) error {
	f.calls = append(f.calls, "create|"+repositoryPath+"|"+worktreePath+"|"+branch+"|"+base)
	return f.createErr
}

func (f *fakeGit) RevertMergeCommit(_ context.Context, worktreePath, sha string) error {
	f.calls = append(f.calls, "revert|"+worktreePath+"|"+sha)
	return f.revertErr
}

func (f *fakeGit) AbortRevert(_ context.Context, worktreePath string) error {
	f.calls = append(f.calls, "abort|"+worktreePath)
	return f.abortErr
}

func (f *fakeGit) PushBranch(_ context.Context, worktreePath, remote, branch string) error {
	f.calls = append(f.calls, "push|"+worktreePath+"|"+remote+"|"+branch)
	return f.pushErr
}

type fakeGitHub struct {
	request github.DraftPRRequest
	result  github.PullRequest
	err     error
}

func (f *fakeGitHub) CreateDraftPR(_ context.Context, _ github.Repository, request github.DraftPRRequest) (github.PullRequest, error) {
	f.request = request
	return f.result, f.err
}

func TestCreateRevertUsesManagedWorktreeAndExplicitPush(t *testing.T) {
	managed := t.TempDir()
	repoPath := t.TempDir()
	worktreePath := filepath.Join(managed, "revert-184")
	mergeSHA := "0123456789abcdef0123456789abcdef01234567"
	git := &fakeGit{}
	gh := &fakeGitHub{result: github.PullRequest{Number: 200}}
	service := New(git, gh)
	request := Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repoPath,
		ManagedRoot:    managed,
		WorktreePath:   worktreePath,
		Parent:         github.Issue{Number: 184, Title: "Parent"},
		DefaultBranch:  "main",
		BaseCommit:     "89abcdef0123456789abcdef0123456789abcdef",
		MergeSHA:       mergeSHA,
		Reason:         "rollback after regression",
	}
	got, err := service.Create(context.Background(), request)
	if err != nil || got.Number != 200 {
		t.Fatalf("pr=%+v err=%v", got, err)
	}
	wantBranch := "revert/184-0123456789ab"
	wantCalls := []string{
		"create|" + repoPath + "|" + worktreePath + "|" + wantBranch + "|89abcdef0123456789abcdef0123456789abcdef",
		"revert|" + worktreePath + "|" + mergeSHA,
		"push|" + worktreePath + "|origin|" + wantBranch,
	}
	if !reflect.DeepEqual(git.calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", git.calls, wantCalls)
	}
	if gh.request.Head != wantBranch || gh.request.Base != "main" || gh.request.IssueNumber != 184 {
		t.Fatalf("request=%+v", gh.request)
	}
	if !strings.Contains(gh.request.Body, "#184") || !strings.Contains(gh.request.Body, "rollback after regression") {
		t.Fatalf("body=%q", gh.request.Body)
	}
}

func TestCreateRevertConflictAbortsAndBlocks(t *testing.T) {
	_, _, _, request := validRequest(t)
	git := &fakeGit{revertErr: ErrConflict}
	service := New(git, &fakeGitHub{})
	_, err := service.Create(context.Background(), request)
	if !errors.Is(err, ErrBlocked) || len(git.calls) != 3 || !strings.HasPrefix(git.calls[2], "abort|") {
		t.Fatalf("calls=%v err=%v", git.calls, err)
	}
}

func TestCreateRevertConflictIncludesAbortFailure(t *testing.T) {
	_, _, _, request := validRequest(t)
	abortErr := errors.New("abort failed")
	git := &fakeGit{revertErr: ErrConflict, abortErr: abortErr}
	_, err := New(git, &fakeGitHub{}).Create(context.Background(), request)
	if !errors.Is(err, ErrBlocked) || !errors.Is(err, abortErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateRevertRejectsUnsafePathBeforeCommands(t *testing.T) {
	managed, repoPath, _, request := validRequest(t)
	request.WorktreePath = filepath.Join(filepath.Dir(managed), "outside")
	git := &fakeGit{}
	_, err := New(git, &fakeGitHub{}).Create(context.Background(), request)
	if !errors.Is(err, ErrUnsafeTarget) || len(git.calls) != 0 {
		t.Fatalf("calls=%v err=%v", git.calls, err)
	}
	request.WorktreePath = repoPath
	_, err = New(git, &fakeGitHub{}).Create(context.Background(), request)
	if !errors.Is(err, ErrUnsafeTarget) || len(git.calls) != 0 {
		t.Fatalf("repository target calls=%v err=%v", git.calls, err)
	}
}

func TestCreateRevertPropagatesPushAndPRFailures(t *testing.T) {
	_, _, _, request := validRequest(t)
	pushErr := errors.New("push failed")
	git := &fakeGit{pushErr: pushErr}
	_, err := New(git, &fakeGitHub{}).Create(context.Background(), request)
	if !errors.Is(err, pushErr) {
		t.Fatalf("push err=%v", err)
	}
	prErr := errors.New("PR failed")
	git = &fakeGit{}
	_, err = New(git, &fakeGitHub{err: prErr}).Create(context.Background(), request)
	if !errors.Is(err, prErr) {
		t.Fatalf("PR err=%v", err)
	}
}

func TestCreateRevertEscapesReason(t *testing.T) {
	_, _, _, request := validRequest(t)
	request.Reason = "line\n\"quote\""
	gh := &fakeGitHub{}
	if _, err := New(&fakeGit{}, gh).Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gh.request.Body, "line\n\"quote\"") || !strings.Contains(gh.request.Body, `"line\n\"quote\""`) {
		t.Fatalf("body=%q", gh.request.Body)
	}
}

func validRequest(t *testing.T) (string, string, string, Request) {
	t.Helper()
	managed := t.TempDir()
	repoPath := t.TempDir()
	worktreePath := filepath.Join(managed, "revert-184")
	return managed, repoPath, worktreePath, Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repoPath,
		ManagedRoot:    managed,
		WorktreePath:   worktreePath,
		Parent:         github.Issue{Number: 184},
		DefaultBranch:  "main",
		BaseCommit:     "89abcdef0123456789abcdef0123456789abcdef",
		MergeSHA:       "0123456789abcdef0123456789abcdef01234567",
		Reason:         "reason",
	}
}
