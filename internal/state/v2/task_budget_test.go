package statev2

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	contractv2 "thread-dock/internal/contract/v2"
	"unicode/utf8"
)

func TestRepairReservationDebitsAndInvalidatesEvidenceAtomically(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "candidate", candidateTransition())); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	gate := TaskTransition{TaskID: "task-1", Action: TaskRecordGate, Role: roleBuilder, BuilderAttempt: 1, At: invocationAt(6), Gate: &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"fail"}, Passed: false, ObservedAt: invocationAt(6)}}
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "gate", gate)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	reserve := builderReserveTransition(invocationAt(7))
	reserve.InvocationID = "repair-inv"
	reserve.LogicalWorkID = "repair-work"
	reserve.BuilderAttempt = 2
	reserve.ReturnStage = TaskGateFailed
	snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "repair", reserve))
	if err != nil {
		t.Fatal(err)
	}
	state := snapshot.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.RepairCount != 1 || state.BuilderAttempt != 2 || state.Candidate != nil || state.Gate != nil || state.Invocation == nil || state.Invocation.InvocationID != "repair-inv" || len(state.PriorAttempts) != 1 {
		t.Fatalf("repair state = %#v", state)
	}
	if state.PriorAttempts[0].CandidateSHA != candidateSHA || state.PriorAttempts[0].Outcome != "gate_failed" {
		t.Fatalf("repair summary = %#v", state.PriorAttempts[0])
	}
	replay, err := store.Apply(ctx, taskRequest(t, snapshot, "repair", reserve))
	if err != nil {
		t.Fatal(err)
	}
	if replay.Revision != snapshot.Revision || replay.TaskStates["task-1"].RepairCount != 1 {
		t.Fatalf("repair replay changed budget: %#v", replay.TaskStates["task-1"])
	}
}

func TestRepairBudgetExhaustionNeedsOperatorWithoutReservation(t *testing.T) {
	_, snapshot := approvedStore(t)
	state := snapshot.TaskStates["task-1"]
	state.Status, state.BuilderAttempt, state.RepairCount = TaskGateFailed, 2, 2
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical-1", Role: roleBuilder, BuilderAttempt: 2}
	state.Candidate = &CandidateEvidence{BuilderAttempt: 2, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"x"}}
	state.Gate = &GateEvidence{BuilderAttempt: 2, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"fail"}, ObservedAt: invocationAt(1)}
	snapshot.TaskStates["task-1"] = state
	if err := validateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	reserve := builderReserveTransition(invocationAt(2))
	reserve.InvocationID = "repair-inv"
	reserve.LogicalWorkID = "repair-work"
	reserve.BuilderAttempt = 3
	reserve.ReturnStage = TaskGateFailed
	reserve.Blocker = &OperatorBlocker{Kind: BlockerKindRepairBudgetExhausted, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "repair budget exhausted"}
	if err := applyTaskTransition(&snapshot, reserve, "repair-exhausted"); err != nil {
		t.Fatal(err)
	}
	got := snapshot
	state = got.TaskStates["task-1"]
	if state.Status != TaskNeedsOperator || state.RepairCount != 2 || state.Invocation != nil || got.Control.Blocker == nil || got.Control.Blocker.Kind != BlockerKindRepairBudgetExhausted {
		t.Fatalf("exhausted state = %#v blocker=%#v", state, got.Control.Blocker)
	}
}

func TestRecoveryReservationUsesTransientTerminatedInvocationAndBudget(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, true)
	tr := builderReserveTransition(invocationAt(5))
	tr.InvocationID = "recovery-inv"
	tr.Transient = true
	got, err := store.Apply(ctx, taskRequest(t, snapshot, "recovery", tr))
	if err != nil {
		t.Fatal(err)
	}
	state := got.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.RecoveryCount != 1 || state.BuilderAttempt != 1 || state.Invocation.InvocationID != "recovery-inv" {
		t.Fatalf("recovery state = %#v", state)
	}
	if state.Candidate != nil || state.Gate != nil {
		t.Fatalf("recovery changed evidence: %#v %#v", state.Candidate, state.Gate)
	}
}

