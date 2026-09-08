package statev2

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func round5Publication(t *testing.T, status PublicationStatus) PublicationState {
	t.Helper()
	p := PublicationState{
		IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: PublicationParentIssue,
		Status: status, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one",
		Target: *publicationTarget(), Attempts: 1,
	}
	switch status {
	case PublicationCompleted:
		p.Receipt = &PublicationReceipt{NodeID: "node-1", PublishedAt: time.Date(2026, 9, 8, 8, 9, 10, 0, time.UTC)}
	case PublicationFailed, PublicationConflict:
		p.LastError = "publication failed"
	}
	return p
}

func round5PersistAndLoadError(t *testing.T, mutate func(*WorkSnapshot)) error {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	s, snapshot := approvedPublicationStoreAt(t, root)
	mutate(&snapshot)
	persistPublicationSnapshotForTest(t, root, snapshot)
	_, err := s.Load(ctx, snapshot.WorkID)
	return err
}

func TestRound5LoadRejectsIsolatedPublicationStatusShapes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*WorkSnapshot)
		want   string
	}{
		{name: "pending receipt", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationPending)}
			p := s.Publications["intent-1"]
			p.Receipt = &PublicationReceipt{NodeID: "node", PublishedAt: time.Date(2026, 9, 8, 8, 9, 10, 0, time.UTC)}
			s.Publications["intent-1"] = p
		}, want: "pending publication shape"},
		{name: "completed missing receipt", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationCompleted)}
			p := s.Publications["intent-1"]
			p.Receipt = nil
			s.Publications["intent-1"] = p
		}, want: "completed publication shape"},
		{name: "failed missing error", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationFailed)}
			p := s.Publications["intent-1"]
			p.LastError = ""
			s.Publications["intent-1"] = p
		}, want: "failed publication shape"},
		{name: "conflict receipt", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationConflict)}
			p := s.Publications["intent-1"]
			p.Receipt = &PublicationReceipt{NodeID: "node", PublishedAt: time.Date(2026, 9, 8, 8, 9, 10, 0, time.UTC)}
			s.Publications["intent-1"] = p
			s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"}
		}, want: "conflict publication shape"},
		{name: "superseded without newer", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationSuperseded)}
		}, want: "has no newer generation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := round5PersistAndLoadError(t, tc.mutate)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want diagnostic containing %q", err, tc.want)
			}
		})
	}
}

func TestRound5LoadRejectsIsolatedPublicationIdentityAndGenerationCorruption(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*WorkSnapshot)
		want   string
	}{
		{name: "missing payload hash", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationPending)}
			p := s.Publications["intent-1"]
			p.PayloadHash = ""
			s.Publications["intent-1"] = p
		}, want: "invalid publication payload identity"},
		{name: "missing payload reference", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationPending)}
			p := s.Publications["intent-1"]
			p.PayloadRef = ""
			s.Publications["intent-1"] = p
		}, want: "invalid publication payload identity"},
		{name: "invalid target", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationPending)}
			p := s.Publications["intent-1"]
			p.Target.Key = "issue:2"
			s.Publications["intent-1"] = p
		}, want: "invalid publication target"},
		{name: "noncontiguous generation", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationPending)}
			p := s.Publications["intent-1"]
			p.Generation = 2
			s.Publications["intent-1"] = p
		}, want: "non-contiguous generations"},
		{name: "duplicate key generation", mutate: func(s *WorkSnapshot) {
			p := round5Publication(t, PublicationPending)
			s.Publications = map[PublicationIntentID]PublicationState{
				"intent-1": p,
				"intent-2": {IntentID: "intent-2", Key: p.Key, Generation: p.Generation, Kind: p.Kind, Status: PublicationPending, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://two", Target: *publicationTarget(), Attempts: 1},
			}
		}, want: "duplicates key generation"},
		{name: "superseded without newer", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationSuperseded)}
		}, want: "has no newer generation"},
		{name: "multiple conflicts", mutate: func(s *WorkSnapshot) {
			first := round5Publication(t, PublicationConflict)
			second := round5Publication(t, PublicationConflict)
			second.IntentID, second.Generation = "intent-2", 2
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": first, "intent-2": second}
			s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"}
		}, want: "multiple publication conflicts"},
		{name: "unmatched conflict", mutate: func(s *WorkSnapshot) {
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": round5Publication(t, PublicationConflict)}
			s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "other", Diagnostic: "ambiguous"}
		}, want: "publication conflict requires matching blocker"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := round5PersistAndLoadError(t, tc.mutate)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want diagnostic containing %q", err, tc.want)
			}
		})
	}
}

