package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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
	rawRole, rawTaskID := role, taskID
	role = normalizeAgentPart(role)
	taskID = normalizeAgentPart(taskID)
	if role == "" {
		role = "agent"
	}
	if taskID == "" {
		taskID = "task"
	}
	digest := sha256.Sum256([]byte(string(runID) + "\x00" + rawRole + "\x00" + rawTaskID))
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

func parallelTaskAction(prefix, taskID string) string {
	digest := sha256.Sum256([]byte(taskID))
	return prefix + normalizeAgentPart(taskID) + "-" + hex.EncodeToString(digest[:])[:8]
}

func pendingParallelTaskID(snapshot *state.RunSnapshot, action, prefix string) string {
	if snapshot.PendingTaskID != "" {
		return snapshot.PendingTaskID
	}
	value := strings.TrimPrefix(action, prefix)
	if index := strings.LastIndex(value, "-"); index > 0 && len(value)-index-1 == 8 {
		value = value[:index]
	}
	return value
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
	if snapshot.Phase == contract.PhaseMerging && snapshot.ProtectedConfirmed {
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
type ProjectStatusReader = github.ProjectStatusReader

func projectMutationPorts(client github.Client) (github.ProjectItemAdder, github.ProjectStatusUpdater) {
	adder, _ := client.(github.ProjectItemAdder)
	updater, _ := client.(github.ProjectStatusUpdater)
	return adder, updater
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
	if want := parallelProjectStatus(snapshot.Phase); want != "" && snapshot.ProjectStatus != want {
		// Project observation/add/update is itself one Advance action. Do not
		// continue into the phase body with a freshly prepared intent.
		return o.reconcileParallelProject(ctx, snapshot, runtime)
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

func (o *Orchestrator) parallelPrepareTask(ctx context.Context, snapshot *state.RunSnapshot, taskID, action, message string, data map[string]any) error {
	snapshot.PendingTaskID = taskID
	return o.parallelPrepare(ctx, snapshot, action, message, data)
}

func (o *Orchestrator) parallelFinish(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
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
	snapshot.PendingTaskID = ""
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
	reader, readOK := o.deps.GitHub.(ProjectStatusReader)
	if !readOK || snapshot.Registration.NodeID == "" {
		return o.parallelBlock(ctx, snapshot, "Project status read port is unavailable")
	}
	observeAction := "parallel_project_observe_" + strings.ToLower(strings.ReplaceAll(want, " ", "_"))
	if err := o.parallelPrepare(ctx, snapshot, observeAction, "Project 상태 관찰", map[string]any{"status": want}); err != nil {
		return err
	}
	observed, err := reader.ReadProjectStatus(ctx, o.deps.Project, snapshot.Registration.NodeID)
	if err != nil {
		return o.parallelBlock(ctx, snapshot, "Project status read is uncertain")
	}
	if !projectItemPresent(observed) {
		snapshot.PendingAction = "parallel_project_add_" + strings.ToLower(strings.ReplaceAll(want, " ", "_"))
		snapshot.PendingTaskID = ""
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project item 추가", Data: map[string]any{"action": snapshot.PendingAction, "status": want}})
	}
	if strings.TrimSpace(observed.ItemID) == "" {
		return o.parallelBlock(ctx, snapshot, "Project item identity is unavailable")
	}
	snapshot.ProjectItemID = observed.ItemID
	if projectStatusPresent(observed) && observed.Status == want {
		snapshot.ProjectStatus = want
		return o.parallelFinishPreserveCursor(ctx, snapshot, "Project status 관찰 완료")
	}
	snapshot.PendingAction = "parallel_project_update_" + strings.ToLower(strings.ReplaceAll(want, " ", "_"))
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project status 업데이트", Data: map[string]any{"action": snapshot.PendingAction, "itemId": observed.ItemID, "status": want}})
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
	if len(snapshot.TaskOrder) == 0 {
		for id := range snapshot.Tasks {
			snapshot.TaskOrder = append(snapshot.TaskOrder, id)
		}
		sort.Strings(snapshot.TaskOrder)
	}
	// Continue one already-dispatched task in round-robin order. This gives
	// each Advance one deterministic action without starving another slot.
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
	// Fill both scheduler slots before observing any one running agent. This
	// keeps independent ready tasks concurrent and avoids treating a working
	// agent with no evidence yet as a failure.
	if active < 2 {
		selected := scheduler.Next(parallelGraph(runtime.contract), parallelSchedulerStates(snapshot), 2)
		for _, id := range selected {
			if snapshot.Tasks[id].State == "pending" {
				return o.createParallelTaskWorktree(ctx, snapshot, runtime, id)
			}
		}
	}
	start := 0
	for index, id := range snapshot.TaskOrder {
		if id == snapshot.CurrentTask {
			start = (index + 1) % len(snapshot.TaskOrder)
			break
		}
	}
	for offset := 0; offset < len(snapshot.TaskOrder); offset++ {
		id := snapshot.TaskOrder[(start+offset)%len(snapshot.TaskOrder)]
		taskState := snapshot.Tasks[id]
		if taskState.State == "running" {
			if taskState.Stage == "inspected" {
				continue
			}
			return o.advanceParallelTask(ctx, snapshot, runtime, id, taskState)
		}
	}
	for _, taskState := range snapshot.Tasks {
		if taskState.State == "running" && taskState.Stage == "inspected" {
			return o.parallelTransition(ctx, snapshot, contract.PhaseIntegrating, "inspected Builder commit ready for incremental integration")
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
	expectedLabel := "threaddock-" + string(snapshot.RunID) + "-" + id
	stateBeforeCreate := snapshot.Tasks[id]
	stateBeforeCreate.ExpectedBranch = task.Branch
	stateBeforeCreate.ExpectedLabel = expectedLabel
	stateBeforeCreate.ExpectedBaseCommit = runtime.contract.BaseCommit
	stateBeforeCreate.Worktree.Branch = task.Branch
	stateBeforeCreate.State = "pending"
	snapshot.Tasks[id] = stateBeforeCreate
	action := parallelTaskAction("parallel_create_worktree_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "Builder Worktree 생성", map[string]any{"taskId": id, "expectedBranch": task.Branch, "label": expectedLabel}); err != nil {
		return err
	}
	created, err := o.deps.Herdr.CreateWorktree(ctx, herdr.CreateWorktreeRequest{Cwd: snapshot.RepositoryPath, Branch: task.Branch, Base: runtime.contract.BaseCommit, Label: expectedLabel})
	if err != nil {
		return err
	}
	if strings.TrimSpace(created.Path) == "" || strings.TrimSpace(created.WorkspaceID) == "" || strings.TrimSpace(created.PaneID) == "" {
		return errors.New("Herdr returned incomplete parallel Builder Worktree identity")
	}
	stateBeforeCreate.State, stateBeforeCreate.Stage = "running", "start"
	stateBeforeCreate.Worktree = state.WorktreeState{Path: created.Path, WorkspaceID: created.WorkspaceID, PaneID: created.PaneID, Branch: task.Branch}
	snapshot.Tasks[id] = stateBeforeCreate
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
	case "recovery_prompt":
		return o.parallelRecoveryPromptTask(ctx, snapshot, id, taskState)
	case "resume":
		return o.parallelResumeTask(ctx, snapshot, id, taskState)
	case "evidence", "repair_evidence":
		return o.parallelCollectTaskEvidence(ctx, snapshot, runtime, id, task, taskState)
	case "fingerprint", "repair_fingerprint":
		return o.parallelFingerprintTask(ctx, snapshot, id, taskState)
	case "recovery_observe":
		return o.parallelObserveRecovery(ctx, snapshot, id, taskState)
	case "inspect", "repair_inspect":
		return o.parallelInspectTask(ctx, snapshot, runtime, id, task, taskState)
	default:
		return o.parallelBlock(ctx, snapshot, "알 수 없는 Builder 단계입니다")
	}
}

func (o *Orchestrator) parallelStartTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	if taskState.Agent.Name == "" {
		return errors.New("parallel Builder Agent name is required before start")
	}
	action := parallelTaskAction("parallel_start_agent_", id)
	snapshot.Tasks[id] = taskState
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "Builder Agent 시작", map[string]any{"taskId": id, "agent": taskState.Agent.Name}); err != nil {
		return err
	}
	if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: taskState.Agent.Name, PaneID: taskState.Worktree.PaneID}); err != nil {
		return err
	}
	taskState.Stage = "baseline"
	taskState.Agent.IdentitySource = "provider"
	taskState.LastProgressAt = o.now()
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder Agent 준비 완료")
}

func (o *Orchestrator) parallelBaselineTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return errors.New("parallel Builder identity lookup is required")
	}
	action := parallelTaskAction("parallel_baseline_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "Builder prompt baseline 조회", map[string]any{"taskId": id}); err != nil {
		return err
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	if errors.Is(err, herdr.ErrAgentNotFound) {
		return o.parallelScheduleRecovery(ctx, snapshot, id, taskState, "Builder Agent is absent during identity observation")
	}
	if err != nil {
		return o.parallelScheduleRecovery(ctx, snapshot, id, taskState, "Builder Agent state observation failed")
	}
	if info.State == herdr.AgentStateBlocked || info.State == herdr.AgentStateUnknown {
		return o.parallelOperatorBlock(ctx, snapshot, id, "live Agent identity is stale, blocked, unknown, or absent")
	}
	if !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return o.parallelOperatorBlock(ctx, snapshot, id, "Builder identity reconciliation failed")
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
	taskState.LastProgressAt = o.now()
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder prompt baseline 저장")
}

