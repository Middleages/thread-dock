package statev2

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

func publicationRequest(t *testing.T, snapshot WorkSnapshot, requestID contractv2.RequestID, transition PublicationTransition) TransitionRequest {
	t.Helper()
	req := TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Publication: &transition}
	hash, err := TransitionPayloadHash(req)
	if err != nil {
		t.Fatal(err)
	}
	req.PayloadHash = hash
	return req
}

func approvedPublicationStore(t *testing.T) (Store, WorkSnapshot) {
	t.Helper()
	return approvedPublicationStoreAt(t, t.TempDir())
}

func approvedPublicationStoreAt(t *testing.T, root string) (Store, WorkSnapshot) {
	t.Helper()
	ctx := context.Background()
	s := NewStore(root)
	snapshot := validSnapshot()
	if _, err := createPlan(s, ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	approve := transitionRequest(t, snapshot, "approve-publication", WorkTransition{Action: WorkApprove, ApprovalRef: "operator", ContractHash: snapshot.ContractHash})
	approved, err := s.Apply(ctx, approve)
	if err != nil {
		t.Fatal(err)
	}
	return s, approved
}

func persistPublicationSnapshotForTest(t *testing.T, root string, snapshot WorkSnapshot) {
	t.Helper()
	if err := writeSnapshot(filepath.Join(root, "v2", "work", string(snapshot.WorkID), "work.json"), snapshot); err != nil {
		t.Fatal(err)
	}
}

func publicationTarget() *PublicationTarget {
	return &PublicationTarget{Host: "github.com", Repository: "app", Key: "issue:1", Resource: "1", Base: "main"}
}

func ptrBool(value bool) *bool { return &value }

func TestPublicationBeginPersistsFirstGenerationBeforeExternalWrite(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	beforeTasks := snapshot.TaskStates
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{
		Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue,
		Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(true),
	})
	got, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	publication := got.Publications["intent-1"]
	if publication.Status != PublicationPending || publication.Generation != 1 || publication.Attempts != 1 || !publication.CompletionRequired {
		t.Fatalf("publication = %#v", publication)
	}
	if len(got.TaskStates) != len(beforeTasks) {
		t.Fatal("publication begin changed task state map")
	}
}

func TestPublicationCompleteRequiresMatchingImmutableReceipt(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	complete := publicationRequest(t, pending, "publication-complete", PublicationTransition{Action: PublicationComplete, IntentID: "intent-1", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "node-1", URL: "https://github.com/org/app/issues/1", PublishedAt: at}})
	completed, err := s.Apply(ctx, complete)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Publications["intent-1"].Status != PublicationCompleted || completed.Publications["intent-1"].Receipt == nil || completed.Publications["intent-1"].Receipt.NodeID != "node-1" {
		t.Fatalf("completed publication = %#v", completed.Publications["intent-1"])
	}
	changed := publicationRequest(t, completed, "publication-complete-changed", PublicationTransition{Action: PublicationComplete, IntentID: "intent-1", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "node-2", URL: "https://github.com/org/app/issues/1", PublishedAt: at}})
	if _, err := s.Apply(ctx, changed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("changed receipt error = %v, want invalid transition", err)
	}
}

