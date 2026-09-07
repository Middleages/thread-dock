package statev2

import (
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

type LogicalWorkID string
type InvocationID string

type LogicalWorkState struct {
	LogicalWorkID         LogicalWorkID `json:"logicalWorkId"`
	Role                  string        `json:"role"`
	BuilderAttempt        uint32        `json:"builderAttempt"`
	Purpose               string        `json:"purpose"`
	RepairCount           uint32        `json:"repairCount"`
	RecoveryCount         uint32        `json:"recoveryCount"`
	RepairBudgetDebited   bool          `json:"repairBudgetDebited"`
	RecoveryBudgetDebited bool          `json:"recoveryBudgetDebited"`
}

type WorktreeIdentity struct {
	CanonicalPath string `json:"canonicalPath"`
	GitCommonDir  string `json:"gitCommonDir"`
	Branch        string `json:"branch"`
	BaseSHA       string `json:"baseSha"`
}

type ProviderIdentity struct {
	Provider string `json:"provider,omitempty"`
	Session  string `json:"session,omitempty"`
	Pane     string `json:"pane,omitempty"`
	Process  string `json:"process,omitempty"`
}

type InvocationState struct {
	InvocationID         InvocationID         `json:"invocationId"`
	LogicalWorkID        LogicalWorkID        `json:"logicalWorkId"`
	TransitionRequestID  contractv2.RequestID `json:"transitionRequestId"`
	Role                 string               `json:"role"`
	ReturnStage          TaskStatus           `json:"returnStage"`
	LogicalProfile       string               `json:"logicalProfile"`
	RuntimeFingerprint   string               `json:"runtimeFingerprint"`
	ProviderIdentity     string               `json:"providerIdentity,omitempty"`
	ProviderSession      string               `json:"providerSession,omitempty"`
	ProviderPane         string               `json:"providerPane,omitempty"`
	ProviderProcess      string               `json:"providerProcess,omitempty"`
	StartedAt            *time.Time           `json:"startedAt,omitempty"`
	EndedAt              *time.Time           `json:"endedAt,omitempty"`
	LaunchRequested      bool                 `json:"launchRequested"`
	TerminationConfirmed bool                 `json:"terminationConfirmed"`
	TerminationReason    string               `json:"terminationReason,omitempty"`
	TransientFailure     bool                 `json:"transientFailure"`
}

type CandidateEvidence struct {
	BuilderAttempt uint32   `json:"builderAttempt"`
	CandidateSHA   string   `json:"candidateSha"`
	TreeSHA        string   `json:"treeSha"`
	ChangedFiles   []string `json:"changedFiles"`
	Diagnostic     string   `json:"diagnostic,omitempty"`
}

type GateEvidence struct {
	BuilderAttempt uint32    `json:"builderAttempt"`
	CandidateSHA   string    `json:"candidateSha"`
	Commands       []string  `json:"commands"`
	Outcomes       []string  `json:"outcomes"`
	Passed         bool      `json:"passed"`
	ObservedAt     time.Time `json:"observedAt"`
	Diagnostic     string    `json:"diagnostic,omitempty"`
}

type ReviewFinding struct {
	Code       string `json:"code"`
	Severity   string `json:"severity,omitempty"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type ReviewEvidence struct {
	ReviewerInvocationID InvocationID    `json:"reviewerInvocationId"`
	BuilderAttempt       uint32          `json:"builderAttempt"`
	CandidateSHA         string          `json:"candidateSha"`
	ReviewSHA            string          `json:"reviewSha"`
	Accepted             bool            `json:"accepted"`
	Findings             []ReviewFinding `json:"findings"`
	ObservedAt           time.Time       `json:"observedAt"`
	Diagnostic           string          `json:"diagnostic,omitempty"`
}

type IntegrationEvidence struct {
	BuilderAttempt  uint32    `json:"builderAttempt"`
	CandidateSHA    string    `json:"candidateSha"`
	IntegrationHEAD string    `json:"integrationHead"`
	ObservedAt      time.Time `json:"observedAt"`
	Diagnostic      string    `json:"diagnostic,omitempty"`
}

type AttemptSummary struct {
	BuilderAttempt uint32 `json:"builderAttempt"`
	CandidateSHA   string `json:"candidateSha,omitempty"`
	TreeSHA        string `json:"treeSha,omitempty"`
	Outcome        string `json:"outcome"`
	FailureReason  string `json:"failureReason,omitempty"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}

type OperatorBlocker struct {
	Kind         string              `json:"kind"`
	OperatorRef  string              `json:"operatorRef"`
	TaskID       contractv2.TaskID   `json:"taskId,omitempty"`
	InvocationID InvocationID        `json:"invocationId,omitempty"`
	IntentID     PublicationIntentID `json:"intentId,omitempty"`
	Diagnostic   string              `json:"diagnostic"`
}

type PublicationKind string

const (
	PublicationParentIssue PublicationKind = "parent_issue"
	PublicationChildIssue  PublicationKind = "child_issue"
	PublicationProjectItem PublicationKind = "project_item"
	PublicationHandoff     PublicationKind = "handoff"
	PublicationPullRequest PublicationKind = "pull_request"
	PublicationWiki        PublicationKind = "wiki"
)

type PublicationStatus string

const (
	PublicationPending    PublicationStatus = "pending"
	PublicationCompleted  PublicationStatus = "completed"
	PublicationFailed     PublicationStatus = "failed"
	PublicationConflict   PublicationStatus = "conflict"
	PublicationSuperseded PublicationStatus = "superseded"
)

type PublicationTarget struct {
	Host       string             `json:"host"`
	Repository contractv2.RepoKey `json:"repository,omitempty"`
	Resource   string             `json:"resource,omitempty"`
	Key        PublicationKey     `json:"key"`
	Base       string             `json:"base,omitempty"`
}

type PublicationReceipt struct {
	NodeID      string    `json:"nodeId,omitempty"`
	Number      uint64    `json:"number,omitempty"`
	URL         string    `json:"url,omitempty"`
	Base        string    `json:"base,omitempty"`
	Head        string    `json:"head,omitempty"`
	PublishedAt time.Time `json:"publishedAt"`
}

type WorkAction string

const (
	WorkApprove WorkAction = "approve"
	WorkPause   WorkAction = "pause"
	WorkResume  WorkAction = "resume"
	WorkResolve WorkAction = "resolve"
)

type ResolveKind string

const (
	ResolveRetryVerifiedStage    ResolveKind = "retry_verified_stage"
	ResolveExtendBudget          ResolveKind = "extend_budget"
	ResolveRuntimeNotStarted     ResolveKind = "runtime_not_started"
	ResolveRuntimeTerminated     ResolveKind = "runtime_terminated"
	ResolvePublicationReconciled ResolveKind = "publication_reconciled"
)

type BudgetKind string

const (
	BudgetKindRepair   BudgetKind = "repair"
	BudgetKindRecovery BudgetKind = "recovery"
	BudgetRepair                  = BudgetKindRepair
	BudgetRecovery                = BudgetKindRecovery
)

type WorkTransition struct {
	Action       WorkAction      `json:"action"`
	ApprovalRef  string          `json:"approvalRef,omitempty"`
	ContractHash string          `json:"contractHash,omitempty"`
	Resolve      *ResolvePayload `json:"resolve,omitempty"`
}

type ResolvePayload struct {
	Kind         ResolveKind         `json:"kind"`
	OperatorRef  string              `json:"operatorRef"`
	TaskID       contractv2.TaskID   `json:"taskId,omitempty"`
	InvocationID InvocationID        `json:"invocationId,omitempty"`
	IntentID     PublicationIntentID `json:"intentId,omitempty"`
	Evidence     *ResolutionEvidence `json:"evidence,omitempty"`
	Budget       BudgetKind          `json:"budget,omitempty"`
	NewLimit     uint32              `json:"newLimit,omitempty"`
}

type ResolutionEvidence struct {
	OwnerTerminated bool   `json:"ownerTerminated"`
	LaunchRequested bool   `json:"launchRequested"`
	ProviderAbsent  bool   `json:"providerAbsent"`
	RemoteMatch     bool   `json:"remoteMatch"`
	NotPublished    bool   `json:"notPublished"`
	Diagnostic      string `json:"diagnostic,omitempty"`
}

type TaskAction string

const (
	TaskReserveInvocation   TaskAction = "reserve_invocation"
	TaskBeginLaunch         TaskAction = "begin_launch"
	TaskMarkRunning         TaskAction = "mark_running"
	TaskRequestTermination  TaskAction = "request_termination"
	TaskConfirmTermination  TaskAction = "confirm_termination"
	TaskReconcileNotStarted TaskAction = "reconcile_not_started"
	TaskRecordCandidate     TaskAction = "record_candidate"
	TaskRecordGate          TaskAction = "record_gate"
	TaskRecordReview        TaskAction = "record_review"
	TaskRecordIntegration   TaskAction = "record_integration"
	TaskNeedsOperatorAction TaskAction = "needs_operator"
)

type TaskTransition struct {
	TaskID         contractv2.TaskID    `json:"taskId"`
	Action         TaskAction           `json:"action"`
	InvocationID   InvocationID         `json:"invocationId,omitempty"`
	LogicalWorkID  LogicalWorkID        `json:"logicalWorkId,omitempty"`
	Role           string               `json:"role,omitempty"`
	ReturnStage    TaskStatus           `json:"returnStage,omitempty"`
	BuilderAttempt uint32               `json:"builderAttempt,omitempty"`
	At             time.Time            `json:"at"`
	Worktree       *WorktreeIdentity    `json:"worktree,omitempty"`
	Invocation     *InvocationState     `json:"invocation,omitempty"`
	Candidate      *CandidateEvidence   `json:"candidate,omitempty"`
	Gate           *GateEvidence        `json:"gate,omitempty"`
	Review         *ReviewEvidence      `json:"review,omitempty"`
	Integration    *IntegrationEvidence `json:"integration,omitempty"`
	Blocker        *OperatorBlocker     `json:"blocker,omitempty"`
	Transient      bool                 `json:"transient"`
}

type PublicationAction string

const (
	PublicationBegin           PublicationAction = "begin"
	PublicationComplete        PublicationAction = "complete"
	PublicationFail            PublicationAction = "fail"
	PublicationActionConflict  PublicationAction = "conflict"
	PublicationActionSupersede PublicationAction = "supersede"
)

type PublicationTransition struct {
	Action      PublicationAction   `json:"action"`
	IntentID    PublicationIntentID `json:"intentId"`
	Key         PublicationKey      `json:"key,omitempty"`
	Generation  uint32              `json:"generation,omitempty"`
	Kind        PublicationKind     `json:"kind,omitempty"`
	PayloadHash string              `json:"payloadHash,omitempty"`
	PayloadRef  string              `json:"payloadRef,omitempty"`
	Target      *PublicationTarget  `json:"target,omitempty"`
	Receipt     *PublicationReceipt `json:"receipt,omitempty"`
	Diagnostic  string              `json:"diagnostic,omitempty"`
}

type TransitionRequest struct {
	WorkID           contractv2.WorkID      `json:"workId"`
	ExpectedRevision contractv2.Revision    `json:"expectedRevision"`
	RequestID        contractv2.RequestID   `json:"requestId"`
	PayloadHash      string                 `json:"payloadHash"`
	Work             *WorkTransition        `json:"work,omitempty"`
	Task             *TaskTransition        `json:"task,omitempty"`
	Publication      *PublicationTransition `json:"publication,omitempty"`
}

func validPublicationKind(kind PublicationKind) bool {
	switch kind {
	case PublicationParentIssue, PublicationChildIssue, PublicationProjectItem, PublicationHandoff, PublicationPullRequest, PublicationWiki:
		return true
	default:
		return false
	}
}

func validPublicationStatus(status PublicationStatus) bool {
	switch status {
	case PublicationPending, PublicationCompleted, PublicationFailed, PublicationConflict, PublicationSuperseded:
		return true
	default:
		return false
	}
}
