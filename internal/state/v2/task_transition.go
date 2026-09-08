package statev2

import (
	"encoding/hex"
	"path"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

const (
	roleBuilder  = "builder"
	roleReviewer = "reviewer"
)

func applyTaskTransition(snapshot *WorkSnapshot, transition TaskTransition, requestID contractv2.RequestID) error {
	if transition.At.IsZero() || transition.At.Location() != time.UTC {
		return invalidTransition("task transition time must be a nonzero UTC time")
	}
	if transition.TaskID == "" {
		return invalidTransition("task ID is required")
	}
	task, ok := snapshot.TaskStates[transition.TaskID]
	if !ok {
		return invalidTransition("task is not in contract")
	}
	if snapshot.Contract.WorkID != snapshot.WorkID || task.TaskID != transition.TaskID {
		return invalidTransition("task identity does not match contract")
	}
	if transition.Role != roleBuilder && transition.Role != roleReviewer {
		return invalidTransition("invalid invocation role")
	}
	if snapshot.Control.ApprovedContractHash == "" {
		return invalidTransition("work is not approved")
	}
	if snapshot.Control.Blocker != nil {
		return invalidTransition("work has an operator blocker")
	}
	switch transition.Action {
	case TaskReserveInvocation:
		return reserveInvocation(snapshot, &task, transition, requestID)
	case TaskRecordCandidate:
		if err := recordCandidate(&task, transition); err != nil {
			return err
		}
	case TaskRecordGate:
		if err := recordGate(&task, transition); err != nil {
			return err
		}
	case TaskRecordReview:
		if err := recordReview(&task, transition); err != nil {
			return err
		}
	case TaskRecordIntegration:
		if err := recordIntegration(&task, transition); err != nil {
			return err
		}
	case TaskBeginLaunch:
		if err := beginLaunch(&task, transition); err != nil {
			return err
		}
	case TaskMarkRunning:
		if err := markRunning(&task, transition); err != nil {
			return err
		}
	case TaskRequestTermination:
		if err := requestTermination(&task, transition); err != nil {
			return err
		}
	case TaskConfirmTermination:
		if err := confirmTermination(&task, transition); err != nil {
			return err
		}
	case TaskReconcileNotStarted:
		if err := reconcileNotStarted(&task, transition); err != nil {
			return err
		}
	case TaskNeedsOperatorAction:
		return needsOperator(snapshot, &task, transition)
	default:
		return invalidTransition("unsupported task action %q", transition.Action)
	}
	snapshot.TaskStates[task.TaskID] = task
	reduce(snapshot)
	return nil
}

func reserveInvocation(snapshot *WorkSnapshot, task *TaskExecutionState, transition TaskTransition, requestID contractv2.RequestID) error {
	if snapshot.State == StateAwaitingApproval || snapshot.State == StateDraft || snapshot.State == StatePaused || snapshot.State == StateNeedsOperator || snapshot.State == StateCompleted {
		return invalidTransition("work cannot dispatch in state %q", snapshot.State)
	}
	if snapshot.Control.PauseRequested {
		return invalidTransition("work pause is requested")
	}
	if transition.InvocationID == "" || transition.LogicalWorkID == "" || transition.Invocation == nil || transition.Worktree == nil {
		return invalidTransition("invocation identity and launch inputs are required")
	}
	if strings.TrimSpace(string(transition.InvocationID)) == "" || strings.TrimSpace(string(transition.LogicalWorkID)) == "" {
		return invalidTransition("invocation identity and launch inputs are required")
	}
	for _, invocationID := range task.InvocationHistory {
		if invocationID == transition.InvocationID {
			return invalidTransition("invocation ID was already used")
		}
	}
	if err := validateReservationInputs(transition); err != nil {
		return err
	}
	isRepair := transition.Role == roleBuilder && !transition.Transient && (task.Status == TaskGateFailed || task.Status == TaskReviewBlocked)
	isRecovery := transition.Transient && task.Status == TaskTerminated
	if isRepair {
		return reserveRepair(snapshot, task, transition, requestID)
	}
	if isRecovery {
		return reserveRecovery(snapshot, task, transition, requestID)
	}
	if task.Invocation != nil && !(task.Status == TaskGatePassed && transition.Role == roleReviewer && task.Invocation.TerminationConfirmed && task.Invocation.EndedAt != nil) {
		return invalidTransition("task already has an invocation")
	}
	if task.Status == TaskPending && task.LogicalWork == nil {
		if transition.Role != roleBuilder || transition.ReturnStage != TaskPending || transition.BuilderAttempt != 1 || task.BuilderAttempt != 0 || task.LogicalWork != nil {
			return invalidTransition("invalid initial builder reservation")
		}
		for _, dep := range taskDependencies(snapshot, task.TaskID) {
			if dep.Status != TaskIntegrated {
				return invalidTransition("task dependencies are not integrated")
			}
		}
		task.BuilderAttempt = 1
		task.LogicalWork = &LogicalWorkState{LogicalWorkID: transition.LogicalWorkID, Role: transition.Role, BuilderAttempt: 1, Purpose: "task invocation"}
	} else if task.Status == TaskGatePassed && transition.Role == roleReviewer {
		if transition.ReturnStage != TaskGatePassed || transition.BuilderAttempt != task.BuilderAttempt || task.BuilderAttempt == 0 || task.Candidate == nil || task.Gate == nil || task.LogicalWork == nil {
			return invalidTransition("invalid reviewer reservation")
		}
		if task.LogicalWork.Role == roleReviewer {
			if task.LogicalWork.LogicalWorkID != transition.LogicalWorkID || task.LogicalWork.BuilderAttempt != transition.BuilderAttempt {
				return invalidTransition("reviewer reservation does not match logical work")
			}
		} else {
			if task.LogicalWork.Role != roleBuilder && task.LogicalWork.Role != "" {
				return invalidTransition("reviewer reservation has invalid prior logical work")
			}
			if task.LogicalWork.LogicalWorkID == transition.LogicalWorkID {
				return invalidTransition("reviewer reservation must use a new logical work")
			}
			// Initial reviewer work is distinct while candidate and gate evidence
			// remain attached to the task.
			task.LogicalWork = &LogicalWorkState{LogicalWorkID: transition.LogicalWorkID, Role: transition.Role, BuilderAttempt: task.BuilderAttempt, Purpose: "task invocation"}
		}
		task.Invocation = nil
	} else {
		if task.LogicalWork == nil || task.LogicalWork.LogicalWorkID != transition.LogicalWorkID || task.LogicalWork.Role != transition.Role || task.LogicalWork.BuilderAttempt != transition.BuilderAttempt || task.BuilderAttempt != transition.BuilderAttempt {
			return invalidTransition("reservation does not match logical work")
		}
		if transition.ReturnStage != task.LogicalWorkReturnStage() {
			return invalidTransition("reservation return stage does not match task")
		}
		if !reservationStage(task.Status, transition.Role, transition.ReturnStage) {
			return invalidTransition("task is not at a reservable stage")
		}
	}
	if transition.Role == roleReviewer && (task.Status != TaskGatePassed || transition.ReturnStage != TaskGatePassed) {
		return invalidTransition("reviewer requires gate_passed task")
	}
	if transition.Role == roleBuilder && task.Status != TaskPending && task.Status != TaskGateFailed && task.Status != TaskReviewBlocked {
		return invalidTransition("builder task is not at a reservable stage")
	}
	invocation := *transition.Invocation
	invocation.InvocationID = transition.InvocationID
	invocation.LogicalWorkID = transition.LogicalWorkID
	invocation.Role = transition.Role
	invocation.ReturnStage = transition.ReturnStage
	invocation.TransitionRequestID = requestID
	task.Worktree = cloneWorktree(transition.Worktree)
	task.Invocation = &invocation
	task.InvocationHistory = append(task.InvocationHistory, transition.InvocationID)
	task.Status = TaskInvocationReserved
	snapshot.TaskStates[task.TaskID] = *task
	reduce(snapshot)
	return nil
}

func validateReservationInputs(transition TaskTransition) error {
	if strings.TrimSpace(transition.Invocation.LogicalProfile) == "" || strings.TrimSpace(transition.Invocation.RuntimeFingerprint) == "" {
		return invalidTransition("logical profile and runtime fingerprint are required")
	}
	if strings.TrimSpace(transition.Worktree.CanonicalPath) == "" || strings.TrimSpace(transition.Worktree.GitCommonDir) == "" || strings.TrimSpace(transition.Worktree.Branch) == "" || strings.TrimSpace(transition.Worktree.BaseSHA) == "" {
		return invalidTransition("complete worktree identity is required")
	}
	if transition.Invocation.ProviderIdentity != "" || transition.Invocation.ProviderSession != "" || transition.Invocation.ProviderPane != "" || transition.Invocation.ProviderProcess != "" {
		return invalidTransition("provider identity is not accepted when reserving")
	}
	if transition.Invocation.StartedAt != nil || transition.Invocation.EndedAt != nil || transition.Invocation.LaunchRequested || transition.Invocation.TerminationConfirmed {
		return invalidTransition("invalid initial invocation state")
	}
	return nil
}

func reserveRepair(snapshot *WorkSnapshot, task *TaskExecutionState, transition TaskTransition, requestID contractv2.RequestID) error {
	if transition.InvocationID == "" || transition.LogicalWorkID == "" || transition.Invocation == nil || transition.Worktree == nil || transition.BuilderAttempt != task.BuilderAttempt+1 || transition.ReturnStage != task.Status {
		return invalidTransition("invalid repair reservation")
	}
	if task.LogicalWork == nil || task.LogicalWork.Role != roleBuilder || task.LogicalWork.BuilderAttempt != task.BuilderAttempt {
		return invalidTransition("repair reservation has invalid prior logical work")
	}
	if task.RepairCount >= task.RepairLimit {
		blocker := OperatorBlocker{Kind: BlockerKindRepairBudgetExhausted, TaskID: task.TaskID, Diagnostic: "repair budget exhausted; remaining=0"}
		if transition.Blocker != nil {
			blocker.OperatorRef = transition.Blocker.OperatorRef
		}
		snapshot.Control.Blocker = &blocker
		task.Status = TaskNeedsOperator
		snapshot.TaskStates[task.TaskID] = *task
		reduce(snapshot)
		return nil
	}
	if len(task.PriorAttempts) >= MaxPriorAttempts {
		return invalidTransition("prior attempt summary limit reached")
	}
	summary := AttemptSummary{BuilderAttempt: task.BuilderAttempt, Outcome: string(task.Status)}
	if task.Candidate != nil {
		summary.CandidateSHA, summary.TreeSHA = task.Candidate.CandidateSHA, task.Candidate.TreeSHA
		summary.Diagnostic = task.Candidate.Diagnostic
	}
	if task.Gate != nil && task.Gate.Diagnostic != "" {
		summary.FailureReason, summary.Diagnostic = "gate_failed", task.Gate.Diagnostic
	}
	if task.Review != nil && task.Review.Diagnostic != "" {
		summary.FailureReason, summary.Diagnostic = "review_blocked", task.Review.Diagnostic
	}
	task.PriorAttempts = append(task.PriorAttempts, summary)
	task.RepairCount++
	task.BuilderAttempt++
	task.Candidate, task.Gate, task.Review, task.Integration = nil, nil, nil, nil
	task.LogicalWork = &LogicalWorkState{LogicalWorkID: transition.LogicalWorkID, Role: roleBuilder, BuilderAttempt: task.BuilderAttempt, Purpose: "task invocation", RepairCount: task.RepairCount, RecoveryCount: task.RecoveryCount, RepairBudgetDebited: true}
	task.Invocation = nil
	if err := installInvocation(task, transition, requestID); err != nil {
		return err
	}
	snapshot.TaskStates[task.TaskID] = *task
	reduce(snapshot)
	return nil
}

func reserveRecovery(snapshot *WorkSnapshot, task *TaskExecutionState, transition TaskTransition, requestID contractv2.RequestID) error {
	if task.Invocation == nil || !task.Invocation.TerminationConfirmed || task.Invocation.EndedAt == nil || !task.Invocation.TransientFailure || transition.InvocationID == task.Invocation.InvocationID || transition.Role != task.Invocation.Role || transition.LogicalWorkID != task.Invocation.LogicalWorkID || transition.BuilderAttempt != task.BuilderAttempt || transition.ReturnStage != task.Invocation.ReturnStage {
		return invalidTransition("invalid recovery reservation")
	}
	if task.RecoveryCount >= task.RecoveryLimit {
		blocker := OperatorBlocker{Kind: BlockerKindRecoveryBudgetExhausted, TaskID: task.TaskID, InvocationID: task.Invocation.InvocationID, Diagnostic: "recovery budget exhausted; remaining=0"}
		if transition.Blocker != nil {
			blocker.OperatorRef = transition.Blocker.OperatorRef
		}
		snapshot.Control.Blocker = &blocker
		task.Status = TaskNeedsOperator
		snapshot.TaskStates[task.TaskID] = *task
		reduce(snapshot)
		return nil
	}
	task.RecoveryCount++
	if task.LogicalWork != nil {
		task.LogicalWork.RecoveryCount = task.RecoveryCount
		task.LogicalWork.RecoveryBudgetDebited = true
	}
	task.Invocation = nil
	if err := installInvocation(task, transition, requestID); err != nil {
		return err
	}
	snapshot.TaskStates[task.TaskID] = *task
	reduce(snapshot)
	return nil
}

func installInvocation(task *TaskExecutionState, transition TaskTransition, requestID contractv2.RequestID) error {
	invocation := *transition.Invocation
	invocation.InvocationID = transition.InvocationID
	invocation.LogicalWorkID = transition.LogicalWorkID
	invocation.Role = transition.Role
	invocation.ReturnStage = transition.ReturnStage
	invocation.TransitionRequestID = requestID
	task.Worktree = cloneWorktree(transition.Worktree)
	task.Invocation = &invocation
	task.InvocationHistory = append(task.InvocationHistory, transition.InvocationID)
	task.Status = TaskInvocationReserved
	return nil
}

func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func recordCandidate(task *TaskExecutionState, transition TaskTransition) error {
	if transition.Role != roleBuilder || transition.Transient || task.Status != TaskTerminated || task.Invocation == nil || task.Invocation.Role != roleBuilder || !task.Invocation.TerminationConfirmed || task.Invocation.EndedAt == nil || task.Invocation.TransientFailure || transition.InvocationID != task.Invocation.InvocationID || transition.LogicalWorkID != task.Invocation.LogicalWorkID || transition.BuilderAttempt != task.BuilderAttempt || transition.ReturnStage != TaskPending || transition.Candidate == nil {
		return invalidTransition("candidate requires a normal terminated builder invocation")
	}
	c := transition.Candidate
	if c.BuilderAttempt != task.BuilderAttempt || !validSHA(c.CandidateSHA) || !validSHA(c.TreeSHA) || c.ChangedFiles == nil || len(c.ChangedFiles) == 0 {
		return invalidTransition("candidate evidence does not match current attempt")
	}
	seen := map[string]bool{}
	for _, path := range c.ChangedFiles {
		if !validRepositoryPath(path) || seen[path] {
			return invalidTransition("candidate changed files are not repository-relative and unique")
		}
		seen[path] = true
	}
	if err := validateDiagnostic(c.Diagnostic); err != nil {
		return invalidTransition("candidate diagnostic: %v", err)
	}
	clone := *c
	clone.ChangedFiles = append([]string(nil), c.ChangedFiles...)
	task.Candidate = &clone
	task.Status = TaskCandidateReady
	return nil
}

func validRepositoryPath(value string) bool {
	if strings.TrimSpace(value) != value || value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value || value == "." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return false
	}
	return true
}