func TestPublicationFailureRetryAndSupersedePreserveTaskEvidence(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	hash := strings.Repeat("b", 64)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: hash, PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "timeout"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	beforeTasks := failed.TaskStates
	retry := publicationRequest(t, failed, "publication-retry", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1})
	retried, err := s.Apply(ctx, retry)
	if err != nil {
		t.Fatal(err)
	}
	if p := retried.Publications["intent-1"]; p.Status != PublicationPending || p.Attempts != 2 || p.LastError != "" {
		t.Fatalf("retry publication = %#v", p)
	}
	if !reflect.DeepEqual(beforeTasks, retried.TaskStates) {
		t.Fatal("publication retry changed task evidence")
	}

	failAgain := publicationRequest(t, retried, "publication-fail-again", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "confirmed no write"})
	failedAgain, err := s.Apply(ctx, failAgain)
	if err != nil {
		t.Fatal(err)
	}
	supersede := publicationRequest(t, failedAgain, "publication-supersede", PublicationTransition{Action: PublicationActionSupersede, IntentID: "intent-2", Supersedes: "intent-1", Key: "issue:1", Generation: 2, Kind: PublicationParentIssue, PayloadHash: strings.Repeat("c", 64), PayloadRef: "artifact://approved/2", Target: publicationTarget(), CompletionRequired: ptrBool(false), Resolution: &ResolutionEvidence{NotPublished: true, Diagnostic: "confirmed no write"}})
	got, err := s.Apply(ctx, supersede)
	if err != nil {
		t.Fatal(err)
	}
	if got.Publications["intent-1"].Status != PublicationSuperseded || got.Publications["intent-2"].Status != PublicationPending {
		t.Fatalf("supersede publications = %#v", got.Publications)
	}
	if !reflect.DeepEqual(beforeTasks, got.TaskStates) {
		t.Fatal("supersede changed task evidence")
	}
	lateRetry := publicationRequest(t, got, "publication-late-retry", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1})
	if _, err := s.Apply(ctx, lateRetry); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("late retry error = %v, want ErrStaleGeneration", err)
	}
}

func TestUnknownNewPublicationStalePrecedesMissingCompletionHint(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	unknown := publicationRequest(t, pending, "publication-unknown-missing-hint", PublicationTransition{Action: PublicationBegin, IntentID: "intent-unknown", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://approved/unknown", Target: publicationTarget()})
	if _, err := s.Apply(ctx, unknown); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("unknown stale begin error = %v, want ErrStaleGeneration", err)
	}
	current, err := s.Load(ctx, pending.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != pending.Revision || !reflect.DeepEqual(current.Publications, pending.Publications) {
		t.Fatalf("unknown stale begin wrote state: %#v", current)
	}
}

func TestSupersedeMissingCompletionHintIsInvalidWithoutPanicOrWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, snapshot := approvedPublicationStoreAt(t, root)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("c", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "no write"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	before := failed
	supersede := publicationRequest(t, failed, "publication-supersede-missing-hint", PublicationTransition{Action: PublicationActionSupersede, IntentID: "intent-2", Supersedes: "intent-1", Key: "issue:1", Generation: 2, Kind: PublicationParentIssue, PayloadHash: strings.Repeat("d", 64), PayloadRef: "artifact://approved/2", Target: publicationTarget(), Resolution: &ResolutionEvidence{NotPublished: true, Diagnostic: "confirmed no write"}})
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("supersede panicked: %v", recovered)
			}
		}()
		if _, err := s.Apply(ctx, supersede); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("missing completion hint error = %v, want ErrInvalidTransition", err)
		}
	}()
	after, err := s.Load(ctx, before.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || !reflect.DeepEqual(after.Publications, before.Publications) {
		t.Fatalf("invalid supersede wrote state: before=%#v after=%#v", before, after)
	}
}

func TestPublicationAttemptsCountDispatchesOnlyAndRetryOverflowIsNoWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, snapshot := approvedPublicationStoreAt(t, root)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("e", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "timeout"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Publications["intent-1"].Attempts != 1 {
		t.Fatalf("failure counted as dispatch: %#v", failed.Publications["intent-1"])
	}
	retry := publicationRequest(t, failed, "publication-retry", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1})
	retried, err := s.Apply(ctx, retry)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Publications["intent-1"].Attempts != 2 {
		t.Fatalf("retry attempts = %d, want 2", retried.Publications["intent-1"].Attempts)
	}
	mutated := retried
	p := mutated.Publications["intent-1"]
	p.Status = PublicationFailed
	p.Attempts = ^uint32(0)
	p.LastError = "exhausted"
	mutated.Publications["intent-1"] = p
	persistPublicationSnapshotForTest(t, root, mutated)
	before := mutated
	overflow := publicationRequest(t, before, "publication-retry-overflow", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1})
	if _, err := s.Apply(ctx, overflow); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("overflow retry error = %v, want ErrInvalidTransition", err)
	}
	after, err := s.Load(ctx, before.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.Publications["intent-1"].Attempts != ^uint32(0) || after.Publications["intent-1"].Status != PublicationFailed {
		t.Fatalf("overflow retry wrote state: %#v", after)
	}
}

