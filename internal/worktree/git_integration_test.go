package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

func TestMergeCommitNoFFUsesImmutableSHA(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	r := &fakeRunner{}
	git, _, target := configuredGit(t, r)
	if err := git.MergeCommitNoFF(context.Background(), target, sha); err != nil {
		t.Fatal(err)
	}
	want := []string{"merge", "--no-ff", "--no-edit", sha}
	if !reflect.DeepEqual(r.calls[0].args, want) {
		t.Fatalf("args=%#v want=%#v", r.calls[0].args, want)
	}
}

func TestMergeCommitNoFFConfirmsUnmergedPathBeforeConflict(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	r := &fakeRunner{results: []runner.Result{{ExitCode: 1, Stderr: "CONFLICT (content): merge conflict"}, {Stdout: "UU src/file.go\n"}}, errors: []error{errors.New("exit status 1")}}
	git, _, target := configuredGit(t, r)
	err := git.MergeCommitNoFF(context.Background(), target, sha)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 2 || !reflect.DeepEqual(r.calls[1].args, []string{"status", "--porcelain=v1"}) {
		t.Fatalf("calls=%#v", r.calls)
	}
}

func TestMergeCommitNoFFDoesNotClassifyUnrelatedErrorAsConflict(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	r := &fakeRunner{results: []runner.Result{{ExitCode: 1, Stderr: "fatal: repository unavailable"}}, errors: []error{errors.New("exit status 1")}}
	git, _, target := configuredGit(t, r)
	err := git.MergeCommitNoFF(context.Background(), target, sha)
	if errors.Is(err, ErrConflict) || err == nil {
		t.Fatalf("err=%v", err)
	}
}

func TestAbortMergeUsesAbortOnly(t *testing.T) {
	r := &fakeRunner{}
	git, _, target := configuredGit(t, r)
	if err := git.AbortMerge(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls[0].args, []string{"merge", "--abort"}) {
		t.Fatalf("args=%#v", r.calls[0].args)
	}
}

func TestRunChecksUsesBashAndNormalizesResultWithoutOutput(t *testing.T) {
	r := &fakeRunner{results: []runner.Result{{ExitCode: 0, Stdout: "passed output", Stderr: "diagnostic"}, {ExitCode: 2, Stdout: "failed output", Stderr: "secret"}}}
	start := time.Unix(100, 0)
	clock := &testClock{times: []time.Time{start, start.Add(1500 * time.Millisecond), start.Add(3 * time.Second)}}
	git, _, target := configuredGit(t, r)
	git.Clock = clock
	checks, err := git.RunChecks(context.Background(), target, []string{" go test ./... ", "go vet ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 || checks[0].Command != "go test ./..." || checks[0].Outcome != "passed" || checks[0].Duration != "1.5s" || checks[0].ExitCode != 0 || checks[1].Outcome != "failed" || checks[1].ExitCode != 2 {
		t.Fatalf("checks=%#v", checks)
	}
	if _, exposed := reflect.TypeOf(checks[0]).FieldByName("Output"); exposed {
		t.Fatal("raw output field exposed in verification result")
	}
	if !reflect.DeepEqual(r.calls[0].args, []string{"-lc", "go test ./..."}) || !reflect.DeepEqual(r.calls[1].args, []string{"-lc", "go vet ./..."}) {
		t.Fatalf("calls=%#v", r.calls)
	}
}

func TestRunChecksRejectsEmptyCommandAndInfrastructureError(t *testing.T) {
	r := &fakeRunner{errors: []error{errors.New("context canceled")}}
	git, _, target := configuredGit(t, r)
	if _, err := git.RunChecks(context.Background(), target, []string{"  "}); err == nil {
		t.Fatal("expected empty command rejection")
	}
	if _, err := git.RunChecks(context.Background(), target, []string{"go test ./..."}); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("err=%v", err)
	}
}

