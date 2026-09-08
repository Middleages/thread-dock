package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

// State is the narrow durable state port used by the publication dispatcher.
type State interface {
	Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error)
	Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error)
}

// PublicationObservation describes a provider's result without allowing the
// provider to mutate durable workflow state.
type PublicationObservation struct {
	State      string
	Receipt    *statev2.PublicationReceipt
	Diagnostic string
}

const (
	PublicationObservationMatch   = "match"
	PublicationObservationAbsent  = "absent"
	PublicationObservationUnknown = "unknown"
	ObservationMatch              = PublicationObservationMatch
	ObservationAbsent             = PublicationObservationAbsent
	ObservationUnknown            = PublicationObservationUnknown
)

// Publisher observes and, only when proven absent, publishes one immutable intent.
type Publisher interface {
	Observe(context.Context, statev2.PublicationState) (PublicationObservation, error)
	Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error)
}

type CommandResult struct {
	Snapshot statev2.WorkSnapshot
	Err      error
}

var (
	ErrDispatcherClosed   = errors.New("publication dispatcher is closed")
	ErrPublicationStale   = errors.New("publication intent is stale")
	ErrPublicationPaused  = errors.New("publication dispatch is paused")
	ErrPublicationBlocked = errors.New("publication dispatch is blocked")
)

var dispatchRequestSequence atomic.Uint64

func (d *publicationDispatcher) handlePublication(ctx context.Context, q *publicationQueue, intentID statev2.PublicationIntentID) CommandResult {
	if err := ctx.Err(); err != nil {
		return CommandResult{Err: err}
	}
	snapshot, err := d.state.Load(ctx, q.workID)
	if err != nil {
		return CommandResult{Err: err}
	}
	p, err := d.pendingIntent(snapshot, q, intentID)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	if p.Status == statev2.PublicationCompleted {
		return CommandResult{Snapshot: snapshot}
	}
	if p.Status != statev2.PublicationPending && p.Status != statev2.PublicationFailed {
		return CommandResult{Snapshot: snapshot, Err: ErrPublicationStale}
	}
	if p.Status == statev2.PublicationFailed {
		request := d.publicationRequest(snapshot, statev2.PublicationTransition{
			Action: statev2.PublicationBegin, IntentID: p.IntentID, Key: p.Key,
			Generation: p.Generation, Kind: p.Kind, PayloadHash: p.PayloadHash,
			PayloadRef: p.PayloadRef, Target: publicationTargetPtr(p.Target),
			CompletionRequired: boolPtr(p.CompletionRequired),
		})
		snapshot, err = d.state.Apply(ctx, request)
		if err != nil {
			return d.latestResult(ctx, snapshot, err)
		}
		p = snapshot.Publications[intentID]
	}
	// Reload even for an already-pending intent: this is the last durable
	// observation before entering provider code.
	snapshot, err = d.state.Load(ctx, q.workID)
	if err != nil {
		return CommandResult{Err: err}
	}
	p, err = d.pendingIntent(snapshot, q, intentID)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	if p.Status == statev2.PublicationCompleted {
		return CommandResult{Snapshot: snapshot}
	}
	if p.Status != statev2.PublicationPending {
		return CommandResult{Snapshot: snapshot, Err: ErrPublicationStale}
	}
	// This load is deliberately immediately before provider I/O and is also
	// repeated between Observe and Publish.
	if err := d.validateDispatch(snapshot, q, p); err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	observation, err := d.publisher.Observe(ctx, p)
	if err != nil {
		return d.settleFailure(ctx, snapshot, p, "publication observe failed")
	}
	if observation.State == PublicationObservationMatch && validReceipt(observation.Receipt) {
		return d.settleComplete(ctx, snapshot, p, observation.Receipt)
	}
	if observation.State != PublicationObservationAbsent || observation.Receipt != nil {
		return d.settleConflict(ctx, snapshot, p, "publication observation is unknown or mismatched")
	}

	latest, loadErr := d.state.Load(ctx, q.workID)
	if loadErr != nil {
		return CommandResult{Err: loadErr}
	}
	if err := d.validateDispatch(latest, q, p); err != nil {
		return CommandResult{Snapshot: latest, Err: err}
	}
	receipt, err := d.publisher.Publish(ctx, p)
	if err != nil {
		return d.settleFailure(ctx, latest, p, "publication publish failed")
	}
	if !validReceipt(&receipt) {
		return d.settleFailure(ctx, latest, p, "publication publish returned an invalid receipt")
	}
	return d.settleComplete(ctx, latest, p, &receipt)
}

