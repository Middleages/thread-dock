// Package github contains the deliberately small GitHub Enterprise Server
// boundary used by the ThreadDock orchestrator.
package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"thread-dock/internal/contract"
)

// Repository identifies a repository on a GHES installation.
type Repository struct {
	Owner string
	Name  string
}

// Issue is the subset of an issue returned by GHES that the orchestrator needs.
type Issue struct {
	Number  int    `json:"number"`
	NodeID  string `json:"node_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

// IssueBundle is the Parent Issue and its Child Issues created for one run.
type IssueBundle struct {
	Parent   Issue
	Children []Issue
}

// DraftPRRequest describes a draft pull request to create.
type DraftPRRequest struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Head        string `json:"head"`
	Base        string `json:"base"`
	IssueNumber int    `json:"-"`
}

// MaxIssueCommentBytes is the conservative limit applied before posting an
// operator or audit comment.
const MaxIssueCommentBytes = 16 * 1024

// MaxDraftPRTitleBytes and MaxDraftPRBodyBytes bound the safe draft-PR port
// before any filesystem or network side effect.
const (
	MaxDraftPRTitleBytes = 256
	MaxDraftPRBodyBytes  = 16 * 1024
)

// PullRequest is the subset of a pull request used by ThreadDock.
type PullRequest struct {
	Number         int    `json:"number"`
	NodeID         string `json:"node_id"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	HTMLURL        string `json:"html_url"`
	State          string `json:"state"`
	Merged         bool   `json:"merged,omitempty"`
	Draft          bool   `json:"draft"`
	Head           string `json:"head"`
	HeadSHA        string `json:"headSha"`
	Base           string `json:"base"`
	Mergeable      *bool  `json:"mergeable"`
	MergeCommitSHA string `json:"mergeCommitSha,omitempty"`
}

// CheckState is the normalized state returned by the Checks API.
type CheckState struct {
	Name  string
	State string
}

// MergePullRequestResult is the typed GitHub merge response.
type MergePullRequestResult struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// MergeError reports a successful HTTP response that did not merge the PR.
type MergeError struct {
	Result MergePullRequestResult
}

func (e *MergeError) Error() string { return "github pull request was not merged" }

// EndpointError is a safe error for the newer optional REST ports. Provider
// response bodies are intentionally discarded rather than exposed.
type EndpointError struct {
	Operation  string
	StatusCode int
}

func (e *EndpointError) Error() string {
	if e == nil {
		return "github endpoint failed"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("github %s failed (%d)", e.Operation, e.StatusCode)
	}
	return fmt.Sprintf("github %s failed", e.Operation)
}

// ProjectRef identifies an Organization Project status field and its options.
type ProjectRef struct {
	ID            string
	StatusFieldID string
	StatusOptions map[string]string
}

type ProjectStatus struct {
	Found  bool
	ItemID string
	Status string
}

// ProjectStatusReader reads one exact ProjectV2 item/field for reconciliation
// after a status mutation intent survives a crash.
type ProjectStatusReader interface {
	ReadProjectStatus(context.Context, ProjectRef, string) (ProjectStatus, error)
}

type ProjectItemAdder interface {
	AddProjectItem(context.Context, ProjectRef, string) (string, error)
}

type ProjectStatusUpdater interface {
	UpdateProjectStatus(context.Context, ProjectRef, string, string) error
}

// Client is the narrow GHES interface used by the orchestrator.
type Client interface {
	FindIssueBundle(context.Context, Repository, string) (IssueBundle, bool, error)
	CreateIssueBundle(context.Context, Repository, contract.TaskContract, string) (IssueBundle, error)
	CreateDraftPR(context.Context, Repository, DraftPRRequest) (PullRequest, error)
	UpdateIssueState(context.Context, Repository, int, string) error
	SetProjectStatus(context.Context, ProjectRef, string, string) error
	GetPullRequest(context.Context, Repository, int) (PullRequest, error)
}

// CheckReader reads normalized checks for one immutable commit.
type CheckReader interface {
	GetChecks(context.Context, Repository, string) ([]CheckState, error)
}

// IssueCommenter creates an issue comment without widening the Client port.
type IssueCommenter interface {
	CreateIssueComment(context.Context, Repository, int, string) error
}

// PullRequestReadier transitions a draft PR to ready for review.
type PullRequestReadier interface {
	MarkReadyForReview(context.Context, Repository, int) (PullRequest, error)
}

// PullRequestFinder finds an existing open PR for an exact head/base pair.
type PullRequestFinder interface {
	FindOpenPullRequest(context.Context, Repository, string, string) (PullRequest, bool, error)
}

// SafeDraftPRCreator creates a draft PR while suppressing provider response
// bodies in endpoint errors. It is intentionally separate from legacy Client.
type SafeDraftPRCreator interface {
	CreateSafeDraftPR(context.Context, Repository, DraftPRRequest) (PullRequest, error)
}

// ValidateSafeDraftPRRequest validates the canonical, bounded request shared
// by the Revert service and REST safe-draft creator.
func ValidateSafeDraftPRRequest(repo Repository, req DraftPRRequest) error {
	if err := validateRepository(repo); err != nil {
		return err
	}
	if req.IssueNumber <= 0 || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Title) != req.Title || len(req.Title) > MaxDraftPRTitleBytes || !safeUserText(req.Title) || strings.TrimSpace(req.Body) == "" || strings.TrimSpace(req.Body) != req.Body || len(req.Body) > MaxDraftPRBodyBytes || !safeUserText(req.Body) || !validRefInput(req.Head) || !validRefInput(req.Base) {
		return errors.New("github draft pull request request is invalid")
	}
	return nil
}

// PullRequestMerger merges a PR only at an exact commit SHA.
type PullRequestMerger interface {
	MergePullRequest(context.Context, Repository, int, string, string) (MergePullRequestResult, error)
}
