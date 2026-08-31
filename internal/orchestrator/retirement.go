package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	retirementpolicy "thread-dock/internal/retirement"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

const (
	retirementAgentAction     = "retirement_agent:"
	retirementGitAction       = "retirement_git:"
	retirementWorkspaceAction = "retirement_workspace:"
	retirementCloseAction     = "retirement_close:"
)

// BeginRetirement creates (or resumes) one durable session-retirement plan.
// It performs no Herdr or Git operation; callers advance the plan through the
// normal one-action Advance method.
func (o *Orchestrator) BeginRetirement(ctx context.Context, id contract.RunID, targetPhase contract.RunPhase, automatic bool) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := o.validateStore(); err != nil {
		return err
	}
	release, err := o.claim(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	snapshot, err := o.deps.Store.Load(ctx, id)
	if err != nil {
		return err
	}
	return o.beginRetirementSnapshot(ctx, &snapshot, targetPhase, automatic)
}

// beginRetirementSnapshot is also used by the terminal parallel merge hook,
// which already owns the run lease. Keeping the save in this helper makes the
// merge result and retirement plan durable before any retirement call.
func (o *Orchestrator) beginRetirementSnapshot(ctx context.Context, snapshot *state.RunSnapshot, targetPhase contract.RunPhase, automatic bool) error {
	if snapshot == nil {
		return errors.New("retirement snapshot is nil")
	}
	if snapshot.Retirement.Status == "retired" {
		return nil
	}
	if snapshot.Retirement.Status == "needs_operator" {
		return errors.New("retirement requires operator")
	}
	if snapshot.Phase == contract.PhaseRetiring && snapshot.Retirement.Status != "" {
		// A restart or repeated command resumes the exact existing plan. Never
		// derive a new target list after a close intent has been persisted.
		return nil
	}
	if targetPhase != contract.PhaseCompleted && targetPhase != contract.PhaseBlocked {
		return fmt.Errorf("retirement target phase %q is not terminal", targetPhase)
	}
	if snapshot.Phase != targetPhase && snapshot.Phase != contract.PhaseRetiring {
		return fmt.Errorf("run phase %q cannot begin retirement", snapshot.Phase)
	}
	plan, err := retirementpolicy.Build(*snapshot, automatic, targetPhase)
	if err != nil {
		// An automatic terminal hook runs immediately after a provider merge.
		// Preserve that durable outcome even when a malformed target prevents a
		// retirement plan; the merge must never be replayed or hidden.
		if automatic && targetPhase == contract.PhaseCompleted && snapshot.Phase == contract.PhaseCompleted && strings.TrimSpace(snapshot.MergeSHA) != "" {
			snapshot.Retirement = state.RetirementState{Status: "needs_operator", TargetPhase: targetPhase, Automatic: true, UpdatedAt: o.now()}
			snapshot.Phase = contract.PhaseRetiring
			snapshot.PendingAction = ""
			snapshot.PendingTaskID = ""
			snapshot.Summary = "retirement target identity is incomplete: " + err.Error()
			snapshot.UpdatedAt = o.now()
			if saveErr := o.deps.Store.Save(ctx, *snapshot); saveErr != nil {
				return saveErr
			}
			if appendErr := o.append(ctx, snapshot.RunID, state.Event{Type: "retirement_needs_operator", Phase: snapshot.Phase, Message: snapshot.Summary}); appendErr != nil {
				return appendErr
			}
			return nil
		}
		return err
	}
	snapshot.Retirement = plan
	snapshot.Phase = contract.PhaseRetiring
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.Summary = "Execution sessions are retiring"
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{
		Type: "intent", Kind: "retirement_prepare", Phase: contract.PhaseRetiring,
		Message: "retirement_prepare", Data: map[string]any{
			"automatic": automatic, "targetPhase": targetPhase, "targets": len(plan.Targets),
		},
	})
}

// BeginRetirement routes explicit retirement requests through the selected
// persisted strategy when the caller uses Auto.
func (a *Auto) BeginRetirement(ctx context.Context, id contract.RunID, targetPhase contract.RunPhase, automatic bool) error {
	selected, err := a.selected(ctx, id)
	if err != nil {
		return err
	}
	return selected.BeginRetirement(ctx, id, targetPhase, automatic)
}

