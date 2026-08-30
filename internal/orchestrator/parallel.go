package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/dag"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/mergegate"
	"thread-dock/internal/recovery"
	"thread-dock/internal/review"
	"thread-dock/internal/scheduler"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

// taskAgentName returns the deterministic Herdr address for one role/task in
// a run. The readable prefix is retained where it fits; the digest makes
// distinct task IDs and long run IDs collision-resistant within Herdr's
// 32-character name limit.
func taskAgentName(role string, runID contract.RunID, taskID string) string {
	role = normalizeAgentPart(role)
	taskID = normalizeAgentPart(taskID)
	if role == "" {
		role = "agent"
	}
	if taskID == "" {
		taskID = "task"
	}
	legacy := role + "-" + string(runID) + "-" + taskID
	if validHerdrAgentName(legacy) {
		return legacy
	}
	digest := sha256.Sum256([]byte(string(runID) + "\x00" + taskID))
	suffix := hex.EncodeToString(digest[:])
	prefix := role + "-" + taskID + "-"
	if len(prefix) >= maxHerdrAgentNameLength {
		prefix = role + "-"
	}
	available := maxHerdrAgentNameLength - len(prefix)
	if available < 1 {
		return suffix[:maxHerdrAgentNameLength]
	}
	if available > len(suffix) {
		available = len(suffix)
	}
	return prefix + suffix[:available]
}

func normalizeAgentPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			b.WriteRune(char)
		}
	}
	return b.String()
}

// Auto chooses the parallel strategy for contracts with multiple tasks and
// retains the historical Single-run strategy for one-task contracts. The
// choice is persisted in each snapshot, so a fresh process can route an
// existing run deterministically.
type Auto struct {
	deps     Dependencies
	single   *Orchestrator
	parallel *Orchestrator
	mu       sync.Mutex
	modes    map[contract.RunID]*Orchestrator
}

func NewAuto(deps Dependencies) *Auto {
	return &Auto{deps: deps, single: New(deps), parallel: NewParallel(deps), modes: make(map[contract.RunID]*Orchestrator)}
}

func (a *Auto) Start(ctx context.Context, contractPath string) (contract.RunID, error) {
	if a == nil {
		return "", errors.New("orchestrator selector is nil")
	}
	c, err := readContract(contractPath)
	if err != nil {
		return "", err
	}
	selected := a.single
	if len(c.Tasks) > 1 {
		selected = a.parallel
	}
	id, err := selected.Start(ctx, contractPath)
	if err == nil || id != "" {
		a.mu.Lock()
		a.modes[id] = selected
		a.mu.Unlock()
	}
	return id, err
}

func (a *Auto) selected(ctx context.Context, id contract.RunID) (*Orchestrator, error) {
	a.mu.Lock()
	selected := a.modes[id]
	a.mu.Unlock()
	if selected != nil {
		return selected, nil
	}
	if a == nil || a.deps.Store == nil {
		return nil, errors.New("orchestrator selector dependencies are incomplete")
	}
	snapshot, err := a.deps.Store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if snapshot.Strategy == "parallel" {
		selected = a.parallel
	} else {
		selected = a.single
	}
	a.mu.Lock()
	a.modes[id] = selected
	a.mu.Unlock()
	return selected, nil
}

func (a *Auto) Advance(ctx context.Context, id contract.RunID) error {
	selected, err := a.selected(ctx, id)
	if err != nil {
		return err
	}
	return selected.Advance(ctx, id)
}

func (a *Auto) Stop(ctx context.Context, id contract.RunID) error {
	selected, err := a.selected(ctx, id)
	if err != nil {
		return err
	}
	return selected.Stop(ctx, id)
}

// ConfirmProtectedChange records the operator decision exactly once and then
// resumes the run. It is intentionally separate from the Merge Gate so the
// CLI can inject a narrow service without exposing a GitHub mutation.
func (o *Orchestrator) ConfirmProtectedChange(ctx context.Context, id contract.RunID) error {
	if o == nil || o.deps.Store == nil {
		return errors.New("orchestrator dependencies are incomplete")
	}
	release, err := o.claim(ctx, id)
	if err != nil {
		return err
	}
	snapshot, err := o.deps.Store.Load(ctx, id)
	if err != nil {
		release()
		return err
	}
	if len(snapshot.ProtectedReasons) == 0 || (snapshot.Phase != contract.PhaseNeedsOperator && snapshot.Phase != contract.PhaseMerging && snapshot.Phase != contract.PhaseCompleted) {
		release()
		return errors.New("protected-change confirmation is not valid for this run")
	}
	if snapshot.Phase == contract.PhaseCompleted && snapshot.ProtectedConfirmed {
		release()
		return nil
	}
	if snapshot.Phase == contract.PhaseNeedsOperator && !snapshot.ProtectedConfirmed {
		if err := o.append(ctx, id, state.Event{Type: "intent", Kind: "protected-change-confirmation", Phase: snapshot.Phase, Message: "operator confirmed protected change"}); err != nil {
			release()
			return err
		}
		snapshot.ProtectedConfirmed = true
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, snapshot); err != nil {
			release()
			return err
		}
		if err := o.append(ctx, id, state.Event{Type: "audit", Phase: snapshot.Phase, Message: "protected change confirmation recorded", Data: map[string]any{"reasons": append([]string(nil), snapshot.ProtectedReasons...)}}); err != nil {
			release()
			return err
		}
	}
	release()
	return o.Advance(ctx, id)
}

func (a *Auto) ConfirmProtectedChange(ctx context.Context, id contract.RunID) error {
	selected, err := a.selected(ctx, id)
	if err != nil {
		return err
	}
	return selected.ConfirmProtectedChange(ctx, id)
}

var _ interface {
	Start(context.Context, string) (contract.RunID, error)
	Advance(context.Context, contract.RunID) error
	Stop(context.Context, contract.RunID) error
} = (*Auto)(nil)

// ProjectStatusReader is an optional read port used to reconcile a status
// mutation that may have succeeded immediately before a process crash.
type ProjectStatusReader interface {
	GetProjectStatus(context.Context, github.ProjectRef, string) (string, error)
}

// FingerprintReader returns Git's managed fingerprint without exposing raw
// process output. It is optional so legacy adapters remain usable by the
// Single-run strategy.
type FingerprintReader interface {
	Fingerprint(context.Context, string) (string, error)
}

// RecoveryFingerprint is the durable continuation key for a parallel run.
// It deliberately includes only immutable Git/evidence identifiers and
// sorted task IDs; no terminal text or provider payload is retained.
func RecoveryFingerprint(commitSHA, managedGitFingerprint string, completedIDs []string, verification []state.VerificationEvidence) string {
	ids := append([]string(nil), completedIDs...)
	sort.Strings(ids)
	checks := append([]state.VerificationEvidence(nil), verification...)
	sort.Slice(checks, func(i, j int) bool { return checks[i].Command < checks[j].Command })
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n", strings.TrimSpace(commitSHA), strings.TrimSpace(managedGitFingerprint))
	for _, id := range ids {
		b.WriteString(id)
		b.WriteByte('\n')
	}
	for _, check := range checks {
		fmt.Fprintf(&b, "%s\x00%s\x00%s\n", check.Command, check.Outcome, check.Duration)
	}
	digest := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(digest[:])
}

// advanceParallel is the multi-task state machine. It deliberately uses the
// existing Orchestrator persistence, locking, intent, and validation helpers;
// only the strategy-specific phase transitions live here.
func (o *Orchestrator) advanceParallel(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	if snapshot.PendingAction != "" {
		return o.reconcileParallelPending(ctx, snapshot, runtime)
	}
	if err := o.reconcileParallelProject(ctx, snapshot, runtime); err != nil {
		return err
	}
	switch snapshot.Phase {
	case contract.PhaseRegistered:
		return o.parallelTransition(ctx, snapshot, contract.PhaseAnalyzing, "병렬 Task 분석 시작")
	case contract.PhaseAnalyzing:
		if snapshot.ActionCursor == 0 {
			return o.createIntegrationWorktree(ctx, snapshot, runtime)
		}
		return o.parallelTransition(ctx, snapshot, contract.PhaseBuilding, "Builder dispatch 시작")
	case contract.PhaseBuilding:
		return o.advanceParallelBuilding(ctx, snapshot, runtime)
	case contract.PhaseIntegrating:
		return o.advanceParallelIntegrating(ctx, snapshot, runtime)
	case contract.PhaseReviewing:
		return o.advanceParallelReview(ctx, snapshot, runtime)
	case contract.PhaseCI:
		return o.advanceParallelCI(ctx, snapshot, runtime)
	case contract.PhaseNeedsOperator:
		if !snapshot.ProtectedConfirmed {
			snapshot.Summary = "Protected Change 확인을 기다리는 중"
			snapshot.UpdatedAt = o.now()
			return o.deps.Store.Save(ctx, *snapshot)
		}
		return o.parallelTransition(ctx, snapshot, contract.PhaseMerging, "Protected Change 확인 완료; 병합 준비")
	case contract.PhaseMerging:
		return o.advanceParallelMerge(ctx, snapshot, runtime)
	default:
		return fmt.Errorf("unsupported parallel run phase %q", snapshot.Phase)
	}
}