func (o *Orchestrator) parallelOperatorBlock(ctx context.Context, snapshot *state.RunSnapshot, id, message string) error {
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
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
	action := parallelTaskAction("parallel_prompt_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "Builder packet 전송", map[string]any{"taskId": id, "agent": taskState.Agent.Name, "requestId": taskState.Prompt.RequestID}); err != nil {
		return err
	}
	packet := builderPacket(runtime.contract, task, taskState.Prompt.RequestID)
	if taskState.Stage == "repair_prompt" {
		findings := make([]review.Finding, 0, len(snapshot.ReviewFindings))
		for _, finding := range snapshot.ReviewFindings {
			findings = append(findings, review.Finding{ID: finding.ID, Summary: finding.Summary, Paths: append([]string(nil), finding.Paths...)})
		}
		var err error
		baseSHA := snapshot.RepairBaseSHA
		if baseSHA == "" {
			baseSHA = snapshot.IntegrationSHA
		}
		packet, err = review.BuildRepairPacket(review.RepairPacketInput{AcceptanceCriteria: append(append([]string(nil), runtime.contract.Parent.AcceptanceCriteria...), task.AcceptanceCriteria...), IntegrationSHA: baseSHA, BlockingFindings: findings, AllowedPaths: task.AllowedPaths, RemainingBudget: 2 - snapshot.RepairCount})
		if err != nil {
			return o.parallelBlock(ctx, snapshot, "Repair packet를 생성할 수 없습니다")
		}
		packet += "\nUse requestId=" + taskState.Prompt.RequestID + "."
	}
	if err := o.deps.Herdr.Prompt(ctx, taskState.Agent.Name, packet); err != nil {
		return err
	}
	taskState.Stage = "evidence"
	taskState.LastProgressAt = o.now()
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder packet 전송 완료")
}

func (o *Orchestrator) parallelRecoveryPromptTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	action := parallelTaskAction("parallel_recovery_prompt_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "recovery continuation prompt", map[string]any{"requestId": taskState.Prompt.RequestID}); err != nil {
		return err
	}
	packet := recovery.ContinuationInstruction + "\nUse requestId=" + taskState.Prompt.RequestID + "."
	if err := o.deps.Herdr.Prompt(ctx, taskState.Agent.Name, packet); err != nil {
		return err
	}
	taskState.Stage = "evidence"
	taskState.LastProgressAt = o.now()
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "recovery continuation prompt sent")
}

func (o *Orchestrator) parallelResumeTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	if !taskState.NativeResume {
		return o.parallelOperatorBlock(ctx, snapshot, id, "terminal-only Agent cannot be natively resumed")
	}
	action := parallelTaskAction("parallel_resume_agent_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "provider Agent session resume", nil); err != nil {
		return err
	}
	resumer, ok := o.deps.Herdr.(herdr.SessionResumer)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	if err := resumer.ResumeAgent(ctx, herdr.ResumeAgentRequest{Name: taskState.Agent.Name, PaneID: taskState.Worktree.PaneID, SessionID: taskState.Agent.SessionID}); err != nil {
		return err
	}
	taskState.Stage = "baseline"
	taskState.LastProgressAt = o.now()
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "provider Agent resume requested")
}

func (o *Orchestrator) parallelCollectTaskEvidence(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string, task contract.Task, taskState state.TaskRunState) error {
	previousCommit := taskState.Agent.CommitSHA
	reader, ok := o.deps.Herdr.(EvidenceReader)
	if !ok {
		return o.parallelRecoveryFailure(ctx, snapshot, id, taskState, "Builder evidence reader is unavailable")
	}
	action := parallelTaskAction("parallel_collect_evidence_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "Builder structured evidence 수집", map[string]any{"taskId": id}); err != nil {
		return err
	}
	evidence, err := reader.ReadEvidence(ctx, taskState.Agent.Name)
	commitSHA := strings.ToLower(evidence.CommitSHA)
	if err != nil || evidence.RequestID != taskState.Prompt.RequestID || !validCommitSHA(commitSHA) || !parallelVerificationValid(evidence.Verification, task.Verification) {
		return o.parallelScheduleRecovery(ctx, snapshot, id, taskState, "Builder structured evidence is stale or incomplete")
	}
	taskState.Agent.RequestID = evidence.RequestID
	taskState.Agent.CommitSHA = commitSHA
	taskState.Agent.VerificationEvidence = make([]state.VerificationEvidence, 0, len(evidence.Verification))
	for _, check := range evidence.Verification {
		taskState.Agent.VerificationEvidence = append(taskState.Agent.VerificationEvidence, state.VerificationEvidence{Command: strings.TrimSpace(check.Command), Outcome: "passed", Duration: strings.TrimSpace(check.Duration)})
	}
	wasFreshRequired := taskState.RequiresFreshCommit
	previousCommitSHA := taskState.PreviousCommitSHA
	taskState.PreviousCommitSHA = previousCommit
	taskState.PreviousFingerprint = taskState.ProgressFingerprint
	if wasFreshRequired && commitSHA == previousCommitSHA {
		// The stale-SHA check is an observation result. Defer the recovery
		// policy's Agent lookup to the next Advance so this Advance performs
		// only the evidence read and never compounds it with GetInfo.
		return o.parallelScheduleRecovery(ctx, snapshot, id, taskState, "Builder repair did not produce a fresh commit or fingerprint")
	}
	taskState.RecoveryCount = 0
	taskState.RequiresFreshCommit = false
	taskState.Stage = "fingerprint"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder structured evidence 확인")
}

func (o *Orchestrator) parallelFingerprintTask(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	action := parallelTaskAction("parallel_fingerprint_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "managed Git fingerprint 확인", nil); err != nil {
		return err
	}
	managedFingerprint := ""
	if reader, ok := o.deps.Worktree.(FingerprintReader); ok {
		fingerprint, err := reader.Fingerprint(ctx, snapshot.Integration.Path)
		if err != nil {
			return o.pendingUncertain(ctx, snapshot)
		}
		managedFingerprint = strings.TrimSpace(fingerprint)
	}
	taskState.ProgressFingerprint = RecoveryFingerprint(taskState.Agent.CommitSHA, managedFingerprint, completedTaskIDs(snapshot), taskState.Agent.VerificationEvidence)
	taskState.LastProgressAt = o.now()
	taskState.Stage = "inspect"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "managed Git fingerprint 저장")
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
	action := parallelTaskAction("parallel_inspect_commit_", id)
	if err := o.parallelPrepareTask(ctx, snapshot, id, action, "Git에서 immutable Builder commit 확인", map[string]any{"taskId": id, "commitSha": taskState.Agent.CommitSHA}); err != nil {
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
	locator, hasLocator := o.deps.Herdr.(AgentLocator)
	info, infoErr := herdr.AgentInfo{}, error(nil)
	if hasLocator {
		info, infoErr = locator.GetInfo(ctx, taskState.Agent.Name)
	}
	return o.parallelApplyRecovery(ctx, snapshot, id, taskState, info, infoErr, message)
}

