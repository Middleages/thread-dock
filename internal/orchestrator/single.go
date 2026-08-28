package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

var (
	ErrInvalidDependencies = errors.New("orchestrator dependencies are incomplete")
	ErrRunPaused           = errors.New("run is paused")
	ErrRunFinished         = errors.New("run is no longer active")
	ErrBuilderEvidence     = errors.New("builder commit and verification evidence are required")
)

var commitEvidencePattern = regexp.MustCompile(`(?im)(?:commit(?:_sha)?|sha)\s*[:=]\s*([a-f0-9]{40})`)
var verificationPattern = regexp.MustCompile(`(?im)^\s*(?:verification|verified)\s*:\s*(.+?)\s*$`)
var changedFilePattern = regexp.MustCompile(`(?im)^\s*(?:changed_file|changed)\s*:\s*(.+?)\s*$`)

const (
	maxGitHubAttempts = 4
	connectionProblem = "GitHub 연결 문제"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type runRuntime struct {
	contract        contract.TaskContract
	bundle          github.IssueBundle
	worktree        herdr.Worktree
	builderTask     contract.Task
	builderBranch   string
	integrationPath string
	stage           int
}

// Orchestrator advances exactly one run. Runtime metadata is deliberately
// small and complements the durable snapshot; the snapshot and event log are
// the source of truth for phase and evidence.
type Orchestrator struct {
	deps Dependencies
	mu   sync.Mutex
	seq  uint64
	runs map[contract.RunID]*runRuntime
}

func New(deps Dependencies) *Orchestrator {
	if deps.Worktree == nil {
		deps.Worktree = deps.Git
	}
	if deps.Clock == nil {
		deps.Clock = realClock{}
	}
	return &Orchestrator{deps: deps, runs: make(map[contract.RunID]*runRuntime)}
}

// Start validates the contract, commits a local registered run, then
// reconciles the durable GHES Issue bundle. The returned ID is useful for
// recovery even when GHES registration fails.
func (o *Orchestrator) Start(ctx context.Context, contractPath string) (contract.RunID, error) {
	if err := checkContext(ctx); err != nil {
		return "", err
	}
	c, err := readContract(contractPath)
	if err != nil {
		return "", err
	}
	if err := o.validate(); err != nil {
		return "", err
	}

	id := o.nextRunID()
	repositoryPath := o.deps.RepositoryPath
	if repositoryPath == "" {
		repositoryPath = filepath.Dir(contractPath)
	}
	worktreeRoot := o.deps.WorktreeRoot
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(filepath.Dir(contractPath), ".threaddock-worktrees")
	}
	integrationPath := filepath.Join(worktreeRoot, string(id))
	snapshot := state.RunSnapshot{
		ContractVersion: c.Version,
		RunID:           id,
		Phase:           contract.PhaseRegistered,
		ContractPath:    contractPath,
		RepositoryPath:  repositoryPath,
		IntegrationPath: integrationPath,
		UpdatedAt:       o.now(),
	}
	if err := o.deps.Store.Create(ctx, snapshot); err != nil {
		return "", err
	}
	runtime := &runRuntime{
		contract:        c,
		builderTask:     firstBuilder(c),
		builderBranch:   firstBuilder(c).Branch,
		integrationPath: integrationPath,
	}
	o.mu.Lock()
	o.runs[id] = runtime
	o.mu.Unlock()

	if err := o.intent(ctx, id, contract.PhaseRegistered, "Issue bundle 생성", map[string]any{"marker": marker(id)}); err != nil {
		return id, err
	}
	bundle, err := o.createIssueBundle(ctx, id, c)
	if err != nil {
		_ = o.append(ctx, id, state.Event{Type: "action_failed", Phase: contract.PhaseRegistered, Message: fmt.Sprintf("Issue bundle 생성 실패: %v", err)})
		return id, err
	}
	runtime.bundle = bundle
	snapshot.ParentIssue = bundle.Parent.Number
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return id, err
	}
	if err := o.append(ctx, id, state.Event{Type: "action_succeeded", Phase: contract.PhaseRegistered, Message: "Issue bundle 준비 완료", Data: map[string]any{"parentIssue": bundle.Parent.Number}}); err != nil {
		return id, err
	}
	return id, nil
}