func (o *Orchestrator) parallelTransition(ctx context.Context, snapshot *state.RunSnapshot, phase contract.RunPhase, message string) error {
	snapshot.Phase = phase
	snapshot.ActionCursor = 0
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "phase_transition", Phase: phase, Message: message})
}

func (o *Orchestrator) parallelPrepare(ctx context.Context, snapshot *state.RunSnapshot, action, message string, data map[string]any) error {
	if snapshot.PendingAction != "" && snapshot.PendingAction != action {
		return fmt.Errorf("another action is pending: %s", snapshot.PendingAction)
	}
	snapshot.PendingAction = action
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	if data == nil {
		data = map[string]any{}
	}
	data["action"] = action
	return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: message, Data: data})
}

func (o *Orchestrator) parallelFinish(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.ActionCursor++
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: message})
}

func (o *Orchestrator) parallelFinishPreserveCursor(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	cursor := snapshot.ActionCursor
	if err := o.parallelFinish(ctx, snapshot, message); err != nil {
		return err
	}
	snapshot.ActionCursor = cursor
	snapshot.UpdatedAt = o.now()
	return o.deps.Store.Save(ctx, *snapshot)
}

func (o *Orchestrator) parallelBlock(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.Phase = contract.PhaseBlocked
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "blocked", Phase: contract.PhaseBlocked, Message: message})
}

func (o *Orchestrator) reconcileParallelProject(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	want := parallelProjectStatus(snapshot.Phase)
	if want == "" || snapshot.ProjectStatus == want {
		return nil
	}
	if !snapshot.ProjectAutomationEnabled {
		snapshot.ProjectStatus = want
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "project_automation_skipped", Phase: snapshot.Phase, Message: "Project automation disabled; placeholder IDs are not evidence", Data: map[string]any{"status": want}})
	}
	action := "parallel_project_status_" + strings.ToLower(strings.ReplaceAll(want, " ", "_"))
	if err := o.parallelPrepare(ctx, snapshot, action, "Project 상태 업데이트", map[string]any{"status": want}); err != nil {
		return err
	}
	if o.deps.GitHub == nil || snapshot.Registration.NodeID == "" {
		return errors.New("Project status requires registered Issue identity")
	}
	if err := o.deps.GitHub.SetProjectStatus(ctx, o.deps.Project, snapshot.Registration.NodeID, want); err != nil {
		return err
	}
	snapshot.ProjectStatus = want
	return o.parallelFinishPreserveCursor(ctx, snapshot, "Project 상태 업데이트 완료")
}

func parallelProjectStatus(phase contract.RunPhase) string {
	switch phase {
	case contract.PhaseRegistered:
		return "Ready"
	case contract.PhaseAnalyzing, contract.PhaseBuilding, contract.PhaseIntegrating:
		return "In Progress"
	case contract.PhaseReviewing, contract.PhaseCI, contract.PhaseNeedsOperator, contract.PhaseMerging:
		return "Review"
	case contract.PhaseCompleted:
		return "Done"
	default:
		return ""
	}
}

func parallelTask(c contract.TaskContract, id string) (contract.Task, bool) {
	for _, task := range c.Tasks {
		if task.ID == id {
			return task, true
		}
	}
	return contract.Task{}, false
}

func parallelGraph(c contract.TaskContract) dag.Graph {
	nodes := make([]dag.Node, 0, len(c.Tasks))
	for _, task := range c.Tasks {
		nodes = append(nodes, dag.Node{ID: task.ID, DependsOn: append([]string(nil), task.DependsOn...)})
	}
	graph, _ := dag.Build(nodes)
	return graph
}

func (o *Orchestrator) advanceParallelBuilding(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	if len(snapshot.Tasks) == 0 {
		return o.parallelBlock(ctx, snapshot, "병렬 Task 상태가 없습니다")
	}
	// First continue the earliest already-dispatched task. This gives each
	// Advance one deterministic action while preserving contract order.
	active := 0
	allInspected := true
	hasOutstanding := false
	for _, id := range snapshot.TaskOrder {
		taskState := snapshot.Tasks[id]
		if taskState.State == "completed" {
			continue
		}
		hasOutstanding = true
		if taskState.State == "running" {
			active++
		} else {
			allInspected = false
		}
		if taskState.Stage != "inspected" {
			allInspected = false
		}
	}
	if hasOutstanding && allInspected {
		return o.parallelTransition(ctx, snapshot, contract.PhaseIntegrating, "모든 Builder 결과가 immutable integration을 기다리는 중")
	}
	for _, id := range snapshot.TaskOrder {
		taskState := snapshot.Tasks[id]
		if taskState.State == "running" {
			if taskState.Stage == "inspected" {
				continue
			}
			return o.advanceParallelTask(ctx, snapshot, runtime, id, taskState)
		}
	}
	if active < 2 {
		selected := scheduler.Next(parallelGraph(runtime.contract), parallelSchedulerStates(snapshot), 2)
		for _, id := range selected {
			if snapshot.Tasks[id].State == "pending" {
				return o.createParallelTaskWorktree(ctx, snapshot, runtime, id)
			}
		}
	}
	return o.parallelBlock(ctx, snapshot, "병렬 Scheduler가 진행 가능한 Task를 찾지 못했습니다")
}

func parallelSchedulerStates(snapshot *state.RunSnapshot) map[string]scheduler.TaskState {
	states := make(map[string]scheduler.TaskState, len(snapshot.Tasks))
	for id, taskState := range snapshot.Tasks {
		switch taskState.State {
		case "pending":
			states[id] = scheduler.Pending
		case "running":
			states[id] = scheduler.Running
		case "completed":
			states[id] = scheduler.Completed
		default:
			states[id] = scheduler.Blocked
		}
	}
	return states
}

func (o *Orchestrator) createParallelTaskWorktree(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string) error {
	task, ok := parallelTask(runtime.contract, id)
	if !ok {
		return o.parallelBlock(ctx, snapshot, "알 수 없는 Task를 dispatch할 수 없습니다")
	}
	action := "parallel_create_worktree_" + normalizeAgentPart(id)
	if err := o.parallelPrepare(ctx, snapshot, action, "Builder Worktree 생성", map[string]any{"taskId": id, "branch": task.Branch}); err != nil {
		return err
	}
	created, err := o.deps.Herdr.CreateWorktree(ctx, herdr.CreateWorktreeRequest{Cwd: snapshot.RepositoryPath, Branch: task.Branch, Base: runtime.contract.BaseCommit, Label: "threaddock-" + string(snapshot.RunID) + "-" + id})
	if err != nil {
		return err
	}
	if strings.TrimSpace(created.Path) == "" || strings.TrimSpace(created.WorkspaceID) == "" || strings.TrimSpace(created.PaneID) == "" {
		return errors.New("Herdr returned incomplete parallel Builder Worktree identity")
	}
	snapshot.Tasks[id] = state.TaskRunState{State: "running", Stage: "start", Worktree: state.WorktreeState{Path: created.Path, WorkspaceID: created.WorkspaceID, PaneID: created.PaneID, Branch: task.Branch}}
	snapshot.CurrentTask = id
	return o.parallelFinish(ctx, snapshot, "Builder Worktree 준비 완료")
}

func (o *Orchestrator) advanceParallelTask(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string, taskState state.TaskRunState) error {
	task, ok := parallelTask(runtime.contract, id)
	if !ok {
		return o.parallelBlock(ctx, snapshot, "알 수 없는 Task 상태입니다")
	}
	name := taskAgentName(task.Role, snapshot.RunID, id)
	if taskState.Agent.Name == "" {
		taskState.Agent.Name = name
	}
	snapshot.CurrentTask = id
	switch taskState.Stage {
	case "start":
		return o.parallelStartTask(ctx, snapshot, id, taskState)
	case "baseline":
		return o.parallelBaselineTask(ctx, snapshot, id, taskState)
	case "prompt", "repair_prompt":
		return o.parallelPromptTask(ctx, snapshot, runtime, id, task, taskState)
	case "evidence", "repair_evidence":
		return o.parallelCollectTaskEvidence(ctx, snapshot, runtime, id, task, taskState)
	case "inspect", "repair_inspect":
		return o.parallelInspectTask(ctx, snapshot, runtime, id, task, taskState)
	default:
		return o.parallelBlock(ctx, snapshot, "알 수 없는 Builder 단계입니다")
	}
}

func (o *Orchestrator) parallelStartTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	action := "parallel_start_agent_" + normalizeAgentPart(id)
	if err := o.parallelPrepare(ctx, snapshot, action, "Builder Agent 시작", map[string]any{"taskId": id, "agent": taskState.Agent.Name}); err != nil {
		return err
	}
	if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: taskState.Agent.Name, PaneID: taskState.Worktree.PaneID}); err != nil {
		return err
	}
	taskState.Stage = "baseline"
	taskState.Agent.IdentitySource = "provider"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder Agent 준비 완료")
}

