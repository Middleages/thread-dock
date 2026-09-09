package coordinator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	contractv2 "thread-dock/internal/contract/v2"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
)

type RuntimeObservation struct {
	State            string
	ProviderIdentity string
	EndedAt          *time.Time
	Diagnostic       string
	Artifact         *runtimecontract.ArtifactEnvelope
}

type Runtime interface {
	Observe(context.Context, statev2.InvocationState, runtimecontract.Invocation) (RuntimeObservation, error)
	Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity, runtimecontract.Invocation) (string, error)
	// Terminate returns nil only after the provider has confirmed that this
	// exact invocation ended. An adapter that only sends a signal or request
	// must continue observing and return a non-nil error until confirmation.
	Terminate(context.Context, statev2.InvocationState) error
}

type RuntimeResult struct {
	InvocationID statev2.InvocationID
	// Artifact is reserved for a future runtime-adapter/candidate-ingestion
	// slice; the lifecycle port currently cannot produce an artifact.
	Artifact json.RawMessage
	Err      error
}

const (
	RuntimeObservationNotStarted = "not_started"
	RuntimeObservationActive     = "active"
	RuntimeObservationEnded      = "ended"
	RuntimeObservationUnknown    = "unknown"
)

var (
	ErrRuntimePaused  = errors.New("runtime dispatch is paused")
	ErrRuntimeBlocked = errors.New("runtime dispatch is blocked")
	ErrRuntimeStale   = errors.New("runtime invocation is stale")
)

const runtimeObservationTimeout = 30 * time.Second

type runtimeCommand struct {
	ctx          context.Context
	taskID       contractv2.TaskID
	invocationID statev2.InvocationID
	result       chan<- CommandResult
}

type runtimeMessage struct {
	command *runtimeCommand
	event   *runtimeEvent
}

type runtimeKey struct {
	taskID       contractv2.TaskID
	invocationID statev2.InvocationID
}

type runtimeOperationKind string

const (
	runtimeLaunch    runtimeOperationKind = "launch"
	runtimeObserve   runtimeOperationKind = "observe"
	runtimeTerminate runtimeOperationKind = "terminate"
)

type runtimeOperation struct {
	key     runtimeKey
	kind    runtimeOperationKind
	waiters []runtimeWaiter
	ctx     context.Context
	cancel  context.CancelFunc
}

type runtimeWaiter struct {
	result chan<- CommandResult
	stop   func() bool
}

type runtimeEvent struct {
	key         runtimeKey
	operation   runtimeOperationKind
	result      RuntimeResult
	observation RuntimeObservation
	identity    string
}

