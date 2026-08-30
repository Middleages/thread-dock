// Package revert composes a safe local merge-commit revert with a draft PR.
// Git operations stay behind the Git port; the GitHub port only creates the
// resulting draft pull request.
package revert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"thread-dock/internal/github"
	"thread-dock/internal/worktree"
)

var (
	ErrUnsafeTarget = worktree.ErrUnsafeTarget
	ErrConflict     = worktree.ErrConflict
	ErrBlocked      = errors.New("revert blocked")
)

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
	CreateManagedWorktree(context.Context, string, string, string, string) error
	RevertMergeCommit(context.Context, string, string) error
	AbortRevert(context.Context, string) error
	PushBranch(context.Context, string, string, string) error
}

// GitHub is deliberately narrower than github.Client so the REST adapter
// cannot acquire a local Git responsibility.
type GitHub interface {
	CreateDraftPR(context.Context, github.Repository, github.DraftPRRequest) (github.PullRequest, error)
}

// Request describes one immutable merge commit to revert.
type Request struct {
	Repository           github.Repository
	RepositoryRef        github.Repository
	Repo                 github.Repository
	RepositoryPath       string
	RepoPath             string
	ManagedRoot          string
	WorktreePath         string
	Parent               github.Issue
	ParentIssue          int
	ParentIssueNumber    int
	DefaultBranch        string
	CurrentDefaultBranch string
	BaseBranch           string
	Base                 string
	BaseCommit           string
	BaseSHA              string
	MergeSHA             string
	MergeCommitSHA       string
	Remote               string
	Reason               string
}

// Service creates a managed revert branch and its draft PR.
type Service struct {
	Git    Git
	GitHub GitHub
	git    Git
	github GitHub
}

func New(git Git, gh GitHub) *Service { return &Service{Git: git, GitHub: gh, git: git, github: gh} }

// NewService is an explicit constructor alias for callers that prefer the
// service-oriented name.
func NewService(git Git, gh GitHub) *Service { return New(git, gh) }

// Create validates every target before invoking Git, then creates a draft PR
// only after the revert commit has been pushed to its explicit branch.
func (s *Service) Create(ctx context.Context, request Request) (github.PullRequest, error) {
	if s == nil {
		return github.PullRequest{}, errors.New("revert Git and GitHub ports are required")
	}
	git := s.git
	if git == nil {
		git = s.Git
	}
	gh := s.github
	if gh == nil {
		gh = s.GitHub
	}
	if git == nil || gh == nil {
		return github.PullRequest{}, errors.New("revert Git and GitHub ports are required")
	}
	normalized, err := validateRequest(request)
	if err != nil {
		return github.PullRequest{}, err
	}
	if err := git.CreateManagedWorktree(ctx, normalized.repositoryPath, normalized.worktreePath, normalized.branch, normalized.baseCommit); err != nil {
		return github.PullRequest{}, err
	}
	if err := git.RevertMergeCommit(ctx, normalized.worktreePath, normalized.mergeSHA); err != nil {
		if !errors.Is(err, ErrConflict) {
			return github.PullRequest{}, err
		}
		abortErr := git.AbortRevert(ctx, normalized.worktreePath)
		if abortErr != nil {
			return github.PullRequest{}, &BlockedError{Cause: errors.Join(err, fmt.Errorf("abort revert: %w", abortErr))}
		}
		return github.PullRequest{}, &BlockedError{Cause: err}
	}
	if err := git.PushBranch(ctx, normalized.worktreePath, normalized.remote, normalized.branch); err != nil {
		return github.PullRequest{}, err
	}
	pr, err := gh.CreateDraftPR(ctx, normalized.repository, github.DraftPRRequest{
		Title:       "Revert merge " + normalized.mergeSHA[:12],
		Body:        normalized.body,
		Head:        normalized.branch,
		Base:        normalized.defaultBranch,
		IssueNumber: normalized.parentIssue,
	})
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
}