// Advance performs no more than one external adapter call. Intent is
// appended before that call, and the resulting snapshot is saved afterwards.
func (o *Orchestrator) Advance(ctx context.Context, id contract.RunID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := o.validate(); err != nil {
		return err
	}
	snapshot, err := o.deps.Store.Load(ctx, id)
	if err != nil {
		return err
	}
	if snapshot.Phase == contract.PhasePaused {
		return ErrRunPaused
	}
	if snapshot.Phase == contract.PhaseCompleted || snapshot.Phase == contract.PhaseBlocked {
		return ErrRunFinished
	}
	runtime, err := o.runtimeFor(ctx, snapshot)
	if err != nil {
		return err
	}

	switch snapshot.Phase {
	case contract.PhaseRegistered:
		if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "분석 단계 전환", nil); err != nil {
			return err
		}
		return o.transition(ctx, snapshot, contract.PhaseAnalyzing, "분석 단계 시작")
	case contract.PhaseAnalyzing:
		return o.createWorktree(ctx, snapshot, runtime)
	case contract.PhaseBuilding:
		return o.advanceBuilding(ctx, snapshot, runtime)
	case contract.PhaseIntegrating:
		return o.integrate(ctx, snapshot, runtime)
	case contract.PhaseReviewing:
		return o.advanceReview(ctx, snapshot, runtime)
	default:
		return fmt.Errorf("unsupported run phase %q", snapshot.Phase)
	}
}

// Stop pauses only local orchestration. It intentionally does not remove a
// Worktree, stop an Agent, or discard evidence.
func (o *Orchestrator) Stop(ctx context.Context, id contract.RunID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := o.validateStore(); err != nil {
		return err
	}
	snapshot, err := o.deps.Store.Load(ctx, id)
	if err != nil {
		return err
	}
	if snapshot.Phase == contract.PhasePaused {
		return nil
	}
	if snapshot.Phase == contract.PhaseCompleted || snapshot.Phase == contract.PhaseBlocked {
		return ErrRunFinished
	}
	snapshot.Phase = contract.PhasePaused
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, id, state.Event{Type: "paused", Phase: contract.PhasePaused, Message: "실행을 일시 중지했습니다"})
}

func (o *Orchestrator) createWorktree(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "Builder Worktree 생성", map[string]any{"branch": runtime.builderBranch, "path": snapshot.IntegrationPath}); err != nil {
		return err
	}
	created, err := o.deps.Herdr.CreateWorktree(ctx, herdr.CreateWorktreeRequest{
		Cwd:    snapshot.RepositoryPath,
		Branch: runtime.builderBranch,
		Base:   runtime.contract.BaseCommit,
		Label:  "threaddock-" + string(snapshot.RunID),
	})
	if err != nil {
		_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Builder Worktree 생성 실패: %v", err)})
		return err
	}
	runtime.worktree = created
	runtime.stage = 1
	snapshot.Phase = contract.PhaseBuilding
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: "Builder Worktree 준비 완료", Data: map[string]any{"workspaceId": created.WorkspaceID, "paneId": created.PaneID}})
}