func recordGate(task *TaskExecutionState, transition TaskTransition) error {
	if transition.Role != roleBuilder || task.Status != TaskCandidateReady || task.Candidate == nil || transition.BuilderAttempt != task.BuilderAttempt || transition.Gate == nil {
		return invalidTransition("gate requires current candidate")
	}
	g := transition.Gate
	if g.BuilderAttempt != task.BuilderAttempt || g.CandidateSHA != task.Candidate.CandidateSHA || g.ObservedAt.IsZero() || g.ObservedAt.Location() != time.UTC || len(g.Commands) == 0 || len(g.Commands) != len(g.Outcomes) {
		return invalidTransition("gate evidence does not match candidate")
	}
	for i := range g.Commands {
		if strings.TrimSpace(g.Commands[i]) != g.Commands[i] || strings.TrimSpace(g.Outcomes[i]) != g.Outcomes[i] || g.Commands[i] == "" || g.Outcomes[i] == "" {
			return invalidTransition("gate command and outcome must be trimmed")
		}
	}
	if err := validateDiagnostic(g.Diagnostic); err != nil {
		return invalidTransition("gate diagnostic: %v", err)
	}
	clone := *g
	clone.Commands = append([]string(nil), g.Commands...)
	clone.Outcomes = append([]string(nil), g.Outcomes...)
	task.Gate = &clone
	if g.Passed {
		task.Status = TaskGatePassed
	} else {
		task.Status = TaskGateFailed
	}
	return nil
}

