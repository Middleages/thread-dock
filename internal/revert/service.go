// Package revert composes a safe local merge-commit revert with a draft PR.
// Git operations stay behind the Git port; the GitHub port only creates the
// resulting draft pull request.
package revert

import (
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"thread-dock/internal/github"
	"thread-dock/internal/worktree"
)

var (
	ErrUnsafeTarget = worktree.ErrUnsafeTarget
	ErrConflict     = worktree.ErrConflict
	ErrBlocked      = errors.New("revert blocked")
)

// MaxRevertReasonBytes is the conservative limit for one operator-provided
// explanation embedded in a draft revert PR.
const MaxRevertReasonBytes = 8 * 1024

// BlockedError identifies a confirmed revert conflict. Cause includes an
// abort failure when aborting the conflict itself was unsuccessful.
type BlockedError struct {
	Cause error
}

func (e *BlockedError) Error() string {
	if e == nil || e.Cause == nil {
		return ErrBlocked.Error()
	}
	return ErrBlocked.Error() + ": " + e.Cause.Error()
}

func (e *BlockedError) Unwrap() error {
	if e == nil {
		return ErrBlocked
	}
	return e.Cause
}

func (e *BlockedError) Is(target error) bool { return target == ErrBlocked }

// Git is the narrow set of explicit Worktree operations needed for a revert.
type Git interface {
	FetchRemoteHead(context.Context, string, string, string) (string, error)
	IsAncestorOf(context.Context, string, string, string) (bool, error)
	ReconcileRevertWorktree(context.Context, string, string, string, string, string) (worktree.RevertWorktreeStatus, error)
	CreateManagedWorktree(context.Context, string, string, string, string) error
	RevertMergeCommit(context.Context, string, string) error
	AbortRevert(context.Context, string) error
	PushBranch(context.Context, string, string, string) error
}

// GitHub is deliberately narrower than github.Client so the REST adapter
// cannot acquire a local Git responsibility.
type GitHub interface {
	FindOpenPullRequest(context.Context, github.Repository, string, string) (github.PullRequest, bool, error)
	CreateSafeDraftPR(context.Context, github.Repository, github.DraftPRRequest) (github.PullRequest, error)
}

// Request describes one immutable merge commit to revert.
type Request struct {
	Repository     github.Repository
	RepositoryPath string
	ManagedRoot    string
	Parent         github.Issue
	DefaultBranch  string
	BaseCommit     string
	MergeSHA       string
	Remote         string
	Reason         string
}

// Service creates a managed revert branch and its draft PR.
type Service struct {
	git    Git
	github GitHub
}

func New(git Git, gh GitHub) *Service { return &Service{git: git, github: gh} }