func (o *Orchestrator) advanceBuilding(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	if runtime.worktree.PaneID == "" {
		return errors.New("Builder Worktree ID is unavailable; resume requires reconciliation")
	}
	name := "builder-" + string(snapshot.RunID)
	switch runtime.stage {
	case 1:
		if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "Builder Agent 시작", map[string]any{"agent": name}); err != nil {
			return err
		}
		if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: name, PaneID: runtime.worktree.PaneID}); err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Builder Agent 시작 실패: %v", err)})
			return err
		}
		runtime.stage = 2
		snapshot.Builder = state.AgentEvidence{Name: name, SessionID: name}
		return o.saveAction(ctx, snapshot, "Builder Agent 준비 완료")
	case 2:
		if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "Builder packet 전송", map[string]any{"agent": name}); err != nil {
			return err
		}
		if err := o.deps.Herdr.Prompt(ctx, name, builderPacket(runtime.contract, runtime.builderTask)); err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Builder packet 전송 실패: %v", err)})
			return err
		}
		runtime.stage = 3
		return o.saveAction(ctx, snapshot, "Builder 작업 packet 전송 완료")
	case 3:
		if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "Builder verification evidence 수집", map[string]any{"agent": name}); err != nil {
			return err
		}
		evidence, err := o.builderEvidence(ctx, name, runtime.builderTask)
		if err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Builder evidence 부족: %v", err)})
			return err
		}
		evidence.Name = name
		if evidence.SessionID == "" {
			evidence.SessionID = name
		}
		snapshot.Builder = evidence
		runtime.stage = 4
		snapshot.Phase = contract.PhaseIntegrating
		return o.saveAction(ctx, snapshot, "Builder commit과 verification evidence 확인")
	default:
		return fmt.Errorf("invalid Builder stage %d", runtime.stage)
	}
}

func (o *Orchestrator) integrate(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	if !hasEvidence(snapshot.Builder) {
		return ErrBuilderEvidence
	}
	if strings.EqualFold(runtime.builderBranch, runtime.contract.Repository.DefaultBranch) || strings.EqualFold(runtime.builderBranch, "main") {
		return errors.New("main 병합은 single-run orchestrator의 책임이 아닙니다")
	}
	if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "integration Worktree에 Builder commit 반영", map[string]any{"commitSha": snapshot.Builder.CommitSHA, "branch": runtime.builderBranch}); err != nil {
		return err
	}
	if err := o.deps.Worktree.Merge(ctx, snapshot.IntegrationPath, runtime.builderBranch); err != nil {
		_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("integration 실패: %v", err)})
		return err
	}
	runtime.stage = 5
	snapshot.Phase = contract.PhaseReviewing
	return o.saveAction(ctx, snapshot, "integration 완료; main은 변경하지 않음")
}

func (o *Orchestrator) advanceReview(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	name := "reviewer-" + string(snapshot.RunID)
	if runtime.stage < 6 {
		if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "새 Reviewer Agent session 시작", map[string]any{"agent": name, "builderTranscript": false}); err != nil {
			return err
		}
		if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: name, PaneID: runtime.worktree.PaneID}); err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Reviewer Agent 시작 실패: %v", err)})
			return err
		}
		runtime.stage = 6
		snapshot.Reviewer = state.AgentEvidence{Name: name, SessionID: name}
		return o.saveAction(ctx, snapshot, "독립 Reviewer session 준비 완료")
	}
	if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "Reviewer acceptance packet 전송", map[string]any{"agent": name, "builderTranscript": false}); err != nil {
		return err
	}
	if err := o.deps.Herdr.Prompt(ctx, name, reviewerPacket(runtime.contract, runtime.builderTask, snapshot.Builder)); err != nil {
		_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Reviewer packet 전송 실패: %v", err)})
		return err
	}
	snapshot.Reviewer.Verification = []string{"review schema sent"}
	return o.saveAction(ctx, snapshot, "독립 Reviewer가 acceptance criteria를 확인 중")
}

func (o *Orchestrator) builderEvidence(ctx context.Context, name string, task contract.Task) (state.AgentEvidence, error) {
	if provider, ok := o.deps.Herdr.(interface {
		BuilderEvidence(context.Context, string) (state.AgentEvidence, error)
	}); ok {
		evidence, err := provider.BuilderEvidence(ctx, name)
		if err != nil {
			return state.AgentEvidence{}, err
		}
		if !hasEvidence(evidence) {
			return state.AgentEvidence{}, ErrBuilderEvidence
		}
		return evidence, nil
	}
	recent, err := o.deps.Herdr.ReadRecent(ctx, name)
	if err != nil {
		return state.AgentEvidence{}, err
	}
	evidence := parseEvidence(recent)
	if len(evidence.Verification) == 0 && len(task.Verification) == 1 && strings.Contains(recent, task.Verification[0]) {
		evidence.Verification = []string{task.Verification[0]}
	}
	if !hasEvidence(evidence) {
		return state.AgentEvidence{}, ErrBuilderEvidence
	}
	return evidence, nil
}

