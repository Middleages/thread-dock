// Package state persists recoverable local execution state for a ThreadDock
// run. GitHub remains the durable work record; these files are recovery aids.
package state

import (
	"time"

	"thread-dock/internal/contract"
)

type AgentEvidence struct {
	Name                 string                 `json:"name"`
	SessionID            string                 `json:"sessionId"`
	CommitSHA            string                 `json:"commitSha"`
	ChangedFiles         []string               `json:"changedFiles"`
	Verification         []string               `json:"verification"`
	Branch               string                 `json:"branch"`
	Patch                string                 `json:"patch"`
	VerificationEvidence []VerificationEvidence `json:"verificationEvidence"`
}

type VerificationEvidence struct {
	Command  string `json:"command"`
	Outcome  string `json:"outcome"`
	Duration string `json:"duration"`
}

// WorktreeState is the durable identity returned by Herdr or Git. Paths and
// IDs are persisted so a new agentctl process can reconcile instead of
// blindly creating another workspace.
type WorktreeState struct {
	Path        string `json:"path"`
	WorkspaceID string `json:"workspaceId"`
	PaneID      string `json:"paneId"`
	Branch      string `json:"branch"`
}

type RegistrationState struct {
	Status string `json:"status"`
	Marker string `json:"marker"`
	NodeID string `json:"nodeId"`
	Issue  int    `json:"issue"`
}

type RunSnapshot struct {
	ContractVersion  int               `json:"contractVersion"`
	RunID            contract.RunID    `json:"runId"`
	Phase            contract.RunPhase `json:"phase"`
	ContractPath     string            `json:"contractPath"`
	RepositoryPath   string            `json:"repositoryPath"`
	IntegrationPath  string            `json:"integrationPath"`
	ParentIssue      int               `json:"parentIssue"`
	RepairCount      int               `json:"repairCount"`
	RecoveryCount    int               `json:"recoveryCount"`
	Builder          AgentEvidence     `json:"builder"`
	Reviewer         AgentEvidence     `json:"reviewer"`
	Integration      WorktreeState     `json:"integration"`
	BuilderWorktree  WorktreeState     `json:"builderWorktree"`
	ReviewerWorktree WorktreeState     `json:"reviewerWorktree"`
	Registration     RegistrationState `json:"registration"`
	ActionCursor     int               `json:"actionCursor"`
	PendingAction    string            `json:"pendingAction"`
	Summary          string            `json:"summary"`
	PreviousPhase    contract.RunPhase `json:"previousPhase,omitempty"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

// Event is one append-only state transition or diagnostic record.
type Event struct {
	RunID     contract.RunID    `json:"runId,omitempty"`
	Type      string            `json:"type"`
	Kind      string            `json:"kind,omitempty"`
	Phase     contract.RunPhase `json:"phase,omitempty"`
	At        time.Time         `json:"at"`
	Timestamp time.Time         `json:"timestamp,omitempty"`
	CreatedAt time.Time         `json:"createdAt,omitempty"`
	Message   string            `json:"message,omitempty"`
	Data      map[string]any    `json:"data,omitempty"`
}