func (d *publicationDispatcher) handleRuntimeCommand(q *publicationQueue, command runtimeCommand) {
	key := runtimeKey{taskID: command.taskID, invocationID: command.invocationID}
	if command.ctx.Err() != nil {
		if ops := q.runtimeOps[key]; len(ops) == 1 {
			for _, existing := range ops {
				existing.waiters = append(existing.waiters, d.runtimeWaiter(q, existing, command))
				return
			}
		}
		command.result <- CommandResult{Err: command.ctx.Err()}
		return
	}
	snapshot, err := d.state.Load(q.ctx, q.workID)
	if err != nil {
		command.result <- CommandResult{Err: err}
		return
	}
	task, invocation, err := d.validateRuntimeIdentity(snapshot, q, key)
	if err != nil {
		command.result <- CommandResult{Snapshot: snapshot, Err: err}
		return
	}
	if snapshot.Control.Blocker != nil {
		command.result <- CommandResult{Snapshot: snapshot, Err: ErrRuntimeBlocked}
		return
	}
	var kind runtimeOperationKind
	switch task.Status {
	case statev2.TaskInvocationReserved:
		if !invocation.LaunchRequested {
			if snapshot.Control.PauseRequested {
				command.result <- CommandResult{Snapshot: snapshot, Err: ErrRuntimePaused}
				return
			}
			_, applyErr := d.applyRuntimeTask(q.ctx, snapshot, task, invocation, statev2.TaskBeginLaunch, "")
			if applyErr != nil {
				command.result <- CommandResult{Snapshot: snapshot, Err: applyErr}
				return
			}
			snapshot, err = d.state.Load(q.ctx, q.workID)
			if err != nil {
				command.result <- CommandResult{Err: err}
				return
			}
			task, invocation, err = d.validateRuntimeIdentity(snapshot, q, key)
			if err != nil {
				command.result <- CommandResult{Snapshot: snapshot, Err: err}
				return
			}
			if snapshot.Control.PauseRequested {
				command.result <- CommandResult{Snapshot: snapshot, Err: ErrRuntimePaused}
				return
			}
			kind = runtimeLaunch
		} else {
			kind = runtimeObserve
		}
	case statev2.TaskRunning:
		if snapshot.Control.PauseRequested {
			if _, applyErr := d.applyRuntimeTask(q.ctx, snapshot, task, invocation, statev2.TaskRequestTermination, "pause requested"); applyErr != nil {
				command.result <- CommandResult{Snapshot: snapshot, Err: applyErr}
				return
			}
			snapshot, err = d.state.Load(q.ctx, q.workID)
			if err != nil {
				command.result <- CommandResult{Err: err}
				return
			}
			task, invocation, err = d.validateRuntimeIdentity(snapshot, q, key)
			if err != nil {
				command.result <- CommandResult{Snapshot: snapshot, Err: err}
				return
			}
			kind = runtimeTerminate
		} else {
			kind = runtimeObserve
		}
	case statev2.TaskTerminationPending:
		kind = runtimeTerminate
	case statev2.TaskTerminated:
		if task.Candidate != nil || invocation.InvocationID == "" || d.inspector == nil {
			command.result <- CommandResult{Snapshot: snapshot}
			return
		}
		// A confirmed Builder may have been persisted immediately before the
		// process crashed while ingesting its artifact. Re-observe only that
		// narrow case; never relaunch or terminate it again.
		if invocation.Role != "builder" || !invocation.TerminationConfirmed || invocation.EndedAt == nil {
			command.result <- CommandResult{Snapshot: snapshot}
			return
		}
		kind = runtimeObserve
	case statev2.TaskNeedsOperator:
		command.result <- CommandResult{Snapshot: snapshot, Err: ErrRuntimeStale}
		return
	default:
		command.result <- CommandResult{Snapshot: snapshot, Err: ErrRuntimeStale}
		return
	}
	if existing := q.runtimeOps[key][kind]; existing != nil {
		existing.waiters = append(existing.waiters, d.runtimeWaiter(q, existing, command))
		return
	}
	op := &runtimeOperation{key: key, kind: kind}
	op.ctx, op.cancel = context.WithCancel(q.ctx)
	op.waiters = append(op.waiters, d.runtimeWaiter(q, op, command))
	if q.runtimeOps[key] == nil {
		q.runtimeOps[key] = make(map[runtimeOperationKind]*runtimeOperation)
	}
	q.runtimeOps[key][kind] = op
	q.runtimeWG.Add(1)
	go d.runRuntimeWorker(q, op)
}

func (d *publicationDispatcher) runRuntimeWorker(q *publicationQueue, op *runtimeOperation) {
	defer q.runtimeWG.Done()
	if op.cancel != nil {
		defer op.cancel()
	}
	event := runtimeEvent{key: op.key, operation: op.kind, result: RuntimeResult{InvocationID: op.key.invocationID}}
	ctx := op.ctx
	snapshot, err := d.state.Load(ctx, q.workID)
	if err != nil {
		event.result.Err = err
		d.sendRuntimeEvent(q, event)
		return
	}
	task, invocation, err := d.validateRuntimeIdentity(snapshot, q, op.key)
	if err != nil {
		event.result.Err = err
		d.sendRuntimeEvent(q, event)
		return
	}
	if snapshot.Control.Blocker != nil {
		event.result.Err = ErrRuntimeBlocked
		d.sendRuntimeEvent(q, event)
		return
	}
	if op.kind == runtimeLaunch && (snapshot.Control.PauseRequested || task.Status != statev2.TaskInvocationReserved || !invocation.LaunchRequested) {
		event.result.Err = ErrRuntimePaused
		d.sendRuntimeEvent(q, event)
		return
	}
	if op.kind == runtimeTerminate && task.Status != statev2.TaskTerminationPending && task.Status != statev2.TaskRunning {
		event.result.Err = ErrRuntimeStale
		d.sendRuntimeEvent(q, event)
		return
	}
	// State implementations normally return decoded snapshots, but keep the
	// provider boundary independent of any pointer-backed snapshot it returns.
	invocationCopy := *invocation
	worktreeCopy := *task.Worktree
	if task.Worktree.IntegratedDependencies != nil {
		worktreeCopy.IntegratedDependencies = make(map[contractv2.TaskID]string, len(task.Worktree.IntegratedDependencies))
		for id, head := range task.Worktree.IntegratedDependencies {
			worktreeCopy.IntegratedDependencies[id] = head
		}
	}
	runtimeInvocation, invocationErr := buildRuntimeInvocation(snapshot, task.TaskID, invocationCopy, worktreeCopy)
	if invocationErr != nil {
		event.result.Err = invocationErr
		d.sendRuntimeEvent(q, event)
		return
	}
	switch op.kind {
	case runtimeLaunch:
		event.identity, event.result.Err = d.runtime.Launch(ctx, invocationCopy, worktreeCopy, runtimeInvocation)
	case runtimeObserve:
		observeCtx, cancel := context.WithTimeout(ctx, runtimeObservationTimeout)
		event.observation, event.result.Err = d.runtime.Observe(observeCtx, invocationCopy, runtimeInvocation)
		cancel()
	case runtimeTerminate:
		event.result.Err = d.runtime.Terminate(ctx, invocationCopy)
	}
	d.sendRuntimeEvent(q, event)
}

