package coordinator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

// ReconcileResult is the immutable projection returned by one reconcile pass.
// It intentionally contains no controller handles or mutable state.
type ReconcileResult struct {
	WorkID       contractv2.WorkID
	State        statev2.WorkState
	NextAction   string
	EvidenceRefs []string
}

// Coordinator owns one process's restart/reconcile operations. It is
// deliberately one-shot: callers decide when to invoke Reconcile and when to
// activate any execution queue.
type Coordinator struct {
	State       State
	Runtime     Runtime
	Publisher   Publisher
	OwnerLocker OwnerLocker
	OwnerID     OwnerID
	PID         int
	StartedAt   time.Time
}

// NewCoordinator constructs a coordinator with stable process metadata. A
// zero PID, start time, or owner ID is filled once at construction time.
func NewCoordinator(state State, runtime Runtime, publisher Publisher, locker OwnerLocker, ownerID OwnerID, pid int, startedAt time.Time) *Coordinator {
	if pid == 0 {
		pid = os.Getpid()
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	} else {
		startedAt = startedAt.UTC()
	}
	if ownerID == "" {
		ownerID = OwnerID(fmt.Sprintf("coordinator-%d-%d", pid, startedAt.UnixNano()))
	}
	return &Coordinator{State: state, Runtime: runtime, Publisher: publisher, OwnerLocker: locker, OwnerID: ownerID, PID: pid, StartedAt: startedAt}
}

var (
	ErrReconcileRuntimeUnknown      = errors.New("runtime state is unknown; operator action required")
	ErrReconcilePublicationConflict = errors.New("publication state is unknown; operator action required")
)

