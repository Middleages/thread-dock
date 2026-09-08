package coordinator

import (
	"context"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

// OwnerID identifies the process that currently owns a work item.
type OwnerID string

// OwnerRecord is the persisted identity of a process owner lease.
type OwnerRecord struct {
	WorkID    contractv2.WorkID `json:"workId"`
	OwnerID   OwnerID           `json:"ownerId"`
	PID       int               `json:"pid"`
	StartedAt time.Time         `json:"startedAt"`
}

// OwnerLease holds a work-item ownership lock until Release is called or the
// owning process exits.
type OwnerLease interface {
	Record() OwnerRecord
	Release() error
}

// OwnerLocker acquires process ownership for a work item.
type OwnerLocker interface {
	Acquire(context.Context, contractv2.WorkID, OwnerID, int, time.Time) (OwnerLease, error)
}
