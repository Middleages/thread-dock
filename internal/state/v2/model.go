package statev2

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	contractv2 "thread-dock/internal/contract/v2"
)

const (
	DefaultRepairLimit           = 2
	DefaultRecoveryLimit         = 1
	MaxPriorAttempts             = 5
	MaxDiagnosticBytes           = 1024
	BlockerKindMalformedArtifact = "malformed_artifact"
)

type TaskStatus string

const (
	TaskPending            TaskStatus = "pending"
	TaskInvocationReserved TaskStatus = "invocation_reserved"
	TaskRunning            TaskStatus = "running"
	TaskTerminationPending TaskStatus = "termination_pending"
	TaskTerminated         TaskStatus = "terminated"
	TaskCandidateReady     TaskStatus = "candidate_ready"
	TaskGateFailed         TaskStatus = "gate_failed"
	TaskGatePassed         TaskStatus = "gate_passed"
	TaskReviewBlocked      TaskStatus = "review_blocked"
	TaskAccepted           TaskStatus = "accepted"
	TaskIntegrated         TaskStatus = "integrated"
	TaskNeedsOperator      TaskStatus = "needs_operator"
)

type WorkControl struct {
	ApprovedContractHash string           `json:"approvedContractHash,omitempty"`
	ApprovalRef          string           `json:"approvalRef,omitempty"`
	PauseRequested       bool             `json:"pauseRequested"`
	Blocker              *OperatorBlocker `json:"blocker,omitempty"`
}

type TaskExecutionState struct {
	TaskID            contractv2.TaskID    `json:"taskId"`
	Status            TaskStatus           `json:"status"`
	BuilderAttempt    uint32               `json:"builderAttempt"`
	RepairCount       uint32               `json:"repairCount"`
	RepairLimit       uint32               `json:"repairLimit"`
	RecoveryCount     uint32               `json:"recoveryCount"`
	RecoveryLimit     uint32               `json:"recoveryLimit"`
	LogicalWork       *LogicalWorkState    `json:"logicalWork,omitempty"`
	Worktree          *WorktreeIdentity    `json:"worktree,omitempty"`
	Invocation        *InvocationState     `json:"invocation,omitempty"`
	Candidate         *CandidateEvidence   `json:"candidate,omitempty"`
	Gate              *GateEvidence        `json:"gate,omitempty"`
	Review            *ReviewEvidence      `json:"review,omitempty"`
	Integration       *IntegrationEvidence `json:"integration,omitempty"`
	PriorAttempts     []AttemptSummary     `json:"priorAttempts"`
	InvocationHistory []InvocationID       `json:"invocationHistory"`
}

type PublicationIntentID string
type PublicationKey string

type PublicationState struct {
	IntentID           PublicationIntentID `json:"intentId"`
	Key                PublicationKey      `json:"key"`
	Generation         uint32              `json:"generation"`
	Kind               PublicationKind     `json:"kind"`
	Status             PublicationStatus   `json:"status"`
	PayloadHash        string              `json:"payloadHash"`
	PayloadRef         string              `json:"payloadRef"`
	Target             PublicationTarget   `json:"target"`
	Receipt            *PublicationReceipt `json:"receipt,omitempty"`
	Attempts           uint32              `json:"attempts"`
	LastError          string              `json:"lastError,omitempty"`
	CompletionRequired bool                `json:"completionRequired,omitempty"`
}

func newTaskExecutionState(id contractv2.TaskID) TaskExecutionState {
	return TaskExecutionState{
		TaskID: id, Status: TaskPending, RepairLimit: DefaultRepairLimit,
		RecoveryLimit: DefaultRecoveryLimit, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{},
	}
}

func initialTaskStates(contract contractv2.WorkItemContract) map[contractv2.TaskID]TaskExecutionState {
	states := make(map[contractv2.TaskID]TaskExecutionState, len(contract.Tasks))
	for _, task := range contract.Tasks {
		states[task.TaskID] = newTaskExecutionState(task.TaskID)
	}
	return states
}