// Reconcile acquires one Work lease, serially settles durable in-flight work,
// and releases the lease on every return. It never launches a background
// worker or activates a queue.
func (c *Coordinator) Reconcile(ctx context.Context, workID contractv2.WorkID) (result ReconcileResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.State == nil || c.OwnerLocker == nil {
		return ReconcileResult{WorkID: workID}, errors.New("coordinator state and owner locker are required")
	}
	lease, err := c.OwnerLocker.Acquire(ctx, workID, c.OwnerID, c.PID, c.StartedAt)
	if err != nil {
		return ReconcileResult{WorkID: workID}, err
	}
	defer func() {
		releaseErr := lease.Release()
		if err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	snapshot, err := c.State.Load(ctx, workID)
	if err != nil {
		return ReconcileResult{WorkID: workID}, err
	}
	if snapshot.WorkID != workID {
		return c.result(snapshot), fmt.Errorf("reconcile work identity mismatch: loaded %q, requested %q", snapshot.WorkID, workID)
	}
	if snapshot.Control.Blocker != nil {
		// An existing operator decision owns the next transition; reconcile does
		// not perform additional provider I/O while that blocker is unresolved.
		return c.result(snapshot), nil
	}

	q := &publicationQueue{workID: workID, lease: lease, owner: lease.Record(), runtimeOps: make(map[runtimeKey]map[runtimeOperationKind]*runtimeOperation)}
	d := &publicationDispatcher{state: c.State, runtime: c.Runtime, publisher: c.Publisher, locker: c.OwnerLocker, ownerID: c.OwnerID, pid: c.PID, startedAt: c.StartedAt}
	for _, contractTask := range snapshot.Contract.Tasks {
		if err := ctx.Err(); err != nil {
			return c.result(snapshot), err
		}
		latest, loadErr := c.State.Load(ctx, workID)
		if loadErr != nil {
			return c.result(snapshot), loadErr
		}
		snapshot = latest
		task, ok := snapshot.TaskStates[contractTask.TaskID]
		if !ok || task.Invocation == nil {
			continue
		}
		updated, taskErr := c.reconcileInvocation(ctx, d, q, snapshot, contractTask.TaskID, task)
		snapshot = updated
		if taskErr != nil {
			return c.result(snapshot), taskErr
		}
	}
	latest, err := c.State.Load(ctx, workID)
	if err != nil {
		return c.result(snapshot), err
	}
	snapshot = latest
	publicationIDs := make([]string, 0, len(snapshot.Publications))
	for id := range snapshot.Publications {
		publicationIDs = append(publicationIDs, string(id))
	}
	sort.Strings(publicationIDs)
	for _, id := range publicationIDs {
		publication := snapshot.Publications[statev2.PublicationIntentID(id)]
		if publication.Status != statev2.PublicationPending && publication.Status != statev2.PublicationFailed {
			continue
		}
		updated, publicationErr := c.reconcilePublication(ctx, d, q, snapshot, publication)
		snapshot = updated
		if publicationErr != nil {
			return c.result(snapshot), publicationErr
		}
		// Only one intent can be settled by a serial pass when a provider
		// transition changes the aggregate. Reload before considering the next.
		latest, loadErr := c.State.Load(ctx, workID)
		if loadErr != nil {
			return c.result(snapshot), loadErr
		}
		snapshot = latest
	}
	latest, err = c.State.Load(ctx, workID)
	if err != nil {
		return c.result(snapshot), err
	}
	return c.result(latest), nil
}

func (c *Coordinator) reconcileInvocation(ctx context.Context, d *publicationDispatcher, q *publicationQueue, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, task statev2.TaskExecutionState) (statev2.WorkSnapshot, error) {
	// Do not rely on the caller's projection: this is the last durable read
	// before any runtime provider call.
	latest, err := c.State.Load(ctx, snapshot.WorkID)
	if err != nil {
		return snapshot, err
	}
	snapshot = latest
	task, ok := snapshot.TaskStates[taskID]
	if !ok {
		return snapshot, ErrRuntimeStale
	}
	if task.Invocation == nil {
		return snapshot, nil
	}
	// An invocation retained on a settled evidence stage is historical
	// provenance, not an outstanding runtime operation.
	if task.Status != statev2.TaskInvocationReserved && task.Status != statev2.TaskRunning && task.Status != statev2.TaskTerminationPending {
		return snapshot, nil
	}
	key := runtimeKey{taskID: taskID, invocationID: task.Invocation.InvocationID}
	if _, _, err := d.validateRuntimeIdentity(snapshot, q, key); err != nil {
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, err.Error())
	}
	invocation := *task.Invocation
	if task.Status == statev2.TaskInvocationReserved && !invocation.LaunchRequested {
		// The successor owns the Work exclusively and the durable false bit is
		// proof that this process never authorized a provider launch.
		transition := runtimeTransitionAt(task, task.Invocation, statev2.TaskReconcileNotStarted, "", nil)
		transition.Resolution = &statev2.ResolutionEvidence{OwnerTerminated: true, LaunchRequested: false, ProviderAbsent: true, Diagnostic: "successor owner acquired before launch authorization"}
		return c.applyTask(ctx, d, snapshot, transition)
	}
	if c.Runtime == nil {
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime adapter is not configured")
	}

	// LaunchRequested is an intent, never a permission to relaunch after a
	// restart. Every path below therefore begins with Observe.
	observation, observeErr := c.Runtime.Observe(ctx, invocation)
	if observeErr != nil {
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, observeErr.Error())
	}
	if _, _, err := d.validateRuntimeIdentity(snapshot, q, key); err != nil {
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, err.Error())
	}

	if task.Status == statev2.TaskTerminationPending {
		if observation.State != RuntimeObservationActive && observation.State != RuntimeObservationEnded {
			return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, runtimeDiagnostic(observation))
		}
		return c.terminateInvocation(ctx, d, q, snapshot, taskID, task, observation)
	}
	switch observation.State {
	case RuntimeObservationActive:
		if strings.TrimSpace(observation.ProviderIdentity) == "" || (invocation.ProviderIdentity != "" && invocation.ProviderIdentity != observation.ProviderIdentity) {
			return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime active identity does not match persisted identity")
		}
		if task.Status == statev2.TaskInvocationReserved {
			transition := runtimeTransitionAt(task, task.Invocation, statev2.TaskMarkRunning, observation.ProviderIdentity, nil)
			updated, err := c.applyTask(ctx, d, snapshot, transition)
			if err != nil {
				return updated, err
			}
			if updated.Control.PauseRequested {
				return c.terminateInvocation(ctx, d, q, updated, taskID, updated.TaskStates[taskID], observation)
			}
			return updated, nil
		}
		if snapshot.Control.PauseRequested {
			return c.terminateInvocation(ctx, d, q, snapshot, taskID, task, observation)
		}
		return snapshot, nil
	case RuntimeObservationEnded:
		if strings.TrimSpace(observation.ProviderIdentity) == "" || (invocation.ProviderIdentity != "" && invocation.ProviderIdentity != observation.ProviderIdentity) {
			return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime ended identity does not match persisted identity")
		}
		if task.Status == statev2.TaskInvocationReserved {
			transition := runtimeTransitionAt(task, task.Invocation, statev2.TaskMarkRunning, observation.ProviderIdentity, nil)
			updated, err := c.applyTask(ctx, d, snapshot, transition)
			if err != nil {
				return updated, err
			}
			snapshot = updated
			task = snapshot.TaskStates[taskID]
		}
		at := observation.EndedAt
		if at == nil {
			now := time.Now().UTC()
			at = &now
		}
		transition := runtimeTransitionAt(task, task.Invocation, statev2.TaskConfirmTermination, observation.Diagnostic, at)
		return c.applyTask(ctx, d, snapshot, transition)
	case RuntimeObservationNotStarted, RuntimeObservationUnknown, "":
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, runtimeDiagnostic(observation))
	default:
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime returned an unsupported observation")
	}
}

