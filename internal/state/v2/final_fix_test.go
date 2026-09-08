package statev2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func finalFixRequest(t *testing.T, snapshot WorkSnapshot, id contractv2.RequestID, tr TaskTransition) TransitionRequest {
	t.Helper()
	r := TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: id, Task: &tr}
	var err error
	r.PayloadHash, err = TransitionPayloadHash(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFinalFixApprovalMustBeCoherentForLoadAndAuthorization(t *testing.T) {
	base := validSnapshot()
	for name, control := range map[string]WorkControl{
		"hash only":  {ApprovedContractHash: base.ContractHash},
		"ref only":   {ApprovalRef: "operator"},
		"mismatched": {ApprovedContractHash: strings.Repeat("0", 64), ApprovalRef: "operator"},
	} {
		t.Run(name, func(t *testing.T) {
			s := base
			s.Control = control
			if err := validateSnapshot(s); err == nil {
				t.Fatal("incoherent approval accepted")
			}
		})
	}
	root := t.TempDir()
	s := NewStore(root)
	if _, err := createPlan(s, context.Background(), base); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(context.Background(), base.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Control = WorkControl{ApprovedContractHash: strings.Repeat("0", 64), ApprovalRef: "operator"}
	if err := writeSnapshot(filepath.Join(root, "v2", "work", string(base.WorkID), "work.json"), loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(context.Background(), base.WorkID); err == nil {
		t.Fatal("tampered approval loaded")
	}
	unauthorized := invocationSnapshot()
	unauthorized.Control = WorkControl{ApprovedContractHash: unauthorized.ContractHash}
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&unauthorized, TransitionRequest{Task: &reserve}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("task authorized with hash-only approval: %v", err)
	}
	publication := PublicationTransition{Action: PublicationBegin, IntentID: "intent", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one", Target: publicationTarget(), CompletionRequired: ptrBool(false)}
	if err := applyTransition(&unauthorized, TransitionRequest{Publication: &publication}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("publication authorized with hash-only approval: %v", err)
	}
}

func TestFinalFixPauseBlocksBeginLaunchWithoutWriteButAllowsCleanup(t *testing.T) {
	s := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	if err := applyTransition(&s, TransitionRequest{Work: &WorkTransition{Action: WorkPause, At: invocationAt(2)}}); err != nil {
		t.Fatal(err)
	}
	before := marshalSnapshot(t, s)
	begin := TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3)}
	if err := applyTransition(&s, TransitionRequest{Task: &begin}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("begin under pause = %v", err)
	}
	if got := marshalSnapshot(t, s); string(got) != string(before) {
		t.Fatal("begin under pause mutated state")
	}
	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3)}}); err == nil {
		t.Fatal("begin unexpectedly succeeded")
	}

	// A durable begin before pausing remains observable and can be cleaned up.
	s = invocationSnapshot()
	reserve = builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	begin = TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
	if err := applyTransition(&s, TransitionRequest{Task: &begin}); err != nil {
		t.Fatal(err)
	}
	if err := applyTransition(&s, TransitionRequest{Work: &WorkTransition{Action: WorkPause, At: invocationAt(3)}}); err != nil {
		t.Fatal(err)
	}
	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Invocation: &InvocationState{ProviderIdentity: "provider"}}}); err != nil {
		t.Fatal("mark running cleanup: ", err)
	}
	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskRequestTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(5), Reason: "paused"}}); err != nil {
		t.Fatal("request termination cleanup: ", err)
	}
}

func TestFinalFixPauseProjectionReservedReconciles(t *testing.T) {
	s := invocationSnapshot()
	state := s.TaskStates["task-1"]
	state.Status = TaskInvocationReserved
	state.Invocation = &InvocationState{InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1", LaunchRequested: false}
	state.InvocationHistory = []InvocationID{"inv-1"}
	s.TaskStates["task-1"] = state
	s.Control.PauseRequested = true

	Reduce(&s)
	if s.State != StateRunning || s.NextAction != "reconcile" {
		t.Fatalf("paused reserved projection = %q/%q, want running/reconcile", s.State, s.NextAction)
	}
}

func TestFinalFixReviewerRereservationIdentityDriftDoesNotWrite(t *testing.T) {
	base := invocationSnapshot()
	state := base.TaskStates["task-1"]
	state.Status = TaskGatePassed
	state.BuilderAttempt = 1
	state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40)}
	state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), Passed: true, ObservedAt: invocationAt(1)}
	worktree := &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "main", BaseSHA: strings.Repeat("b", 40), IntegratedDependencies: map[contractv2.TaskID]string{}}
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "review-work", Role: roleReviewer, BuilderAttempt: 1, LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1", Worktree: worktree}
	base.TaskStates["task-1"] = state
	for name, mutate := range map[string]func(*TaskTransition){
		"profile":  func(tr *TaskTransition) { tr.Invocation.LogicalProfile = "other" },
		"runtime":  func(tr *TaskTransition) { tr.Invocation.RuntimeFingerprint = "other" },
		"worktree": func(tr *TaskTransition) { tr.Worktree.Branch = "other" },
		"dependencies": func(tr *TaskTransition) {
			tr.Worktree.IntegratedDependencies = map[contractv2.TaskID]string{"other": strings.Repeat("c", 40)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := cloneSnapshot(t, base)
			tr := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(2), Worktree: cloneWorktree(worktree), Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
			mutate(&tr)
			before := marshalSnapshot(t, s)
			if err := applyTransition(&s, TransitionRequest{Task: &tr}); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("identity drift error = %v", err)
			}
			if got := marshalSnapshot(t, s); string(got) != string(before) {
				t.Fatalf("identity drift mutated state: before=%s after=%s", before, got)
			}
		})
	}
}