func (o *Orchestrator) advanceRetirement(ctx context.Context, snapshot *state.RunSnapshot) error {
	if snapshot.Retirement.Status == "needs_operator" {
		return ErrRunFinished
	}
	if snapshot.PendingAction != "" {
		return o.reconcileRetirementPending(ctx, snapshot)
	}
	decision := retirementpolicy.Next(snapshot.Retirement, nil)
	switch decision.Kind {
	case retirementpolicy.Complete:
		return o.finishRetirement(ctx, snapshot)
	case retirementpolicy.NeedsOperator:
		return o.retirementNeedsOperator(ctx, snapshot, decision)
	case retirementpolicy.ObserveAgent:
		return o.observeRetirementAgent(ctx, snapshot, decision)
	case retirementpolicy.ProveGit:
		return o.proveRetirementGit(ctx, snapshot, decision)
	case retirementpolicy.ObserveWorkspace:
		return o.observeRetirementWorkspace(ctx, snapshot, decision)
	case retirementpolicy.CloseWorkspace:
		return o.closeRetirementWorkspace(ctx, snapshot, decision)
	case retirementpolicy.MarkRetired, retirementpolicy.RecordWorkspace:
		return o.retirementNeedsOperator(ctx, snapshot, decision)
	default:
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: decision.TargetKey, Reason: "unknown retirement decision"})
	}
}

// completeOrBeginRetirement is the single terminal transition used by the
// parallel merge hooks. Automatic runs persist their complete retirement plan
// while the run is still in PhaseRetiring; they never write a completed
// snapshot first and then attempt to retrofit retirement state.
func (o *Orchestrator) completeOrBeginRetirement(ctx context.Context, snapshot *state.RunSnapshot, message string, terminalEvent *state.Event) error {
	if snapshot == nil {
		return errors.New("retirement snapshot is nil")
	}
	if o.deps.AutoRetireCompletedSessions {
		plan, err := retirementpolicy.Build(*snapshot, true, contract.PhaseCompleted)
		if err != nil {
			snapshot.Retirement = state.RetirementState{Status: "needs_operator", TargetPhase: contract.PhaseCompleted, Automatic: true, UpdatedAt: o.now()}
			snapshot.Phase = contract.PhaseRetiring
			snapshot.PendingAction, snapshot.PendingTaskID = "", ""
			snapshot.Summary = "retirement target identity is incomplete: " + err.Error()
			snapshot.UpdatedAt = o.now()
			if saveErr := o.deps.Store.Save(ctx, *snapshot); saveErr != nil {
				return saveErr
			}
			return o.append(ctx, snapshot.RunID, state.Event{Type: "retirement_needs_operator", Phase: contract.PhaseRetiring, Message: snapshot.Summary})
		}
		snapshot.Retirement = plan
		snapshot.Phase = contract.PhaseRetiring
		snapshot.PendingAction, snapshot.PendingTaskID = "", ""
		snapshot.Summary = "Execution sessions are retiring"
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		if terminalEvent != nil {
			terminalEvent.Phase = contract.PhaseRetiring
			if err := o.append(ctx, snapshot.RunID, *terminalEvent); err != nil {
				return err
			}
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Kind: "retirement_prepare", Phase: contract.PhaseRetiring, Message: "retirement_prepare", Data: map[string]any{"automatic": true, "targetPhase": contract.PhaseCompleted, "targets": len(plan.Targets)}})
	}

	snapshot.Retirement = state.RetirementState{Status: "active", TargetPhase: contract.PhaseCompleted, Automatic: false, UpdatedAt: o.now()}
	snapshot.Phase = contract.PhaseCompleted
	snapshot.PendingAction, snapshot.PendingTaskID = "", ""
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	if terminalEvent != nil {
		terminalEvent.Phase = contract.PhaseCompleted
		if err := o.append(ctx, snapshot.RunID, *terminalEvent); err != nil {
			return err
		}
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "completed", Phase: contract.PhaseCompleted, Message: message})
}

func (o *Orchestrator) observeRetirementAgent(ctx context.Context, snapshot *state.RunSnapshot, decision retirementpolicy.Decision) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "Agent observation port is unavailable"))
	}
	target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement target is missing"))
	}
	if err := o.prepareRetirementAction(ctx, snapshot, retirementAgentAction+target.Key, "retirement agent observation", decision.TargetKey); err != nil {
		return err
	}
	info, err := locator.GetInfo(ctx, target.AgentName)
	if errors.Is(err, herdr.ErrAgentNotFound) {
		return o.applyRetirementObservation(ctx, snapshot, retirementpolicy.Observation{TargetKey: target.Key, AgentState: "done"}, decision, "Agent missing after terminal outcome; Git proof may proceed")
	}
	if err != nil {
		return err
	}
	if reason := retirementAgentIdentityMismatch(*snapshot, target, info); reason != "" {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, reason))
	}
	return o.applyRetirementObservation(ctx, snapshot, retirementpolicy.Observation{TargetKey: target.Key, AgentState: string(info.State)}, decision, "retirement agent observation recorded")
}

