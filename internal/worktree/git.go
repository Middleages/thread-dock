// Package worktree contains safe, explicit Git Worktree operations.
package worktree

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"thread-dock/internal/runner"
)

var (
	ErrDirtyWorktree = errors.New("worktree is dirty")
	ErrUnsafeTarget  = errors.New("unsafe worktree target")
	ErrConflict      = errors.New("git merge has unmerged paths")
)

// Clock supplies the time source used to measure verification commands.
// Production callers may leave it nil to use time.Now.
type Clock interface {
	Now() time.Time
}

// VerificationCheck is the intentionally minimal result returned by Git
// checks. Process output is deliberately not retained at this boundary.
type VerificationCheck struct {
	Command  string
	Outcome  string
	Duration string
	ExitCode int
}

// Git is an adapter for the small set of Git commands ThreadDock needs.
type Git struct {
	Runner         runner.Runner
	Binary         string
	ManagedRoot    string
	RepositoryRoot string
	Clock          Clock
}

// CommitInspection is derived from Git, not from Agent prose. Patch is the
// bounded unified diff that a Reviewer may inspect.
type CommitInspection struct {
	CommitSHA    string
	Branch       string
	ChangedFiles []string
	Patch        string
}

// RevertWorktreeStatus is the read-only reconciliation result for a
// deterministic revert worktree.
type RevertWorktreeStatus struct {
	Exists   bool
	Ready    bool
	Reverted bool
}

// RevertWorktreeInspection is the identity of an existing deterministic
// revert worktree. BaseCommit is the exact commit from which the branch was
// created (the revert commit's parent when Stage is reverted). ParentCommit is
// retained explicitly so callers never have to infer identity from a moving
// remote ref.
type RevertWorktreeInspection struct {
	Exists       bool
	Stage        string
	HeadCommit   string
	BaseCommit   string
	ParentCommit string
}

// ValidateTrustedManagedRoot enforces the ownership and non-writable-mode
// invariant used before creating managed worktrees.
func ValidateTrustedManagedRoot(path string) error { return validateTrustedManagedRoot(path) }

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

// CreateManagedWorktree creates a fresh branch Worktree from an explicit base
// commit. Unlike the legacy Create method, its target must be strictly inside
// the configured ManagedRoot and the target itself must not already exist.
func (g *Git) CreateManagedWorktree(ctx context.Context, repositoryPath, worktreePath, branch, base string) error {
	if err := g.validateManagedCreateTarget(repositoryPath, worktreePath, branch, base); err != nil {
		return err
	}
	target, exists, err := resolveCreatePath(worktreePath)
	if err != nil || exists {
		return ErrUnsafeTarget
	}
	return g.run(ctx, repositoryPath, "worktree", "add", "-b", branch, target, base)
}

