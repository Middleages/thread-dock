package statev2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func validSnapshot() WorkSnapshot {
	snapshot := WorkSnapshot{SchemaVersion: 2, ProjectID: "project-1", WorkID: "work-1", Revision: 1, State: StateAwaitingApproval, Contract: contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "ship it", AcceptanceCriteria: []string{"works"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}}}, Documentation: contractv2.DocumentationPlan{Required: false, Reason: "not required"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]Receipt{}}
	canonical, _ := canonicalContract(snapshot.Contract)
	sum := sha256.Sum256(canonical)
	snapshot.ContractHash = hex.EncodeToString(sum[:])
	return snapshot
}

func createPlan(s Store, ctx context.Context, snapshot WorkSnapshot) (WorkSnapshot, error) {
	return s.CreatePlan(ctx, snapshot, contractv2.RequestID("create-")+contractv2.RequestID(snapshot.WorkID), "payload-"+string(snapshot.WorkID))
}

func snapshotForWork(id contractv2.WorkID, state WorkState) WorkSnapshot {
	snapshot := validSnapshot()
	snapshot.WorkID = id
	snapshot.State = state
	snapshot.Contract.WorkID = id
	canonical, _ := canonicalContract(snapshot.Contract)
	sum := sha256.Sum256(canonical)
	snapshot.ContractHash = hex.EncodeToString(sum[:])
	return snapshot
}

func TestCreatePlanStoresImmutableContractRevisionOne(t *testing.T) {
	s := NewStore(t.TempDir())
	want := validSnapshot()
	got, err := createPlan(s, context.Background(), want)
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
	if _, err := createPlan(s, ctx, validSnapshot()); err != nil {
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
			if _, err := createPlan(s, context.Background(), before); err != nil {
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
			if got.Revision != before.Revision || got.State != before.State || !reflect.DeepEqual(got.Contract, before.Contract) || len(got.Receipts) != 1 {
				t.Fatalf("failed mutation changed persisted snapshot: %#v", got)
			}
		})
	}
}