func validateRequest(request Request) (validatedRequest, error) {
	repository := request.Repository
	if repository.Owner == "" && repository.Name == "" {
		repository = request.RepositoryRef
	}
	if repository.Owner == "" && repository.Name == "" {
		repository = request.Repo
	}
	if strings.TrimSpace(repository.Owner) == "" || strings.TrimSpace(repository.Name) == "" || strings.TrimSpace(repository.Owner) != repository.Owner || strings.TrimSpace(repository.Name) != repository.Name || strings.ContainsAny(repository.Owner+repository.Name, "/\\\x00\r\n") {
		return validatedRequest{}, errors.New("revert repository is invalid")
	}
	repositoryPathInput := request.RepositoryPath
	if repositoryPathInput == "" {
		repositoryPathInput = request.RepoPath
	}
	repositoryPath, err := resolveExistingDirectory(repositoryPathInput)
	if err != nil || isFilesystemRoot(repositoryPath) {
		return validatedRequest{}, ErrUnsafeTarget
	}
	managedRoot, err := resolveExistingDirectory(request.ManagedRoot)
	if err != nil || isFilesystemRoot(managedRoot) {
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
	if baseBranch == "" {
		baseBranch = request.CurrentDefaultBranch
	}
	if baseBranch == "" {
		baseBranch = request.BaseBranch
	} else if request.BaseBranch != "" && request.BaseBranch != baseBranch {
		return validatedRequest{}, errors.New("revert default and base branches differ")
	}
	if baseBranch == "" {
		baseBranch = repository.DefaultBranch
	}
	if baseBranch == "" {
		baseBranch = request.Base
	} else if request.Base != "" && request.Base != baseBranch {
		return validatedRequest{}, errors.New("revert default and base branches differ")
	}
	baseCommit := request.BaseCommit
	if baseCommit == "" {
		baseCommit = request.BaseSHA
	}
	mergeSHA := request.MergeSHA
	if mergeSHA == "" {
		mergeSHA = request.MergeCommitSHA
	}
	if !validRef(baseBranch) || !isSHA(baseCommit) || !isSHA(mergeSHA) {
		return validatedRequest{}, errors.New("revert requires valid branches and exact lowercase SHAs")
	}
	parentIssue := request.ParentIssue
	if parentIssue == 0 {
		parentIssue = request.ParentIssueNumber
	}
	if request.Parent.Number > 0 {
		if parentIssue != 0 && parentIssue != request.Parent.Number {
			return validatedRequest{}, errors.New("revert parent issue numbers differ")
		}
		parentIssue = request.Parent.Number
	}
	if parentIssue <= 0 {
		return validatedRequest{}, errors.New("revert parent issue number must be positive")
	}
	branch := fmt.Sprintf("revert/%d-%s", parentIssue, mergeSHA[:12])
	if !validRef(branch) {
		return validatedRequest{}, errors.New("revert branch is invalid")
	}
	worktreePath, err := resolveCreateTarget(request.WorktreePath, managedRoot, branch)
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
	reason := strings.TrimSpace(request.Reason)
	if reason == "" || reason != request.Reason || strings.ContainsRune(reason, '\x00') {
		return validatedRequest{}, errors.New("revert reason must be a canonical non-empty string")
	}
	body := fmt.Sprintf("Reverts merge commit `%s`.\n\nParent issue: #%d\n\nReason: %s", mergeSHA, parentIssue, strconv.Quote(reason))
	return validatedRequest{repository: repository, repositoryPath: repositoryPath, worktreePath: worktreePath, branch: branch, defaultBranch: baseBranch, baseCommit: baseCommit, mergeSHA: mergeSHA, remote: remote, parentIssue: parentIssue, body: body}, nil
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

func resolveCreateTarget(raw, managedRoot, branch string) (string, error) {
	if raw == "" {
		raw = filepath.Join(managedRoot, branch)
	}
	if strings.TrimSpace(raw) != raw {
		return "", ErrUnsafeTarget
	}
	absPath, err := filepath.Abs(raw)
	if err != nil {
		return "", ErrUnsafeTarget
	}
	absPath = filepath.Clean(absPath)
	if _, err := os.Lstat(absPath); err == nil {
		return "", ErrUnsafeTarget
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