func (o *Orchestrator) parallelScheduleRecovery(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState, message string) error {
	taskState.Stage = "recovery_observe"
	snapshot.Tasks[id] = taskState
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "recovery_observation_required", Phase: snapshot.Phase, Message: message, Data: map[string]any{"taskId": id}})
}

func (o *Orchestrator) parallelObserveRecovery(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	return o.parallelApplyRecovery(ctx, snapshot, id, taskState, info, err, "Builder structured evidence is stale or incomplete")
}

func (o *Orchestrator) parallelApplyRecovery(ctx context.Context, snapshot *state.RunSnapshot, id string, taskState state.TaskRunState, info herdr.AgentInfo, infoErr error, message string) error {
	// The failed observation itself did not produce a new progress fingerprint;
	// acknowledge the last durable fingerprint before applying the recovery
	// policy so a prior successful step does not mask repeated no-progress.
	taskState.PreviousFingerprint = taskState.ProgressFingerprint
	if infoErr == nil && info.Name != "" && !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return o.parallelOperatorBlock(ctx, snapshot, id, "Agent identity changed during recovery")
	}
	if infoErr == nil && info.Name == "" {
		return o.parallelOperatorBlock(ctx, snapshot, id, "Agent state identity is unavailable")
	}
	if errors.Is(infoErr, herdr.ErrAgentNotFound) && !taskState.NativeResume {
		return o.parallelOperatorBlock(ctx, snapshot, id, "terminal-only Agent is absent; operator action is required")
	}
	workingWait := o.deps.WorkingWait
	if workingWait <= 0 {
		workingWait = 60 * time.Minute
	}
	limit := o.deps.RecoveryLimit
	if limit <= 0 {
		limit = 3
	}
	stateName := string(info.State)
	if stateName == "" {
		stateName = "idle"
	}
	alive := infoErr == nil && info.Name != ""
	decision := recovery.Decide(o.now(), recovery.Policy{WorkingWait: workingWait, Limit: limit}, recovery.AgentSnapshot{
		State: stateName, Alive: alive, LastProgress: taskState.LastProgressAt,
		ProgressFingerprint: taskState.ProgressFingerprint, PreviousFingerprint: taskState.PreviousFingerprint,
		RecoveryCount: taskState.RecoveryCount, CanNativeResume: taskState.NativeResume,
	})
	switch decision.Kind {
	case recovery.Wait:
		snapshot.PendingAction = ""
		snapshot.PendingTaskID = ""
		snapshot.Summary = "live Agent progress wait: " + message
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "recovery_wait", Phase: snapshot.Phase, Message: snapshot.Summary, Data: map[string]any{"taskId": id}})
	case recovery.AskOperator:
		return o.parallelOperatorBlock(ctx, snapshot, id, message)
	case recovery.Block:
		return o.parallelBlock(ctx, snapshot, "recovery limit exhausted")
	case recovery.Continue, recovery.ResumeSession:
		if decision.NextCount >= limit {
			return o.parallelBlock(ctx, snapshot, "recovery limit exhausted")
		}
		taskState.RecoveryCount = decision.NextCount
		if decision.Kind == recovery.ResumeSession {
			taskState.Stage = "resume"
		} else {
			taskState.Stage = "recovery_prompt"
		}
		taskState.Prompt.RequestID = parallelPromptRequestID(snapshot.RunID, id, taskState.RecoveryCount)
		taskState.LastProgressAt = o.now()
		snapshot.Tasks[id] = taskState
		snapshot.PendingAction = ""
		snapshot.PendingTaskID = ""
		snapshot.Summary = message
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "recovery_requested", Phase: snapshot.Phase, Message: message, Data: map[string]any{"taskId": id, "count": taskState.RecoveryCount}})
	default:
		return o.parallelBlock(ctx, snapshot, "unknown recovery decision")
	}
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
			action := parallelTaskAction("parallel_merge_task_", id)
			if err := o.parallelPrepareTask(ctx, snapshot, id, action, "immutable Task commit 병합", map[string]any{"taskId": id, "commitSha": taskState.Agent.CommitSHA}); err != nil {
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
		action := parallelTaskAction("parallel_merge_task_", id)
		if err := o.parallelPrepareTask(ctx, snapshot, id, action, "immutable Task commit 병합", map[string]any{"taskId": id, "commitSha": taskState.Agent.CommitSHA}); err != nil {
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
			return o.parallelTransition(ctx, snapshot, contract.PhaseBuilding, "incremental integration 완료; 다음 ready Task dispatch")
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
		snapshot.Reviewer.Name = name
		snapshot.ReviewerExpectedPath = snapshot.IntegrationPath
		snapshot.ReviewerExpectedBranch = snapshot.Integration.Branch
		snapshot.ReviewerExpectedLabel = "threaddock-review-" + string(snapshot.RunID)
		opener, ok := o.deps.Herdr.(WorktreeOpener)
		if !ok {
			return errors.New("parallel Reviewer Worktree opener is required")
		}
		snapshot.ReviewerWorktree = state.WorktreeState{Path: snapshot.ReviewerExpectedPath, Branch: snapshot.ReviewerExpectedBranch}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_open_reviewer_worktree", "Reviewer Worktree 연결", map[string]any{"path": snapshot.ReviewerExpectedPath, "branch": snapshot.ReviewerExpectedBranch, "label": snapshot.ReviewerExpectedLabel}); err != nil {
			return err
		}
		opened, err := opener.OpenWorktree(ctx, herdr.OpenWorktreeRequest{Cwd: snapshot.RepositoryPath, Path: snapshot.IntegrationPath, Label: "threaddock-review-" + string(snapshot.RunID)})
		if err != nil || strings.TrimSpace(opened.Path) == "" || strings.TrimSpace(opened.WorkspaceID) == "" || strings.TrimSpace(opened.PaneID) == "" || filepath.Clean(opened.Path) != filepath.Clean(snapshot.IntegrationPath) {
			return errors.New("Herdr returned incomplete parallel Reviewer Worktree identity")
		}
		snapshot.ReviewerWorktree = state.WorktreeState{Path: opened.Path, WorkspaceID: opened.WorkspaceID, PaneID: opened.PaneID, Branch: snapshot.Integration.Branch}
		return o.parallelFinish(ctx, snapshot, "Reviewer Worktree 준비 완료")
	case 1:
		if err := o.parallelPrepare(ctx, snapshot, "parallel_start_reviewer", "Reviewer Agent 시작", map[string]any{"agent": snapshot.Reviewer.Name}); err != nil {
			return err
		}
		if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: snapshot.Reviewer.Name, PaneID: snapshot.ReviewerWorktree.PaneID}); err != nil {
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
		if err := o.deps.Herdr.Prompt(ctx, name, parallelReviewerPacket(runtime.contract, snapshot)); err != nil {
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
		return o.applyParallelReviewEvidence(ctx, snapshot, runtime, evidence)
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

func parallelReviewerPacket(c contract.TaskContract, snapshot *state.RunSnapshot) string {
	var b strings.Builder
	b.WriteString("Reviewer acceptance criteria:\n")
	for _, criterion := range c.Parent.AcceptanceCriteria {
		fmt.Fprintf(&b, "- %s\n", criterion)
	}
	b.WriteString("\nTask criteria and evidence:\n")
	for _, id := range snapshot.TaskOrder {
		task, ok := parallelTask(c, id)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\nTask %s (%s):\n", task.ID, task.Role)
		for _, criterion := range task.AcceptanceCriteria {
			fmt.Fprintf(&b, "- %s\n", criterion)
		}
		for _, command := range task.Verification {
			fmt.Fprintf(&b, "Required verification: %s\n", command)
		}
		if evidence, exists := snapshot.Tasks[id]; exists {
			fmt.Fprintf(&b, "Commit SHA: %s\nChanged files: %s\nPatch:\n%s\n", evidence.Agent.CommitSHA, strings.Join(evidence.Agent.ChangedFiles, ", "), redactPatch(evidence.Agent.Patch))
			for _, check := range evidence.Agent.VerificationEvidence {
				fmt.Fprintf(&b, "Verification: %s / %s / %s\n", check.Command, check.Outcome, check.Duration)
			}
		}
	}
	fmt.Fprintf(&b, "\nFinal integration SHA: %s\nFinal integration verification:\n", snapshot.IntegrationSHA)
	for _, check := range snapshot.IntegrationVerification {
		fmt.Fprintf(&b, "- %s / %s / %s\n", check.Command, check.Outcome, check.Duration)
	}
	b.WriteString("\nYour final response must contain exactly one strict Review Evidence JSON object between these markers.\n")
	b.WriteString(herdr.THREADDOCK_REVIEW_BEGIN + "\n")
	b.WriteString(herdr.ReviewEvidenceSchemaExample + "\n")
	b.WriteString(herdr.THREADDOCK_REVIEW_END + "\n")
	fmt.Fprintf(&b, "The JSON requestId must be %s. decision must be accept or block; blocking decisions require blockingFindings and riskCategories must be an array.\n", strconv.Quote(snapshot.ReviewerPrompt.RequestID))
	b.WriteString("Use requestId=" + snapshot.ReviewerPrompt.RequestID + ".\n")
	return b.String()
}

func firstParallelTaskID(snapshot *state.RunSnapshot) string {
	for _, id := range snapshot.TaskOrder {
		if snapshot.Tasks[id].State == "completed" || snapshot.Tasks[id].State == "running" {
			return id
		}
	}
	return ""
}

func parallelRepairTaskID(snapshot *state.RunSnapshot, c contract.TaskContract, findings []review.Finding) string {
	for _, finding := range findings {
		for _, id := range snapshot.TaskOrder {
			task, ok := parallelTask(c, id)
			if !ok || snapshot.Tasks[id].State != "completed" {
				continue
			}
			for _, path := range finding.Paths {
				for _, allowed := range task.AllowedPaths {
					if allowedPath(path, []string{allowed}) {
						return id
					}
				}
			}
		}
	}
	return firstParallelTaskID(snapshot)
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
	if snapshot.PullRequest > 0 {
		finder, found := o.deps.GitHub.(github.PullRequestFinder)
		if !found {
			return o.parallelBlock(ctx, snapshot, "existing PR reconciliation port is unavailable")
		}
		pr, exists, err := finder.FindOpenPullRequest(ctx, repo, snapshot.Integration.Branch, runtime.contract.Repository.DefaultBranch)
		if err != nil || !exists || pr.Number != snapshot.PullRequest || pr.Head != snapshot.Integration.Branch || pr.Base != runtime.contract.Repository.DefaultBranch || pr.HeadSHA != snapshot.IntegrationSHA {
			return o.parallelBlock(ctx, snapshot, "existing PR does not match repaired integration SHA")
		}
		snapshot.PullRequestURL, snapshot.PullRequestHeadSHA, snapshot.PullRequestDraft = pr.HTMLURL, pr.HeadSHA, pr.Draft
		if err := o.parallelFinish(ctx, snapshot, "existing Draft PR reconciled after repair"); err != nil {
			return err
		}
		return o.parallelTransition(ctx, snapshot, contract.PhaseCI, "repaired integration latest-main sequence started")
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
		locator, ok := o.deps.Worktree.(RemoteHeadLocator)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "latest remote main locator is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_read_latest_main", "latest main SHA 확인", nil); err != nil {
			return err
		}
		sha, err := locator.FetchRemoteHead(ctx, snapshot.RepositoryPath, o.remote(), runtime.contract.Repository.DefaultBranch)
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
		if err := o.parallelPrepare(ctx, snapshot, "parallel_mark_pr_ready", "Draft PR ready 상태 확인", nil); err != nil {
			return err
		}
		if !snapshot.PullRequestDraft {
			return o.parallelFinish(ctx, snapshot, "PR already ready; ready mutation skipped")
		}
		readier, ok := o.deps.GitHub.(github.PullRequestReadier)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "pull request ready port is unavailable")
		}
		snapshot.Summary = "Draft PR를 ready로 전환"
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
		if pr.Mergeable == nil {
			snapshot.MergeabilityReads++
			if snapshot.MergeabilityReads >= 3 {
				return o.parallelBlock(ctx, snapshot, "PR mergeability remained unknown after bounded rereads")
			}
			snapshot.ActionCursor = 5
			return o.parallelFinish(ctx, snapshot, "PR mergeability unknown; bounded reread scheduled")
		}
		snapshot.MergeabilityReads = 0
		snapshot.MergeabilityKnown = true
		snapshot.Mergeable = *pr.Mergeable
		snapshot.CIState = map[bool]string{true: "mergeable", false: "conflict"}[*pr.Mergeable]
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
			snapshot.PendingTaskID = ""
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
				snapshot.PendingTaskID = ""
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
		locator, ok := o.deps.Worktree.(RemoteHeadLocator)
		if !ok {
			return o.parallelBlock(ctx, snapshot, "latest remote main re-read port is unavailable")
		}
		if err := o.parallelPrepare(ctx, snapshot, "parallel_reread_latest_main", "Merge Gate 직전 latest main 재확인", nil); err != nil {
			return err
		}
		sha, err := locator.FetchRemoteHead(ctx, snapshot.RepositoryPath, o.remote(), runtime.contract.Repository.DefaultBranch)
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

func (o *Orchestrator) remote() string {
	if strings.TrimSpace(o.deps.Remote) == "" {
		return "origin"
	}
	return o.deps.Remote
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
	repairPath := "src"
	taskID := firstParallelTaskID(snapshot)
	if taskID != "" {
		if len(snapshot.Tasks[taskID].Agent.ChangedFiles) > 0 {
			repairPath = snapshot.Tasks[taskID].Agent.ChangedFiles[0]
		} else if task, ok := parallelTask(runtime.contract, taskID); ok && len(task.AllowedPaths) > 0 {
			repairPath = strings.TrimSuffix(task.AllowedPaths[0], "/**")
		}
	}
	findings := []review.Finding{{ID: "ci", Summary: message, Paths: []string{repairPath}}}
	taskID = parallelRepairTaskID(snapshot, runtime.contract, findings)
	decision := review.Decide(*snapshot, review.ReviewResult{Source: "ci", Blocking: true, Findings: findings})
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
		taskState.RequiresFreshCommit = true
		taskState.PreviousCommitSHA = taskState.Agent.CommitSHA
		taskState.PreviousFingerprint = taskState.ProgressFingerprint
		taskState.Prompt = state.PromptReceipt{RequestID: parallelPromptRequestID(snapshot.RunID, taskID, taskState.RepairCount)}
		taskState.Agent.VerificationEvidence = nil
		taskState.Agent.ChangedFiles = nil
		taskState.Agent.Patch = ""
		snapshot.ReviewFindings = []state.ReviewFinding{{ID: "ci", Summary: message, Paths: []string{repairPath}}}
		snapshot.Tasks[taskID] = taskState
		snapshot.RepairBaseSHA = snapshot.IntegrationSHA
		snapshot.IntegrationSHA, snapshot.IntegrationVerification = "", nil
		snapshot.FinalSHA, snapshot.FinalChecksSHA, snapshot.FinalChecks = "", "", nil
		snapshot.PendingAction = ""
		snapshot.PendingTaskID = ""
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
	if !snapshot.MergePreflightReady {
		if err := o.parallelPrepare(ctx, snapshot, "parallel_merge_preflight", "main merge preflight PR evidence", map[string]any{"pullRequest": snapshot.PullRequest, "sha": snapshot.FinalSHA}); err != nil {
			return err
		}
		currentPR, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		if err != nil || currentPR.Number != snapshot.PullRequest || currentPR.State != "open" || currentPR.HeadSHA != snapshot.FinalSHA || currentPR.Base != runtime.contract.Repository.DefaultBranch || currentPR.Mergeable == nil || !*currentPR.Mergeable {
			return o.parallelBlock(ctx, snapshot, "merge preflight PR evidence is stale or unmergeable")
		}
		snapshot.ExpectedMergeHeadSHA = snapshot.FinalSHA
		snapshot.MergePreflightReady = true
		return o.parallelFinish(ctx, snapshot, "main merge preflight ready")
	}
	if err := o.parallelPrepare(ctx, snapshot, "parallel_merge_main", "main에 PR merge", map[string]any{"pullRequest": snapshot.PullRequest, "sha": snapshot.ExpectedMergeHeadSHA}); err != nil {
		return err
	}
	result, err := merger.MergePullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest, snapshot.ExpectedMergeHeadSHA, "merge")
	if err != nil {
		return err
	}
	if !result.Merged || !validCommitSHA(result.SHA) {
		return errors.New("GitHub merge response is incomplete")
	}
	snapshot.MergeSHA = result.SHA
	snapshot.MergePreflightReady = false
	snapshot.PullRequestMerged = true
	return o.finishParallelMainMerge(ctx, snapshot)
}