func (o *Orchestrator) parallelBaselineTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return errors.New("parallel Builder identity lookup is required")
	}
	action := "parallel_baseline_" + normalizeAgentPart(id)
	if err := o.parallelPrepare(ctx, snapshot, action, "Builder prompt baseline 조회", map[string]any{"taskId": id}); err != nil {
		return err
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	if errors.Is(err, herdr.ErrAgentNotFound) || info.State == herdr.AgentStateBlocked || info.State == herdr.AgentStateUnknown {
		return o.parallelOperatorBlock(ctx, snapshot, id, "live Agent identity is stale, blocked, unknown, or absent")
	}
	if err != nil || !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return o.parallelRecoveryFailure(ctx, snapshot, id, taskState, "Builder identity reconciliation failed")
	}
	taskState.Agent.SessionID = strings.TrimSpace(info.SessionID)
	if isTerminalIdentity(info.SessionID) {
		taskState.Agent.IdentitySource = "terminal"
		taskState.NativeResume = false
	} else {
		taskState.Agent.IdentitySource = "provider"
		taskState.NativeResume = true
	}
	taskState.Prompt = state.PromptReceipt{RequestID: parallelPromptRequestID(snapshot.RunID, id, taskState.RepairCount), BaselineSeq: info.StateChangeSeq}
	taskState.Stage = "prompt"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder prompt baseline 저장")
}

func (o *Orchestrator) parallelOperatorBlock(ctx context.Context, snapshot *state.RunSnapshot, id, message string) error {
	snapshot.PendingAction = ""
	snapshot.Phase = contract.PhaseNeedsOperator
	snapshot.CurrentTask = id
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "needs_operator", Phase: snapshot.Phase, Message: message, Data: map[string]any{"taskId": id}})
}

func parallelPromptRequestID(id contract.RunID, taskID string, repair int) string {
	if repair <= 0 {
		return string(id) + ":" + taskID + ":prompt"
	}
	return fmt.Sprintf("%s:%s:repair-%d", id, taskID, repair)
}

func (o *Orchestrator) parallelPromptTask(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string, task contract.Task, taskState state.TaskRunState) error {
	if taskState.Prompt.RequestID == "" {
		return errors.New("parallel Builder prompt receipt is missing")
	}
	action := "parallel_prompt_" + normalizeAgentPart(id)
	if err := o.parallelPrepare(ctx, snapshot, action, "Builder packet 전송", map[string]any{"taskId": id, "agent": taskState.Agent.Name, "requestId": taskState.Prompt.RequestID}); err != nil {
		return err
	}
	packet := builderPacket(runtime.contract, task, taskState.Prompt.RequestID)
	if taskState.Stage == "repair_prompt" {
		findings := make([]review.Finding, 0, len(snapshot.ReviewFindings))
		for _, finding := range snapshot.ReviewFindings {
			findings = append(findings, review.Finding{ID: finding.ID, Summary: finding.Summary, Paths: append([]string(nil), finding.Paths...)})
		}
		var err error
		packet, err = review.BuildRepairPacket(review.RepairPacketInput{AcceptanceCriteria: append(append([]string(nil), runtime.contract.Parent.AcceptanceCriteria...), task.AcceptanceCriteria...), IntegrationSHA: snapshot.IntegrationSHA, BlockingFindings: findings, AllowedPaths: task.AllowedPaths, RemainingBudget: 2 - snapshot.RepairCount})
		if err != nil {
			return o.parallelBlock(ctx, snapshot, "Repair packet를 생성할 수 없습니다")
		}
		packet += "\nUse requestId=" + taskState.Prompt.RequestID + "."
	}
	if err := o.deps.Herdr.Prompt(ctx, taskState.Agent.Name, packet); err != nil {
		return err
	}
	taskState.Stage = "evidence"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder packet 전송 완료")
}

func (o *Orchestrator) parallelCollectTaskEvidence(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string, task contract.Task, taskState state.TaskRunState) error {
	reader, ok := o.deps.Herdr.(EvidenceReader)
	if !ok {
		return o.parallelRecoveryFailure(ctx, snapshot, id, taskState, "Builder evidence reader is unavailable")
	}
	action := "parallel_collect_evidence_" + normalizeAgentPart(id)
	if err := o.parallelPrepare(ctx, snapshot, action, "Builder structured evidence 수집", map[string]any{"taskId": id}); err != nil {
		return err
	}
	evidence, err := reader.ReadEvidence(ctx, taskState.Agent.Name)
	if err != nil || evidence.RequestID != taskState.Prompt.RequestID || !validCommitSHA(strings.ToLower(evidence.CommitSHA)) || !parallelVerificationValid(evidence.Verification, task.Verification) {
		return o.parallelRecoveryFailure(ctx, snapshot, id, taskState, "Builder structured evidence is stale or incomplete")
	}
	taskState.Agent.RequestID = evidence.RequestID
	taskState.Agent.CommitSHA = strings.ToLower(evidence.CommitSHA)
	taskState.Agent.VerificationEvidence = make([]state.VerificationEvidence, 0, len(evidence.Verification))
	for _, check := range evidence.Verification {
		taskState.Agent.VerificationEvidence = append(taskState.Agent.VerificationEvidence, state.VerificationEvidence{Command: strings.TrimSpace(check.Command), Outcome: "passed", Duration: strings.TrimSpace(check.Duration)})
	}
	taskState.RecoveryCount = 0
	managedFingerprint := ""
	if reader, ok := o.deps.Worktree.(FingerprintReader); ok {
		if fingerprint, fingerprintErr := reader.Fingerprint(ctx, snapshot.Integration.Path); fingerprintErr == nil {
			managedFingerprint = strings.TrimSpace(fingerprint)
		}
	}
	taskState.ProgressFingerprint = RecoveryFingerprint(taskState.Agent.CommitSHA, managedFingerprint, completedTaskIDs(snapshot), taskState.Agent.VerificationEvidence)
	taskState.Stage = "inspect"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder structured evidence 확인")
}

func parallelVerificationValid(got []herdr.VerificationCheck, required []string) bool {
	if len(got) != len(required) || len(required) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(got))
	for _, check := range got {
		command := strings.TrimSpace(check.Command)
		if command == "" || check.Command != command || strings.ToLower(strings.TrimSpace(check.Outcome)) != "passed" || strings.TrimSpace(check.Duration) == "" {
			return false
		}
		if _, err := time.ParseDuration(strings.TrimSpace(check.Duration)); err != nil {
			return false
		}
		seen[command] = struct{}{}
	}
	for _, command := range required {
		if _, ok := seen[strings.TrimSpace(command)]; !ok {
			return false
		}
	}
	return true
}

func completedTaskIDs(snapshot *state.RunSnapshot) []string {
	ids := make([]string, 0, len(snapshot.Tasks))
	for id, task := range snapshot.Tasks {
		if task.State == "completed" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (o *Orchestrator) parallelInspectTask(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string, task contract.Task, taskState state.TaskRunState) error {
	inspector, ok := o.deps.Worktree.(WorktreeInspector)
	if !ok {
		return o.parallelBlock(ctx, snapshot, "Git inspection port is unavailable")
	}
	action := "parallel_inspect_commit_" + normalizeAgentPart(id)
	if err := o.parallelPrepare(ctx, snapshot, action, "Git에서 immutable Builder commit 확인", map[string]any{"taskId": id, "commitSha": taskState.Agent.CommitSHA}); err != nil {
		return err
	}
	inspection, err := inspector.InspectCommit(ctx, taskState.Worktree.Path, runtime.contract.BaseCommit, task.Branch, taskState.Agent.CommitSHA)
	if err != nil {
		return err
	}
	if err := validateInspection(inspection, taskState.Agent.CommitSHA, task.Branch, task.AllowedPaths); err != nil {
		return err
	}
	if containsCredential(inspection.Patch) {
		return o.parallelBlock(ctx, snapshot, "민감 credential patch 차단")
	}
	taskState.Agent.Branch = inspection.Branch
	taskState.Agent.ChangedFiles = append([]string(nil), inspection.ChangedFiles...)
	taskState.Agent.Patch = inspection.Patch
	taskState.Stage = "inspected"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Git immutable Builder patch 검증 완료")
}

func (o *Orchestrator) parallelRecoveryFailure(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState, message string) error {
	taskState.RecoveryCount++
	snapshot.Tasks[id] = taskState
	decision := recovery.Decide(o.now(), recovery.Policy{WorkingWait: time.Nanosecond, Limit: 3}, recovery.AgentSnapshot{State: "working", Alive: false, RecoveryCount: taskState.RecoveryCount, CanNativeResume: taskState.NativeResume})
	if taskState.RecoveryCount >= 3 || decision.Kind == recovery.Block {
		return o.parallelBlock(ctx, snapshot, "세 번의 동일한 recovery 이후 진행을 중단했습니다")
	}
	snapshot.PendingAction = ""
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "recovery_requested", Phase: snapshot.Phase, Message: message, Data: map[string]any{"taskId": id, "count": taskState.RecoveryCount}})
}