func TestFingerprintUsesCommitPathsAndObjectIDs(t *testing.T) {
	path := t.TempDir()
	managed := filepath.Join(path, "managed")
	repo := filepath.Join(path, "repo")
	target := filepath.Join(managed, "target")
	for _, dir := range []string{managed, repo, target} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(target, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for name := range map[string]string{"src/z.go": "worktree", "untracked.txt": "untracked"} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := &fakeRunner{results: []runner.Result{
		{Stdout: "0123456789abcdef0123456789abcdef01234567\n"},
		{Stdout: " M src/z.go\x00D  src/deleted.go\x00?? untracked.txt\x00"},
		{Stdout: "100644 0000000000000000000000000000000000000000 0 src/deleted.go\n"},
		{},
		{Stdout: "2222222222222222222222222222222222222222\n"},
		{Stdout: "100644 1111111111111111111111111111111111111111 0 src/z.go\n"},
		{Stdout: "3333333333333333333333333333333333333333\n"},
	}}
	git := New(r, "git", managed, repo)
	got, err := git.Fingerprint(context.Background(), target)
	if err != nil || len(got) != 64 {
		t.Fatalf("fingerprint=%q err=%v", got, err)
	}
	if strings.Contains(got, "src/") || strings.Contains(got, "git diff") {
		t.Fatalf("fingerprint contains raw output: %q", got)
	}
	if !reflect.DeepEqual(r.calls[1].args, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames"}) {
		t.Fatalf("status call=%#v", r.calls[1].args)
	}
}

func TestFingerprintPreservesNULSafeUnusualPathAndContentIdentity(t *testing.T) {
	git, _, target := configuredGit(t, &fakeRunner{})
	name := "odd\tline\nquote\".txt"
	if err := os.WriteFile(filepath.Join(target, name), []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	makeRunner := func(objectID string) *fakeRunner {
		return &fakeRunner{results: []runner.Result{
			{Stdout: "0123456789abcdef0123456789abcdef01234567\n"},
			{Stdout: "?? " + name + "\x00"},
			{},
			{Stdout: objectID + "\n"},
		}}
	}
	first := makeRunner("1111111111111111111111111111111111111111")
	git.Runner = first
	fingerprintOne, err := git.Fingerprint(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	second := makeRunner("2222222222222222222222222222222222222222")
	git.Runner = second
	fingerprintTwo, err := git.Fingerprint(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintOne == fingerprintTwo {
		t.Fatal("content object ID change did not change fingerprint")
	}
	for _, calls := range [][]fakeCall{first.calls, second.calls} {
		if len(calls) != 4 || calls[2].args[len(calls[2].args)-1] != name || calls[3].args[len(calls[3].args)-1] != name {
			t.Fatalf("unusual path was not preserved: %#v", calls)
		}
	}
}

func TestFingerprintWorktreeAllowsLinkedHerdrWorktreeOutsideManagedRoot(t *testing.T) {
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
	linked := filepath.Join(root, "herdr-worktree")
	runSetupGit(t, repository, "worktree", "add", "--detach", linked, "HEAD")
	managed := filepath.Join(root, "managed")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}

	git := New(runner.OSRunner{}, "git", managed, repository)
	got, err := git.FingerprintWorktree(context.Background(), linked)
	if err != nil || len(got) != 64 {
		t.Fatalf("fingerprint=%q err=%v", got, err)
	}
	if strings.Contains(got, "README") || strings.Contains(got, "git diff") {
		t.Fatalf("fingerprint exposed raw output: %q", got)
	}
	if _, err := git.FingerprintWorktree(context.Background(), repository); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("canonical repository fingerprint err=%v, want ErrUnsafeTarget", err)
	}
}

func TestFingerprintWorktreeRejectsForeignRepository(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "configured")
	foreign := filepath.Join(root, "foreign")
	for _, path := range []string{configured, foreign} {
		runSetupGit(t, "", "init", path)
		runSetupGit(t, path, "config", "user.email", "test@example.com")
		runSetupGit(t, path, "config", "user.name", "ThreadDock Test")
		writeTestFile(t, filepath.Join(path, "README.md"), path+"\n")
		runSetupGit(t, path, "add", "README.md")
		runSetupGit(t, path, "commit", "-m", "base")
	}
	managed := filepath.Join(root, "managed")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}
	git := New(runner.OSRunner{}, "git", managed, configured)
	if _, err := git.FingerprintWorktree(context.Background(), foreign); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("foreign repository fingerprint err=%v, want ErrUnsafeTarget", err)
	}
}

