package statev2

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestMutateRejectsTransitionThatChangesImmutableFieldsOrReceipts(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*WorkSnapshot)
	}{
		{"project", func(s *WorkSnapshot) { s.ProjectID = "other-project" }},
		{"schema", func(s *WorkSnapshot) { s.SchemaVersion = 99 }},
		{"hash", func(s *WorkSnapshot) { s.ContractHash = "other-hash" }},
		{"contract", func(s *WorkSnapshot) { s.Contract.Request = "changed" }},
		{"receipts", func(s *WorkSnapshot) {
			s.Receipts["foreign"] = Receipt{RequestID: "foreign", PayloadHash: "foreign", Status: "committed"}
		}},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(t.TempDir())
			before := validSnapshot()
			if _, err := s.CreatePlan(context.Background(), before); err != nil {
				t.Fatal(err)
			}
			_, err := s.Mutate(context.Background(), Mutation{WorkID: before.WorkID, ExpectedRevision: 1, RequestID: "request-immutable", PayloadHash: "payload", Transition: func(next *WorkSnapshot) error { tc.change(next); return nil }})
			if err == nil {
				t.Fatal("mutation accepted changes to immutable snapshot fields")
			}
			got, loadErr := s.Load(context.Background(), before.WorkID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if !reflect.DeepEqual(got, before) {
				t.Fatalf("failed mutation changed persisted snapshot: %#v", got)
			}
		})
	}
}

func TestMutateRejectsUnknownWorkflowState(t *testing.T) {
	s := NewStore(t.TempDir())
	before := validSnapshot()
	if _, err := s.CreatePlan(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	_, err := s.Mutate(context.Background(), Mutation{WorkID: before.WorkID, ExpectedRevision: 1, RequestID: "request-state", PayloadHash: "payload", Transition: func(next *WorkSnapshot) error { next.State = WorkState("made_up"); return nil }})
	if err == nil {
		t.Fatal("mutation accepted unknown workflow state")
	}
}

func TestCreatePlanRecoversCommittedContractWhenSnapshotPublicationIsInterrupted(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	snapshot := validSnapshot()
	contracts := filepath.Join(root, "v2", "work", string(snapshot.WorkID), "contracts")
	if err := os.MkdirAll(contracts, 0700); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(contracts, "1.json")
	f, err := os.OpenFile(contractPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := contractv2.Write(f, snapshot.Contract); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := s.CreatePlan(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("recovery did not publish missing snapshot: %v", err)
	}
	if got.WorkID != snapshot.WorkID || got.Revision != 1 {
		t.Fatalf("recovered plan = %#v", got)
	}
}

func TestCreatePlanRecoveryAcceptsCanonicalEquivalentExplicitEmptyOptionalSlices(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	snapshot := validSnapshot()
	snapshot.Contract.RepositoryPlans[0].Verification = []contractv2.CommandSpec{}
	snapshot.Contract.Tasks[0].DependsOn = []contractv2.TaskID{}
	contracts := filepath.Join(root, "v2", "work", string(snapshot.WorkID), "contracts")
	if err := os.MkdirAll(contracts, 0700); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(contracts, "1.json")
	f, err := os.OpenFile(contractPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := contractv2.Write(f, snapshot.Contract); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePlan(context.Background(), snapshot); err != nil {
		t.Fatalf("canonical-equivalent recovery rejected explicit empty slices: %v", err)
	}
}

func TestCreatePlanHardensPreexistingDirectoriesAndTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	contracts := filepath.Join(root, "v2", "work", "work-1", "contracts")
	if err := os.MkdirAll(contracts, 0755); err != nil {
		t.Fatal(err)
	}
	contractTmp := filepath.Join(contracts, "1.json.tmp")
	if err := os.WriteFile(contractTmp, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	workTmp := filepath.Join(filepath.Dir(contracts), "work.json.tmp")
	if err := os.WriteFile(workTmp, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root).CreatePlan(context.Background(), validSnapshot()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "v2", "work"), filepath.Dir(contracts), contracts} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0700 {
			t.Fatalf("%s mode = %o, want 700", path, info.Mode().Perm())
		}
	}
	for _, path := range []string{filepath.Join(contracts, "1.json"), filepath.Join(filepath.Dir(contracts), "work.json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s mode = %o, want 600", path, info.Mode().Perm())
		}
	}
}
