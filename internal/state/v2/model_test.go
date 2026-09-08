package statev2

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

func TestValidateSnapshotRequiresCanonicalTaskStatesAndPublications(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*WorkSnapshot)
	}{
		{"missing contract task", func(s *WorkSnapshot) { delete(s.TaskStates, "task-1") }},
		{"mismatched map key", func(s *WorkSnapshot) {
			state := s.TaskStates["task-1"]
			delete(s.TaskStates, "task-1")
			s.TaskStates["other"] = state
		}},
		{"mismatched internal task id", func(s *WorkSnapshot) {
			state := s.TaskStates["task-1"]
			state.TaskID = "other"
			s.TaskStates["task-1"] = state
		}},
		{"foreign task", func(s *WorkSnapshot) {
			s.TaskStates["foreign"] = TaskExecutionState{TaskID: "foreign", Status: TaskPending, RepairLimit: DefaultRepairLimit, RecoveryLimit: DefaultRecoveryLimit, PriorAttempts: []AttemptSummary{}}
		}},
		{"nil publications", func(s *WorkSnapshot) { s.Publications = nil }},
		{"zero repair limit", func(s *WorkSnapshot) {
			state := s.TaskStates["task-1"]
			state.RepairLimit = 0
			s.TaskStates["task-1"] = state
		}},
		{"zero recovery limit", func(s *WorkSnapshot) {
			state := s.TaskStates["task-1"]
			state.RecoveryLimit = 0
			s.TaskStates["task-1"] = state
		}},
		{"unknown task status", func(s *WorkSnapshot) {
			state := s.TaskStates["task-1"]
			state.Status = TaskStatus("unknown")
			s.TaskStates["task-1"] = state
		}},
		{"too many prior attempts", func(s *WorkSnapshot) {
			state := s.TaskStates["task-1"]
			state.PriorAttempts = make([]AttemptSummary, MaxPriorAttempts+1)
			s.TaskStates["task-1"] = state
		}},
		{"oversized diagnostic", func(s *WorkSnapshot) {
			s.Control.Blocker = &OperatorBlocker{Diagnostic: strings.Repeat("x", MaxDiagnosticBytes+1)}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSnapshot()
			tc.mutate(&s)
			if err := validateSnapshot(s); err == nil {
				t.Fatal("validateSnapshot accepted invalid typed state")
			}
		})
	}
}

func TestValidateSnapshotAcceptsCompleteTaskStateModel(t *testing.T) {
	if err := validateSnapshot(validSnapshot()); err != nil {
		t.Fatalf("valid typed snapshot rejected: %v", err)
	}
}

func TestDecodeSnapshotNormalizesOnlyUnstartedFoundationSnapshots(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.TaskStates = nil
	snapshot.Publications = nil
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(struct {
		SchemaVersion int                              `json:"schemaVersion"`
		ProjectID     contractv2.ProjectID             `json:"projectId"`
		WorkID        contractv2.WorkID                `json:"workId"`
		Revision      contractv2.Revision              `json:"revision"`
		State         WorkState                        `json:"state"`
		ContractHash  string                           `json:"contractHash"`
		Contract      contractv2.WorkItemContract      `json:"contract"`
		SyncStatus    string                           `json:"syncStatus"`
		NextAction    string                           `json:"nextAction"`
		EvidenceRefs  []string                         `json:"evidenceRefs"`
		Receipts      map[contractv2.RequestID]Receipt `json:"receipts"`
	}{snapshot.SchemaVersion, snapshot.ProjectID, snapshot.WorkID, snapshot.Revision, snapshot.State, snapshot.ContractHash, snapshot.Contract, snapshot.SyncStatus, snapshot.NextAction, snapshot.EvidenceRefs, snapshot.Receipts}); err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSnapshot(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("foundation snapshot rejected: %v", err)
	}
	if decoded.TaskStates == nil || decoded.Publications == nil {
		t.Fatalf("normalized maps are nil: taskStates=%#v publications=%#v", decoded.TaskStates, decoded.Publications)
	}
	if len(decoded.TaskStates) != len(snapshot.Contract.Tasks) || len(decoded.Publications) != 0 {
		t.Fatalf("normalized maps = %#v %#v", decoded.TaskStates, decoded.Publications)
	}
	for _, task := range snapshot.Contract.Tasks {
		state, ok := decoded.TaskStates[task.TaskID]
		if !ok || state.TaskID != task.TaskID || state.Status != TaskPending || state.RepairLimit != DefaultRepairLimit || state.RecoveryLimit != DefaultRecoveryLimit || state.PriorAttempts == nil {
			t.Fatalf("normalized task %q = %#v", task.TaskID, state)
		}
	}
}