func TestFinalFixStorePauseProjectionAndLifecycleCleanup(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	reserve := builderReserveTransition(invocationAt(1))
	var err error
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "pause-reserve", reserve)); err != nil {
		t.Fatal(err)
	}
	if snapshot, err = store.Apply(ctx, transitionRequest(t, snapshot, "pause", WorkTransition{Action: WorkPause, At: invocationAt(2)})); err != nil {
		t.Fatal(err)
	}
	if snapshot.State != StateRunning || snapshot.NextAction != "reconcile" {
		t.Fatalf("paused reserved projection = %q/%q, want running/reconcile", snapshot.State, snapshot.NextAction)
	}
	persistedBefore, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	before := marshalSnapshot(t, persistedBefore)
	begin := TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3)}
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "paused-begin", begin)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("begin under pause = %v", err)
	}
	after, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := marshalSnapshot(t, after); string(got) != string(before) {
		t.Fatalf("begin under pause mutated persisted state: before=%s after=%s", before, got)
	}

	store, snapshot = approvedStore(t)
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "cleanup-reserve", reserve)); err != nil {
		t.Fatal(err)
	}
	begin.At = invocationAt(2)
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "cleanup-begin", begin)); err != nil {
		t.Fatal(err)
	}
	if snapshot, err = store.Apply(ctx, transitionRequest(t, snapshot, "cleanup-pause", WorkTransition{Action: WorkPause, At: invocationAt(3)})); err != nil {
		t.Fatal(err)
	}
	if snapshot.State != StateRunning || snapshot.NextAction != "reconcile" {
		t.Fatalf("paused launched projection = %q/%q, want running/reconcile", snapshot.State, snapshot.NextAction)
	}
	mark := TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Invocation: &InvocationState{ProviderIdentity: "provider"}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "cleanup-mark", mark)); err != nil {
		t.Fatalf("mark cleanup: %v", err)
	}
	if snapshot.State != StateRunning || snapshot.NextAction != "terminate" {
		t.Fatalf("paused running projection = %q/%q, want running/terminate", snapshot.State, snapshot.NextAction)
	}
	for _, item := range []struct {
		id string
		tr TaskTransition
	}{
		{id: "request", tr: TaskTransition{TaskID: "task-1", Action: TaskRequestTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(5), Reason: "paused"}},
		{id: "confirm", tr: TaskTransition{TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(6)}},
	} {
		if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID("cleanup-"+item.id), item.tr)); err != nil {
			t.Fatalf("%s cleanup: %v", item.id, err)
		}
		if item.tr.Action == TaskRequestTermination && snapshot.TaskStates["task-1"].Status != TaskTerminationPending {
			t.Fatalf("request cleanup status = %q, want termination_pending", snapshot.TaskStates["task-1"].Status)
		}
		if item.tr.Action == TaskConfirmTermination && snapshot.TaskStates["task-1"].Status != TaskTerminated {
			t.Fatalf("confirm cleanup status = %q, want terminated", snapshot.TaskStates["task-1"].Status)
		}
	}
	if snapshot.State != StatePaused || snapshot.NextAction != "resume" {
		t.Fatalf("cleaned paused projection = %q/%q, want paused/resume", snapshot.State, snapshot.NextAction)
	}
}