// Create validates every target before invoking Git, then creates a draft PR
// only after the revert commit has been pushed to its explicit branch.
func (s *Service) Create(ctx context.Context, request Request) (github.PullRequest, error) {
	if s == nil {
		return github.PullRequest{}, errors.New("revert Git and GitHub ports are required")
	}
	if s.git == nil || s.github == nil {
		return github.PullRequest{}, errors.New("revert Git and GitHub ports are required")
	}
	normalized, err := validateRequest(request)
	if err != nil {
		return github.PullRequest{}, err
	}
	draftRequest := github.DraftPRRequest{
		Title:       "Revert merge " + normalized.mergeSHA[:12],
		Body:        normalized.body,
		Head:        normalized.branch,
		Base:        normalized.defaultBranch,
		IssueNumber: normalized.parentIssue,
	}
	if err := github.ValidateSafeDraftPRRequest(normalized.repository, draftRequest); err != nil {
		return github.PullRequest{}, err
	}
	if err := ensureGeneratedParent(normalized.createParent, normalized.managedRoot); err != nil {
		return github.PullRequest{}, err
	}
	remoteHead, err := s.git.FetchRemoteHead(ctx, normalized.repositoryPath, normalized.remote, normalized.defaultBranch)
	if err != nil || !isSHA(strings.TrimSpace(remoteHead)) {
		if err != nil {
			return github.PullRequest{}, err
		}
		return github.PullRequest{}, errors.New("revert remote head is not an exact commit SHA")
	}
	remoteHead = strings.TrimSpace(remoteHead)
	present, err := s.git.IsAncestorOf(ctx, normalized.repositoryPath, normalized.mergeSHA, remoteHead)
	if err != nil {
		return github.PullRequest{}, err
	}
	if !present {
		return github.PullRequest{}, ErrUnsafeTarget
	}
	// The request's BaseCommit identifies the immutable merge record. The
	// worktree itself must start at the exact remote default-branch head just
	// fetched, never at a stale/local or otherwise unknown object.
	normalized.baseCommit = remoteHead
	status, err := s.git.ReconcileRevertWorktree(ctx, normalized.repositoryPath, normalized.worktreePath, normalized.branch, normalized.baseCommit, normalized.mergeSHA)
	if err != nil {
		return github.PullRequest{}, err
	}
	if !status.Exists {
		if err := s.git.CreateManagedWorktree(ctx, normalized.repositoryPath, normalized.worktreePath, normalized.branch, normalized.baseCommit); err != nil {
			return github.PullRequest{}, err
		}
		status = worktree.RevertWorktreeStatus{Exists: true, Ready: true}
	}
	if !status.Reverted && !status.Ready {
		return github.PullRequest{}, ErrUnsafeTarget
	}
	if status.Ready {
		if err := s.git.RevertMergeCommit(ctx, normalized.worktreePath, normalized.mergeSHA); err != nil {
			if !errors.Is(err, ErrConflict) {
				return github.PullRequest{}, err
			}
			abortErr := s.git.AbortRevert(ctx, normalized.worktreePath)
			if abortErr != nil {
				return github.PullRequest{}, &BlockedError{Cause: errors.Join(err, fmt.Errorf("abort revert: %w", abortErr))}
			}
			return github.PullRequest{}, &BlockedError{Cause: err}
		}
	}
	if err := s.git.PushBranch(ctx, normalized.worktreePath, normalized.remote, normalized.branch); err != nil {
		return github.PullRequest{}, err
	}
	existing, found, err := s.github.FindOpenPullRequest(ctx, normalized.repository, normalized.branch, normalized.defaultBranch)
	if err != nil {
		return github.PullRequest{}, err
	}
	if found {
		return existing, nil
	}
	pr, err := s.github.CreateSafeDraftPR(ctx, normalized.repository, draftRequest)
	if err != nil {
		return github.PullRequest{}, err
	}
	return pr, nil
}

type validatedRequest struct {
	repository     github.Repository
	repositoryPath string
	worktreePath   string
	branch         string
	defaultBranch  string
	baseCommit     string
	mergeSHA       string
	remote         string
	parentIssue    int
	body           string
	createParent   string
	managedRoot    string
}

func validateRequest(request Request) (validatedRequest, error) {
	repository := request.Repository
	if strings.TrimSpace(repository.Owner) == "" || strings.TrimSpace(repository.Name) == "" || strings.TrimSpace(repository.Owner) != repository.Owner || strings.TrimSpace(repository.Name) != repository.Name || repository.Owner == "." || repository.Owner == ".." || repository.Name == "." || repository.Name == ".." || strings.ContainsAny(repository.Owner+repository.Name, "/\\\x00\r\n") {
		return validatedRequest{}, errors.New("revert repository is invalid")
	}
	repositoryPathInput := request.RepositoryPath
	repositoryPath, err := resolveExistingDirectory(repositoryPathInput)
	if err != nil || isFilesystemRoot(repositoryPath) {
		return validatedRequest{}, ErrUnsafeTarget
	}
	managedRoot, err := resolveExistingDirectory(request.ManagedRoot)
	if err != nil || isFilesystemRoot(managedRoot) || worktree.ValidateTrustedManagedRoot(managedRoot) != nil {
		return validatedRequest{}, ErrUnsafeTarget
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return validatedRequest{}, ErrUnsafeTarget
	}
	home, err = resolveExistingDirectory(home)
	if err != nil || samePath(managedRoot, home) || samePath(repositoryPath, home) || samePath(managedRoot, repositoryPath) {
		return validatedRequest{}, ErrUnsafeTarget
	}
	baseBranch := request.DefaultBranch
	baseCommit := request.BaseCommit
	mergeSHA := request.MergeSHA
	if !validRef(baseBranch) || !isSHA(baseCommit) || !isSHA(mergeSHA) {
		return validatedRequest{}, errors.New("revert requires valid branches and exact lowercase SHAs")
	}
	parentIssue := request.Parent.Number
	if parentIssue <= 0 {
		return validatedRequest{}, errors.New("revert parent issue number must be positive")
	}
	branch := fmt.Sprintf("revert/%d-%s", parentIssue, mergeSHA[:12])
	if !validRef(branch) {
		return validatedRequest{}, errors.New("revert branch is invalid")
	}
	createParent := filepath.Join(managedRoot, ".revert-worktrees")
	worktreePath, err := resolveCreateTarget(managedRoot, branch)
	if err != nil {
		return validatedRequest{}, err
	}
	remote := request.Remote
	if remote == "" {
		remote = "origin"
	}
	if !validRef(remote) {
		return validatedRequest{}, errors.New("revert remote is invalid")
	}
	reason := request.Reason
	if len(reason) == 0 || len(reason) > MaxRevertReasonBytes || !safeReason(reason) {
		return validatedRequest{}, errors.New("revert reason must be a canonical non-empty string")
	}
	body := fmt.Sprintf("Reverts merge commit `%s`.\n\nParent issue: #%d\n\nReason:\n<pre>%s</pre>", mergeSHA, parentIssue, html.EscapeString(reason))
	return validatedRequest{repository: repository, repositoryPath: repositoryPath, worktreePath: worktreePath, branch: branch, defaultBranch: baseBranch, baseCommit: baseCommit, mergeSHA: mergeSHA, remote: remote, parentIssue: parentIssue, body: body, createParent: createParent, managedRoot: managedRoot}, nil
}