func (o *Orchestrator) proveRetirementGit(ctx context.Context, snapshot *state.RunSnapshot, decision retirementpolicy.Decision) error {
	inspector := o.deps.RetirementGitInspector
	var ok bool
	if inspector == nil {
		inspector, ok = o.deps.Git.(RetirementGitInspector)
		if !ok {
			inspector, ok = o.deps.Worktree.(RetirementGitInspector)
		}
	} else {
		ok = true
	}
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement Git inspection port is unavailable"))
	}
	target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement target is missing"))
	}
	if err := o.prepareRetirementAction(ctx, snapshot, retirementGitAction+target.Key, "retirement Git proof", target.Key); err != nil {
		return err
	}
	proof, err := inspector.InspectRetirementTarget(ctx, snapshot.RepositoryPath, target.Path, target.Branch, target.HeadSHA)
	if err != nil {
		if errors.Is(err, worktree.ErrUnsafeTarget) {
			return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement Git identity is unsafe"))
		}
		return err
	}
	if !validRetirementProof(proof) {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement Git proof is not canonical"))
	}
	observation := retirementpolicy.Observation{TargetKey: target.Key, GitProven: true, RepositoryCommonDir: proof.RepositoryCommonDir, Path: proof.Path, Branch: proof.Branch, HeadSHA: proof.HeadSHA}
	return o.applyRetirementObservation(ctx, snapshot, observation, decision, "retirement Git proof recorded")
}

func (o *Orchestrator) observeRetirementWorkspace(ctx context.Context, snapshot *state.RunSnapshot, decision retirementpolicy.Decision) error {
	reader := o.deps.WorkspaceReader
	var ok bool
	if reader == nil {
		reader, ok = o.deps.Herdr.(WorkspaceReader)
	} else {
		ok = true
	}
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "Workspace observation port is unavailable"))
	}
	target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement target is missing"))
	}
	if err := o.prepareRetirementAction(ctx, snapshot, retirementWorkspaceAction+target.Key, "retirement Workspace observation", target.Key); err != nil {
		return err
	}
	info, found, err := reader.GetWorkspace(ctx, target.WorkspaceID)
	if err != nil {
		return err
	}
	observation := retirementpolicy.Observation{TargetKey: target.Key, WorkspaceObserved: true, WorkspaceFound: found}
	if found {
		observation.WorkspaceID, observation.PaneID, observation.Path, observation.WorkspaceState = info.WorkspaceID, info.RootPaneID, info.Path, string(info.State)
	}
	return o.applyRetirementObservation(ctx, snapshot, observation, decision, "retirement Workspace observation recorded")
}

func (o *Orchestrator) closeRetirementWorkspace(ctx context.Context, snapshot *state.RunSnapshot, decision retirementpolicy.Decision) error {
	closer := o.deps.WorkspaceCloser
	var ok bool
	if closer == nil {
		closer, ok = o.deps.Herdr.(WorkspaceCloser)
	} else {
		ok = true
	}
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "Workspace close port is unavailable"))
	}
	target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement target is missing"))
	}
	// The closing status and intent are durable before CloseWorkspace. The
	// response is deliberately never interpreted as proof of closure.
	if err := setRetirementTargetStatus(&snapshot.Retirement, target.Key, "closing"); err != nil {
		return err
	}
	if err := o.prepareRetirementAction(ctx, snapshot, retirementCloseAction+target.Key, "retirement Workspace close", target.Key); err != nil {
		return err
	}
	if err := closer.CloseWorkspace(ctx, target.WorkspaceID); err != nil {
		// PendingAction remains the durable close intent. A later Advance reads
		// Workspace state and can distinguish response loss from a failed close.
		return err
	}
	return nil
}

