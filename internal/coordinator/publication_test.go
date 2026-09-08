package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

func TestValidReceiptMirrorsStateContract(t *testing.T) {
	at := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	valid := &statev2.PublicationReceipt{NodeID: "node-1", URL: "https://example.test/1", Base: "main", Head: "abc", PublishedAt: at}
	if !validReceipt(valid) {
		t.Fatal("valid receipt rejected")
	}
	for name, mutate := range map[string]func(*statev2.PublicationReceipt){
		"node whitespace": func(r *statev2.PublicationReceipt) { r.NodeID = " node-1" },
		"url whitespace":  func(r *statev2.PublicationReceipt) { r.URL = "https://example.test/1 " },
		"base whitespace": func(r *statev2.PublicationReceipt) { r.Base = " main" },
		"head whitespace": func(r *statev2.PublicationReceipt) { r.Head = "abc\t" },
		"non utf8":        func(r *statev2.PublicationReceipt) { r.NodeID = string([]byte{0xff}) },
	} {
		t.Run(name, func(t *testing.T) {
			got := *valid
			mutate(&got)
			if validReceipt(&got) {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
}

func TestPublicationRequestIDsAreUniqueAcrossDispatchers(t *testing.T) {
	snapshot := statev2.WorkSnapshot{WorkID: "work-1", Revision: 1}
	d1 := &publicationDispatcher{}
	d2 := &publicationDispatcher{}
	first, err := d1.publicationRequest(snapshot, statev2.PublicationTransition{Action: statev2.PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := d2.publicationRequest(snapshot, statev2.PublicationTransition{Action: statev2.PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestID == second.RequestID {
		t.Fatalf("request IDs collided: %q", first.RequestID)
	}
}

func TestOwnerRecordMustMatchBeforePublisherIO(t *testing.T) {
	workID := contractv2.WorkID("work-1")
	intentID := statev2.PublicationIntentID("intent-1")
	st := newFakeState(workID, intentID, statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{})}
	locker := &fakeOwnerLocker{mismatch: true}
	d := NewDispatcher(st, pub, locker, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitPublication(context.Background(), workID, intentID)
	if !errors.Is(result.Err, ErrPublicationStale) || pub.observes != 0 || pub.publishes != 0 {
		t.Fatalf("result=%v observe=%d publish=%d", result.Err, pub.observes, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestLoadedWorkIDMustMatchQueueBeforePublisherIO(t *testing.T) {
	st := newFakeState("different-work", "intent-1", statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	resultCh := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	close(pub.allow)
	result := <-resultCh
	if !errors.Is(result.Err, ErrPublicationStale) || pub.observes != 0 || pub.publishes != 0 {
		t.Fatalf("result=%v observe=%d publish=%d", result.Err, pub.observes, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestPublicationObservationMatrix(t *testing.T) {
	at := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name        string
		observation PublicationObservation
		observeErr  error
		publishErr  error
		wantStatus  statev2.PublicationStatus
		wantCalls   int
	}{
		{name: "remote match adoption", observation: PublicationObservation{State: PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: at}}, wantStatus: statev2.PublicationCompleted, wantCalls: 0},
		{name: "invalid match receipt conflict", observation: PublicationObservation{State: PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{NodeID: " node-1", PublishedAt: at}}, wantStatus: statev2.PublicationConflict, wantCalls: 0},
		{name: "unknown conflict", observation: PublicationObservation{State: PublicationObservationUnknown}, wantStatus: statev2.PublicationConflict, wantCalls: 0},
		{name: "observe error", observeErr: errors.New("provider unavailable"), wantStatus: statev2.PublicationFailed, wantCalls: 0},
		{name: "publish error", publishErr: errors.New("provider unavailable"), wantStatus: statev2.PublicationFailed, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
			allow := make(chan struct{})
			close(allow)
			pub := &blockingPublisher{allow: allow, observation: tc.observation, observeErr: tc.observeErr, publishErr: tc.publishErr}
			d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
			result := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
			if result.Err != nil {
				t.Fatalf("result error: %v", result.Err)
			}
			if got := st.snapshot.Publications["intent-1"].Status; got != tc.wantStatus {
				t.Fatalf("status = %q, want %q", got, tc.wantStatus)
			}
			if pub.publishes != tc.wantCalls {
				t.Fatalf("publish calls = %d, want %d", pub.publishes, tc.wantCalls)
			}
			_ = d.Close(context.Background())
		})
	}
}

func TestPublicationMutationAfterObservePreventsPublish(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{}), entered: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	resultCh := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	<-pub.entered
	st.mu.Lock()
	st.snapshot.Control.PauseRequested = true
	st.mu.Unlock()
	close(pub.allow)
	result := <-resultCh
	if !errors.Is(result.Err, ErrPublicationPaused) || pub.publishes != 0 {
		t.Fatalf("result=%v publish calls=%d", result.Err, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestFailedPublicationRetriesPersistedIdentityBeforeIO(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationFailed)
	before := st.snapshot.Publications["intent-1"]
	allow := make(chan struct{})
	close(allow)
	pub := &blockingPublisher{allow: allow}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	after := st.snapshot.Publications["intent-1"]
	if after.Status != statev2.PublicationCompleted || after.Key != before.Key || after.Generation != before.Generation || after.PayloadHash != before.PayloadHash || after.PayloadRef != before.PayloadRef || after.Target != before.Target {
		t.Fatalf("retry changed immutable identity: before=%#v after=%#v", before, after)
	}
	if st.applies < 2 {
		t.Fatalf("Apply count = %d, want begin and settlement", st.applies)
	}
	if len(st.transitions) < 2 || st.transitions[0].Publication.Action != statev2.PublicationBegin || st.transitions[0].Publication.Key != before.Key || st.transitions[0].Publication.PayloadHash != before.PayloadHash || st.transitions[0].Publication.PayloadRef != before.PayloadRef || *st.transitions[0].Publication.Target != before.Target {
		t.Fatalf("retry begin did not preserve identity: transitions=%#v", st.transitions)
	}
	_ = d.Close(context.Background())
}

func TestDispatcherSkipsNewerGenerationAndCompletedIntent(t *testing.T) {
	for _, status := range []statev2.PublicationStatus{statev2.PublicationCompleted, statev2.PublicationPending} {
		t.Run(string(status), func(t *testing.T) {
			st := newFakeState("work-1", "intent-1", status)
			if status == statev2.PublicationPending {
				newer := st.snapshot.Publications["intent-1"]
				newer.IntentID = "intent-2"
				newer.Generation = 2
				st.snapshot.Publications["intent-2"] = newer
			}
			allow := make(chan struct{})
			close(allow)
			pub := &blockingPublisher{allow: allow}
			d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
			result := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
			if pub.observes != 0 || pub.publishes != 0 {
				t.Fatalf("publisher calls observe=%d publish=%d", pub.observes, pub.publishes)
			}
			if status == statev2.PublicationCompleted && result.Err != nil {
				t.Fatalf("completed result error: %v", result.Err)
			}
			_ = d.Close(context.Background())
		})
	}
}

func TestDispatcherSkipsBlockedIntentAndDoesNotHoldStateLockDuringObserve(t *testing.T) {
	blocked := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	blocked.snapshot.Control.Blocker = &statev2.OperatorBlocker{Kind: "evidence_mismatch", OperatorRef: "operator", TaskID: "task-1", Diagnostic: "blocked"}
	blockedPublisher := &blockingPublisher{allow: make(chan struct{})}
	blockedDispatcher := NewDispatcher(blocked, blockedPublisher, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	blockedResult := <-blockedDispatcher.SubmitPublication(context.Background(), "work-1", "intent-1")
	if !errors.Is(blockedResult.Err, ErrPublicationBlocked) || blockedPublisher.observes != 0 || blockedPublisher.publishes != 0 {
		t.Fatalf("blocked result=%v observe=%d publish=%d", blockedResult.Err, blockedPublisher.observes, blockedPublisher.publishes)
	}
	_ = blockedDispatcher.Close(context.Background())

	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{}), entered: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	resultCh := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	<-pub.entered
	loaded := make(chan struct{})
	go func() {
		_, _ = st.Load(context.Background(), "work-1")
		close(loaded)
	}()
	select {
	case <-loaded:
	case <-time.After(time.Second):
		t.Fatal("State.Load blocked while Publisher.Observe was blocked")
	}
	close(pub.allow)
	if got := <-resultCh; got.Err != nil {
		t.Fatalf("result error: %v", got.Err)
	}
	_ = d.Close(context.Background())
}

func TestSettlementRevisionConflictReturnsLatestSnapshotWithoutRepublish(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	st.settlementConflict = true
	allow := make(chan struct{})
	close(allow)
	pub := &blockingPublisher{allow: allow}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
	if result.Err == nil || result.Snapshot.Revision != 2 || pub.publishes != 1 {
		t.Fatalf("result=%#v publish calls=%d", result, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestQueuedPublicationRechecksMutatedDurableState(t *testing.T) {
	for _, mutation := range []struct {
		name  string
		apply func(*fakeState)
	}{
		{name: "blocker", apply: func(s *fakeState) {
			s.snapshot.Control.Blocker = &statev2.OperatorBlocker{Kind: "evidence_mismatch", OperatorRef: "operator", TaskID: "task-1", Diagnostic: "blocked"}
		}},
		{name: "newer generation", apply: func(s *fakeState) {
			newer := s.snapshot.Publications["intent-1"]
			newer.IntentID = "intent-2"
			newer.Generation = 2
			s.snapshot.Publications["intent-2"] = newer
		}},
		{name: "completed", apply: func(s *fakeState) {
			p := s.snapshot.Publications["intent-1"]
			p.Status = statev2.PublicationCompleted
			s.snapshot.Publications["intent-1"] = p
		}},
		{name: "superseded", apply: func(s *fakeState) {
			p := s.snapshot.Publications["intent-1"]
			p.Status = statev2.PublicationSuperseded
			s.snapshot.Publications["intent-1"] = p
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
			loadEntered := make(chan struct{})
			loadAllow := make(chan struct{})
			st.loadEntered = loadEntered
			st.loadAllow = loadAllow
			pub := &blockingPublisher{allow: make(chan struct{})}
			d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
			resultCh := d.SubmitPublication(context.Background(), "work-1", "intent-1")
			<-loadEntered
			st.mu.Lock()
			mutation.apply(st)
			st.mu.Unlock()
			close(loadAllow)
			result := <-resultCh
			if pub.observes != 0 || pub.publishes != 0 {
				t.Fatalf("publisher calls observe=%d publish=%d", pub.observes, pub.publishes)
			}
			if mutation.name == "completed" && result.Err != nil {
				t.Fatalf("completed result error: %v", result.Err)
			}
			_ = d.Close(context.Background())
		})
	}
}

type fakeState struct {
	mu                 sync.Mutex
	snapshot           statev2.WorkSnapshot
	applies            int
	transitions        []statev2.TransitionRequest
	settlementConflict bool
	loadEntered        chan struct{}
	loadAllow          chan struct{}
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
	if s.loadEntered != nil {
		close(s.loadEntered)
		s.loadEntered = nil
		allow := s.loadAllow
		s.mu.Unlock()
		<-allow
		s.mu.Lock()
	}
	defer s.mu.Unlock()
	return s.snapshot, nil
}

func (s *fakeState) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ExpectedRevision != s.snapshot.Revision {
		return s.snapshot, statev2.ErrStaleRevision
	}
	hash, err := statev2.TransitionPayloadHash(req)
	if err != nil || hash != req.PayloadHash {
		return s.snapshot, errors.New("invalid canonical transition hash")
	}
	if req.Publication == nil {
		return s.snapshot, errors.New("publication transition required")
	}
	s.transitionCopy(req)
	if s.settlementConflict && req.Publication.Action != statev2.PublicationBegin {
		s.snapshot.Revision++
		return s.snapshot, errors.New("settlement revision conflict")
	}
	s.applies++
	p := s.snapshot.Publications[req.Publication.IntentID]
	switch req.Publication.Action {
	case statev2.PublicationBegin:
		p.Status = statev2.PublicationPending
		p.Attempts++
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

func (s *fakeState) transitionCopy(req statev2.TransitionRequest) {
	copyReq := req
	if req.Publication != nil {
		copyTransition := *req.Publication
		copyReq.Publication = &copyTransition
	}
	s.transitions = append(s.transitions, copyReq)
}

type fakeOwnerLocker struct {
	mu       sync.Mutex
	leases   int
	releases int
	mismatch bool
}

func (l *fakeOwnerLocker) Acquire(_ context.Context, workID contractv2.WorkID, ownerID OwnerID, pid int, startedAt time.Time) (OwnerLease, error) {
	l.mu.Lock()
	l.leases++
	l.mu.Unlock()
	record := OwnerRecord{WorkID: workID, OwnerID: ownerID, PID: pid, StartedAt: startedAt}
	if l.mismatch {
		record.OwnerID = "other-owner"
	}
	return &fakeOwnerLease{record: record, locker: l}, nil
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
	mu          sync.Mutex
	allow       chan struct{}
	entered     chan struct{}
	observation PublicationObservation
	observeErr  error
	publishErr  error
	observes    int
	publishes   int
}

func (p *blockingPublisher) Observe(ctx context.Context, _ statev2.PublicationState) (PublicationObservation, error) {
	p.mu.Lock()
	p.observes++
	if p.entered != nil {
		close(p.entered)
	}
	p.mu.Unlock()
	select {
	case <-p.allow:
	case <-ctx.Done():
		return PublicationObservation{}, ctx.Err()
	}
	if p.observeErr != nil {
		return PublicationObservation{}, p.observeErr
	}
	if p.observation.State == "" {
		return PublicationObservation{State: PublicationObservationAbsent}, nil
	}
	return p.observation, nil
}
func (p *blockingPublisher) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	p.mu.Lock()
	p.publishes++
	p.mu.Unlock()
	if p.publishErr != nil {
		return statev2.PublicationReceipt{}, p.publishErr
	}
	return statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Now().UTC()}, nil
}