func round5SeedPublication(t *testing.T, status PublicationStatus) (Store, WorkSnapshot) {
	t.Helper()
	ctx := context.Background()
	s, snapshot := approvedPublicationStore(t)
	begin := publicationRequest(t, snapshot, "round5-begin", PublicationTransition{
		Action: PublicationBegin, IntentID: "intent-1", Key: "issue:1", Kind: PublicationParentIssue,
		Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one", Target: publicationTarget(), CompletionRequired: ptrBool(false),
	})
	current, err := s.Apply(ctx, begin)
	if err != nil {
		t.Fatal(err)
	}
	if status == PublicationFailed {
		failed, err := s.Apply(ctx, publicationRequest(t, current, "round5-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "failed"}))
		if err != nil {
			t.Fatal(err)
		}
		current = failed
	}
	return s, current
}

func round5ActionTransition(action PublicationAction) PublicationTransition {
	t := PublicationTransition{
		Action: action, IntentID: "intent-1", Key: "issue:1", Generation: 1,
		Kind: PublicationParentIssue, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one",
		Target: publicationTarget(), CompletionRequired: ptrBool(false),
	}
	switch action {
	case PublicationComplete:
		t.Receipt = &PublicationReceipt{NodeID: "node", PublishedAt: time.Date(2026, 9, 8, 9, 10, 11, 0, time.UTC)}
	case PublicationFail:
		t.Diagnostic = "failed"
	case PublicationActionConflict:
		t.Diagnostic = "ambiguous"
		t.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"}
	case PublicationBegin:
		// Retry uses the existing identity and does not need immutable fields.
	case PublicationActionSupersede:
		t.IntentID, t.Generation, t.PayloadHash, t.PayloadRef = "intent-2", 2, strings.Repeat("b", 64), "artifact://two"
		t.Supersedes = "intent-1"
		t.Resolution = &ResolutionEvidence{NotPublished: true, Diagnostic: "not published"}
	}
	return t
}

func TestRound5ImmutableIdentityGuardsAreReachedForEveryPublicationAction(t *testing.T) {
	fields := []struct {
		name   string
		mutate func(*PublicationTransition)
	}{
		{name: "kind", mutate: func(t *PublicationTransition) { t.Kind = PublicationChildIssue }},
		{name: "payload hash", mutate: func(t *PublicationTransition) { t.PayloadHash = strings.Repeat("c", 64) }},
		{name: "payload ref", mutate: func(t *PublicationTransition) { t.PayloadRef = "artifact://different" }},
		{name: "target", mutate: func(t *PublicationTransition) { target := *t.Target; target.Resource = "different"; t.Target = &target }},
		{name: "completion required", mutate: func(t *PublicationTransition) { t.CompletionRequired = ptrBool(true) }},
		{name: "key", mutate: func(t *PublicationTransition) { t.Key = "issue:2" }},
		{name: "generation", mutate: func(t *PublicationTransition) { t.Generation = 2 }},
		{name: "intent ID", mutate: func(t *PublicationTransition) { t.IntentID = "intent-other" }},
	}
	actions := []struct {
		name   string
		action PublicationAction
		seed   PublicationStatus
	}{
		{name: "retry", action: PublicationBegin, seed: PublicationFailed},
		{name: "complete", action: PublicationComplete, seed: PublicationPending},
		{name: "fail", action: PublicationFail, seed: PublicationPending},
		{name: "conflict", action: PublicationActionConflict, seed: PublicationPending},
		{name: "supersede", action: PublicationActionSupersede, seed: PublicationFailed},
	}
	for _, action := range actions {
		for _, field := range fields {
			t.Run(action.name+"/"+field.name, func(t *testing.T) {
				ctx := context.Background()
				s, snapshot := round5SeedPublication(t, action.seed)
				tr := round5ActionTransition(action.action)
				if action.action == PublicationActionSupersede {
					switch field.name {
					case "kind":
						tr.Kind = PublicationKind("unsupported")
					case "payload hash":
						tr.PayloadHash = "not-a-sha256"
					case "payload ref":
						tr.PayloadRef = ""
					case "completion required":
						tr.CompletionRequired = nil
					case "intent ID":
						tr.IntentID = "intent-1" // The new ID may not alias Supersedes.
					case "generation":
						tr.Generation = 3
					default:
						field.mutate(&tr)
					}
				} else {
					field.mutate(&tr)
				}
				before, err := s.Load(ctx, snapshot.WorkID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.Apply(ctx, publicationRequest(t, before, "round5-immutable", tr)); !errors.Is(err, ErrInvalidTransition) && !errors.Is(err, ErrStaleGeneration) {
					t.Fatalf("immutable %s error = %v", field.name, err)
				}
				after, err := s.Load(ctx, before.WorkID)
				if err != nil {
					t.Fatal(err)
				}
				if after.Revision != before.Revision || !reflect.DeepEqual(after.Publications, before.Publications) {
					t.Fatalf("immutable %s wrote state: before=%#v after=%#v", field.name, before.Publications, after.Publications)
				}
			})
		}
	}
}

func round5MakeUnrelatedBlocker(snapshot *WorkSnapshot) {
	task := snapshot.TaskStates["task-1"]
	task.Status = TaskNeedsOperator
	snapshot.TaskStates["task-1"] = task
	snapshot.Control.Blocker = &OperatorBlocker{Kind: BlockerKindEvidenceMismatch, OperatorRef: "task-operator", TaskID: "task-1", Diagnostic: "unrelated"}
	snapshot.State, snapshot.NextAction = StateNeedsOperator, "resolve"
}

func TestRound5BeginAndSupersedeAreBlockedByPauseOrUnrelatedBlocker(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		seed   PublicationStatus
		action PublicationAction
		mutate func(*WorkSnapshot)
	}{
		{name: "begin paused", seed: PublicationPending, action: PublicationBegin, mutate: func(s *WorkSnapshot) {
			s.Control.PauseRequested = true
			s.State = StatePaused
		}},
		{name: "begin unrelated blocker", seed: PublicationPending, action: PublicationBegin, mutate: round5MakeUnrelatedBlocker},
		{name: "supersede paused", seed: PublicationFailed, action: PublicationActionSupersede, mutate: func(s *WorkSnapshot) {
			s.Control.PauseRequested = true
			s.State = StatePaused
		}},
		{name: "supersede unrelated blocker", seed: PublicationFailed, action: PublicationActionSupersede, mutate: round5MakeUnrelatedBlocker},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			s, snapshot := approvedPublicationStoreAt(t, root)
			if tc.seed == PublicationPending {
				begin := publicationRequest(t, snapshot, "round5-control-seed", round5ActionTransition(PublicationBegin))
				var err error
				snapshot, err = s.Apply(ctx, begin)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var err error
				begin := publicationRequest(t, snapshot, "round5-control-seed", round5ActionTransition(PublicationBegin))
				snapshot, err = s.Apply(ctx, begin)
				if err != nil {
					t.Fatal(err)
				}
				fail := publicationRequest(t, snapshot, "round5-control-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: "failed"})
				snapshot, err = s.Apply(ctx, fail)
				if err != nil {
					t.Fatal(err)
				}
			}
			tc.mutate(&snapshot)
			before := snapshot
			persistPublicationSnapshotForTest(t, root, snapshot)
			tr := round5ActionTransition(tc.action)
			if tc.action == PublicationBegin && tc.seed == PublicationPending {
				tr.IntentID = "intent-2"
				tr.Generation = 2
				tr.PayloadHash = strings.Repeat("b", 64)
				tr.PayloadRef = "artifact://two"
			}
			if _, err := s.Apply(ctx, publicationRequest(t, snapshot, "round5-control-action", tr)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("control guard error = %v, want invalid transition", err)
			}
			after, err := s.Load(ctx, before.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision || !reflect.DeepEqual(after.Publications, before.Publications) || !reflect.DeepEqual(after.Control, before.Control) {
				t.Fatalf("control guard wrote state: before=%#v after=%#v", before, after)
			}
		})
	}
}

func round5Conflict(t *testing.T) (Store, WorkSnapshot, WorkSnapshot) {
	t.Helper()
	ctx := context.Background()
	s, snapshot := round5SeedPublication(t, PublicationPending)
	conflict := PublicationTransition{
		Action: PublicationActionConflict, IntentID: "intent-1", Key: "issue:1", Generation: 1,
		Diagnostic: "ambiguous", Blocker: &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"},
	}
	blocked, err := s.Apply(ctx, publicationRequest(t, snapshot, "round5-conflict", conflict))
	if err != nil {
		t.Fatal(err)
	}
	return s, snapshot, blocked
}

func TestRound5PublicationReconciliationRemoteMatchAndEvidenceMismatches(t *testing.T) {
	ctx := context.Background()
	t.Run("remote match completes and preserves tasks", func(t *testing.T) {
		s, beforeTasks, blocked := round5Conflict(t)
		receipt := &PublicationReceipt{NodeID: "node-1", URL: "https://example.test/issue/1", PublishedAt: time.Date(2026, 9, 8, 10, 11, 12, 0, time.UTC)}
		resolve := WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolvePublicationReconciled, OperatorRef: "operator", IntentID: "intent-1", Evidence: &ResolutionEvidence{RemoteMatch: true}, PublicationReceipt: receipt}}
		got, err := s.Apply(ctx, transitionRequest(t, blocked, "round5-reconcile-remote", resolve))
		if err != nil {
			t.Fatal(err)
		}
		if got.Publications["intent-1"].Status != PublicationCompleted || got.Control.Blocker != nil || got.SyncStatus != "synced" {
			t.Fatalf("remote reconciliation = %#v", got)
		}
		if !reflect.DeepEqual(got.TaskStates, beforeTasks.TaskStates) {
			t.Fatal("remote reconciliation changed task states")
		}
	})

	cases := []struct {
		name   string
		mutate func(*ResolvePayload)
	}{
		{name: "operator mismatch", mutate: func(p *ResolvePayload) { p.OperatorRef = "other" }},
		{name: "intent mismatch", mutate: func(p *ResolvePayload) { p.IntentID = "other" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, blocked := round5Conflict(t)
			payload := &ResolvePayload{Kind: ResolvePublicationReconciled, OperatorRef: "operator", IntentID: "intent-1", Evidence: &ResolutionEvidence{RemoteMatch: true}, PublicationReceipt: &PublicationReceipt{NodeID: "node", PublishedAt: time.Date(2026, 9, 8, 10, 11, 12, 0, time.UTC)}}
			tc.mutate(payload)
			before := blocked
			_, err := s.Apply(ctx, transitionRequest(t, blocked, "round5-reconcile-mismatch", WorkTransition{Action: WorkResolve, Resolve: payload}))
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("reconciliation error = %v", err)
			}
			after, err := s.Load(ctx, blocked.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision || !reflect.DeepEqual(after.Publications, before.Publications) || !reflect.DeepEqual(after.Control, before.Control) {
				t.Fatalf("mismatched reconciliation wrote state: %#v", after)
			}
		})
	}
}