func TestRecoveryExhaustionPersistsAndExtendBudgetResolves(t *testing.T) {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval"}
	s.State = StateRunning
	s.State = StateRunning
	at := invocationAt(1)
	state := s.TaskStates["task-1"]
	state.Status = TaskTerminated
	state.BuilderAttempt = 1
	state.RecoveryCount, state.RecoveryLimit = 1, 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: roleBuilder, BuilderAttempt: 1}
	state.Invocation = &InvocationState{InvocationID: "old", LogicalWorkID: "logical", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "p", RuntimeFingerprint: "r", TerminationConfirmed: true, EndedAt: &at, TransientFailure: true}
	state.InvocationHistory = []InvocationID{"old"}
	s.TaskStates["task-1"] = state
	tr := builderReserveTransition(invocationAt(2))
	tr.InvocationID = "new"
	tr.LogicalWorkID = "logical"
	tr.Transient = true
	tr.Blocker = &OperatorBlocker{Kind: BlockerKindRecoveryBudgetExhausted, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "recovery exhausted"}
	if err := applyTaskTransition(&s, tr, "exhaust"); err != nil {
		t.Fatal(err)
	}
	if s.TaskStates["task-1"].Status != TaskNeedsOperator || s.Control.Blocker == nil {
		t.Fatalf("exhaustion = %#v %#v", s.TaskStates["task-1"], s.Control.Blocker)
	}
	if err := applyWorkTransition(&s, WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolveExtendBudget, OperatorRef: "operator", TaskID: "task-1", Budget: BudgetRecovery, NewLimit: 2}}); err != nil {
		t.Fatal(err)
	}
	if s.TaskStates["task-1"].RecoveryLimit != 2 || s.Control.Blocker != nil {
		t.Fatalf("extended recovery = %#v %#v", s.TaskStates["task-1"], s.Control.Blocker)
	}
}

func TestPauseResumeLeavesTaskBudgetsAndEvidenceUnchanged(t *testing.T) {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval"}
	s.State = StateRunning
	state := s.TaskStates["task-1"]
	state.Status = TaskCandidateReady
	state.BuilderAttempt = 1
	state.RepairCount, state.RecoveryCount = 1, 1
	state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
	state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"x"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)}
	s.TaskStates["task-1"] = state
	before := cloneSnapshot(t, s)
	if err := applyWorkTransition(&s, WorkTransition{Action: WorkPause}); err != nil {
		t.Fatal(err)
	}
	if err := applyWorkTransition(&s, WorkTransition{Action: WorkResume}); err != nil {
		t.Fatal(err)
	}
	after := s.TaskStates["task-1"]
	if before.TaskStates["task-1"].RepairCount != after.RepairCount || before.TaskStates["task-1"].RecoveryCount != after.RecoveryCount || !reflect.DeepEqual(before.TaskStates["task-1"].Candidate, after.Candidate) || !reflect.DeepEqual(before.TaskStates["task-1"].Gate, after.Gate) {
		t.Fatalf("pause/resume mutated task: before=%#v after=%#v", before.TaskStates["task-1"], after)
	}
}

func TestRetryVerifiedStageDerivesTaskStatusFromEvidence(t *testing.T) {
	_, snapshot := approvedStore(t)
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskNeedsOperator
	state.BuilderAttempt = 1
	state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"x"}}
	state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)}
	snapshot.TaskStates["task-1"] = state
	snapshot.State = StateNeedsOperator
	snapshot.Control.Blocker = &OperatorBlocker{Kind: BlockerKindRetryVerifiedStage, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "repair"}
	if err := validateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	err := applyWorkTransition(&snapshot, WorkTransition{Action: WorkResolve, At: invocationAt(2), Resolve: &ResolvePayload{Kind: ResolveRetryVerifiedStage, OperatorRef: "operator", TaskID: "task-1", Evidence: &ResolutionEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Diagnostic: "verified"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot
	if got.TaskStates["task-1"].Status != TaskGatePassed || got.Control.Blocker != nil {
		t.Fatalf("resolved state = %#v blocker=%#v", got.TaskStates["task-1"], got.Control.Blocker)
	}
}

func TestRetryVerifiedStageRequiresExactBlockerAndEvidence(t *testing.T) {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval", Blocker: &OperatorBlocker{Kind: BlockerKindEvidenceMismatch, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "blocked"}}
	s.State = StateNeedsOperator
	s.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskNeedsOperator, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1, Candidate: &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{"x"}}}
	before := cloneSnapshot(t, s)
	err := applyWorkTransition(&s, WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolveRetryVerifiedStage, OperatorRef: "operator", TaskID: "task-1", Evidence: &ResolutionEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Diagnostic: "verified"}}})
	if !errors.Is(err, ErrInvalidTransition) || !reflect.DeepEqual(s, before) {
		t.Fatalf("wrong blocker err=%v state=%#v", err, s)
	}
}

