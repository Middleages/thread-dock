package coordinator

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

type Dispatcher interface {
	SubmitPublication(context.Context, contractv2.WorkID, statev2.PublicationIntentID) <-chan CommandResult
	SubmitRuntime(context.Context, contractv2.WorkID, contractv2.TaskID, statev2.InvocationID) <-chan CommandResult
	Close(context.Context) error
}

type publicationCommand struct {
	ctx      context.Context
	intentID statev2.PublicationIntentID
	result   chan<- CommandResult
}

type publicationQueue struct {
	workID          contractv2.WorkID
	lease           OwnerLease
	ctx             context.Context
	cancel          context.CancelFunc
	items           chan publicationCommand
	runtimeMessages chan runtimeMessage
	done            chan struct{}
	submitMu        sync.Mutex
	submitWG        sync.WaitGroup
	runtimeWG       sync.WaitGroup
	releaseMu       sync.Mutex
	releaseErr      error
	stopped         bool
	owner           OwnerRecord
	beforeSelect    func()
	runtimeOps      map[runtimeKey]map[runtimeOperationKind]*runtimeOperation
}

type publicationDispatcher struct {
	state     State
	publisher Publisher
	locker    OwnerLocker
	ownerID   OwnerID
	pid       int
	startedAt time.Time
	runtime   Runtime
	inspector CandidateInspector

	mu     sync.Mutex
	closed bool
	queues map[contractv2.WorkID]*publicationQueue
}

// NewDispatcher creates a Work-scoped publication dispatcher. Owner metadata
// is stable for the process lifetime and each Work queue retains one lease.
func NewDispatcher(state State, publisher Publisher, locker OwnerLocker, ownerID OwnerID, pid int, startedAt time.Time) Dispatcher {
	return newDispatcher(state, publisher, nil, nil, locker, ownerID, pid, startedAt)
}

func newDispatcher(state State, publisher Publisher, runtime Runtime, inspector CandidateInspector, locker OwnerLocker, ownerID OwnerID, pid int, startedAt time.Time) *publicationDispatcher {
	if pid == 0 {
		pid = os.Getpid()
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return &publicationDispatcher{state: state, publisher: publisher, runtime: runtime, inspector: inspector, locker: locker, ownerID: ownerID, pid: pid, startedAt: startedAt, queues: make(map[contractv2.WorkID]*publicationQueue)}
}

func (d *publicationDispatcher) installQueueLocked(lease OwnerLease) (*publicationQueue, error) {
	if lease == nil {
		return nil, errors.New("owner lease is required")
	}
	record := lease.Record()
	if record.WorkID == "" {
		return nil, errors.New("owner lease work ID is required")
	}
	if existing := d.queues[record.WorkID]; existing != nil {
		return existing, nil
	}
	queueCtx, cancel := context.WithCancel(context.Background())
	q := &publicationQueue{workID: record.WorkID, lease: lease, owner: record, ctx: queueCtx, cancel: cancel, items: make(chan publicationCommand, 64), runtimeMessages: make(chan runtimeMessage, 64), done: make(chan struct{}), runtimeOps: make(map[runtimeKey]map[runtimeOperationKind]*runtimeOperation)}
	d.queues[record.WorkID] = q
	go d.runQueue(q)
	return q, nil
}

func (d *publicationDispatcher) SubmitPublication(ctx context.Context, workID contractv2.WorkID, intentID statev2.PublicationIntentID) <-chan CommandResult {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan CommandResult, 1)
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		result <- CommandResult{Err: ErrDispatcherClosed}
		return result
	}
	q, err := d.queueLocked(ctx, workID)
	if err != nil {
		d.mu.Unlock()
		result <- CommandResult{Err: err}
		return result
	}
	d.mu.Unlock()
	command := publicationCommand{ctx: ctx, intentID: intentID, result: result}
	q.submitMu.Lock()
	if q.stopped {
		q.submitMu.Unlock()
		result <- CommandResult{Err: ErrDispatcherClosed}
		return result
	}
	q.submitWG.Add(1)
	q.submitMu.Unlock()
	defer q.submitWG.Done()
	if q.beforeSelect != nil {
		q.beforeSelect()
	}
	select {
	case q.items <- command:
	case <-ctx.Done():
		result <- CommandResult{Err: ctx.Err()}
	case <-q.ctx.Done():
		result <- CommandResult{Err: ErrDispatcherClosed}
	}
	return result
}