func TestNewOperationsRejectUnmanagedTargetsWithoutCommands(t *testing.T) {
	git, repo, target := configuredGit(t, &fakeRunner{})
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(t.TempDir(), "escaped")
	if err := os.Mkdir(escaped, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(git.ManagedRoot, "escape")
	if err := os.Symlink(escaped, link); err != nil {
		t.Fatal(err)
	}
	cases := []string{"", repo, outside, link, "/"}
	for _, path := range cases {
		for name, call := range map[string]func(string) error{
			"merge": func(path string) error {
				return git.MergeCommitNoFF(context.Background(), path, "0123456789abcdef0123456789abcdef01234567")
			},
			"abort": func(path string) error { return git.AbortMerge(context.Background(), path) },
			"checks": func(path string) error {
				_, err := git.RunChecks(context.Background(), path, []string{"true"})
				return err
			},
			"fingerprint": func(path string) error { _, err := git.Fingerprint(context.Background(), path); return err },
		} {
			fake := &fakeRunner{}
			git.Runner = fake
			if err := call(path); !errors.Is(err, ErrUnsafeTarget) {
				t.Errorf("%s target %q err=%v", name, path, err)
			}
			if len(fake.calls) != 0 {
				t.Errorf("%s target %q made calls=%#v", name, path, fake.calls)
			}
		}
	}
	git.Runner = &fakeRunner{}
	git.ManagedRoot = ""
	if err := git.AbortMerge(context.Background(), target); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("blank managed root err=%v", err)
	}
	git.ManagedRoot = filepath.Dir(target)
	git.RepositoryRoot = ""
	if _, err := git.Fingerprint(context.Background(), target); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("blank repository root err=%v", err)
	}
}

func TestRunChecksHonorsContextAndDistinguishesProcessExit(t *testing.T) {
	git, _, target := configuredGit(t, &fakeRunner{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := git.RunChecks(ctx, target, []string{"true"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context err=%v", err)
	}

	generic := errors.New("transport failed")
	git.Runner = &fakeRunner{results: []runner.Result{{ExitCode: 137}}, errors: []error{generic}}
	if _, err := git.RunChecks(context.Background(), target, []string{"true"}); !errors.Is(err, generic) {
		t.Fatalf("generic positive-exit error=%v", err)
	}

	git.Runner = &fakeRunner{results: []runner.Result{{ExitCode: 7}}, errors: []error{processExitError{code: 7}}}
	checks, err := git.RunChecks(context.Background(), target, []string{"true"})
	if err != nil || len(checks) != 1 || checks[0].Outcome != "failed" || checks[0].ExitCode != 7 {
		t.Fatalf("process exit checks=%#v err=%v", checks, err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	git.Runner = &fakeRunner{results: []runner.Result{{ExitCode: 137}}, errors: []error{processExitError{code: 137}}}
	cancel()
	if _, err := git.RunChecks(ctx, target, []string{"true"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled process err=%v", err)
	}
}

func TestMergeCommitNoFFRejectsNegativeExit(t *testing.T) {
	git, _, target := configuredGit(t, &fakeRunner{results: []runner.Result{{ExitCode: -1}}, errors: []error{nil}})
	if err := git.MergeCommitNoFF(context.Background(), target, "0123456789abcdef0123456789abcdef01234567"); err == nil {
		t.Fatal("expected negative merge exit failure")
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

type processExitError struct{ code int }

func (e processExitError) Error() string { return "process exited" }
func (e processExitError) ExitCode() int { return e.code }

type testClock struct{ times []time.Time }

func (c *testClock) Now() time.Time {
	if len(c.times) == 0 {
		return time.Now()
	}
	now := c.times[0]
	c.times = c.times[1:]
	return now
}