func safeReason(reason string) bool {
	if !utf8.ValidString(reason) {
		return false
	}
	for _, character := range reason {
		if character == '\x00' || character == '\x7f' || character < ' ' && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return strings.TrimSpace(reason) != "" && strings.TrimSpace(reason) == reason
}

func resolveExistingDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path {
		return "", ErrUnsafeTarget
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", ErrUnsafeTarget
	}
	return filepath.Clean(resolved), nil
}

func resolveCreateTarget(managedRoot, branch string) (string, error) {
	raw := filepath.Join(managedRoot, ".revert-worktrees", strings.ReplaceAll(branch, "/", "-"))
	if strings.TrimSpace(raw) != raw {
		return "", ErrUnsafeTarget
	}
	absPath, err := filepath.Abs(raw)
	if err != nil {
		return "", ErrUnsafeTarget
	}
	absPath = filepath.Clean(absPath)
	if info, err := os.Lstat(absPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", ErrUnsafeTarget
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", ErrUnsafeTarget
	}
	parent := absPath
	var suffix []string
	for {
		if _, err := os.Lstat(parent); err == nil {
			resolvedParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", ErrUnsafeTarget
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolvedParent = filepath.Join(resolvedParent, suffix[i])
			}
			target := filepath.Clean(resolvedParent)
			if !strictlyContained(managedRoot, target) || isFilesystemRoot(target) {
				return "", ErrUnsafeTarget
			}
			return target, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", ErrUnsafeTarget
		}
		next := filepath.Dir(parent)
		suffix = append(suffix, filepath.Base(parent))
		if next == parent {
			return "", ErrUnsafeTarget
		}
		parent = next
	}
}

func ensureGeneratedParent(parent, managedRoot string) error {
	if info, err := os.Lstat(parent); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrUnsafeTarget
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafeTarget
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return ErrUnsafeTarget
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || !samePath(resolved, parent) || !strictlyContained(managedRoot, resolved) {
		return ErrUnsafeTarget
	}
	if err := os.Chmod(resolved, 0o700); err != nil || worktree.ValidateTrustedManagedRoot(resolved) != nil {
		return ErrUnsafeTarget
	}
	return nil
}

func strictlyContained(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left, right string) bool { return filepath.Clean(left) == filepath.Clean(right) }

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	return clean == filepath.VolumeName(clean)+string(filepath.Separator)
}

func validRef(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && !strings.HasPrefix(value, "-") && !strings.HasPrefix(value, ".") && !strings.ContainsAny(value, "\x00\r\n ~^:?*[\\") && !strings.Contains(value, "..") && !strings.Contains(value, "@{") && !strings.Contains(value, "//") && !strings.Contains(value, "/.") && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, ".lock")
}

func isSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}