func recordReview(task *TaskExecutionState, transition TaskTransition) error {
	if transition.Role != roleReviewer || task.Status != TaskTerminated || task.Invocation == nil || task.Invocation.Role != roleReviewer || !task.Invocation.TerminationConfirmed || task.Invocation.EndedAt == nil || task.Invocation.TransientFailure || transition.InvocationID != task.Invocation.InvocationID || transition.LogicalWorkID != task.Invocation.LogicalWorkID || transition.BuilderAttempt != task.BuilderAttempt || transition.ReturnStage != TaskGatePassed || task.Candidate == nil || task.Gate == nil || !task.Gate.Passed || transition.Review == nil {
		return invalidTransition("review requires a normal terminated reviewer invocation")
	}
	r := transition.Review
	if r.ReviewerInvocationID != task.Invocation.InvocationID || r.BuilderAttempt != task.BuilderAttempt || r.CandidateSHA != task.Candidate.CandidateSHA || r.ReviewSHA != task.Candidate.CandidateSHA || r.ObservedAt.IsZero() || r.ObservedAt.Location() != time.UTC || r.Findings == nil {
		return invalidTransition("review evidence does not match candidate")
	}
	blocking := 0
	for _, f := range r.Findings {
		if err := validateDiagnostic(f.Diagnostic); err != nil {
			return invalidTransition("review finding: %v", err)
		}
		if strings.TrimSpace(f.Code) == "" {
			return invalidTransition("review finding code is required")
		}
		if strings.EqualFold(f.Severity, "blocking") {
			blocking++
		}
	}
	if r.Accepted && blocking != 0 {
		return invalidTransition("accepted review has blocking findings")
	}
	if !r.Accepted && len(r.Findings) == 0 {
		return invalidTransition("blocked review requires a finding")
	}
	if err := validateDiagnostic(r.Diagnostic); err != nil {
		return invalidTransition("review diagnostic: %v", err)
	}
	clone := *r
	if r.Findings != nil {
		clone.Findings = append([]ReviewFinding{}, r.Findings...)
	}
	task.Review = &clone
	if r.Accepted {
		task.Status = TaskAccepted
	} else {
		task.Status = TaskReviewBlocked
	}
	return nil
}