func TestRetryVerifiedStageDerivesEveryEvidenceBranch(t *testing.T) {
	cases := []struct {
		name  string
		want  TaskStatus
		build func(*TaskExecutionState)
	}{
		{"integrated", TaskIntegrated, func(s *TaskExecutionState) {
			s.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			s.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Passed: true, Commands: []string{"x"}, Outcomes: []string{"pass"}, ObservedAt: invocationAt(1)}
			s.Review = &ReviewEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: true, Findings: []ReviewFinding{}, ObservedAt: invocationAt(1)}
			s.Integration = &IntegrationEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, IntegrationHEAD: integrationSHA, RelationVerified: true, ObservedAt: invocationAt(1)}
		}},
		{"accepted", TaskAccepted, func(s *TaskExecutionState) {
			s.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			s.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Passed: true, Commands: []string{"x"}, Outcomes: []string{"pass"}, ObservedAt: invocationAt(1)}
			s.Review = &ReviewEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: true, Findings: []ReviewFinding{}, ObservedAt: invocationAt(1)}
		}},
		{"review_blocked", TaskReviewBlocked, func(s *TaskExecutionState) {
			s.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			s.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Passed: true, Commands: []string{"x"}, Outcomes: []string{"pass"}, ObservedAt: invocationAt(1)}
			s.Review = &ReviewEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Findings: []ReviewFinding{{Code: "x"}}, ObservedAt: invocationAt(1)}
		}},
		{"gate_passed", TaskGatePassed, func(s *TaskExecutionState) {
			s.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			s.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Passed: true, Commands: []string{"x"}, Outcomes: []string{"pass"}, ObservedAt: invocationAt(1)}
		}},
		{"gate_failed", TaskGateFailed, func(s *TaskExecutionState) {
			s.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			s.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Passed: false, Commands: []string{"x"}, Outcomes: []string{"fail"}, ObservedAt: invocationAt(1)}
		}},
		{"candidate_ready", TaskCandidateReady, func(s *TaskExecutionState) {
			s.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
		}},
		{"terminated", TaskTerminated, func(s *TaskExecutionState) {
			at := invocationAt(1)
			s.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: roleBuilder, BuilderAttempt: 1}
			s.Invocation = &InvocationState{InvocationID: "inv", LogicalWorkID: "logical", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "p", RuntimeFingerprint: "r", TerminationConfirmed: true, EndedAt: &at}
			s.InvocationHistory = []InvocationID{"inv"}
		}},
		{"pending", TaskPending, func(*TaskExecutionState) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSnapshot()
			s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval", Blocker: &OperatorBlocker{Kind: BlockerKindRetryVerifiedStage, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "blocked"}}
			s.State = StateNeedsOperator
			state := s.TaskStates["task-1"]
			state.BuilderAttempt = 1
			tc.build(&state)
			s.TaskStates["task-1"] = state
			ev := &ResolutionEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Diagnostic: "verified"}
			if tc.name == "pending" || tc.name == "terminated" || tc.name == "gate_passed" || tc.name == "gate_failed" || tc.name == "candidate_ready" {
				if tc.name == "pending" || tc.name == "terminated" {
					ev.CandidateSHA = ""
				}
				ev.ReviewSHA = ""
			}
			if err := applyWorkTransition(&s, WorkTransition{Action: WorkResolve, Resolve: &ResolvePayload{Kind: ResolveRetryVerifiedStage, OperatorRef: "operator", TaskID: "task-1", Evidence: ev}}); err != nil {
				t.Fatal(err)
			}
			if s.TaskStates["task-1"].Status != tc.want {
				t.Fatalf("derived status=%q want=%q", s.TaskStates["task-1"].Status, tc.want)
			}
		})
	}
}

