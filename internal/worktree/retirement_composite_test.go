package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"thread-dock/internal/runner"
)

func TestCompositeRetirementInspectorSelectsManagedOrHerdrRoot(t *testing.T) {
	herdrGit, repo, herdrRoot, herdrTarget, sha := realRetirementRepo(t)
	stateRoot := filepath.Join(filepath.Dir(herdrRoot), "state", "worktrees")
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	stateTarget := filepath.Join(stateRoot, "state-target")
	runSetupGit(t, repo, "worktree", "add", "-b", "agent/state", stateTarget, sha)
	stateGit := New(runner.OSRunner{}, "git", stateRoot, repo)
	composite := NewCompositeRetirementInspector(stateGit, herdrGit, stateRoot, herdrRoot)

	for _, test := range []struct {
		name, path, branch string
	}{
		{name: "managed", path: stateTarget, branch: "agent/state"},
		{name: "herdr", path: herdrTarget, branch: "agent/task"},
	} {
		t.Run(test.name, func(t *testing.T) {
			proof, err := composite.InspectRetirementTarget(context.Background(), repo, test.path, test.branch, sha)
			if err != nil {
				t.Fatal(err)
			}
			if proof.Path != test.path || proof.HeadSHA != sha {
				t.Fatalf("proof=%+v", proof)
			}
		})
	}
	if _, err := composite.InspectRetirementTargetForRole(context.Background(), repo, "builder", stateTarget, "agent/state", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("builder accepted state-managed path: %v", err)
	}
	if _, err := composite.InspectRetirementTargetForRole(context.Background(), repo, "reviewer", herdrTarget, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("reviewer accepted Herdr path: %v", err)
	}
}

func TestCompositeRetirementInspectorRejectsOverlapOutsideAndSymlink(t *testing.T) {
	herdrGit, repo, herdrRoot, herdrTarget, sha := realRetirementRepo(t)
	stateRoot := filepath.Join(filepath.Dir(herdrRoot), "state", "worktrees")
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	stateGit := New(runner.OSRunner{}, "git", stateRoot, repo)

	overlap := NewCompositeRetirementInspector(stateGit, herdrGit, filepath.Dir(herdrRoot), herdrRoot)
	if _, err := overlap.InspectRetirementTarget(context.Background(), repo, herdrTarget, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("overlap err=%v", err)
	}
	outside := NewCompositeRetirementInspector(stateGit, herdrGit, stateRoot, herdrRoot)
	if _, err := outside.InspectRetirementTarget(context.Background(), repo, repo, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("outside err=%v", err)
	}
	symlink := filepath.Join(herdrRoot, "symlink-target")
	if err := os.Symlink(herdrTarget, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := outside.InspectRetirementTarget(context.Background(), repo, symlink, "agent/task", sha); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("symlink err=%v", err)
	}
}