func recordIntegration(task *TaskExecutionState, transition TaskTransition) error {
	if transition.Role != roleBuilder || task.Status != TaskAccepted || task.Candidate == nil || task.Review == nil || !task.Review.Accepted || transition.BuilderAttempt != task.BuilderAttempt || transition.Integration == nil {
		return invalidTransition("integration requires accepted review")
	}
	i := transition.Integration
	if i.BuilderAttempt != task.BuilderAttempt || i.CandidateSHA != task.Candidate.CandidateSHA || !validSHA(i.IntegrationHEAD) || i.IntegrationHEAD == task.Candidate.CandidateSHA || !i.RelationVerified || i.ObservedAt.IsZero() || i.ObservedAt.Location() != time.UTC {
		return invalidTransition("integration evidence does not match candidate")
	}
	if err := validateDiagnostic(i.Diagnostic); err != nil {
		return invalidTransition("integration diagnostic: %v", err)
	}
	clone := *i
	task.Integration = &clone
	task.Status = TaskIntegrated
	return nil
}

func taskDependencies(snapshot *WorkSnapshot, id contractv2.TaskID) []TaskExecutionState {
	for _, contractTask := range snapshot.Contract.Tasks {
		if contractTask.TaskID != id {
			continue
		}
		deps := make([]TaskExecutionState, 0, len(contractTask.DependsOn))
		for _, depID := range contractTask.DependsOn {
			if dep, ok := snapshot.TaskStates[depID]; ok {
				deps = append(deps, dep)
			}
		}
		return deps
	}
	return nil
}

