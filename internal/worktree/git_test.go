package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestFetchRemoteHeadUsesExplicitRemoteRef(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	runner := &fakeRunner{results: []runner.Result{{ExitCode: 0}, {ExitCode: 0, Stdout: sha + "\n"}}}
	git := &Git{Runner: runner, Binary: "git"}
	got, err := git.FetchRemoteHead(context.Background(), "/repo", "origin", "main")
	if err != nil || got != sha {
		t.Fatalf("sha=%q err=%v", got, err)
	}
	if len(runner.calls) != 2 || !reflect.DeepEqual(runner.calls[0].args, []string{"fetch", "--no-tags", "origin", "main"}) || !reflect.DeepEqual(runner.calls[1].args, []string{"rev-parse", "refs/remotes/origin/main"}) {
		t.Fatalf("calls=%v", runner.calls)
	}
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

func TestCreateManagedWorktreeRejectsConfiguredRepositoryMismatchBeforeCommand(t *testing.T) {
	r := &fakeRunner{}
	git, _, _ := configuredGit(t, r)
	other := filepath.Join(t.TempDir(), "other-repo")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	err := git.CreateManagedWorktree(context.Background(), other, filepath.Join(git.ManagedRoot, "new"), "revert/184", "0123456789abcdef0123456789abcdef01234567")
	if !errors.Is(err, ErrUnsafeTarget) || len(r.calls) != 0 {
		t.Fatalf("calls=%#v err=%v", r.calls, err)
	}
}

func TestCreateManagedWorktreeRejectsUntrustedManagedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix ownership and mode invariant")
	}
	r := &fakeRunner{}
	git, repo, _ := configuredGit(t, r)
	if err := os.Chmod(git.ManagedRoot, 0770); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(git.ManagedRoot, 0700) }()
	err := git.CreateManagedWorktree(context.Background(), repo, filepath.Join(git.ManagedRoot, "new"), "revert/184", "0123456789abcdef0123456789abcdef01234567")
	if !errors.Is(err, ErrUnsafeTarget) || len(r.calls) != 0 {
		t.Fatalf("calls=%#v err=%v", r.calls, err)
	}
}