func (o *Orchestrator) reconcileRetirementPending(ctx context.Context, snapshot *state.RunSnapshot) error {
	action := snapshot.PendingAction
	if strings.HasPrefix(action, retirementCloseAction) || strings.HasPrefix(action, retirementWorkspaceAction) {
		return o.reconcileRetirementWorkspace(ctx, snapshot, retirementActionKey(action))
	}
	if strings.HasPrefix(action, retirementAgentAction) {
		return o.reconcileRetirementAgent(ctx, snapshot, retirementActionKey(action))
	}
	if strings.HasPrefix(action, retirementGitAction) {
		return o.reconcileRetirementGit(ctx, snapshot, retirementActionKey(action))
	}
	return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{Reason: "unknown retirement pending action"})
}

func (o *Orchestrator) reconcileRetirementAgent(ctx context.Context, snapshot *state.RunSnapshot, key string) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "Agent observation port is unavailable"})
	}
	target, ok := retirementTarget(snapshot.Retirement, key)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "retirement target is missing"})
	}
	info, err := locator.GetInfo(ctx, target.AgentName)
	if errors.Is(err, herdr.ErrAgentNotFound) {
		return o.applyRetirementObservation(ctx, snapshot, retirementpolicy.Observation{TargetKey: key, AgentState: "done"}, retirementpolicy.Decision{TargetKey: key}, "Agent missing after terminal outcome; Git proof may proceed")
	}
	if err != nil {
		return err
	}
	if reason := retirementAgentIdentityMismatch(*snapshot, target, info); reason != "" {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: reason})
	}
	return o.applyRetirementObservation(ctx, snapshot, retirementpolicy.Observation{TargetKey: key, AgentState: string(info.State)}, retirementpolicy.Decision{TargetKey: key}, "retirement agent intent reconciled")
}

func (o *Orchestrator) reconcileRetirementGit(ctx context.Context, snapshot *state.RunSnapshot, key string) error {
	inspector := o.deps.RetirementGitInspector
	var ok bool
	if inspector == nil {
		inspector, ok = o.deps.Git.(RetirementGitInspector)
		if !ok {
			inspector, ok = o.deps.Worktree.(RetirementGitInspector)
		}
	} else {
		ok = true
	}
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "retirement Git inspection port is unavailable"})
	}
	target, ok := retirementTarget(snapshot.Retirement, key)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "retirement target is missing"})
	}
	proof, err := inspector.InspectRetirementTarget(ctx, snapshot.RepositoryPath, target.Path, target.Branch, target.HeadSHA)
	if err != nil {
		if errors.Is(err, worktree.ErrUnsafeTarget) {
			return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "retirement Git identity is unsafe"})
		}
		return err
	}
	if !validRetirementProof(proof) {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "retirement Git proof is not canonical"})
	}
	return o.applyRetirementObservation(ctx, snapshot, retirementpolicy.Observation{TargetKey: key, GitProven: true, RepositoryCommonDir: proof.RepositoryCommonDir, Path: proof.Path, Branch: proof.Branch, HeadSHA: proof.HeadSHA}, retirementpolicy.Decision{TargetKey: key}, "retirement Git intent reconciled")
}

func (o *Orchestrator) reconcileRetirementWorkspace(ctx context.Context, snapshot *state.RunSnapshot, key string) error {
	reader := o.deps.WorkspaceReader
	var ok bool
	if reader == nil {
		reader, ok = o.deps.Herdr.(WorkspaceReader)
	} else {
		ok = true
	}
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "Workspace observation port is unavailable"})
	}
	target, ok := retirementTarget(snapshot.Retirement, key)
	if !ok {
		return o.retirementNeedsOperator(ctx, snapshot, retirementpolicy.Decision{TargetKey: key, Reason: "retirement target is missing"})
	}
	info, found, err := reader.GetWorkspace(ctx, target.WorkspaceID)
	if err != nil {
		return err
	}
	observation := retirementpolicy.Observation{TargetKey: key, WorkspaceObserved: true, WorkspaceFound: found}
	if found {
		observation.WorkspaceID, observation.PaneID, observation.Path, observation.WorkspaceState = info.WorkspaceID, info.RootPaneID, info.Path, string(info.State)
	}
	decision := retirementpolicy.Next(snapshot.Retirement, &observation)
	if decision.Kind == retirementpolicy.NeedsOperator {
		return o.retirementNeedsOperator(ctx, snapshot, decision)
	}
	if decision.Kind == retirementpolicy.MarkRetired || (decision.Kind == retirementpolicy.Complete && decision.NextStatus == "retired") {
		return o.applyRetirementObservation(ctx, snapshot, observation, decision, "retirement Workspace close reconciled as missing")
	}
	if strings.HasPrefix(snapshot.PendingAction, retirementCloseAction) && decision.Kind == retirementpolicy.CloseWorkspace {
		// Exact presence after a close response loss means the close result is
		// unknown. Clear the old intent and retry the close on a later Advance.
		if err := setRetirementTargetStatus(&snapshot.Retirement, key, "workspace_observed"); err != nil {
			return err
		}
		snapshot.PendingAction = ""
		snapshot.PendingTaskID = ""
		snapshot.Summary = "retirement Workspace remains present; close will retry"
		snapshot.Retirement.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "reconcile_not_executed", Phase: snapshot.Phase, Message: snapshot.Summary})
	}
	return o.applyRetirementObservation(ctx, snapshot, observation, decision, "retirement Workspace observation reconciled")
}