func TestPublicationSettlementPreservesUnrelatedBlocker(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		action PublicationAction
	}{
		{name: "complete", action: PublicationComplete},
		{name: "fail", action: PublicationFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			s, snapshot := approvedPublicationStoreAt(t, root)
			begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("f", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
			pending, err := s.Apply(ctx, begin)
			if err != nil {
				t.Fatal(err)
			}
			pending.State = StateNeedsOperator
			pending.NextAction = "resolve"
			pending.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskNeedsOperator, RepairLimit: DefaultRepairLimit, RecoveryLimit: DefaultRecoveryLimit, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{}}
			pending.Control.Blocker = &OperatorBlocker{Kind: BlockerKindEvidenceMismatch, OperatorRef: "task-operator", TaskID: "task-1", Diagnostic: "unrelated task blocker"}
			persistPublicationSnapshotForTest(t, root, pending)
			before := pending
			var tr PublicationTransition
			switch tc.action {
			case PublicationComplete:
				tr = PublicationTransition{Action: tc.action, IntentID: "intent-1", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 5, 6, 7, 0, time.UTC)}}
			case PublicationFail:
				tr = PublicationTransition{Action: tc.action, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "provider unavailable"}
			}
			got, err := s.Apply(ctx, publicationRequest(t, before, "publication-settle", tr))
			if err != nil {
				t.Fatal(err)
			}
			if got.State != StateNeedsOperator || !reflect.DeepEqual(got.Control.Blocker, before.Control.Blocker) || got.TaskStates["task-1"].Status != TaskNeedsOperator {
				t.Fatalf("unrelated blocker was not preserved: %#v", got)
			}
			wantSync := "synced"
			if tc.action == PublicationFail {
				wantSync = "failed"
			}
			if got.SyncStatus != wantSync {
				t.Fatalf("sync status = %q, want %q", got.SyncStatus, wantSync)
			}
		})
	}
}

func TestPublicationConflictCannotOverwriteUnrelatedBlocker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, snapshot := approvedPublicationStoreAt(t, root)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("0", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	pending.State = StateNeedsOperator
	pending.NextAction = "resolve"
	pending.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskNeedsOperator, RepairLimit: DefaultRepairLimit, RecoveryLimit: DefaultRecoveryLimit, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{}}
	pending.Control.Blocker = &OperatorBlocker{Kind: BlockerKindEvidenceMismatch, OperatorRef: "task-operator", TaskID: "task-1", Diagnostic: "unrelated task blocker"}
	persistPublicationSnapshotForTest(t, root, pending)
	request := publicationRequest(t, pending, "publication-conflict", PublicationTransition{Action: PublicationActionConflict, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "ambiguous", Blocker: &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "publication-operator", IntentID: "intent-1", Diagnostic: "ambiguous"}})
	if _, err := s.Apply(ctx, request); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("conflict error = %v, want ErrInvalidTransition", err)
	}
	after, err := s.Load(ctx, pending.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != pending.Revision || !reflect.DeepEqual(after.Control.Blocker, pending.Control.Blocker) || after.Publications["intent-1"].Status != PublicationPending {
		t.Fatalf("conflict overwrote unrelated blocker: %#v", after)
	}
}

