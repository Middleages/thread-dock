package coordinator

import (
	"context"
	"sync"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

type fakeState struct {
	mu       sync.Mutex
	snapshot statev2.WorkSnapshot
	applies  int
}

func newFakeState(workID contractv2.WorkID, intentID statev2.PublicationIntentID, status statev2.PublicationStatus) *fakeState {
	return &fakeState{snapshot: statev2.WorkSnapshot{WorkID: workID, Revision: 1, State: statev2.StatePublicationPending, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{intentID: {
		IntentID: intentID, Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, Status: status,
		PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PayloadRef: "artifact://one",
		Target: statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: true, Attempts: 1,
	}}}}
}

func (s *fakeState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot, nil
}

func (s *fakeState) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applies++
	p := s.snapshot.Publications[req.Publication.IntentID]
	switch req.Publication.Action {
	case statev2.PublicationComplete:
		p.Status = statev2.PublicationCompleted
		p.Receipt = req.Publication.Receipt
	case statev2.PublicationFail:
		p.Status = statev2.PublicationFailed
	case statev2.PublicationActionConflict:
		p.Status = statev2.PublicationConflict
	}
	s.snapshot.Publications[p.IntentID] = p
	s.snapshot.Revision++
	return s.snapshot, nil
}

type fakeOwnerLocker struct {
	mu       sync.Mutex
	leases   int
	releases int
}

func (l *fakeOwnerLocker) Acquire(_ context.Context, workID contractv2.WorkID, ownerID OwnerID, pid int, startedAt time.Time) (OwnerLease, error) {
	l.mu.Lock()
	l.leases++
	l.mu.Unlock()
	return &fakeOwnerLease{record: OwnerRecord{WorkID: workID, OwnerID: ownerID, PID: pid, StartedAt: startedAt}, locker: l}, nil
}

type fakeOwnerLease struct {
	record OwnerRecord
	locker *fakeOwnerLocker
}

func (l *fakeOwnerLease) Record() OwnerRecord { return l.record }
func (l *fakeOwnerLease) Release() error {
	l.locker.mu.Lock()
	l.locker.releases++
	l.locker.mu.Unlock()
	return nil
}

type blockingPublisher struct {
	mu        sync.Mutex
	allow     chan struct{}
	observes  int
	publishes int
	sawApply  bool
}

func (p *blockingPublisher) Observe(context.Context, statev2.PublicationState) (PublicationObservation, error) {
	p.mu.Lock()
	p.observes++
	p.mu.Unlock()
	<-p.allow
	return PublicationObservation{State: PublicationObservationAbsent}, nil
}
func (p *blockingPublisher) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	p.mu.Lock()
	p.publishes++
	p.sawApply = true
	p.mu.Unlock()
	return statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Now().UTC()}, nil
}