func TestFinalFixStoreTaskBlockerAllowsOnlyOtherTaskCleanup(t *testing.T) {
	ctx := context.Background()
	initial := validSnapshot()
	initial.Contract.Tasks = append(initial.Contract.Tasks, contractv2.Task{TaskID: "task-2", RepoKey: "app", AllowedPaths: []string{"internal/task2"}, AcceptanceCriteria: []string{"works"}})
	initial.TaskStates["task-2"] = TaskExecutionState{TaskID: "task-2", Status: TaskPending, RepairLimit: DefaultRepairLimit, RecoveryLimit: DefaultRecoveryLimit, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{}}
	canonical, err := canonicalContract(initial.Contract)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	initial.ContractHash = hex.EncodeToString(sum[:])
	store := NewStore(t.TempDir())
	if _, err := createPlan(store, ctx, initial); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, initial.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot, err = store.Apply(ctx, transitionRequest(t, snapshot, "approve-two", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash})); err != nil {
		t.Fatal(err)
	}
	builder := builderReserveTransition(invocationAt(1))
	builder.TaskID = "task-2"
	builder.InvocationID = "inv-2"
	builder.LogicalWorkID = "logical-2"
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "reserve-two", builder)); err != nil {
		t.Fatal(err)
	}
	begin := TaskTransition{TaskID: "task-2", Action: TaskBeginLaunch, InvocationID: "inv-2", LogicalWorkID: "logical-2", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "begin-two", begin)); err != nil {
		t.Fatal(err)
	}
	blocker := TaskTransition{TaskID: "task-1", Action: TaskNeedsOperatorAction, Role: roleBuilder, ReturnStage: TaskPending, At: invocationAt(2), Blocker: &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "operator", TaskID: "task-1", Diagnostic: "task A blocked"}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "block-one", blocker)); err != nil {
		t.Fatal(err)
	}
	mark := TaskTransition{TaskID: "task-2", Action: TaskMarkRunning, InvocationID: "inv-2", LogicalWorkID: "logical-2", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Invocation: &InvocationState{ProviderIdentity: "provider"}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "mark-two", mark)); err != nil {
		t.Fatal("mark cleanup: ", err)
	}
	request := TaskTransition{TaskID: "task-2", Action: TaskRequestTermination, InvocationID: "inv-2", LogicalWorkID: "logical-2", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(4), Reason: "blocked work cleanup"}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "request-two", request)); err != nil {
		t.Fatal("request cleanup: ", err)
	}
	confirm := TaskTransition{TaskID: "task-2", Action: TaskConfirmTermination, InvocationID: "inv-2", LogicalWorkID: "logical-2", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(5)}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "confirm-two", confirm)); err != nil {
		t.Fatal("confirm cleanup: ", err)
	}
	if snapshot.TaskStates["task-2"].Status != TaskTerminated {
		t.Fatalf("task B cleanup status = %q", snapshot.TaskStates["task-2"].Status)
	}
	persistedBefore, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	before := marshalSnapshot(t, persistedBefore)
	fresh := builderReserveTransition(invocationAt(6))
	fresh.TaskID, fresh.InvocationID, fresh.LogicalWorkID = "task-2", "inv-3", "logical-2"
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "fresh-reserve", fresh)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("fresh reserve under blocker = %v", err)
	}
	launch := TaskTransition{TaskID: "task-2", Action: TaskBeginLaunch, InvocationID: "inv-2", LogicalWorkID: "logical-2", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(6)}
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "fresh-launch", launch)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("fresh launch under blocker = %v", err)
	}
	evidence := TaskTransition{TaskID: "task-2", Action: TaskRecordCandidate, InvocationID: "inv-2", LogicalWorkID: "logical-2", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(6), Candidate: &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), ChangedFiles: []string{}}}
	if _, err := store.Apply(ctx, taskRequest(t, snapshot, "fresh-evidence", evidence)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("fresh evidence under blocker = %v", err)
	}
	after, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := marshalSnapshot(t, after); string(got) != string(before) {
		t.Fatalf("blocked fresh work mutated persisted state: before=%s after=%s", before, got)
	}
}

func TestFinalFixStoreRecoveryIdentityDriftDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*TaskTransition){
		"profile":  func(tr *TaskTransition) { tr.Invocation.LogicalProfile = "other" },
		"runtime":  func(tr *TaskTransition) { tr.Invocation.RuntimeFingerprint = "other" },
		"worktree": func(tr *TaskTransition) { tr.Worktree.Branch = "other" },
		"dependencies": func(tr *TaskTransition) {
			tr.Worktree.IntegratedDependencies = map[contractv2.TaskID]string{"other": strings.Repeat("c", 40)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			store, snapshot := approvedStore(t)
			snapshot = advanceBuilderToTerminated(t, store, snapshot, true)
			beforeSnapshot, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			recovery := builderReserveTransition(invocationAt(5))
			recovery.InvocationID = "recovery-drift"
			recovery.Transient = true
			mutate(&recovery)
			before := marshalSnapshot(t, beforeSnapshot)
			if _, err := store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID("recovery-drift-"+name), recovery)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("recovery identity drift error = %v", err)
			}
			after, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if got := marshalSnapshot(t, after); string(got) != string(before) {
				t.Fatalf("recovery identity drift mutated state: before=%s after=%s", before, got)
			}
		})
	}
}