func (d *publicationDispatcher) runtimeWaiter(_ *publicationQueue, op *runtimeOperation, command runtimeCommand) runtimeWaiter {
	return runtimeWaiter{result: command.result, stop: context.AfterFunc(command.ctx, op.cancel)}
}

func (d *publicationDispatcher) sendRuntimeEvent(q *publicationQueue, event runtimeEvent) {
	select {
	case q.runtimeMessages <- runtimeMessage{event: &event}:
	case <-q.ctx.Done():
	}
}

func (d *publicationDispatcher) handleRuntimeEvent(q *publicationQueue, event runtimeEvent) {
	ops := q.runtimeOps[event.key]
	op := ops[event.operation]
	if op == nil {
		return
	}
	delete(ops, event.operation)
	if len(ops) == 0 {
		delete(q.runtimeOps, event.key)
	}
	if q.ctx.Err() != nil {
		d.resolveRuntimeWaiters(op, CommandResult{Err: ErrDispatcherClosed})
		return
	}
	if event.result.InvocationID != event.key.invocationID {
		d.resolveRuntimeWaiters(op, CommandResult{Err: ErrRuntimeStale})
		return
	}
	snapshot, loadErr := d.state.Load(q.ctx, q.workID)
	if loadErr != nil {
		d.resolveRuntimeWaiters(op, CommandResult{Err: loadErr})
		return
	}
	result := CommandResult{Snapshot: snapshot, Err: event.result.Err}
	if event.result.Err == nil {
		result = d.settleRuntimeEvent(q, snapshot, event)
	} else if event.operation == runtimeObserve && (errors.Is(event.result.Err, context.Canceled) || errors.Is(event.result.Err, context.DeadlineExceeded)) {
		task := snapshot.TaskStates[event.key.taskID]
		if task.Status == statev2.TaskInvocationReserved {
			result = d.runtimeUnknown(q, snapshot, event.key, "runtime observation was canceled before identity was established")
		} else {
			result = d.requestRuntimeTermination(q, snapshot, event.key, "runtime observation ended", op.waiters...)
			if result.Err == nil {
				// The termination operation now owns these completion channels.
				op.waiters = nil
				return
			}
		}
	} else if event.operation == runtimeLaunch && (errors.Is(event.result.Err, context.Canceled) || errors.Is(event.result.Err, context.DeadlineExceeded)) {
		result = d.runtimeUnknown(q, snapshot, event.key, "runtime launch was canceled after durable launch request")
	}
	if result.Err == nil && event.operation != runtimeTerminate && runtimeTerminationPending(result.Snapshot, event.key.taskID) {
		if terminate := q.runtimeOps[event.key][runtimeTerminate]; terminate != nil {
			terminate.waiters = append(terminate.waiters, op.waiters...)
			op.waiters = nil
			return
		}
	}
	d.resolveRuntimeWaiters(op, result)
}

func runtimeTerminationPending(snapshot statev2.WorkSnapshot, taskID contractv2.TaskID) bool {
	task, ok := snapshot.TaskStates[taskID]
	return ok && task.Status == statev2.TaskTerminationPending
}