func TestValidateTrustedManagedRootIsPlatformSafe(t *testing.T) {
	managed := t.TempDir()
	err := ValidateTrustedManagedRoot(managed)
	if runtime.GOOS == "windows" {
		if !errors.Is(err, ErrUnsafeTarget) {
			t.Fatalf("windows err=%v want unsupported rejection", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("unix err=%v", err)
	}
}

func TestReconcileRevertWorktreeReportsReadyAtExactBase(t *testing.T) {
	base := "0123456789abcdef0123456789abcdef01234567"
	git, repo, target := configuredGit(t, &fakeRunner{})
	commonDir := filepath.Join(repo, ".git")
	if err := os.Mkdir(commonDir, 0700); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{results: []runner.Result{{Stdout: commonDir + "\n"}, {Stdout: commonDir + "\n"}, {Stdout: "revert/184-0123456789ab\n"}, {Stdout: ""}, {Stdout: base + "\n"}}}
	git.Runner = r
	status, err := git.ReconcileRevertWorktree(context.Background(), repo, target, "revert/184-0123456789ab", base, "89abcdef0123456789abcdef0123456789abcdef")
	if err != nil || !status.Exists || !status.Ready || status.Reverted {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if len(r.calls) != 5 || !reflect.DeepEqual(r.calls[0].args, []string{"rev-parse", "--git-common-dir"}) || !reflect.DeepEqual(r.calls[1].args, []string{"rev-parse", "--git-common-dir"}) || !reflect.DeepEqual(r.calls[2].args, []string{"rev-parse", "--abbrev-ref", "HEAD"}) || !reflect.DeepEqual(r.calls[3].args, []string{"status", "--porcelain=v1"}) || !reflect.DeepEqual(r.calls[4].args, []string{"rev-parse", "HEAD"}) {
		t.Fatalf("calls=%#v", r.calls)
	}
}

func TestReconcileRevertWorktreeReportsExactRevertCommit(t *testing.T) {
	base := "0123456789abcdef0123456789abcdef01234567"
	merge := "89abcdef0123456789abcdef0123456789abcdef"
	git, repo, target := configuredGit(t, &fakeRunner{})
	commonDir := filepath.Join(repo, ".git")
	if err := os.Mkdir(commonDir, 0700); err != nil {
		t.Fatal(err)
	}
	head := "abcdef0123456789abcdef0123456789abcdef01"
	r := &fakeRunner{results: []runner.Result{{Stdout: commonDir + "\n"}, {Stdout: commonDir + "\n"}, {Stdout: "revert/184-0123456789ab\n"}, {Stdout: ""}, {Stdout: head + "\n"}, {Stdout: head + " " + base + "\n"}, {Stdout: "Revert change\n\nThis reverts commit " + merge + ",\n"}, {Stdout: "src/file.go\n"}}}
	git.Runner = r
	status, err := git.ReconcileRevertWorktree(context.Background(), repo, target, "revert/184-0123456789ab", base, merge)
	if err != nil || !status.Exists || status.Ready || !status.Reverted {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestInspectRevertWorktreeReturnsExistingBaseAndStageWithoutRequestedBase(t *testing.T) {
	base := "0123456789abcdef0123456789abcdef01234567"
	merge := "89abcdef0123456789abcdef0123456789abcdef"
	head := "abcdef0123456789abcdef0123456789abcdef01"
	git, repo, target := configuredGit(t, &fakeRunner{})
	commonDir := filepath.Join(repo, ".git")
	if err := os.Mkdir(commonDir, 0700); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{results: []runner.Result{
		{Stdout: commonDir + "\n"}, {Stdout: commonDir + "\n"},
		{Stdout: "revert/184-0123456789ab\n"}, {Stdout: ""},
		{Stdout: head + "\n"}, {Stdout: head + " " + base + "\n"},
		{Stdout: "Revert change\n\nThis reverts commit " + merge + ".\n"},
		{Stdout: "src/file.go\n"},
	}}
	git.Runner = r
	got, err := git.InspectRevertWorktree(context.Background(), repo, target, "revert/184-0123456789ab", merge)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists || got.Stage != "reverted" || got.HeadCommit != head || got.BaseCommit != base || got.ParentCommit != base {
		t.Fatalf("inspection=%+v", got)
	}
}

func TestReconcileRevertWorktreeRejectsWrongParentOrEmptyDiff(t *testing.T) {
	base := "0123456789abcdef0123456789abcdef01234567"
	merge := "89abcdef0123456789abcdef0123456789abcdef"
	git, repo, target := configuredGit(t, &fakeRunner{})
	commonDir := filepath.Join(repo, ".git")
	if err := os.Mkdir(commonDir, 0700); err != nil {
		t.Fatal(err)
	}
	head := "abcdef0123456789abcdef0123456789abcdef01"
	for name, parents := range map[string]string{"wrong parent": head + " 1111111111111111111111111111111111111111\n", "multiple parents": head + " " + base + " 1111111111111111111111111111111111111111\n"} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{results: []runner.Result{{Stdout: commonDir + "\n"}, {Stdout: commonDir + "\n"}, {Stdout: "revert/184-0123456789ab\n"}, {Stdout: ""}, {Stdout: head + "\n"}, {Stdout: parents}, {Stdout: "This reverts commit " + merge + ".\n"}, {Stdout: "src/file.go\n"}}}
			git.Runner = r
			status, err := git.ReconcileRevertWorktree(context.Background(), repo, target, "revert/184-0123456789ab", base, merge)
			if !errors.Is(err, ErrUnsafeTarget) || status.Exists {
				t.Fatalf("status=%+v err=%v", status, err)
			}
		})
	}
	r := &fakeRunner{results: []runner.Result{{Stdout: commonDir + "\n"}, {Stdout: commonDir + "\n"}, {Stdout: "revert/184-0123456789ab\n"}, {Stdout: ""}, {Stdout: head + "\n"}, {Stdout: head + " " + base + "\n"}, {Stdout: "This reverts commit " + merge + ".\n"}, {Stdout: "\n"}}}
	git.Runner = r
	status, err := git.ReconcileRevertWorktree(context.Background(), repo, target, "revert/184-0123456789ab", base, merge)
	if !errors.Is(err, ErrUnsafeTarget) || status.Exists {
		t.Fatalf("empty diff status=%+v err=%v", status, err)
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

func TestRemoveRetiredRequiresExactRegisteredCleanWorktree(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Path != target || proof.Branch != "agent/task" || proof.HeadSHA != sha {
		t.Fatalf("proof=%+v", proof)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target remains: %v", err)
	}
	if strings.TrimSpace(runSetupGit(t, repo, "branch", "--list", "agent/task")) == "" {
		t.Fatal("retirement removed the branch")
	}
}

func TestInspectRetirementTargetRejectsDirtyWorktree(t *testing.T) {
	git, repo, _, target, sha := realRetirementRepo(t)
	writeTestFile(t, filepath.Join(target, "dirty.txt"), "dirty\n")
	if _, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("err=%v, want ErrUnsafeTarget", err)
	}
}

func TestRemoveRetiredRejectsMovedHeadBeforeMutation(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	writeTestFile(t, filepath.Join(target, "new.txt"), "new\n")
	runSetupGit(t, target, "add", "new.txt")
	runSetupGit(t, target, "commit", "-m", "new commit")
	proof := RetirementProof{RepositoryCommonDir: filepath.Join(repo, ".git"), Path: target, Branch: "agent/task", HeadSHA: sha}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("err=%v, want ErrUnsafeTarget", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target was removed after moved HEAD: %v", err)
	}
}

func TestRemoveRetiredAllowsResponseLossOnlyWhenTargetAndRegistrationAreGone(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); err != nil {
		t.Fatal(err)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); err != nil {
		t.Fatalf("second removal should be idempotent after response loss: %v", err)
	}
}

func TestRemoveRetiredRejectsContradictoryStaleRegistration(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	// Recreate an administrative registration at the original path without a
	// checkout. RemoveRetired must not mistake a stale registration for a
	// completed response.
	gitDir := filepath.Join(repo, ".git", "worktrees", "stale")
	if err := os.MkdirAll(gitDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(gitDir, "gitdir"), filepath.Join(target, ".git")+"\n")
	writeTestFile(t, filepath.Join(gitDir, "HEAD"), sha+"\n")
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("err=%v, want ErrUnsafeTarget", err)
	}
}

func TestInspectRetirementTargetRejectsUnregisteredCheckoutAndSiblingEscape(t *testing.T) {
	git, repo, herdrRoot, _, sha := realRetirementRepo(t)
	foreign := filepath.Join(filepath.Dir(herdrRoot), filepath.Base(herdrRoot)+"-sibling")
	runSetupGit(t, repo, "worktree", "add", "--detach", foreign, sha)
	if _, err := git.InspectRetirementTarget(context.Background(), repo, foreign, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("sibling target err=%v, want ErrUnsafeTarget", err)
	}
}

func TestRetirementSupportsConfiguredRepositoryAsLinkedWorktree(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	configured := filepath.Join(t.TempDir(), "configured repository")
	runSetupGit(t, repo, "worktree", "add", "--detach", configured, sha)
	proof, err := git.InspectRetirementTarget(context.Background(), configured, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	if proof.RepositoryCommonDir != filepath.Join(repo, ".git") {
		t.Fatalf("common dir=%q, want %q", proof.RepositoryCommonDir, filepath.Join(repo, ".git"))
	}
	if err := git.RemoveRetired(context.Background(), configured, herdrRoot, proof); err != nil {
		t.Fatal(err)
	}
}

func TestInspectRetirementTargetRejectsDetachedAndForeignWorktrees(t *testing.T) {
	git, repo, herdrRoot, _, sha := realRetirementRepo(t)
	detached := filepath.Join(herdrRoot, "detached")
	runSetupGit(t, repo, "worktree", "add", "--detach", detached, sha)
	if _, err := git.InspectRetirementTarget(context.Background(), repo, detached, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("detached target err=%v, want ErrUnsafeTarget", err)
	}
	foreign := filepath.Join(herdrRoot, "foreign")
	runSetupGit(t, "", "init", foreign)
	runSetupGit(t, foreign, "config", "user.email", "test@example.com")
	runSetupGit(t, foreign, "config", "user.name", "ThreadDock Test")
	writeTestFile(t, filepath.Join(foreign, "README.md"), "foreign\n")
	runSetupGit(t, foreign, "add", "README.md")
	runSetupGit(t, foreign, "commit", "-m", "foreign")
	if _, err := git.InspectRetirementTarget(context.Background(), repo, foreign, "master", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("foreign target err=%v, want ErrUnsafeTarget", err)
	}
}

func TestInspectRetirementTargetRejectsRootsAndMissingConfiguration(t *testing.T) {
	git, repo, herdrRoot, _, sha := realRetirementRepo(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{herdrRoot, repo, home, "/"} {
		if _, err := git.InspectRetirementTarget(context.Background(), repo, path, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("target %q err=%v, want ErrUnsafeTarget", path, err)
		}
	}
	git.ManagedRoot = filepath.Join(herdrRoot, "missing")
	if _, err := git.InspectRetirementTarget(context.Background(), repo, filepath.Join(herdrRoot, "target"), "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("missing Herdr root err=%v, want ErrUnsafeTarget", err)
	}
	git.ManagedRoot = herdrRoot
	git.RepositoryRoot = filepath.Join(repo, "missing")
	if _, err := git.InspectRetirementTarget(context.Background(), repo, filepath.Join(herdrRoot, "target"), "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("missing repository root err=%v, want ErrUnsafeTarget", err)
	}
}

func TestRemoveRetiredRejectsMovedRegisteredTarget(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(herdrRoot, "moved target")
	runSetupGit(t, repo, "worktree", "move", target, moved)
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("err=%v, want ErrUnsafeTarget", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("moved target was removed: %v", err)
	}
}

func TestRetirementRejectsSymlinkProofPathWithoutRemovingTarget(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(herdrRoot, "symlink target")
	runSetupGit(t, repo, "worktree", "move", target, moved)
	if err := os.Symlink(moved, target); err != nil {
		t.Fatal(err)
	}
	trace := &recordingRunner{}
	git.Runner = trace
	if _, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("Inspect symlink err=%v, want ErrUnsafeTarget", err)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("Remove symlink err=%v, want ErrUnsafeTarget", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("symlink target was removed: %v", err)
	}
	assertNoWorktreeRemove(t, trace.calls)
}

func TestRetirementRejectsUnregisteredOrdinaryDirectoryWithoutMutation(t *testing.T) {
	git, repo, herdrRoot, _, sha := realRetirementRepo(t)
	ordinary := filepath.Join(herdrRoot, "ordinary directory")
	if err := os.Mkdir(ordinary, 0700); err != nil {
		t.Fatal(err)
	}
	proof := RetirementProof{RepositoryCommonDir: filepath.Join(repo, ".git"), Path: ordinary, Branch: "agent/task", HeadSHA: sha}
	trace := &recordingRunner{}
	git.Runner = trace
	if _, err := git.InspectRetirementTarget(context.Background(), repo, ordinary, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("Inspect ordinary directory err=%v, want ErrUnsafeTarget", err)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("Remove ordinary directory err=%v, want ErrUnsafeTarget", err)
	}
	if _, err := os.Stat(ordinary); err != nil {
		t.Fatalf("ordinary directory was removed: %v", err)
	}
	assertNoWorktreeRemove(t, trace.calls)
}

func TestRemoveRetiredRejectsCommonDirSymlinkAlias(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(herdrRoot), "common-dir-alias")
	if err := os.Symlink(proof.RepositoryCommonDir, alias); err != nil {
		t.Fatal(err)
	}
	proof.RepositoryCommonDir = alias
	trace := &recordingRunner{}
	git.Runner = trace
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("CommonDir alias err=%v, want ErrUnsafeTarget", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target was removed after CommonDir alias: %v", err)
	}
	assertNoWorktreeRemove(t, trace.calls)
}

func TestRemoveRetiredAllowsResponseLossWithDifferentBranchAtSameHead(t *testing.T) {
	git, repo, herdrRoot, target, sha := realRetirementRepo(t)
	other := filepath.Join(herdrRoot, "other branch")
	runSetupGit(t, repo, "worktree", "add", "-b", "agent/other", other, sha)
	proof, err := git.InspectRetirementTarget(context.Background(), repo, target, "agent/task", sha)
	if err != nil {
		t.Fatal(err)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); err != nil {
		t.Fatal(err)
	}
	if err := git.RemoveRetired(context.Background(), repo, herdrRoot, proof); err != nil {
		t.Fatalf("same-head different-branch retry err=%v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("different-branch worktree was removed: %v", err)
	}
}

func assertNoWorktreeRemove(t *testing.T, calls []fakeCall) {
	t.Helper()
	for _, call := range calls {
		if len(call.args) >= 2 && call.args[0] == "worktree" && call.args[1] == "remove" {
			t.Fatalf("unsafe worktree removal call: %#v", call)
		}
	}
}

type recordingRunner struct {
	calls []fakeCall
}

func (r *recordingRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	r.calls = append(r.calls, fakeCall{cwd: cwd, exec: executable, args: append([]string(nil), args...)})
	return (runner.OSRunner{}).Run(ctx, cwd, executable, args...)
}

func realRetirementRepo(t *testing.T) (*Git, string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	herdrRoot := filepath.Join(root, "herdr", "worktrees")
	if err := os.MkdirAll(herdrRoot, 0700); err != nil {
		t.Fatal(err)
	}
	runSetupGit(t, "", "init", "--bare", bare)
	runSetupGit(t, "", "clone", bare, repo)
	runSetupGit(t, repo, "config", "user.email", "test@example.com")
	runSetupGit(t, repo, "config", "user.name", "ThreadDock Test")
	writeTestFile(t, filepath.Join(repo, "README.md"), "base\n")
	runSetupGit(t, repo, "add", "README.md")
	runSetupGit(t, repo, "commit", "-m", "base")
	sha := strings.TrimSpace(runSetupGit(t, repo, "rev-parse", "HEAD"))
	target := filepath.Join(herdrRoot, "odd target [retired]")
	runSetupGit(t, repo, "worktree", "add", "-b", "agent/task", target, sha)
	return New(runner.OSRunner{}, "git", herdrRoot, repo), repo, herdrRoot, target, sha
}
