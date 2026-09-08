package statev2

import (
	"errors"
	"fmt"
	"strings"

	contractv2 "thread-dock/internal/contract/v2"
)

const (
	DefaultRepairLimit   = 2
	DefaultRecoveryLimit = 1
	MaxPriorAttempts     = 5
	MaxDiagnosticBytes   = 1024
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
	IntentID    PublicationIntentID `json:"intentId"`
	Key         PublicationKey      `json:"key"`
	Generation  uint32              `json:"generation"`
	Kind        PublicationKind     `json:"kind"`
	Status      PublicationStatus   `json:"status"`
	PayloadHash string              `json:"payloadHash"`
	PayloadRef  string              `json:"payloadRef"`
	Target      PublicationTarget   `json:"target"`
	Receipt     *PublicationReceipt `json:"receipt,omitempty"`
	Attempts    uint32              `json:"attempts"`
	LastError   string              `json:"lastError,omitempty"`
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
		if err := validateTaskEvidence(task); err != nil {
			return fmt.Errorf("task %q: %w", key, err)
		}
	}
	for id, publication := range s.Publications {
		if id == "" || id != publication.IntentID {
			return fmt.Errorf("publication key %q does not match intent ID %q", id, publication.IntentID)
		}
		if publication.Key == "" || publication.Generation == 0 {
			return fmt.Errorf("publication %q has invalid identity", id)
		}
		if !validPublicationKind(publication.Kind) {
			return fmt.Errorf("publication %q has unknown kind %q", id, publication.Kind)
		}
		if !validPublicationStatus(publication.Status) {
			return fmt.Errorf("publication %q has unknown status %q", id, publication.Status)
		}
		if err := validateDiagnostic(publication.LastError); err != nil {
			return fmt.Errorf("publication %q: %w", id, err)
		}
	}
	if s.Control.Blocker != nil {
		if err := validateDiagnostic(s.Control.Blocker.Diagnostic); err != nil {
			return err
		}
	}
	return nil
}

func validateTaskEvidence(task TaskExecutionState) error {
	if task.Invocation != nil && !validTaskStatus(task.Invocation.ReturnStage) {
		return fmt.Errorf("unknown invocation return stage %q", task.Invocation.ReturnStage)
	}
	if task.Candidate != nil {
		if err := validateDiagnostic(task.Candidate.Diagnostic); err != nil {
			return err
		}
	}
	if task.Gate != nil {
		if err := validateDiagnostic(task.Gate.Diagnostic); err != nil {
			return err
		}
	}
	if task.Review != nil {
		if err := validateDiagnostic(task.Review.Diagnostic); err != nil {
			return err
		}
		for _, finding := range task.Review.Findings {
			if err := validateDiagnostic(finding.Diagnostic); err != nil {
				return err
			}
		}
	}
	if task.Integration != nil {
		if err := validateDiagnostic(task.Integration.Diagnostic); err != nil {
			return err
		}
	}
	return nil
}

func validateDiagnostic(value string) error {
	if len([]byte(value)) > MaxDiagnosticBytes {
		return fmt.Errorf("diagnostic exceeds %d bytes", MaxDiagnosticBytes)
	}
	return nil
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
