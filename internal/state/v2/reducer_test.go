package statev2

import (
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func TestReducerChoosesHighestPriorityTaskRegardlessOfContractOrder(t *testing.T) {
	snapshot := WorkSnapshot{
		State:        StateQueued,
		ContractHash: "contract-hash",
		Control:      WorkControl{ApprovedContractHash: "contract-hash"},
		Contract:     contractv2.WorkItemContract{Tasks: []contractv2.Task{{TaskID: "task-running"}, {TaskID: "task-review"}}},
		TaskStates: map[contractv2.TaskID]TaskExecutionState{
			"task-running": {TaskID: "task-running", Status: TaskRunning},
			"task-review":  {TaskID: "task-review", Status: TaskAccepted},
		},
		Publications: map[PublicationIntentID]PublicationState{},
	}

	reduce(&snapshot)

	if snapshot.State != StateReview || snapshot.NextAction != "integrate" {
		t.Fatalf("reducer state = %q/%q, want review/integrate", snapshot.State, snapshot.NextAction)
	}
}

func TestReducerTaskNeedsOperatorWinsOverEveryTaskStage(t *testing.T) {
	snapshot := WorkSnapshot{
		ContractHash: "contract-hash",
		Control:      WorkControl{ApprovedContractHash: "contract-hash"},
		Contract:     contractv2.WorkItemContract{Tasks: []contractv2.Task{{TaskID: "task-ready"}, {TaskID: "task-operator"}}},
		TaskStates: map[contractv2.TaskID]TaskExecutionState{
			"task-ready":    {TaskID: "task-ready", Status: TaskCandidateReady},
			"task-operator": {TaskID: "task-operator", Status: TaskNeedsOperator},
		},
		Publications: map[PublicationIntentID]PublicationState{},
	}

	reduce(&snapshot)

	if snapshot.State != StateNeedsOperator || snapshot.NextAction != "resolve" {
		t.Fatalf("reducer state = %q/%q, want needs_operator/resolve", snapshot.State, snapshot.NextAction)
	}
}
