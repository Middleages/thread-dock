package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/runner"
)

type fakeRunner struct {
	results []runner.Result
	errors  []error
	calls   []fakeCall
}

type fakeCall struct {
	cwd  string
	exec string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	f.calls = append(f.calls, fakeCall{cwd: cwd, exec: executable, args: append([]string(nil), args...)})
	var result runner.Result
	if len(f.results) > 0 {
		result, f.results = f.results[0], f.results[1:]
	}
	var err error
	if len(f.errors) > 0 {
		err, f.errors = f.errors[0], f.errors[1:]
	}
	return result, err
}

func TestRemoveSafeRejectsDirtyWorktree(t *testing.T) {
	r := &fakeRunner{results: []runner.Result{{Stdout: " M src/pay.go\n"}}}
	git, _, target := configuredGit(t, r)
	err := git.RemoveSafe(context.Background(), target)
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateUsesExplicitGitArguments(t *testing.T) {
	r := &fakeRunner{}
	git := New(r, "git")
	if err := git.Create(context.Background(), "/repo", "/work/issue-184", "agent/184", "main"); err != nil {
		t.Fatal(err)
	}
	want := fakeCall{cwd: "/repo", exec: "git", args: []string{"worktree", "add", "-b", "agent/184", "/work/issue-184", "main"}}
	if !reflect.DeepEqual(r.calls, []fakeCall{want}) {
		t.Fatalf("calls=%#v want=%#v", r.calls, []fakeCall{want})
	}
}

func TestStatusUsesPorcelain(t *testing.T) {
	r := &fakeRunner{results: []runner.Result{{Stdout: " M file.go\n"}}}
	git := New(r, "git")
	got, err := git.Status(context.Background(), "/work/issue-184")
	if err != nil || got != " M file.go\n" {
		t.Fatalf("status=%q err=%v", got, err)
	}
	if !reflect.DeepEqual(r.calls[0].args, []string{"status", "--porcelain=v1"}) || r.calls[0].cwd != "/work/issue-184" {
		t.Fatalf("call=%#v", r.calls[0])
	}
}

func TestCommitReturnsHeadSHA(t *testing.T) {
	r := &fakeRunner{results: []runner.Result{{Stdout: ""}, {Stdout: "commit output\n"}, {Stdout: "0123456789abcdef\n"}}}
	git := New(r, "git")
	got, err := git.Commit(context.Background(), "/work/issue-184", "메시지")
	if err != nil || got != "0123456789abcdef" {
		t.Fatalf("sha=%q err=%v", got, err)
	}
	want := [][]string{{"add", "-A"}, {"commit", "-m", "메시지"}, {"rev-parse", "HEAD"}}
	if len(r.calls) != len(want) {
		t.Fatalf("calls=%#v", r.calls)
	}
	for i := range want {
		if !reflect.DeepEqual(r.calls[i].args, want[i]) {
			t.Fatalf("call %d=%#v want=%#v", i, r.calls[i].args, want[i])
		}
	}
}

func TestMergeUsesBranchArgument(t *testing.T) {
	r := &fakeRunner{}
	git := New(r, "git")
	if err := git.Merge(context.Background(), "/work/issue-184", "agent/184"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls[0].args, []string{"merge", "--no-edit", "agent/184"}) {
		t.Fatalf("args=%#v", r.calls[0].args)
	}
}

func TestInspectCommitReturnsActualBoundedPatch(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	r := &fakeRunner{results: []runner.Result{{Stdout: sha + "\n"}, {Stdout: ""}, {Stdout: "src/payments/retry.go\n"}, {Stdout: "diff --git a/src/payments/retry.go b/src/payments/retry.go\n"}}}
	git := New(r, "git")
	got, err := git.InspectCommit(context.Background(), "/work/integration", sha, "agent/api", sha)
	if err != nil {
		t.Fatal(err)
	}
	if got.CommitSHA != sha || got.Branch != "agent/api" || len(got.ChangedFiles) != 1 || got.Patch == "" {
		t.Fatalf("inspection=%#v", got)
	}
}

func TestMergeCommitUsesImmutableSHA(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	r := &fakeRunner{}
	git := New(r, "git")
	if err := git.MergeCommit(context.Background(), "/work/integration", sha); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls[0].args, []string{"merge", "--ff-only", sha}) {
		t.Fatalf("call=%#v", r.calls[0].args)
	}
}

func TestReconcileIntegrationWorktreeRequiresContractBaseHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integration")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	base := "0123456789abcdef0123456789abcdef01234567"
	current := "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	r := &fakeRunner{results: []runner.Result{{Stdout: ""}, {Stdout: "agent/integration\n"}, {Stdout: current + "\n"}}}
	git := New(r, "git")
	found, err := git.ReconcileIntegrationWorktree(context.Background(), path, "agent/integration", base)
	if err == nil || found || !strings.Contains(err.Error(), "base commit") {
		t.Fatalf("found=%v err=%v want base commit mismatch", found, err)
	}
}

func TestReconcileIntegrationWorktreeReturnsNotFoundForMissingPath(t *testing.T) {
	git := New(&fakeRunner{}, "git")
	missing := filepath.Join(t.TempDir(), "not-created")
	found, err := git.ReconcileIntegrationWorktree(context.Background(), missing, "agent/integration", "0123456789abcdef0123456789abcdef01234567")
	if err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestRemoveSafeRejectsDangerousTargets(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	managed := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	git := New(r, "git")
	git.ManagedRoot = managed
	git.RepositoryRoot = repo
	homeAlias := filepath.Join(managed, "home-alias")
	if err := os.Symlink(home, homeAlias); err != nil {
		t.Fatal(err)
	}
	repoAlias := filepath.Join(managed, "repo-alias")
	if err := os.Symlink(repo, repoAlias); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", "/", home, homeAlias, repo, repoAlias, managed} {
		r := &fakeRunner{}
		git.Runner = r
		if err := git.RemoveSafe(context.Background(), target); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("target %q err=%v", target, err)
		}
		if len(r.calls) != 0 {
			t.Errorf("target %q made calls=%#v", target, r.calls)
		}
	}
}

