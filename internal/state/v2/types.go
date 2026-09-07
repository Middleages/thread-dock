package statev2

import (
	"context"
	"encoding/json"
	"fmt"

	contractv2 "thread-dock/internal/contract/v2"
)

type WorkState string

const (
	StateDraft              WorkState = "draft"
	StateAwaitingApproval   WorkState = "awaiting_approval"
	StateQueued             WorkState = "queued"
	StateRunning            WorkState = "running"
	StatePaused             WorkState = "paused"
	StateNeedsOperator      WorkState = "needs_operator"
	StateReadyForPR         WorkState = "ready_for_pr"
	StateReview             WorkState = "review"
	StatePartiallyMerged    WorkState = "partially_merged"
	StatePublicationPending WorkState = "publication_pending"
	StateCompleted          WorkState = "completed"
)

type Receipt struct {
	RequestID   contractv2.RequestID `json:"requestId"`
	PayloadHash string               `json:"payloadHash"`
	Status      string               `json:"status"`
	Result      json.RawMessage      `json:"result,omitempty"`
}
type WorkSnapshot struct {
	SchemaVersion int                              `json:"schemaVersion"`
	ProjectID     contractv2.ProjectID             `json:"projectId"`
	WorkID        contractv2.WorkID                `json:"workId"`
	Revision      contractv2.Revision              `json:"revision"`
	State         WorkState                        `json:"state"`
	ContractHash  string                           `json:"contractHash"`
	Contract      contractv2.WorkItemContract      `json:"contract"`
	SyncStatus    string                           `json:"syncStatus"`
	NextAction    string                           `json:"nextAction"`
	EvidenceRefs  []string                         `json:"evidenceRefs"`
	Receipts      map[contractv2.RequestID]Receipt `json:"receipts"`
}
type Mutation struct {
	WorkID           contractv2.WorkID
	ExpectedRevision contractv2.Revision
	RequestID        contractv2.RequestID
	PayloadHash      string
	Transition       Transition
}
type Transition func(*WorkSnapshot) error
type Store interface {
	CreatePlan(context.Context, WorkSnapshot) (WorkSnapshot, error)
	Load(context.Context, contractv2.WorkID) (WorkSnapshot, error)
	List(context.Context) ([]WorkSnapshot, error)
	Mutate(context.Context, Mutation) (WorkSnapshot, error)
}

type StaleRevisionError struct {
	CurrentRevision contractv2.Revision
	CurrentState    WorkState
}

func (e *StaleRevisionError) Error() string {
	return fmt.Sprintf("stale revision: current revision %d state %s", e.CurrentRevision, e.CurrentState)
}

func (e *StaleRevisionError) Unwrap() error { return ErrStaleRevision }

type RequestConflictError struct {
	RequestID                 contractv2.RequestID
	ExistingHash, PayloadHash string
}

func (e *RequestConflictError) Error() string {
	return fmt.Sprintf("request %q was already used with a different payload", e.RequestID)
}
