package revert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"thread-dock/internal/github"
	"thread-dock/internal/runner"
	"thread-dock/internal/worktree"
)

type fakeGit struct {
	calls          []string
	statuses       []worktree.RevertWorktreeStatus
	reconcileCalls int
	reconcileErr   error
	fetchSHA       string
	fetchErr       error
	ancestor       bool
	ancestorSet    bool
	ancestorErr    error
	revertErr      error
	abortErr       error
	createErr      error
	pushErr        error
}

func (f *fakeGit) FetchRemoteHead(_ context.Context, repositoryPath, remote, branch string) (string, error) {
	f.calls = append(f.calls, "fetch|"+repositoryPath+"|"+remote+"|"+branch)
	if f.fetchErr != nil {
		return "", f.fetchErr
	}
	if f.fetchSHA != "" {
		return f.fetchSHA, nil
	}
	return "0123456789abcdef0123456789abcdef01234567", nil
}

func (f *fakeGit) IsAncestorOf(_ context.Context, repositoryPath, ancestor, descendant string) (bool, error) {
	f.calls = append(f.calls, "ancestor|"+repositoryPath+"|"+ancestor+"|"+descendant)
	if f.ancestorErr != nil {
		return false, f.ancestorErr
	}
	if !f.ancestorSet {
		return true, nil
	}
	return f.ancestor, nil
}

func (f *fakeGit) ReconcileRevertWorktree(_ context.Context, _, _, _, _, _ string) (worktree.RevertWorktreeStatus, error) {
	f.reconcileCalls++
	if f.reconcileErr != nil {
		return worktree.RevertWorktreeStatus{}, f.reconcileErr
	}
	if len(f.statuses) > 0 {
		status := f.statuses[0]
		f.statuses = f.statuses[1:]
		return status, nil
	}
	return worktree.RevertWorktreeStatus{}, nil
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
	request      github.DraftPRRequest
	result       github.PullRequest
	existing     github.PullRequest
	found        bool
	autoExisting bool
	created      bool
	lookupCalls  int
	createCalls  int
	err          error
}

func (f *fakeGitHub) FindOpenPullRequest(_ context.Context, _ github.Repository, _, _ string) (github.PullRequest, bool, error) {
	f.lookupCalls++
	if f.autoExisting && f.created {
		return f.result, true, nil
	}
	return f.existing, f.found, nil
}

func (f *fakeGitHub) CreateSafeDraftPR(_ context.Context, _ github.Repository, request github.DraftPRRequest) (github.PullRequest, error) {
	f.createCalls++
	f.created = true
	f.request = request
	return f.result, f.err
}

