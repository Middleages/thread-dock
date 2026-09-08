// Package monitor defines the aggregate, provider-neutral status wire.
package monitor

import (
	"context"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

type SnapshotSource interface {
	FetchAll(context.Context) (Snapshot, error)
}

type Freshness struct {
	State        string    `json:"state"`
	SyncStatus   string    `json:"syncStatus"`
	ObservedAt   time.Time `json:"observedAt,omitempty"`
	LastSyncedAt time.Time `json:"lastSyncedAt,omitempty"`
}

type Snapshot struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Revision      contractv2.Revision `json:"revision"`
	ObservedAt    time.Time           `json:"observedAt"`
	Freshness     Freshness           `json:"freshness"`
	State         string              `json:"state"`
	SyncStatus    string              `json:"syncStatus"`
	NextAction    string              `json:"nextAction"`
	EvidenceRefs  []string            `json:"evidenceRefs"`
	Projects      []Project           `json:"projects"`
}

type Project struct {
	ProjectID    contractv2.ProjectID `json:"projectId"`
	Name         string               `json:"name"`
	State        string               `json:"state"`
	SyncStatus   string               `json:"syncStatus"`
	NextAction   string               `json:"nextAction"`
	EvidenceRefs []string             `json:"evidenceRefs"`
	UpdatedAt    time.Time            `json:"updatedAt,omitempty"`
	WorkItems    []WorkItem           `json:"workItems,omitempty"`
}

type WorkItem struct {
	WorkID       contractv2.WorkID   `json:"workId"`
	Title        string              `json:"title"`
	Request      string              `json:"request,omitempty"`
	State        string              `json:"state"`
	SyncStatus   string              `json:"syncStatus"`
	NextAction   string              `json:"nextAction"`
	EvidenceRefs []string            `json:"evidenceRefs"`
	UpdatedAt    time.Time           `json:"updatedAt,omitempty"`
	Tasks        []TaskDetail        `json:"tasks,omitempty"`
	Blocker      string              `json:"blocker,omitempty"`
	Publications []PublicationDetail `json:"publications"`
	Decisions    []DecisionDetail    `json:"decisions,omitempty"`
	Handoffs     []HandoffDetail     `json:"handoffs,omitempty"`
	Links        []Link              `json:"links,omitempty"`
}

type TaskDetail struct {
	TaskID       contractv2.TaskID  `json:"taskId"`
	RepoKey      contractv2.RepoKey `json:"repoKey"`
	State        string             `json:"state"`
	Verification string             `json:"verification,omitempty"`
	Review       string             `json:"review,omitempty"`
	Merge        string             `json:"merge,omitempty"`
}

// PublicationDetail is the provider-neutral portion of a publication intent.
// Payload and target identity remain local state and are deliberately absent.
type PublicationDetail struct {
	IntentID   string `json:"intentId"`
	Key        string `json:"key"`
	Generation uint32 `json:"generation"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Attempts   uint32 `json:"attempts"`
	URL        string `json:"url,omitempty"`
}

type DecisionDetail struct {
	DecisionID string    `json:"decisionId"`
	Summary    string    `json:"summary"`
	Status     string    `json:"status"`
	ObservedAt time.Time `json:"observedAt,omitempty"`
}
type HandoffDetail struct {
	Summary      string    `json:"summary"`
	NextAction   string    `json:"nextAction"`
	EvidenceRefs []string  `json:"evidenceRefs"`
	ObservedAt   time.Time `json:"observedAt,omitempty"`
}
type Link struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	URL   string `json:"url"`
}
