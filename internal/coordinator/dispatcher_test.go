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

func newRuntimeDispatcherForTest(state State, publisher Publisher, runtime Runtime, locker OwnerLocker, ownerID OwnerID, pid int, startedAt time.Time) Dispatcher {
	return newDispatcher(state, publisher, runtime, nil, locker, ownerID, pid, startedAt)
}

func TestDispatcherDeduplicatesPublicationAndReleasesOwner(t *testing.T) {
	workID := contractv2.WorkID("work-1")
	intentID := statev2.PublicationIntentID("intent-1")
	st := newFakeState(workID, intentID, statev2.PublicationPending)
	allow := make(chan struct{})
	close(allow)
	pub := &blockingPublisher{allow: allow}
	locker := &fakeOwnerLocker{}
	d := NewDispatcher(st, pub, locker, OwnerID("owner-1"), 42, time.Now().UTC())

	results := make([]<-chan CommandResult, 20)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = d.SubmitPublication(context.Background(), workID, intentID)
		}(i)
	}
	wg.Wait()
	for _, result := range results {
		select {
		case got := <-result:
			if got.Err != nil {
				t.Fatalf("result error: %v", got.Err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for publication result")
		}
	}
	if pub.observes != 1 || pub.publishes != 1 {
		t.Fatalf("publisher calls = observe %d publish %d", pub.observes, pub.publishes)
	}
	if st.applies < 1 || len(st.transitions) != 1 || st.transitions[0].Publication.Action != statev2.PublicationComplete {
		t.Fatalf("publish settlement did not Apply: applies=%d transitions=%#v", st.applies, st.transitions)
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if locker.releases != 1 {
		t.Fatalf("owner releases = %d, want 1", locker.releases)
	}
}

func TestDispatcherRuntimeAndPublicationShareWorkQueue(t *testing.T) {
	st := &sharedWorkState{runtimeTestState: runtimeTestState{snapshot: runtimeTestSnapshot()}}
	st.snapshot.Publications = map[statev2.PublicationIntentID]statev2.PublicationState{
		"intent-1": {IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, Status: statev2.PublicationPending, PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PayloadRef: "artifact://one", Target: statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: true},
	}
	rt := &barrierRuntime{launchEntered: make(chan struct{}), launchRelease: make(chan struct{}), observeEntered: make(chan struct{}), observeRelease: make(chan struct{})}
	allow := make(chan struct{})
	close(allow)
	pub := &blockingPublisher{allow: allow}
	locker := &fakeOwnerLocker{}
	d := newRuntimeDispatcherForTest(st, pub, rt, locker, OwnerID("owner-1"), 42, time.Now().UTC())
	runtimeResult := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.launchEntered:
	case <-time.After(time.Second):
		t.Fatal("runtime launch did not enter barrier")
	}
	publicationResult := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	select {
	case result := <-publicationResult:
		if result.Err != nil {
			t.Fatalf("publication result: %v", result.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("publication was blocked by runtime worker")
	}
	if got := len(d.(*publicationDispatcher).queues); got != 1 {
		t.Fatalf("work queues = %d, want 1", got)
	}
	if locker.leases != 1 {
		t.Fatalf("owner leases = %d, want 1", locker.leases)
	}
	close(rt.launchRelease)
	select {
	case result := <-runtimeResult:
		if result.Err != nil {
			t.Fatalf("runtime result: %v", result.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime result stranded")
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type sharedWorkState struct{ runtimeTestState }

func (s *sharedWorkState) Apply(ctx context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	if req.Publication == nil {
		return s.runtimeTestState.Apply(ctx, req)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ExpectedRevision != s.snapshot.Revision {
		return s.snapshot, statev2.ErrStaleRevision
	}
	hash, err := statev2.TransitionPayloadHash(req)
	if err != nil || hash != req.PayloadHash {
		return s.snapshot, errors.New("invalid transition hash")
	}
	p := s.snapshot.Publications[req.Publication.IntentID]
	s.transitions = append(s.transitions, req)
	if req.Publication.Action == statev2.PublicationBegin {
		p.Status = statev2.PublicationPending
	} else if req.Publication.Action == statev2.PublicationComplete {
		p.Status = statev2.PublicationCompleted
		p.Receipt = req.Publication.Receipt
	}
	s.snapshot.Publications[p.IntentID] = p
	s.snapshot.Revision++
	return s.snapshot, nil
}

func TestDispatcherSkipsPausedIntentQueuedBeforePause(t *testing.T) {
	workID := contractv2.WorkID("work-1")
	intentID := statev2.PublicationIntentID("intent-1")
	st := newFakeState(workID, intentID, statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	st.mu.Lock()
	st.snapshot.Control.PauseRequested = true
	st.mu.Unlock()
	result := <-d.SubmitPublication(context.Background(), workID, intentID)
	close(pub.allow)
	if result.Err == nil || pub.observes != 0 || pub.publishes != 0 {
		t.Fatalf("paused result=%v observe=%d publish=%d", result.Err, pub.observes, pub.publishes)
	}
	_ = d.Close(context.Background())
}

func TestDispatcherCloseSubmitBarrierResolvesEveryResult(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		workID := contractv2.WorkID("work-1")
		intentID := statev2.PublicationIntentID("intent-1")
		st := newFakeState(workID, intentID, statev2.PublicationPending)
		pub := &blockingPublisher{allow: make(chan struct{}), entered: make(chan struct{})}
		d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
		first := d.SubmitPublication(context.Background(), workID, intentID)
		select {
		case <-pub.entered:
		case <-time.After(time.Second):
			t.Fatal("publisher did not enter Observe")
		}
		closeStarted := make(chan struct{})
		closeDone := make(chan error, 1)
		go func() {
			close(closeStarted)
			closeDone <- d.Close(context.Background())
		}()
		<-closeStarted
		second := d.SubmitPublication(context.Background(), workID, intentID)
		close(pub.allow)
		select {
		case <-first:
		case <-time.After(time.Second):
			t.Fatal("first result stranded")
		}
		select {
		case got := <-second:
			if !errors.Is(got.Err, ErrDispatcherClosed) {
				t.Fatalf("second result error = %v", got.Err)
			}
		case <-time.After(time.Second):
			t.Fatal("second result stranded")
		}
		if err := <-closeDone; err != nil {
			t.Fatal(err)
		}
	}
}

func TestDispatcherCloseCancelsSaturatedQueueAndResolvesEverySubmitter(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{}), entered: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	results := make([]<-chan CommandResult, 65)
	results[0] = d.SubmitPublication(context.Background(), "work-1", "intent-1")
	select {
	case <-pub.entered:
	case <-time.After(time.Second):
		t.Fatal("publisher did not enter Observe")
	}
	var submitWG sync.WaitGroup
	for i := 1; i <= 64; i++ {
		submitWG.Add(1)
		go func(i int) {
			defer submitWG.Done()
			results[i] = d.SubmitPublication(context.Background(), "work-1", "intent-1")
		}(i)
	}
	submitWG.Wait()
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Close(closeCtx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for i, result := range results {
		select {
		case <-result:
		case <-time.After(time.Second):
			t.Fatalf("result %d stranded", i)
		}
		select {
		case extra := <-result:
			t.Fatalf("result %d delivered twice: %#v", i, extra)
		default:
		}
	}
}

func TestDispatcherCloseCancelsSubmitterAfterRegistrationBeforeSend(t *testing.T) {
	st := newFakeState("work-1", "intent-1", statev2.PublicationPending)
	pub := &blockingPublisher{allow: make(chan struct{}), entered: make(chan struct{})}
	d := NewDispatcher(st, pub, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	first := d.SubmitPublication(context.Background(), "work-1", "intent-1")
	<-pub.entered
	q := d.(*publicationDispatcher).queues["work-1"]
	registered := make(chan struct{})
	resume := make(chan struct{})
	q.beforeSelect = func() {
		close(registered)
		<-resume
	}
	secondReturned := make(chan struct{})
	var second <-chan CommandResult
	go func() {
		second = d.SubmitPublication(context.Background(), "work-1", "intent-1")
		close(secondReturned)
	}()
	<-registered
	closeDone := make(chan error, 1)
	go func() { closeDone <- d.Close(context.Background()) }()
	// Close marks stopped and cancels the queue while holding submitMu. Wait
	// for that observable cancellation before releasing the registered sender;
	// a timing delay would leave the Add-before-select interleaving racy.
	select {
	case <-q.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel queue")
	}
	close(resume)
	select {
	case <-secondReturned:
	case <-time.After(time.Second):
		t.Fatal("registered submitter did not return")
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	<-first
	secondResult := <-second
	if !errors.Is(secondResult.Err, ErrDispatcherClosed) {
		t.Fatalf("registered submitter error = %v, want ErrDispatcherClosed", secondResult.Err)
	}
	// Close waits for the worker and all registered submitters, so no later
	// result can arrive. A non-blocking receive therefore proves one delivery.
	for name, result := range map[string]<-chan CommandResult{"first": first, "second": second} {
		select {
		case extra := <-result:
			t.Fatalf("%s result delivered twice: %#v", name, extra)
		default:
		}
	}
}