func (c *Coordinator) terminateInvocation(ctx context.Context, d *publicationDispatcher, q *publicationQueue, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, task statev2.TaskExecutionState, observation RuntimeObservation) (statev2.WorkSnapshot, error) {
	if task.Invocation == nil || c.Runtime == nil {
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime adapter is not configured")
	}
	if observation.State != RuntimeObservationActive && observation.State != RuntimeObservationEnded {
		return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, runtimeDiagnostic(observation))
	}
	if observation.State == RuntimeObservationEnded {
		if strings.TrimSpace(observation.ProviderIdentity) == "" || observation.ProviderIdentity != task.Invocation.ProviderIdentity {
			return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime ended identity does not match persisted identity")
		}
	} else if observation.State == RuntimeObservationActive {
		if observation.ProviderIdentity != task.Invocation.ProviderIdentity || observation.ProviderIdentity == "" {
			return c.runtimeUnknown(ctx, d, snapshot, taskID, task.Invocation, "runtime active identity does not match persisted identity")
		}
		if task.Status == statev2.TaskRunning {
			transition := runtimeTransitionAt(task, task.Invocation, statev2.TaskRequestTermination, "pause requested", nil)
			var err error
			snapshot, err = c.applyTask(ctx, d, snapshot, transition)
			if err != nil {
				return snapshot, err
			}
		}
	}

	// Re-load and re-check immutable identity immediately before Terminate.
	latest, err := c.State.Load(ctx, snapshot.WorkID)
	if err != nil {
		return snapshot, err
	}
	current, _, err := d.validateRuntimeIdentity(latest, q, runtimeKey{taskID: taskID, invocationID: task.Invocation.InvocationID})
	if err != nil || (current.Status != statev2.TaskTerminationPending && current.Status != statev2.TaskRunning) {
		if err == nil {
			err = ErrRuntimeStale
		}
		return c.runtimeUnknown(ctx, d, latest, taskID, latest.TaskStates[taskID].Invocation, err.Error())
	}
	if observation.State != RuntimeObservationEnded {
		if err := c.Runtime.Terminate(ctx, *current.Invocation); err != nil {
			return c.runtimeUnknown(ctx, d, latest, taskID, current.Invocation, err.Error())
		}
	}
	// Confirmation also uses a fresh snapshot so a concurrent durable identity
	// change cannot be acknowledged as this invocation.
	latest, err = c.State.Load(ctx, snapshot.WorkID)
	if err != nil {
		return snapshot, err
	}
	current, invocation, err := d.validateRuntimeIdentity(latest, q, runtimeKey{taskID: taskID, invocationID: task.Invocation.InvocationID})
	if err != nil || invocation == nil {
		if err == nil {
			err = ErrRuntimeStale
		}
		return c.runtimeUnknown(ctx, d, latest, taskID, latest.TaskStates[taskID].Invocation, err.Error())
	}
	at := observation.EndedAt
	if at == nil {
		now := time.Now().UTC()
		at = &now
	}
	transition := runtimeTransitionAt(current, invocation, statev2.TaskConfirmTermination, "runtime terminated", at)
	return c.applyTask(ctx, d, latest, transition)
}