func (d *publicationDispatcher) settleRuntimeEvent(q *publicationQueue, snapshot statev2.WorkSnapshot, event runtimeEvent) CommandResult {
	task, invocation, err := d.validateRuntimeIdentity(snapshot, q, event.key)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	if task.Status == statev2.TaskTerminated && invocation.TerminationConfirmed && invocation.EndedAt != nil {
		if invocation.Role != "builder" || task.Candidate != nil || event.operation != runtimeObserve {
			return CommandResult{Snapshot: snapshot}
		}
		if event.observation.State != RuntimeObservationEnded {
			return d.runtimeUnknown(q, snapshot, event.key, "confirmed terminated Builder was observed active")
		}
		if strings.TrimSpace(event.observation.ProviderIdentity) == "" || event.observation.ProviderIdentity != invocation.ProviderIdentity {
			return d.runtimeUnknown(q, snapshot, event.key, "runtime ended identity does not match persisted identity")
		}
		if event.observation.Artifact == nil {
			return CommandResult{Snapshot: snapshot}
		}
		result, _ := d.ingestBuilderArtifact(q.ctx, snapshot, event.key.taskID, event.observation.Artifact)
		return result
	}
	endedAt, endedAtErr := runtimeEndedAt(event.observation.EndedAt)
	if endedAtErr != nil {
		return d.runtimeUnknown(q, snapshot, event.key, endedAtErr.Error())
	}
	switch event.operation {
	case runtimeLaunch:
		if strings.TrimSpace(event.identity) == "" {
			return d.runtimeUnknown(q, snapshot, event.key, "runtime launch identity is unknown")
		}
		result := d.applyRuntimeAndReload(q.ctx, snapshot, task, invocation, statev2.TaskMarkRunning, event.identity)
		if result.Err == nil && result.Snapshot.Control.PauseRequested {
			return d.requestRuntimeTermination(q, result.Snapshot, event.key, "pause requested")
		}
		return result
	case runtimeObserve:
		switch event.observation.State {
		case RuntimeObservationActive:
			if event.observation.Artifact != nil {
				blocked, _ := d.blockBuilderArtifact(q.ctx, snapshot, task, invocation, errors.New("active runtime returned an artifact before termination"))
				return blocked
			}
			if task.Status != statev2.TaskInvocationReserved && (strings.TrimSpace(event.observation.ProviderIdentity) == "" || event.observation.ProviderIdentity != invocation.ProviderIdentity) {
				return d.runtimeUnknown(q, snapshot, event.key, "runtime active identity does not match persisted identity")
			}
			if task.Status == statev2.TaskInvocationReserved && strings.TrimSpace(event.observation.ProviderIdentity) == "" {
				return d.runtimeUnknown(q, snapshot, event.key, "runtime active identity is unknown")
			}
			if task.Status == statev2.TaskInvocationReserved {
				identity := event.observation.ProviderIdentity
				if identity == "" {
					identity = invocation.ProviderIdentity
				}
				result := d.applyRuntimeAndReload(q.ctx, snapshot, task, invocation, statev2.TaskMarkRunning, identity)
				if result.Err == nil && result.Snapshot.Control.PauseRequested {
					return d.requestRuntimeTermination(q, result.Snapshot, event.key, "pause requested")
				}
				return result
			}
			if snapshot.Control.PauseRequested {
				return d.requestRuntimeTermination(q, snapshot, event.key, "pause requested")
			}
			return CommandResult{Snapshot: snapshot}
		case RuntimeObservationEnded:
			if task.Status == statev2.TaskInvocationReserved {
				if strings.TrimSpace(event.observation.ProviderIdentity) == "" {
					return d.runtimeUnknown(q, snapshot, event.key, "runtime ended without identity")
				}
				result := d.applyRuntimeAndReload(q.ctx, snapshot, task, invocation, statev2.TaskMarkRunning, event.observation.ProviderIdentity)
				if result.Err != nil {
					return result
				}
				snapshot = result.Snapshot
				task, invocation, err = d.validateRuntimeIdentity(snapshot, q, event.key)
				if err != nil {
					return CommandResult{Snapshot: snapshot, Err: err}
				}
			} else if strings.TrimSpace(event.observation.ProviderIdentity) == "" || event.observation.ProviderIdentity != invocation.ProviderIdentity {
				return d.runtimeUnknown(q, snapshot, event.key, "runtime ended identity does not match persisted identity")
			}
			settled := d.applyRuntimeAndReloadAt(q.ctx, snapshot, task, invocation, statev2.TaskConfirmTermination, event.observation.Diagnostic, endedAt)
			if settled.Err != nil || event.observation.Artifact == nil {
				return settled
			}
			ingested, _ := d.ingestBuilderArtifact(q.ctx, settled.Snapshot, event.key.taskID, event.observation.Artifact)
			return ingested
		case RuntimeObservationNotStarted, RuntimeObservationUnknown:
			return d.runtimeUnknown(q, snapshot, event.key, runtimeDiagnostic(event.observation))
		default:
			return d.runtimeUnknown(q, snapshot, event.key, "runtime returned an unsupported observation")
		}
	case runtimeTerminate:
		return d.applyRuntimeAndReload(q.ctx, snapshot, task, invocation, statev2.TaskConfirmTermination, "runtime terminated")
	default:
		return CommandResult{Snapshot: snapshot, Err: ErrRuntimeStale}
	}
}

