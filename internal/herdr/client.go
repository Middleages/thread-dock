// Package herdr adapts the Herdr v0.8.2 CLI to ThreadDock's runtime boundary.
package herdr

import "context"

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
}

type StartAgentRequest struct {
	Name   string
	Kind   string
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