func (o *Orchestrator) applyRetirementObservation(ctx context.Context, snapshot *state.RunSnapshot, observation retirementpolicy.Observation, prior retirementpolicy.Decision, message string) error {
	decision := retirementpolicy.Next(snapshot.Retirement, &observation)
	if prior.TargetKey != "" && decision.TargetKey != prior.TargetKey {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(prior, "retirement observation target identity mismatch"))
	}
	if decision.Kind == retirementpolicy.NeedsOperator {
		return o.retirementNeedsOperator(ctx, snapshot, decision)
	}
	if decision.NextStatus == "" && decision.Kind != retirementpolicy.Complete && decision.Kind != retirementpolicy.MarkRetired {
		return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "retirement observation made no progress"))
	}
	if decision.TargetKey != "" && decision.NextStatus != "" {
		if err := setRetirementTargetStatus(&snapshot.Retirement, decision.TargetKey, decision.NextStatus); err != nil {
			return err
		}
		if observation.GitProven && decision.NextStatus == "git_proven" {
			target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey)
			if !ok || strings.TrimSpace(observation.RepositoryCommonDir) == "" {
				return o.retirementNeedsOperator(ctx, snapshot, decisionWithReason(decision, "git repository identity is required"))
			}
			target.RepositoryCommonDir = filepath.Clean(observation.RepositoryCommonDir)
		}
		if decision.NextStatus == "retired" {
			if target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey); ok {
				target.RetiredAt = o.now()
			}
		}
	}
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.Retirement.UpdatedAt = o.now()
	snapshot.Summary = message
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	if err := o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: message}); err != nil {
		return err
	}
	if decision.Kind == retirementpolicy.Complete || allRetirementTargetsRetired(snapshot.Retirement) {
		return o.finishRetirement(ctx, snapshot)
	}
	return nil
}

func (o *Orchestrator) prepareRetirementAction(ctx context.Context, snapshot *state.RunSnapshot, action, message, key string) error {
	if snapshot.PendingAction != "" && snapshot.PendingAction != action {
		return fmt.Errorf("another retirement action is pending: %s", snapshot.PendingAction)
	}
	snapshot.PendingAction = action
	snapshot.PendingTaskID = key
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Kind: "retirement", Phase: snapshot.Phase, Message: message, Data: map[string]any{"action": action, "targetKey": key}})
}

func (o *Orchestrator) retirementNeedsOperator(ctx context.Context, snapshot *state.RunSnapshot, decision retirementpolicy.Decision) error {
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = "retirement requires operator"
	}
	if decision.TargetKey != "" {
		if target, ok := retirementTarget(snapshot.Retirement, decision.TargetKey); ok {
			target.Status = "needs_operator"
			target.LastError = reason
		}
	}
	snapshot.Retirement.Status = "needs_operator"
	snapshot.Retirement.UpdatedAt = o.now()
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.Summary = reason
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "retirement_needs_operator", Phase: snapshot.Phase, Message: reason, Data: map[string]any{"targetKey": decision.TargetKey}})
}