func TestCreateRevertRetriesAgainstRealGitLinkedWorktree(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(bare, 0700); err != nil {
		t.Fatal(err)
	}
	runSetupGit(t, "", "init", "--bare", bare)
	runSetupGit(t, "", "clone", bare, repository)
	runSetupGit(t, repository, "config", "user.email", "test@example.com")
	runSetupGit(t, repository, "config", "user.name", "ThreadDock Test")
	writeTestFile(t, filepath.Join(repository, "README.md"), "base\n")
	runSetupGit(t, repository, "add", "README.md")
	runSetupGit(t, repository, "commit", "-m", "base")
	runSetupGit(t, repository, "branch", "-M", "main")
	base := strings.TrimSpace(runSetupGit(t, repository, "rev-parse", "HEAD"))
	runSetupGit(t, repository, "push", "origin", "main")
	runSetupGit(t, repository, "checkout", "-b", "feature")
	writeTestFile(t, filepath.Join(repository, "change.txt"), "feature\n")
	runSetupGit(t, repository, "add", "change.txt")
	runSetupGit(t, repository, "commit", "-m", "feature")
	runSetupGit(t, repository, "checkout", "main")
	runSetupGit(t, repository, "merge", "--no-ff", "--no-edit", "feature")
	mergeSHA := strings.TrimSpace(runSetupGit(t, repository, "rev-parse", "HEAD"))
	runSetupGit(t, repository, "push", "origin", "main")
	configured := filepath.Join(root, "configured-linked")
	runSetupGit(t, repository, "worktree", "add", "--detach", configured, base)
	managed := filepath.Join(root, "managed")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}

	git := worktree.New(runner.OSRunner{}, "git", managed, configured)
	prErr := errors.New("simulated transient draft response")
	gh := &fakeGitHub{autoExisting: true, result: github.PullRequest{Number: 207}, err: prErr}
	request := Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: configured,
		ManagedRoot:    managed,
		Parent:         github.Issue{Number: 184},
		DefaultBranch:  "main",
		BaseCommit:     mergeSHA,
		MergeSHA:       mergeSHA,
		Reason:         "real git retry",
	}
	service := New(git, gh)
	if _, err := service.Create(context.Background(), request); !errors.Is(err, prErr) {
		t.Fatalf("first create err=%v", err)
	}
	branch := "revert/184-" + mergeSHA[:12]
	target := filepath.Join(managed, ".revert-worktrees", strings.ReplaceAll(branch, "/", "-"))
	headAfterFirst := strings.TrimSpace(runSetupGit(t, target, "rev-parse", "HEAD"))
	remoteAfterFirst := strings.TrimSpace(runSetupGit(t, "", "--git-dir", bare, "rev-parse", "refs/heads/"+branch))
	if headAfterFirst != remoteAfterFirst || gh.createCalls != 1 {
		t.Fatalf("first head=%s remote=%s creates=%d", headAfterFirst, remoteAfterFirst, gh.createCalls)
	}
	gh.err = nil
	got, err := service.Create(context.Background(), request)
	if err != nil || got.Number != 207 {
		status, reconcileErr := git.ReconcileRevertWorktree(context.Background(), configured, target, branch, mergeSHA, mergeSHA)
		t.Fatalf("second pr=%+v err=%v reconcile=%+v reconcileErr=%v", got, err, status, reconcileErr)
	}
	headAfterSecond := strings.TrimSpace(runSetupGit(t, target, "rev-parse", "HEAD"))
	remoteAfterSecond := strings.TrimSpace(runSetupGit(t, "", "--git-dir", bare, "rev-parse", "refs/heads/"+branch))
	if headAfterSecond != headAfterFirst || remoteAfterSecond != remoteAfterFirst || gh.createCalls != 1 {
		t.Fatalf("second head=%s/%s remote=%s/%s creates=%d", headAfterSecond, headAfterFirst, remoteAfterSecond, remoteAfterFirst, gh.createCalls)
	}
}

func TestCreateRevertFetchesMergeOnlyAvailableOnRemote(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	repository := filepath.Join(root, "repo")
	contributor := filepath.Join(root, "contributor")
	if err := os.Mkdir(bare, 0700); err != nil {
		t.Fatal(err)
	}
	runSetupGit(t, "", "init", "--bare", bare)
	runSetupGit(t, "", "clone", bare, repository)
	runSetupGit(t, repository, "config", "user.email", "test@example.com")
	runSetupGit(t, repository, "config", "user.name", "ThreadDock Test")
	writeTestFile(t, filepath.Join(repository, "README.md"), "base\n")
	runSetupGit(t, repository, "add", "README.md")
	runSetupGit(t, repository, "commit", "-m", "base")
	runSetupGit(t, repository, "branch", "-M", "main")
	runSetupGit(t, repository, "push", "origin", "main")
	runSetupGit(t, "", "clone", "--branch", "main", bare, contributor)
	runSetupGit(t, contributor, "config", "user.email", "test@example.com")
	runSetupGit(t, contributor, "config", "user.name", "ThreadDock Test")
	runSetupGit(t, contributor, "checkout", "-b", "feature")
	writeTestFile(t, filepath.Join(contributor, "change.txt"), "feature\n")
	runSetupGit(t, contributor, "add", "change.txt")
	runSetupGit(t, contributor, "commit", "-m", "feature")
	runSetupGit(t, contributor, "checkout", "main")
	runSetupGit(t, contributor, "merge", "--no-ff", "--no-edit", "feature")
	mergeSHA := strings.TrimSpace(runSetupGit(t, contributor, "rev-parse", "HEAD"))
	runSetupGit(t, contributor, "push", "origin", "main")
	remoteHead := mergeSHA
	missing, err := (runner.OSRunner{}).Run(context.Background(), repository, "git", "cat-file", "-e", mergeSHA+"^{commit}")
	if err == nil && missing.ExitCode == 0 {
		t.Fatalf("merge commit unexpectedly existed before service fetch")
	}
	managed := filepath.Join(root, "managed")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}
	git := worktree.New(runner.OSRunner{}, "git", managed, repository)
	gh := &fakeGitHub{result: github.PullRequest{Number: 208}}
	request := Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repository,
		ManagedRoot:    managed,
		Parent:         github.Issue{Number: 184},
		DefaultBranch:  "main",
		BaseCommit:     mergeSHA,
		MergeSHA:       mergeSHA,
		Reason:         "remote-only merge",
	}
	if _, err := New(git, gh).Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(managed, ".revert-worktrees", "revert-184-"+mergeSHA[:12])
	parent := strings.TrimSpace(runSetupGit(t, worktreePath, "rev-parse", "HEAD^"))
	if parent != remoteHead {
		t.Fatalf("revert parent=%s, want fetched remote head=%s", parent, remoteHead)
	}
}

func runSetupGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	result, err := (runner.OSRunner{}).Run(context.Background(), cwd, "git", args...)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("git %v cwd=%q exit=%d err=%v stderr=%q", args, cwd, result.ExitCode, err, result.Stderr)
	}
	return result.Stdout
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRevertUsesManagedWorktreeAndExplicitPush(t *testing.T) {
	managed := t.TempDir()
	repoPath := t.TempDir()
	mergeSHA := "0123456789abcdef0123456789abcdef01234567"
	git := &fakeGit{}
	gh := &fakeGitHub{result: github.PullRequest{Number: 200}}
	service := New(git, gh)
	request := Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repoPath,
		ManagedRoot:    managed,
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
	worktreePath := filepath.Join(managed, ".revert-worktrees", "revert-184-0123456789ab")
	wantCalls := []string{
		"fetch|" + repoPath + "|origin|main",
		"ancestor|" + repoPath + "|" + mergeSHA + "|0123456789abcdef0123456789abcdef01234567",
		"create|" + repoPath + "|" + worktreePath + "|" + wantBranch + "|0123456789abcdef0123456789abcdef01234567",
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

func TestCreateRevertFetchesRemoteHeadAndCreatesFromFetchedHead(t *testing.T) {
	managed := t.TempDir()
	repoPath := t.TempDir()
	mergeSHA := "0123456789abcdef0123456789abcdef01234567"
	remoteHead := "fedcba9876543210fedcba9876543210fedcba98"
	git := &fakeGit{fetchSHA: remoteHead, ancestor: true}
	request := Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repoPath,
		ManagedRoot:    managed,
		Parent:         github.Issue{Number: 184},
		DefaultBranch:  "main",
		BaseCommit:     mergeSHA,
		MergeSHA:       mergeSHA,
		Reason:         "remote-only merge",
	}
	if _, err := New(git, &fakeGitHub{result: github.PullRequest{Number: 201}}).Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(git.calls) < 4 || git.calls[0] != "fetch|"+repoPath+"|origin|main" || git.calls[1] != "ancestor|"+repoPath+"|"+mergeSHA+"|"+remoteHead {
		t.Fatalf("calls=%v, want remote fetch and ancestry before worktree", git.calls)
	}
	if !strings.Contains(git.calls[2], "|"+remoteHead) {
		t.Fatalf("create did not use fetched remote head: %v", git.calls)
	}
}

func TestCreateRevertRejectsMergeNotReachableFromFetchedHead(t *testing.T) {
	managed := t.TempDir()
	repoPath := t.TempDir()
	request := Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repoPath,
		ManagedRoot:    managed,
		Parent:         github.Issue{Number: 184},
		DefaultBranch:  "main",
		BaseCommit:     "0123456789abcdef0123456789abcdef01234567",
		MergeSHA:       "0123456789abcdef0123456789abcdef01234567",
		Reason:         "foreign merge",
	}
	git := &fakeGit{fetchSHA: "fedcba9876543210fedcba9876543210fedcba98", ancestorSet: true, ancestor: false}
	if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); err == nil || len(git.calls) != 2 {
		t.Fatalf("calls=%v err=%v, want fetch+ancestry rejection", git.calls, err)
	}
}

