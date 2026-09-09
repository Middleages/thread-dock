package statev2

// Reduce recomputes the derived Work state from its typed control, task, and
// publication state. It does not persist or otherwise perform side effects.
func Reduce(snapshot *WorkSnapshot) {
	reduce(snapshot)
}

func reduce(snapshot *WorkSnapshot) {
	publicationPending, publicationFailed, publicationConflict, completionRequiredPending, publicationSettled := publicationAggregate(snapshot)
	if publicationConflict {
		snapshot.SyncStatus = "conflict"
	} else if publicationFailed {
		snapshot.SyncStatus = "failed"
	} else if publicationPending {
		snapshot.SyncStatus = "pending"
	} else if publicationSettled {
		snapshot.SyncStatus = "synced"
	}
	if snapshot.State == StateDraft && snapshot.Control.ApprovedContractHash == "" && len(snapshot.TaskStates) == 0 && len(snapshot.Publications) == 0 {
		snapshot.NextAction = ""
		return
	}
	if snapshot.Control.ApprovedContractHash == "" {
		snapshot.State = StateAwaitingApproval
		snapshot.NextAction = "approve"
		return
	}
	// Operator attention is the strongest durable signal. A task-level blocker
	// is authoritative even when the control blocker was not persisted by an
	// older foundation snapshot.
	for _, task := range snapshot.TaskStates {
		if task.Status == TaskNeedsOperator {
			snapshot.State = StateNeedsOperator
			snapshot.NextAction = "resolve"
			return
		}
	}
	if snapshot.Control.Blocker != nil {
		snapshot.State = StateNeedsOperator
		snapshot.NextAction = "resolve"
		return
	}
	if snapshot.Control.PauseRequested {
		for _, task := range snapshot.TaskStates {
			if task.Status == TaskWorktreePreparing || task.Status == TaskInvocationReserved {
				snapshot.State = StateRunning
				snapshot.NextAction = "reconcile"
				return
			}
		}
		for _, task := range snapshot.TaskStates {
			if task.Status == TaskRunning || task.Status == TaskTerminationPending {
				snapshot.State = StateRunning
				snapshot.NextAction = "terminate"
				return
			}
		}
		snapshot.State = StatePaused
		snapshot.NextAction = "resume"
		return
	}
	// Required publications are a distinct work stage. Keep this ahead of
	// task execution stages so a mixed snapshot cannot hide a durable publish
	// obligation behind a lower-priority running/review task.
	if completionRequiredPending {
		snapshot.State = StatePublicationPending
		snapshot.NextAction = "publish"
		return
	}
	// Review is higher priority than running. Iterating the contract order
	// would otherwise make the aggregate depend on YAML task ordering.
	for _, contractTask := range snapshot.Contract.Tasks {
		task, ok := snapshot.TaskStates[contractTask.TaskID]
		if ok && (task.Status == TaskAccepted || task.Status == TaskGatePassed) {
			snapshot.State = StateReview
			snapshot.NextAction = map[TaskStatus]string{TaskAccepted: "integrate", TaskGatePassed: "review"}[task.Status]
			return
		}
	}
	for _, contractTask := range snapshot.Contract.Tasks {
		task, ok := snapshot.TaskStates[contractTask.TaskID]
		if !ok {
			continue
		}
		switch task.Status {
		case TaskWorktreePreparing:
			snapshot.State = StateRunning
			snapshot.NextAction = "reconcile"
			return
		case TaskInvocationReserved:
			snapshot.State = StateRunning
			if snapshot.Control.PauseRequested {
				snapshot.NextAction = "reconcile"
			} else if task.Invocation != nil && task.Invocation.LaunchRequested {
				snapshot.NextAction = "reconcile"
			} else {
				snapshot.NextAction = "launch"
			}
			return
		case TaskRunning:
			snapshot.State = StateRunning
			snapshot.NextAction = "observe"
			return
		case TaskTerminationPending:
			snapshot.State = StateRunning
			snapshot.NextAction = "terminate"
			return
		case TaskTerminated:
			snapshot.State = StateRunning
			snapshot.NextAction = "inspect_candidate"
			return
		case TaskIntegrated:
			// Terminal task evidence is considered below once all tasks are inspected.
		case TaskGateFailed, TaskReviewBlocked:
			snapshot.State = StateRunning
			snapshot.NextAction = "repair"
			return
		case TaskCandidateReady:
			snapshot.State = StateRunning
			snapshot.NextAction = "verify"
			return
		}
	}
	allIntegrated := len(snapshot.TaskStates) > 0
	for _, task := range snapshot.TaskStates {
		if task.Status != TaskIntegrated {
			allIntegrated = false
			break
		}
	}
	if allIntegrated {
		if completionRequiredPending {
			snapshot.State = StatePublicationPending
			snapshot.NextAction = "publish"
			if publicationFailed {
				snapshot.SyncStatus = "failed"
			} else {
				snapshot.SyncStatus = "pending"
			}
			return
		}
		if publicationPending {
			if publicationFailed {
				snapshot.SyncStatus = "failed"
			} else {
				snapshot.SyncStatus = "pending"
			}
		}
		if snapshot.State == StateCompleted && (len(snapshot.Publications) == 0 || publicationSettled) {
			snapshot.NextAction = ""
			return
		}
		snapshot.State = StateReadyForPR
		snapshot.NextAction = "prepare_docs"
		return
	}
	allPending := true
	for _, task := range snapshot.TaskStates {
		if task.Status != TaskPending {
			allPending = false
			break
		}
	}
	if allPending {
		snapshot.State = StateQueued
		snapshot.NextAction = "run"
		return
	}
	snapshot.State = StateQueued
	snapshot.NextAction = "run"
}

func publicationAggregate(snapshot *WorkSnapshot) (pending, failed, conflict, completionRequiredPending, settled bool) {
	if len(snapshot.Publications) == 0 {
		return false, false, false, false, false
	}
	settled = true
	for _, publication := range snapshot.Publications {
		switch publication.Status {
		case PublicationPending:
			settled = false
			pending = true
			if publication.CompletionRequired {
				completionRequiredPending = true
			}
		case PublicationFailed:
			settled = false
			failed = true
			pending = true
			if publication.CompletionRequired {
				completionRequiredPending = true
			}
		case PublicationConflict:
			settled = false
			conflict = true
			if publication.CompletionRequired {
				completionRequiredPending = true
			}
		}
	}
	return
}

func hasActiveInvocation(snapshot *WorkSnapshot) bool {
	for _, task := range snapshot.TaskStates {
		switch task.Status {
		case TaskWorktreePreparing, TaskInvocationReserved, TaskRunning, TaskTerminationPending:
			return true
		}
	}
	return false
}

func hasActiveOrUnknownInvocation(snapshot *WorkSnapshot) bool {
	for _, task := range snapshot.TaskStates {
		switch task.Status {
		case TaskWorktreePreparing, TaskInvocationReserved, TaskRunning, TaskTerminationPending:
			return true
		}
		if task.Invocation != nil {
			if task.Status == TaskTerminated && task.Invocation.TerminationConfirmed && task.Invocation.EndedAt != nil {
				continue
			}
			return true
		}
	}
	return false
}