func TestMutateRejectsUnknownWorkflowState(t *testing.T) {
	s := NewStore(t.TempDir())
	before := validSnapshot()
	if _, err := createPlan(s, context.Background(), before); err != nil {
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
	got, err := createPlan(s, context.Background(), snapshot)
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
	if _, err := createPlan(s, context.Background(), snapshot); err != nil {
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
	if _, err := createPlan(NewStore(root), context.Background(), validSnapshot()); err != nil {
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

func TestListIncludesTerminalAndOperatorStatesInWorkIDOrder(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	ctx := context.Background()
	for i, target := range []struct {
		id    contractv2.WorkID
		state WorkState
	}{
		{"work-z", StateCompleted},
		{"work-a", StateNeedsOperator},
	} {
		snapshot := snapshotForWork(target.id, StateAwaitingApproval)
		if _, err := createPlan(s, ctx, snapshot); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Mutate(ctx, Mutation{WorkID: target.id, ExpectedRevision: 1, RequestID: contractv2.RequestID("request-") + contractv2.RequestID(string(rune('a'+i))), PayloadHash: "payload", Transition: func(next *WorkSnapshot) error {
			next.State = target.state
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "v2", "work", "not-a-work-file"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "v2", "work", "ignored.tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v2", "work", "ignored.tmp", "work.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 2 {
		t.Fatalf("listed snapshots = %#v", got)
	}
	if got[0].WorkID != "work-a" || got[0].State != StateNeedsOperator || got[1].WorkID != "work-z" || got[1].State != StateCompleted {
		t.Fatalf("listed snapshots = %#v", got)
	}
}

func TestListRejectsCorruptDirectChildSnapshot(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	ctx := context.Background()
	if _, err := createPlan(s, ctx, snapshotForWork("work-valid", StateAwaitingApproval)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mutate(ctx, Mutation{WorkID: "work-valid", ExpectedRevision: 1, RequestID: "request-valid", PayloadHash: "payload", Transition: func(next *WorkSnapshot) error {
		next.State = StateCompleted
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	corruptDir := filepath.Join(root, "v2", "work", "work-corrupt")
	if err := os.MkdirAll(corruptDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "work.json"), []byte(strings.TrimSpace(`{"schemaVersion":2}`)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(ctx); err == nil {
		t.Fatal("List accepted corrupt direct-child snapshot")
	}
}

func TestListOnEmptyRootReturnsNonNilEmptySlice(t *testing.T) {
	got, err := NewStore(t.TempDir()).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("listed snapshots = %#v", got)
	}
}

func TestCreatePlanIsIdempotentByRequestAndPersistsCreationReceipt(t *testing.T) {
	s := NewStore(t.TempDir())
	want := validSnapshot()
	first, err := s.CreatePlan(context.Background(), want, "request-plan", "payload-plan")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CreatePlan(context.Background(), want, "request-plan", "payload-plan")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay=%#v first=%#v", replay, first)
	}
	persisted, err := s.Load(context.Background(), want.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := persisted.Receipts["request-plan"]
	if !ok || receipt.Status != "committed" || len(receipt.Result) == 0 {
		t.Fatalf("creation receipt=%#v", receipt)
	}
	if _, err := s.CreatePlan(context.Background(), want, "request-plan", "payload-other"); !errors.As(err, new(*RequestConflictError)) {
		t.Fatalf("changed payload error=%v, want typed request conflict", err)
	}
}

func TestMutationReplayReturnsOriginalResultAfterLaterMutation(t *testing.T) {
	s := NewStore(t.TempDir())
	ctx := context.Background()
	if _, err := s.CreatePlan(ctx, validSnapshot(), "create-plan", "create-payload"); err != nil {
		t.Fatal(err)
	}
	approve := Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-a", PayloadHash: "payload-a", Transition: func(next *WorkSnapshot) error {
		next.State = StateQueued
		next.NextAction = "run"
		return nil
	}}
	original, err := s.Mutate(ctx, approve)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 2, RequestID: "request-b", PayloadHash: "payload-b", Transition: func(next *WorkSnapshot) error {
		next.State = StateRunning
		next.NextAction = "monitor"
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	replay, err := s.Mutate(ctx, approve)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replay, original) {
		t.Fatalf("replay=%#v original=%#v", replay, original)
	}
	current, err := s.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 3 || current.State != StateRunning {
		t.Fatalf("current=%#v", current)
	}
}

func TestLoadAndMutateRejectContractTampering(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	ctx := context.Background()
	if _, err := s.CreatePlan(ctx, validSnapshot(), "request-tamper", "payload-tamper"); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, "v2", "work", "work-1", "contracts", "1.json")
	data, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"request": "ship it"`), []byte(`"request": "tampered"`), 1)
	if err := os.WriteFile(contractPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, "work-1"); err == nil {
		t.Fatal("Load accepted tampered contract file")
	}
	if _, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-after-tamper", PayloadHash: "payload", Transition: func(next *WorkSnapshot) error {
		next.State = StateQueued
		return nil
	}}); err == nil {
		t.Fatal("Mutate accepted tampered contract file")
	}
}

func TestLoadAndMutateRejectEmbeddedContractAndHashTampering(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "embedded contract", mutate: func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"request":"ship it"`), []byte(`"request":"tampered"`), 1)
		}},
		{name: "contract hash", mutate: func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"contractHash":"`+validSnapshot().ContractHash+`"`), []byte(`"contractHash":"`+strings.Repeat("f", 64)+`"`), 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			s := NewStore(root)
			ctx := context.Background()
			if _, err := s.CreatePlan(ctx, validSnapshot(), "request-tamper", "payload-tamper"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "v2", "work", "work-1", "work.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, tc.mutate(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Load(ctx, "work-1"); err == nil {
				t.Fatal("Load accepted tampered snapshot")
			}
			if _, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-after-tamper", PayloadHash: "payload", Transition: func(next *WorkSnapshot) error {
				next.State = StateQueued
				return nil
			}}); err == nil {
				t.Fatal("Mutate accepted tampered snapshot")
			}
		})
	}
}

func TestSequentialMutationReceiptsUseBoundedEmptyProjection(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	ctx := context.Background()
	if _, err := s.CreatePlan(ctx, validSnapshot(), "create-plan", "create-payload"); err != nil {
		t.Fatal(err)
	}
	first, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-a", PayloadHash: "payload-a", Transition: func(next *WorkSnapshot) error {
		next.State = StateQueued
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 2, RequestID: "request-b", PayloadHash: "payload-b", Transition: func(next *WorkSnapshot) error {
		next.State = StateRunning
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipts == nil || len(first.Receipts) != 0 || second.Receipts == nil || len(second.Receipts) != 0 {
		t.Fatalf("client projections first=%#v second=%#v", first.Receipts, second.Receipts)
	}
	persisted, err := s.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []contractv2.RequestID{"request-a", "request-b"} {
		receipt := persisted.Receipts[id]
		if len(receipt.Result) == 0 || bytes.Contains(receipt.Result, []byte(`"request-a"`)) || bytes.Contains(receipt.Result, []byte(`"request-b"`)) || bytes.Contains(receipt.Result, []byte(`"receipts":{"`)) {
			t.Fatalf("receipt %q is nested or missing: %s", id, receipt.Result)
		}
	}
	replay, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-a", PayloadHash: "payload-a", Transition: func(next *WorkSnapshot) error {
		next.State = StateQueued
		return nil
	}})
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay=%#v first=%#v err=%v", replay, first, err)
	}
	if len(persisted.Receipts["request-b"].Result) > len(persisted.Receipts["request-a"].Result)*2 {
		t.Fatalf("receipt result grew unexpectedly: a=%d b=%d", len(persisted.Receipts["request-a"].Result), len(persisted.Receipts["request-b"].Result))
	}
}

func TestReceiptReplayRejectsMalformedResultWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "unknown field", mutate: func(raw []byte) []byte {
			return append(bytes.TrimSpace(raw[:len(raw)-1]), []byte(`,"unexpected":true}`)...)
		}},
		{name: "trailing JSON", mutate: func(raw []byte) []byte {
			return append(raw, []byte(` {}`)...)
		}},
		{name: "nested receipts", mutate: func(raw []byte) []byte {
			var result WorkSnapshot
			if err := json.Unmarshal(raw, &result); err != nil {
				panic(err)
			}
			result.Receipts = map[contractv2.RequestID]Receipt{"nested": {RequestID: "nested", PayloadHash: "nested", Status: "committed"}}
			encoded, _ := json.Marshal(result)
			return encoded
		}},
		{name: "changed contract", mutate: func(raw []byte) []byte {
			var result WorkSnapshot
			if err := json.Unmarshal(raw, &result); err != nil {
				panic(err)
			}
			result.Contract.Request = "changed"
			encoded, _ := json.Marshal(result)
			return encoded
		}},
		{name: "bad contract hash", mutate: func(raw []byte) []byte {
			var result WorkSnapshot
			if err := json.Unmarshal(raw, &result); err != nil {
				panic(err)
			}
			result.ContractHash = strings.Repeat("f", 64)
			encoded, _ := json.Marshal(result)
			return encoded
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			s := NewStore(root)
			ctx := context.Background()
			if _, err := s.CreatePlan(ctx, validSnapshot(), "create-plan", "create-payload"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-a", PayloadHash: "payload-a", Transition: func(next *WorkSnapshot) error {
				next.State = StateQueued
				return nil
			}}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "v2", "work", "work-1", "work.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot WorkSnapshot
			if err := json.Unmarshal(data, &snapshot); err != nil {
				t.Fatal(err)
			}
			receipt := snapshot.Receipts["request-a"]
			originalResult := append([]byte(nil), receipt.Result...)
			receipt.Result = tc.mutate(receipt.Result)
			snapshot.Receipts["request-a"] = receipt
			tampered, err := json.Marshal(snapshot)
			if err != nil {
				tampered = bytes.Replace(data, originalResult, receipt.Result, 1)
			}
			if err := os.WriteFile(path, tampered, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Mutate(ctx, Mutation{WorkID: "work-1", ExpectedRevision: 1, RequestID: "request-a", PayloadHash: "payload-a", Transition: func(next *WorkSnapshot) error {
				t.Fatal("tampered replay invoked transition")
				return nil
			}}); err == nil {
				t.Fatal("replay accepted malformed receipt result")
			}
			persisted, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(persisted, tampered) {
				t.Fatal("replay changed persisted state")
			}
		})
	}
}

func TestReceiptDecoderRejectsMismatchedCurrentContractHash(t *testing.T) {
	snapshot := validSnapshot()
	result, err := json.Marshal(clientResultProjection(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{Status: "committed", Result: result}
	if _, err := decodeReceiptResult(receipt, snapshot.Contract, strings.Repeat("f", 64)); err == nil {
		t.Fatal("receipt decoder accepted mismatched current ContractHash")
	}
}