func reservationStage(status TaskStatus, role string, stage TaskStatus) bool {
	if role == roleReviewer {
		return status == TaskGatePassed && stage == TaskGatePassed
	}
	return (status == TaskPending && stage == TaskPending) ||
		(status == TaskGateFailed && stage == TaskGateFailed) ||
		(status == TaskReviewBlocked && stage == TaskReviewBlocked)
}

func (task *TaskExecutionState) LogicalWorkReturnStage() TaskStatus {
	if task.LogicalWork == nil {
		return ""
	}
	switch task.Status {
	case TaskPending:
		return TaskPending
	case TaskGateFailed:
		return TaskGateFailed
	case TaskReviewBlocked:
		return TaskReviewBlocked
	case TaskGatePassed:
		return TaskGatePassed
	default:
		return task.InvocationReturnStage()
	}
}

func (task *TaskExecutionState) InvocationReturnStage() TaskStatus {
	if task.Invocation == nil {
		return ""
	}
	return task.Invocation.ReturnStage
}

func beginLaunch(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskInvocationReserved); err != nil {
		return err
	}
	if task.Invocation.LaunchRequested {
		return invalidTransition("launch was already requested")
	}
	task.Invocation.LaunchRequested = true
	return nil
}

func markRunning(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskInvocationReserved); err != nil {
		return err
	}
	if !task.Invocation.LaunchRequested {
		return invalidTransition("launch has not been requested")
	}
	if task.Invocation.StartedAt != nil || task.Invocation.EndedAt != nil {
		return invalidTransition("invocation timing is already recorded")
	}
	if transition.Invocation == nil || transition.Invocation.ProviderIdentity == "" && transition.Invocation.ProviderSession == "" && transition.Invocation.ProviderPane == "" && transition.Invocation.ProviderProcess == "" {
		return invalidTransition("provider identity is required")
	}
	provider := transition.Invocation
	task.Invocation.ProviderIdentity = provider.ProviderIdentity
	task.Invocation.ProviderSession = provider.ProviderSession
	task.Invocation.ProviderPane = provider.ProviderPane
	task.Invocation.ProviderProcess = provider.ProviderProcess
	at := transition.At
	task.Invocation.StartedAt = &at
	task.Status = TaskRunning
	return nil
}

