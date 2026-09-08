package statev2

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

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
	state.Invocation = &InvocationState{ReturnStage: TaskStatus("unknown")}
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
			state.Invocation = &InvocationState{ReturnStage: stage}
			snapshot.TaskStates["task-1"] = state
			if err := validateSnapshot(snapshot); err != nil {
				t.Fatalf("supported invocation return stage rejected: %v", err)
			}
		})
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