func parseEvidence(recent string) state.AgentEvidence {
	evidence := state.AgentEvidence{}
	if match := commitEvidencePattern.FindStringSubmatch(recent); len(match) == 2 {
		evidence.CommitSHA = match[1]
	}
	for _, match := range verificationPattern.FindAllStringSubmatch(recent, -1) {
		if value := strings.TrimSpace(match[1]); value != "" {
			evidence.Verification = append(evidence.Verification, value)
		}
	}
	for _, match := range changedFilePattern.FindAllStringSubmatch(recent, -1) {
		if value := strings.TrimSpace(match[1]); value != "" {
			evidence.ChangedFiles = append(evidence.ChangedFiles, value)
		}
	}
	return evidence
}

func hasEvidence(evidence state.AgentEvidence) bool {
	return strings.TrimSpace(evidence.CommitSHA) != "" && len(evidence.Verification) > 0
}

func builderPacket(c contract.TaskContract, task contract.Task) string {
	return fmt.Sprintf("Builder acceptance criteria:\n- %s\n\nTask acceptance criteria:\n- %s\n\nVerification commands:\n- %s\n\nReport commit_sha, changed_file and verification lines.", strings.Join(c.Parent.AcceptanceCriteria, "\n- "), strings.Join(task.AcceptanceCriteria, "\n- "), strings.Join(task.Verification, "\n- "))
}

func reviewerPacket(c contract.TaskContract, task contract.Task, evidence state.AgentEvidence) string {
	return fmt.Sprintf("Reviewer acceptance criteria:\n- %s\n- %s\n\nFinal diff:\n- %s\n\nVerification evidence:\n- %s\n\nReview schema:\ndecision: approve | request_changes\nfindings: list of concrete acceptance-criterion findings", strings.Join(c.Parent.AcceptanceCriteria, "\n- "), strings.Join(task.AcceptanceCriteria, "\n- "), strings.Join(evidence.ChangedFiles, "\n- "), strings.Join(evidence.Verification, "\n- ")+"\n- commit "+evidence.CommitSHA)
}

func (o *Orchestrator) createIssueBundle(ctx context.Context, id contract.RunID, c contract.TaskContract) (github.IssueBundle, error) {
	var bundle github.IssueBundle
	err := o.withGitHubRetry(ctx, id, "Issue bundle 생성", func() error {
		var err error
		bundle, err = o.deps.GitHub.CreateIssueBundle(ctx, github.Repository{Owner: c.Repository.Owner, Name: c.Repository.Name}, c, marker(id))
		return err
	})
	return bundle, err
}

func (o *Orchestrator) withGitHubRetry(ctx context.Context, id contract.RunID, operation string, action func() error) error {
	delays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}
	for attempt := 0; attempt < maxGitHubAttempts; attempt++ {
		err := action()
		if err == nil {
			return nil
		}
		var temporary *github.TemporaryError
		if !errors.As(err, &temporary) || attempt == maxGitHubAttempts-1 {
			if errors.As(err, &temporary) {
				_ = o.append(ctx, id, state.Event{Type: "blocked", Phase: contract.PhaseRegistered, Message: connectionProblem, Data: map[string]any{"operation": operation, "attempts": attempt + 1}})
			}
			return err
		}
		delay := delays[attempt]
		if err := o.append(ctx, id, state.Event{Type: "intent_retry", Phase: contract.PhaseRegistered, Message: fmt.Sprintf("%s 재시도 예약", operation), Data: map[string]any{"attempt": attempt + 2, "delay": delay.String()}}); err != nil {
			return err
		}
		if err := o.sleep(ctx, delay); err != nil {
			return err
		}
	}
	return errors.New("unreachable GitHub retry state")
}