func requestTermination(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskRunning); err != nil {
		return err
	}
	if strings.TrimSpace(transition.Reason) == "" || len([]byte(transition.Reason)) > MaxDiagnosticBytes {
		return invalidTransition("bounded termination reason is required")
	}
	task.Invocation.TerminationReason = transition.Reason
	task.Status = TaskTerminationPending
	return nil
}

func confirmTermination(task *TaskExecutionState, transition TaskTransition) error {
	if task.Status != TaskRunning && task.Status != TaskTerminationPending {
		return invalidTransition("task is not running")
	}
	if err := matchInvocation(task, transition, task.Status); err != nil {
		return err
	}
	if task.Invocation.TerminationConfirmed || task.Invocation.EndedAt != nil {
		return invalidTransition("invocation termination is already confirmed")
	}
	if transition.Reason != "" && len([]byte(transition.Reason)) > MaxDiagnosticBytes {
		return invalidTransition("termination reason exceeds limit")
	}
	at := transition.At
	task.Invocation.EndedAt = &at
	task.Invocation.TerminationConfirmed = true
	if transition.Transient {
		task.Invocation.TransientFailure = true
	}
	if transition.Reason != "" {
		task.Invocation.TerminationReason = transition.Reason
	}
	task.Status = TaskTerminated
	return nil
}