func TestFinalFixStoreReviewerRereservationIdentityDriftDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*TaskTransition){
		"profile":  func(tr *TaskTransition) { tr.Invocation.LogicalProfile = "other" },
		"runtime":  func(tr *TaskTransition) { tr.Invocation.RuntimeFingerprint = "other" },
		"worktree": func(tr *TaskTransition) { tr.Worktree.Branch = "other" },
		"dependencies": func(tr *TaskTransition) {
			tr.Worktree.IntegratedDependencies = map[contractv2.TaskID]string{"other": strings.Repeat("c", 40)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			store, snapshot, reviewer := setupFinalFixReviewerLifecycle(t, false)
			reviewer.InvocationID = "review-rereserve"
			reviewer.At = invocationAt(11)
			mutate(&reviewer)
			beforeSnapshot, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			before := marshalSnapshot(t, beforeSnapshot)
			if _, err := store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID("review-rereserve-"+name), reviewer)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("reviewer identity drift error = %v", err)
			}
			after, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if got := marshalSnapshot(t, after); string(got) != string(before) {
				t.Fatalf("reviewer identity drift mutated state: before=%s after=%s", before, got)
			}
		})
	}
}

func TestFinalFixStoreReviewerRecoveryIdentityDriftDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*TaskTransition){
		"profile":  func(tr *TaskTransition) { tr.Invocation.LogicalProfile = "other" },
		"runtime":  func(tr *TaskTransition) { tr.Invocation.RuntimeFingerprint = "other" },
		"worktree": func(tr *TaskTransition) { tr.Worktree.Branch = "other" },
		"dependencies": func(tr *TaskTransition) {
			tr.Worktree.IntegratedDependencies = map[contractv2.TaskID]string{"other": strings.Repeat("c", 40)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			store, snapshot, reviewer := setupFinalFixReviewerLifecycle(t, true)
			reviewer.InvocationID = "review-recovery-drift"
			reviewer.At = invocationAt(11)
			reviewer.Transient = true
			mutate(&reviewer)
			beforeSnapshot, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			before := marshalSnapshot(t, beforeSnapshot)
			if _, err := store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID("review-recovery-drift-"+name), reviewer)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("reviewer recovery identity drift error = %v", err)
			}
			after, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if got := marshalSnapshot(t, after); string(got) != string(before) {
				t.Fatalf("reviewer recovery identity drift mutated state: before=%s after=%s", before, got)
			}
		})
	}
}

func setupFinalFixReviewerLifecycle(t *testing.T, transient bool) (Store, WorkSnapshot, TaskTransition) {
	t.Helper()
	ctx := context.Background()
	store, snapshot := approvedStore(t)
	snapshot = advanceBuilderToTerminated(t, store, snapshot, false)
	var err error
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "final-fix-candidate", candidateTransition())); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	gate := TaskTransition{TaskID: "task-1", Action: TaskRecordGate, Role: roleBuilder, BuilderAttempt: 1, At: invocationAt(6), Gate: &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(6)}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "final-fix-gate", gate)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := TaskTransition{TaskID: "task-1", Action: TaskReserveInvocation, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(7), Worktree: &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "review", BaseSHA: treeSHA, IntegratedDependencies: map[contractv2.TaskID]string{}}, Invocation: &InvocationState{LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1"}}
	if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, "final-fix-review-reserve", reviewer)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := []TaskTransition{
		{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(8)},
		{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(9), Invocation: &InvocationState{ProviderProcess: "review"}},
		{TaskID: "task-1", Action: TaskConfirmTermination, InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, BuilderAttempt: 1, At: invocationAt(10), Transient: transient},
	}
	for i, tr := range lifecycle {
		if snapshot, err = store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID("final-fix-review-lifecycle-"+string(rune('a'+i))), tr)); err != nil {
			t.Fatal(err)
		}
		snapshot, err = store.Load(ctx, snapshot.WorkID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return store, snapshot, reviewer
}