func TestRemoveSafeDoesNotForce(t *testing.T) {
	r := &fakeRunner{results: []runner.Result{{Stdout: ""}}}
	git, repo, target := configuredGit(t, r)
	if err := git.RemoveSafe(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	want := []fakeCall{
		{cwd: target, exec: "git", args: []string{"status", "--porcelain=v1"}},
		{cwd: repo, exec: "git", args: []string{"worktree", "remove", target}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
	if strings.Contains(strings.Join(r.calls[1].args, " "), "force") {
		t.Fatalf("force removal: %#v", r.calls[1].args)
	}
}

func TestRemoveSafeRejectsOutsideTarget(t *testing.T) {
	git, _, _ := configuredGit(t, &fakeRunner{})
	outside := filepath.Join(filepath.Dir(git.ManagedRoot), "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		outside,
		filepath.Join(git.ManagedRoot, "..", "outside"),
		filepath.Join(filepath.Dir(git.ManagedRoot), filepath.Base(git.ManagedRoot)+"-sibling"),
	} {
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := git.RemoveSafe(context.Background(), target); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("target %q err=%v", target, err)
		}
	}
}

func TestRemoveSafeRejectsSymlinkEscape(t *testing.T) {
	git, _, _ := configuredGit(t, &fakeRunner{})
	escaped := filepath.Join(filepath.Dir(git.ManagedRoot), "escaped")
	if err := os.Mkdir(escaped, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(git.ManagedRoot, "link")
	if err := os.Symlink(escaped, link); err != nil {
		t.Fatal(err)
	}
	if err := git.RemoveSafe(context.Background(), link); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("err=%v", err)
	}
}

func TestRemoveSafeRejectsResolutionFailures(t *testing.T) {
	git, _, target := configuredGit(t, &fakeRunner{})
	for name, mutate := range map[string]func(){
		"managed root":    func() { git.ManagedRoot = filepath.Join(git.ManagedRoot, "missing") },
		"repository root": func() { git.RepositoryRoot = filepath.Join(git.RepositoryRoot, "missing") },
		"target":          func() { target = filepath.Join(target, "missing") },
	} {
		mutate()
		if err := git.RemoveSafe(context.Background(), target); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("%s err=%v", name, err)
		}
	}
}

func TestRemoveSafeRequiresConfiguredRoots(t *testing.T) {
	r := &fakeRunner{}
	git := New(r, "git")
	if err := git.RemoveSafe(context.Background(), t.TempDir()); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("err=%v", err)
	}
}

func configuredGit(t *testing.T, r *fakeRunner) (*Git, string, string) {
	t.Helper()
	managed := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	target := filepath.Join(managed, "worktree")
	for _, path := range []string{repo, target} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	git := New(r, "git")
	git.ManagedRoot = managed
	git.RepositoryRoot = repo
	return git, repo, target
}

func TestCreateManagedWorktreeUsesExplicitBaseAndValidatesTarget(t *testing.T) {
	r := &fakeRunner{}
	git, repo, _ := configuredGit(t, r)
	target := filepath.Join(git.ManagedRoot, "new-revert")
	if err := git.CreateManagedWorktree(context.Background(), repo, target, "revert/184-0123456789ab", "0123456789abcdef0123456789abcdef01234567"); err != nil {
		t.Fatal(err)
	}
	want := fakeCall{cwd: repo, exec: "git", args: []string{"worktree", "add", "-b", "revert/184-0123456789ab", target, "0123456789abcdef0123456789abcdef01234567"}}
	if !reflect.DeepEqual(r.calls, []fakeCall{want}) {
		t.Fatalf("calls=%#v want=%#v", r.calls, []fakeCall{want})
	}
}

func TestRevertMergeCommitUsesMainlineAndConfirmsConflict(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	r := &fakeRunner{results: []runner.Result{{ExitCode: 1}, {Stdout: "UU src/file.go\n"}}, errors: []error{errors.New("conflict"), nil}}
	git, _, target := configuredGit(t, r)
	err := git.RevertMergeCommit(context.Background(), target, sha)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	want := []fakeCall{
		{cwd: target, exec: "git", args: []string{"revert", "-m", "1", "--no-edit", sha}},
		{cwd: target, exec: "git", args: []string{"status", "--porcelain=v1"}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
}

func TestAbortRevertAndPushBranchNeverForce(t *testing.T) {
	r := &fakeRunner{}
	git, _, target := configuredGit(t, r)
	if err := git.AbortRevert(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if err := git.PushBranch(context.Background(), target, "origin", "revert/184-0123456789ab"); err != nil {
		t.Fatal(err)
	}
	want := []fakeCall{
		{cwd: target, exec: "git", args: []string{"revert", "--abort"}},
		{cwd: target, exec: "git", args: []string{"push", "origin", "revert/184-0123456789ab:revert/184-0123456789ab"}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
	if strings.Contains(strings.Join(r.calls[1].args, " "), "--force") || strings.Contains(strings.Join(r.calls[1].args, " "), " -f") {
		t.Fatalf("force push: %#v", r.calls[1].args)
	}
}