func (d *publicationDispatcher) queueLocked(ctx context.Context, workID contractv2.WorkID) (*publicationQueue, error) {
	q := d.queues[workID]
	if q != nil {
		return q, nil
	}
	if d.locker == nil {
		return nil, errors.New("owner locker is required")
	}
	lease, err := d.locker.Acquire(ctx, workID, d.ownerID, d.pid, d.startedAt)
	if err != nil {
		return nil, err
	}
	q, err = d.installQueueLocked(lease)
	if err != nil {
		_ = lease.Release()
		return nil, err
	}
	return q, nil
}

func (d *publicationDispatcher) SubmitRuntime(ctx context.Context, workID contractv2.WorkID, taskID contractv2.TaskID, invocationID statev2.InvocationID) <-chan CommandResult {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan CommandResult, 1)
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		result <- CommandResult{Err: ErrDispatcherClosed}
		return result
	}
	if d.runtime == nil {
		d.mu.Unlock()
		result <- CommandResult{Err: errors.New("runtime is required")}
		return result
	}
	q, err := d.queueLocked(ctx, workID)
	if err != nil {
		d.mu.Unlock()
		result <- CommandResult{Err: err}
		return result
	}
	d.mu.Unlock()
	command := runtimeCommand{ctx: ctx, taskID: taskID, invocationID: invocationID, result: result}
	q.submitMu.Lock()
	if q.stopped {
		q.submitMu.Unlock()
		result <- CommandResult{Err: ErrDispatcherClosed}
		return result
	}
	q.submitWG.Add(1)
	q.submitMu.Unlock()
	defer q.submitWG.Done()
	select {
	case q.runtimeMessages <- runtimeMessage{command: &command}:
	case <-ctx.Done():
		result <- CommandResult{Err: ctx.Err()}
	case <-q.ctx.Done():
		result <- CommandResult{Err: ErrDispatcherClosed}
	}
	return result
}

func (d *publicationDispatcher) runQueue(q *publicationQueue) {
	defer close(q.done)
	defer func() {
		for {
			select {
			case command := <-q.items:
				command.result <- CommandResult{Err: ErrDispatcherClosed}
			case message := <-q.runtimeMessages:
				if message.command != nil {
					message.command.result <- CommandResult{Err: ErrDispatcherClosed}
				}
			default:
				return
			}
		}
	}()
	defer func() {
		q.releaseMu.Lock()
		q.releaseErr = q.lease.Release()
		q.releaseMu.Unlock()
	}()
	defer q.submitWG.Wait()
	defer q.runtimeWG.Wait()
	defer d.resolveRuntimeOperations(q)
	for {
		select {
		case <-q.ctx.Done():
			return
		default:
		}
		select {
		case <-q.ctx.Done():
			return
		case command := <-q.items:
			if command.ctx.Err() != nil {
				command.result <- CommandResult{Err: command.ctx.Err()}
				continue
			}
			commandCtx, cancel := context.WithCancel(q.ctx)
			go func() {
				select {
				case <-command.ctx.Done():
					cancel()
				case <-commandCtx.Done():
				}
			}()
			result := d.handlePublication(commandCtx, q, command.intentID)
			cancel()
			command.result <- result
		case message := <-q.runtimeMessages:
			if message.command != nil {
				d.handleRuntimeCommand(q, *message.command)
			} else if message.event != nil {
				d.handleRuntimeEvent(q, *message.event)
			}
		}
	}
}

func (d *publicationDispatcher) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	if d.closed {
		queues := make([]*publicationQueue, 0, len(d.queues))
		for _, q := range d.queues {
			queues = append(queues, q)
		}
		d.mu.Unlock()
		return waitPublicationQueues(ctx, queues)
	}
	d.closed = true
	queues := make([]*publicationQueue, 0, len(d.queues))
	for _, q := range d.queues {
		queues = append(queues, q)
		q.submitMu.Lock()
		q.stopped = true
		q.cancel()
		q.submitMu.Unlock()
	}
	d.mu.Unlock()
	return waitPublicationQueues(ctx, queues)
}

func waitPublicationQueues(ctx context.Context, queues []*publicationQueue) error {
	for _, q := range queues {
		registeredDone := make(chan struct{})
		go func(q *publicationQueue) {
			q.submitWG.Wait()
			close(registeredDone)
		}(q)
		select {
		case <-registeredDone:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-q.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		q.releaseMu.Lock()
		releaseErr := q.releaseErr
		q.releaseMu.Unlock()
		if releaseErr != nil {
			return releaseErr
		}
	}
	return nil
}