func TestCreateRevertConflictAbortsAndBlocks(t *testing.T) {
	_, _, _, request := validRequest(t)
	git := &fakeGit{revertErr: ErrConflict}
	service := New(git, &fakeGitHub{})
	_, err := service.Create(context.Background(), request)
	if !errors.Is(err, ErrBlocked) || len(git.calls) != 5 || !strings.HasPrefix(git.calls[4], "abort|") {
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

func TestCreateRevertUsesOnlyGeneratedTarget(t *testing.T) {
	_, _, _, request := validRequest(t)
	git := &fakeGit{}
	if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); err != nil {
		t.Fatal(err)
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
	if strings.Contains(gh.request.Body, "line\n\"quote\"") || !strings.Contains(gh.request.Body, "line\n&#34;quote&#34;") || !strings.Contains(gh.request.Body, "<pre>") {
		t.Fatalf("body=%q", gh.request.Body)
	}
}

func TestCreateRevertRetrySkipsCompletedRevertStages(t *testing.T) {
	managed, _, _, request := validRequest(t)
	git := &fakeGit{statuses: []worktree.RevertWorktreeStatus{{}, {Exists: true, Reverted: true}}, pushErr: errors.New("push response lost")}
	gh := &fakeGitHub{result: github.PullRequest{Number: 202}}
	service := New(git, gh)
	if _, err := service.Create(context.Background(), request); !errors.Is(err, git.pushErr) {
		t.Fatalf("first err=%v", err)
	}
	if err := os.MkdirAll(filepath.Join(managed, ".revert-worktrees", "revert-184-0123456789ab"), 0700); err != nil {
		t.Fatal(err)
	}
	git.pushErr = nil
	got, err := service.Create(context.Background(), request)
	if err != nil || got.Number != 202 {
		t.Fatalf("second pr=%+v err=%v", got, err)
	}
	if git.reconcileCalls != 2 || len(git.calls) != 8 || strings.Count(strings.Join(git.calls, "\n"), "create|") != 1 || strings.Count(strings.Join(git.calls, "\n"), "revert|") != 1 || strings.Count(strings.Join(git.calls, "\n"), "push|") != 2 {
		t.Fatalf("reconcile=%d calls=%v", git.reconcileCalls, git.calls)
	}
}

func TestCreateRevertRetryAfterRevertFailureSkipsToPush(t *testing.T) {
	managed, _, _, request := validRequest(t)
	revertErr := errors.New("revert response lost")
	git := &fakeGit{statuses: []worktree.RevertWorktreeStatus{{}, {Exists: true, Reverted: true}}, revertErr: revertErr}
	gh := &fakeGitHub{result: github.PullRequest{Number: 205}}
	service := New(git, gh)
	if _, err := service.Create(context.Background(), request); !errors.Is(err, revertErr) {
		t.Fatalf("first err=%v", err)
	}
	if err := os.MkdirAll(filepath.Join(managed, ".revert-worktrees", "revert-184-0123456789ab"), 0700); err != nil {
		t.Fatal(err)
	}
	git.revertErr = nil
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(git.calls, "\n"), "revert|") != 1 || strings.Count(strings.Join(git.calls, "\n"), "push|") != 1 {
		t.Fatalf("calls=%v", git.calls)
	}
}

func TestCreateRevertRetryAfterDraftPRFailureSkipsToNextCreate(t *testing.T) {
	managed, _, _, request := validRequest(t)
	prErr := errors.New("draft response lost")
	git := &fakeGit{statuses: []worktree.RevertWorktreeStatus{{}, {Exists: true, Reverted: true}}}
	gh := &fakeGitHub{err: prErr, result: github.PullRequest{Number: 206}}
	service := New(git, gh)
	if _, err := service.Create(context.Background(), request); !errors.Is(err, prErr) {
		t.Fatalf("first err=%v", err)
	}
	if err := os.MkdirAll(filepath.Join(managed, ".revert-worktrees", "revert-184-0123456789ab"), 0700); err != nil {
		t.Fatal(err)
	}
	gh.err = nil
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(git.calls, "\n"), "create|") != 1 || strings.Count(strings.Join(git.calls, "\n"), "revert|") != 1 || gh.createCalls != 2 {
		t.Fatalf("git=%v createCalls=%d", git.calls, gh.createCalls)
	}
}

func TestCreateRevertReconcilesExistingDeterministicTarget(t *testing.T) {
	managed, _, _, request := validRequest(t)
	target := filepath.Join(managed, ".revert-worktrees", "revert-184-0123456789ab")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{statuses: []worktree.RevertWorktreeStatus{{Exists: true, Reverted: true}}}
	gh := &fakeGitHub{existing: github.PullRequest{Number: 204}, found: true}
	if _, err := New(git, gh).Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(git.calls, "\n"), "create|") || strings.Contains(strings.Join(git.calls, "\n"), "revert|") {
		t.Fatalf("existing target was recreated: %v", git.calls)
	}
}