func validateTaskStates(s WorkSnapshot) error {
	if s.TaskStates == nil {
		return errors.New("task states map is required")
	}
	if s.Publications == nil {
		return errors.New("publications map is required")
	}
	canonical := make(map[contractv2.TaskID]struct{}, len(s.Contract.Tasks))
	for _, task := range s.Contract.Tasks {
		canonical[task.TaskID] = struct{}{}
	}
	if len(s.TaskStates) != len(canonical) {
		return errors.New("task states must contain exactly every contract task")
	}
	for key, task := range s.TaskStates {
		if key != task.TaskID {
			return fmt.Errorf("task state key %q does not match task ID %q", key, task.TaskID)
		}
		if _, ok := canonical[key]; !ok {
			return fmt.Errorf("task state %q is not in contract", key)
		}
		if !validTaskStatus(task.Status) {
			return fmt.Errorf("unknown task status %q", task.Status)
		}
		if task.RepairLimit == 0 || task.RecoveryLimit == 0 {
			return fmt.Errorf("task %q has a zero budget limit", key)
		}
		if uint32(len(task.PriorAttempts)) > MaxPriorAttempts {
			return fmt.Errorf("task %q has too many prior attempts", key)
		}
		if task.InvocationHistory == nil {
			if task.Invocation != nil {
				return fmt.Errorf("task %q has active invocation without history", key)
			}
			task.InvocationHistory = []InvocationID{}
			s.TaskStates[key] = task
		}
		seenInvocations := make(map[InvocationID]struct{}, len(task.InvocationHistory))
		for _, invocationID := range task.InvocationHistory {
			if strings.TrimSpace(string(invocationID)) == "" {
				return fmt.Errorf("task %q has an empty invocation history ID", key)
			}
			if _, seen := seenInvocations[invocationID]; seen {
				return fmt.Errorf("task %q has duplicate invocation history ID %q", key, invocationID)
			}
			seenInvocations[invocationID] = struct{}{}
		}
		if task.Invocation != nil {
			if task.Invocation.InvocationID == "" {
				return fmt.Errorf("task %q has active invocation without an ID", key)
			}
			if _, ok := seenInvocations[task.Invocation.InvocationID]; !ok {
				return fmt.Errorf("task %q active invocation is absent from history", key)
			}
		}
		for _, attempt := range task.PriorAttempts {
			if err := validateDiagnostic(attempt.Diagnostic); err != nil {
				return fmt.Errorf("task %q: %w", key, err)
			}
		}
		if err := validateTaskEvidence(task, s.Control.Blocker); err != nil {
			return fmt.Errorf("task %q: %w", key, err)
		}
	}
	byKey := make(map[PublicationKey]map[uint32]PublicationIntentID)
	conflictIntents := make([]PublicationIntentID, 0, 1)
	for id, publication := range s.Publications {
		if id == "" || id != publication.IntentID || strings.TrimSpace(string(publication.IntentID)) != string(publication.IntentID) {
			return fmt.Errorf("publication key %q does not match intent ID %q", id, publication.IntentID)
		}
		if strings.TrimSpace(string(publication.Key)) != string(publication.Key) || publication.Key == "" || publication.Generation == 0 {
			return fmt.Errorf("publication %q has invalid identity", id)
		}
		if !validPublicationKind(publication.Kind) {
			return fmt.Errorf("publication %q has unknown kind %q", id, publication.Kind)
		}
		if !validPublicationStatus(publication.Status) {
			return fmt.Errorf("publication %q has unknown status %q", id, publication.Status)
		}
		if err := validatePublicationState(publication); err != nil {
			return fmt.Errorf("publication %q: %w", id, err)
		}
		gens := byKey[publication.Key]
		if gens == nil {
			gens = map[uint32]PublicationIntentID{}
			byKey[publication.Key] = gens
		}
		if prior, ok := gens[publication.Generation]; ok && prior != id {
			return fmt.Errorf("publication %q duplicates key generation", id)
		}
		gens[publication.Generation] = id
		if publication.Status == PublicationConflict {
			conflictIntents = append(conflictIntents, publication.IntentID)
		}
	}
	sort.Slice(conflictIntents, func(i, j int) bool { return conflictIntents[i] < conflictIntents[j] })
	if len(conflictIntents) > 1 {
		return errors.New("multiple publication conflicts are not allowed")
	}
	if len(conflictIntents) == 1 && (s.Control.Blocker == nil || s.Control.Blocker.Kind != BlockerKindPublicationConflict || s.Control.Blocker.IntentID != conflictIntents[0]) {
		return errors.New("publication conflict requires matching blocker")
	}
	for key, gens := range byKey {
		max := uint32(0)
		for gen := range gens {
			if gen > max {
				max = gen
			}
		}
		for gen := uint32(1); gen <= max; gen++ {
			if _, ok := gens[gen]; !ok {
				return fmt.Errorf("publication key %q has non-contiguous generations", key)
			}
		}
		for _, id := range gens {
			publication := s.Publications[id]
			if publication.Status == PublicationSuperseded && publication.Generation >= max {
				return fmt.Errorf("superseded publication %q has no newer generation", id)
			}
		}
	}
	if s.Control.Blocker != nil {
		if s.Control.Blocker.Kind == BlockerKindPublicationConflict {
			publication, ok := s.Publications[s.Control.Blocker.IntentID]
			if s.Control.Blocker.IntentID == "" || !ok || publication.Status != PublicationConflict || publication.Generation != maxPublicationGeneration(&s, publication.Key) {
				return errors.New("publication conflict blocker intent is required")
			}
		} else if strings.TrimSpace(string(s.Control.Blocker.TaskID)) == "" {
			return errors.New("operator blocker task is not in contract")
		}
	}
	if s.Control.Blocker != nil {
		if err := validateDiagnostic(s.Control.Blocker.Diagnostic); err != nil {
			return fmt.Errorf("operator blocker: %w", err)
		}
	}
	if s.Control.Blocker != nil {
		if strings.TrimSpace(s.Control.Blocker.Kind) == "" || strings.TrimSpace(s.Control.Blocker.OperatorRef) == "" || (s.Control.Blocker.Kind != BlockerKindPublicationConflict && strings.TrimSpace(string(s.Control.Blocker.TaskID)) == "") || strings.TrimSpace(s.Control.Blocker.Diagnostic) == "" {
			return errors.New("operator blocker identity is required")
		}
		if !validOperatorBlockerKind(s.Control.Blocker.Kind) {
			return fmt.Errorf("operator blocker kind is unknown: %q", s.Control.Blocker.Kind)
		}
		if s.Control.Blocker.Kind != BlockerKindPublicationConflict {
			blockedTask, ok := s.TaskStates[contractv2.TaskID(s.Control.Blocker.TaskID)]
			if !ok {
				return errors.New("operator blocker task is not in contract")
			}
			if blockedTask.Status != TaskNeedsOperator {
				return errors.New("operator blocker task is not needs_operator")
			}
			if s.Control.Blocker.InvocationID != "" && (blockedTask.Invocation == nil || blockedTask.Invocation.InvocationID != s.Control.Blocker.InvocationID) {
				return errors.New("operator blocker invocation does not match task")
			}
		}
	}
	return nil
}

