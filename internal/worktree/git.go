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
	ManagedRoot    string
	RepositoryRoot string
}

// CommitInspection is derived from Git, not from Agent prose. Patch is the
// bounded unified diff that a Reviewer may inspect.
type CommitInspection struct {
	CommitSHA    string
	Branch       string
	ChangedFiles []string
	Patch        string
}

const maxReviewerPatchBytes = 512 * 1024

// New constructs a Git adapter. The optional binary defaults to git. An
// absolute optional argument is treated as the canonical repository root;
// this also keeps New(runner, repositoryRoot) convenient for callers.
func New(processRunner runner.Runner, options ...string) *Git {
	binary := "git"
	var absoluteOptions []string
	var repositoryRoot string
	for _, option := range options {
		if strings.TrimSpace(option) == "" {
			continue
		}
		if filepath.IsAbs(option) && repositoryRoot == "" {
			absoluteOptions = append(absoluteOptions, option)
			continue
		}
		binary = option
	}
	if len(absoluteOptions) == 1 {
		repositoryRoot = absoluteOptions[0]
	}
	git := &Git{Runner: processRunner, Binary: binary, RepositoryRoot: repositoryRoot}
	if len(absoluteOptions) >= 2 {
		git.ManagedRoot = absoluteOptions[0]
		git.RepositoryRoot = absoluteOptions[1]
	}
	return git
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

// InspectCommit verifies that sha exists, is reachable from branch, and
// returns the actual changed paths and bounded patch against base.
func (g *Git) InspectCommit(ctx context.Context, worktreePath, base, branch, sha string) (CommitInspection, error) {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return CommitInspection{}, err
	}
	if !isCommitSHA(sha) || strings.TrimSpace(branch) == "" || strings.TrimSpace(base) == "" {
		return CommitInspection{}, errors.New("commit inspection requires base, branch, and a 40-character SHA")
	}
	resolved, err := g.command(ctx, worktreePath, "rev-parse", sha+"^{commit}")
	if err != nil || strings.TrimSpace(resolved.Stdout) != sha {
		return CommitInspection{}, errors.New("commit SHA could not be resolved")
	}
	if _, err := g.command(ctx, worktreePath, "merge-base", "--is-ancestor", sha, branch); err != nil {
		return CommitInspection{}, errors.New("commit is not contained in the Builder branch")
	}
	names, err := g.command(ctx, worktreePath, "diff", "--name-only", base+".."+sha)
	if err != nil {
		return CommitInspection{}, err
	}
	patch, err := g.command(ctx, worktreePath, "diff", "--binary", base+".."+sha)
	if err != nil {
		return CommitInspection{}, err
	}
	if len(patch.Stdout) > maxReviewerPatchBytes {
		return CommitInspection{}, errors.New("reviewer patch exceeds the size limit")
	}
	files := splitLines(names.Stdout)
	return CommitInspection{CommitSHA: sha, Branch: branch, ChangedFiles: files, Patch: patch.Stdout}, nil
}

// MergeCommit merges the immutable, already-inspected SHA rather than a
// movable branch name. It never targets the canonical checkout implicitly.
func (g *Git) MergeCommit(ctx context.Context, worktreePath, sha string) error {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return err
	}
	if !isCommitSHA(sha) {
		return errors.New("merge requires a 40-character commit SHA")
	}
	return g.run(ctx, worktreePath, "merge", "--ff-only", sha)
}

func (g *Git) CurrentCommit(ctx context.Context, worktreePath string) (string, error) {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return "", err
	}
	result, err := g.command(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (g *Git) CurrentBranch(ctx context.Context, worktreePath string) (string, error) {
	if err := g.validateWorktreePath(worktreePath); err != nil {
		return "", err
	}
	result, err := g.command(ctx, worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

// ReconcileIntegrationWorktree is read-only. It verifies that the persisted
// path is usable and that its current branch and commit are still real; it
// never creates or mutates a Worktree.
func (g *Git) ReconcileIntegrationWorktree(ctx context.Context, path, branch, base string) (bool, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(branch) == "" || strings.TrimSpace(base) == "" {
		return false, ErrUnsafeTarget
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, ErrUnsafeTarget
	}
	if !info.IsDir() {
		return false, ErrUnsafeTarget
	}
	if _, err := g.Status(ctx, path); err != nil {
		return false, err
	}
	currentBranch, err := g.CurrentBranch(ctx, path)
	if err != nil || currentBranch != branch {
		return false, errors.New("integration Worktree branch does not match durable state")
	}
	currentCommit, err := g.CurrentCommit(ctx, path)
	if err != nil || !isCommitSHA(currentCommit) {
		return false, errors.New("integration Worktree commit is unavailable")
	}
	if currentCommit != base {
		return false, errors.New("integration Worktree HEAD does not match contract base commit")
	}
	return true, nil
}

func isCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func splitLines(output string) []string {
	var result []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

func (g *Git) RemoveSafe(ctx context.Context, worktreePath string) error {
	managedRoot, repositoryRoot, target, home, err := g.resolveRemovalPaths(worktreePath)
	if err != nil {
		return err
	}
	if !strictlyContained(managedRoot, target) || samePath(target, home) || samePath(target, repositoryRoot) {
		return ErrUnsafeTarget
	}
	status, err := g.statusAt(ctx, target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w: %s", ErrDirtyWorktree, strings.TrimSpace(status))
	}
	return g.run(ctx, repositoryRoot, "worktree", "remove", target)
}

func (g *Git) statusAt(ctx context.Context, worktreePath string) (string, error) {
	result, err := g.command(ctx, worktreePath, "status", "--porcelain=v1")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
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
	_, _, _, _, err := g.resolveRemovalPaths(path)
	return err
}

func (g *Git) resolveRemovalPaths(targetPath string) (managedRoot, repositoryRoot, target, home string, err error) {
	if err := g.validateWorktreePath(targetPath); err != nil {
		return "", "", "", "", err
	}
	if g == nil || strings.TrimSpace(g.ManagedRoot) == "" || strings.TrimSpace(g.RepositoryRoot) == "" {
		return "", "", "", "", ErrUnsafeTarget
	}
	managedRoot, err = resolvePath(g.ManagedRoot)
	if err != nil {
		return "", "", "", "", err
	}
	repositoryRoot, err = resolvePath(g.RepositoryRoot)
	if err != nil {
		return "", "", "", "", err
	}
	target, err = resolvePath(targetPath)
	if err != nil {
		return "", "", "", "", err
	}
	homePath, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve home: %w: %w", err, ErrUnsafeTarget)
	}
	home, err = resolvePath(homePath)
	if err != nil {
		return "", "", "", "", err
	}
	if isFilesystemRoot(target) || samePath(managedRoot, home) || samePath(managedRoot, repositoryRoot) || samePath(repositoryRoot, home) {
		return "", "", "", "", ErrUnsafeTarget
	}
	return managedRoot, repositoryRoot, target, home, nil
}

func resolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrUnsafeTarget
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w: %w", path, err, ErrUnsafeTarget)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absPath))
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w: %w", path, err, ErrUnsafeTarget)
	}
	return filepath.Clean(resolved), nil
}

func strictlyContained(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	return clean == filepath.VolumeName(clean)+string(filepath.Separator)
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