func TestCreateRevertCreatesPrivateParentForGeneratedTarget(t *testing.T) {
	managed, repoPath, _, request := validRequest(t)
	request.RepositoryPath = repoPath
	request.ManagedRoot = managed
	git := &fakeGit{}
	if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(managed, ".revert-worktrees")
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		t.Fatalf("parent=%q info=%v err=%v", parent, info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
		t.Fatalf("mode=%o want 0700", info.Mode().Perm())
	}
	if len(git.calls) == 0 || !strings.Contains(strings.Join(git.calls, "\n"), parent) {
		t.Fatalf("calls=%v", git.calls)
	}
}

func TestCreateRevertRejectsGeneratedParentSymlinkEscape(t *testing.T) {
	managed, repoPath, _, request := validRequest(t)
	request.RepositoryPath = repoPath
	request.ManagedRoot = managed
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(managed, ".revert-worktrees")); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{}
	if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); !errors.Is(err, ErrUnsafeTarget) || len(git.calls) != 0 {
		t.Fatalf("calls=%v err=%v", git.calls, err)
	}
}

func TestCreateRevertReturnsExistingOpenPRAfterPush(t *testing.T) {
	_, _, _, request := validRequest(t)
	gh := &fakeGitHub{existing: github.PullRequest{Number: 203}, found: true}
	got, err := New(&fakeGit{}, gh).Create(context.Background(), request)
	if err != nil || got.Number != 203 || gh.lookupCalls != 1 || gh.createCalls != 0 {
		t.Fatalf("pr=%+v lookup=%d create=%d err=%v", got, gh.lookupCalls, gh.createCalls, err)
	}
}

func TestCreateRevertRejectsOversizedOrControlReasonBeforeCommands(t *testing.T) {
	_, _, _, request := validRequest(t)
	for _, reason := range []string{strings.Repeat("x", MaxRevertReasonBytes+1), "bad\x01reason", " leading-space"} {
		request.Reason = reason
		git := &fakeGit{}
		if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); err == nil || len(git.calls) != 0 {
			t.Fatalf("reason=%q calls=%v err=%v", reason, git.calls, err)
		}
	}
}

func TestCreateRevertRejectsEscapedDraftExpansionBeforeAnyPort(t *testing.T) {
	_, _, _, request := validRequest(t)
	request.Reason = strings.Repeat("<", MaxRevertReasonBytes)
	git := &fakeGit{}
	gh := &fakeGitHub{}
	if _, err := New(git, gh).Create(context.Background(), request); err == nil || len(git.calls) != 0 || git.reconcileCalls != 0 || gh.lookupCalls != 0 || gh.createCalls != 0 {
		t.Fatalf("calls=%v reconcile=%d lookup=%d create=%d err=%v", git.calls, git.reconcileCalls, gh.lookupCalls, gh.createCalls, err)
	}
}

func TestCreateRevertRejectsDotRepositoryPartsBeforeCommands(t *testing.T) {
	_, _, _, request := validRequest(t)
	for _, repository := range []github.Repository{{Owner: ".", Name: "payments-api"}, {Owner: "platform", Name: ".."}} {
		request.Repository = repository
		git := &fakeGit{}
		if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); err == nil || len(git.calls) != 0 {
			t.Fatalf("repository=%+v calls=%v err=%v", repository, git.calls, err)
		}
	}
}

func TestCreateRevertRequiresRequestDefaultBranch(t *testing.T) {
	_, _, _, request := validRequest(t)
	request.DefaultBranch = ""
	git := &fakeGit{}
	if _, err := New(git, &fakeGitHub{}).Create(context.Background(), request); err == nil || len(git.calls) != 0 {
		t.Fatalf("calls=%v err=%v", git.calls, err)
	}
}

func TestRequestHasOnlyGeneratedWorktreeTarget(t *testing.T) {
	if _, ok := reflect.TypeOf(Request{}).FieldByName("WorktreePath"); ok {
		t.Fatal("Request must not accept a caller-supplied worktree target")
	}
}

func validRequest(t *testing.T) (string, string, string, Request) {
	t.Helper()
	managed := t.TempDir()
	repoPath := t.TempDir()
	return managed, repoPath, "", Request{
		Repository:     github.Repository{Owner: "platform", Name: "payments-api"},
		RepositoryPath: repoPath,
		ManagedRoot:    managed,
		Parent:         github.Issue{Number: 184},
		DefaultBranch:  "main",
		BaseCommit:     "89abcdef0123456789abcdef0123456789abcdef",
		MergeSHA:       "0123456789abcdef0123456789abcdef01234567",
		Reason:         "reason",
	}
}
