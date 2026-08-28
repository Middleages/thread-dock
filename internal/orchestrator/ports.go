// Package orchestrator coordinates one approved ThreadDock contract.
package orchestrator

import (
	"context"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

// Store is the local persistence port used by the state machine.
type Store interface {
	Create(context.Context, state.RunSnapshot) error
	Load(context.Context, contract.RunID) (state.RunSnapshot, error)
	Save(context.Context, state.RunSnapshot) error
	Append(context.Context, contract.RunID, state.Event) error
}

// Worktree is the Git/Worktree port. It deliberately contains only operations
// used by a single run; destructive cleanup is owned by the operator flow.
type Worktree interface {
	Create(context.Context, string, string, string, string) error
	Status(context.Context, string) (string, error)
	Commit(context.Context, string, string) (string, error)
	Merge(context.Context, string, string) error
}

// Git is retained as an alias because the implementation is the Git adapter
// while the orchestration concern is the worktree it operates on.
type Git = Worktree

// Clock supplies time and a testable scheduler. Implementations may provide
// Sleep through Sleeper; a real timer is used when they do not.
type Clock interface {
	Now() time.Time
}

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}

// Dependencies are all side-effecting ports of the single-run state machine.
// RepositoryPath and WorktreeRoot are optional convenience settings for local
// adapters; the contract repository name and contract directory are safe
// fallbacks for tests and callers that do not configure them.
type Dependencies struct {
	Store          Store
	GitHub         github.Client
	Herdr          herdr.Client
	Git            Worktree
	Worktree       Worktree
	Clock          Clock
	RepositoryPath string
	WorktreeRoot   string
	RunID          func(time.Time) contract.RunID
}