// finishParallelMainMerge makes the Project Done observation part of the
// durable merge result. This is shared by the normal merge response and the
// pending-merge reconciliation path, so a lost merge response cannot bypass
// enabled Project automation or mark the run complete too early.
func (o *Orchestrator) finishParallelMainMerge(ctx context.Context, snapshot *state.RunSnapshot) error {
	if snapshot.ProjectAutomationEnabled {
		// Keep Done ProjectV2 work in later Advances; this Advance has already
		// performed the single main merge side effect.
		snapshot.PendingAction = "parallel_project_observe_done"
		snapshot.PendingTaskID = ""
		snapshot.Phase = contract.PhaseMerging
		snapshot.Summary = "main merge 완료; Project Done 상태 대기 중"
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		if err := o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: "main merge 완료"}); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project Done 상태 관찰", Data: map[string]any{"action": snapshot.PendingAction}})
	}
	snapshot.Phase = contract.PhaseCompleted
	snapshot.Summary = "병렬 실행과 감독형 자동 병합 완료"
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	if err := o.append(ctx, snapshot.RunID, state.Event{Type: "project_automation_skipped", Phase: snapshot.Phase, Message: "Project automation disabled; placeholder IDs are not evidence", Data: map[string]any{"status": "Done"}}); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "completed", Phase: contract.PhaseCompleted, Message: snapshot.Summary})
}

