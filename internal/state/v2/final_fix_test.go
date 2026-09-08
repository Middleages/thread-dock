package statev2

import (
	"context"
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