func TestPublicationReconciliationRequiresExactEvidenceAndReceiptShape(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		eval    *ResolutionEvidence
		receipt *PublicationReceipt
	}{
		{name: "both", eval: &ResolutionEvidence{RemoteMatch: true, NotPublished: true}, receipt: &PublicationReceipt{NodeID: "node", PublishedAt: time.Date(2026, 9, 8, 6, 7, 8, 0, time.UTC)}},
		{name: "neither", eval: &ResolutionEvidence{}, receipt: nil},
		{name: "missing receipt", eval: &ResolutionEvidence{RemoteMatch: true}, receipt: nil},
		{name: "invalid receipt", eval: &ResolutionEvidence{RemoteMatch: true}, receipt: &PublicationReceipt{PublishedAt: time.Date(2026, 9, 8, 6, 7, 8, 0, time.UTC)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, snapshot := approvedPublicationStore(t)
			begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("1", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
			pending, err := s.Apply(ctx, begin)
			if err != nil {
				t.Fatal(err)
			}
			conflict := publicationRequest(t, pending, "publication-conflict", PublicationTransition{Action: PublicationActionConflict, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "ambiguous", Blocker: &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"}})
			blocked, err := s.Apply(ctx, conflict)
			if err != nil {
				t.Fatal(err)
			}
			resolve := WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolvePublicationReconciled, OperatorRef: "operator", IntentID: "intent-1", Evidence: tc.eval, PublicationReceipt: tc.receipt}}
			if _, err := s.Apply(ctx, transitionRequest(t, blocked, "publication-reconcile-invalid", resolve)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("reconciliation error = %v, want ErrInvalidTransition", err)
			}
			after, err := s.Load(ctx, blocked.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != blocked.Revision || after.Publications["intent-1"].Status != PublicationConflict || after.Control.Blocker == nil {
				t.Fatalf("invalid reconciliation wrote state: %#v", after)
			}
		})
	}
}

func TestReducerPublicationMatrixPreservesIntegratedExecutionProjection(t *testing.T) {
	receipt := &PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 7, 8, 9, 0, time.UTC)}
	cases := []struct {
		name               string
		status             PublicationStatus
		completionRequired bool
		lastError          string
		wantState          WorkState
		wantNext           string
		wantSync           string
	}{
		{name: "ordinary pending", status: PublicationPending, wantState: StateReadyForPR, wantNext: "prepare_docs", wantSync: "pending"},
		{name: "ordinary failed", status: PublicationFailed, lastError: "failed", wantState: StateReadyForPR, wantNext: "prepare_docs", wantSync: "failed"},
		{name: "required pending", status: PublicationPending, completionRequired: true, wantState: StatePublicationPending, wantNext: "publish", wantSync: "pending"},
		{name: "required failed", status: PublicationFailed, completionRequired: true, lastError: "failed", wantState: StatePublicationPending, wantNext: "publish", wantSync: "failed"},
		{name: "required completed", status: PublicationCompleted, completionRequired: true, wantState: StateReadyForPR, wantNext: "prepare_docs", wantSync: "synced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSnapshot()
			s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval"}
			s.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskIntegrated}
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": {IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: PublicationParentIssue, Status: tc.status, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one", Target: *publicationTarget(), Attempts: 1, LastError: tc.lastError, CompletionRequired: tc.completionRequired, Receipt: receipt}}
			if tc.status != PublicationCompleted {
				publication := s.Publications["intent-1"]
				publication.Receipt = nil
				s.Publications["intent-1"] = publication
			}
			reduce(&s)
			if s.State != tc.wantState || s.NextAction != tc.wantNext || s.SyncStatus != tc.wantSync {
				t.Fatalf("reducer projection = %q/%q/%q, want %q/%q/%q", s.State, s.NextAction, s.SyncStatus, tc.wantState, tc.wantNext, tc.wantSync)
			}
		})
	}
}

func TestPublicationReplayPayloadConflictAndStaleCASAreNoWrite(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("2", 64), PayloadRef: "artifact://one", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	first, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Apply(ctx, begin)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("publication replay = %#v, err=%v", replay, err)
	}
	changed := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("2", 64), PayloadRef: "artifact://changed", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	if _, err := s.Apply(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed publication replay error = %v, want ErrConflict", err)
	}
	stale := publicationRequest(t, snapshot, "publication-stale-cas", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "stale"})
	if _, err := s.Apply(ctx, stale); !errors.As(err, new(*StaleRevisionError)) {
		t.Fatalf("stale publication CAS error = %v", err)
	}
	after, err := s.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != first.Revision || after.Publications["intent-1"].Status != PublicationPending {
		t.Fatalf("replay/conflict/CAS mutated publication: %#v", after)
	}
}