func reconcileNotStarted(task *TaskExecutionState, transition TaskTransition) error {
	if err := matchInvocation(task, transition, TaskInvocationReserved); err != nil {
		return err
	}
	if task.Invocation.LaunchRequested || hasProviderIdentity(task.Invocation) || transition.Resolution == nil || !transition.Resolution.OwnerTerminated || transition.Resolution.LaunchRequested || !transition.Resolution.ProviderAbsent {
		return invalidTransition("positive no-launch proof is required")
	}
	if err := validateDiagnostic(transition.Resolution.Diagnostic); err != nil {
		return invalidTransition("resolution diagnostic: %v", err)
	}
	if len(task.PriorAttempts) >= MaxPriorAttempts {
		return invalidTransition("prior attempt summary limit reached")
	}
	task.PriorAttempts = append(task.PriorAttempts, AttemptSummary{BuilderAttempt: task.BuilderAttempt, Outcome: "abandoned_not_started", FailureReason: transition.Reason, Diagnostic: transition.Resolution.Diagnostic})
	return restoreAfterReconcile(task)
}

func restoreAfterReconcile(task *TaskExecutionState) error {
	if task.Invocation == nil || task.LogicalWork == nil {
		return invalidTransition("invocation logical work is missing")
	}
	stage := task.Invocation.ReturnStage
	if !validReturnStage(task.Invocation.Role, stage) {
		return invalidTransition("invalid invocation return stage")
	}
	task.Status = stage
	task.Invocation = nil
	return nil
}

func needsOperator(snapshot *WorkSnapshot, task *TaskExecutionState, transition TaskTransition) error {
	blocker := transition.Blocker
	if blocker == nil || strings.TrimSpace(blocker.Kind) == "" || strings.TrimSpace(blocker.OperatorRef) == "" || strings.TrimSpace(blocker.Diagnostic) == "" || len([]byte(blocker.Diagnostic)) > MaxDiagnosticBytes || blocker.TaskID != task.TaskID {
		return invalidTransition("invalid task operator blocker")
	}
	if task.Invocation == nil {
		if blocker.InvocationID != "" || transition.InvocationID != "" {
			return invalidTransition("operator blocker invocation does not match task")
		}
	} else if blocker.InvocationID != task.Invocation.InvocationID || transition.InvocationID != task.Invocation.InvocationID {
		return invalidTransition("operator blocker invocation does not match task")
	}
	if task.Invocation != nil {
		if transition.ReturnStage != task.Invocation.ReturnStage || transition.BuilderAttempt != task.BuilderAttempt {
			return invalidTransition("operator blocker stage does not match task")
		}
	} else if !validReturnStage(transition.Role, transition.ReturnStage) || transition.BuilderAttempt != task.BuilderAttempt {
		return invalidTransition("operator blocker stage does not match task")
	}
	copy := *blocker
	snapshot.Control.Blocker = &copy
	task.Status = TaskNeedsOperator
	snapshot.TaskStates[task.TaskID] = *task
	reduce(snapshot)
	return nil
}

func matchInvocation(task *TaskExecutionState, transition TaskTransition, status TaskStatus) error {
	if task.Status != status {
		return invalidTransition("task status %q does not allow action", task.Status)
	}
	if task.Invocation == nil || transition.InvocationID == "" || task.Invocation.InvocationID != transition.InvocationID || task.Invocation.LogicalWorkID != transition.LogicalWorkID || task.Invocation.Role != transition.Role || task.BuilderAttempt != transition.BuilderAttempt {
		return invalidTransition("invocation identity does not match task")
	}
	if !validReturnStage(task.Invocation.Role, task.Invocation.ReturnStage) {
		return invalidTransition("invalid invocation return stage")
	}
	if transition.ReturnStage != task.Invocation.ReturnStage {
		return invalidTransition("invocation return stage does not match task")
	}
	return nil
}

func validReturnStage(role string, stage TaskStatus) bool {
	if role == roleReviewer {
		return stage == TaskGatePassed
	}
	return role == roleBuilder && (stage == TaskPending || stage == TaskGateFailed || stage == TaskReviewBlocked)
}

func hasProviderIdentity(invocation *InvocationState) bool {
	return invocation != nil && (invocation.ProviderIdentity != "" || invocation.ProviderSession != "" || invocation.ProviderPane != "" || invocation.ProviderProcess != "")
}

func cloneWorktree(worktree *WorktreeIdentity) *WorktreeIdentity {
	if worktree == nil {
		return nil
	}
	clone := *worktree
	return &clone
}
