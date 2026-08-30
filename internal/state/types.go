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
	RequestID            string                 `json:"requestId"`
	CommitSHA            string                 `json:"commitSha"`
	ChangedFiles         []string               `json:"changedFiles"`
	Verification         []string               `json:"verification"`
	Branch               string                 `json:"branch"`
	Patch                string                 `json:"patch"`
	VerificationEvidence []VerificationEvidence `json:"verificationEvidence"`
	// IdentitySource records whether SessionID came from a provider session or
	// the terminal fallback. Terminal identities are addressable for
	// reconciliation but never eligible for native resume.
	IdentitySource string `json:"identitySource,omitempty"`
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

type PromptReceipt struct {
	RequestID   string `json:"requestId"`
	BaselineSeq int64  `json:"baselineSeq"`
}

// TaskRunState is the durable execution state for one contract task. The
// nested identities reuse the same evidence, worktree, and prompt records as
// the legacy single-run flow.
type TaskRunState struct {
	State               string        `json:"state"`
	Stage               string        `json:"stage,omitempty"`
	Agent               AgentEvidence `json:"agent"`
	Worktree            WorktreeState `json:"worktree"`
	Prompt              PromptReceipt `json:"prompt"`
	ProgressFingerprint string        `json:"progressFingerprint"`
	PreviousFingerprint string        `json:"previousFingerprint,omitempty"`
	LastProgressAt      time.Time     `json:"lastProgressAt"`
	RecoveryCount       int           `json:"recoveryCount"`
	NativeResume        bool          `json:"nativeResume,omitempty"`
	RepairCount         int           `json:"repairCount,omitempty"`
	RequiresFreshCommit bool          `json:"requiresFreshCommit,omitempty"`
	PreviousCommitSHA   string        `json:"previousCommitSha,omitempty"`
	ExpectedPath        string        `json:"expectedPath,omitempty"`
	ExpectedBranch      string        `json:"expectedBranch,omitempty"`
	ExpectedLabel       string        `json:"expectedLabel,omitempty"`
}

// ReviewFinding is the durable, provider-neutral form of one blocking
// Reviewer/CI finding. It intentionally contains no provider payload.
type ReviewFinding struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Paths   []string `json:"paths"`
}

type RunSnapshot struct {
	ContractVersion  int                     `json:"contractVersion"`
	RunID            contract.RunID          `json:"runId"`
	Phase            contract.RunPhase       `json:"phase"`
	ContractPath     string                  `json:"contractPath"`
	RepositoryPath   string                  `json:"repositoryPath"`
	IntegrationPath  string                  `json:"integrationPath"`
	ParentIssue      int                     `json:"parentIssue"`
	RepairCount      int                     `json:"repairCount"`
	RecoveryCount    int                     `json:"recoveryCount"`
	Builder          AgentEvidence           `json:"builder"`
	Reviewer         AgentEvidence           `json:"reviewer"`
	Integration      WorktreeState           `json:"integration"`
	BuilderWorktree  WorktreeState           `json:"builderWorktree"`
	ReviewerWorktree WorktreeState           `json:"reviewerWorktree"`
	Registration     RegistrationState       `json:"registration"`
	ActionCursor     int                     `json:"actionCursor"`
	PendingAction    string                  `json:"pendingAction"`
	PendingTaskID    string                  `json:"pendingTaskId,omitempty"`
	Summary          string                  `json:"summary"`
	PreviousPhase    contract.RunPhase       `json:"previousPhase,omitempty"`
	BuilderPrompt    PromptReceipt           `json:"builderPrompt"`
	ReviewerPrompt   PromptReceipt           `json:"reviewerPrompt"`
	Tasks            map[string]TaskRunState `json:"tasks,omitempty"`
	// Strategy is additive so legacy snapshots decode as the original
	// Single-run strategy when it is absent.
	Strategy                 string                 `json:"strategy,omitempty"`
	CurrentTask              string                 `json:"currentTask,omitempty"`
	TaskOrder                []string               `json:"taskOrder,omitempty"`
	ReviewerExpectedPath     string                 `json:"reviewerExpectedPath,omitempty"`
	ReviewerExpectedBranch   string                 `json:"reviewerExpectedBranch,omitempty"`
	ReviewerExpectedLabel    string                 `json:"reviewerExpectedLabel,omitempty"`
	IntegrationSHA           string                 `json:"integrationSha,omitempty"`
	RepairBaseSHA            string                 `json:"repairBaseSha,omitempty"`
	IntegrationVerification  []VerificationEvidence `json:"integrationVerification,omitempty"`
	PullRequest              int                    `json:"pullRequest,omitempty"`
	PullRequestURL           string                 `json:"pullRequestUrl,omitempty"`
	PullRequestHeadSHA       string                 `json:"pullRequestHeadSha,omitempty"`
	ExpectedMergeHeadSHA     string                 `json:"expectedMergeHeadSha,omitempty"`
	PullRequestDraft         bool                   `json:"pullRequestDraft,omitempty"`
	PullRequestMerged        bool                   `json:"pullRequestMerged,omitempty"`
	FinalSHA                 string                 `json:"finalSha,omitempty"`
	FinalChecksSHA           string                 `json:"finalChecksSha,omitempty"`
	FinalChecks              []VerificationEvidence `json:"finalChecks,omitempty"`
	MainSHA                  string                 `json:"mainSha,omitempty"`
	ProtectedReasons         []string               `json:"protectedReasons,omitempty"`
	ProtectedConfirmed       bool                   `json:"protectedConfirmed,omitempty"`
	ProjectAutomationEnabled bool                   `json:"projectAutomationEnabled,omitempty"`
	ProjectStatus            string                 `json:"projectStatus,omitempty"`
	ReviewDecision           string                 `json:"reviewDecision,omitempty"`
	ReviewFindings           []ReviewFinding        `json:"reviewFindings,omitempty"`
	ReviewRiskCategories     []string               `json:"reviewRiskCategories,omitempty"`
	CIState                  string                 `json:"ciState,omitempty"`
	MergeabilityKnown        bool                   `json:"mergeabilityKnown,omitempty"`
	Mergeable                bool                   `json:"mergeable,omitempty"`
	MergeabilityReads        int                    `json:"mergeabilityReads,omitempty"`
	MergePreflightReady      bool                   `json:"mergePreflightReady,omitempty"`
	MergeSHA                 string                 `json:"mergeSha,omitempty"`
	UpdatedAt                time.Time              `json:"updatedAt"`
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