func (c *Coordinator) runtimeUnknown(ctx context.Context, d *publicationDispatcher, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, invocation *statev2.InvocationState, diagnostic string) (statev2.WorkSnapshot, error) {
	if invocation == nil {
		return snapshot, ErrReconcileRuntimeUnknown
	}
	if diagnostic == "" {
		diagnostic = ErrReconcileRuntimeUnknown.Error()
	}
	if len([]byte(diagnostic)) > statev2.MaxDiagnosticBytes {
		diagnostic = string([]byte(diagnostic)[:statev2.MaxDiagnosticBytes])
	}
	task := snapshot.TaskStates[taskID]
	transition := runtimeTransitionAt(task, invocation, statev2.TaskNeedsOperatorAction, diagnostic, nil)
	transition.Blocker = &statev2.OperatorBlocker{Kind: statev2.BlockerKindRuntimeUnknown, OperatorRef: string(c.OwnerID), TaskID: taskID, InvocationID: invocation.InvocationID, Diagnostic: diagnostic}
	updated, err := c.applyTask(ctx, d, snapshot, transition)
	if err != nil {
		return updated, err
	}
	return updated, ErrReconcileRuntimeUnknown
}

func (c *Coordinator) applyTask(ctx context.Context, d *publicationDispatcher, snapshot statev2.WorkSnapshot, transition statev2.TaskTransition) (statev2.WorkSnapshot, error) {
	request, err := d.runtimeRequest(snapshot, transition)
	if err != nil {
		return snapshot, err
	}
	updated, err := c.State.Apply(ctx, request)
	if err != nil {
		latest, loadErr := c.State.Load(ctx, snapshot.WorkID)
		if loadErr == nil {
			snapshot = latest
		}
		return snapshot, err
	}
	return updated, nil
}

