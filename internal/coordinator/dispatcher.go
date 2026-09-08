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
	Close(context.Context) error
}

type publicationCommand struct {
	ctx      context.Context
	intentID statev2.PublicationIntentID
	result   chan<- CommandResult
}

type publicationQueue struct {
	workID     contractv2.WorkID
	lease      OwnerLease
	ctx        context.Context
	cancel     context.CancelFunc
	items      chan publicationCommand
	done       chan struct{}
	submitMu   sync.Mutex
	submitWG   sync.WaitGroup
	releaseMu  sync.Mutex
	releaseErr error
	stopped    bool
	owner      OwnerRecord
}

type publicationDispatcher struct {
	state     State
	publisher Publisher
	locker    OwnerLocker
	ownerID   OwnerID
	pid       int
	startedAt time.Time

	mu     sync.Mutex
	closed bool
	queues map[contractv2.WorkID]*publicationQueue
}

// NewDispatcher creates a Work-scoped publication dispatcher. Owner metadata
// is stable for the process lifetime and each Work queue retains one lease.
func NewDispatcher(state State, publisher Publisher, locker OwnerLocker, ownerID OwnerID, pid int, startedAt time.Time) Dispatcher {
	if pid == 0 {
		pid = os.Getpid()
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return &publicationDispatcher{state: state, publisher: publisher, locker: locker, ownerID: ownerID, pid: pid, startedAt: startedAt, queues: make(map[contractv2.WorkID]*publicationQueue)}
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
	q := d.queues[workID]
	if q == nil {
		if d.locker == nil {
			d.mu.Unlock()
			result <- CommandResult{Err: errors.New("owner locker is required")}
			return result
		}
		lease, err := d.locker.Acquire(ctx, workID, d.ownerID, d.pid, d.startedAt)
		if err != nil {
			d.mu.Unlock()
			result <- CommandResult{Err: err}
			return result
		}
		queueCtx, cancel := context.WithCancel(context.Background())
		q = &publicationQueue{workID: workID, lease: lease, owner: lease.Record(), ctx: queueCtx, cancel: cancel, items: make(chan publicationCommand, 64), done: make(chan struct{})}
		d.queues[workID] = q
		go d.runQueue(q)
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
	select {
	case q.items <- command:
	case <-ctx.Done():
		result <- CommandResult{Err: ctx.Err()}
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