func (o *Orchestrator) advanceParallelIntegrating(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	for _, id := range snapshot.TaskOrder {
		taskState := snapshot.Tasks[id]
		if taskState.State != "running" || taskState.Stage != "inspected" {
			continue
		}
		merger, ok := o.deps.Worktree.(interface {
			MergeCommitNoFF(context.Context, string, string) error
		})
		if !ok {
			// A legacy fake may expose only the original immutable merge port;
			// production worktree.Git implements MergeCommitNoFF.
			legacy, legacyOK := o.deps.Worktree.(ImmutableMerger)
			if !legacyOK {
				return o.parallelBlock(ctx, snapshot, "immutable integration port is unavailable")
			}
			action := "parallel_merge_task_" + normalizeAgentPart(id)
			if err := o.parallelPrepare(ctx, snapshot, action, "immutable Task commit 병합", map[string]any{"taskId": id, "commitSha": taskState.Agent.CommitSHA}); err != nil {
				return err
			}
			if err := legacy.MergeCommit(ctx, snapshot.Integration.Path, taskState.Agent.CommitSHA); err != nil {
				return o.parallelBlock(ctx, snapshot, "Task immutable merge 충돌")
			}
			taskState.State = "completed"
			taskState.Stage = "merged"
			snapshot.Tasks[id] = taskState
			return o.parallelFinish(ctx, snapshot, "Task immutable merge 완료")
		}
		action := "parallel_merge_task_" + normalizeAgentPart(id)
		if err := o.parallelPrepare(ctx, snapshot, action, "immutable Task commit 병합", map[string]any{"taskId": id, "commitSha": taskState.Agent.CommitSHA}); err != nil {
			return err
		}
		if err := merger.MergeCommitNoFF(ctx, snapshot.Integration.Path, taskState.Agent.CommitSHA); err != nil {
			if errors.Is(err, worktree.ErrConflict) {
				return o.parallelBlock(ctx, snapshot, "Git merge conflict blocks integration")
			}
			return err
		}
		taskState.State = "completed"
		taskState.Stage = "merged"
		snapshot.Tasks[id] = taskState
		return o.parallelFinish(ctx, snapshot, "Task immutable merge 완료")
	}
	for _, taskState := range snapshot.Tasks {
		if taskState.State != "completed" {
			return o.parallelBlock(ctx, snapshot, "모든 Task immutable merge가 완료되지 않았습니다")
		}
	}
	if snapshot.IntegrationVerification == nil {
		checker, ok := o.deps.Worktree.(interface {
			RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error)
		})
		if !ok {
			return o.parallelBlock(ctx, snapshot, "Wave End Verification port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_wave_end_verification", "Wave End Verification 실행", nil); err != nil {
			return err
		}
		checks, err := checker.RunChecks(ctx, snapshot.Integration.Path, runtime.contract.Verification)
		if err != nil {
			return o.parallelBlock(ctx, snapshot, "Wave End Verification failed")
		}
		if !parallelGitChecksValid(checks, runtime.contract.Verification) {
			return o.parallelBlock(ctx, snapshot, "Wave End Verification evidence is incomplete")
		}
		snapshot.IntegrationVerification = parallelVerificationEvidence(checks)
		return o.parallelFinish(ctx, snapshot, "Wave End Verification 완료")
	}
	if snapshot.IntegrationSHA == "" {
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "integration commit locator is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_locate_integration_sha", "integration final SHA 확인", nil); err != nil {
			return err
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(sha) {
			return o.parallelBlock(ctx, snapshot, "integration final SHA is unavailable")
		}
		snapshot.IntegrationSHA = strings.ToLower(strings.TrimSpace(sha))
		return o.parallelFinish(ctx, snapshot, "integration final SHA 저장")
	}
	return o.parallelTransition(ctx, snapshot, contract.PhaseReviewing, "독립 Reviewer 검토 시작")
}

func parallelGitChecksValid(checks []worktree.VerificationCheck, required []string) bool {
	if len(checks) != len(required) || len(required) == 0 {
		return false
	}
	seen := make(map[string]bool, len(checks))
	for _, check := range checks {
		if check.Command == "" || check.Outcome != "passed" || check.Duration == "" || check.ExitCode != 0 {
			return false
		}
		duration, err := time.ParseDuration(check.Duration)
		if err != nil || duration <= 0 {
			return false
		}
		if seen[check.Command] {
			return false
		}
		seen[check.Command] = true
	}
	for _, command := range required {
		if !seen[command] {
			return false
		}
	}
	return true
}

func parallelVerificationEvidence(checks []worktree.VerificationCheck) []state.VerificationEvidence {
	result := make([]state.VerificationEvidence, 0, len(checks))
	for _, check := range checks {
		result = append(result, state.VerificationEvidence{Command: strings.TrimSpace(check.Command), Outcome: strings.TrimSpace(check.Outcome), Duration: strings.TrimSpace(check.Duration)})
	}
	return result
}

func (o *Orchestrator) advanceParallelReview(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	name := agentName("reviewer", snapshot.RunID)
	switch snapshot.ActionCursor {
	case 0:
		opener, ok := o.deps.Herdr.(WorktreeOpener)
		if !ok {
			return errors.New("parallel Reviewer Worktree opener is required")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_open_reviewer_worktree", "Reviewer Worktree 연결", map[string]any{"path": snapshot.IntegrationPath}); err != nil {
			return err
		}
		opened, err := opener.OpenWorktree(ctx, herdr.OpenWorktreeRequest{Cwd: snapshot.RepositoryPath, Path: snapshot.IntegrationPath, Label: "threaddock-review-" + string(snapshot.RunID)})
		if err != nil || strings.TrimSpace(opened.Path) == "" || strings.TrimSpace(opened.WorkspaceID) == "" || strings.TrimSpace(opened.PaneID) == "" || filepath.Clean(opened.Path) != filepath.Clean(snapshot.IntegrationPath) {
			return errors.New("Herdr returned incomplete parallel Reviewer Worktree identity")
		}
		snapshot.ReviewerWorktree = state.WorktreeState{Path: opened.Path, WorkspaceID: opened.WorkspaceID, PaneID: opened.PaneID, Branch: snapshot.Integration.Branch}
		return o.parallelFinish(ctx, snapshot, "Reviewer Worktree 준비 완료")
	case 1:
		if err := o.parallelPrepare(ctx, snapshot, "parallel_start_reviewer", "Reviewer Agent 시작", map[string]any{"agent": name}); err != nil {
			return err
		}
		if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: name, PaneID: snapshot.ReviewerWorktree.PaneID}); err != nil {
			return err
		}
		snapshot.Reviewer = state.AgentEvidence{Name: name, IdentitySource: "provider"}
		return o.parallelFinish(ctx, snapshot, "Reviewer Agent 준비 완료")
	case 2:
		locator, ok := o.deps.Herdr.(AgentLocator)
		if !ok {
			return errors.New("parallel Reviewer identity lookup is required")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_baseline_reviewer", "Reviewer prompt baseline 조회", nil); err != nil {
			return err
		}
		info, err := locator.GetInfo(ctx, name)
		if err != nil || !matchesParallelAgentIdentity(info, name, snapshot.ReviewerWorktree) {
			return errors.New("Reviewer identity reconciliation failed")
		}
		snapshot.Reviewer.SessionID = info.SessionID
		snapshot.Reviewer.IdentitySource = "provider"
		if isTerminalIdentity(info.SessionID) {
			snapshot.Reviewer.IdentitySource = "terminal"
		}
		snapshot.ReviewerPrompt = state.PromptReceipt{RequestID: string(snapshot.RunID) + ":reviewer-prompt", BaselineSeq: info.StateChangeSeq}
		return o.parallelFinish(ctx, snapshot, "Reviewer prompt baseline 저장")
	case 3:
		if err := o.parallelPrepare(ctx, snapshot, "parallel_prompt_reviewer", "Reviewer acceptance packet 전송", nil); err != nil {
			return err
		}
		reviewEvidence := parallelReviewerEvidence(snapshot)
		if err := o.deps.Herdr.Prompt(ctx, name, reviewerPacket(runtime.contract, firstBuilder(runtime.contract), reviewEvidence, snapshot.ReviewerPrompt.RequestID)); err != nil {
			return err
		}
		snapshot.Reviewer.Verification = []string{"review schema sent"}
		return o.parallelFinish(ctx, snapshot, "Reviewer packet 전송 완료")
	case 4:
		reader, ok := o.deps.Herdr.(ReviewEvidenceReader)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "strict Reviewer evidence reader is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_collect_review", "Reviewer structured evidence 수집", nil); err != nil {
			return err
		}
		evidence, err := reader.ReadReviewEvidence(ctx, name, snapshot.ReviewerPrompt.RequestID)
		if err != nil {
			return o.parallelBlock(ctx, snapshot, "Reviewer evidence is missing or stale")
		}
		findings := make([]state.ReviewFinding, 0, len(evidence.BlockingFindings))
		for _, finding := range evidence.BlockingFindings {
			findings = append(findings, state.ReviewFinding{ID: finding.ID, Summary: finding.Summary, Paths: append([]string(nil), finding.Paths...)})
		}
		reviewResult := review.ReviewResult{Source: "reviewer", Blocking: evidence.Decision == "block", RiskCategories: append([]string(nil), evidence.RiskCategories...)}
		for _, finding := range findings {
			reviewResult.Findings = append(reviewResult.Findings, review.Finding{ID: finding.ID, Summary: finding.Summary, Paths: append([]string(nil), finding.Paths...)})
		}
		decision := review.Decide(*snapshot, reviewResult)
		snapshot.ReviewFindings = findings
		snapshot.ReviewRiskCategories = append([]string(nil), evidence.RiskCategories...)
		switch decision.Kind {
		case review.Accept:
			snapshot.ReviewDecision = "accept"
			return o.parallelFinish(ctx, snapshot, "Reviewer acceptance 확인")
		case review.Repair:
			snapshot.RepairCount = decision.NextCount
			taskID := firstParallelTaskID(snapshot)
			if taskID == "" {
				return o.parallelBlock(ctx, snapshot, "repair 대상 Task가 없습니다")
			}
			taskState := snapshot.Tasks[taskID]
			taskState.State = "running"
			taskState.Stage = "repair_prompt"
			taskState.RepairCount++
			taskState.Agent.RequestID = ""
			taskState.Prompt = state.PromptReceipt{RequestID: parallelPromptRequestID(snapshot.RunID, taskID, taskState.RepairCount)}
			snapshot.Tasks[taskID] = taskState
			snapshot.CurrentTask = taskID
			snapshot.PendingAction = ""
			return o.parallelTransition(ctx, snapshot, contract.PhaseBuilding, "Reviewer findings에 대한 bounded repair 시작")
		default:
			return o.parallelBlock(ctx, snapshot, "Reviewer repair budget exhausted")
		}
	default:
		if snapshot.ActionCursor == 5 {
			return o.parallelPushIntegration(ctx, snapshot, runtime)
		}
		if snapshot.ActionCursor == 6 {
			return o.parallelCreateDraftPR(ctx, snapshot, runtime)
		}
		return o.parallelBlock(ctx, snapshot, "알 수 없는 Reviewer 단계입니다")
	}
}