func validateTaskEvidence(task TaskExecutionState, blocker *OperatorBlocker) error {
	if task.Invocation != nil {
		if err := validateInvocation(task); err != nil {
			return err
		}
	}
	if task.Status == TaskNeedsOperator && blocker != nil && !validOperatorBlockerKind(blocker.Kind) {
		return fmt.Errorf("operator blocker kind is unknown: %q", blocker.Kind)
	}
	switch task.Status {
	case TaskCandidateReady:
		if task.Candidate == nil {
			return errors.New("candidate_ready task has no candidate evidence")
		}
	case TaskGatePassed, TaskGateFailed:
		if task.Candidate == nil || task.Gate == nil {
			return errors.New("gate task has incomplete evidence")
		}
	case TaskAccepted, TaskReviewBlocked:
		if task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review == nil {
			return errors.New("review task has incomplete evidence")
		}
	case TaskIntegrated:
		if task.Candidate == nil || task.Review == nil || !task.Review.Accepted || task.Integration == nil {
			return errors.New("integrated task has incomplete evidence")
		}
	}
	if task.Invocation != nil {
		inv := task.Invocation
		switch task.Status {
		case TaskInvocationReserved:
			if inv.StartedAt != nil || inv.EndedAt != nil || inv.TerminationConfirmed {
				return errors.New("reserved invocation has lifecycle completion")
			}
		case TaskRunning, TaskTerminationPending:
			if inv.StartedAt == nil || inv.EndedAt != nil || inv.TerminationConfirmed {
				return errors.New("active invocation lifecycle is inconsistent")
			}
		case TaskTerminated, TaskCandidateReady, TaskGateFailed, TaskGatePassed, TaskReviewBlocked, TaskAccepted, TaskIntegrated:
			if inv.EndedAt == nil || !inv.TerminationConfirmed {
				return errors.New("evidence stage lacks terminated invocation")
			}
		}
	}
	if task.Candidate != nil {
		if task.Candidate.BuilderAttempt == 0 || task.Candidate.BuilderAttempt != task.BuilderAttempt || !validSHA(task.Candidate.CandidateSHA) || !validSHA(task.Candidate.TreeSHA) || task.Candidate.ChangedFiles == nil {
			return errors.New("invalid candidate evidence")
		}
		seen := map[string]bool{}
		for _, path := range task.Candidate.ChangedFiles {
			if !validRepositoryPath(path) || seen[path] {
				return errors.New("invalid candidate changed files")
			}
			seen[path] = true
		}
		if err := validateDiagnostic(task.Candidate.Diagnostic); err != nil {
			return err
		}
	}
	if task.Gate != nil {
		gateStatusMatches := true
		switch task.Status {
		case TaskGatePassed:
			gateStatusMatches = task.Gate.Passed
		case TaskGateFailed:
			gateStatusMatches = !task.Gate.Passed
		case TaskAccepted, TaskReviewBlocked, TaskIntegrated:
			gateStatusMatches = task.Gate.Passed
		}
		if task.Candidate == nil || task.Gate.BuilderAttempt != task.BuilderAttempt || task.Gate.BuilderAttempt != task.Candidate.BuilderAttempt || task.Gate.CandidateSHA != task.Candidate.CandidateSHA || !gateStatusMatches || task.Gate.ObservedAt.IsZero() || task.Gate.ObservedAt.Location() != time.UTC || len(task.Gate.Commands) == 0 || len(task.Gate.Commands) != len(task.Gate.Outcomes) {
			return fmt.Errorf("invalid gate evidence: status=%q candidate=%#v gate=%#v", task.Status, task.Candidate, task.Gate)
		}
		for i := range task.Gate.Commands {
			if strings.TrimSpace(task.Gate.Commands[i]) != task.Gate.Commands[i] || strings.TrimSpace(task.Gate.Outcomes[i]) != task.Gate.Outcomes[i] || task.Gate.Commands[i] == "" || task.Gate.Outcomes[i] == "" {
				return errors.New("invalid gate command and outcome")
			}
		}
		if err := validateDiagnostic(task.Gate.Diagnostic); err != nil {
			return err
		}
	}
	if task.Review != nil {
		reviewStatusMatches := true
		if task.Status == TaskAccepted || task.Status == TaskIntegrated {
			reviewStatusMatches = task.Review.Accepted
		}
		if task.Status == TaskReviewBlocked {
			reviewStatusMatches = !task.Review.Accepted
		}
		if task.Candidate == nil || task.Review.BuilderAttempt != task.BuilderAttempt || task.Review.CandidateSHA != task.Candidate.CandidateSHA || task.Review.ReviewSHA != task.Candidate.CandidateSHA || task.Review.ObservedAt.IsZero() || task.Review.ObservedAt.Location() != time.UTC || task.Review.Findings == nil || !reviewStatusMatches {
			return errors.New("invalid review evidence")
		}
		if task.Invocation == nil || task.Invocation.Role != roleReviewer || !task.Invocation.TerminationConfirmed || task.Invocation.EndedAt == nil || task.Review.ReviewerInvocationID != task.Invocation.InvocationID {
			return errors.New("review invocation does not match terminated reviewer")
		}
		blocking := 0
		if err := validateDiagnostic(task.Review.Diagnostic); err != nil {
			return err
		}
		for _, finding := range task.Review.Findings {
			if strings.TrimSpace(finding.Code) == "" {
				return errors.New("review finding code is required")
			}
			if strings.EqualFold(finding.Severity, "blocking") {
				blocking++
			}
			if err := validateDiagnostic(finding.Diagnostic); err != nil {
				return err
			}
		}
		if task.Review.Accepted && blocking != 0 || !task.Review.Accepted && len(task.Review.Findings) == 0 {
			return errors.New("review acceptance does not match findings")
		}
	}
	if task.Integration != nil {
		if task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review == nil || !task.Review.Accepted || task.Integration.BuilderAttempt != task.BuilderAttempt || task.Integration.CandidateSHA != task.Candidate.CandidateSHA || !validSHA(task.Integration.IntegrationHEAD) || task.Integration.IntegrationHEAD == task.Candidate.CandidateSHA || !task.Integration.RelationVerified || task.Integration.ObservedAt.IsZero() || task.Integration.ObservedAt.Location() != time.UTC {
			return errors.New("invalid integration evidence")
		}
		if err := validateDiagnostic(task.Integration.Diagnostic); err != nil {
			return err
		}
	}
	return validateTaskStageMatrix(task, blocker)
}