func TestDecodeSnapshotRejectsModernMapsWithoutControl(t *testing.T) {
	snapshot := validSnapshot()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "control")
	modern, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSnapshot(bytes.NewReader(modern)); err == nil {
		t.Fatal("decodeSnapshot accepted modern maps without control")
	}
}

func TestValidateSnapshotRejectsUnknownInvocationReturnStage(t *testing.T) {
	snapshot := validSnapshot()
	state := snapshot.TaskStates["task-1"]
	state.Invocation = &InvocationState{InvocationID: "invocation", ReturnStage: TaskStatus("unknown")}
	state.InvocationHistory = []InvocationID{"invocation"}
	snapshot.TaskStates["task-1"] = state
	if err := validateSnapshot(snapshot); err == nil {
		t.Fatal("validateSnapshot accepted unknown invocation return stage")
	}
}

func TestValidateSnapshotAcceptsSupportedInvocationReturnStages(t *testing.T) {
	for _, stage := range []TaskStatus{TaskPending, TaskGateFailed, TaskReviewBlocked, TaskGatePassed} {
		t.Run(string(stage), func(t *testing.T) {
			snapshot := validSnapshot()
			state := snapshot.TaskStates["task-1"]
			role := roleBuilder
			if stage == TaskGatePassed {
				role = roleReviewer
				state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
				state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)}
			}
			state.Status = TaskInvocationReserved
			state.BuilderAttempt = 1
			state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: role, BuilderAttempt: 1}
			state.Invocation = &InvocationState{InvocationID: "invocation", LogicalWorkID: "logical", Role: role, ReturnStage: stage, LogicalProfile: "profile", RuntimeFingerprint: "runtime"}
			state.InvocationHistory = []InvocationID{"invocation"}
			snapshot.TaskStates["task-1"] = state
			if err := validateSnapshot(snapshot); err != nil {
				t.Fatalf("supported invocation return stage rejected: %v", err)
			}
		})
	}
}

func TestValidateSnapshotRejectsCorruptInvocationLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*TaskExecutionState)
	}{
		{"roleless", func(s *TaskExecutionState) { s.Invocation.Role = "" }},
		{"history mismatch", func(s *TaskExecutionState) { s.InvocationHistory = []InvocationID{"other"} }},
		{"ended without confirmation", func(s *TaskExecutionState) { at := invocationAt(1); s.Invocation.EndedAt = &at }},
		{"non-UTC start", func(s *TaskExecutionState) {
			at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
			s.Invocation.StartedAt = &at
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validSnapshot()
			state := snapshot.TaskStates["task-1"]
			state.Status = TaskInvocationReserved
			state.BuilderAttempt = 1
			state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: roleBuilder, BuilderAttempt: 1}
			state.Invocation = &InvocationState{InvocationID: "invocation", LogicalWorkID: "logical", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "profile", RuntimeFingerprint: "runtime"}
			state.InvocationHistory = []InvocationID{"invocation"}
			tc.mutate(&state)
			snapshot.TaskStates["task-1"] = state
			if err := validateSnapshot(snapshot); err == nil {
				t.Fatal("validateSnapshot accepted corrupt invocation")
			}
		})
	}
}