func parallelReviewerEvidence(snapshot *state.RunSnapshot) state.AgentEvidence {
	evidence := state.AgentEvidence{CommitSHA: snapshot.IntegrationSHA, Branch: snapshot.Integration.Branch, VerificationEvidence: append([]state.VerificationEvidence(nil), snapshot.IntegrationVerification...)}
	var patches []string
	for _, id := range snapshot.TaskOrder {
		task := snapshot.Tasks[id]
		evidence.ChangedFiles = append(evidence.ChangedFiles, task.Agent.ChangedFiles...)
		if task.Agent.Patch != "" {
			patches = append(patches, task.Agent.Patch)
		}
	}
	evidence.Patch = strings.Join(patches, "\n")
	return evidence
}

func firstParallelTaskID(snapshot *state.RunSnapshot) string {
	for _, id := range snapshot.TaskOrder {
		if snapshot.Tasks[id].State == "completed" || snapshot.Tasks[id].State == "running" {
			return id
		}
	}
	return ""
}

func matchesParallelAgentIdentity(info herdr.AgentInfo, expectedName string, expectedWorktree state.WorktreeState) bool {
	return strings.TrimSpace(info.Name) == strings.TrimSpace(expectedName) &&
		strings.TrimSpace(info.SessionID) != "" &&
		strings.TrimSpace(info.WorkspaceID) == strings.TrimSpace(expectedWorktree.WorkspaceID) &&
		strings.TrimSpace(info.PaneID) == strings.TrimSpace(expectedWorktree.PaneID) &&
		canonicalPathEqual(info.Path, expectedWorktree.Path)
}

func canonicalPathEqual(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}

func isTerminalIdentity(identity string) bool {
	lower := strings.ToLower(strings.TrimSpace(identity))
	return strings.Contains(lower, "terminal")
}

func (o *Orchestrator) parallelPushIntegration(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	git, ok := o.deps.Worktree.(interface {
		PushBranch(context.Context, string, string, string) error
	})
	if !ok {
		return o.parallelBlock(ctx, snapshot, "integration push port is unavailable")
	}
	if err := o.parallelPrepare(ctx, snapshot, "parallel_push_integration", "integration branch push", map[string]any{"branch": snapshot.Integration.Branch, "sha": snapshot.IntegrationSHA}); err != nil {
		return err
	}
	if err := git.PushBranch(ctx, snapshot.Integration.Path, "origin", snapshot.Integration.Branch); err != nil {
		return err
	}
	return o.parallelFinish(ctx, snapshot, "integration branch push 완료")
}

func (o *Orchestrator) parallelCreateDraftPR(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	if o.deps.GitHub == nil {
		return errors.New("GitHub draft PR port is unavailable")
	}
	repo := github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}
	action := "parallel_create_draft_pr"
	if err := o.parallelPrepare(ctx, snapshot, action, "Draft PR 생성", map[string]any{"head": snapshot.Integration.Branch, "base": runtime.contract.Repository.DefaultBranch}); err != nil {
		return err
	}
	request := github.DraftPRRequest{Title: runtime.contract.Parent.Title, Body: runtime.contract.Parent.Body, Head: snapshot.Integration.Branch, Base: runtime.contract.Repository.DefaultBranch, IssueNumber: snapshot.ParentIssue}
	pr, err := o.deps.GitHub.CreateDraftPR(ctx, repo, request)
	if err != nil {
		return err
	}
	if pr.Number <= 0 || pr.Head != snapshot.Integration.Branch || pr.Base != runtime.contract.Repository.DefaultBranch || pr.HeadSHA != snapshot.IntegrationSHA {
		return errors.New("draft PR response does not match exact integration identity")
	}
	snapshot.PullRequest = pr.Number
	snapshot.PullRequestURL = pr.HTMLURL
	snapshot.PullRequestHeadSHA = pr.HeadSHA
	snapshot.PullRequestDraft = pr.Draft
	if err := o.parallelFinish(ctx, snapshot, "Draft PR 생성 완료"); err != nil {
		return err
	}
	return o.parallelTransition(ctx, snapshot, contract.PhaseCI, "latest main 통합과 Full Suite 시작")
}

