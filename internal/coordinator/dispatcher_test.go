package coordinator

import (
	"context"
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
	if st.applies < 1 || !pub.sawApply {
		t.Fatalf("publish did not follow state Apply: applies=%d sawApply=%v", st.applies, pub.sawApply)
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
