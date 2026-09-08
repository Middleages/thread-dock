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
		{name: "observe error is ambiguous conflict", observeErr: errors.New("provider unavailable"), wantStatus: statev2.PublicationConflict, wantCalls: 0},
		{name: "publish error is ambiguous conflict", publishErr: errors.New("provider unavailable"), wantStatus: statev2.PublicationConflict, wantCalls: 1},
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

func TestAmbiguousPublishErrorBlocksLaterSubmissionWithoutRepublish(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	allow := make(chan struct{})
	close(allow)
	pub := &blockingPublisher{allow: allow, publishErr: errors.New("remote timeout")}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	first := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
	if first.Err != nil || st.snapshot.Publications["intent-1"].Status != statev2.PublicationConflict {
		t.Fatalf("first result=%v status=%q", first.Err, st.snapshot.Publications["intent-1"].Status)
	}
	second := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
	if second.Err == nil || pub.publishes != 1 {
		t.Fatalf("second result=%v publish calls=%d, want one publish and a blocked retry", second.Err, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestImmutablePublicationMutationAfterObservePreventsPublish(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{}), entered: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	resultCh := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	<-pub.entered
	st.mu.Lock()
	p := st.snapshot.Publications["intent-1"]
	p.PayloadHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	p.Target.Resource = "changed-resource"
	st.snapshot.Publications["intent-1"] = p
	st.mu.Unlock()
	close(pub.allow)
	result := <-resultCh
	if !errors.Is(result.Err, ErrPublicationStale) || pub.publishes != 0 {
		t.Fatalf("result=%v publish calls=%d", result.Err, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestFailedPublicationRetriesPersistedIdentityBeforeIO(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationFailed)
	before := st.snapshot.Publications["intent-1"]
	beforeRevision := st.snapshot.Revision
	allow := make(chan struct{})
	pub := &blockingPublisher{allow: allow, entered: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	resultCh := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	<-pub.entered
	pub.mu.Lock()
	if len(pub.observed) != 1 {
		t.Fatalf("Observe calls = %d, want 1", len(pub.observed))
	}
	observed := pub.observed[0]
	pub.mu.Unlock()
	st.mu.Lock()
	if st.snapshot.Publications["intent-1"].Status != statev2.PublicationPending || len(st.transitions) < 1 || st.transitions[0].Publication.Action != statev2.PublicationBegin {
		t.Fatalf("Observe entered before persisted retry begin: snapshot=%#v transitions=%#v", st.snapshot.Publications["intent-1"], st.transitions)
	}
	beginRequest := st.transitions[0]
	if beginRequest.ExpectedRevision != beforeRevision {
		t.Fatalf("retry begin expected revision = %d, want %d", beginRequest.ExpectedRevision, beforeRevision)
	}
	canonicalHash, err := statev2.TransitionPayloadHash(beginRequest)
	if err != nil {
		t.Fatalf("hash retry begin: %v", err)
	}
	if beginRequest.PayloadHash != canonicalHash {
		t.Fatalf("retry begin payload hash = %q, want canonical %q", beginRequest.PayloadHash, canonicalHash)
	}
	begin := beginRequest.Publication
	if begin == nil || begin.IntentID != before.IntentID || begin.Key != before.Key || begin.Generation != before.Generation || begin.Kind != before.Kind || begin.PayloadHash != before.PayloadHash || begin.PayloadRef != before.PayloadRef || begin.Target == nil || *begin.Target != before.Target || begin.CompletionRequired == nil || *begin.CompletionRequired != before.CompletionRequired {
		t.Fatalf("retry begin did not preserve full immutable identity: %#v", begin)
	}
	if observed.Status != statev2.PublicationPending || observed.IntentID != before.IntentID || observed.Key != before.Key || observed.Generation != before.Generation || observed.Kind != before.Kind || observed.PayloadHash != before.PayloadHash || observed.PayloadRef != before.PayloadRef || observed.Target != before.Target || observed.CompletionRequired != before.CompletionRequired {
		t.Fatalf("Observe received unexpected pending intent: %#v", observed)
	}
	st.mu.Unlock()
	close(allow)
	result := <-resultCh
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
	for _, tc := range []struct {
		name      string
		setup     func(*fakeState)
		completed bool
	}{
		{name: "already completed", completed: true, setup: func(s *fakeState) {
			p := s.snapshot.Publications["intent-1"]
			p.Status = statev2.PublicationCompleted
			p.Receipt = &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)}
			s.snapshot.Publications["intent-1"] = p
		}},
		{name: "newer pending successor", setup: func(s *fakeState) {
			p := s.snapshot.Publications["intent-1"]
			p.Status = statev2.PublicationCompleted
			p.Receipt = &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)}
			s.snapshot.Publications["intent-1"] = p
			newer := p
			newer.IntentID = "intent-2"
			newer.Generation = 2
			newer.Status = statev2.PublicationPending
			newer.Receipt = nil
			s.snapshot.Publications["intent-2"] = newer
		}},
		{name: "superseded prior with pending successor", setup: func(s *fakeState) {
			p := s.snapshot.Publications["intent-1"]
			p.Status = statev2.PublicationSuperseded
			s.snapshot.Publications["intent-1"] = p
			newer := p
			newer.IntentID = "intent-2"
			newer.Generation = 2
			newer.Status = statev2.PublicationPending
			s.snapshot.Publications["intent-2"] = newer
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
			tc.setup(st)
			allow := make(chan struct{})
			close(allow)
			pub := &blockingPublisher{allow: allow}
			d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
			result := <-d.SubmitPublication(context.Background(), "work-1", "intent-1")
			if pub.observes != 0 || pub.publishes != 0 {
				t.Fatalf("publisher calls observe=%d publish=%d", pub.observes, pub.publishes)
			}
			if tc.completed && result.Err != nil {
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
			older := s.snapshot.Publications["intent-1"]
			older.Status = statev2.PublicationCompleted
			older.Receipt = &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)}
			s.snapshot.Publications["intent-1"] = older
			newer := older
			newer.IntentID = "intent-2"
			newer.Generation = 2
			newer.Status = statev2.PublicationPending
			newer.Receipt = nil
			newer.LastError = ""
			s.snapshot.Publications["intent-2"] = newer
		}},
		{name: "completed", apply: func(s *fakeState) {
			p := s.snapshot.Publications["intent-1"]
			p.Status = statev2.PublicationCompleted
			p.Receipt = &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)}
			s.snapshot.Publications["intent-1"] = p
		}},
		{name: "superseded", apply: func(s *fakeState) {
			older := s.snapshot.Publications["intent-1"]
			older.Status = statev2.PublicationSuperseded
			older.Receipt = nil
			older.LastError = ""
			s.snapshot.Publications["intent-1"] = older
			newer := older
			newer.IntentID = "intent-2"
			newer.Generation = 2
			newer.Status = statev2.PublicationPending
			newer.Receipt = nil
			newer.LastError = ""
			s.snapshot.Publications["intent-2"] = newer
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
	p := statev2.PublicationState{
		IntentID: intentID, Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, Status: status,
		PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PayloadRef: "artifact://one",
		Target: statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: true, Attempts: 1,
	}
	if status == statev2.PublicationFailed {
		p.LastError = "previous provider failure"
	}
	if status == statev2.PublicationCompleted {
		p.Receipt = &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)}
	}
	return &fakeState{snapshot: statev2.WorkSnapshot{WorkID: workID, Revision: 1, State: statev2.StatePublicationPending, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{intentID: p}}}
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
		p.LastError = ""
		p.Receipt = nil
	case statev2.PublicationComplete:
		p.Status = statev2.PublicationCompleted
		p.Receipt = req.Publication.Receipt
		p.LastError = ""
	case statev2.PublicationFail:
		p.Status = statev2.PublicationFailed
		p.Receipt = nil
		p.LastError = req.Publication.Diagnostic
		if p.LastError == "" {
			p.LastError = "provider failure"
		}
	case statev2.PublicationActionConflict:
		p.Status = statev2.PublicationConflict
		p.Receipt = nil
		p.LastError = req.Publication.Diagnostic
		if p.LastError == "" {
			p.LastError = "publication conflict"
		}
		s.snapshot.Control.Blocker = req.Publication.Blocker
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
	observed    []statev2.PublicationState
}

func (p *blockingPublisher) Observe(ctx context.Context, intent statev2.PublicationState) (PublicationObservation, error) {
	p.mu.Lock()
	p.observes++
	p.observed = append(p.observed, intent)
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