func TestFinalFixStoreIntegratedDependencyHeadCases(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*TaskTransition){
		"positive": func(_ *TaskTransition) {},
		"missing":  func(tr *TaskTransition) { tr.Worktree.IntegratedDependencies = map[contractv2.TaskID]string{} },
		"mismatch": func(tr *TaskTransition) { tr.Worktree.IntegratedDependencies["task-1"] = strings.Repeat("d", 40) },
		"extra":    func(tr *TaskTransition) { tr.Worktree.IntegratedDependencies["other"] = strings.Repeat("c", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			store, snapshot, head := setupFinalFixDependencyStore(t)
			reserve := builderReserveTransition(invocationAt(20))
			reserve.TaskID, reserve.InvocationID, reserve.LogicalWorkID = "task-2", "dep-inv", "dep-work"
			reserve.Worktree.IntegratedDependencies = map[contractv2.TaskID]string{"task-1": head}
			mutate(&reserve)
			beforeSnapshot, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			before := marshalSnapshot(t, beforeSnapshot)
			got, err := store.Apply(ctx, taskRequest(t, snapshot, contractv2.RequestID("dependency-"+name), reserve))
			if name == "positive" {
				if err != nil || got.TaskStates["task-2"].Status != TaskInvocationReserved {
					t.Fatalf("positive dependency reservation = %#v, %v", got, err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("%s dependency error = %v", name, err)
			}
			after, err := store.Load(ctx, snapshot.WorkID)
			if err != nil {
				t.Fatal(err)
			}
			if got := marshalSnapshot(t, after); string(got) != string(before) {
				t.Fatalf("%s dependency failure mutated state: before=%s after=%s", name, before, got)
			}
		})
	}
}

func setupFinalFixDependencyStore(t *testing.T) (Store, WorkSnapshot, string) {
	t.Helper()
	ctx := context.Background()
	initial := validSnapshot()
	initial.Contract.Tasks = append(initial.Contract.Tasks, contractv2.Task{TaskID: "task-2", RepoKey: "app", DependsOn: []contractv2.TaskID{"task-1"}, AllowedPaths: []string{"internal/task2"}, AcceptanceCriteria: []string{"works"}})
	head := strings.Repeat("c", 40)
	at := invocationAt(1)
	reviewWorktree := &WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "review", BaseSHA: treeSHA, IntegratedDependencies: map[contractv2.TaskID]string{}}
	initial.TaskStates["task-1"] = TaskExecutionState{TaskID: "task-1", Status: TaskIntegrated, BuilderAttempt: 1, RepairLimit: DefaultRepairLimit, RecoveryLimit: DefaultRecoveryLimit, LogicalWork: &LogicalWorkState{LogicalWorkID: "review-work", Role: roleReviewer, BuilderAttempt: 1, LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1", Worktree: reviewWorktree}, Worktree: cloneWorktree(reviewWorktree), Invocation: &InvocationState{InvocationID: "review-inv", LogicalWorkID: "review-work", Role: roleReviewer, ReturnStage: TaskGatePassed, LogicalProfile: "reviewer", RuntimeFingerprint: "runtime-v1", StartedAt: &at, EndedAt: &at, TerminationConfirmed: true}, Candidate: &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}, Gate: &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: at}, Review: &ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: true, Findings: []ReviewFinding{}, ObservedAt: at}, Integration: &IntegrationEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, IntegrationHEAD: head, RelationVerified: true, ObservedAt: at}, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{"review-inv"}}
	initial.TaskStates["task-2"] = TaskExecutionState{TaskID: "task-2", Status: TaskPending, RepairLimit: DefaultRepairLimit, RecoveryLimit: DefaultRecoveryLimit, PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{}}
	canonical, err := canonicalContract(initial.Contract)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	initial.ContractHash = hex.EncodeToString(sum[:])
	store := NewStore(t.TempDir())
	if _, err := createPlan(store, ctx, initial); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, initial.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Apply(ctx, transitionRequest(t, snapshot, "dependency-approve", WorkTransition{Action: WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash}))
	if err != nil {
		t.Fatal(err)
	}
	return store, snapshot, head
}

func TestFinalFixStorePublicationLineageMismatchAndImpossibleHistory(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedPublicationStore(t)
	begin := PublicationTransition{Action: PublicationBegin, IntentID: "pub-one", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one", Target: publicationTarget(), CompletionRequired: ptrBool(false)}
	var err error
	if snapshot, err = store.Apply(ctx, publicationRequest(t, snapshot, "pub-begin", begin)); err != nil {
		t.Fatal(err)
	}
	failed, err := store.Apply(ctx, publicationRequest(t, snapshot, "pub-fail", PublicationTransition{Action: PublicationFail, IntentID: "pub-one", Key: "issue:1", Generation: 1, Diagnostic: "failed"}))
	if err != nil {
		t.Fatal(err)
	}
	persistedBefore, err := store.Load(ctx, failed.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	before := marshalSnapshot(t, persistedBefore)
	next := PublicationTransition{Action: PublicationBegin, IntentID: "pub-two", Key: "issue:1", Kind: PublicationWiki, Generation: 2, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://two", Target: publicationTarget(), CompletionRequired: ptrBool(false)}
	if _, err := store.Apply(ctx, publicationRequest(t, failed, "pub-next-mismatch", next)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("next publication lineage error = %v", err)
	}
	after, err := store.Load(ctx, failed.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := marshalSnapshot(t, after); string(got) != string(before) {
		t.Fatal("next publication lineage mismatch mutated state")
	}
	supersede := PublicationTransition{Action: PublicationActionSupersede, IntentID: "pub-three", Supersedes: "pub-one", Key: "issue:1", Generation: 2, Kind: PublicationWiki, PayloadHash: strings.Repeat("c", 64), PayloadRef: "artifact://three", Target: publicationTarget(), CompletionRequired: ptrBool(false), Resolution: &ResolutionEvidence{NotPublished: true, Diagnostic: "not published"}}
	if _, err := store.Apply(ctx, publicationRequest(t, failed, "pub-supersede-mismatch", supersede)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("supersede publication lineage error = %v", err)
	}
	after, err = store.Load(ctx, failed.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := marshalSnapshot(t, after); string(got) != string(before) {
		t.Fatal("supersede publication lineage mismatch mutated state")
	}
	for name, status := range map[string]PublicationStatus{"pending": PublicationPending, "completed": PublicationCompleted, "failed": PublicationFailed, "conflict": PublicationConflict, "superseded": PublicationSuperseded} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			corruptStore, current := approvedPublicationStoreAt(t, root)
			prior := PublicationState{IntentID: "prior", Key: "issue:1", Generation: 1, Kind: PublicationParentIssue, Status: status, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://prior", Target: *publicationTarget(), Attempts: 1}
			switch status {
			case PublicationCompleted:
				prior.Receipt = &PublicationReceipt{NodeID: "node-prior", PublishedAt: invocationAt(3)}
			case PublicationFailed, PublicationConflict:
				prior.LastError = "provider failed"
			}
			current.Publications = map[PublicationIntentID]PublicationState{
				"prior": prior,
				"new":   {IntentID: "new", Key: "issue:1", Generation: 2, Kind: PublicationParentIssue, Status: PublicationPending, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://new", Target: *publicationTarget(), Attempts: 1},
			}
			if status == PublicationConflict {
				current.Control.Blocker = &OperatorBlocker{Kind: BlockerKindPublicationConflict, OperatorRef: "operator", IntentID: "prior", Diagnostic: "conflict"}
			}
			persistPublicationSnapshotForTest(t, root, current)
			_, err := corruptStore.Load(ctx, current.WorkID)
			if status == PublicationPending || status == PublicationFailed {
				if err == nil || !strings.Contains(err.Error(), "impossible prior-generation status") {
					t.Fatalf("impossible %s publication history error = %v, want nonterminal prior-generation diagnostic", name, err)
				}
			} else if status == PublicationConflict {
				if err == nil || !strings.Contains(err.Error(), "impossible prior-generation status") {
					t.Fatalf("impossible conflict publication history error = %v, want nonterminal prior-generation diagnostic", err)
				}
			} else if err != nil {
				t.Fatalf("valid %s publication history rejected: %v", name, err)
			}
		})
	}
}

func TestFinalFixStoreCompletedPublicationLineageMismatchDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	store, snapshot := approvedPublicationStore(t)
	begin := PublicationTransition{Action: PublicationBegin, IntentID: "completed-one", Key: "issue:1", Kind: PublicationParentIssue, Generation: 1, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://completed-one", Target: publicationTarget(), CompletionRequired: ptrBool(false)}
	var err error
	if snapshot, err = store.Apply(ctx, publicationRequest(t, snapshot, "completed-begin", begin)); err != nil {
		t.Fatal(err)
	}
	complete := PublicationTransition{Action: PublicationComplete, IntentID: "completed-one", Key: "issue:1", Generation: 1, Receipt: &PublicationReceipt{NodeID: "node-1", PublishedAt: invocationAt(2)}}
	if snapshot, err = store.Apply(ctx, publicationRequest(t, snapshot, "completed-finish", complete)); err != nil {
		t.Fatal(err)
	}
	persistedBefore, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	before := marshalSnapshot(t, persistedBefore)
	next := PublicationTransition{Action: PublicationBegin, IntentID: "completed-two", Key: "issue:1", Kind: PublicationWiki, Generation: 2, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://completed-two", Target: publicationTarget(), CompletionRequired: ptrBool(false)}
	if _, err := store.Apply(ctx, publicationRequest(t, snapshot, "completed-next-mismatch", next)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("completed next lineage error = %v", err)
	}
	after, err := store.Load(ctx, snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	if got := marshalSnapshot(t, after); string(got) != string(before) {
		t.Fatalf("completed next lineage mismatch mutated state: before=%s after=%s", before, got)
	}
}

func TestFinalFixBlockerOnlyAllowsMatchingLifecycleCleanup(t *testing.T) {
	s := invocationSnapshot()
	reserve := builderReserveTransition(invocationAt(1))
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); err != nil {
		t.Fatal(err)
	}
	begin := TaskTransition{TaskID: "task-1", Action: TaskBeginLaunch, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2)}
	if err := applyTransition(&s, TransitionRequest{Task: &begin}); err != nil {
		t.Fatal(err)
	}
	s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "operator", TaskID: "task-other", Diagnostic: "other task"}
	task := s.TaskStates["task-1"]
	task.Status = TaskInvocationReserved
	s.TaskStates["task-1"] = task
	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskMarkRunning, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(3), Invocation: &InvocationState{ProviderIdentity: "provider"}}}); err != nil {
		t.Fatal("cleanup with unrelated blocker: ", err)
	}
	before := marshalSnapshot(t, s)
	reserve = builderReserveTransition(invocationAt(4))
	reserve.InvocationID = "inv-2"
	if err := applyTransition(&s, TransitionRequest{Task: &reserve}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reserve with blocker = %v", err)
	}
	if got := marshalSnapshot(t, s); string(got) != string(before) {
		t.Fatal("blocked reserve mutated state")
	}
}

func TestFinalFixLogicalWorkCapturesAndMatchesExecutionIdentity(t *testing.T) {
	s := invocationSnapshot()
	tr := builderReserveTransition(invocationAt(1))
	tr.Invocation.LogicalProfile = "wrong-profile"
	if err := applyTransition(&s, TransitionRequest{Task: &tr}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("profile mismatch = %v", err)
	}
	tr.Invocation.LogicalProfile = "builder"
	tr.Invocation.RuntimeFingerprint = "runtime-v1"
	tr.Worktree.BaseSHA = strings.Repeat("a", 40)
	if err := applyTransition(&s, TransitionRequest{Task: &tr}); err != nil {
		t.Fatal(err)
	}
	lw := s.TaskStates["task-1"].LogicalWork
	if lw == nil || lw.LogicalProfile != "builder" || lw.RuntimeFingerprint != "runtime-v1" || lw.Worktree == nil || lw.Worktree.BaseSHA != strings.Repeat("a", 40) {
		t.Fatalf("logical execution identity not captured: %#v", lw)
	}
	if err := applyTransition(&s, TransitionRequest{Task: &TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Resolution: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true}}}); err != nil {
		t.Fatal(err)
	}
	drift := tr
	drift.InvocationID, drift.Invocation = "inv-2", &InvocationState{LogicalProfile: "builder", RuntimeFingerprint: "changed"}
	if err := applyTransition(&s, TransitionRequest{Task: &drift}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("fingerprint drift = %v", err)
	}
}