func (o *Orchestrator) reconcileProjectObservation(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, status string) error {
	reader, ok := o.deps.GitHub.(ProjectStatusReader)
	if !ok || snapshot.Registration.NodeID == "" {
		return o.pendingUncertain(ctx, snapshot)
	}
	observed, err := reader.ReadProjectStatus(ctx, o.deps.Project, snapshot.Registration.NodeID)
	if err != nil {
		return o.parallelBlock(ctx, snapshot, "Project status is uncertain after restart")
	}
	if projectItemPresent(observed) && observed.ItemID != "" {
		snapshot.ProjectItemID = observed.ItemID
	}
	if !projectItemPresent(observed) {
		snapshot.PendingAction = "parallel_project_add_" + strings.ToLower(strings.ReplaceAll(status, " ", "_"))
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project item add", Data: map[string]any{"action": snapshot.PendingAction}})
	}
	if strings.TrimSpace(observed.ItemID) == "" {
		return o.pendingUncertain(ctx, snapshot)
	}
	if projectStatusPresent(observed) && observed.Status == status {
		snapshot.ProjectStatus = status
		if status == "Done" && snapshot.PullRequestMerged {
			snapshot.Phase = contract.PhaseCompleted
			if err := o.parallelFinishPreserveCursor(ctx, snapshot, "Project status reconciled"); err != nil {
				return err
			}
			return o.append(ctx, snapshot.RunID, state.Event{Type: "completed", Phase: contract.PhaseCompleted, Message: "병렬 실행과 감독형 자동 병합 완료"})
		}
		return o.parallelFinishPreserveCursor(ctx, snapshot, "Project status reconciled")
	}
	snapshot.PendingAction = "parallel_project_update_" + strings.ToLower(strings.ReplaceAll(status, " ", "_"))
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project status update", Data: map[string]any{"action": snapshot.PendingAction, "itemId": snapshot.ProjectItemID}})
}

func projectItemPresent(observed github.ProjectStatus) bool {
	// Found/ItemID preserve compatibility with older reader implementations
	// that predate the explicit presence bits.
	return observed.ItemPresent || observed.ItemFound || observed.Found || strings.TrimSpace(observed.ItemID) != ""
}

func projectStatusPresent(observed github.ProjectStatus) bool {
	return observed.StatusPresent || observed.StatusFound || observed.Found || strings.TrimSpace(observed.Status) != ""
}

func projectObserveAction(status string) string {
	return "parallel_project_observe_" + strings.ToLower(strings.ReplaceAll(status, " ", "_"))
}

// prepareProjectMutationObservation records the observation that must follow
// a Project mutation before the mutation is attempted. If the process dies
// after the provider accepts the write but before its response is persisted,
// restart therefore performs a read first and cannot blindly repeat it.
func (o *Orchestrator) prepareProjectMutationObservation(ctx context.Context, snapshot *state.RunSnapshot, status string) error {
	action := projectObserveAction(status)
	snapshot.PendingAction = action
	snapshot.PendingTaskID = ""
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project status observe after mutation", Data: map[string]any{"action": action, "status": status}})
}

func projectStatusFromAction(action, prefix string) string {
	value := strings.TrimPrefix(action, prefix)
	switch value {
	case "ready":
		return "Ready"
	case "in_progress":
		return "In Progress"
	case "review":
		return "Review"
	case "done":
		return "Done"
	default:
		return strings.ReplaceAll(value, "_", " ")
	}
}

func (o *Orchestrator) applyParallelReviewEvidence(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, evidence herdr.ReviewEvidence) error {
	findings := make([]state.ReviewFinding, 0, len(evidence.BlockingFindings))
	result := review.ReviewResult{Source: "reviewer", Blocking: evidence.Decision == "block", RiskCategories: append([]string(nil), evidence.RiskCategories...)}
	for _, finding := range evidence.BlockingFindings {
		findings = append(findings, state.ReviewFinding{ID: finding.ID, Summary: finding.Summary, Paths: append([]string(nil), finding.Paths...)})
		result.Findings = append(result.Findings, review.Finding{ID: finding.ID, Summary: finding.Summary, Paths: append([]string(nil), finding.Paths...)})
	}
	decision := review.Decide(*snapshot, result)
	snapshot.ReviewFindings = findings
	snapshot.ReviewRiskCategories = append([]string(nil), evidence.RiskCategories...)
	switch decision.Kind {
	case review.Accept:
		snapshot.ReviewDecision = "accept"
		return o.parallelFinish(ctx, snapshot, "Reviewer acceptance 확인")
	case review.Repair:
		snapshot.RepairCount = decision.NextCount
		taskID := parallelRepairTaskID(snapshot, runtime.contract, result.Findings)
		if taskID == "" {
			return o.parallelBlock(ctx, snapshot, "repair 대상 Task가 없습니다")
		}
		taskState := snapshot.Tasks[taskID]
		taskState.State, taskState.Stage = "running", "repair_prompt"
		taskState.RepairCount++
		taskState.RequiresFreshCommit = true
		taskState.PreviousCommitSHA = taskState.Agent.CommitSHA
		taskState.PreviousFingerprint = taskState.ProgressFingerprint
		taskState.Agent.RequestID = ""
		taskState.Agent.VerificationEvidence, taskState.Agent.ChangedFiles, taskState.Agent.Patch = nil, nil, ""
		taskState.Prompt = state.PromptReceipt{RequestID: parallelPromptRequestID(snapshot.RunID, taskID, taskState.RepairCount)}
		snapshot.Tasks[taskID] = taskState
		snapshot.CurrentTask = taskID
		snapshot.RepairBaseSHA = snapshot.IntegrationSHA
		snapshot.IntegrationSHA, snapshot.IntegrationVerification = "", nil
		snapshot.FinalSHA, snapshot.FinalChecksSHA, snapshot.FinalChecks = "", "", nil
		snapshot.PendingAction, snapshot.PendingTaskID = "", ""
		return o.parallelTransition(ctx, snapshot, contract.PhaseBuilding, "Reviewer findings에 대한 bounded repair 시작")
	default:
		return o.parallelBlock(ctx, snapshot, "Reviewer repair budget exhausted")
	}
}