func (d *publicationDispatcher) requestRuntimeTermination(q *publicationQueue, snapshot statev2.WorkSnapshot, key runtimeKey, reason string, waiters ...runtimeWaiter) CommandResult {
	task, invocation, err := d.validateRuntimeIdentity(snapshot, q, key)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	if task.Status == statev2.TaskRunning {
		result := d.applyRuntimeAndReload(q.ctx, snapshot, task, invocation, statev2.TaskRequestTermination, reason)
		if result.Err != nil {
			return result
		}
		snapshot = result.Snapshot
		task, invocation, err = d.validateRuntimeIdentity(snapshot, q, key)
		if err != nil {
			return CommandResult{Snapshot: snapshot, Err: err}
		}
	}
	if task.Status == statev2.TaskTerminationPending {
		if q.runtimeOps[key] == nil {
			q.runtimeOps[key] = make(map[runtimeOperationKind]*runtimeOperation)
		}
		if existing := q.runtimeOps[key][runtimeTerminate]; existing != nil {
			existing.waiters = append(existing.waiters, waiters...)
		} else {
			op := &runtimeOperation{key: key, kind: runtimeTerminate, waiters: append([]runtimeWaiter(nil), waiters...)}
			op.ctx, op.cancel = context.WithCancel(q.ctx)
			q.runtimeOps[key][runtimeTerminate] = op
			q.runtimeWG.Add(1)
			go d.runRuntimeWorker(q, op)
		}
	}
	return CommandResult{Snapshot: snapshot}
}

func (d *publicationDispatcher) runtimeUnknown(q *publicationQueue, snapshot statev2.WorkSnapshot, key runtimeKey, diagnostic string) CommandResult {
	if len([]byte(diagnostic)) > statev2.MaxDiagnosticBytes {
		diagnostic = string([]byte(diagnostic)[:statev2.MaxDiagnosticBytes])
		for !utf8.ValidString(diagnostic) {
			diagnostic = diagnostic[:len(diagnostic)-1]
		}
	}
	if snapshot.Control.Blocker != nil {
		return CommandResult{Snapshot: snapshot, Err: ErrRuntimeStale}
	}
	task, invocation, err := d.validateRuntimeIdentity(snapshot, q, key)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	result := d.applyRuntimeAndReload(q.ctx, snapshot, task, invocation, statev2.TaskNeedsOperatorAction, diagnostic)
	if result.Err == nil {
		result.Err = errors.New("runtime state is unknown; operator action required")
	}
	return result
}

func runtimeDiagnostic(observation RuntimeObservation) string {
	if strings.TrimSpace(observation.Diagnostic) != "" {
		return observation.Diagnostic
	}
	return "runtime state is unknown; operator inspection required"
}

