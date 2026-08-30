// Package scheduler computes the next Builder tasks without performing any
// process or persistence work.
package scheduler

import "thread-dock/internal/dag"

// TaskState is the durable lifecycle state of a task from the scheduler's
// perspective.
type TaskState string

const (
	Pending   TaskState = "pending"
	Running   TaskState = "running"
	Paused    TaskState = "paused"
	Failed    TaskState = "failed"
	Blocked   TaskState = "blocked"
	Repairing TaskState = "repairing"
	Completed TaskState = "completed"
)

const maxBuilders = 2

// Next returns the IDs of pending tasks that can be started now. IDs retain
// the graph's contract order, and at most two Builders can be selected. A
// missing state entry is treated as Pending; all other known non-pending
// states are excluded. Unknown states fail closed for their task.
func Next(graph dag.Graph, states map[string]TaskState, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if limit > maxBuilders {
		limit = maxBuilders
	}

	active := 0
	completed := make(map[string]bool, len(states))
	for id, state := range states {
		switch state {
		case Running, Repairing:
			active++
		case Completed:
			completed[id] = true
		}
	}
	available := limit - active
	if available <= 0 {
		return []string{}
	}

	ready := graph.Ready(completed)
	selected := make([]string, 0, available)
	for _, id := range ready {
		state, exists := states[id]
		if exists && state != Pending {
			continue
		}
		selected = append(selected, id)
		if len(selected) == available {
			break
		}
	}
	return selected
}