func TestFinalFixWorktreeBaseSHAAndDependencyHeadsAreExact(t *testing.T) {
	tr := builderReserveTransition(invocationAt(1))
	tr.Worktree.BaseSHA = "ABCDEF" + strings.Repeat("0", 34)
	if err := validateReservationInputs(tr); err == nil {
		t.Fatal("invalid BaseSHA accepted")
	}
	if err := validateReservationInputs(func() TaskTransition { x := tr; x.Worktree.BaseSHA = strings.Repeat("a", 40); return x }()); err != nil {
		t.Fatal(err)
	}
	if got := (&WorktreeIdentity{IntegratedDependencies: map[contractv2.TaskID]string{"dep": strings.Repeat("a", 40)}}).IntegratedDependencies["dep"]; got == "" {
		t.Fatal("integrated dependency field missing")
	}
}

func TestFinalFixReconcileUsesRollingNewestFiveForTaskAndWork(t *testing.T) {
	s := invocationSnapshot()
	state := s.TaskStates["task-1"]
	state.PriorAttempts = make([]AttemptSummary, MaxPriorAttempts)
	state.BuilderAttempt = 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical-1", Role: roleBuilder, BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1", Worktree: &WorktreeIdentity{BaseSHA: strings.Repeat("a", 40)}}
	state.Invocation = &InvocationState{InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1", LaunchRequested: false}
	state.Status = TaskInvocationReserved
	state.InvocationHistory = []InvocationID{"inv-1"}
	s.TaskStates["task-1"] = state
	tr := TaskTransition{TaskID: "task-1", Action: TaskReconcileNotStarted, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, BuilderAttempt: 1, At: invocationAt(2), Resolution: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true}}
	if err := applyTransition(&s, TransitionRequest{Task: &tr}); err != nil {
		t.Fatal("task rolling reconcile: ", err)
	}
	if len(s.TaskStates["task-1"].PriorAttempts) != MaxPriorAttempts || s.TaskStates["task-1"].PriorAttempts[0].BuilderAttempt != 0 {
		t.Fatalf("task history not rolled: %#v", s.TaskStates["task-1"].PriorAttempts)
	}
	state = s.TaskStates["task-1"]
	state.BuilderAttempt = 1
	state.PriorAttempts = make([]AttemptSummary, MaxPriorAttempts)
	state.Status = TaskNeedsOperator
	state.InvocationHistory = []InvocationID{"inv-2"}
	state.Invocation = &InvocationState{InvocationID: "inv-2", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}
	s.TaskStates["task-1"] = state
	s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "operator", TaskID: "task-1", InvocationID: "inv-2", Diagnostic: "runtime unknown"}
	s.State = StateNeedsOperator
	resolve := WorkTransition{Action: WorkResolve, At: invocationAt(3), Resolve: &ResolvePayload{Kind: ResolveRuntimeNotStarted, OperatorRef: "operator", TaskID: "task-1", InvocationID: "inv-2", Evidence: &ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true}}}
	if err := applyWorkTransition(&s, resolve); err != nil {
		t.Fatal("work rolling reconcile: ", err)
	}
	if len(s.TaskStates["task-1"].PriorAttempts) != MaxPriorAttempts || s.TaskStates["task-1"].PriorAttempts[0].BuilderAttempt != 0 {
		t.Fatalf("work history not rolled: %#v", s.TaskStates["task-1"].PriorAttempts)
	}
}