// buildRuntimeInvocation derives the provider payload from the durable
// contract and invocation identity. It deliberately does not consult agent
// output or mutable provider state.
func buildRuntimeInvocation(snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, invocation statev2.InvocationState, worktree statev2.WorktreeIdentity) (runtimecontract.Invocation, error) {
	requestID := contractv2.RequestID(invocation.InvocationID)
	result := runtimecontract.Invocation{
		RequestID: requestID,
		Role:      runtimecontract.Role(invocation.Role),
		ProfileID: invocation.LogicalProfile,
		Worktree:  worktree.CanonicalPath,
		ReadOnly:  invocation.Role != string(runtimecontract.RoleBuilder),
	}
	if invocation.Role != string(runtimecontract.RoleBuilder) {
		return result, nil
	}
	var task contractv2.Task
	for _, candidate := range snapshot.Contract.Tasks {
		if candidate.TaskID == taskID {
			task = candidate
			break
		}
	}
	if task.TaskID == "" {
		// A few lifecycle-only callers use a provider-neutral snapshot without
		// the contract projection. Preserve that legacy path; durable v2 work
		// snapshots always contain the task and therefore still fail closed.
		if len(snapshot.Contract.Tasks) != 0 {
			return runtimecontract.Invocation{}, ErrRuntimeStale
		}
		task.TaskID = taskID
	}
	packet, err := json.Marshal(runtimecontract.BuilderPacket{
		TaskID:             task.TaskID,
		AllowedPaths:       append([]string(nil), task.AllowedPaths...),
		AcceptanceCriteria: append([]string(nil), task.AcceptanceCriteria...),
		Verification:       append([]contractv2.CommandSpec(nil), task.Verification...),
	})
	if err != nil {
		return runtimecontract.Invocation{}, fmt.Errorf("marshal builder packet: %w", err)
	}
	result.OutputSchema = runtimecontract.BuilderOutputSchema
	result.Packet = packet
	return result, nil
}

func (d *publicationDispatcher) validateRuntimeIdentity(snapshot statev2.WorkSnapshot, q *publicationQueue, key runtimeKey) (statev2.TaskExecutionState, *statev2.InvocationState, error) {
	if snapshot.WorkID != q.workID || (q.lease != nil && !ownerRecordMatches(d, q)) {
		return statev2.TaskExecutionState{}, nil, ErrRuntimeStale
	}
	task, ok := snapshot.TaskStates[key.taskID]
	if !ok || task.Invocation == nil || task.Invocation.InvocationID != key.invocationID {
		return statev2.TaskExecutionState{}, nil, ErrRuntimeStale
	}
	invocation := task.Invocation
	if task.LogicalWork == nil || task.Worktree == nil || task.LogicalWork.Worktree == nil || invocation.LogicalWorkID != task.LogicalWork.LogicalWorkID || invocation.Role != task.LogicalWork.Role || invocation.LogicalProfile != task.LogicalWork.LogicalProfile || invocation.RuntimeFingerprint != task.LogicalWork.RuntimeFingerprint || task.BuilderAttempt != task.LogicalWork.BuilderAttempt || !reflect.DeepEqual(task.Worktree, task.LogicalWork.Worktree) {
		return statev2.TaskExecutionState{}, nil, ErrRuntimeStale
	}
	return task, invocation, nil
}

func hasRuntimeProvider(invocation *statev2.InvocationState) bool {
	return invocation != nil && (invocation.ProviderIdentity != "" || invocation.ProviderSession != "" || invocation.ProviderPane != "" || invocation.ProviderProcess != "")
}

func (d *publicationDispatcher) applyRuntimeTask(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState, action statev2.TaskAction, identity string) (statev2.WorkSnapshot, error) {
	return d.applyRuntimeTaskAt(ctx, snapshot, task, invocation, action, identity, nil)
}

func (d *publicationDispatcher) applyRuntimeTaskAt(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState, action statev2.TaskAction, identity string, at *time.Time) (statev2.WorkSnapshot, error) {
	transition := runtimeTransitionAt(task, invocation, action, identity, at)
	if action == statev2.TaskNeedsOperatorAction {
		transition.Blocker = &statev2.OperatorBlocker{Kind: statev2.BlockerKindRuntimeUnknown, OperatorRef: string(d.ownerID), TaskID: task.TaskID, InvocationID: invocation.InvocationID, Diagnostic: identity}
	}
	request, err := d.runtimeRequest(snapshot, transition)
	if err != nil {
		return snapshot, err
	}
	return d.state.Apply(ctx, request)
}

func (d *publicationDispatcher) applyRuntimeAndReload(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState, action statev2.TaskAction, identity string) CommandResult {
	return d.applyRuntimeAndReloadAt(ctx, snapshot, task, invocation, action, identity, nil)
}

func (d *publicationDispatcher) applyRuntimeAndReloadAt(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState, action statev2.TaskAction, identity string, at *time.Time) CommandResult {
	updated, err := d.applyRuntimeTaskAt(ctx, snapshot, task, invocation, action, identity, at)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	latest, loadErr := d.state.Load(ctx, snapshot.WorkID)
	if loadErr == nil {
		updated = latest
	}
	return CommandResult{Snapshot: updated, Err: loadErr}
}

