package statev2

import (
	"context"
	"errors"
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
	ctx := context.Background()
	s := NewStore(t.TempDir())
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

func publicationTarget() *PublicationTarget {
	return &PublicationTarget{Host: "github.com", Repository: "app", Key: "issue:1", Resource: "1", Base: "main"}
}

func TestPublicationBeginPersistsFirstGenerationBeforeExternalWrite(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	beforeTasks := snapshot.TaskStates
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{
		Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue,
		Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget(), CompletionRequired: true,
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
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget()})
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
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: hash, PayloadRef: "artifact://approved/1", Target: publicationTarget()})
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
	if p := retried.Publications["intent-1"]; p.Status != PublicationPending || p.Attempts != 3 || p.LastError != "" {
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
	supersede := publicationRequest(t, failedAgain, "publication-supersede", PublicationTransition{Action: PublicationActionSupersede, IntentID: "intent-2", Supersedes: "intent-1", Key: "issue:1", Generation: 2, Kind: PublicationParentIssue, PayloadHash: strings.Repeat("c", 64), PayloadRef: "artifact://approved/2", Target: publicationTarget(), Resolution: &ResolutionEvidence{NotPublished: true, Diagnostic: "confirmed no write"}})
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

func TestPublicationConflictReconciliationPreservesTaskEvidence(t *testing.T) {
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("d", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget()})
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
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("e", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget()})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	fail := publicationRequest(t, pending, "publication-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "no write"})
	failed, err := s.Apply(ctx, fail)
	if err != nil {
		t.Fatal(err)
	}
	advance := publicationRequest(t, failed, "publication-advance", PublicationTransition{Action: PublicationBegin, IntentID: "intent-2", Key: "issue:1", Kind: PublicationParentIssue, Generation: 2, PayloadHash: strings.Repeat("f", 64), PayloadRef: "artifact://approved/2", Target: publicationTarget()})
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
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("1", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget()})
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
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("3", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget()})
	pending, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	unknown := publicationRequest(t, pending, "publication-unknown-stale", PublicationTransition{Action: PublicationBegin, IntentID: "intent-unknown", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("4", 64), PayloadRef: "artifact://approved/unknown", Target: publicationTarget()})
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
	begin := publicationRequest(t, snapshot, "publication-begin", PublicationTransition{Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("5", 64), PayloadRef: "artifact://approved/1", Target: publicationTarget()})
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