func TestReviewBlockedRepairUsesNewBuilderLogicalWork(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "candidate", candidateTransition())); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	gate := TaskTransition{TaskID: "task-1", Action: TaskRecordGate, Role: roleBuilder, BuilderAttempt: 1, At: invocationAt(6), Gate: &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"go test"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(6)}}
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "gate", gate)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	reviewReserve := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(7), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "review", BaseSHA: treeSHA}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "review-reserve", reviewReserve)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	for _, tr := range []TaskTransition{{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(8)}, {TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(9), Invocation: &InvocationState{ProviderProcess: "review"}}, {TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(10)}} {
		if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID(string(tr.Action)), tr)); err != nil {
			t.Fatal(err)
		} else {
			_ = snapshot
		}
		snapshot, _ = store.Load(ctx, "work-1")
	}
	review := TaskTransition{TaskID: "task-1", Action: TaskRecordReview, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(11), Review: &ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: false, Findings: []ReviewFinding{{Code: "bad", Severity: "blocking"}}, ObservedAt: invocationAt(11)}}
	if snapshot, err := store.Apply(ctx, taskRequest(t, snapshot, "review", review)); err != nil {
		t.Fatal(err)
	} else {
		_ = snapshot
	}
	snapshot, _ = store.Load(ctx, "work-1")
	old := snapshot.TaskStates["task-1"].LogicalWork.LogicalWorkID
	reserve := builderReserveTransition(invocationAt(12))
	reserve.InvocationID = "repair-inv"
	reserve.LogicalWorkID = "repair-work"
	reserve.BuilderAttempt = 2
	reserve.ReturnStage = TaskReviewBlocked
	got, err := store.Apply(ctx, taskRequest(t, snapshot, "repair", reserve))
	if err != nil {
		t.Fatal(err)
	}
	state := got.TaskStates["task-1"]
	if state.Status != TaskInvocationReserved || state.LogicalWork.LogicalWorkID != "repair-work" || state.LogicalWork.LogicalWorkID == old || state.LogicalWork.Role != roleBuilder || state.RepairCount != 1 || state.BuilderAttempt != 2 || state.Review != nil {
		t.Fatalf("review repair state = %#v", state)
	}
}

func TestBudgetExhaustionRequiresMatchingOperatorBlocker(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval"}
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskGateFailed
	state.BuilderAttempt = 2
	state.RepairCount = 2
	state.RepairLimit = 2
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "old", Role: roleBuilder, BuilderAttempt: 2}
	snapshot.TaskStates["task-1"] = state
	snapshot.State = StateRunning
	tr := builderReserveTransition(invocationAt(1))
	tr.InvocationID = "new"
	tr.LogicalWorkID = "new-work"
	tr.BuilderAttempt = 3
	tr.ReturnStage = TaskGateFailed
	before := cloneSnapshot(t, snapshot)
	if err := applyTaskTransition(&snapshot, tr, "request"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("missing blocker error = %v", err)
	}
	if !reflect.DeepEqual(snapshot, before) {
		t.Fatal("missing blocker mutated state")
	}
	tr.Blocker = &OperatorBlocker{Kind: BlockerKindRepairBudgetExhausted, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "repair budget exhausted"}
	if err := applyTaskTransition(&snapshot, tr, "request-2"); err != nil {
		t.Fatal(err)
	}
	if snapshot.TaskStates["task-1"].Status != TaskNeedsOperator || snapshot.Control.Blocker == nil || snapshot.Control.Blocker.OperatorRef != "operator" {
		t.Fatalf("blocker = %#v", snapshot.Control.Blocker)
	}
	if len([]byte(snapshot.Control.Blocker.Diagnostic)) > MaxDiagnosticBytes {
		t.Fatalf("diagnostic exceeds bound: %d", len([]byte(snapshot.Control.Blocker.Diagnostic)))
	}
}