func (d *publicationDispatcher) pendingIntent(snapshot statev2.WorkSnapshot, q *publicationQueue, intentID statev2.PublicationIntentID) (statev2.PublicationState, error) {
	if snapshot.WorkID != q.workID || q.lease.Record().WorkID != q.workID {
		return statev2.PublicationState{}, fmt.Errorf("%w: owner record does not match Work", ErrPublicationStale)
	}
	p, ok := snapshot.Publications[intentID]
	if !ok {
		return statev2.PublicationState{}, fmt.Errorf("%w: publication intent not found", ErrPublicationStale)
	}
	if !isLatestGeneration(snapshot, p) {
		return statev2.PublicationState{}, ErrPublicationStale
	}
	if p.Status == statev2.PublicationCompleted {
		return p, nil
	}
	if snapshot.Control.PauseRequested {
		return statev2.PublicationState{}, ErrPublicationPaused
	}
	if snapshot.Control.Blocker != nil {
		return statev2.PublicationState{}, ErrPublicationBlocked
	}
	if p.Status != statev2.PublicationPending && p.Status != statev2.PublicationFailed {
		return p, nil
	}
	return p, nil
}

func (d *publicationDispatcher) validateDispatch(snapshot statev2.WorkSnapshot, q *publicationQueue, p statev2.PublicationState) error {
	if snapshot.WorkID != q.workID || q.lease.Record().WorkID != q.workID {
		return fmt.Errorf("%w: owner record does not match Work", ErrPublicationStale)
	}
	if snapshot.Control.PauseRequested {
		return ErrPublicationPaused
	}
	if snapshot.Control.Blocker != nil {
		return ErrPublicationBlocked
	}
	current, ok := snapshot.Publications[p.IntentID]
	if !ok || current.Generation != p.Generation || current.Key != p.Key || current.Status != statev2.PublicationPending || !isLatestGeneration(snapshot, current) {
		return ErrPublicationStale
	}
	return nil
}

func isLatestGeneration(snapshot statev2.WorkSnapshot, p statev2.PublicationState) bool {
	if p.Key == "" {
		return false
	}
	var latest uint32
	for _, candidate := range snapshot.Publications {
		if candidate.Key == p.Key && candidate.Generation > latest {
			latest = candidate.Generation
		}
	}
	return latest == p.Generation
}

func (d *publicationDispatcher) settleComplete(ctx context.Context, snapshot statev2.WorkSnapshot, p statev2.PublicationState, receipt *statev2.PublicationReceipt) CommandResult {
	return d.applySettlement(ctx, snapshot, p, statev2.PublicationComplete, receipt, "")
}

func (d *publicationDispatcher) settleFailure(ctx context.Context, snapshot statev2.WorkSnapshot, p statev2.PublicationState, diagnostic string) CommandResult {
	return d.applySettlement(ctx, snapshot, p, statev2.PublicationFail, nil, diagnostic)
}

func (d *publicationDispatcher) settleConflict(ctx context.Context, snapshot statev2.WorkSnapshot, p statev2.PublicationState, diagnostic string) CommandResult {
	return d.applySettlement(ctx, snapshot, p, statev2.PublicationActionConflict, nil, diagnostic)
}

func (d *publicationDispatcher) applySettlement(ctx context.Context, snapshot statev2.WorkSnapshot, p statev2.PublicationState, action statev2.PublicationAction, receipt *statev2.PublicationReceipt, diagnostic string) CommandResult {
	transition := statev2.PublicationTransition{Action: action, IntentID: p.IntentID, Key: p.Key, Generation: p.Generation, Kind: p.Kind, PayloadHash: p.PayloadHash, PayloadRef: p.PayloadRef, Target: publicationTargetPtr(p.Target), CompletionRequired: boolPtr(p.CompletionRequired), Receipt: receipt, Diagnostic: diagnostic}
	if action == statev2.PublicationActionConflict {
		transition.Blocker = &statev2.OperatorBlocker{Kind: statev2.BlockerKindPublicationConflict, OperatorRef: string(d.ownerID), IntentID: p.IntentID, Diagnostic: diagnostic}
	}
	result, err := d.state.Apply(ctx, d.publicationRequest(snapshot, transition))
	if err != nil {
		return d.latestResult(ctx, snapshot, err)
	}
	return CommandResult{Snapshot: result}
}

func (d *publicationDispatcher) latestResult(ctx context.Context, fallback statev2.WorkSnapshot, cause error) CommandResult {
	latest, err := d.state.Load(ctx, fallback.WorkID)
	if err == nil {
		fallback = latest
	}
	return CommandResult{Snapshot: fallback, Err: cause}
}

func (d *publicationDispatcher) publicationRequest(snapshot statev2.WorkSnapshot, transition statev2.PublicationTransition) statev2.TransitionRequest {
	req := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: d.nextRequestID(), Publication: &transition}
	req.PayloadHash, _ = statev2.TransitionPayloadHash(req)
	return req
}

func (d *publicationDispatcher) nextRequestID() contractv2.RequestID {
	seq := dispatchRequestSequence.Add(1)
	return contractv2.RequestID(fmt.Sprintf("publication-dispatch-%d", seq))
}

func publicationTargetPtr(target statev2.PublicationTarget) *statev2.PublicationTarget {
	return &target
}
func boolPtr(value bool) *bool { return &value }

func validReceipt(receipt *statev2.PublicationReceipt) bool {
	return receipt != nil && !receipt.PublishedAt.IsZero() && receipt.PublishedAt.Location() == time.UTC && (receipt.NodeID != "" || receipt.Number != 0 || strings.TrimSpace(receipt.URL) != "")
}