func TestValidateSnapshotRejectsCorruptPersistedEvidenceMatrix(t *testing.T) {
	terminatedBuilder := func() TaskExecutionState {
		at := invocationAt(1)
		return TaskExecutionState{
			TaskID: "task-1", Status: TaskTerminated, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1,
			LogicalWork:   &LogicalWorkState{LogicalWorkID: "logical", Role: roleBuilder, BuilderAttempt: 1},
			Invocation:    &InvocationState{InvocationID: "inv", LogicalWorkID: "logical", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "p", RuntimeFingerprint: "r", TerminationConfirmed: true, EndedAt: &at},
			PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{"inv"},
		}
	}
	terminatedReviewer := func() TaskExecutionState {
		at := invocationAt(1)
		return TaskExecutionState{
			TaskID: "task-1", Status: TaskTerminated, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1,
			LogicalWork:   &LogicalWorkState{LogicalWorkID: "review", Role: roleReviewer, BuilderAttempt: 1},
			Invocation:    &InvocationState{InvocationID: "review-inv", LogicalWorkID: "review", Role: roleReviewer, ReturnStage: TaskGatePassed, LogicalProfile: "p", RuntimeFingerprint: "r", TerminationConfirmed: true, EndedAt: &at},
			Candidate:     &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}},
			Gate:          &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)},
			PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{"review-inv"},
		}
	}
	validReview := func() *ReviewEvidence {
		return &ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: true, Findings: []ReviewFinding{}, ObservedAt: invocationAt(2)}
	}
	validIntegration := func() *IntegrationEvidence {
		return &IntegrationEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, IntegrationHEAD: integrationSHA, RelationVerified: true, ObservedAt: invocationAt(3)}
	}
	cases := []struct {
		name  string
		build func() WorkSnapshot
		want  string
	}{
		{"terminated missing invocation", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedBuilder()
			state.Invocation = nil
			state.InvocationHistory = []InvocationID{}
			s.TaskStates["task-1"] = state
			return s
		}, "terminated task has no terminated invocation"},
		{"terminated builder with candidate ahead", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedBuilder()
			state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			s.TaskStates["task-1"] = state
			return s
		}, "terminated builder lifecycle is incoherent"},
		{"terminated reviewer missing gate", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedReviewer()
			state.Gate = nil
			s.TaskStates["task-1"] = state
			return s
		}, "terminated reviewer lifecycle is incoherent"},
		{"pending with candidate ahead", func() WorkSnapshot {
			s := validSnapshot()
			state := s.TaskStates["task-1"]
			state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			state.BuilderAttempt = 1
			s.TaskStates["task-1"] = state
			return s
		}, "pending task has active invocation or evidence"},
		{"needs operator without blocker", func() WorkSnapshot {
			s := validSnapshot()
			state := s.TaskStates["task-1"]
			state.Status = TaskNeedsOperator
			state.BuilderAttempt = 1
			state.PriorAttempts = []AttemptSummary{}
			s.TaskStates["task-1"] = state
			return s
		}, "operator blocker is required without invocation or evidence"},
		{"invalid builder return stage", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedBuilder()
			state.Invocation.ReturnStage = TaskGatePassed
			s.TaskStates["task-1"] = state
			return s
		}, "builder invocation return stage is invalid"},
		{"gate status mismatch", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedBuilder()
			state.Status = TaskGatePassed
			state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"fail"}, Passed: false, ObservedAt: invocationAt(2)}
			s.TaskStates["task-1"] = state
			return s
		}, "invalid gate evidence"},
		{"review status mismatch", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedReviewer()
			state.Status = TaskAccepted
			state.Review = validReview()
			state.Review.Accepted = false
			state.Review.Findings = []ReviewFinding{{Code: "bad", Severity: "blocking"}}
			s.TaskStates["task-1"] = state
			return s
		}, "invalid review evidence"},
		{"integrated incomplete chain", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedReviewer()
			state.Status = TaskIntegrated
			state.Review = validReview()
			state.Integration = validIntegration()
			state.Gate = nil
			s.TaskStates["task-1"] = state
			return s
		}, "invalid integration evidence"},
		{"non-UTC gate", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedBuilder()
			state.Status = TaskGatePassed
			state.Candidate = &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}}
			at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("offset", 3600))
			state.Gate = &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: at}
			s.TaskStates["task-1"] = state
			return s
		}, "invalid gate evidence"},
		{"non-UTC review", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedReviewer()
			state.Status = TaskAccepted
			state.Review = validReview()
			state.Review.ObservedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("offset", 3600))
			s.TaskStates["task-1"] = state
			return s
		}, "invalid review evidence"},
		{"non-UTC integration", func() WorkSnapshot {
			s := validSnapshot()
			state := terminatedReviewer()
			state.Status = TaskIntegrated
			state.Review = validReview()
			state.Integration = validIntegration()
			state.Integration.ObservedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("offset", 3600))
			s.TaskStates["task-1"] = state
			return s
		}, "invalid integration evidence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSnapshot(tc.build()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateSnapshot error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateSnapshotRejectsTaskNeedsOperatorInvocationOnlyCorruption(t *testing.T) {
	base := func(role string, status TaskStatus, started, ended *time.Time, confirmed bool, reason string) WorkSnapshot {
		s := validSnapshot()
		state := s.TaskStates["task-1"]
		state.Status = TaskNeedsOperator
		state.BuilderAttempt = 1
		state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: role, BuilderAttempt: 1}
		state.Invocation = &InvocationState{
			InvocationID: "inv", LogicalWorkID: "logical", Role: role, ReturnStage: TaskPending,
			LogicalProfile: "p", RuntimeFingerprint: "r", StartedAt: started, EndedAt: ended,
			TerminationConfirmed: confirmed, TerminationReason: reason,
		}
		state.InvocationHistory = []InvocationID{"inv"}
		s.TaskStates["task-1"] = state
		s.Control.Blocker = &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "op", TaskID: "task-1", InvocationID: "inv", Diagnostic: "runtime unknown"}
		return s
	}
	at := invocationAt(1)
	cases := []struct {
		name  string
		build func() WorkSnapshot
		want  string
		valid bool
	}{
		{"roleless", func() WorkSnapshot {
			s := base("", TaskPending, nil, nil, false, "")
			return s
		}, "invocation role is required", false},
		{"unknown role", func() WorkSnapshot {
			s := base("scout", TaskPending, nil, nil, false, "")
			return s
		}, "invocation role is invalid", false},
		{"builder reserved", func() WorkSnapshot {
			s := base(roleBuilder, TaskPending, nil, nil, false, "")
			return s
		}, "", true},
		{"builder running without reason", func() WorkSnapshot {
			s := base(roleBuilder, TaskPending, &at, nil, false, "")
			return s
		}, "", true},
		{"builder termination pending", func() WorkSnapshot {
			s := base(roleBuilder, TaskPending, &at, nil, false, "stopping")
			return s
		}, "", true},
		{"builder terminated", func() WorkSnapshot {
			s := base(roleBuilder, TaskPending, &at, &at, true, "done")
			return s
		}, "", true},
		{"reviewer invocation-only", func() WorkSnapshot {
			s := base(roleReviewer, TaskGatePassed, nil, nil, false, "")
			s.TaskStates["task-1"].Invocation.ReturnStage = TaskGatePassed
			return s
		}, "reviewer invocation-only state is invalid", false},
		{"started and ended without confirmation", func() WorkSnapshot {
			s := base(roleBuilder, TaskPending, &at, &at, false, "done")
			return s
		}, "invocation termination lifecycle is inconsistent", false},
		{"reserved with termination reason", func() WorkSnapshot {
			s := base(roleBuilder, TaskPending, nil, nil, false, "stopping")
			return s
		}, "operator invocation-only state is invalid", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSnapshot(tc.build())
			if tc.valid {
				if err != nil {
					t.Fatalf("validateSnapshot rejected valid invocation-only state: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateSnapshot error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateSnapshotRejectsUnknownTaskNeedsOperatorBlockerKind(t *testing.T) {
	snapshot := validSnapshot()
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskNeedsOperator
	state.BuilderAttempt = 1
	state.PriorAttempts = []AttemptSummary{}
	snapshot.TaskStates["task-1"] = state
	snapshot.Control.Blocker = &OperatorBlocker{Kind: "unknown", OperatorRef: "op", TaskID: "task-1", Diagnostic: "blocked"}
	if err := validateSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "operator blocker kind is unknown") {
		t.Fatalf("validateSnapshot error = %v, want unknown blocker diagnostic", err)
	}
}

func TestValidateSnapshotAcceptsNeedsOperatorReviewerEvidenceChains(t *testing.T) {
	newReviewer := func() (WorkSnapshot, TaskExecutionState) {
		s := validSnapshot()
		at := invocationAt(1)
		state := TaskExecutionState{
			TaskID: "task-1", Status: TaskNeedsOperator, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1,
			LogicalWork:   &LogicalWorkState{LogicalWorkID: "review", Role: roleReviewer, BuilderAttempt: 1},
			Invocation:    &InvocationState{InvocationID: "review-inv", LogicalWorkID: "review", Role: roleReviewer, ReturnStage: TaskGatePassed, LogicalProfile: "p", RuntimeFingerprint: "r", TerminationConfirmed: true, EndedAt: &at},
			Candidate:     &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}},
			Gate:          &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"pass"}, Passed: true, ObservedAt: invocationAt(1)},
			PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{"review-inv"},
		}
		s.TaskStates["task-1"] = state
		s.Control = WorkControl{Blocker: &OperatorBlocker{Kind: BlockerKindRuntimeUnknown, OperatorRef: "op", TaskID: "task-1", InvocationID: "review-inv", Diagnostic: "unknown"}}
		return s, state
	}
	t.Run("active reviewer gate chain", func(t *testing.T) {
		s, state := newReviewer()
		state.Invocation.EndedAt = nil
		state.Invocation.TerminationConfirmed = false
		state.Invocation.StartedAt = ptrTime(invocationAt(2))
		s.TaskStates["task-1"] = state
		if err := validateSnapshot(s); err != nil {
			t.Fatalf("active reviewer gate chain rejected: %v", err)
		}
	})
	t.Run("terminated accepted review chain", func(t *testing.T) {
		s, state := newReviewer()
		state.Status = TaskNeedsOperator
		state.Review = &ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidateSHA, ReviewSHA: candidateSHA, Accepted: true, Findings: []ReviewFinding{}, ObservedAt: invocationAt(2)}
		state.Integration = nil
		s.TaskStates["task-1"] = state
		if err := validateSnapshot(s); err != nil {
			t.Fatalf("terminated accepted reviewer chain rejected: %v", err)
		}
	})
	t.Run("terminated builder failed gate chain", func(t *testing.T) {
		s := validSnapshot()
		at := invocationAt(1)
		state := TaskExecutionState{
			TaskID: "task-1", Status: TaskNeedsOperator, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1,
			LogicalWork:   &LogicalWorkState{LogicalWorkID: "builder", Role: roleBuilder, BuilderAttempt: 1},
			Invocation:    &InvocationState{InvocationID: "builder-inv", LogicalWorkID: "builder", Role: roleBuilder, ReturnStage: TaskGateFailed, LogicalProfile: "p", RuntimeFingerprint: "r", TerminationConfirmed: true, EndedAt: &at},
			Candidate:     &CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, TreeSHA: treeSHA, ChangedFiles: []string{}},
			Gate:          &GateEvidence{BuilderAttempt: 1, CandidateSHA: candidateSHA, Commands: []string{"check"}, Outcomes: []string{"fail"}, Passed: false, ObservedAt: invocationAt(1)},
			PriorAttempts: []AttemptSummary{}, InvocationHistory: []InvocationID{"builder-inv"},
		}
		s.TaskStates["task-1"] = state
		s.Control = WorkControl{Blocker: &OperatorBlocker{Kind: BlockerKindRepairBudgetExhausted, OperatorRef: "op", TaskID: "task-1", Diagnostic: "failed gate"}}
		if err := validateSnapshot(s); err != nil {
			t.Fatalf("terminated builder failed gate chain rejected: %v", err)
		}
	})
}