func (o *Orchestrator) transition(ctx context.Context, snapshot state.RunSnapshot, phase contract.RunPhase, message string) error {
	snapshot.Phase = phase
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "phase_transition", Phase: phase, Message: message})
}

func (o *Orchestrator) saveAction(ctx context.Context, snapshot state.RunSnapshot, message string) error {
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: message})
}

func (o *Orchestrator) intent(ctx context.Context, id contract.RunID, phase contract.RunPhase, message string, data map[string]any) error {
	return o.append(ctx, id, state.Event{Type: "intent", Phase: phase, Message: message, Data: data})
}

func (o *Orchestrator) append(ctx context.Context, id contract.RunID, event state.Event) error {
	event.At = o.now()
	return o.deps.Store.Append(ctx, id, event)
}

func (o *Orchestrator) runtimeFor(ctx context.Context, snapshot state.RunSnapshot) (*runRuntime, error) {
	o.mu.Lock()
	runtime := o.runs[snapshot.RunID]
	o.mu.Unlock()
	if runtime != nil {
		return runtime, nil
	}
	if strings.TrimSpace(snapshot.ContractPath) == "" {
		return nil, errors.New("contract path is missing")
	}
	c, err := readContract(snapshot.ContractPath)
	if err != nil {
		return nil, err
	}
	builder := firstBuilder(c)
	runtime = &runRuntime{contract: c, builderTask: builder, builderBranch: builder.Branch, integrationPath: snapshot.IntegrationPath, stage: stageFromSnapshot(snapshot)}
	o.mu.Lock()
	o.runs[snapshot.RunID] = runtime
	o.mu.Unlock()
	return runtime, nil
}

func stageFromSnapshot(snapshot state.RunSnapshot) int {
	if snapshot.Reviewer.Name != "" {
		return 6
	}
	if snapshot.Phase == contract.PhaseIntegrating || snapshot.Phase == contract.PhaseReviewing || snapshot.Builder.CommitSHA != "" {
		return 5
	}
	if snapshot.Builder.Name != "" {
		return 2
	}
	return 1
}

func firstBuilder(c contract.TaskContract) contract.Task {
	for _, task := range c.Tasks {
		if strings.EqualFold(task.Role, "builder") {
			return task
		}
	}
	if len(c.Tasks) > 0 {
		return c.Tasks[0]
	}
	return contract.Task{}
}

func readContract(path string) (contract.TaskContract, error) {
	file, err := os.Open(path)
	if err != nil {
		return contract.TaskContract{}, contract.NewUnreadableError(err)
	}
	defer file.Close()
	return contract.Read(file)
}

func (o *Orchestrator) validate() error {
	if err := o.validateStore(); err != nil {
		return err
	}
	if o.deps.GitHub == nil || o.deps.Herdr == nil || o.deps.Worktree == nil {
		return ErrInvalidDependencies
	}
	return nil
}

func (o *Orchestrator) validateStore() error {
	if o == nil || o.deps.Store == nil {
		return ErrInvalidDependencies
	}
	return nil
}

func (o *Orchestrator) now() time.Time {
	if o.deps.Clock == nil {
		return time.Now()
	}
	return o.deps.Clock.Now()
}

func (o *Orchestrator) sleep(ctx context.Context, delay time.Duration) error {
	if sleeper, ok := o.deps.Clock.(Sleeper); ok {
		return sleeper.Sleep(ctx, delay)
	}
	return realClock{}.Sleep(ctx, delay)
}

func (o *Orchestrator) nextRunID() contract.RunID {
	now := o.now()
	if o.deps.RunID != nil {
		return o.deps.RunID(now)
	}
	o.mu.Lock()
	o.seq++
	sequence := o.seq
	o.mu.Unlock()
	return contract.RunID(fmt.Sprintf("run-%d-%d", now.UnixNano(), sequence))
}

func marker(id contract.RunID) string { return "td:" + string(id) }

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