func validateTaskStageMatrix(task TaskExecutionState, blocker *OperatorBlocker) error {
	inv := task.Invocation
	terminated := inv != nil && inv.Role != "" && inv.TerminationConfirmed && inv.EndedAt != nil
	noEvidence := task.Candidate == nil && task.Gate == nil && task.Review == nil && task.Integration == nil
	switch task.Status {
	case TaskPending:
		if inv != nil || !noEvidence {
			return errors.New("pending task has active invocation or evidence")
		}
	case TaskInvocationReserved, TaskRunning, TaskTerminationPending:
		if inv == nil {
			return errors.New("active task has no invocation")
		}
		if inv.Role == roleBuilder {
			if inv.ReturnStage != TaskPending && inv.ReturnStage != TaskGateFailed && inv.ReturnStage != TaskReviewBlocked || !noEvidence {
				return errors.New("builder active lifecycle is incoherent")
			}
		} else if inv.Role == roleReviewer {
			if inv.ReturnStage != TaskGatePassed || task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review != nil || task.Integration != nil {
				return errors.New("reviewer active lifecycle is incoherent")
			}
		} else {
			return errors.New("active invocation role is invalid")
		}
	case TaskCandidateReady:
		if !terminated || inv.Role != roleBuilder || task.Candidate == nil || task.Gate != nil || task.Review != nil || task.Integration != nil {
			return errors.New("candidate_ready lifecycle is incoherent")
		}
	case TaskGateFailed, TaskGatePassed:
		if !terminated || inv.Role != roleBuilder || task.Candidate == nil || task.Gate == nil || task.Review != nil || task.Integration != nil || task.Gate.Passed != (task.Status == TaskGatePassed) {
			return errors.New("gate lifecycle is incoherent")
		}
	case TaskReviewBlocked, TaskAccepted:
		if !terminated || inv.Role != roleReviewer || task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review == nil || task.Integration != nil || task.Review.Accepted != (task.Status == TaskAccepted) {
			return errors.New("review lifecycle is incoherent")
		}
	case TaskIntegrated:
		if !terminated || inv.Role != roleReviewer || task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review == nil || !task.Review.Accepted || task.Integration == nil || !task.Integration.RelationVerified {
			return errors.New("integration lifecycle is incoherent")
		}
	case TaskTerminated:
		if inv == nil || !terminated {
			return errors.New("terminated task has no terminated invocation")
		}
		if inv.Role == roleBuilder {
			if !noEvidence {
				return errors.New("terminated builder lifecycle is incoherent")
			}
		} else if inv.Role == roleReviewer {
			if inv.ReturnStage != TaskGatePassed || task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review != nil || task.Integration != nil {
				return errors.New("terminated reviewer lifecycle is incoherent")
			}
		} else {
			return errors.New("terminated invocation role is invalid")
		}
	case TaskNeedsOperator:
		if task.Integration != nil {
			if !terminated || inv.Role != roleReviewer || task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || task.Review == nil || !task.Review.Accepted || !task.Integration.RelationVerified {
				return errors.New("operator integration lifecycle is incoherent")
			}
		} else if task.Review != nil {
			if !terminated || inv.Role != roleReviewer || task.Gate == nil || !task.Gate.Passed || task.Candidate == nil {
				return errors.New("operator review lifecycle is incoherent")
			}
		} else if task.Gate != nil {
			if task.Candidate == nil || inv == nil || (inv.Role == roleReviewer && (!task.Gate.Passed || !validInvocationLifecycleShape(inv))) || (inv.Role == roleBuilder && !terminated) || (inv.Role != roleBuilder && inv.Role != roleReviewer) {
				return errors.New("operator gate lifecycle is incoherent")
			}
		} else if task.Candidate != nil {
			if !terminated || inv.Role != roleBuilder {
				return errors.New("operator candidate lifecycle is incoherent")
			}
		} else if inv != nil {
			if inv.Role != roleBuilder {
				return errors.New("reviewer invocation-only state is invalid")
			}
			if !validBuilderOperatorInvocationShape(inv) {
				return errors.New("operator invocation-only state is invalid")
			}
		} else if inv == nil {
			if !validOperatorBlocker(task, blocker) {
				return errors.New("operator blocker is required without invocation or evidence")
			}
		}
	}
	return nil
}