func TestRound5PublicationReconciliationRejectsStaleGeneration(t *testing.T) {
	ctx := context.Background()
	s, failed := round5SeedPublication(t, PublicationFailed)
	superseded, err := s.Apply(ctx, publicationRequest(t, failed, "round5-supersede", round5ActionTransition(PublicationActionSupersede)))
	if err != nil {
		t.Fatal(err)
	}
	conflict := PublicationTransition{
		Action: PublicationActionConflict, IntentID: "intent-2", Key: "issue:1", Generation: 2,
		Diagnostic: "ambiguous", Blocker: &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-2", Diagnostic: "ambiguous"},
	}
	blocked, err := s.Apply(ctx, publicationRequest(t, superseded, "round5-conflict-current", conflict))
	if err != nil {
		t.Fatal(err)
	}
	before := blocked
	resolve := WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolvePublicationReconciled, OperatorRef: "operator", IntentID: "intent-1", Evidence: &ResolutionEvidence{NotPublished: true, Diagnostic: "old generation"}}}
	if _, err := s.Apply(ctx, transitionRequest(t, blocked, "round5-reconcile-stale", resolve)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("stale reconciliation error = %v, want invalid transition", err)
	}
	after, err := s.Load(ctx, blocked.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || !reflect.DeepEqual(after.Publications, before.Publications) || !reflect.DeepEqual(after.Control, before.Control) {
		t.Fatalf("stale reconciliation wrote state: %#v", after)
	}
}