func runtimeTransition(task statev2.TaskExecutionState, invocation *statev2.InvocationState, action statev2.TaskAction, identity string) statev2.TaskTransition {
	return runtimeTransitionAt(task, invocation, action, identity, nil)
}

func runtimeTransitionAt(task statev2.TaskExecutionState, invocation *statev2.InvocationState, action statev2.TaskAction, identity string, at *time.Time) statev2.TaskTransition {
	transitionAt := time.Now().UTC()
	if at != nil {
		transitionAt = *at
	}
	transition := statev2.TaskTransition{TaskID: task.TaskID, Action: action, InvocationID: invocation.InvocationID, LogicalWorkID: invocation.LogicalWorkID, Role: invocation.Role, ReturnStage: invocation.ReturnStage, BuilderAttempt: task.BuilderAttempt, At: transitionAt}
	if task.Worktree != nil {
		worktree := *task.Worktree
		transition.Worktree = &worktree
	}
	if action == statev2.TaskMarkRunning {
		transition.Invocation = &statev2.InvocationState{ProviderIdentity: identity}
	} else if action == statev2.TaskRequestTermination || action == statev2.TaskConfirmTermination {
		reason := strings.TrimSpace(identity)
		if reason == "" {
			reason = "runtime terminated"
		}
		if len([]byte(reason)) > statev2.MaxDiagnosticBytes {
			reason = string([]byte(reason)[:statev2.MaxDiagnosticBytes])
			for !utf8.ValidString(reason) {
				reason = reason[:len(reason)-1]
			}
		}
		transition.Reason = reason
	}
	return transition
}

func runtimeEndedAt(at *time.Time) (*time.Time, error) {
	if at == nil {
		return nil, nil
	}
	if at.IsZero() || at.Location() != time.UTC {
		return nil, errors.New("runtime ended timestamp is invalid")
	}
	copy := *at
	return &copy, nil
}

func (d *publicationDispatcher) runtimeRequest(snapshot statev2.WorkSnapshot, transition statev2.TaskTransition) (statev2.TransitionRequest, error) {
	request := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, Task: &transition}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return statev2.TransitionRequest{}, fmt.Errorf("generate runtime request ID: %w", err)
	}
	request.RequestID = contractv2.RequestID("runtime-dispatch-" + hex.EncodeToString(bytes))
	hash, err := statev2.TransitionPayloadHash(request)
	if err != nil {
		return statev2.TransitionRequest{}, err
	}
	request.PayloadHash = hash
	return request, nil
}

func (d *publicationDispatcher) reconcileRuntimeNotStarted(ctx context.Context, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, invocationID statev2.InvocationID, evidence statev2.ResolutionEvidence) CommandResult {
	key := runtimeKey{taskID: taskID, invocationID: invocationID}
	if !evidence.OwnerTerminated || evidence.LaunchRequested || !evidence.ProviderAbsent {
		return CommandResult{Snapshot: snapshot, Err: errors.New("positive no-launch proof is required")}
	}
	task, invocation, err := d.validateRuntimeIdentity(snapshot, &publicationQueue{workID: snapshot.WorkID, owner: OwnerRecord{WorkID: snapshot.WorkID, OwnerID: d.ownerID, PID: d.pid, StartedAt: d.startedAt}, lease: nil}, key)
	if err != nil || task.Status != statev2.TaskInvocationReserved || invocation.LaunchRequested {
		return CommandResult{Snapshot: snapshot, Err: ErrRuntimeStale}
	}
	transition := runtimeTransition(task, invocation, statev2.TaskReconcileNotStarted, "")
	transition.Resolution = &evidence
	request, err := d.runtimeRequest(snapshot, transition)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}
	}
	updated, err := d.state.Apply(ctx, request)
	return CommandResult{Snapshot: updated, Err: err}
}

func (d *publicationDispatcher) resolveRuntimeWaiters(op *runtimeOperation, result CommandResult) {
	for _, waiter := range op.waiters {
		if waiter.stop != nil {
			waiter.stop()
		}
		waiter.result <- result
	}
}

func (d *publicationDispatcher) resolveRuntimeOperations(q *publicationQueue) {
	for _, ops := range q.runtimeOps {
		for _, op := range ops {
			d.resolveRuntimeWaiters(op, CommandResult{Err: ErrDispatcherClosed})
		}
	}
	q.runtimeOps = make(map[runtimeKey]map[runtimeOperationKind]*runtimeOperation)
}