func validInvocationLifecycleShape(inv *InvocationState) bool {
	if inv == nil {
		return false
	}
	if inv.EndedAt != nil {
		return inv.TerminationConfirmed
	}
	if inv.StartedAt == nil && strings.TrimSpace(inv.TerminationReason) != "" {
		return false
	}
	return !inv.TerminationConfirmed
}

func validOperatorBlocker(task TaskExecutionState, blocker *OperatorBlocker) bool {
	return blocker != nil && blocker.TaskID == task.TaskID && validOperatorBlockerKind(blocker.Kind) && strings.TrimSpace(blocker.OperatorRef) != "" && strings.TrimSpace(blocker.Diagnostic) != ""
}

func validOperatorBlockerKind(kind string) bool {
	switch kind {
	case BlockerKindRepairBudgetExhausted, BlockerKindRecoveryBudgetExhausted, BlockerKindRuntimeUnknown, BlockerKindRetryVerifiedStage, BlockerKindEvidenceMismatch, BlockerKindMalformedArtifact, BlockerKindPublicationConflict:
		return true
	default:
		return false
	}
}

func validBuilderOperatorInvocationShape(inv *InvocationState) bool {
	if inv == nil || inv.Role != roleBuilder {
		return false
	}
	if inv.EndedAt != nil {
		return inv.TerminationConfirmed
	}
	if inv.StartedAt == nil {
		return !inv.TerminationConfirmed && strings.TrimSpace(inv.TerminationReason) == ""
	}
	return !inv.TerminationConfirmed
}