func (o *Orchestrator) finishRetirement(ctx context.Context, snapshot *state.RunSnapshot) error {
	if snapshot.Retirement.TargetPhase == "" {
		snapshot.Retirement.TargetPhase = contract.PhaseCompleted
	}
	snapshot.Retirement.Status = "retired"
	snapshot.Retirement.UpdatedAt = o.now()
	// The audit is the commit record for retirement. Keep the durable snapshot
	// in PhaseRetiring until this append succeeds; a failed append must never
	// expose a terminal run without its retirement audit.
	snapshot.Phase = contract.PhaseRetiring
	snapshot.Summary = "Execution sessions retired"
	retiredKeys := make([]string, 0, len(snapshot.Retirement.Targets))
	for _, target := range snapshot.Retirement.Targets {
		retiredKeys = append(retiredKeys, target.Key)
	}
	if err := o.append(ctx, snapshot.RunID, state.Event{Type: "sessions_retired", Kind: "sessions_retired", Phase: contract.PhaseRetiring, Message: snapshot.Summary, Data: map[string]any{"targets": retiredKeys}}); err != nil {
		return err
	}
	snapshot.Phase = snapshot.Retirement.TargetPhase
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	if snapshot.Phase == contract.PhaseCompleted {
		return o.append(ctx, snapshot.RunID, state.Event{Type: "completed", Phase: contract.PhaseCompleted, Message: snapshot.Summary})
	}
	return nil
}

func retirementTarget(plan state.RetirementState, key string) (*state.RetirementTarget, bool) {
	for i := range plan.Targets {
		if plan.Targets[i].Key == key {
			return &plan.Targets[i], true
		}
	}
	return nil, false
}

func setRetirementTargetStatus(plan *state.RetirementState, key, status string) error {
	target, ok := retirementTarget(*plan, key)
	if !ok {
		return fmt.Errorf("retirement target %q is missing", key)
	}
	target.Status = status
	if status != "needs_operator" {
		target.LastError = ""
	}
	return nil
}

func allRetirementTargetsRetired(plan state.RetirementState) bool {
	if len(plan.Targets) == 0 {
		return true
	}
	for _, target := range plan.Targets {
		if target.Status != "retired" {
			return false
		}
	}
	return true
}

func retirementActionKey(action string) string {
	for _, prefix := range []string{retirementAgentAction, retirementGitAction, retirementWorkspaceAction, retirementCloseAction} {
		if strings.HasPrefix(action, prefix) {
			return strings.TrimPrefix(action, prefix)
		}
	}
	for _, prefix := range []string{"retirement_agent_", "retirement_git_", "retirement_workspace_", "retirement_close_"} {
		if strings.HasPrefix(action, prefix) {
			return strings.TrimPrefix(action, prefix)
		}
	}
	return ""
}

func retirementAgentIdentityMismatch(snapshot state.RunSnapshot, target *state.RetirementTarget, info herdr.AgentInfo) string {
	if target == nil || !canonicalRetirementAgentName(info.Name) || info.Name != target.AgentName || !canonicalRetirementProviderID(info.SessionID) || !canonicalRetirementProviderID(info.WorkspaceID) || info.WorkspaceID != target.WorkspaceID || !canonicalRetirementProviderID(info.PaneID) || info.PaneID != target.PaneID || !canonicalRetirementPath(info.Path) || info.Path != target.Path {
		return "Agent identity mismatch"
	}
	expected := retirementAgentEvidence(snapshot, *target)
	if expected.SessionID != "" && (!canonicalRetirementProviderID(expected.SessionID) || expected.SessionID != info.SessionID) {
		return "Agent session identity mismatch"
	}
	return ""
}

func canonicalRetirementProviderID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "/\\\r\n\t") && !strings.Contains(value, "..")
}

func canonicalRetirementAgentName(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 32 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func canonicalRetirementPath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func retirementAgentEvidence(snapshot state.RunSnapshot, target state.RetirementTarget) state.AgentEvidence {
	if target.Role == "reviewer" {
		return snapshot.Reviewer
	}
	if target.TaskID != "" {
		if task, ok := snapshot.Tasks[target.TaskID]; ok {
			return task.Agent
		}
	}
	return snapshot.Builder
}

func decisionWithReason(decision retirementpolicy.Decision, reason string) retirementpolicy.Decision {
	decision.Kind = retirementpolicy.NeedsOperator
	decision.Reason = reason
	decision.Reasons = []string{reason}
	return decision
}

func validRetirementProof(proof worktree.RetirementProof) bool {
	return strings.TrimSpace(proof.RepositoryCommonDir) == proof.RepositoryCommonDir &&
		filepath.IsAbs(proof.RepositoryCommonDir) && filepath.Clean(proof.RepositoryCommonDir) == proof.RepositoryCommonDir &&
		strings.TrimSpace(proof.Path) == proof.Path && filepath.IsAbs(proof.Path) && filepath.Clean(proof.Path) == proof.Path &&
		strings.TrimSpace(proof.Branch) == proof.Branch && strings.TrimSpace(proof.HeadSHA) == proof.HeadSHA
}
