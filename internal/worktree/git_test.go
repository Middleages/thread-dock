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
	git := New(&fakeRunner{results: []runner.Result{{Stdout: " M src/pay.go\n"}}}, "git")
	err := git.RemoveSafe(context.Background(), "/work/issue-184")
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

func TestRemoveSafeRejectsDangerousTargets(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, target := range []string{"", "/", home, root} {
		r := &fakeRunner{}
		git := New(r, "git")
		git.RepositoryRoot = root
		if err := git.RemoveSafe(context.Background(), target); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("target %q err=%v", target, err)
		}
		if len(r.calls) != 0 {
			t.Errorf("target %q made calls=%#v", target, r.calls)
		}
	}
}

func TestRemoveSafeDoesNotForce(t *testing.T) {
	r := &fakeRunner{}
	git := New(r, "git")
	git.RepositoryRoot = filepath.Join(t.TempDir(), "repo")
	if err := git.RemoveSafe(context.Background(), filepath.Join(t.TempDir(), "worktree")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(r.calls[0].args, " "), "force") {
		t.Fatalf("force removal: %#v", r.calls[0].args)
	}
}
