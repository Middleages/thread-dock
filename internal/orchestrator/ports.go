// Package orchestrator coordinates one approved ThreadDock contract.
package orchestrator

import (
	"context"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
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

type LockingStore interface {
	Acquire(context.Context, contract.RunID) (*state.Lease, error)
}

type WorktreeInspector interface {
	InspectCommit(context.Context, string, string, string, string) (worktree.CommitInspection, error)
}

type ImmutableMerger interface {
	MergeCommit(context.Context, string, string) error
}

type CurrentCommitLocator interface {
	CurrentCommit(context.Context, string) (string, error)
}

type RemoteHeadLocator interface {
	FetchRemoteHead(context.Context, string, string, string) (string, error)
}

type CommitPresenceReader interface {
	IsAncestor(context.Context, string, string) (bool, error)
}

type IntegrationWorktreeLocator interface {
	ReconcileIntegrationWorktree(context.Context, string, string, string) (bool, error)
}

type WorktreeLocator interface {
	FindWorktree(context.Context, string, string, string) (herdr.Worktree, bool, error)
}

type BranchWorktreeLocator interface {
	FindWorktreeByBranch(context.Context, string, string, string) (herdr.Worktree, bool, error)
}

type AgentLocator interface {
	GetInfo(context.Context, string) (herdr.AgentInfo, error)
}

// WorkspaceReader observes one exact Herdr Workspace during session
// retirement. It is intentionally separate from AgentLocator so each
// Advance can perform one provider operation only.
type WorkspaceReader interface {
	GetWorkspace(context.Context, string) (herdr.WorkspaceInfo, bool, error)
}

// WorkspaceCloser closes one exact Herdr Workspace. Closing a Workspace does
// not remove its Git Worktree or any ThreadDock evidence.
type WorkspaceCloser interface {
	CloseWorkspace(context.Context, string) error
}

// RetirementGitInspector proves the immutable Git identity retained in a
// retirement target before the corresponding Workspace is closed.
type RetirementGitInspector interface {
	InspectRetirementTarget(context.Context, string, string, string, string) (worktree.RetirementProof, error)
}

// RoleAwareRetirementGitInspector lets adapters enforce the distinct trusted
// roots for Reviewer/Integration and Builder targets while preserving the
// original provider-neutral inspector port for legacy implementations.
type RoleAwareRetirementGitInspector interface {
	InspectRetirementTargetForRole(context.Context, string, string, string, string, string) (worktree.RetirementProof, error)
}

// PromptReceiptReader performs the single logical external read used to
// reconcile a pending prompt. Implementations return agent sequence metadata
// and only whether the exact request ID was observed, never terminal output.
type PromptReceiptReader interface {
	ReadPromptReceipt(context.Context, string, string) (herdr.AgentInfo, bool, error)
}

type EvidenceReader interface {
	ReadEvidence(context.Context, string) (herdr.Evidence, error)
}

type WorktreeOpener interface {
	OpenWorktree(context.Context, herdr.OpenWorktreeRequest) (herdr.Worktree, error)
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
	// Project automation is opt-in. The project reference remains structurally
	// available for legacy configuration compatibility, but no ProjectV2 call
	// is made unless ProjectAutomationEnabled is true.
	ProjectAutomationEnabled bool
	Project                  github.ProjectRef
	WorkingWait              time.Duration
	RecoveryLimit            int
	Remote                   string
	// BuilderOpenCodeAgent and ReviewerOpenCodeAgent select the OpenCode
	// profile for their respective roles. They are copied into the initial
	// snapshot so later actions and recovery remain pinned to the run's
	// original routing.
	BuilderOpenCodeAgent  string
	ReviewerOpenCodeAgent string
	// Retirement adapters are optional so legacy Herdr/Git clients continue
	// to satisfy the core dependency set. When unset, the orchestrator
	// discovers the narrow port on Herdr or Git/Worktree.
	WorkspaceReader        WorkspaceReader
	WorkspaceCloser        WorkspaceCloser
	RetirementGitInspector RetirementGitInspector
	// AutoRetireCompletedSessions opts newly completed parallel runs into the
	// one-action Workspace retirement state machine. Configuration parsing
	// supplies the default true; a zero-value Dependencies remains compatible
	// with existing callers and tests.
	AutoRetireCompletedSessions bool
	// HerdrWorktreeRoot is the trusted root retained by Git retirement proof
	// adapters. It is not used to remove anything during session retirement.
	HerdrWorktreeRoot string
}

// ReviewEvidenceReader is optional on legacy Herdr clients and required by
// the parallel strategy before a review can be accepted.
type ReviewEvidenceReader interface {
	ReadReviewEvidence(context.Context, string, string) (herdr.ReviewEvidence, error)
}

// ParallelGit is the optional set of immutable integration and publication
// operations consumed by NewParallel. Keeping these as narrow assertions
// preserves the original Worktree port and test doubles.
type ParallelGit interface {
	MergeCommitNoFF(context.Context, string, string) error
	AbortMerge(context.Context, string) error
	RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error)
	CurrentCommit(context.Context, string) (string, error)
	PushBranch(context.Context, string, string, string) error
}
