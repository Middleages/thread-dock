// Package worktree contains safe, explicit Git Worktree operations.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"thread-dock/internal/runner"
)

var (
	ErrDirtyWorktree = errors.New("worktree is dirty")
	ErrUnsafeTarget  = errors.New("unsafe worktree target")
)

// Git is an adapter for the small set of Git commands ThreadDock needs.
type Git struct {
	Runner         runner.Runner
	Binary         string
	RepositoryRoot string
}

// New constructs a Git adapter. The optional binary defaults to git. An
// absolute optional argument is treated as the canonical repository root;
// this also keeps New(runner, repositoryRoot) convenient for callers.
func New(processRunner runner.Runner, options ...string) *Git {
	binary := "git"
	var repositoryRoot string
	for _, option := range options {
		if strings.TrimSpace(option) == "" {
			continue
		}
		if filepath.IsAbs(option) && repositoryRoot == "" {
			repositoryRoot = option
			continue
		}
		binary = option
	}
	return &Git{Runner: processRunner, Binary: binary, RepositoryRoot: repositoryRoot}
}

func (g *Git) Create(ctx context.Context, repositoryPath, worktreePath, branch, base string) error {
	if err := require(g, repositoryPath, worktreePath, branch, base); err != nil {
		return err
	}
	return g.run(ctx, repositoryPath, "worktree", "add", "-b", branch, worktreePath, base)
}

func (g *Git) Status(ctx context.Context, worktreePath string) (string, error) {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return "", err
	}
	result, err := g.command(ctx, worktreePath, "status", "--porcelain=v1")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

func (g *Git) Commit(ctx context.Context, worktreePath, message string) (string, error) {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return "", errors.New("commit message is required")
	}
	if _, err := g.command(ctx, worktreePath, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := g.command(ctx, worktreePath, "commit", "-m", message); err != nil {
		return "", err
	}
	result, err := g.command(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (g *Git) Merge(ctx context.Context, worktreePath, branch string) error {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return err
	}
	if strings.TrimSpace(branch) == "" {
		return errors.New("merge branch is required")
	}
	return g.run(ctx, worktreePath, "merge", "--no-edit", branch)
}

func (g *Git) RemoveSafe(ctx context.Context, worktreePath string) error {
	if err := g.validateRemovalTarget(worktreePath); err != nil {
		return err
	}
	status, err := g.Status(ctx, worktreePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w: %s", ErrDirtyWorktree, strings.TrimSpace(status))
	}
	cwd := g.RepositoryRoot
	if cwd == "" {
		cwd = filepath.Dir(worktreePath)
	}
	return g.run(ctx, cwd, "worktree", "remove", worktreePath)
}

func (g *Git) run(ctx context.Context, cwd string, args ...string) error {
	_, err := g.command(ctx, cwd, args...)
	return err
}

func (g *Git) command(ctx context.Context, cwd string, args ...string) (runner.Result, error) {
	if g == nil || g.Runner == nil {
		return runner.Result{}, errors.New("runner is required")
	}
	result, err := g.Runner.Run(ctx, cwd, g.Binary, args...)
	if err != nil {
		return result, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return result, nil
}

func require(g *Git, values ...string) error {
	if g == nil || g.Runner == nil {
		return errors.New("runner is required")
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return errors.New("git argument is required")
		}
	}
	return nil
}

func (g *Git) validateWorktreePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrUnsafeTarget
	}
	return nil
}

func (g *Git) validateRemovalTarget(path string) error {
	if err := g.validateWorktreePath(path); err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve worktree target: %w", err)
	}
	absPath = filepath.Clean(absPath)
	if absPath == filepath.VolumeName(absPath)+string(filepath.Separator) {
		return ErrUnsafeTarget
	}
	home, err := os.UserHomeDir()
	if err == nil && samePath(absPath, home) {
		return ErrUnsafeTarget
	}
	if g != nil && g.RepositoryRoot != "" {
		root, err := filepath.Abs(g.RepositoryRoot)
		if err == nil && samePath(absPath, root) {
			return ErrUnsafeTarget
		}
	}
	return nil
}

func samePath(left, right string) bool {
	left = canonicalPath(left)
	right = canonicalPath(right)
	return left == right
}

func canonicalPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return path
}
