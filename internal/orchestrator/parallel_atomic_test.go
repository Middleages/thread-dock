package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/state"
)

type atomicCursorStore struct {
	saves     []state.RunSnapshot
	appendErr error
}

func (s *atomicCursorStore) Create(context.Context, state.RunSnapshot) error { return nil }
func (s *atomicCursorStore) Load(context.Context, contract.RunID) (state.RunSnapshot, error) {
	return state.RunSnapshot{}, nil
}
func (s *atomicCursorStore) Save(_ context.Context, snapshot state.RunSnapshot) error {
	s.saves = append(s.saves, snapshot)
	return nil
}
func (s *atomicCursorStore) Append(context.Context, contract.RunID, state.Event) error {
	return s.appendErr
}

func TestParallelFinishPreserveCursorPersistsSafeCursorBeforeAppend(t *testing.T) {
	appendErr := errors.New("event append failed")
	store := &atomicCursorStore{appendErr: appendErr}
	o := &Orchestrator{deps: Dependencies{Store: store, Clock: &fakeClock{now: time.Unix(100, 0)}}}
	snapshot := state.RunSnapshot{RunID: "run-atomic", PendingAction: "parallel_read_pr_node", PendingTaskID: "api", ActionCursor: 6}

	err := o.parallelFinishPreserveCursor(context.Background(), &snapshot, "PR node observed")
	if !errors.Is(err, appendErr) {
		t.Fatalf("err=%v, want append failure", err)
	}
	if len(store.saves) != 1 {
		t.Fatalf("saves=%d, want one atomic snapshot save", len(store.saves))
	}
	saved := store.saves[0]
	if saved.ActionCursor != 6 || saved.PendingAction != "" || saved.PendingTaskID != "" || saved.Summary != "PR node observed" {
		t.Fatalf("saved snapshot=%+v", saved)
	}
}