func TestRound5PublicationLastErrorIsValidUTF8AtByteBoundary(t *testing.T) {
	ctx := context.Background()
	s, snapshot := round5SeedPublication(t, PublicationPending)
	diagnostic := strings.Repeat("界", 341) + "x" // 1024 UTF-8 bytes, ending on a rune boundary.
	if len([]byte(diagnostic)) != MaxDiagnosticBytes || !utf8.ValidString(diagnostic) {
		t.Fatalf("test diagnostic bytes=%d valid=%v", len([]byte(diagnostic)), utf8.ValidString(diagnostic))
	}
	failed, err := s.Apply(ctx, publicationRequest(t, snapshot, "round5-utf8-fail", PublicationTransition{Action: PublicationFail, IntentID: "intent-1", Key: "issue:1", Generation: 1, Diagnostic: diagnostic}))
	if err != nil {
		t.Fatal(err)
	}
	persisted := failed.Publications["intent-1"].LastError
	if len([]byte(persisted)) > MaxDiagnosticBytes || !utf8.ValidString(persisted) || persisted != diagnostic {
		t.Fatalf("persisted error bytes=%d valid=%v equal=%v", len([]byte(persisted)), utf8.ValidString(persisted), persisted == diagnostic)
	}
}

func TestRound5ReducerPreservesControlPriorityWhileProjectingPublicationSync(t *testing.T) {
	cases := []struct {
		name      string
		status    PublicationStatus
		required  bool
		pause     bool
		blocker   bool
		wantState WorkState
		wantSync  string
	}{
		{name: "pending pause", status: PublicationPending, pause: true, wantState: StatePaused, wantSync: "pending"},
		{name: "failed pause", status: PublicationFailed, pause: true, wantState: StatePaused, wantSync: "failed"},
		{name: "conflict blocker", status: PublicationConflict, blocker: true, wantState: StateNeedsOperator, wantSync: "conflict"},
		{name: "completed pause", status: PublicationCompleted, pause: true, wantState: StatePaused, wantSync: "synced"},
		{name: "required pending blocker", status: PublicationPending, required: true, blocker: true, wantState: StateNeedsOperator, wantSync: "pending"},
		{name: "required failed pause", status: PublicationFailed, required: true, pause: true, wantState: StatePaused, wantSync: "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSnapshot()
			s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval", PauseRequested: tc.pause}
			s.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskIntegrated}
			p := round5Publication(t, tc.status)
			p.CompletionRequired = tc.required
			s.Publications = map[PublicationIntentID]PublicationState{"intent-1": p}
			if tc.blocker {
				s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "intent-1", Diagnostic: "ambiguous"}
			}
			reduce(&s)
			if s.State != tc.wantState || s.SyncStatus != tc.wantSync {
				t.Fatalf("projection = state %q sync %q, want state %q sync %q", s.State, s.SyncStatus, tc.wantState, tc.wantSync)
			}
		})
	}
}