func TestBudgetExhaustionTruncatesMultibyteDiagnosticAtUTF8Boundary(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval"}
	snapshot.State = StateRunning
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskGateFailed
	state.BuilderAttempt = 2
	state.RepairCount = 2
	state.RepairLimit = 2
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "old", Role: roleBuilder, BuilderAttempt: 2}
	snapshot.TaskStates["task-1"] = state
	tr := builderReserveTransition(invocationAt(1))
	tr.InvocationID = "new"
	tr.LogicalWorkID = "new"
	tr.BuilderAttempt = 3
	tr.ReturnStage = TaskGateFailed
	tr.Blocker = &OperatorBlocker{Kind: BlockerKindRepairBudgetExhausted, OperatorRef: "operator", TaskID: "task-1", Diagnostic: strings.Repeat("한", MaxDiagnosticBytes)}
	if err := applyTaskTransition(&snapshot, tr, "utf8"); err != nil {
		t.Fatal(err)
	}
	if len([]byte(snapshot.Control.Blocker.Diagnostic)) > MaxDiagnosticBytes || !utf8.ValidString(snapshot.Control.Blocker.Diagnostic) {
		t.Fatalf("bounded diagnostic invalid: bytes=%d value=%q", len([]byte(snapshot.Control.Blocker.Diagnostic)), snapshot.Control.Blocker.Diagnostic)
	}
}

func TestReducerMixedIntegratedAndPendingRemainsQueued(t *testing.T) {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "approval"}
	s.Contract.Tasks = append(s.Contract.Tasks, contractv2.Task{TaskID: "task-2", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}})
	s.TaskStates["task-2"] = newTaskExecutionState("task-2")
	s.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskIntegrated, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{}}
	reduce(&s)
	if s.State != StateQueued || s.NextAction != "run" {
		t.Fatalf("mixed reducer state = %q/%q", s.State, s.NextAction)
	}
}

func TestInBudgetRepairRollsPriorAttemptsToNewestFive(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Control = WorkControl{ApprovedContractHash: snapshot.ContractHash, ApprovalRef: "approval"}
	snapshot.State = StateRunning
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskGateFailed
	state.BuilderAttempt = 6
	state.RepairCount = 1
	state.RepairLimit = 2
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "old", Role: roleBuilder, BuilderAttempt: 6}
	state.Candidate = &CandidateEvidence{BuilderAttempt: 6, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
	state.Gate = &GateEvidence{BuilderAttempt: 6, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"fail"}, ObservedAt: invocationAt(1)}
	for i := 1; i <= MaxPriorAttempts; i++ {
		state.PriorAttempts = append(state.PriorAttempts, AttemptSummary{BuilderAttempt: uint32(i), Outcome: "old"})
	}
	snapshot.TaskStates["task-1"] = state
	tr := builderReserveTransition(invocationAt(2))
	tr.InvocationID = "repair"
	tr.LogicalWorkID = "new"
	tr.BuilderAttempt = 7
	tr.ReturnStage = TaskGateFailed
	if err := applyTaskTransition(&snapshot, tr, "roll"); err != nil {
		t.Fatal(err)
	}
	got := snapshot.TaskStates["task-1"]
	if got.Status != TaskInvocationReserved || len(got.PriorAttempts) != MaxPriorAttempts || got.PriorAttempts[0].BuilderAttempt != 2 || got.PriorAttempts[MaxPriorAttempts-1].BuilderAttempt != 6 {
		t.Fatalf("rolled history = %#v", got.PriorAttempts)
	}
}

func TestUnsupportedEvidenceActionIsRejected(t *testing.T) {
	tr := TaskTransition{TaskID: "task-1", Action: TaskRecordCandidate}
	if _, err := TransitionPayloadHash(TransitionRequest{Task: &tr}); err != nil {
		t.Fatal(err)
	}
	if err := applyTransition(&WorkSnapshot{}, TransitionRequest{Task: &tr}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v", err)
	}
}
