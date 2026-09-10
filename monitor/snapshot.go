package main

import (
	"context"
	"time"

	"thread-dock/internal/runner"
)

// SnapshotSource is the narrow aggregate source exposed to Wails.
type SnapshotSource interface {
	FetchAll(context.Context) (Snapshot, error)
}

// CommandRunner is the process seam used by the GitHub adapter and later
// read-only sources. Implementations must preserve argument boundaries.
type CommandRunner interface {
	Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error)
}

type Freshness struct {
	State        string     `json:"state"`
	SyncStatus   string     `json:"syncStatus"`
	ObservedAt   *time.Time `json:"observedAt,omitempty"`
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
}

type Snapshot struct {
	SchemaVersion int            `json:"schemaVersion"`
	Revision      uint64         `json:"revision"`
	ObservedAt    time.Time      `json:"observedAt"`
	Freshness     Freshness      `json:"freshness"`
	State         string         `json:"state"`
	SyncStatus    string         `json:"syncStatus"`
	NextAction    string         `json:"nextAction"`
	EvidenceRefs  []string       `json:"evidenceRefs"`
	Projects      []Project      `json:"projects"`
	Source        string         `json:"source,omitempty"`
	Notices       []string       `json:"notices"`
	Herdr         *HerdrSnapshot `json:"herdr,omitempty"`
}

type Project struct {
	ProjectID    string     `json:"projectId"`
	Name         string     `json:"name"`
	State        string     `json:"state"`
	SyncStatus   string     `json:"syncStatus"`
	NextAction   string     `json:"nextAction"`
	EvidenceRefs []string   `json:"evidenceRefs"`
	UpdatedAt    *time.Time `json:"updatedAt,omitempty"`
	ObservedAt   *time.Time `json:"observedAt,omitempty"`
	WorkItems    []WorkItem `json:"workItems"`
	Source       string     `json:"source,omitempty"`
	Notices      []string   `json:"notices"`
	Links        []Link     `json:"links"`
}

type WorkItem struct {
	WorkID       string      `json:"workId"`
	Title        string      `json:"title"`
	Request      string      `json:"request,omitempty"`
	State        string      `json:"state"`
	SyncStatus   string      `json:"syncStatus"`
	NextAction   string      `json:"nextAction"`
	EvidenceRefs []string    `json:"evidenceRefs"`
	UpdatedAt    *time.Time  `json:"updatedAt,omitempty"`
	Blocker      string      `json:"blocker,omitempty"`
	Decisions    []Decision  `json:"decisions"`
	Handoffs     []Handoff   `json:"handoffs"`
	Links        []Link      `json:"links"`
	GitHub       *GitHubWork `json:"github,omitempty"`
}

type Decision struct {
	DecisionID string     `json:"decisionId"`
	Summary    string     `json:"summary"`
	Status     string     `json:"status"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
}

type Handoff struct {
	Summary      string     `json:"summary"`
	NextAction   string     `json:"nextAction"`
	EvidenceRefs []string   `json:"evidenceRefs"`
	ObservedAt   *time.Time `json:"observedAt,omitempty"`
}

type Link struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

type GitHubCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	URL        string `json:"url,omitempty"`
}

type GitHubWork struct {
	Kind                   string            `json:"kind"`
	Number                 *int              `json:"number,omitempty"`
	URL                    string            `json:"url,omitempty"`
	State                  string            `json:"state,omitempty"`
	ReviewDecision         string            `json:"reviewDecision,omitempty"`
	Checks                 []GitHubCheck     `json:"checks"`
	RelatedIssueURLs       []string          `json:"relatedIssueUrls"`
	RelatedPullRequestURLs []string          `json:"relatedPullRequestUrls"`
	Fields                 map[string]string `json:"fields"`
	ContentAvailable       bool              `json:"contentAvailable"`
	Author                 string            `json:"author,omitempty"`
	ObservedAt             *time.Time        `json:"observedAt,omitempty"`
	Stale                  bool              `json:"stale,omitempty"`
}