// reconcileParallelPending performs only read/reconcile operations for an
// intent that survived a crash. A missing observation clears the intent so a
// later Advance may retry; it never blindly repeats an ambiguous write.
func (o *Orchestrator) reconcileParallelPending(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	action := snapshot.PendingAction
	if strings.HasPrefix(action, "parallel_project_observe_") {
		status := projectStatusFromAction(action, "parallel_project_observe_")
		return o.reconcileProjectObservation(ctx, snapshot, runtime, status)
	}
	if strings.HasPrefix(action, "parallel_project_add_") {
		status := projectStatusFromAction(action, "parallel_project_add_")
		adder, _ := projectMutationPorts(o.deps.GitHub)
		if adder == nil || snapshot.Registration.NodeID == "" {
			return o.pendingUncertain(ctx, snapshot)
		}
		if err := o.prepareProjectMutationObservation(ctx, snapshot, status); err != nil {
			return err
		}
		itemID, err := adder.AddProjectItem(ctx, o.deps.Project, snapshot.Registration.NodeID)
		if err != nil {
			// The observation intent was durably recorded before the call; leave
			// it in place even when the provider response is lost.
			return err
		}
		if itemID == "" {
			return errors.New("Project item add response is incomplete")
		}
		snapshot.ProjectItemID = itemID
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project status observe after add", Data: map[string]any{"action": snapshot.PendingAction, "itemId": itemID}})
	}
	if strings.HasPrefix(action, "parallel_project_update_") {
		status := projectStatusFromAction(action, "parallel_project_update_")
		_, updater := projectMutationPorts(o.deps.GitHub)
		if updater == nil || snapshot.ProjectItemID == "" {
			return o.pendingUncertain(ctx, snapshot)
		}
		if err := o.prepareProjectMutationObservation(ctx, snapshot, status); err != nil {
			return err
		}
		if err := updater.UpdateProjectStatus(ctx, o.deps.Project, snapshot.ProjectItemID, status); err != nil {
			return err
		}
		snapshot.ProjectStatus = status
		snapshot.UpdatedAt = o.now()
		if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
			return err
		}
		return o.append(ctx, snapshot.RunID, state.Event{Type: "intent", Phase: snapshot.Phase, Message: "Project status observe after update", Data: map[string]any{"action": snapshot.PendingAction, "itemId": snapshot.ProjectItemID}})
	}
	if strings.HasPrefix(action, "parallel_project_status_") {
		// Older snapshots used one compound status action. Convert it to the
		// new observe action and stop here; the next Advance performs the read,
		// and a later Advance performs at most one add/update mutation.
		status := projectStatusFromAction(action, "parallel_project_status_")
		if status == "" {
			status = parallelProjectStatus(snapshot.Phase)
		}
		if !snapshot.ProjectAutomationEnabled {
			if status == "Done" && snapshot.PullRequestMerged {
				return o.finishParallelMainMerge(ctx, snapshot)
			}
			snapshot.ProjectStatus = status
			snapshot.PendingAction = ""
			snapshot.PendingTaskID = ""
			snapshot.UpdatedAt = o.now()
			if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
				return err
			}
			return o.append(ctx, snapshot.RunID, state.Event{Type: "project_automation_skipped", Phase: snapshot.Phase, Message: "Project automation disabled; placeholder IDs are not evidence", Data: map[string]any{"status": status}})
		}
		return o.prepareProjectMutationObservation(ctx, snapshot, status)
	}
	if strings.HasPrefix(action, "parallel_create_worktree_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_create_worktree_")
		return o.reconcileParallelTaskWorktree(ctx, snapshot, runtime, id)
	}
	if strings.HasPrefix(action, "parallel_start_agent_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_start_agent_")
		return o.reconcileParallelTaskAgent(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_resume_agent_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_resume_agent_")
		return o.reconcileParallelTaskAgent(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_baseline_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_baseline_")
		if id == "reviewer" {
			return o.reconcileParallelReviewerBaseline(ctx, snapshot)
		}
		return o.reconcileParallelTaskBaseline(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_prompt_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_prompt_")
		if id == "reviewer" {
			return o.reconcileParallelPromptReceipt(ctx, snapshot, snapshot.Reviewer.Name, snapshot.ReviewerPrompt, func() { snapshot.Reviewer.Verification = []string{"review schema sent"} })
		}
		return o.reconcileParallelTaskPrompt(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_recovery_prompt_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_recovery_prompt_")
		return o.reconcileParallelTaskPrompt(ctx, snapshot, id)
	}
	if strings.HasPrefix(action, "parallel_collect_evidence_") {
		return o.reconcileParallelTaskEvidence(ctx, snapshot, runtime, pendingParallelTaskID(snapshot, action, "parallel_collect_evidence_"))
	}
	if strings.HasPrefix(action, "parallel_fingerprint_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_fingerprint_")
		taskState, exists := snapshot.Tasks[id]
		if !exists {
			return o.pendingUncertain(ctx, snapshot)
		}
		managedFingerprint := ""
		if reader, ok := o.deps.Worktree.(FingerprintReader); ok {
			fingerprint, err := reader.Fingerprint(ctx, snapshot.Integration.Path)
			if err != nil {
				return o.pendingUncertain(ctx, snapshot)
			}
			managedFingerprint = strings.TrimSpace(fingerprint)
		}
		taskState.ProgressFingerprint = RecoveryFingerprint(taskState.Agent.CommitSHA, managedFingerprint, completedTaskIDs(snapshot), taskState.Agent.VerificationEvidence)
		taskState.Stage = "inspect"
		snapshot.Tasks[id] = taskState
		return o.parallelFinish(ctx, snapshot, "managed Git fingerprint intent reconciled")
	}
	if strings.HasPrefix(action, "parallel_inspect_commit_") {
		return o.reconcileParallelTaskInspection(ctx, snapshot, runtime, pendingParallelTaskID(snapshot, action, "parallel_inspect_commit_"))
	}
	if strings.HasPrefix(action, "parallel_merge_task_") {
		id := pendingParallelTaskID(snapshot, action, "parallel_merge_task_")
		presence, ok := o.deps.Worktree.(CommitPresenceReader)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		taskState, exists := snapshot.Tasks[id]
		if !exists {
			return o.pendingUncertain(ctx, snapshot)
		}
		present, err := presence.IsAncestor(ctx, snapshot.Integration.Path, taskState.Agent.CommitSHA)
		if err != nil {
			return o.pendingUncertain(ctx, snapshot)
		}
		if !present {
			return o.clearParallelPending(ctx, snapshot, "Task commit is not present; immutable merge may be retried")
		}
		taskState.State, taskState.Stage = "completed", "merged"
		snapshot.Tasks[id] = taskState
		return o.parallelFinish(ctx, snapshot, "Task immutable merge intent reconciled")
	}
	switch action {
	case "parallel_wave_end_verification":
		return o.reconcileParallelChecks(ctx, snapshot, runtime.contract.Verification, true)
	case "parallel_full_suite":
		return o.reconcileParallelChecks(ctx, snapshot, runtime.contract.Verification, false)
	case "parallel_read_final_sha":
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(sha) {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.FinalSHA, snapshot.FinalChecksSHA = strings.TrimSpace(sha), ""
		return o.parallelFinish(ctx, snapshot, "final SHA intent reconciled")
	case "parallel_reread_latest_main":
		locator, ok := o.deps.Worktree.(RemoteHeadLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		sha, err := locator.FetchRemoteHead(ctx, snapshot.RepositoryPath, o.remote(), runtime.contract.Repository.DefaultBranch)
		if err != nil || !validCommitSHA(sha) {
			return o.pendingUncertain(ctx, snapshot)
		}
		if strings.TrimSpace(sha) != snapshot.MainSHA {
			snapshot.MainSHA, snapshot.FinalSHA, snapshot.FinalChecksSHA, snapshot.FinalChecks, snapshot.ActionCursor = strings.TrimSpace(sha), "", "", nil, 0
		}
		return o.parallelFinish(ctx, snapshot, "latest remote main intent reconciled")
	case "parallel_open_reviewer_worktree":
		locator, ok := o.deps.Herdr.(WorktreeLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		path, label := snapshot.ReviewerExpectedPath, snapshot.ReviewerExpectedLabel
		if path == "" {
			path = snapshot.IntegrationPath
		}
		if label == "" {
			label = "threaddock-review-" + string(snapshot.RunID)
		}
		found, exists, err := locator.FindWorktree(ctx, snapshot.RepositoryPath, path, label)
		if err != nil || !exists || !canonicalPathEqual(found.Path, path) || found.WorkspaceID == "" || found.PaneID == "" {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.ReviewerWorktree = state.WorktreeState{Path: found.Path, WorkspaceID: found.WorkspaceID, PaneID: found.PaneID, Branch: snapshot.Integration.Branch}
		return o.parallelFinish(ctx, snapshot, "Reviewer Worktree intent reconciled")
	case "parallel_start_reviewer":
		locator, ok := o.deps.Herdr.(AgentLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		info, err := locator.GetInfo(ctx, snapshot.Reviewer.Name)
		if err != nil || !matchesParallelAgentIdentity(info, snapshot.Reviewer.Name, snapshot.ReviewerWorktree) {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.Reviewer.SessionID = info.SessionID
		return o.parallelFinish(ctx, snapshot, "Reviewer Agent intent reconciled")
	case "parallel_prompt_reviewer":
		return o.reconcileParallelPromptReceipt(ctx, snapshot, snapshot.Reviewer.Name, snapshot.ReviewerPrompt, func() { snapshot.Reviewer.Verification = []string{"review schema sent"} })
	case "parallel_collect_review":
		reader, ok := o.deps.Herdr.(ReviewEvidenceReader)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		evidence, err := reader.ReadReviewEvidence(ctx, snapshot.Reviewer.Name, snapshot.ReviewerPrompt.RequestID)
		if err != nil {
			return o.pendingUncertain(ctx, snapshot)
		}
		return o.applyParallelReviewEvidence(ctx, snapshot, runtime, evidence)
	case "parallel_read_latest_main":
		locator, ok := o.deps.Worktree.(RemoteHeadLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		sha, err := locator.FetchRemoteHead(ctx, snapshot.RepositoryPath, o.remote(), runtime.contract.Repository.DefaultBranch)
		if err != nil || !validCommitSHA(sha) {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.MainSHA = strings.TrimSpace(sha)
		return o.parallelFinish(ctx, snapshot, "latest remote main intent reconciled")
	case "parallel_merge_latest_main":
		presence, ok := o.deps.Worktree.(CommitPresenceReader)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		present, err := presence.IsAncestor(ctx, snapshot.Integration.Path, snapshot.MainSHA)
		if err != nil {
			return o.pendingUncertain(ctx, snapshot)
		}
		if !present {
			return o.clearParallelPending(ctx, snapshot, "latest main SHA is not present; merge may be retried")
		}
		return o.parallelFinish(ctx, snapshot, "latest-main merge intent reconciled")
	case "parallel_locate_integration_sha":
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		sha, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || !validCommitSHA(sha) {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.IntegrationSHA = strings.TrimSpace(sha)
		return o.parallelFinish(ctx, snapshot, "integration SHA intent reconciled")
	case "parallel_push_integration", "parallel_push_final_sha":
		locator, ok := o.deps.Worktree.(RemoteHeadLocator)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		expected := snapshot.IntegrationSHA
		if action == "parallel_push_final_sha" {
			expected = snapshot.FinalSHA
		}
		if !validCommitSHA(expected) {
			return o.pendingUncertain(ctx, snapshot)
		}
		remoteSHA, err := locator.FetchRemoteHead(ctx, snapshot.RepositoryPath, o.remote(), snapshot.Integration.Branch)
		if err != nil {
			return o.pendingUncertain(ctx, snapshot)
		}
		if strings.TrimSpace(remoteSHA) != expected {
			return o.clearParallelPending(ctx, snapshot, "remote branch does not contain expected SHA; push may be retried safely")
		}
		return o.parallelFinish(ctx, snapshot, "branch push intent reconciled")
	case "parallel_create_draft_pr":
		finder, ok := o.deps.GitHub.(github.PullRequestFinder)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		pr, found, err := finder.FindOpenPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.Integration.Branch, runtime.contract.Repository.DefaultBranch)
		if err != nil {
			return o.pendingUncertain(ctx, snapshot)
		}
		if !found {
			return o.clearParallelPending(ctx, snapshot, "Draft PR not found; next Advance may create it")
		}
		if pr.Number <= 0 || pr.Head != snapshot.Integration.Branch || pr.Base != runtime.contract.Repository.DefaultBranch || pr.HeadSHA != snapshot.IntegrationSHA {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.PullRequest, snapshot.PullRequestURL, snapshot.PullRequestHeadSHA, snapshot.PullRequestDraft = pr.Number, pr.HTMLURL, pr.HeadSHA, pr.Draft
		return o.parallelFinish(ctx, snapshot, "Draft PR intent reconciled")
	case "parallel_read_checks":
		reader, ok := o.deps.GitHub.(github.CheckReader)
		if !ok {
			return o.pendingUncertain(ctx, snapshot)
		}
		checks, err := reader.GetChecks(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.FinalSHA)
		if err != nil || !parallelGitHubChecksValid(checks) {
			return o.pendingUncertain(ctx, snapshot)
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
		if err != nil || pr.Number != snapshot.PullRequest || pr.HeadSHA != snapshot.FinalSHA {
			return o.pendingUncertain(ctx, snapshot)
		}
		if pr.Draft {
			return o.clearParallelPending(ctx, snapshot, "PR remains draft; ready mutation may be retried")
		}
		snapshot.PullRequestDraft = false
		return o.parallelFinish(ctx, snapshot, "PR ready intent reconciled")
	case "parallel_read_pr":
		pr, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		if err != nil || pr.Number != snapshot.PullRequest {
			return o.pendingUncertain(ctx, snapshot)
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
		} else {
			snapshot.MergeabilityReads++
			if snapshot.MergeabilityReads >= 3 {
				return o.parallelBlock(ctx, snapshot, "PR mergeability remained unknown after restart")
			}
			snapshot.ActionCursor = 5
		}
		return o.parallelFinish(ctx, snapshot, "PR read intent reconciled")
	case "parallel_merge_main":
		pr, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		expectedHead := snapshot.ExpectedMergeHeadSHA
		if expectedHead == "" {
			expectedHead = snapshot.FinalSHA
		}
		if err != nil || pr.Number != snapshot.PullRequest || pr.HeadSHA != expectedHead || pr.Base != runtime.contract.Repository.DefaultBranch {
			return o.pendingUncertain(ctx, snapshot)
		}
		if !pr.Merged {
			if pr.State == "open" {
				return o.clearParallelPending(ctx, snapshot, "main merge not observed; exact merge may be retried")
			}
			return o.pendingUncertain(ctx, snapshot)
		}
		if pr.State != "closed" || !validCommitSHA(pr.MergeCommitSHA) || snapshot.MergeSHA != "" && pr.MergeCommitSHA != snapshot.MergeSHA {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.MergeSHA = pr.MergeCommitSHA
		snapshot.PullRequestMerged = true
		return o.finishParallelMainMerge(ctx, snapshot)
	case "parallel_merge_preflight":
		pr, err := o.deps.GitHub.GetPullRequest(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, snapshot.PullRequest)
		if err != nil || pr.Number != snapshot.PullRequest || pr.State != "open" || pr.HeadSHA != snapshot.FinalSHA || pr.Base != runtime.contract.Repository.DefaultBranch || pr.Mergeable == nil || !*pr.Mergeable {
			return o.pendingUncertain(ctx, snapshot)
		}
		snapshot.ExpectedMergeHeadSHA = snapshot.FinalSHA
		snapshot.MergePreflightReady = true
		return o.parallelFinish(ctx, snapshot, "main merge preflight intent reconciled")
	default:
		return o.pendingUncertain(ctx, snapshot)
	}
}

func (o *Orchestrator) clearParallelPending(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.PendingTaskID = ""
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "reconcile_not_executed", Phase: snapshot.Phase, Message: message})
}

func (o *Orchestrator) pendingUncertain(ctx context.Context, snapshot *state.RunSnapshot) error {
	return o.parallelBlock(ctx, snapshot, "pending action reconciliation is uncertain: "+snapshot.PendingAction)
}

func (o *Orchestrator) reconcileParallelTaskWorktree(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string) error {
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return o.pendingUncertain(ctx, snapshot)
	}
	if taskState.ExpectedBaseCommit != "" && taskState.ExpectedBaseCommit != runtime.contract.BaseCommit {
		return o.pendingUncertain(ctx, snapshot)
	}
	expectedBranch := strings.TrimSpace(taskState.ExpectedBranch)
	if expectedBranch == "" {
		expectedBranch = strings.TrimSpace(taskState.Worktree.Branch)
	}
	if expectedBranch == "" {
		if task, ok := parallelTask(runtime.contract, id); ok {
			expectedBranch = strings.TrimSpace(task.Branch)
		}
	}
	label := taskState.ExpectedLabel
	if label == "" {
		label = "threaddock-" + string(snapshot.RunID) + "-" + id
	}
	if expectedBranch == "" {
		return o.pendingUncertain(ctx, snapshot)
	}
	if taskState.ExpectedBranch != "" && taskState.Worktree.Branch != "" && taskState.Worktree.Branch != taskState.ExpectedBranch {
		return o.pendingUncertain(ctx, snapshot)
	}
	var found herdr.Worktree
	var foundExists bool
	var err error
	if branchLocator, branchOK := o.deps.Herdr.(BranchWorktreeLocator); branchOK {
		found, foundExists, err = branchLocator.FindWorktreeByBranch(ctx, snapshot.RepositoryPath, expectedBranch, label)
	} else {
		locator, locatorOK := o.deps.Herdr.(WorktreeLocator)
		if !locatorOK {
			return o.pendingUncertain(ctx, snapshot)
		}
		// The parallel create intent is keyed by branch and label. A path is
		// not available in the pre-effect snapshot after a crash, so pass the
		// expected branch to the legacy locator and let the Herdr adapter use
		// its branch-aware lookup. Persisted paths are only an exact-match
		// constraint after a candidate has been found.
		found, foundExists, err = locator.FindWorktree(ctx, snapshot.RepositoryPath, expectedBranch, label)
	}
	if err != nil || !foundExists {
		return o.clearParallelPending(ctx, snapshot, "Builder Worktree not found; next Advance may create it")
	}
	if strings.TrimSpace(found.Path) == "" || found.WorkspaceID == "" || found.PaneID == "" {
		return o.pendingUncertain(ctx, snapshot)
	}
	expectedPath := strings.TrimSpace(taskState.Worktree.Path)
	if expectedPath == "" {
		expectedPath = strings.TrimSpace(taskState.ExpectedPath)
	}
	if expectedPath != "" && found.Path != expectedPath {
		return o.pendingUncertain(ctx, snapshot)
	}
	taskState.Worktree.Path = found.Path
	taskState.Worktree.WorkspaceID, taskState.Worktree.PaneID = found.WorkspaceID, found.PaneID
	taskState.Worktree.Branch = expectedBranch
	taskState.ExpectedBranch = expectedBranch
	taskState.State, taskState.Stage = "running", "start"
	snapshot.Tasks[id] = taskState
	return o.parallelFinish(ctx, snapshot, "Builder Worktree intent reconciled")
}

func (o *Orchestrator) reconcileParallelTaskAgent(ctx context.Context, snapshot *state.RunSnapshot, id string) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return o.pendingUncertain(ctx, snapshot)
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	if errors.Is(err, herdr.ErrAgentNotFound) {
		return o.clearParallelPending(ctx, snapshot, "Builder Agent not found; next Advance may start it")
	}
	if err != nil || !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return o.pendingUncertain(ctx, snapshot)
	}
	if strings.HasPrefix(snapshot.PendingAction, "parallel_resume_agent_") && info.SessionID != taskState.Agent.SessionID {
		return o.pendingUncertain(ctx, snapshot)
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
		return o.pendingUncertain(ctx, snapshot)
	}
	info, err := locator.GetInfo(ctx, snapshot.Reviewer.Name)
	if err != nil || !matchesParallelAgentIdentity(info, snapshot.Reviewer.Name, snapshot.ReviewerWorktree) {
		return o.pendingUncertain(ctx, snapshot)
	}
	snapshot.Reviewer.SessionID, snapshot.ReviewerPrompt = info.SessionID, state.PromptReceipt{RequestID: string(snapshot.RunID) + ":reviewer-prompt", BaselineSeq: info.StateChangeSeq}
	return o.parallelFinish(ctx, snapshot, "Reviewer baseline intent reconciled")
}

func (o *Orchestrator) reconcileParallelTaskBaseline(ctx context.Context, snapshot *state.RunSnapshot, id string) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return o.pendingUncertain(ctx, snapshot)
	}
	info, err := locator.GetInfo(ctx, taskState.Agent.Name)
	if err != nil || !matchesParallelAgentIdentity(info, taskState.Agent.Name, taskState.Worktree) {
		return o.pendingUncertain(ctx, snapshot)
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
		return o.pendingUncertain(ctx, snapshot)
	}
	return o.reconcileParallelPromptReceipt(ctx, snapshot, taskState.Agent.Name, taskState.Prompt, func() { taskState.Stage = "evidence"; snapshot.Tasks[id] = taskState })
}

func (o *Orchestrator) reconcileParallelPromptReceipt(ctx context.Context, snapshot *state.RunSnapshot, name string, receipt state.PromptReceipt, onObserved func()) error {
	reader, ok := o.deps.Herdr.(PromptReceiptReader)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	info, observed, err := reader.ReadPromptReceipt(ctx, name, receipt.RequestID)
	if err != nil || info.Name != name || info.StateChangeSeq <= 0 {
		return o.pendingUncertain(ctx, snapshot)
	}
	if observed {
		onObserved()
		return o.parallelFinish(ctx, snapshot, "prompt intent reconciled")
	}
	if info.StateChangeSeq == receipt.BaselineSeq {
		return o.clearParallelPending(ctx, snapshot, "prompt not observed; next Advance may resend")
	}
	return o.pendingUncertain(ctx, snapshot)
}

func (o *Orchestrator) reconcileParallelTaskEvidence(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime, id string) error {
	taskState, exists := snapshot.Tasks[id]
	if !exists {
		return o.pendingUncertain(ctx, snapshot)
	}
	reader, ok := o.deps.Herdr.(EvidenceReader)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	task, ok := parallelTask(runtime.contract, id)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	previousCommit := taskState.Agent.CommitSHA
	evidence, err := reader.ReadEvidence(ctx, taskState.Agent.Name)
	commitSHA := strings.ToLower(evidence.CommitSHA)
	if err != nil || evidence.RequestID != taskState.Prompt.RequestID || !validCommitSHA(commitSHA) || !parallelVerificationValid(evidence.Verification, task.Verification) || taskState.RequiresFreshCommit && commitSHA == taskState.PreviousCommitSHA {
		// Reconciliation already performed the evidence observation. Defer the
		// recovery policy lookup to a distinct next Advance, including for a
		// stale repair SHA, so this path never reads Agent state in the same
		// Advance as the evidence observation.
		return o.parallelScheduleRecovery(ctx, snapshot, id, taskState, "Builder structured evidence is stale or incomplete")
	}
	taskState.Agent.CommitSHA = commitSHA
	taskState.Agent.RequestID = evidence.RequestID
	taskState.Agent.VerificationEvidence = parallelEvidence(evidence.Verification)
	taskState.Stage = "fingerprint"
	taskState.RequiresFreshCommit = false
	taskState.PreviousCommitSHA = previousCommit
	taskState.RecoveryCount = 0
	taskState.PreviousFingerprint = taskState.ProgressFingerprint
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
		return o.pendingUncertain(ctx, snapshot)
	}
	task, ok := parallelTask(runtime.contract, id)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	inspector, ok := o.deps.Worktree.(WorktreeInspector)
	if !ok {
		return o.pendingUncertain(ctx, snapshot)
	}
	inspection, err := inspector.InspectCommit(ctx, taskState.Worktree.Path, runtime.contract.BaseCommit, task.Branch, taskState.Agent.CommitSHA)
	if err != nil || validateInspection(inspection, taskState.Agent.CommitSHA, task.Branch, task.AllowedPaths) != nil || containsCredential(inspection.Patch) {
		return o.pendingUncertain(ctx, snapshot)
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
		return o.pendingUncertain(ctx, snapshot)
	}
	if commands == nil {
		return o.pendingUncertain(ctx, snapshot)
	}
	checks, err := reader.RunChecks(ctx, snapshot.Integration.Path, commands)
	if err != nil || !parallelGitChecksValid(checks, commands) {
		return o.pendingUncertain(ctx, snapshot)
	}
	if wave {
		snapshot.IntegrationVerification = parallelVerificationEvidence(checks)
	} else {
		snapshot.FinalChecks = parallelVerificationEvidence(checks)
		if snapshot.FinalSHA != "" {
			snapshot.FinalChecksSHA = snapshot.FinalSHA
		}
	}
	return o.parallelFinish(ctx, snapshot, "verification intent reconciled")
}