func ptrTime(at time.Time) *time.Time { return &at }

func TestStoreLoadRejectsPersistedInvocationCorruption(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewStore(root)
	if _, err := createPlan(store, ctx, validSnapshot()); err != nil {
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
	state := snapshot.TaskStates["task-1"]
	state.Status = TaskInvocationReserved
	state.BuilderAttempt = 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical", Role: roleBuilder, BuilderAttempt: 1}
	state.Invocation = &InvocationState{InvocationID: "inv", LogicalWorkID: "logical", Role: "", ReturnStage: TaskPending, LogicalProfile: "profile", RuntimeFingerprint: "runtime"}
	state.InvocationHistory = []InvocationID{"inv"}
	snapshot.TaskStates["task-1"] = state
	corrupt, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, "work-1"); err == nil {
		t.Fatal("Store.Load accepted persisted invocation corruption")
	}
}

func TestValidateSnapshotRejectsInvalidInvocationHistory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		history []InvocationID
	}{
		{name: "empty ID", history: []InvocationID{" "}},
		{name: "duplicate ID", history: []InvocationID{"inv-1", "inv-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validSnapshot()
			state := snapshot.TaskStates["task-1"]
			state.InvocationHistory = tc.history
			snapshot.TaskStates["task-1"] = state
			if err := validateSnapshot(snapshot); err == nil {
				t.Fatal("validateSnapshot accepted invalid invocation history")
			}
		})
	}
}