func (o *Orchestrator) advanceParallelCI(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	repo := github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}
	switch snapshot.ActionCursor {
	case 0:
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "latest-main commit locator is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_read_latest_main", "latest main SHA 확인", nil); err != nil {
			return err
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.RepositoryPath)
		if err != nil || !validCommitSHA(sha) {
			return o.parallelBlock(ctx, snapshot, "latest main SHA is unavailable")
		}
		snapshot.MainSHA = strings.ToLower(strings.TrimSpace(sha))
		return o.parallelFinish(ctx, snapshot, "latest main SHA 저장")
	case 1:
		merger, ok := o.deps.Worktree.(interface {
			MergeCommitNoFF(context.Context, string, string) error
		})
		if !ok {
			return o.parallelBlock(ctx, snapshot, "latest-main integration port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_merge_latest_main", "latest main을 integration에 반영", map[string]any{"sha": snapshot.MainSHA}); err != nil {
			return err
		}
		if err := merger.MergeCommitNoFF(ctx, snapshot.Integration.Path, snapshot.MainSHA); err != nil {
			if errors.Is(err, worktree.ErrConflict) {
				return o.parallelBlock(ctx, snapshot, "latest-main Git merge conflict blocks integration")
			}
			return err
		}
		return o.parallelFinish(ctx, snapshot, "latest main integration 완료")
	case 2:
		checker, ok := o.deps.Worktree.(interface {
			RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error)
		})
		if !ok {
			return o.parallelBlock(ctx, snapshot, "Full Suite verification port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_full_suite", "Full Suite 실행", nil); err != nil {
			return err
		}
		checks, err := checker.RunChecks(ctx, snapshot.Integration.Path, runtime.contract.Verification)
		if err != nil || !parallelGitChecksValid(checks, runtime.contract.Verification) {
			return o.parallelCIRepairOrBlock(ctx, snapshot, runtime, "Full Suite failed")
		}
		snapshot.FinalChecks = parallelVerificationEvidence(checks)
		return o.parallelFinish(ctx, snapshot, "Full Suite evidence 저장")
	case 3:
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "final integration SHA locator is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_read_final_sha", "final integration SHA 확인", nil); err != nil {
			return err
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(sha) {
			return o.parallelBlock(ctx, snapshot, "final integration SHA is unavailable")
		}
		snapshot.FinalSHA = strings.ToLower(strings.TrimSpace(sha))
		snapshot.FinalChecksSHA = ""
		return o.parallelFinish(ctx, snapshot, "final integration SHA 저장")
	case 4:
		git, ok := o.deps.Worktree.(interface {
			PushBranch(context.Context, string, string, string) error
		})
		if !ok {
			return o.parallelBlock(ctx, snapshot, "final SHA push port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_push_final_sha", "final SHA push", map[string]any{"sha": snapshot.FinalSHA}); err != nil {
			return err
		}
		if err := git.PushBranch(ctx, snapshot.Integration.Path, "origin", snapshot.Integration.Branch); err != nil {
			return err
		}
		return o.parallelFinish(ctx, snapshot, "final SHA push 완료")
	case 5:
		readier, ok := o.deps.GitHub.(github.PullRequestReadier)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "pull request ready port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_mark_pr_ready", "Draft PR를 ready로 전환", nil); err != nil {
			return err
		}
		pr, err := readier.MarkReadyForReview(ctx, repo, snapshot.PullRequest)
		if err != nil || pr.Number != snapshot.PullRequest || pr.Draft || pr.HeadSHA != snapshot.FinalSHA {
			return o.parallelBlock(ctx, snapshot, "ready PR response does not match final SHA")
		}
		snapshot.PullRequestDraft = false
		snapshot.PullRequestHeadSHA = pr.HeadSHA
		return o.parallelFinish(ctx, snapshot, "PR ready 전환 완료")
	case 6:
		if err := o.parallelPrepare(ctx, snapshot, "parallel_read_pr", "PR mergeability 확인", nil); err != nil {
			return err
		}
		pr, err := o.deps.GitHub.GetPullRequest(ctx, repo, snapshot.PullRequest)
		if err != nil || pr.Number != snapshot.PullRequest || pr.Base != runtime.contract.Repository.DefaultBranch {
			return o.parallelBlock(ctx, snapshot, "PR identity is unavailable")
		}
		if pr.HeadSHA != snapshot.FinalSHA {
			// Any changed final SHA invalidates old checks and requires a fresh
			// Full Suite/push/check observation sequence.
			snapshot.FinalSHA = pr.HeadSHA
			snapshot.FinalChecksSHA = ""
			snapshot.FinalChecks = nil
			snapshot.ActionCursor = 0
			return o.parallelFinish(ctx, snapshot, "PR head SHA changed; stale checks discarded")
		}
		if pr.Mergeable != nil {
			snapshot.MergeabilityKnown = true
			snapshot.Mergeable = *pr.Mergeable
			snapshot.CIState = map[bool]string{true: "mergeable", false: "conflict"}[*pr.Mergeable]
		}
		return o.parallelFinish(ctx, snapshot, "PR mergeability 확인 완료")
	case 7:
		reader, ok := o.deps.GitHub.(github.CheckReader)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "GitHub checks port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_read_checks", "final SHA checks 확인", map[string]any{"sha": snapshot.FinalSHA}); err != nil {
			return err
		}
		checks, err := reader.GetChecks(ctx, repo, snapshot.FinalSHA)
		if err != nil {
			return err
		}
		if !parallelGitHubChecksValid(checks) {
			return o.parallelBlock(ctx, snapshot, "final SHA checks evidence is malformed")
		}
		if len(checks) == 0 {
			snapshot.PendingAction = ""
			snapshot.Summary = "final SHA checks 대기 중"
			snapshot.UpdatedAt = o.now()
			if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
				return err
			}
			return o.append(ctx, snapshot.RunID, state.Event{Type: "checks_pending", Phase: snapshot.Phase, Message: snapshot.Summary})
		}
		for _, check := range checks {
			if check.State == "pending" {
				snapshot.PendingAction = ""
				snapshot.Summary = "final SHA checks 대기 중"
				snapshot.UpdatedAt = o.now()
				if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
					return err
				}
				return o.append(ctx, snapshot.RunID, state.Event{Type: "checks_pending", Phase: snapshot.Phase, Message: snapshot.Summary})
			}
		}
		for _, check := range checks {
			if check.State != "success" {
				return o.parallelCIRepairOrBlock(ctx, snapshot, runtime, "final SHA checks did not pass")
			}
		}
		snapshot.FinalChecksSHA = snapshot.FinalSHA
		snapshot.CIState = "success"
		return o.parallelFinish(ctx, snapshot, "final SHA checks success 확인")
	case 8:
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "latest-main re-read port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_reread_latest_main", "Merge Gate 직전 latest main 재확인", nil); err != nil {
			return err
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.RepositoryPath)
		if err != nil || !validCommitSHA(sha) {
			return o.parallelBlock(ctx, snapshot, "latest main re-read failed")
		}
		if strings.TrimSpace(sha) != snapshot.MainSHA {
			snapshot.MainSHA = strings.TrimSpace(sha)
			snapshot.FinalChecksSHA = ""
			snapshot.FinalSHA = ""
			snapshot.FinalChecks = nil
			snapshot.ActionCursor = 0
			return o.parallelFinish(ctx, snapshot, "main moved; latest-main sequence restarted")
		}
		return o.parallelFinish(ctx, snapshot, "latest main unchanged")
	case 9:
		return o.evaluateParallelGate(ctx, snapshot, runtime)
	default:
		return o.parallelBlock(ctx, snapshot, "알 수 없는 CI 단계입니다")
	}
}

func parallelGitHubChecksValid(checks []github.CheckState) bool {
	seen := make(map[string]struct{}, len(checks))
	for _, check := range checks {
		if strings.TrimSpace(check.Name) == "" || strings.TrimSpace(check.Name) != check.Name {
			return false
		}
		if _, exists := seen[check.Name]; exists {
			return false
		}
		seen[check.Name] = struct{}{}
		if check.State != "success" && check.State != "pending" && check.State != "failure" {
			return false
		}
	}
	return true
}

func (o *Orchestrator) evaluateParallelGate(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	changed := make([]string, 0)
	for _, taskState := range snapshot.Tasks {
		changed = append(changed, taskState.Agent.ChangedFiles...)
	}
	reasons := mergegate.ProtectedReasons(changed, runtime.contract.RiskCategories, snapshot.ReviewRiskCategories)
	snapshot.ProtectedReasons = reasons
	mergeableKnown := snapshot.MergeabilityKnown
	decision := mergegate.Evaluate(mergegate.Input{
		AcceptanceMet: true, BuildersComplete: parallelAllCompleted(snapshot), ReviewerApproved: snapshot.ReviewDecision == "accept",
		Checks: []mergegate.CheckState{{Name: "final-sha", State: func() string {
			if snapshot.FinalChecksSHA == snapshot.FinalSHA {
				return "success"
			}
			return "pending"
		}()}},
		LatestMainTested:  snapshot.MainSHA != "" && snapshot.FinalChecksSHA == snapshot.FinalSHA && parallelStateChecksValid(snapshot.FinalChecks, runtime.contract.Verification),
		MergeabilityKnown: mergeableKnown, Mergeable: snapshot.Mergeable,
		ProtectedReasons: reasons, OperatorConfirmed: snapshot.ProtectedConfirmed,
	})
	switch decision.Kind {
	case mergegate.Merge:
		return o.parallelTransition(ctx, snapshot, contract.PhaseMerging, "Merge Gate 통과")
	case mergegate.NeedsOperator:
		snapshot.Summary = "Protected Change 확인 필요: " + strings.Join(decision.Reasons, ", ")
		return o.parallelTransition(ctx, snapshot, contract.PhaseNeedsOperator, snapshot.Summary)
	case mergegate.Block:
		return o.parallelBlock(ctx, snapshot, "Merge Gate가 병합을 차단했습니다")
	default:
		snapshot.Summary = "Merge Gate evidence 대기 중"
		snapshot.UpdatedAt = o.now()
		return o.deps.Store.Save(ctx, *snapshot)
	}
}

func parallelStateChecksValid(got []state.VerificationEvidence, required []string) bool {
	if len(got) != len(required) || len(required) == 0 {
		return false
	}
	seen := make(map[string]bool, len(got))
	for _, check := range got {
		if check.Command == "" || check.Outcome != "passed" || check.Duration == "" {
			return false
		}
		duration, err := time.ParseDuration(check.Duration)
		if err != nil || duration <= 0 {
			return false
		}
		if seen[check.Command] {
			return false
		}
		seen[check.Command] = true
	}
	for _, command := range required {
		if !seen[command] {
			return false
		}
	}
	return true
}

func parallelAllCompleted(snapshot *state.RunSnapshot) bool {
	if len(snapshot.Tasks) == 0 {
		return false
	}
	for _, task := range snapshot.Tasks {
		if task.State != "completed" {
			return false
		}
	}
	return true
}

func (o *Orchestrator) parallelCIRepairOrBlock(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, message string) error {
	taskID := firstParallelTaskID(snapshot)
	repairPath := "src"
	if taskID != "" {
		if len(snapshot.Tasks[taskID].Agent.ChangedFiles) > 0 {
			repairPath = snapshot.Tasks[taskID].Agent.ChangedFiles[0]
		} else if task, ok := parallelTask(runtime.contract, taskID); ok && len(task.AllowedPaths) > 0 {
			repairPath = strings.TrimSuffix(task.AllowedPaths[0], "/**")
		}
	}
	decision := review.Decide(*snapshot, review.ReviewResult{Source: "ci", Blocking: true, Findings: []review.Finding{{ID: "ci", Summary: message, Paths: []string{repairPath}}}})
	switch decision.Kind {
	case review.Repair:
		snapshot.RepairCount = decision.NextCount
		if taskID == "" {
			return o.parallelBlock(ctx, snapshot, "CI repair 대상 Task가 없습니다")
		}
		taskState := snapshot.Tasks[taskID]
		taskState.State = "running"
		taskState.Stage = "repair_prompt"
		taskState.RepairCount++
		taskState.Prompt = state.PromptReceipt{RequestID: parallelPromptRequestID(snapshot.RunID, taskID, taskState.RepairCount)}
		snapshot.ReviewFindings = []state.ReviewFinding{{ID: "ci", Summary: message, Paths: []string{repairPath}}}
		snapshot.Tasks[taskID] = taskState
		snapshot.PendingAction = ""
		return o.parallelTransition(ctx, snapshot, contract.PhaseBuilding, "CI findings에 대한 bounded repair 시작")
	default:
		return o.parallelBlock(ctx, snapshot, message)
	}
}