func TestFinalFixDependencyHeadsRequireExactIntegratedMap(t *testing.T) {
	s := validSnapshot()
	s.Contract.Tasks = append(s.Contract.Tasks, contractv2.Task{TaskID: "task-2", RepoKey: "app", DependsOn: []contractv2.TaskID{"task-1"}, AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}})
	dep := s.TaskStates["task-1"]
	dep.Status = TaskIntegrated
	dep.BuilderAttempt = 1
	dep.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), ChangedFiles: []string{}}
	dep.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)}
	dep.Review = &ReviewEvidence{ReviewerInvocationID: "review", BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), ReviewSHA: strings.Repeat("a", 40), Accepted: true, Findings: []ReviewFinding{}, ObservedAt: invocationAt(1)}
	dep.Integration = &IntegrationEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), IntegrationHEAD: strings.Repeat("c", 40), RelationVerified: true, ObservedAt: invocationAt(1)}
	s.TaskStates["task-1"] = dep
	s.TaskStates["task-2"] = TaskExecutionState{TaskID: "task-2", Status: TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []AttemptSummary{}}
	head := dep.Integration.IntegrationHEAD
	if err := validateIntegratedDependencies(&s, "task-2", map[contractv2.TaskID]string{"task-1": head}); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]map[contractv2.TaskID]string{"missing": {}, "mismatch": {"task-1": strings.Repeat("d", 40)}, "extra": {"task-1": head, "other": head}} {
		t.Run(name, func(t *testing.T) {
			if err := validateIntegratedDependencies(&s, "task-2", got); err == nil {
				t.Fatal("invalid dependency map accepted")
			}
		})
	}
}