func TestValidateSnapshotRejectsActiveInvocationMissingFromHistory(t *testing.T) {
	snapshot := validSnapshot()
	state := snapshot.TaskStates["task-1"]
	state.BuilderAttempt = 1
	state.LogicalWork = &LogicalWorkState{LogicalWorkID: "logical-1", Role: roleBuilder, BuilderAttempt: 1}
	state.Invocation = &InvocationState{InvocationID: "inv-active", LogicalWorkID: "logical-1", Role: roleBuilder, ReturnStage: TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime"}
	state.InvocationHistory = nil
	snapshot.TaskStates["task-1"] = state
	if err := validateSnapshot(snapshot); err == nil {
		t.Fatal("validateSnapshot accepted active invocation with missing history")
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	var taskStates map[string]json.RawMessage
	if err := json.Unmarshal(fields["taskStates"], &taskStates); err != nil {
		t.Fatal(err)
	}
	var taskFields map[string]json.RawMessage
	if err := json.Unmarshal(taskStates["task-1"], &taskFields); err != nil {
		t.Fatal(err)
	}
	delete(taskFields, "invocationHistory")
	taskStates["task-1"], err = json.Marshal(taskFields)
	if err != nil {
		t.Fatal(err)
	}
	fields["taskStates"], err = json.Marshal(taskStates)
	if err != nil {
		t.Fatal(err)
	}
	modern, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSnapshot(bytes.NewReader(modern)); err == nil {
		t.Fatal("decodeSnapshot accepted active invocation without history")
	}
}

func TestDecodeSnapshotRejectsPartialOrStartedMissingMaps(t *testing.T) {
	base := validSnapshot()
	base.TaskStates = nil
	base.Publications = nil
	cases := []struct {
		name   string
		state  WorkState
		prefix string
	}{
		{"one map missing", StateAwaitingApproval, `{"publications":{}`},
		{"started state", StateRunning, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := base
			snapshot.State = tc.state
			encodedSnapshot, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encodedSnapshot, &fields); err != nil {
				t.Fatal(err)
			}
			delete(fields, "taskStates")
			delete(fields, "publications")
			delete(fields, "control")
			if tc.prefix != "" {
				fields["publications"] = json.RawMessage(`{}`)
			}
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeSnapshot(bytes.NewReader(data)); err == nil {
				t.Fatal("decodeSnapshot accepted incompatible missing-map snapshot")
			}
		})
	}
}

func TestStoreCreatePlanRejectsMissingTaskState(t *testing.T) {
	snapshot := validSnapshot()
	delete(snapshot.TaskStates, "task-1")
	if _, err := NewStore(t.TempDir()).CreatePlan(context.Background(), snapshot, "request", "payload"); err == nil {
		t.Fatal("CreatePlan accepted a plan missing a contract task state")
	}
}
