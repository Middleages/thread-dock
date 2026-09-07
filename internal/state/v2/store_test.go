package statev2

import (
	"context"
	"errors"
	"reflect"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func validSnapshot() WorkSnapshot {
	return WorkSnapshot{SchemaVersion: 2, ProjectID: "project-1", WorkID: "work-1", Revision: 1, State: StateAwaitingApproval, ContractHash: "hash", Contract: contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "ship it", AcceptanceCriteria: []string{"works"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}}}, Documentation: contractv2.DocumentationPlan{Required: false, Reason: "not required"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]Receipt{}}
}

func TestCreatePlanStoresImmutableContractRevisionOne(t *testing.T) {
	s := NewStore(t.TempDir())
	want := validSnapshot()
	got, err := s.CreatePlan(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 1 || got.Contract.Revision != 1 {
		t.Fatalf("got revisions snapshot=%d contract=%d", got.Revision, got.Contract.Revision)
	}
	loaded, err := s.Load(context.Background(), want.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Contract, got.Contract) {
		t.Fatal("contract changed after load")
	}
}

func TestMutateCASReplayConflictAndStale(t *testing.T) {
	s := NewStore(t.TempDir())
	ctx := context.Background()
	if _, err := s.CreatePlan(ctx, validSnapshot()); err != nil {
		t.Fatal(err)
	}
	transition := func(s *WorkSnapshot) error { s.State = StateQueued; s.NextAction = "run"; return nil }
	m := Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-1", PayloadHash: "payload-1", Transition: transition}
	got, err := s.Mutate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || got.State != StateQueued {
		t.Fatalf("mutation = %#v", got)
	}
	replay, err := s.Mutate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Revision != 2 || replay.State != StateQueued {
		t.Fatalf("replay = %#v", replay)
	}
	m.PayloadHash = "payload-2"
	if _, err := s.Mutate(ctx, m); !errors.Is(err, ErrConflict) {
		t.Fatalf("different payload error = %v", err)
	}
	stale := Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-2", PayloadHash: "payload-3", Transition: transition}
	var staleErr *StaleRevisionError
	if _, err := s.Mutate(ctx, stale); !errors.As(err, &staleErr) || staleErr.CurrentRevision != 2 || staleErr.CurrentState != StateQueued {
		t.Fatalf("stale error = %v", err)
	}
}
