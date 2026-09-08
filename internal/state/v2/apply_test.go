package statev2

import (
	"context"
	"errors"
	"reflect"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func transitionRequest(t *testing.T, snapshot WorkSnapshot, requestID contractv2.RequestID, transition WorkTransition) TransitionRequest {
	t.Helper()
	req := TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Work: &transition}
	hash, err := TransitionPayloadHash(req)
	if err != nil {
		t.Fatal(err)
	}
	req.PayloadHash = hash
	return req
}

func TestTransitionPayloadHashRequiresExactlyOneTypedTransition(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  TransitionRequest
	}{
		{name: "none", req: TransitionRequest{}},
		{name: "multiple", req: TransitionRequest{Work: &WorkTransition{Action: WorkApprove}, Task: &TaskTransition{Action: TaskBeginLaunch}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := TransitionPayloadHash(tc.req); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("hash error = %v, want ErrInvalidTransition", err)
			}
		})
	}
	if _, err := TransitionPayloadHash(TransitionRequest{Task: &TaskTransition{Action: TaskBeginLaunch}}); err != nil {
		t.Fatalf("hash should encode a single unsupported typed transition: %v", err)
	}
}

func TestApplyCanonicalHashReplayConflictStaleAndSingleRevision(t *testing.T) {
	s := NewStore(t.TempDir())
	ctx := context.Background()
	before := validSnapshot()
	if _, err := createPlan(s, ctx, before); err != nil {
		t.Fatal(err)
	}
	transition := WorkTransition{Action: WorkApprove, ApprovalRef: "operator-approval", ContractHash: before.ContractHash}
	req := transitionRequest(t, before, "request-approve", transition)
	first, err := s.Apply(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 2 || first.State != StateQueued || len(first.Receipts) != 0 {
		t.Fatalf("first apply = %#v", first)
	}
	replay, err := s.Apply(ctx, req)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay = %#v, err=%v; want original result", replay, err)
	}
	changed := transitionRequest(t, before, "request-approve", WorkTransition{Action: WorkApprove, ApprovalRef: "different", ContractHash: before.ContractHash})
	if _, err := s.Apply(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed payload error = %v, want conflict", err)
	}
	stale := transitionRequest(t, before, "request-stale", WorkTransition{Action: WorkPause})
	if _, err := s.Apply(ctx, stale); !errors.As(err, new(*StaleRevisionError)) {
		t.Fatalf("stale error = %v", err)
	}
	current, err := s.Load(ctx, before.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 2 || current.State != StateQueued {
		t.Fatalf("current = %#v; apply should increment once", current)
	}
}

func TestApplyRejectsHashMismatchAndInvalidTransitionWithoutWrite(t *testing.T) {
	s := NewStore(t.TempDir())
	ctx := context.Background()
	before := validSnapshot()
	if _, err := createPlan(s, ctx, before); err != nil {
		t.Fatal(err)
	}
	req := TransitionRequest{WorkID: before.WorkID, ExpectedRevision: before.Revision, RequestID: "request-invalid", PayloadHash: "not-the-canonical-hash", Work: &WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: before.ContractHash}}
	if _, err := s.Apply(ctx, req); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("hash mismatch error = %v", err)
	}
	invalid := TransitionRequest{WorkID: before.WorkID, ExpectedRevision: before.Revision, RequestID: "request-invalid-2", PayloadHash: "anything", Task: &TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch}}
	if _, err := s.Apply(ctx, invalid); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pre-approval task error = %v", err)
	}
	after, err := s.Load(ctx, before.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.State != before.State || !reflect.DeepEqual(after.Control, before.Control) || !reflect.DeepEqual(after.TaskStates, before.TaskStates) || len(after.Receipts) != 1 {
		t.Fatalf("invalid apply wrote state: before=%#v after=%#v", before, after)
	}
}