func TestFinalFixPublicationLineageAndStrictHistory(t *testing.T) {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "operator"}
	s.State = StateQueued
	pub := PublicationState{IntentID: "one", Key: "issue:1", Generation: 1, Kind: PublicationParentIssue, Status: PublicationFailed, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one", Target: *publicationTarget(), Attempts: 1, CompletionRequired: true, LastError: "failed"}
	s.Publications = map[PublicationIntentID]PublicationState{"one": pub, "two": {IntentID: "two", Key: "issue:1", Generation: 2, Kind: PublicationWiki, Status: PublicationPending, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://two", Target: *publicationTarget(), Attempts: 1, CompletionRequired: true}}
	if err := validateSnapshot(s); err == nil {
		t.Fatal("lineage mismatch accepted")
	}
	s.Publications["two"] = PublicationState{IntentID: "two", Key: "issue:1", Generation: 2, Kind: PublicationParentIssue, Status: PublicationPending, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://two", Target: *publicationTarget(), Attempts: 1, CompletionRequired: true}
	s.Publications["one"] = pub
	if err := validateSnapshot(s); err == nil {
		t.Fatal("pending prior history accepted")
	}
}

func TestFinalFixPublicationLineageMismatchDoesNotWrite(t *testing.T) {
	s := validSnapshot()
	s.Control = WorkControl{ApprovedContractHash: s.ContractHash, ApprovalRef: "operator"}
	s.State = StateQueued
	completed := PublicationState{IntentID: "one", Key: "issue:1", Generation: 1, Kind: PublicationParentIssue, Status: PublicationCompleted, PayloadHash: strings.Repeat("a", 64), PayloadRef: "artifact://one", Target: *publicationTarget(), Attempts: 1, CompletionRequired: false, Receipt: &PublicationReceipt{NodeID: "n", PublishedAt: invocationAt(1)}}
	s.Publications = map[PublicationIntentID]PublicationState{"one": completed}
	before := marshalSnapshot(t, s)
	next := PublicationTransition{Action: PublicationBegin, IntentID: "two", Key: "issue:1", Generation: 2, Kind: PublicationWiki, PayloadHash: strings.Repeat("b", 64), PayloadRef: "artifact://two", Target: publicationTarget(), CompletionRequired: ptrBool(false)}
	if err := beginPublication(&s, next); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("next lineage mismatch = %v", err)
	}
	if got := marshalSnapshot(t, s); string(got) != string(before) {
		t.Fatal("lineage mismatch mutated state")
	}
	failed := completed
	failed.IntentID, failed.Status, failed.LastError, failed.Receipt = "failed", PublicationFailed, "failed", nil
	s.Publications = map[PublicationIntentID]PublicationState{"failed": failed}
	before = marshalSnapshot(t, s)
	supersede := PublicationTransition{Action: PublicationActionSupersede, IntentID: "next", Supersedes: "failed", Key: "issue:1", Generation: 2, Kind: PublicationWiki, PayloadHash: strings.Repeat("c", 64), PayloadRef: "artifact://three", Target: publicationTarget(), CompletionRequired: ptrBool(false), Resolution: &ResolutionEvidence{NotPublished: true, Diagnostic: "not published"}}
	if err := supersedePublication(&s, supersede); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("supersede lineage mismatch = %v", err)
	}
	if got := marshalSnapshot(t, s); string(got) != string(before) {
		t.Fatal("supersede mismatch mutated state")
	}
}
