package statev2

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
	if snapshot.Control.ApprovedContractHash == "" {
		snapshot.State = StateAwaitingApproval
		snapshot.NextAction = "approve"
		return
	}
	if snapshot.Control.Blocker != nil {
		snapshot.State = StateNeedsOperator
		snapshot.NextAction = "resolve"
		return
	}
	if snapshot.Control.PauseRequested {
		if hasActiveInvocation(snapshot) {
			snapshot.State = StateRunning
			snapshot.NextAction = "terminate"
		} else {
			snapshot.State = StatePaused
			snapshot.NextAction = "resume"
		}
		return
	}
	for _, contractTask := range snapshot.Contract.Tasks {
		task, ok := snapshot.TaskStates[contractTask.TaskID]
		if !ok {
			continue
		}
		switch task.Status {
		case TaskInvocationReserved:
			snapshot.State = StateRunning
			if task.Invocation != nil && task.Invocation.LaunchRequested {
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
		case TaskAccepted:
			snapshot.State = StateReview
			snapshot.NextAction = "integrate"
			return
		case TaskGatePassed:
			snapshot.State = StateReview
			snapshot.NextAction = "review"
			return
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
		case TaskInvocationReserved, TaskRunning, TaskTerminationPending:
			return true
		}
	}
	return false
}

func hasActiveOrUnknownInvocation(snapshot *WorkSnapshot) bool {
	for _, task := range snapshot.TaskStates {
		switch task.Status {
		case TaskInvocationReserved, TaskRunning, TaskTerminationPending:
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
