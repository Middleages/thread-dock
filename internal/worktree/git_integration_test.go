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
	if err := New(r, "git").MergeCommitNoFF(context.Background(), "/work/integration", sha); err != nil {
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
	err := New(r, "git").MergeCommitNoFF(context.Background(), "/work/integration", sha)
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
	err := New(r, "git").MergeCommitNoFF(context.Background(), "/work/integration", sha)
	if errors.Is(err, ErrConflict) || err == nil {
		t.Fatalf("err=%v", err)
	}
}

func TestAbortMergeUsesAbortOnly(t *testing.T) {
	r := &fakeRunner{}
	if err := New(r, "git").AbortMerge(context.Background(), "/work/integration"); err != nil {
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
	git := New(r, "git")
	git.Clock = clock
	checks, err := git.RunChecks(context.Background(), "/work/integration", []string{" go test ./... ", "go vet ./..."})
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
	git := New(r, "git")
	if _, err := git.RunChecks(context.Background(), "/work/integration", []string{"  "}); err == nil {
		t.Fatal("expected empty command rejection")
	}
	if _, err := git.RunChecks(context.Background(), "/work/integration", []string{"go test ./..."}); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("err=%v", err)
	}
}

func TestFingerprintUsesCommitPathsAndObjectIDs(t *testing.T) {
	path := t.TempDir()
	if err := os.MkdirAll(filepath.Join(path, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for name := range map[string]string{"src/z.go": "worktree", "untracked.txt": "untracked"} {
		if err := os.WriteFile(filepath.Join(path, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := &fakeRunner{results: []runner.Result{
		{Stdout: "0123456789abcdef0123456789abcdef01234567\n"},
		{Stdout: " M src/z.go\nD  src/deleted.go\n?? untracked.txt\n"},
		{Stdout: "100644 0000000000000000000000000000000000000000 0 src/deleted.go\n"},
		{},
		{Stdout: "2222222222222222222222222222222222222222\n"},
		{Stdout: "100644 1111111111111111111111111111111111111111 0 src/z.go\n"},
		{Stdout: "3333333333333333333333333333333333333333\n"},
	}}
	got, err := New(r, "git").Fingerprint(context.Background(), path)
	if err != nil || len(got) != 64 {
		t.Fatalf("fingerprint=%q err=%v", got, err)
	}
	if strings.Contains(got, "src/") || strings.Contains(got, "git diff") {
		t.Fatalf("fingerprint contains raw output: %q", got)
	}
	if !reflect.DeepEqual(r.calls[1].args, []string{"status", "--porcelain=v1", "-uall", "--no-renames"}) {
		t.Fatalf("status call=%#v", r.calls[1].args)
	}
}

type testClock struct{ times []time.Time }

func (c *testClock) Now() time.Time {
	if len(c.times) == 0 {
		return time.Now()
	}
	now := c.times[0]
	c.times = c.times[1:]
	return now
}
