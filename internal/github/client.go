// Package github contains the deliberately small GitHub Enterprise Server
// boundary used by the ThreadDock orchestrator.
package github

import (
	"context"

	"thread-dock/internal/contract"
)

// Repository identifies a repository on a GHES installation.
type Repository struct {
	Owner         string
	Name          string
	DefaultBranch string
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

// PullRequest is the subset of a pull request used by ThreadDock.
type PullRequest struct {
	Number    int    `json:"number"`
	NodeID    string `json:"node_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	Draft     bool   `json:"draft"`
	Head      string `json:"head"`
	HeadSHA   string `json:"headSha"`
	Base      string `json:"base"`
	Mergeable *bool  `json:"mergeable"`
}

// CheckState is the normalized state returned by the Checks API.
type CheckState struct {
	Name  string
	State string
}

// Check is retained as a readable alias for callers that prefer the shorter
// name at the optional port boundary.
type Check = CheckState

// MergePullRequestResult is the typed GitHub merge response.
type MergePullRequestResult struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// MergeResult is a compatibility alias for MergePullRequestResult.
type MergeResult = MergePullRequestResult

// PullRequestMergeResult is a descriptive compatibility alias.
type PullRequestMergeResult = MergePullRequestResult

// MergeResponse is a compatibility alias for MergePullRequestResult.
type MergeResponse = MergePullRequestResult

// MergeError reports a successful HTTP response that did not merge the PR.
type MergeError struct {
	Result MergePullRequestResult
}

// PullRequestMergeError is a descriptive compatibility alias.
type PullRequestMergeError = MergeError

// MergeFailure is a compatibility alias for MergeError.
type MergeFailure = MergeError

func (e *MergeError) Error() string {
	if e == nil {
		return "github pull request was not merged"
	}
	if e.Result.Message != "" {
		return "github pull request was not merged: " + e.Result.Message
	}
	return "github pull request was not merged"
}

// ProjectRef identifies an Organization Project status field and its options.
type ProjectRef struct {
	ID            string
	StatusFieldID string
	StatusOptions map[string]string
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

// PullRequestMerger merges a PR only at an exact commit SHA.
type PullRequestMerger interface {
	MergePullRequest(context.Context, Repository, int, string, string) (MergePullRequestResult, error)
}