func TestPublicationConflictReconciliationPreservesTaskEvidence(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("d", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	beforeTasks := pending.TaskStates
	conflict := publicationRequest(t, pending, "publication-conflict", PublicationTransition{Action: PublicationActionConflict, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "ambiguous remote result", Blocker: &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator-1", IntentID: "intent-1", Diagnostic: "ambiguous remote result"}})
	blocked, err := s.Apply(ctx, conflict)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != StateNeedsOperator || blocked.Publications["intent-1"].Status != PublicationConflict {
		t.Fatalf("blocked = %#v", blocked)
	}
	if !reflect.DeepEqual(beforeTasks, blocked.TaskStates) {
		t.Fatal("publication conflict changed task evidence")
	}
	reconcile := WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolvePublicationReconciled, OperatorRef: "operator-1", IntentID: "intent-1", Evidence: &ResolutionEvidence{NotPublished: true, Diagnostic: "provider confirms no publication"}}}
	resolvedReq := transitionRequest(t, blocked, "publication-reconcile", reconcile)
	resolved, err := s.Apply(ctx, resolvedReq)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Publications["intent-1"].Status != PublicationFailed || resolved.Control.Blocker != nil {
		t.Fatalf("resolved = %#v", resolved)
	}
	if !reflect.DeepEqual(beforeTasks, resolved.TaskStates) {
		t.Fatal("reconciliation changed task evidence")
	}
}

func TestPublicationCannotAdvanceNewGenerationFromFailedWithoutSupersede(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("e", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "no write"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	advance := publicationRequest(t, failed, "publication-advance", PublicationTransition{Action: PublicationBegin, IntentID: "intent-2", Key: "issue:1", Kind: PublicationParentIssue, Generation: 2, PayloadHash: strings.Repeat("f", 64), PayloadRef: "artifact://approved/2", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	if _, err := s.Apply(ctx, advance); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("failed generation advance error = %v, want invalid transition", err)
	}
	current, err := s.Load(ctx, failed.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != failed.Revision || len(current.Publications) != 1 || current.Publications["intent-1"].Status != PublicationFailed {
		t.Fatalf("failed generation advance wrote state: %#v", current)
	}
}

func TestPublicationSettlingWhilePausedUpdatesSyncWithoutResumingWork(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("1", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	pausedReq := transitionRequest(t, pending, "publication-pause", WorkTransition{Action: WorkPause})
	paused, err := s.Apply(ctx, pausedReq)
	if err != nil {
		t.Fatal(err)
	}
	if paused.State != StatePaused || !paused.Control.PauseRequested {
		t.Fatalf("paused = %#v", paused)
	}
	fail := publicationRequest(t, paused, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "provider unavailable"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != StatePaused || failed.SyncStatus != "failed" || !failed.Control.PauseRequested {
		t.Fatalf("settled paused publication = %#v", failed)
	}
}

func TestValidateSnapshotRejectsMultiplePublicationConflicts(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control.ApprovedContractHash = snapshot.ContractHash
	base := PublicationState{Key: "issue:1", Kind: PublicationParentIssue, PayloadHash: strings.Repeat("2", 64), PayloadRef: "artifact://approved/1", Target: *publicationTarget(), Attempts: 1, LastError: "ambiguous"}
	first := base
	first.IntentID, first.Generation, first.Status = "intent-1", 1, PublicationConflict
	second := base
	second.IntentID, second.Generation, second.Status = "intent-2", 2, PublicationConflict
	snapshot.Publications = map[PublicationIntentID]PublicationState{"intent-1": first, "intent-2": second}
	snapshot.Control.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"}
	if err := validateSnapshot(snapshot); err == nil {
		t.Fatal("validateSnapshot accepted multiple publication conflicts")
	}
}

func TestUnknownPublicationBeginAtExistingGenerationIsStaleBeforeStatusGuard(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("3", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	unknown := publicationRequest(t, pending, "publication-unknown-stale", PublicationTransition{Action: PublicationBegin, IntentID: "intent-unknown", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("4", 64), PayloadRef: "artifact://approved/unknown", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	if _, err := s.Apply(ctx, unknown); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("unknown stale begin error = %v, want ErrStaleGeneration", err)
	}
	current, err := s.Load(ctx, pending.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != pending.Revision || len(current.Publications) != 1 {
		t.Fatalf("unknown stale begin wrote state: %#v", current)
	}
}

func TestPublicationCompletionClearsSyncPendingWhilePaused(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("5", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	pausedReq := transitionRequest(t, pending, "publication-pause", WorkTransition{Action: WorkPause})
	paused, err := s.Apply(ctx, pausedReq)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 8, 2, 3, 4, 0, time.UTC)
	complete := publicationRequest(t, paused, "publication-complete", PublicationTransition{Action: PublicationComplete, IntentID: "intent-1", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "node-1", PublishedAt: at}})
	completed, err := s.Apply(ctx, complete)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != StatePaused || completed.SyncStatus != "synced" || !completed.Control.PauseRequested || completed.Publications["intent-1"].Status != PublicationCompleted {
		t.Fatalf("paused completion = %#v", completed)
	}
}

func TestCompletionRequiredHintIsPreservedAcrossRetryAndSettlement(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("6", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(true)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, CompletionRequired: ptrBool(true), Diagnostic: "temporary failure"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	retry := publicationRequest(t, failed, "publication-retry", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Generation: 1, CompletionRequired: ptrBool(true)})
	retried, err := s.Apply(ctx, retry)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 8, 3, 4, 5, 0, time.UTC)
	complete := publicationRequest(t, retried, "publication-complete", PublicationTransition{Action: PublicationComplete, IntentID: "intent-1", Key: "issue:1", Generation: 1, CompletionRequired: ptrBool(true), Receipt: &PublicationReceipt{NodeID: "node-1", PublishedAt: at}})
	completed, err := s.Apply(ctx, complete)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Publications["intent-1"].CompletionRequired || completed.Publications["intent-1"].Status != PublicationCompleted {
		t.Fatalf("completion-required publication = %#v", completed.Publications["intent-1"])
	}
}

func TestCompletionRequiredFalseHintCannotMutatePendingIntent(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("7", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(true)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail-mismatch", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, CompletionRequired: ptrBool(false), Diagnostic: "must reject"})
	if _, err := s.Apply(ctx, fail); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("mismatched completion hint error = %v", err)
	}
	current, err := s.Load(ctx, pending.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != pending.Revision || current.Publications["intent-1"].Status != PublicationPending {
		t.Fatalf("mismatched completion hint wrote state: %#v", current)
	}
}

func TestCompletedGenerationAdvancesAndLateActionsAreStaleNoWrites(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("8", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 8, 4, 5, 6, 0, time.UTC)
	complete := publicationRequest(t, pending, "publication-complete", PublicationTransition{Action: PublicationComplete, IntentID: "intent-1", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "node-1", PublishedAt: at}})
	completed, err := s.Apply(ctx, complete)
	if err != nil {
		t.Fatal(err)
	}
	second := publicationRequest(t, completed, "publication-begin-2", PublicationTransition{Action: PublicationBegin, IntentID: "intent-2", Key: "issue:1", Kind: PublicationParentIssue, Generation: 2, PayloadHash: strings.Repeat("9", 64), PayloadRef: "artifact://approved/2", Target: publicationTarget(), CompletionRequired: ptrBool(false)})
	current, err := s.Apply(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := current.Revision
	for _, transition := range []PublicationTransition{
		{Action: PublicationComplete, IntentID: "intent-1", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "late", PublishedAt: at}},
		{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "late"},
		{Action: PublicationActionConflict, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "late", Blocker: &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "late"}},
	} {
		req := publicationRequest(t, current, contractv2.RequestID("late-")+contractv2.RequestID(transition.Action), transition)
		if _, err := s.Apply(ctx, req); !errors.Is(err, ErrStaleGeneration) {
			t.Fatalf("late %s error = %v", transition.Action, err)
		}
	}
	after, err := s.Load(ctx, current.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != beforeRevision || after.Publications["intent-1"].Status != PublicationCompleted || after.Publications["intent-2"].Status != PublicationPending {
		t.Fatalf("late actions wrote state: %#v", after)
	}
}