func (o *Orchestrator) advanceParallelMerge(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	merger, ok := o.deps.GitHub.(github.PullRequestMerger)
	if !ok {
		return o.parallelBlock(ctx, snapshot, "GitHub merge port is unavailable")
	}
	if snapshot.PullRequest <= 0 || !validCommitSHA(snapshot.FinalSHA) || snapshot.PullRequestHeadSHA != snapshot.FinalSHA {
		return o.parallelBlock(ctx, snapshot, "merge requires PR head equal to final SHA")
	}
	currentPR, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
	if err != nil || currentPR.Number != snapshot.PullRequest || currentPR.State != "open" || currentPR.HeadSHA != snapshot.FinalSHA || currentPR.Base != runtime.contract.Repository.DefaultBranch || currentPR.Mergeable == nil || !*currentPR.Mergeable {
		return o.parallelBlock(ctx, snapshot, "merge preflight PR evidence is stale or unmergeable")
	}
	if err := o.parallelPrepare(ctx, snapshot, "parallel_merge_main", "main에 PR merge", map[string]any{"pullRequest": snapshot.PullRequest, "sha": snapshot.FinalSHA}); err != nil {
		return err
	}
	result, err := merger.MergePullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest, snapshot.FinalSHA, "merge")
	if err != nil {
		return err
	}
	if !result.Merged || !validCommitSHA(result.SHA) {
		return errors.New("GitHub merge response is incomplete")
	}
	snapshot.MergeSHA = result.SHA
	snapshot.PullRequestMerged = true
	if snapshot.ProjectAutomationEnabled {
		// Keep the Done ProjectV2 mutation in its own Advance so the main merge
		// remains the sole external side effect of this Advance.
		snapshot.PendingAction = "parallel_project_status_done"
		snapshot.Phase = contract.PhaseMerging
		snapshot.Summary = "main merge 완료; Project Done 상태 대기 중"
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		if err := o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: "main merge 완료"}); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project Done 상태 업데이트", Data: map[string]any{"action": snapshot.PendingAction}})
	}
	snapshot.Phase = contract.PhaseCompleted
	snapshot.Summary = "병렬 실행과 감독형 자동 병합 완료"
	snapshot.PendingAction = ""
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	if err := o.append(ctx, snapshot.RunID, state.Event{Type: "completed", Phase: contract.PhaseCompleted, Message: snapshot.Summary}); err != nil {
		return err
	}
	return nil
}

// reconcileParallelPending performs only read/reconcile operations for an
// intent that survived a crash. A missing observation clears the intent so a
// later Advance may retry; it never blindly repeats an ambiguous write.
func (o *Orchestrator) reconcileParallelPending(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	action := snapshot.PendingAction
	if strings.HasPrefix(action, "parallel_project_status_") {
		status := parallelProjectStatus(snapshot.Phase)
		if strings.HasSuffix(action, "_done") && snapshot.ProjectAutomationEnabled {
			if o.deps.GitHub == nil || snapshot.Registration.NodeID == "" {
				return ErrPendingReconcile
			}
			if err := o.deps.GitHub.SetProjectStatus(ctx, o.deps.Project, snapshot.Registration.NodeID, "Done"); err != nil {
				return err
			}
			snapshot.ProjectStatus = "Done"
			snapshot.Phase = contract.PhaseCompleted
			snapshot.PullRequestMerged = true
			return o.parallelFinish(ctx, snapshot, "Project Done status and main merge reconciled")
		}
		snapshot.ProjectStatus = status
		return o.parallelFinishPreserveCursor(ctx, snapshot, "Project status intent reconciled")
	}
	if strings.HasPrefix(action, "parallel_create_worktree_") {
		id := strings.TrimPrefix(action, "parallel_create_worktree_")
		return o.reconcileParallelTaskWorktree(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_start_agent_") {
		id := strings.TrimPrefix(action, "parallel_start_agent_")
		return o.reconcileParallelTaskAgent(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_baseline_") {
		id := strings.TrimPrefix(action, "parallel_baseline_")
		if id == "reviewer" {
			return o.reconcileParallelReviewerBaseline(ctx, snapshot)
		}
		return o.reconcileParallelTaskBaseline(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_prompt_") {
		id := strings.TrimPrefix(action, "parallel_prompt_")
		if id == "reviewer" {
			return o.reconcileParallelPromptReceipt(ctx, snapshot, snapshot.Reviewer.Name, snapshot.ReviewerPrompt, func() { snapshot.Reviewer.Verification = []string{"review schema sent"} })
		}
		return o.reconcileParallelTaskPrompt(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_collect_evidence_") {
		return o.reconcileParallelTaskEvidence(ctx, snapshot, runtime, strings.TrimPrefix(action, "parallel_collect_evidence_"))
	}
	if strings.HasPrefix(action, "parallel_inspect_commit_") {
		return o.reconcileParallelTaskInspection(ctx, snapshot, runtime, strings.TrimPrefix(action, "parallel_inspect_commit_"))
	}
	if strings.HasPrefix(action, "parallel_merge_task_") {
		id := strings.TrimPrefix(action, "parallel_merge_task_")
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return ErrPendingReconcile
		}
		current, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(current) || strings.TrimSpace(current) == runtime.contract.BaseCommit {
			return ErrPendingReconcile
		}
		taskState, exists := snapshot.Tasks[id]
		if !exists {
			return ErrPendingReconcile
		}
		taskState.State, taskState.Stage = "completed", "merged"
		snapshot.Tasks[id] = taskState
		return o.parallelFinish(ctx, snapshot, "Task immutable merge intent reconciled")
	}
	switch action {
	case "parallel_wave_end_verification":
		return o.reconcileParallelChecks(ctx, snapshot, runtime.contract.Verification, true)
	case "parallel_merge_latest_main":
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return ErrPendingReconcile
		}
		current, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(current) || current == runtime.contract.BaseCommit {
			return ErrPendingReconcile
		}
		return o.parallelFinish(ctx, snapshot, "latest-main merge intent reconciled")
	case "parallel_locate_integration_sha":
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return ErrPendingReconcile
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(sha) {
			return ErrPendingReconcile
		}
		snapshot.IntegrationSHA = strings.TrimSpace(sha)
		return o.parallelFinish(ctx, snapshot, "integration SHA intent reconciled")
	case "parallel_push_integration", "parallel_push_final_sha":
		// A push is reconciled by the exact current integration commit and the
		// durable branch/sha pair. There is no create-on-retry operation here.
		if !validCommitSHA(snapshot.IntegrationSHA) && !validCommitSHA(snapshot.FinalSHA) {
			return ErrPendingReconcile
		}
		return o.parallelFinish(ctx, snapshot, "branch push intent reconciled")
	case "parallel_create_draft_pr":
		finder, ok := o.deps.GitHub.(github.PullRequestFinder)
		if !ok {
			return ErrPendingReconcile
		}
		pr, found, err := finder.FindOpenPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.Integration.Branch, runtime.contract.Repository.DefaultBranch)
		if err != nil {
			return ErrPendingReconcile
		}
		if !found {
			return o.clearParallelPending(ctx, snapshot, "Draft PR not found; next Advance may create it")
		}
		snapshot.PullRequest, snapshot.PullRequestURL, snapshot.PullRequestHeadSHA, snapshot.PullRequestDraft = pr.Number, pr.HTMLURL, pr.HeadSHA, pr.Draft
		return o.parallelFinish(ctx, snapshot, "Draft PR intent reconciled")
	case "parallel_read_checks":
		reader, ok := o.deps.GitHub.(github.CheckReader)
		if !ok {
			return ErrPendingReconcile
		}
		checks, err := reader.GetChecks(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.FinalSHA)
		if err != nil || !parallelGitHubChecksValid(checks) {
			return ErrPendingReconcile
		}
		if len(checks) == 0 {
			return o.clearParallelPending(ctx, snapshot, "final SHA checks still pending")
		}
		for _, check := range checks {
			if check.State == "pending" {
				return o.clearParallelPending(ctx, snapshot, "final SHA checks still pending")
			}
			if check.State != "success" {
				return o.parallelCIRepairOrBlock(ctx, snapshot, runtime, "final SHA checks did not pass")
			}
		}
		snapshot.FinalChecksSHA = snapshot.FinalSHA
		snapshot.CIState = "success"
		return o.parallelFinish(ctx, snapshot, "final SHA checks intent reconciled")
	case "parallel_mark_pr_ready":
		pr, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		if err != nil || pr.Number != snapshot.PullRequest || pr.Draft || pr.HeadSHA != snapshot.FinalSHA {
			return ErrPendingReconcile
		}
		snapshot.PullRequestDraft = false
		return o.parallelFinish(ctx, snapshot, "PR ready intent reconciled")
	case "parallel_read_pr":
		pr, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		if err != nil || pr.Number != snapshot.PullRequest {
			return ErrPendingReconcile
		}
		if pr.HeadSHA != snapshot.FinalSHA {
			snapshot.FinalSHA, snapshot.FinalChecksSHA = pr.HeadSHA, ""
			snapshot.FinalChecks = nil
			snapshot.ActionCursor = 0
		}
		if pr.Mergeable != nil {
			snapshot.MergeabilityKnown, snapshot.Mergeable = true, *pr.Mergeable
			if *pr.Mergeable {
				snapshot.CIState = "mergeable"
			} else {
				snapshot.CIState = "conflict"
			}
		}
		return o.parallelFinish(ctx, snapshot, "PR read intent reconciled")
	case "parallel_merge_main":
		pr, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		if err != nil || pr.State != "closed" {
			return ErrPendingReconcile
		}
		snapshot.PullRequestMerged = true
		snapshot.Phase = contract.PhaseCompleted
		return o.parallelFinish(ctx, snapshot, "main merge intent reconciled")
	default:
		return ErrPendingReconcile
	}
}

func (o *Orchestrator) clearParallelPending(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "reconcile_not_executed", Phase: snapshot.Phase, Message: message})
}