func validateInvocation(task TaskExecutionState) error {
	inv := task.Invocation
	if strings.TrimSpace(inv.Role) == "" {
		return errors.New("invocation role is required")
	}
	if inv.Role != roleBuilder && inv.Role != roleReviewer {
		return errors.New("invocation role is invalid")
	}
	if !validTaskStatus(inv.ReturnStage) {
		return fmt.Errorf("unknown invocation return stage %q", inv.ReturnStage)
	}
	if !validReturnStage(inv.Role, inv.ReturnStage) {
		if inv.Role == roleBuilder {
			return errors.New("builder invocation return stage is invalid")
		}
		return errors.New("reviewer invocation return stage is invalid")
	}
	if strings.TrimSpace(string(inv.InvocationID)) == "" || strings.TrimSpace(string(inv.LogicalWorkID)) == "" || strings.TrimSpace(inv.LogicalProfile) == "" || strings.TrimSpace(inv.RuntimeFingerprint) == "" {
		return errors.New("invocation identity is required")
	}
	if inv.StartedAt != nil && (inv.StartedAt.IsZero() || inv.StartedAt.Location() != time.UTC) || inv.EndedAt != nil && (inv.EndedAt.IsZero() || inv.EndedAt.Location() != time.UTC) {
		return errors.New("invocation timestamps must be nonzero UTC")
	}
	found := false
	for _, id := range task.InvocationHistory {
		if id == inv.InvocationID {
			found = true
			break
		}
	}
	if !found {
		return errors.New("invocation is absent from history")
	}
	if (inv.EndedAt == nil) != !inv.TerminationConfirmed {
		return errors.New("invocation termination lifecycle is inconsistent")
	}
	if task.LogicalWork == nil || task.LogicalWork.LogicalWorkID != inv.LogicalWorkID || task.LogicalWork.Role != inv.Role || task.LogicalWork.BuilderAttempt != task.BuilderAttempt {
		return errors.New("invocation logical work does not match task")
	}
	return nil
}

