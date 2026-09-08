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
	results := make([]<-chan CommandResult, 66)
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
	blockedReturned := make(chan struct{})
	go func() {
		results[65] = d.SubmitPublication(context.Background(), "work-1", "intent-1")
		close(blockedReturned)
	}()
	select {
	case <-blockedReturned:
		t.Fatal("saturated submitter returned before Close")
	case <-time.After(20 * time.Millisecond):
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Close(closeCtx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-blockedReturned:
	case <-time.After(time.Second):
		t.Fatal("blocked submitter did not return")
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
		case <-time.After(5 * time.Millisecond):
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
	close(resume)
	select {
	case <-secondReturned:
	case <-time.After(time.Second):
		t.Fatal("registered submitter did not return")
	}
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("second result did not resolve")
	}
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first result did not resolve")
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}