func (o *Orchestrator) reconcileParallelTaskWorktree(ctx context.Context, snapshot *state.RunSnapshot, id string) error {
	locator, ok := o.deps.Herdr.(WorktreeLocator)
	if !ok {
		return ErrPendingReconcile
	}
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return ErrPendingReconcile
	}
	found, exists, err := locator.FindWorktree(ctx, snapshot.RepositoryPath, taskState.Worktree.Path, "threaddock-"+string(snapshot.RunID)+"-"+id)
	if err != nil || !exists {
		return o.clearParallelPending(ctx, snapshot, "Builder Worktree not found; next Advance may create it")
	}
	if found.Path != taskState.Worktree.Path || found.WorkspaceID == "" || found.PaneID == "" {
		return ErrPendingReconcile
	}
	taskState.Worktree.WorkspaceID, taskState.Worktree.PaneID = found.WorkspaceID, found.PaneID
	taskState.State, taskState.Stage = "running", "start"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder Worktree intent reconciled")
}

func (o *Orchestrator) reconcileParallelTaskAgent(ctx context.Context, snapshot *state.RunSnapshot, id string) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return ErrPendingReconcile
	}
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return ErrPendingReconcile
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	if errors.Is(err, herdr.ErrAgentNotFound) {
		return o.clearParallelPending(ctx, snapshot, "Builder Agent not found; next Advance may start it")
	}
	if err != nil || !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return ErrPendingReconcile
	}
	taskState.Agent.SessionID = info.SessionID
	taskState.Agent.IdentitySource = "provider"
	if isTerminalIdentity(info.SessionID) {
		taskState.Agent.IdentitySource, taskState.NativeResume = "terminal", false
	} else {
		taskState.NativeResume = true
	}
	taskState.Stage = "baseline"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder Agent intent reconciled")
}

func (o *Orchestrator) reconcileParallelReviewerBaseline(ctx context.Context, snapshot *state.RunSnapshot) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return ErrPendingReconcile
	}
	info, err := locator.GetInfo(ctx, snapshot.Reviewer.Name)
	if err != nil || !matchesParallelAgentIdentity(info, snapshot.Reviewer.Name, snapshot.ReviewerWorktree) {
		return ErrPendingReconcile
	}
	snapshot.Reviewer.SessionID, snapshot.ReviewerPrompt = info.SessionID, state.PromptReceipt{RequestID: string(snapshot.RunID) + ":reviewer-prompt", BaselineSeq: info.StateChangeSeq}
	return o.parallelFinish(ctx, snapshot, "Reviewer baseline intent reconciled")
}

func (o *Orchestrator) reconcileParallelTaskBaseline(ctx context.Context, snapshot *state.RunSnapshot, id string) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return ErrPendingReconcile
	}
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return ErrPendingReconcile
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	if err != nil || !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return ErrPendingReconcile
	}
	taskState.Agent.SessionID = info.SessionID
	taskState.Prompt = state.PromptReceipt{RequestID: parallelPromptRequestID(snapshot.RunID, id, taskState.RepairCount), BaselineSeq: info.StateChangeSeq}
	taskState.Stage = "prompt"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder baseline intent reconciled")
}

func (o *Orchestrator) reconcileParallelTaskPrompt(ctx context.Context, snapshot *state.RunSnapshot, id string) error {
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return ErrPendingReconcile
	}
	return o.reconcileParallelPromptReceipt(ctx, snapshot, taskState.Agent.Name, taskState.Prompt, func() { taskState.Stage = "evidence"; snapshot.Tasks[id] = taskState })
}

func (o *Orchestrator) reconcileParallelPromptReceipt(ctx context.Context, snapshot *state.RunSnapshot, name string, receipt state.PromptReceipt, onObserved func()) error {
	reader, ok := o.deps.Herdr.(PromptReceiptReader)
	if !ok {
		return ErrPendingReconcile
	}
	info, observed, err := reader.ReadPromptReceipt(ctx, name, receipt.RequestID)
	if err != nil || strings.TrimSpace(info.Name) != "" && info.Name != name || info.StateChangeSeq <= 0 {
		return ErrPendingReconcile
	}
	if observed {
		onObserved()
		return o.parallelFinish(ctx, snapshot, "prompt intent reconciled")
	}
	if info.StateChangeSeq == receipt.BaselineSeq {
		return o.clearParallelPending(ctx, snapshot, "prompt not observed; next Advance may resend")
	}
	return ErrPendingReconcile
}

func (o *Orchestrator) reconcileParallelTaskEvidence(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string) error {
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return ErrPendingReconcile
	}
	reader, ok := o.deps.Herdr.(EvidenceReader)
	if !ok {
		return ErrPendingReconcile
	}
	task, ok := parallelTask(runtime.contract, id)
	if !ok {
		return ErrPendingReconcile
	}
	evidence, err := reader.ReadEvidence(ctx, taskState.Agent.Name)
	if err != nil || evidence.RequestID != taskState.Prompt.RequestID || !validCommitSHA(strings.ToLower(evidence.CommitSHA)) || !parallelVerificationValid(evidence.Verification, task.Verification) {
		return ErrPendingReconcile
	}
	taskState.Agent.CommitSHA = strings.ToLower(evidence.CommitSHA)
	taskState.Agent.RequestID = evidence.RequestID
	taskState.Agent.VerificationEvidence = parallelEvidence(evidence.Verification)
	taskState.Stage = "inspect"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder evidence intent reconciled")
}

func parallelEvidence(checks []herdr.VerificationCheck) []state.VerificationEvidence {
	result := make([]state.VerificationEvidence, 0, len(checks))
	for _, check := range checks {
		result = append(result, state.VerificationEvidence{Command: check.Command, Outcome: "passed", Duration: check.Duration})
	}
	return result
}

func (o *Orchestrator) reconcileParallelTaskInspection(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string) error {
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return ErrPendingReconcile
	}
	task, ok := parallelTask(runtime.contract, id)
	if !ok {
		return ErrPendingReconcile
	}
	inspector, ok := o.deps.Worktree.(WorktreeInspector)
	if !ok {
		return ErrPendingReconcile
	}
	inspection, err := inspector.InspectCommit(ctx, taskState.Worktree.Path, runtime.contract.BaseCommit, task.Branch, taskState.Agent.CommitSHA)
	if err != nil || validateInspection(inspection, taskState.Agent.CommitSHA, task.Branch, task.AllowedPaths) != nil || containsCredential(inspection.Patch) {
		return ErrPendingReconcile
	}
	taskState.Agent.Branch, taskState.Agent.ChangedFiles, taskState.Agent.Patch = inspection.Branch, append([]string(nil), inspection.ChangedFiles...), inspection.Patch
	taskState.Stage = "inspected"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder inspection intent reconciled")
}

func (o *Orchestrator) reconcileParallelChecks(ctx context.Context, snapshot *state.RunSnapshot, commands []string, wave bool) error {
	reader, ok := o.deps.Worktree.(interface {
		RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error)
	})
	if !ok {
		return ErrPendingReconcile
	}
	if commands == nil {
		return ErrPendingReconcile
	}
	checks, err := reader.RunChecks(ctx, snapshot.Integration.Path, commands)
	if err != nil || !parallelGitChecksValid(checks, commands) {
		return ErrPendingReconcile
	}
	if wave {
		snapshot.IntegrationVerification = parallelVerificationEvidence(checks)
	} else {
		snapshot.FinalChecks = parallelVerificationEvidence(checks)
		snapshot.FinalChecksSHA = snapshot.FinalSHA
	}
	return o.parallelFinish(ctx, snapshot, "verification intent reconciled")
}