func validateDiagnostic(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("diagnostic is not valid UTF-8")
	}
	if len([]byte(value)) > MaxDiagnosticBytes {
		return fmt.Errorf("diagnostic exceeds %d bytes", MaxDiagnosticBytes)
	}
	return nil
}

func validatePublicationState(publication PublicationState) error {
	if !validPayloadHash(publication.PayloadHash) || strings.TrimSpace(publication.PayloadRef) != publication.PayloadRef || publication.PayloadRef == "" || len([]byte(publication.PayloadRef)) > MaxDiagnosticBytes || !utf8.ValidString(publication.PayloadRef) {
		return errors.New("invalid publication payload identity")
	}
	if strings.TrimSpace(publication.Target.Host) != publication.Target.Host || publication.Target.Host == "" || publication.Target.Key != publication.Key || strings.TrimSpace(string(publication.Target.Key)) != string(publication.Target.Key) {
		return errors.New("invalid publication target")
	}
	if publication.Target.Repository != "" && strings.TrimSpace(string(publication.Target.Repository)) != string(publication.Target.Repository) || publication.Target.Resource != "" && strings.TrimSpace(publication.Target.Resource) != publication.Target.Resource || publication.Target.Base != "" && strings.TrimSpace(publication.Target.Base) != publication.Target.Base {
		return errors.New("invalid publication target")
	}
	switch publication.Status {
	case PublicationPending:
		if publication.Receipt != nil || publication.LastError != "" || publication.Attempts == 0 {
			return errors.New("pending publication shape is invalid")
		}
	case PublicationCompleted:
		if publication.Receipt == nil || publication.LastError != "" || !validPublicationReceipt(*publication.Receipt) || publication.Attempts == 0 {
			return errors.New("completed publication shape is invalid")
		}
	case PublicationFailed:
		if publication.Receipt != nil || publication.LastError == "" || publication.Attempts == 0 {
			return errors.New("failed publication shape is invalid")
		}
	case PublicationConflict:
		if publication.Receipt != nil || publication.LastError == "" || publication.Attempts == 0 {
			return errors.New("conflict publication shape is invalid")
		}
	case PublicationSuperseded:
		if publication.Receipt != nil || publication.Attempts == 0 {
			return errors.New("superseded publication shape is invalid")
		}
	}
	return validateDiagnostic(publication.LastError)
}

func validPublicationReceipt(receipt PublicationReceipt) bool {
	if receipt.PublishedAt.IsZero() || receipt.PublishedAt.Location() != time.UTC || (receipt.NodeID == "" && receipt.Number == 0 && receipt.URL == "") {
		return false
	}
	if !utf8.ValidString(receipt.NodeID) || !utf8.ValidString(receipt.URL) || !utf8.ValidString(receipt.Base) || !utf8.ValidString(receipt.Head) {
		return false
	}
	return strings.TrimSpace(receipt.NodeID) == receipt.NodeID && strings.TrimSpace(receipt.URL) == receipt.URL && strings.TrimSpace(receipt.Base) == receipt.Base && strings.TrimSpace(receipt.Head) == receipt.Head
}

func validTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskPending, TaskInvocationReserved, TaskRunning, TaskTerminationPending, TaskTerminated, TaskCandidateReady, TaskGateFailed, TaskGatePassed, TaskReviewBlocked, TaskAccepted, TaskIntegrated, TaskNeedsOperator:
		return true
	default:
		return false
	}
}

func normalizeFoundationSnapshot(s WorkSnapshot, taskStatesPresent, publicationsPresent, controlPresent bool) (WorkSnapshot, error) {
	if taskStatesPresent || publicationsPresent {
		return s, nil
	}
	if controlPresent || (s.State != StateAwaitingApproval && s.State != StateQueued) {
		return s, nil
	}
	s.TaskStates = initialTaskStates(s.Contract)
	s.Publications = map[PublicationIntentID]PublicationState{}
	return s, nil
}