func (c *Coordinator) reconcilePublication(ctx context.Context, d *publicationDispatcher, q *publicationQueue, snapshot statev2.WorkSnapshot, publication statev2.PublicationState) (statev2.WorkSnapshot, error) {
	if c.Publisher == nil {
		return snapshot, errors.New("publisher is not configured")
	}
	// This read is intentionally inside the settlement seam, directly before
	// Observe, so every provider call sees a current immutable intent.
	latest, err := c.State.Load(ctx, snapshot.WorkID)
	if err != nil {
		return snapshot, err
	}
	snapshot = latest
	current, ok := snapshot.Publications[publication.IntentID]
	if !ok || !sameImmutablePublication(current, publication) {
		return snapshot, ErrPublicationStale
	}
	publication = current
	if snapshot.Control.PauseRequested && publication.Status == statev2.PublicationFailed {
		// Retrying a failed intent begins a new provider attempt; pause forbids
		// that new attempt just as it forbids Publish.
		return snapshot, nil
	}
	if publication.Status == statev2.PublicationFailed {
		transition := statev2.PublicationTransition{Action: statev2.PublicationBegin, IntentID: publication.IntentID, Key: publication.Key, Generation: publication.Generation, Kind: publication.Kind, PayloadHash: publication.PayloadHash, PayloadRef: publication.PayloadRef, Target: publicationTargetPtr(publication.Target), CompletionRequired: boolPtr(publication.CompletionRequired)}
		request, err := d.publicationRequest(snapshot, transition)
		if err != nil {
			return snapshot, err
		}
		updated, err := c.State.Apply(ctx, request)
		if err != nil {
			latest, loadErr := c.State.Load(ctx, snapshot.WorkID)
			if loadErr == nil {
				return latest, err
			}
			return snapshot, err
		}
		snapshot = updated
		latest, loadErr := c.State.Load(ctx, snapshot.WorkID)
		if loadErr != nil {
			return snapshot, loadErr
		}
		snapshot = latest
		publication = snapshot.Publications[publication.IntentID]
	}
	if snapshot.Control.Blocker != nil {
		return snapshot, nil
	}
	if !samePublicationLease(d, q) {
		return snapshot, ErrPublicationStale
	}
	observed, err := c.Publisher.Observe(ctx, publication)
	if err != nil {
		// Observe errors cannot prove absence or non-publication. Persist only a
		// bounded static conflict diagnostic; provider error text may contain
		// credentials or request data and must not become durable evidence.
		return c.applyPublicationSettlement(ctx, d, snapshot, publication, statev2.PublicationActionConflict, nil, "publication observation unavailable; operator reconciliation required")
	}
	latest, loadErr := c.State.Load(ctx, snapshot.WorkID)
	if loadErr != nil {
		return snapshot, loadErr
	}
	current, ok = latest.Publications[publication.IntentID]
	if !ok || !sameImmutablePublication(current, publication) || current.Status != statev2.PublicationPending {
		return latest, ErrPublicationStale
	}
	paused := latest.Control.PauseRequested
	if observed.State == PublicationObservationMatch && validReceipt(observed.Receipt) {
		return c.applyPublicationSettlement(ctx, d, latest, current, statev2.PublicationComplete, observed.Receipt, "")
	}
	if observed.State != PublicationObservationAbsent || observed.Receipt != nil {
		return c.applyPublicationSettlement(ctx, d, latest, current, statev2.PublicationActionConflict, nil, "publication observation is unknown or mismatched")
	}
	if paused {
		// Proven absence while paused is durable information, not permission to
		// issue a new provider write.
		return latest, nil
	}
	// This load is immediately before Publish and repeats immutable identity,
	// pause, and owner checks.
	latest, loadErr = c.State.Load(ctx, latest.WorkID)
	if loadErr != nil {
		return latest, loadErr
	}
	current, ok = latest.Publications[publication.IntentID]
	if !ok || !sameImmutablePublication(current, publication) || current.Status != statev2.PublicationPending || latest.Control.PauseRequested || !samePublicationLease(d, q) {
		return latest, ErrPublicationStale
	}
	receipt, err := c.Publisher.Publish(ctx, current)
	if err != nil {
		return c.applyPublicationSettlement(ctx, d, latest, current, statev2.PublicationFail, nil, "publication publish failed")
	}
	if !validReceipt(&receipt) {
		return c.applyPublicationSettlement(ctx, d, latest, current, statev2.PublicationFail, nil, "publication publish returned an invalid receipt")
	}
	return c.applyPublicationSettlement(ctx, d, latest, current, statev2.PublicationComplete, &receipt, "")
}

func samePublicationLease(d *publicationDispatcher, q *publicationQueue) bool {
	return ownerRecordMatches(d, q)
}

func (c *Coordinator) applyPublicationSettlement(ctx context.Context, d *publicationDispatcher, snapshot statev2.WorkSnapshot, publication statev2.PublicationState, action statev2.PublicationAction, receipt *statev2.PublicationReceipt, diagnostic string) (statev2.WorkSnapshot, error) {
	result := d.applySettlement(ctx, snapshot, publication, action, receipt, diagnostic)
	if result.Err != nil {
		return result.Snapshot, result.Err
	}
	if action == statev2.PublicationActionConflict {
		return result.Snapshot, ErrReconcilePublicationConflict
	}
	return result.Snapshot, nil
}

func (c *Coordinator) result(snapshot statev2.WorkSnapshot) ReconcileResult {
	return ReconcileResult{WorkID: snapshot.WorkID, State: snapshot.State, NextAction: snapshot.NextAction, EvidenceRefs: append([]string(nil), snapshot.EvidenceRefs...)}
}
