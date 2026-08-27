// Package github contains the deliberately small GitHub Enterprise Server
// boundary used by the ThreadDock orchestrator.
package github

import (
	"context"

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

// PullRequest is the subset of a pull request used by ThreadDock.
type PullRequest struct {
	Number  int    `json:"number"`
	NodeID  string `json:"node_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	Head    string `json:"head"`
	Base    string `json:"base"`
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
