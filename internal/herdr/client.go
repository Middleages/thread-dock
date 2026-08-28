// Package herdr adapts the Herdr v0.8.2 CLI to ThreadDock's runtime boundary.
package herdr

import (
	"context"
	"errors"
)

var ErrClosedWorkspace = errors.New("Herdr workspace is closed and must be reopened")
var ErrAgentNotFound = errors.New("Herdr agent was not found")

type Client interface {
	CreateWorktree(context.Context, CreateWorktreeRequest) (Worktree, error)
	StartAgent(context.Context, StartAgentRequest) error
	Prompt(context.Context, string, string) error
	Get(context.Context, string) (AgentState, error)
	ReadRecent(context.Context, string) (string, error)
}

type CreateWorktreeRequest struct {
	Cwd    string
	Repo   string // Deprecated: use Cwd; retained for the plan's request shape.
	Branch string
	Base   string
	Label  string
}

type Worktree struct {
	WorkspaceID string
	PaneID      string
	Path        string
}

// OpenWorktreeRequest describes a previously-created Git worktree that Herdr
// should expose in a fresh workspace.
type OpenWorktreeRequest struct {
	Cwd   string
	Path  string
	Label string
}

type VerificationCheck struct {
	Command  string `json:"command"`
	Outcome  string `json:"outcome"`
	Duration string `json:"duration"`
}

// Evidence is the only structured Builder result accepted by the
// orchestrator. Terminal transcript text is deliberately not part of it.
type Evidence struct {
	RequestID    string              `json:"requestId"`
	CommitSHA    string              `json:"commitSha"`
	Verification []VerificationCheck `json:"verification"`
}

const EvidenceSchemaExample = `{"requestId":"<prompt request ID>","commitSha":"<40 lowercase hex>","verification":[{"command":"<required command>","outcome":"passed","duration":"<Go duration>"}]}`

const (
	THREADDOCK_EVIDENCE_BEGIN = "THREADDOCK_EVIDENCE_BEGIN"
	THREADDOCK_EVIDENCE_END   = "THREADDOCK_EVIDENCE_END"
	EvidenceBeginMarker       = THREADDOCK_EVIDENCE_BEGIN
	EvidenceEndMarker         = THREADDOCK_EVIDENCE_END
	MaxEvidencePayloadBytes   = 16 * 1024
)

type AgentInfo struct {
	Name           string
	SessionID      string
	WorkspaceID    string
	PaneID         string
	Path           string
	State          AgentState
	StateChangeSeq int64
}

type StartAgentRequest struct {
	Name   string
	PaneID string
}

type AgentState string

const (
	AgentStateWorking AgentState = "working"
	AgentStateBlocked AgentState = "blocked"
	AgentStateIdle    AgentState = "idle"
	AgentStateDone    AgentState = "done"
	AgentStateUnknown AgentState = "unknown"
)

func ParseAgentState(value string) AgentState {
	switch AgentState(value) {
	case AgentStateWorking, AgentStateBlocked, AgentStateIdle, AgentStateDone:
		return AgentState(value)
	default:
		return AgentStateUnknown
	}
}