// ReconcileRevertWorktree verifies that an existing deterministic revert
// worktree belongs to the configured repository, is on the expected branch,
// is clean, and is either at the exact base commit or contains the exact
// default revert commit for mergeSHA. A missing target is safe to create.
func (g *Git) ReconcileRevertWorktree(ctx context.Context, repositoryPath, worktreePath, branch, baseCommit, mergeSHA string) (RevertWorktreeStatus, error) {
	if g == nil || g.Runner == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(repositoryPath) != repositoryPath || !validGitRef(branch) || !isCommitSHA(baseCommit) || !isCommitSHA(mergeSHA) {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	managedRoot, err := resolvePath(g.ManagedRoot)
	if err != nil || validateTrustedManagedRoot(managedRoot) != nil {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	configuredRepository, err := resolvePath(g.RepositoryRoot)
	if err != nil {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	suppliedRepository, err := resolvePath(repositoryPath)
	if err != nil || !samePath(configuredRepository, suppliedRepository) {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	_, targetExists, err := resolveCreatePath(worktreePath)
	if err != nil {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	if !targetExists {
		return RevertWorktreeStatus{}, nil
	}
	managedRoot, repositoryRoot, target, home, err := g.resolveRemovalPaths(worktreePath)
	if err != nil || !strictlyContained(managedRoot, target) || samePath(target, repositoryRoot) || samePath(target, home) || isFilesystemRoot(target) {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	targetCommonDir, err := g.gitCommonDir(ctx, target)
	if err != nil {
		return RevertWorktreeStatus{}, err
	}
	configuredCommonDir, err := g.gitCommonDir(ctx, repositoryRoot)
	if err != nil || !samePath(targetCommonDir, configuredCommonDir) {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	branchResult, err := g.command(ctx, target, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || strings.TrimSpace(branchResult.Stdout) != branch {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	statusResult, err := g.command(ctx, target, "status", "--porcelain=v1")
	if err != nil {
		return RevertWorktreeStatus{}, err
	}
	if strings.TrimSpace(statusResult.Stdout) != "" {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	headResult, err := g.command(ctx, target, "rev-parse", "HEAD")
	if err != nil {
		return RevertWorktreeStatus{}, err
	}
	head := strings.TrimSpace(headResult.Stdout)
	if !isCommitSHA(head) {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	if head == baseCommit {
		return RevertWorktreeStatus{Exists: true, Ready: true}, nil
	}
	parentsResult, err := g.command(ctx, target, "rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		return RevertWorktreeStatus{}, err
	}
	parents := strings.Fields(parentsResult.Stdout)
	if len(parents) != 2 || parents[0] != head || parents[1] != baseCommit {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	messageResult, err := g.command(ctx, target, "log", "-1", "--format=%B", "HEAD")
	if err != nil {
		return RevertWorktreeStatus{}, err
	}
	if !strings.Contains(messageResult.Stdout, "This reverts commit "+mergeSHA+".") && !strings.Contains(messageResult.Stdout, "This reverts commit "+mergeSHA+",") {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	diffResult, err := g.command(ctx, target, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if err != nil {
		return RevertWorktreeStatus{}, err
	}
	if strings.TrimSpace(diffResult.Stdout) == "" {
		return RevertWorktreeStatus{}, ErrUnsafeTarget
	}
	return RevertWorktreeStatus{Exists: true, Reverted: true}, nil
}

// InspectRevertWorktree performs the same ownership, repository, branch and
// cleanliness checks as ReconcileRevertWorktree, but does not take a caller's
// (possibly stale) base as truth. It reports the exact existing base so a
// retry can safely validate it against the current remote head.
func (g *Git) InspectRevertWorktree(ctx context.Context, repositoryPath, worktreePath, branch, mergeSHA string) (RevertWorktreeInspection, error) {
	if g == nil || g.Runner == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(repositoryPath) != repositoryPath || !validGitRef(branch) || !isCommitSHA(mergeSHA) {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	managedRoot, err := resolvePath(g.ManagedRoot)
	if err != nil || validateTrustedManagedRoot(managedRoot) != nil {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	configuredRepository, err := resolvePath(g.RepositoryRoot)
	if err != nil {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	suppliedRepository, err := resolvePath(repositoryPath)
	if err != nil || !samePath(configuredRepository, suppliedRepository) {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	_, targetExists, err := resolveCreatePath(worktreePath)
	if err != nil {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	if !targetExists {
		return RevertWorktreeInspection{}, nil
	}
	managedRoot, repositoryRoot, target, home, err := g.resolveRemovalPaths(worktreePath)
	if err != nil || !strictlyContained(managedRoot, target) || samePath(target, repositoryRoot) || samePath(target, home) || isFilesystemRoot(target) {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	targetCommonDir, err := g.gitCommonDir(ctx, target)
	if err != nil {
		return RevertWorktreeInspection{}, err
	}
	configuredCommonDir, err := g.gitCommonDir(ctx, repositoryRoot)
	if err != nil || !samePath(targetCommonDir, configuredCommonDir) {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	branchResult, err := g.command(ctx, target, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || strings.TrimSpace(branchResult.Stdout) != branch {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	statusResult, err := g.command(ctx, target, "status", "--porcelain=v1")
	if err != nil {
		return RevertWorktreeInspection{}, err
	}
	if strings.TrimSpace(statusResult.Stdout) != "" {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	headResult, err := g.command(ctx, target, "rev-parse", "HEAD")
	if err != nil {
		return RevertWorktreeInspection{}, err
	}
	head := strings.TrimSpace(headResult.Stdout)
	if !isCommitSHA(head) {
		return RevertWorktreeInspection{}, ErrUnsafeTarget
	}
	parentsResult, err := g.command(ctx, target, "rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		return RevertWorktreeInspection{}, err
	}
	parents := strings.Fields(parentsResult.Stdout)
	if len(parents) == 2 && parents[0] == head && isCommitSHA(parents[1]) {
		messageResult, messageErr := g.command(ctx, target, "log", "-1", "--format=%B", "HEAD")
		if messageErr != nil {
			return RevertWorktreeInspection{}, messageErr
		}
		marker := "This reverts commit "
		if strings.Contains(messageResult.Stdout, marker) && !(strings.Contains(messageResult.Stdout, marker+mergeSHA+".") || strings.Contains(messageResult.Stdout, marker+mergeSHA+",")) {
			return RevertWorktreeInspection{}, ErrUnsafeTarget
		}
		if strings.Contains(messageResult.Stdout, marker+mergeSHA+".") || strings.Contains(messageResult.Stdout, marker+mergeSHA+",") {
			diffResult, diffErr := g.command(ctx, target, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
			if diffErr != nil {
				return RevertWorktreeInspection{}, diffErr
			}
			if strings.TrimSpace(diffResult.Stdout) == "" {
				return RevertWorktreeInspection{}, ErrUnsafeTarget
			}
			return RevertWorktreeInspection{Exists: true, Stage: "reverted", HeadCommit: head, BaseCommit: parents[1], ParentCommit: parents[1]}, nil
		}
	}
	// A clean, owned branch with no exact revert marker is the ready stage.
	// Its HEAD is the immutable base recorded for a later retry.
	return RevertWorktreeInspection{Exists: true, Stage: "ready", HeadCommit: head, BaseCommit: head}, nil
}

func (g *Git) gitCommonDir(ctx context.Context, cwd string) (string, error) {
	result, err := g.command(ctx, cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	commonDir := strings.TrimSpace(result.Stdout)
	if commonDir == "" {
		return "", ErrUnsafeTarget
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(cwd, commonDir)
	}
	resolved, err := resolvePath(commonDir)
	if err != nil {
		return "", ErrUnsafeTarget
	}
	return resolved, nil
}

// RevertMergeCommit reverts an ordinary merge commit using its first parent.
// A conflict is reported only when Git confirms unmerged paths in the target.
func (g *Git) RevertMergeCommit(ctx context.Context, worktreePath, mergeSHA string) error {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return err
	}
	if !isCommitSHA(mergeSHA) {
		return errors.New("revert requires a 40-character commit SHA")
	}
	result, err := g.command(ctx, worktreePath, "revert", "-m", "1", "--no-edit", mergeSHA)
	if err == nil && result.ExitCode == 0 {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("git revert exited with status %d", result.ExitCode)
	}
	status, statusErr := g.command(ctx, worktreePath, "status", "--porcelain=v1")
	if statusErr == nil && hasUnmergedPaths(status.Stdout) {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

// AbortRevert delegates conflict cleanup to git revert --abort. It never
// resets files or removes the managed Worktree.
func (g *Git) AbortRevert(ctx context.Context, worktreePath string) error {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return err
	}
	return g.run(ctx, worktreePath, "revert", "--abort")
}

// PushBranch publishes an explicit branch ref without force or a movable
// source target. The destination is deliberately branch:branch.
func (g *Git) PushBranch(ctx context.Context, worktreePath, remote, branch string) error {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return err
	}
	if !validGitRef(remote) || !validGitRef(branch) {
		return errors.New("push requires valid remote and branch")
	}
	return g.run(ctx, worktreePath, "push", remote, branch+":"+branch)
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

// MergeCommitNoFF merges one immutable commit and never follows a movable
// branch. A merge failure is classified as a conflict only after Git reports
// an actual unmerged path in the target Worktree.
func (g *Git) MergeCommitNoFF(ctx context.Context, worktreePath, sha string) error {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return err
	}
	if !isCommitSHA(sha) {
		return errors.New("merge requires a 40-character commit SHA")
	}
	result, err := g.command(ctx, worktreePath, "merge", "--no-ff", "--no-edit", sha)
	if err == nil && result.ExitCode == 0 {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("git merge exited with status %d", result.ExitCode)
	}
	status, statusErr := g.command(ctx, worktreePath, "status", "--porcelain=v1")
	if statusErr == nil && hasUnmergedPaths(status.Stdout) {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

// AbortMerge delegates recovery to Git and never resets or resolves files.
func (g *Git) AbortMerge(ctx context.Context, worktreePath string) error {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return err
	}
	return g.run(ctx, worktreePath, "merge", "--abort")
}

// RunChecks executes each approved command through an explicit bash -lc
// boundary. A non-zero process exit is a failed check; runner/context errors
// that do not represent a process exit remain infrastructure errors.
func (g *Git) RunChecks(ctx context.Context, worktreePath string, commands []string) ([]VerificationCheck, error) {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return nil, err
	}
	if g == nil || g.Runner == nil {
		return nil, errors.New("runner is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	checks := make([]VerificationCheck, 0, len(commands))
	for _, raw := range commands {
		command := strings.TrimSpace(raw)
		if command == "" {
			return nil, errors.New("verification command is required")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start := g.now()
		result, err := g.Runner.Run(ctx, worktreePath, "bash", "-lc", command)
		end := g.now()
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if end.Before(start) {
			end = start
		}
		duration := end.Sub(start).String()
		if err != nil {
			var exited exitCoder
			if !errors.As(err, &exited) || exited.ExitCode() <= 0 {
				return nil, fmt.Errorf("run verification %q: %w", command, err)
			}
			result.ExitCode = exited.ExitCode()
		}
		if result.ExitCode < 0 {
			return nil, fmt.Errorf("run verification %q: invalid exit code %d", command, result.ExitCode)
		}
		outcome := "passed"
		if result.ExitCode > 0 {
			outcome = "failed"
		}
		checks = append(checks, VerificationCheck{Command: command, Outcome: outcome, Duration: duration, ExitCode: result.ExitCode})
	}
	return checks, nil
}

func (g *Git) now() time.Time {
	if g != nil && g.Clock != nil {
		return g.Clock.Now()
	}
	return time.Now()
}

// Fingerprint returns a deterministic digest of the current commit and all
// changed repository-relative paths. Each path includes its porcelain status,
// index object IDs, and current worktree object ID (or a deletion marker).
// Raw Git output is used only while calculating the digest and is not stored.
func (g *Git) Fingerprint(ctx context.Context, worktreePath string) (string, error) {
	if err := g.validateManagedWorktreePath(worktreePath); err != nil {
		return "", err
	}
	return g.fingerprint(ctx, worktreePath)
}

// FingerprintWorktree returns the same deterministic content digest for an
// existing Herdr-created Worktree. Unlike Fingerprint, this read-only port
// does not require the target to be inside ManagedRoot: it proves the target
// is a linked Worktree of the configured RepositoryRoot before reading it.
func (g *Git) FingerprintWorktree(ctx context.Context, worktreePath string) (string, error) {
	if err := g.validateReadOnlyWorktreePath(ctx, worktreePath); err != nil {
		return "", err
	}
	return g.fingerprint(ctx, worktreePath)
}

func (g *Git) fingerprint(ctx context.Context, worktreePath string) (string, error) {
	commit, err := g.CurrentCommit(ctx, worktreePath)
	if err != nil {
		return "", err
	}
	status, err := g.command(ctx, worktreePath, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return "", err
	}
	entries := parseNULStatusEntries(status.Stdout)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].path == entries[j].path {
			return entries[i].status < entries[j].status
		}
		return entries[i].path < entries[j].path
	})
	var material strings.Builder
	appendFingerprintField(&material, "commit")
	appendFingerprintField(&material, strings.TrimSpace(commit))
	for _, entry := range entries {
		appendFingerprintField(&material, "status")
		appendFingerprintField(&material, entry.status)
		appendFingerprintField(&material, entry.path)
		indexIDs, err := g.indexObjectIDs(ctx, worktreePath, entry.path)
		if err != nil {
			return "", err
		}
		for _, id := range indexIDs {
			appendFingerprintField(&material, "index")
			appendFingerprintField(&material, id)
		}
		worktreeID, exists, err := g.worktreeObjectID(ctx, worktreePath, entry.path)
		if err != nil {
			return "", err
		}
		appendFingerprintField(&material, "worktree")
		if exists {
			appendFingerprintField(&material, worktreeID)
		} else {
			appendFingerprintField(&material, "<deleted>")
		}
	}
	digest := sha256.Sum256([]byte(material.String()))
	return fmt.Sprintf("%x", digest[:]), nil
}

func (g *Git) validateReadOnlyWorktreePath(ctx context.Context, worktreePath string) error {
	if g == nil || g.Runner == nil || strings.TrimSpace(worktreePath) == "" || strings.TrimSpace(worktreePath) != worktreePath || strings.TrimSpace(g.RepositoryRoot) == "" {
		return ErrUnsafeTarget
	}
	target, err := resolvePath(worktreePath)
	if err != nil || isFilesystemRoot(target) {
		return ErrUnsafeTarget
	}
	repositoryRoot, err := resolvePath(g.RepositoryRoot)
	if err != nil || samePath(target, repositoryRoot) {
		return ErrUnsafeTarget
	}
	inside, err := g.command(ctx, target, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside.Stdout) != "true" {
		return ErrUnsafeTarget
	}
	topResult, err := g.command(ctx, target, "rev-parse", "--show-toplevel")
	if err != nil {
		return ErrUnsafeTarget
	}
	top, err := resolvePath(strings.TrimSpace(topResult.Stdout))
	if err != nil || samePath(top, repositoryRoot) || !samePath(top, target) {
		return ErrUnsafeTarget
	}
	targetCommonDir, err := g.gitCommonDir(ctx, target)
	if err != nil {
		return err
	}
	configuredCommonDir, err := g.gitCommonDir(ctx, repositoryRoot)
	if err != nil || !samePath(targetCommonDir, configuredCommonDir) {
		return ErrUnsafeTarget
	}
	return nil
}

type statusEntry struct {
	status string
	path   string
}

func parseStatusEntries(output string) []statusEntry {
	var entries []statusEntry
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 3 || strings.TrimSpace(line) == "" {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		entries = append(entries, statusEntry{status: status, path: path})
	}
	return entries
}

func parseNULStatusEntries(output string) []statusEntry {
	var entries []statusEntry
	for _, record := range strings.Split(output, "\x00") {
		if len(record) < 3 {
			continue
		}
		status := record[:2]
		path := record[3:]
		if path == "" {
			continue
		}
		entries = append(entries, statusEntry{status: status, path: path})
	}
	return entries
}

func appendFingerprintField(material *strings.Builder, value string) {
	fmt.Fprintf(material, "%d:", len(value))
	material.WriteString(value)
	material.WriteByte(';')
}

func (g *Git) indexObjectIDs(ctx context.Context, worktreePath, path string) ([]string, error) {
	result, err := g.command(ctx, worktreePath, "ls-files", "--stage", "-z", "--", path)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(result.Stdout, "\x00") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && isObjectID(fields[1]) {
			ids = append(ids, fields[1])
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (g *Git) worktreeObjectID(ctx context.Context, worktreePath, path string) (string, bool, error) {
	fullPath := filepath.Join(worktreePath, filepath.FromSlash(path))
	if _, err := os.Lstat(fullPath); errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	result, err := g.command(ctx, worktreePath, "hash-object", "--", path)
	if err != nil {
		return "", false, err
	}
	id := strings.TrimSpace(result.Stdout)
	if !isObjectID(id) {
		return "", false, errors.New("git returned an invalid worktree object ID")
	}
	return id, true, nil
}

func isObjectID(value string) bool {
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

func hasUnmergedPaths(output string) bool {
	for _, entry := range parseStatusEntries(output) {
		if isUnmergedStatus(entry.status) {
			return true
		}
	}
	return false
}

func isUnmergedStatus(status string) bool {
	if len(status) != 2 {
		return false
	}
	return status == "DD" || status == "AU" || status == "UD" || status == "UA" || status == "DU" || status == "AA" || status == "UU"
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

// IsAncestor proves that an immutable commit is present in a worktree's
// current history. It is read-only and is used to reconcile an interrupted
// merge without repeating the merge mutation.
func (g *Git) IsAncestor(ctx context.Context, worktreePath, commitSHA string) (bool, error) {
	if err := g.validateWorktreePath(worktreePath); err != nil || !isCommitSHA(commitSHA) {
		return false, ErrUnsafeTarget
	}
	result, err := g.command(ctx, worktreePath, "merge-base", "--is-ancestor", commitSHA, "HEAD")
	if err == nil && result.ExitCode == 0 {
		return true, nil
	}
	return false, err
}

// IsAncestorOf proves that an immutable commit is reachable from an exact
// descendant SHA. It is read-only and is used by revert validation after the
// remote default branch has been fetched.
func (g *Git) IsAncestorOf(ctx context.Context, repositoryPath, ancestorSHA, descendantSHA string) (bool, error) {
	if err := g.validateWorktreePath(repositoryPath); err != nil || !isCommitSHA(ancestorSHA) || !isCommitSHA(descendantSHA) {
		return false, ErrUnsafeTarget
	}
	result, err := g.command(ctx, repositoryPath, "merge-base", "--is-ancestor", ancestorSHA, descendantSHA)
	if err == nil && result.ExitCode == 0 {
		return true, nil
	}
	if result.ExitCode == 1 {
		return false, nil
	}
	if err == nil {
		return false, fmt.Errorf("git merge-base exited with status %d", result.ExitCode)
	}
	return false, err
}

// FetchRemoteHead fetches the named remote/default-branch ref and returns its
// exact immutable SHA. It intentionally does not inspect a local checkout
// HEAD, so merge-gate decisions cannot be made against stale local state.
func (g *Git) FetchRemoteHead(ctx context.Context, repositoryPath, remote, branch string) (string, error) {
	if g == nil || g.Runner == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(repositoryPath) != repositoryPath || !validGitRef(remote) || !validGitRef(branch) {
		return "", ErrUnsafeTarget
	}
	if err := g.run(ctx, repositoryPath, "fetch", "--no-tags", remote, branch); err != nil {
		return "", err
	}
	result, err := g.command(ctx, repositoryPath, "rev-parse", "refs/remotes/"+remote+"/"+branch)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(result.Stdout)
	if !isCommitSHA(sha) {
		return "", errors.New("remote head is not an exact commit SHA")
	}
	return sha, nil
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

// RetirementProof is the immutable Git identity recorded before a Herdr
// Workspace is closed. Path and RepositoryCommonDir are canonical paths;
// Branch and HeadSHA are the exact branch and commit observed by Git.
//
// The proof intentionally carries no patch or command output. It is only an
// identity record that can be checked again before a later non-force remove.
type RetirementProof struct {
	RepositoryCommonDir string
	Path                string
	Branch              string
	HeadSHA             string
}

type registeredRetirementWorktree struct {
	path   string
	head   string
	branch string
}

// InspectRetirementTarget proves that worktreePath is the exact, clean,
// registered linked Worktree represented by the supplied branch and HEAD.
// Every check is read-only. The repository path must resolve to the
// configured repository's common directory, while ManagedRoot is the trusted
// Herdr Worktree root.
func (g *Git) InspectRetirementTarget(ctx context.Context, repositoryPath, worktreePath, expectedBranch, expectedSHA string) (RetirementProof, error) {
	if !validRetirementArguments(g, repositoryPath, worktreePath, expectedBranch, expectedSHA) {
		return RetirementProof{}, ErrUnsafeTarget
	}
	proof, exists, err := g.proveRetirementTarget(ctx, repositoryPath, g.ManagedRoot, worktreePath, expectedBranch, expectedSHA, false)
	if err != nil || !exists {
		if err != nil {
			return RetirementProof{}, err
		}
		return RetirementProof{}, ErrUnsafeTarget
	}
	return proof, nil
}

// RemoveRetired re-proves the durable retirement identity immediately before
// asking Git to remove the target. It never uses --force and never deletes a
// branch. A retry after response loss succeeds only when both the target and
// its Git registration are gone; a stale registration is unsafe.
func (g *Git) RemoveRetired(ctx context.Context, repositoryPath, trustedHerdrRoot string, proof RetirementProof) error {
	if !validRetirementProofArguments(g, repositoryPath, trustedHerdrRoot, proof) {
		return ErrUnsafeTarget
	}
	proofCommonDir, err := resolvePath(proof.RepositoryCommonDir)
	if err != nil {
		return ErrUnsafeTarget
	}
	current, exists, err := g.proveRetirementTarget(ctx, repositoryPath, trustedHerdrRoot, proof.Path, proof.Branch, proof.HeadSHA, true)
	if err != nil {
		return err
	}
	if !exists {
		// No target and no exact registration is the only safe interpretation of
		// a lost successful response. The repository identity still has to
		// match the durable proof so a missing path cannot mask a wrong repo.
		if !samePath(current.RepositoryCommonDir, proofCommonDir) || current.Path != filepath.Clean(proof.Path) {
			return ErrUnsafeTarget
		}
		return nil
	}
	if !sameRetirementProof(current, proof, proofCommonDir) {
		return ErrUnsafeTarget
	}
	repositoryRoot, err := resolvePath(repositoryPath)
	if err != nil {
		return ErrUnsafeTarget
	}
	return g.run(ctx, repositoryRoot, "worktree", "remove", current.Path)
}

func validRetirementArguments(g *Git, repositoryPath, worktreePath, branch, sha string) bool {
	return g != nil && g.Runner != nil && strings.TrimSpace(repositoryPath) != "" && strings.TrimSpace(repositoryPath) == repositoryPath && strings.TrimSpace(worktreePath) != "" && strings.TrimSpace(worktreePath) == worktreePath && validGitRef(branch) && isCommitSHA(sha)
}

func validRetirementProofArguments(g *Git, repositoryPath, trustedHerdrRoot string, proof RetirementProof) bool {
	if !validRetirementArguments(g, repositoryPath, proof.Path, proof.Branch, proof.HeadSHA) {
		return false
	}
	if strings.TrimSpace(trustedHerdrRoot) == "" || strings.TrimSpace(trustedHerdrRoot) != trustedHerdrRoot || strings.TrimSpace(proof.RepositoryCommonDir) == "" || strings.TrimSpace(proof.RepositoryCommonDir) != proof.RepositoryCommonDir {
		return false
	}
	return filepath.IsAbs(proof.Path) && filepath.Clean(proof.Path) == proof.Path && filepath.IsAbs(proof.RepositoryCommonDir) && filepath.Clean(proof.RepositoryCommonDir) == proof.RepositoryCommonDir
}

func (g *Git) proveRetirementTarget(ctx context.Context, repositoryPath, trustedHerdrRoot, worktreePath, expectedBranch, expectedSHA string, allowMissing bool) (RetirementProof, bool, error) {
	paths, err := g.resolveRetirementPaths(repositoryPath, trustedHerdrRoot, worktreePath, allowMissing)
	if err != nil {
		return RetirementProof{}, false, err
	}
	configuredCommonDir, err := g.gitCommonDir(ctx, paths.configuredRoot)
	if err != nil {
		return RetirementProof{}, false, retirementUnsafe(err)
	}
	if !samePath(paths.repositoryRoot, paths.configuredRoot) {
		suppliedCommonDir, suppliedErr := g.gitCommonDir(ctx, paths.repositoryRoot)
		if suppliedErr != nil || !samePath(suppliedCommonDir, configuredCommonDir) {
			return RetirementProof{}, false, retirementUnsafe(suppliedErr)
		}
	}
	registrations, err := g.retirementRegistrations(ctx, paths.repositoryRoot)
	if err != nil {
		return RetirementProof{}, false, retirementUnsafe(err)
	}
	matches := make([]registeredRetirementWorktree, 0, 1)
	for _, registration := range registrations {
		registeredPath, pathErr := canonicalRetirementRegistrationPath(paths.repositoryRoot, registration.path)
		if pathErr != nil {
			return RetirementProof{}, false, ErrUnsafeTarget
		}
		if registeredPath == paths.target {
			registration.path = registeredPath
			matches = append(matches, registration)
		}
	}
	if !paths.targetExists {
		if len(matches) != 0 {
			return RetirementProof{}, false, ErrUnsafeTarget
		}
		if retirementIdentityMoved(registrations, paths, expectedBranch, expectedSHA) {
			return RetirementProof{}, false, ErrUnsafeTarget
		}
		if !allowMissing {
			return RetirementProof{}, false, ErrUnsafeTarget
		}
		return RetirementProof{RepositoryCommonDir: configuredCommonDir, Path: paths.target}, false, nil
	}
	if len(matches) != 1 {
		return RetirementProof{}, false, ErrUnsafeTarget
	}
	registration := matches[0]
	if registration.branch != expectedBranch || registration.head != expectedSHA {
		return RetirementProof{}, false, ErrUnsafeTarget
	}
	targetCommonDir, err := g.gitCommonDir(ctx, paths.target)
	if err != nil {
		return RetirementProof{}, false, retirementUnsafe(err)
	}
	if !samePath(targetCommonDir, configuredCommonDir) {
		return RetirementProof{}, false, ErrUnsafeTarget
	}
	branchResult, err := g.command(ctx, paths.target, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return RetirementProof{}, false, retirementUnsafe(err)
	}
	branch := strings.TrimSpace(branchResult.Stdout)
	if branch != expectedBranch {
		return RetirementProof{}, false, ErrUnsafeTarget
	}
	headResult, err := g.command(ctx, paths.target, "rev-parse", "HEAD")
	if err != nil {
		return RetirementProof{}, false, retirementUnsafe(err)
	}
	head := strings.TrimSpace(headResult.Stdout)
	if head != expectedSHA || !isCommitSHA(head) {
		return RetirementProof{}, false, ErrUnsafeTarget
	}
	statusResult, err := g.command(ctx, paths.target, "status", "--porcelain=v1")
	if err != nil {
		return RetirementProof{}, false, retirementUnsafe(err)
	}
	if strings.TrimSpace(statusResult.Stdout) != "" {
		return RetirementProof{}, false, ErrUnsafeTarget
	}
	return RetirementProof{RepositoryCommonDir: configuredCommonDir, Path: paths.target, Branch: branch, HeadSHA: head}, true, nil
}

func (g *Git) retirementRegistrations(ctx context.Context, repositoryRoot string) ([]registeredRetirementWorktree, error) {
	result, err := g.command(ctx, repositoryRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseRetirementRegistrations(result.Stdout)
}

func parseRetirementRegistrations(output string) ([]registeredRetirementWorktree, error) {
	output = strings.TrimRight(output, "\n")
	if strings.TrimSpace(output) == "" {
		return nil, ErrUnsafeTarget
	}
	var registrations []registeredRetirementWorktree
	for _, record := range strings.Split(output, "\n\n") {
		lines := strings.Split(record, "\n")
		if len(lines) == 1 && lines[0] == "bare" {
			continue
		}
		if len(lines) == 0 || !strings.HasPrefix(lines[0], "worktree ") || strings.TrimPrefix(lines[0], "worktree ") == "" {
			return nil, ErrUnsafeTarget
		}
		registration := registeredRetirementWorktree{path: strings.TrimPrefix(lines[0], "worktree ")}
		seen := map[string]bool{"worktree": true}
		for _, line := range lines[1:] {
			switch {
			case line == "bare":
				if seen["bare"] || seen["head"] || seen["branch"] || seen["detached"] {
					return nil, ErrUnsafeTarget
				}
				seen["bare"] = true
			case strings.HasPrefix(line, "HEAD "):
				if seen["head"] || seen["bare"] {
					return nil, ErrUnsafeTarget
				}
				registration.head = strings.TrimPrefix(line, "HEAD ")
				seen["head"] = true
			case strings.HasPrefix(line, "branch "):
				if seen["branch"] || seen["bare"] || !strings.HasPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/") {
					return nil, ErrUnsafeTarget
				}
				registration.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
				if registration.branch == "" {
					return nil, ErrUnsafeTarget
				}
				seen["branch"] = true
			case line == "detached":
				if seen["detached"] || seen["branch"] || seen["bare"] {
					return nil, ErrUnsafeTarget
				}
				seen["detached"] = true
			case strings.HasPrefix(line, "locked"), strings.HasPrefix(line, "prunable "):
				// These are metadata about a registered Worktree, not identity.
			default:
				return nil, ErrUnsafeTarget
			}
		}
		if seen["bare"] {
			continue
		}
		if !seen["head"] || !isCommitSHA(registration.head) || (!seen["branch"] && !seen["detached"]) {
			return nil, ErrUnsafeTarget
		}
		registrations = append(registrations, registration)
	}
	return registrations, nil
}

func canonicalRetirementRegistrationPath(repositoryRoot, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(repositoryRoot, path)
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	absPath = filepath.Clean(absPath)
	if _, lstatErr := os.Lstat(absPath); lstatErr == nil {
		parent, parentErr := resolvePath(filepath.Dir(absPath))
		if parentErr != nil {
			return "", parentErr
		}
		return filepath.Clean(filepath.Join(parent, filepath.Base(absPath))), nil
	} else if !errors.Is(lstatErr, os.ErrNotExist) {
		return "", lstatErr
	}
	resolved, _, err := resolveCreatePath(absPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// retirementIdentityMoved detects a proof whose exact path disappeared while
// Git still has its branch/HEAD identity registered elsewhere. A HEAD-only
// match is meaningful only when unique; repository roots are excluded from
// that check because a linked target can legitimately share their HEAD.
func retirementIdentityMoved(registrations []registeredRetirementWorktree, paths retirementPaths, branch, head string) bool {
	branchMatches, headMatches := 0, 0
	for _, registration := range registrations {
		registeredPath, err := canonicalRetirementRegistrationPath(paths.repositoryRoot, registration.path)
		if err != nil {
			return true
		}
		if registration.branch == branch && registration.head == head {
			return true
		}
		if registeredPath == paths.repositoryRoot || registeredPath == paths.configuredRoot {
			continue
		}
		if registration.branch == branch {
			branchMatches++
		}
		if registration.head == head {
			headMatches++
		}
	}
	return branchMatches == 1 || headMatches == 1
}

type retirementPaths struct {
	configuredRoot string
	repositoryRoot string
	target         string
	targetExists   bool
}

func (g *Git) resolveRetirementPaths(repositoryPath, trustedHerdrRoot, targetPath string, allowMissing bool) (retirementPaths, error) {
	if g == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(repositoryPath) != repositoryPath || strings.TrimSpace(trustedHerdrRoot) == "" || strings.TrimSpace(trustedHerdrRoot) != trustedHerdrRoot || strings.TrimSpace(targetPath) == "" || strings.TrimSpace(targetPath) != targetPath {
		return retirementPaths{}, ErrUnsafeTarget
	}
	managedRoot, err := resolvePath(trustedHerdrRoot)
	if err != nil || validateTrustedManagedRoot(managedRoot) != nil {
		return retirementPaths{}, ErrUnsafeTarget
	}
	configuredRoot, err := resolvePath(g.RepositoryRoot)
	if err != nil {
		return retirementPaths{}, ErrUnsafeTarget
	}
	suppliedRepository, err := resolvePath(repositoryPath)
	if err != nil {
		return retirementPaths{}, ErrUnsafeTarget
	}
	homePath, err := os.UserHomeDir()
	if err != nil {
		return retirementPaths{}, ErrUnsafeTarget
	}
	home, err := resolvePath(homePath)
	if err != nil || isFilesystemRoot(managedRoot) || isFilesystemRoot(configuredRoot) || isFilesystemRoot(suppliedRepository) || samePath(managedRoot, home) || samePath(configuredRoot, home) || samePath(suppliedRepository, home) || samePath(managedRoot, configuredRoot) || samePath(managedRoot, suppliedRepository) {
		return retirementPaths{}, ErrUnsafeTarget
	}
	var target string
	var targetExists bool
	if allowMissing {
		target, targetExists, err = resolveRetirementTargetPath(targetPath, true)
	} else {
		target, targetExists, err = resolveRetirementTargetPath(targetPath, false)
	}
	if err != nil {
		return retirementPaths{}, ErrUnsafeTarget
	}
	if !strictlyContained(managedRoot, target) || samePath(target, home) || samePath(target, configuredRoot) || samePath(target, suppliedRepository) || samePath(target, managedRoot) || isFilesystemRoot(target) {
		return retirementPaths{}, ErrUnsafeTarget
	}
	if targetExists {
		info, statErr := os.Stat(target)
		if statErr != nil || !info.IsDir() {
			return retirementPaths{}, ErrUnsafeTarget
		}
	}
	return retirementPaths{configuredRoot: configuredRoot, repositoryRoot: suppliedRepository, target: filepath.Clean(target), targetExists: targetExists}, nil
}

// resolveRetirementTargetPath canonicalizes an existing target's parents but
// never follows the target itself. Lstat is intentionally the first check so
// a proof cannot be rebound through a symlink during retirement.
func resolveRetirementTargetPath(path string, allowMissing bool) (string, bool, error) {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", false, err
	}
	absPath = filepath.Clean(absPath)
	info, err := os.Lstat(absPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", true, ErrUnsafeTarget
		}
		parent, parentErr := resolvePath(filepath.Dir(absPath))
		if parentErr != nil {
			return "", true, parentErr
		}
		return filepath.Clean(filepath.Join(parent, filepath.Base(absPath))), true, nil
	}
	if !errors.Is(err, os.ErrNotExist) || !allowMissing {
		return "", false, err
	}
	resolved, exists, resolveErr := resolveCreatePath(absPath)
	if resolveErr != nil {
		return "", false, resolveErr
	}
	if exists {
		// The target appeared between Lstat and resolveCreatePath. Recheck it
		// without allowing EvalSymlinks to choose a new proof path.
		return resolveRetirementTargetPath(absPath, allowMissing)
	}
	return filepath.Clean(resolved), false, nil
}

func sameRetirementProof(actual, expected RetirementProof, expectedCommonDir string) bool {
	return samePath(actual.RepositoryCommonDir, expectedCommonDir) && actual.Path == filepath.Clean(expected.Path) && actual.Branch == expected.Branch && actual.HeadSHA == expected.HeadSHA
}

func retirementUnsafe(err error) error {
	if err == nil {
		return ErrUnsafeTarget
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrUnsafeTarget
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

// validateManagedWorktreePath is deliberately separate from the legacy
// validator. New integration operations require both configured roots and an
// existing target that resolves strictly inside ManagedRoot.
func (g *Git) validateManagedWorktreePath(path string) error {
	managedRoot, repositoryRoot, target, home, err := g.resolveRemovalPaths(path)
	if err != nil {
		return ErrUnsafeTarget
	}
	if !strictlyContained(managedRoot, target) || samePath(target, repositoryRoot) || samePath(target, home) || isFilesystemRoot(target) {
		return ErrUnsafeTarget
	}
	return nil
}

func (g *Git) validateManagedCreateTarget(repositoryPath, worktreePath, branch, base string) error {
	if g == nil || g.Runner == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(worktreePath) == "" {
		return ErrUnsafeTarget
	}
	if strings.TrimSpace(repositoryPath) != repositoryPath || strings.TrimSpace(worktreePath) != worktreePath || !validGitRef(branch) || !isCommitSHA(base) {
		return ErrUnsafeTarget
	}
	managedRoot, err := resolvePath(g.ManagedRoot)
	if err != nil || validateTrustedManagedRoot(managedRoot) != nil {
		return ErrUnsafeTarget
	}
	configuredRepository, err := resolvePath(g.RepositoryRoot)
	if err != nil {
		return ErrUnsafeTarget
	}
	repositoryRoot, err := resolvePath(repositoryPath)
	if err != nil || !samePath(configuredRepository, repositoryRoot) {
		return ErrUnsafeTarget
	}
	homePath, err := os.UserHomeDir()
	if err != nil {
		return ErrUnsafeTarget
	}
	home, err := resolvePath(homePath)
	if err != nil || samePath(managedRoot, home) || samePath(repositoryRoot, home) || samePath(managedRoot, repositoryRoot) {
		return ErrUnsafeTarget
	}
	target, exists, err := resolveCreatePath(worktreePath)
	if err != nil || exists {
		return ErrUnsafeTarget
	}
	if !strictlyContained(managedRoot, target) || samePath(target, repositoryRoot) || samePath(target, home) || isFilesystemRoot(target) {
		return ErrUnsafeTarget
	}
	return nil
}

// resolveCreatePath resolves every existing parent to detect symlink escapes,
// while preserving the not-yet-created leaf needed by worktree add.
func resolveCreatePath(path string) (resolved string, exists bool, err error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}
	absPath = filepath.Clean(absPath)
	if _, statErr := os.Lstat(absPath); statErr == nil {
		resolved, err = filepath.EvalSymlinks(absPath)
		return filepath.Clean(resolved), true, err
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", false, statErr
	}
	parent := absPath
	var suffix []string
	for {
		_, statErr := os.Lstat(parent)
		if statErr == nil {
			resolvedParent, evalErr := filepath.EvalSymlinks(parent)
			if evalErr != nil {
				return "", false, evalErr
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolvedParent = filepath.Join(resolvedParent, suffix[i])
			}
			return filepath.Clean(resolvedParent), false, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", false, statErr
		}
		parent, suffix = filepath.Dir(parent), append(suffix, filepath.Base(parent))
		if parent == filepath.Dir(parent) {
			return "", false, os.ErrNotExist
		}
	}
}

func validGitRef(value string) bool {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.HasPrefix(value, "-") || strings.HasPrefix(value, ".") || strings.ContainsAny(value, "\x00\r\n ~^:?*[\\") || strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.Contains(value, "//") || strings.Contains(value, "/.") || strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.HasSuffix(value, ".lock") || strings.HasPrefix(value, "/") {
		return false
	}
	return true
}

type exitCoder interface {
	ExitCode() int
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
