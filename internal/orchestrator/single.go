package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"thread-dock/internal/worktree"
)

var (
	ErrInvalidDependencies = errors.New("orchestrator dependencies are incomplete")
	ErrRunPaused           = errors.New("run is paused")
	ErrRunFinished         = errors.New("run is no longer active")
	ErrBuilderEvidence     = errors.New("builder commit and verification evidence are required")
	ErrRunBusy             = state.ErrRunBusy
	ErrPendingReconcile    = errors.New("pending action requires reconciliation")
	ErrRegistrationPending = errors.New("GHES registration is pending reconciliation")
	ErrSensitivePatch      = errors.New("reviewer patch contains credentials")
)

const (
	maxGitHubAttempts = 4
	connectionProblem = "GitHub 연결 문제"
)

var credentialPattern = regexp.MustCompile(`(?im)(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|(?:AKIA|ASIA)[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|(?:^|[^A-Za-z0-9])["']?(?:token|secret|password|authorization|api[_-]?key|private[_-]?key|client[_-]?(?:secret|key)|[A-Za-z_][A-Za-z0-9_.-]*(?:token|secret|password|authorization|api[_-]?key|private[_-]?key|client[_-]?(?:secret|key)))["']?[ \t]*[:=][ \t]*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:Bearer[ \t]+)?[^\s,;}\]]+))`)

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
	contract          contract.TaskContract
	bundle            github.IssueBundle
	worktree          herdr.Worktree
	builderTask       contract.Task
	builderBranch     string
	integrationPath   string
	stage             int
	integrationBranch string
}

// Orchestrator advances exactly one run. Runtime metadata is deliberately
// small and complements the durable snapshot; the snapshot and event log are
// the source of truth for phase and evidence.
type Orchestrator struct {
	deps  Dependencies
	mu    sync.Mutex
	seq   uint64
	runs  map[contract.RunID]*runRuntime
	locks map[contract.RunID]*sync.Mutex
}

func New(deps Dependencies) *Orchestrator {
	if deps.Worktree == nil {
		deps.Worktree = deps.Git
	}
	if deps.Clock == nil {
		deps.Clock = realClock{}
	}
	return &Orchestrator{deps: deps, runs: make(map[contract.RunID]*runRuntime), locks: make(map[contract.RunID]*sync.Mutex)}
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
	repositoryPath, err = filepath.Abs(repositoryPath)
	if err != nil {
		return "", err
	}
	repositoryPath, err = filepath.Abs(repositoryPath)
	if err != nil {
		return "", err
	}
	worktreeRoot := o.deps.WorktreeRoot
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(filepath.Dir(contractPath), ".threaddock-worktrees")
	}
	worktreeRoot, err = filepath.Abs(worktreeRoot)
	if err != nil {
		return "", err
	}
	worktreeRoot, err = filepath.Abs(worktreeRoot)
	if err != nil {
		return "", err
	}
	integrationPath := filepath.Join(worktreeRoot, string(id))
	integrationBranch := "agent/" + safeBranchPart(c.Parent.Key) + "-integration"
	snapshot := state.RunSnapshot{
		ContractVersion: c.Version,
		RunID:           id,
		Phase:           contract.PhaseRegistered,
		ContractPath:    contractPath,
		RepositoryPath:  repositoryPath,
		IntegrationPath: integrationPath,
		Integration:     state.WorktreeState{Path: integrationPath, Branch: integrationBranch},
		Registration:    state.RegistrationState{Status: "pending", Marker: marker(id)},
		PendingAction:   "register_issue_bundle",
		Summary:         "GHES Issue 등록 대기 중",
		UpdatedAt:       o.now(),
	}
	if err := o.deps.Store.Create(ctx, snapshot); err != nil {
		return "", err
	}
	runtime := &runRuntime{
		contract:          c,
		builderTask:       firstBuilder(c),
		builderBranch:     firstBuilder(c).Branch,
		integrationPath:   integrationPath,
		integrationBranch: integrationBranch,
	}
	o.mu.Lock()
	o.runs[id] = runtime
	o.mu.Unlock()

	if err := o.intent(ctx, id, contract.PhaseRegistered, "Issue bundle 생성", map[string]any{"marker": marker(id)}); err != nil {
		return id, err
	}
	bundle, err := o.createIssueBundle(ctx, id, c)
	if err != nil {
		snapshot.Summary = connectionProblem
		snapshot.UpdatedAt = o.now()
		_ = o.deps.Store.Save(ctx, snapshot)
		_ = o.append(ctx, id, state.Event{Type: "action_failed", Phase: contract.PhaseRegistered, Message: fmt.Sprintf("Issue bundle 생성 실패: %v", err)})
		return id, err
	}
	runtime.bundle = bundle
	snapshot.ParentIssue = bundle.Parent.Number
	snapshot.Registration = state.RegistrationState{Status: "registered", Marker: marker(id), NodeID: bundle.Parent.NodeID, Issue: bundle.Parent.Number}
	snapshot.PendingAction = ""
	snapshot.Summary = "Issue bundle 준비 완료"
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
	release, err := o.claim(ctx, id)
	if err != nil {
		return err
	}
	defer release()
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
	if snapshot.PendingAction != "" {
		return o.reconcilePending(ctx, &snapshot, runtime)
	}
	if snapshot.Phase == contract.PhaseRegistered && snapshot.Registration.Status != "registered" {
		return o.retryRegistration(ctx, &snapshot, runtime)
	}

	switch snapshot.Phase {
	case contract.PhaseRegistered:
		if err := o.intent(ctx, snapshot.RunID, snapshot.Phase, "분석 단계 전환", nil); err != nil {
			return err
		}
		return o.transition(ctx, snapshot, contract.PhaseAnalyzing, "분석 단계 시작")
	case contract.PhaseAnalyzing:
		if snapshot.ActionCursor == 0 {
			return o.createIntegrationWorktree(ctx, &snapshot, runtime)
		}
		return o.createWorktree(ctx, snapshot, runtime)
	case contract.PhaseBuilding:
		if snapshot.ActionCursor == 1 {
			return o.baselinePrompt(ctx, &snapshot, false)
		}
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
	release, err := o.claim(ctx, id)
	if err != nil {
		return err
	}
	defer release()
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
	snapshot.PreviousPhase = snapshot.Phase
	snapshot.Phase = contract.PhasePaused
	snapshot.Summary = "실행을 일시 중지했습니다"
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, id, state.Event{Type: "paused", Phase: contract.PhasePaused, Message: "실행을 일시 중지했습니다"})
}

func (o *Orchestrator) retryRegistration(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	if err := o.prepare(ctx, snapshot, "register_issue_bundle", "GHES Issue bundle 재등록", map[string]any{"marker": marker(snapshot.RunID)}); err != nil {
		return err
	}
	bundle, err := o.createIssueBundle(ctx, snapshot.RunID, runtime.contract)
	if err != nil {
		snapshot.Summary = connectionProblem
		snapshot.UpdatedAt = o.now()
		_ = o.deps.Store.Save(ctx, *snapshot)
		return err
	}
	snapshot.Registration = state.RegistrationState{Status: "registered", Marker: marker(snapshot.RunID), NodeID: bundle.Parent.NodeID, Issue: bundle.Parent.Number}
	snapshot.ParentIssue = bundle.Parent.Number
	return o.finish(ctx, snapshot, "Issue bundle 재등록 완료", false)
}

func (o *Orchestrator) createIntegrationWorktree(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	if err := o.prepare(ctx, snapshot, "create_integration_worktree", "integration Worktree 생성", map[string]any{"path": snapshot.IntegrationPath, "branch": runtime.integrationBranch}); err != nil {
		return err
	}
	if err := o.deps.Worktree.Create(ctx, snapshot.RepositoryPath, snapshot.IntegrationPath, runtime.integrationBranch, runtime.contract.BaseCommit); err != nil {
		return err
	}
	snapshot.Integration = state.WorktreeState{Path: snapshot.IntegrationPath, Branch: runtime.integrationBranch}
	return o.finish(ctx, snapshot, "integration Worktree 생성 완료", false)
}

func (o *Orchestrator) createWorktree(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	snapshot.BuilderWorktree.Branch = runtime.builderBranch
	if err := o.prepare(ctx, &snapshot, "create_builder_worktree", "Builder Worktree 생성", map[string]any{"branch": runtime.builderBranch}); err != nil {
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
	if created.Path == "" || created.WorkspaceID == "" || created.PaneID == "" {
		return errors.New("Herdr returned incomplete Builder Worktree identity")
	}
	runtime.worktree = created
	runtime.stage = 1
	snapshot.BuilderWorktree = state.WorktreeState{Path: created.Path, WorkspaceID: created.WorkspaceID, PaneID: created.PaneID, Branch: runtime.builderBranch}
	snapshot.PendingAction = ""
	snapshot.ActionCursor = 0
	snapshot.Phase = contract.PhaseBuilding
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: "Builder Worktree 준비 완료", Data: map[string]any{"workspaceId": created.WorkspaceID, "paneId": created.PaneID, "path": created.Path}})
}

func promptRequestID(id contract.RunID, role string) string {
	return string(id) + ":" + role + "-prompt"
}

const maxHerdrAgentNameLength = 32

// agentName returns the stable name used to address a role's Herdr agent.
// Existing names remain unchanged while they fit Herdr's name contract. For
// longer or otherwise incompatible run IDs, the role stays readable and the
// remaining space carries a truncated SHA-256 digest of the run ID.
func agentName(role string, id contract.RunID) string {
	role = strings.ToLower(strings.TrimSpace(role))
	legacy := role + "-" + string(id)
	if validHerdrAgentName(legacy) {
		return legacy
	}

	digest := sha256.Sum256([]byte(id))
	suffix := hex.EncodeToString(digest[:])
	available := maxHerdrAgentNameLength - len(role) - 1
	if available < 1 {
		return suffix[:maxHerdrAgentNameLength]
	}
	if available > len(suffix) {
		available = len(suffix)
	}
	return role + "-" + suffix[:available]
}

func validHerdrAgentName(name string) bool {
	if len(name) == 0 || len(name) > maxHerdrAgentNameLength {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func (o *Orchestrator) baselinePrompt(ctx context.Context, snapshot *state.RunSnapshot, reviewer bool) error {
	locator, ok := o.deps.Herdr.(AgentLocator)
	if !ok {
		return errors.New("Herdr agent state lookup is required before prompt")
	}
	name := snapshot.Builder.Name
	role := "builder"
	if reviewer {
		name = snapshot.Reviewer.Name
		role = "reviewer"
	}
	receipt := &snapshot.BuilderPrompt
	if reviewer {
		receipt = &snapshot.ReviewerPrompt
	}
	receipt.RequestID = promptRequestID(snapshot.RunID, role)
	if err := o.prepare(ctx, snapshot, "baseline_"+role+"_prompt", "prompt baseline 조회", map[string]any{"agent": name, "requestId": receipt.RequestID}); err != nil {
		return err
	}
	info, err := locator.GetInfo(ctx, name)
	work := snapshot.BuilderWorktree
	if reviewer {
		work = snapshot.ReviewerWorktree
	}
	if err != nil || info.StateChangeSeq <= 0 || !matchesAgentIdentity(info, name, work) {
		return errors.New("Herdr agent state_change_seq is unavailable")
	}
	if reviewer {
		snapshot.Reviewer.SessionID = info.SessionID
	} else {
		snapshot.Builder.SessionID = info.SessionID
	}
	receipt.BaselineSeq = info.StateChangeSeq
	return o.finish(ctx, snapshot, "prompt baseline 저장", false)
}

func (o *Orchestrator) advanceBuilding(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	if snapshot.ActionCursor == 0 && snapshot.BuilderWorktree.PaneID == "" && runtime.worktree.PaneID == "" {
		return errors.New("Builder Worktree ID is unavailable; resume requires reconciliation")
	}
	name := agentName("builder", snapshot.RunID)
	if snapshot.ActionCursor == 0 {
		snapshot.Builder.Name = name
		if err := o.prepare(ctx, &snapshot, "start_builder", "Builder Agent 시작", map[string]any{"agent": name}); err != nil {
			return err
		}
		paneID := snapshot.BuilderWorktree.PaneID
		if paneID == "" {
			paneID = runtime.worktree.PaneID
		}
		if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: name, PaneID: paneID}); err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Builder Agent 시작 실패: %v", err)})
			return err
		}
		snapshot.Builder = state.AgentEvidence{Name: name}
		return o.finish(ctx, &snapshot, "Builder Agent 준비 완료", false)
	}
	if snapshot.ActionCursor == 2 {
		if snapshot.BuilderPrompt.RequestID == "" {
			return errors.New("Builder prompt receipt is missing")
		}
		if err := o.prepare(ctx, &snapshot, "prompt_builder", "Builder packet 전송", map[string]any{"agent": name}); err != nil {
			return err
		}
		if err := o.deps.Herdr.Prompt(ctx, name, builderPacket(runtime.contract, runtime.builderTask, snapshot.BuilderPrompt.RequestID)); err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Builder packet 전송 실패: %v", err)})
			return err
		}
		return o.finish(ctx, &snapshot, "Builder 작업 packet 전송 완료", false)
	}
	if err := o.prepare(ctx, &snapshot, "collect_builder_evidence", "Builder verification evidence 수집", map[string]any{"agent": name}); err != nil {
		return err
	}
	reader, ok := o.deps.Herdr.(EvidenceReader)
	if !ok {
		return ErrBuilderEvidence
	}
	evidence, err := reader.ReadEvidence(ctx, name)
	if err != nil || evidence.RequestID != snapshot.BuilderPrompt.RequestID || !validCommitSHA(evidence.CommitSHA) || len(evidence.Verification) == 0 {
		return ErrBuilderEvidence
	}
	if !verificationMatchesTask(evidence.Verification, runtime.builderTask.Verification) {
		return ErrBuilderEvidence
	}
	for _, check := range evidence.Verification {
		if strings.ToLower(strings.TrimSpace(check.Outcome)) != "passed" || strings.TrimSpace(check.Command) == "" || strings.TrimSpace(check.Duration) == "" {
			return ErrBuilderEvidence
		}
		if _, err := time.ParseDuration(strings.TrimSpace(check.Duration)); err != nil {
			return ErrBuilderEvidence
		}
	}
	snapshot.Builder.Name = name
	snapshot.Builder.RequestID = evidence.RequestID
	snapshot.Builder.CommitSHA = strings.ToLower(evidence.CommitSHA)
	snapshot.Builder.Verification = make([]string, 0, len(evidence.Verification))
	snapshot.Builder.VerificationEvidence = make([]state.VerificationEvidence, 0, len(evidence.Verification))
	for _, check := range evidence.Verification {
		command := strings.TrimSpace(check.Command)
		snapshot.Builder.Verification = append(snapshot.Builder.Verification, command)
		snapshot.Builder.VerificationEvidence = append(snapshot.Builder.VerificationEvidence, state.VerificationEvidence{Command: command, Outcome: "passed", Duration: strings.TrimSpace(check.Duration)})
	}
	snapshot.Phase = contract.PhaseIntegrating
	snapshot.ActionCursor = 0
	return o.finish(ctx, &snapshot, "Builder structured evidence 확인", true)
}

func (o *Orchestrator) integrate(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	if snapshot.ActionCursor == 0 {
		inspector, ok := o.deps.Worktree.(WorktreeInspector)
		if !ok || !hasEvidence(snapshot.Builder) || snapshot.Builder.RequestID != snapshot.BuilderPrompt.RequestID {
			return ErrBuilderEvidence
		}
		if err := o.prepare(ctx, &snapshot, "inspect_builder_commit", "Builder commit 검증", map[string]any{"commitSha": snapshot.Builder.CommitSHA}); err != nil {
			return err
		}
		inspection, err := inspector.InspectCommit(ctx, snapshot.BuilderWorktree.Path, runtime.contract.BaseCommit, runtime.builderBranch, snapshot.Builder.CommitSHA)
		if err != nil {
			return err
		}
		if err := validateInspection(inspection, snapshot.Builder.CommitSHA, runtime.builderBranch, runtime.builderTask.AllowedPaths); err != nil {
			return err
		}
		if containsCredential(inspection.Patch) {
			return o.blockSensitiveInspection(ctx, &snapshot)
		}
		snapshot.Builder.Branch = inspection.Branch
		snapshot.Builder.ChangedFiles = append([]string(nil), inspection.ChangedFiles...)
		snapshot.Builder.Patch = inspection.Patch
		return o.finish(ctx, &snapshot, "Git가 생성한 Builder patch 검증 완료", false)
	}
	if !hasEvidence(snapshot.Builder) || snapshot.Builder.RequestID != snapshot.BuilderPrompt.RequestID || snapshot.Builder.Branch != runtime.builderBranch || snapshot.Builder.Patch == "" {
		return ErrBuilderEvidence
	}
	if strings.EqualFold(runtime.builderBranch, runtime.contract.Repository.DefaultBranch) || strings.EqualFold(runtime.builderBranch, "main") {
		return errors.New("main 병합은 single-run orchestrator의 책임이 아닙니다")
	}
	merger, ok := o.deps.Worktree.(ImmutableMerger)
	if !ok {
		return ErrBuilderEvidence
	}
	if err := o.prepare(ctx, &snapshot, "merge_verified_commit", "integration Worktree에 Builder commit 반영", map[string]any{"commitSha": snapshot.Builder.CommitSHA, "branch": runtime.builderBranch}); err != nil {
		return err
	}
	if err := merger.MergeCommit(ctx, snapshot.Integration.Path, snapshot.Builder.CommitSHA); err != nil {
		_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("integration 실패: %v", err)})
		return err
	}
	runtime.stage = 5
	snapshot.Phase = contract.PhaseReviewing
	return o.finish(ctx, &snapshot, "integration 완료; main은 변경하지 않음", true)
}

func (o *Orchestrator) advanceReview(ctx context.Context, snapshot state.RunSnapshot, runtime *runRuntime) error {
	name := agentName("reviewer", snapshot.RunID)
	if snapshot.ActionCursor == 0 {
		opener, ok := o.deps.Herdr.(WorktreeOpener)
		if !ok {
			return errors.New("Herdr integration Worktree opener is required")
		}
		if err := o.prepare(ctx, &snapshot, "open_reviewer_worktree", "integration Worktree를 Reviewer에 연결", map[string]any{"path": snapshot.IntegrationPath}); err != nil {
			return err
		}
		opened, err := opener.OpenWorktree(ctx, herdr.OpenWorktreeRequest{Cwd: snapshot.RepositoryPath, Path: snapshot.IntegrationPath, Label: "threaddock-review-" + string(snapshot.RunID)})
		if err != nil || opened.Path == "" || opened.PaneID == "" || opened.WorkspaceID == "" || filepath.Clean(opened.Path) != filepath.Clean(snapshot.IntegrationPath) {
			return errors.New("Herdr returned incomplete Reviewer Worktree identity")
		}
		snapshot.ReviewerWorktree = state.WorktreeState{Path: opened.Path, WorkspaceID: opened.WorkspaceID, PaneID: opened.PaneID, Branch: snapshot.Integration.Branch}
		return o.finish(ctx, &snapshot, "Reviewer Worktree 준비 완료", false)
	}
	if snapshot.ActionCursor == 1 {
		snapshot.Reviewer.Name = name
		if err := o.prepare(ctx, &snapshot, "start_reviewer", "새 Reviewer Agent session 시작", map[string]any{"agent": name, "builderTranscript": false}); err != nil {
			return err
		}
		if err := o.deps.Herdr.StartAgent(ctx, herdr.StartAgentRequest{Name: name, PaneID: snapshot.ReviewerWorktree.PaneID}); err != nil {
			_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Reviewer Agent 시작 실패: %v", err)})
			return err
		}
		snapshot.Reviewer = state.AgentEvidence{Name: name}
		return o.finish(ctx, &snapshot, "독립 Reviewer session 준비 완료", false)
	}
	if snapshot.ActionCursor == 2 {
		return o.baselinePrompt(ctx, &snapshot, true)
	}
	if containsCredential(snapshot.Builder.Patch) {
		return ErrSensitivePatch
	}
	if err := o.prepare(ctx, &snapshot, "prompt_reviewer", "Reviewer acceptance packet 전송", map[string]any{"agent": name, "builderTranscript": false}); err != nil {
		return err
	}
	if snapshot.ReviewerPrompt.RequestID == "" {
		return errors.New("Reviewer prompt receipt is missing")
	}
	if err := o.deps.Herdr.Prompt(ctx, name, reviewerPacket(runtime.contract, runtime.builderTask, snapshot.Builder, snapshot.ReviewerPrompt.RequestID)); err != nil {
		_ = o.append(ctx, snapshot.RunID, state.Event{Type: "action_failed", Phase: snapshot.Phase, Message: fmt.Sprintf("Reviewer packet 전송 실패: %v", err)})
		return err
	}
	snapshot.Reviewer.Verification = []string{"review schema sent"}
	return o.finish(ctx, &snapshot, "독립 Reviewer가 acceptance criteria를 확인 중", false)
}

func hasEvidence(evidence state.AgentEvidence) bool {
	if !validCommitSHA(evidence.CommitSHA) || len(evidence.VerificationEvidence) == 0 {
		return false
	}
	for _, check := range evidence.VerificationEvidence {
		if check.Command == "" || check.Outcome != "passed" || check.Duration == "" {
			return false
		}
		if _, err := time.ParseDuration(check.Duration); err != nil {
			return false
		}
	}
	return true
}

func matchesAgentIdentity(info herdr.AgentInfo, expectedName string, expectedWorktree state.WorktreeState) bool {
	return strings.TrimSpace(info.Name) == strings.TrimSpace(expectedName) &&
		strings.TrimSpace(info.SessionID) != "" &&
		strings.TrimSpace(info.WorkspaceID) == strings.TrimSpace(expectedWorktree.WorkspaceID) &&
		strings.TrimSpace(info.PaneID) == strings.TrimSpace(expectedWorktree.PaneID)
}

func verificationMatchesTask(checks []herdr.VerificationCheck, required []string) bool {
	if len(checks) != len(required) || len(required) == 0 {
		return false
	}
	want := make(map[string]struct{}, len(required))
	for _, command := range required {
		want[strings.TrimSpace(command)] = struct{}{}
	}
	for _, check := range checks {
		command := strings.TrimSpace(check.Command)
		if _, ok := want[command]; !ok {
			return false
		}
		delete(want, command)
	}
	return len(want) == 0
}

func builderPacket(c contract.TaskContract, task contract.Task, requestID string) string {
	return fmt.Sprintf("Builder acceptance criteria:\n- %s\n\nTask acceptance criteria:\n- %s\n\nRequired verification commands (each must be passed with a real duration):\n- %s\n\nYour final response must contain exactly one strict Evidence JSON payload between these markers, each on its own line:\n%s\n%s\n%s\n%s\nUI or diagnostic text may appear outside the markers. Put no Markdown or other fields inside the envelope. Use requestId=%s. Do not substitute arbitrary commands or boolean values.", strings.Join(c.Parent.AcceptanceCriteria, "\n- "), strings.Join(task.AcceptanceCriteria, "\n- "), strings.Join(task.Verification, "\n- "), herdr.THREADDOCK_EVIDENCE_BEGIN, herdr.EvidenceSchemaExample, herdr.THREADDOCK_EVIDENCE_END, "Return exactly one JSON object inside the envelope.", requestID)
}

func reviewerPacket(c contract.TaskContract, task contract.Task, evidence state.AgentEvidence, requestID string) string {
	checks := make([]string, 0, len(evidence.VerificationEvidence))
	for _, check := range evidence.VerificationEvidence {
		checks = append(checks, fmt.Sprintf("Command: %s\nOutcome: %s\nDuration: %s", check.Command, check.Outcome, check.Duration))
	}
	return fmt.Sprintf("Reviewer acceptance criteria:\n- %s\n- %s\n\nFinal bounded patch:\n%s\n\nStructured verification:\n%s\nCommit SHA: %s\nPrompt request ID: %s\n\nReview schema:\nrequestId: %s (include this exact value in the review result)\ndecision: approve | request_changes\nfindings: list of concrete acceptance-criterion findings", strings.Join(c.Parent.AcceptanceCriteria, "\n- "), strings.Join(task.AcceptanceCriteria, "\n- "), redactPatch(evidence.Patch), strings.Join(checks, "\n"), evidence.CommitSHA, requestID, requestID)
}

func validCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func validateInspection(inspection worktree.CommitInspection, expectedSHA, expectedBranch string, allowed []string) error {
	if !validCommitSHA(inspection.CommitSHA) || inspection.CommitSHA != expectedSHA || inspection.Branch != expectedBranch {
		return errors.New("commit SHA or Builder branch validation failed")
	}
	if len(inspection.ChangedFiles) == 0 || strings.TrimSpace(inspection.Patch) == "" {
		return errors.New("Git returned no bounded patch")
	}
	for _, file := range inspection.ChangedFiles {
		if !allowedPath(file, allowed) {
			return fmt.Errorf("changed path %q is outside allowed paths", file)
		}
	}
	return nil
}

func allowedPath(file string, allowed []string) bool {
	file = strings.TrimPrefix(strings.ReplaceAll(filepath.Clean(file), "\\", "/"), "./")
	for _, pattern := range allowed {
		pattern = strings.TrimPrefix(strings.ReplaceAll(pattern, "\\", "/"), "./")
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if file == prefix || strings.HasPrefix(file, prefix+"/") {
				return true
			}
		} else if file == pattern {
			return true
		}
	}
	return false
}

func redactPatch(patch string) string {
	if len(patch) > 512*1024 {
		patch = patch[:512*1024]
	}
	lines := strings.Split(patch, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, key := range []string{"token", "secret", "password", "authorization"} {
			if index := strings.Index(lower, key); index >= 0 {
				rest := line[index+len(key):]
				if separator := strings.IndexAny(rest, ":="); separator >= 0 {
					position := index + len(key) + separator + 1
					line = line[:position] + " [REDACTED]"
				}
			}
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func containsCredential(patch string) bool {
	return credentialPattern.MatchString(patch)
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
	snapshot.ActionCursor = 0
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "phase_transition", Phase: phase, Message: message})
}

func (o *Orchestrator) saveAction(ctx context.Context, snapshot state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.ActionCursor++
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "action_succeeded", Phase: snapshot.Phase, Message: message})
}

func (o *Orchestrator) prepare(ctx context.Context, snapshot *state.RunSnapshot, action, message string, data map[string]any) error {
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

func (o *Orchestrator) markNotExecuted(ctx context.Context, snapshot *state.RunSnapshot, message string) error {
	snapshot.PendingAction = ""
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	return o.append(ctx, snapshot.RunID, state.Event{Type: "reconcile_not_executed", Phase: snapshot.Phase, Message: message})
}

func (o *Orchestrator) blockSensitiveInspection(ctx context.Context, snapshot *state.RunSnapshot) error {
	snapshot.Builder.Patch = ""
	snapshot.Builder.ChangedFiles = nil
	snapshot.Summary = "민감 credential이 포함된 patch로 Reviewer 전송을 차단했습니다"
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
		return err
	}
	_ = o.append(ctx, snapshot.RunID, state.Event{Type: "blocked", Phase: snapshot.Phase, Message: "민감 credential patch 차단"})
	return ErrSensitivePatch
}

func (o *Orchestrator) finish(ctx context.Context, snapshot *state.RunSnapshot, message string, phaseChanged bool) error {
	snapshot.PendingAction = ""
	snapshot.ActionCursor++
	if phaseChanged {
		snapshot.ActionCursor = 0
	}
	snapshot.Summary = message
	snapshot.UpdatedAt = o.now()
	if err := o.deps.Store.Save(ctx, *snapshot); err != nil {
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

func (o *Orchestrator) claim(ctx context.Context, id contract.RunID) (func(), error) {
	if locker, ok := o.deps.Store.(LockingStore); ok {
		lease, err := locker.Acquire(ctx, id)
		if err != nil {
			return nil, err
		}
		return func() { _ = lease.Release() }, nil
	}
	o.mu.Lock()
	lock := o.locks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		o.locks[id] = lock
	}
	o.mu.Unlock()
	if !lock.TryLock() {
		return nil, ErrRunBusy
	}
	return lock.Unlock, nil
}

func (o *Orchestrator) reconcilePending(ctx context.Context, snapshot *state.RunSnapshot, runtime *runRuntime) error {
	switch snapshot.PendingAction {
	case "register_issue_bundle":
		var bundle github.IssueBundle
		err := o.withGitHubRetry(ctx, snapshot.RunID, "Issue bundle reconcile", func() error {
			var found bool
			var findErr error
			bundle, found, findErr = o.deps.GitHub.FindIssueBundle(ctx, github.Repository{Owner: runtime.contract.Repository.Owner, Name: runtime.contract.Repository.Name}, marker(snapshot.RunID))
			if findErr == nil && !found {
				return ErrRegistrationPending
			}
			return findErr
		})
		if err != nil {
			if errors.Is(err, ErrRegistrationPending) {
				return o.markNotExecuted(ctx, snapshot, "GHES marker 미등록 확인; 다음 Advance에서 Issue bundle 실행")
			}
			return err
		}
		snapshot.Registration = state.RegistrationState{Status: "registered", Marker: marker(snapshot.RunID), NodeID: bundle.Parent.NodeID, Issue: bundle.Parent.Number}
		snapshot.ParentIssue = bundle.Parent.Number
		snapshot.Summary = "Issue bundle reconcile 완료"
		return o.finish(ctx, snapshot, "기존 GHES Issue bundle 확인", false)
	case "create_builder_worktree", "open_reviewer_worktree":
		locator, ok := o.deps.Herdr.(WorktreeLocator)
		if !ok {
			return ErrPendingReconcile
		}
		path, branch := snapshot.Integration.Path, snapshot.Integration.Branch
		if snapshot.PendingAction == "create_builder_worktree" {
			path, branch = snapshot.BuilderWorktree.Path, snapshot.BuilderWorktree.Branch
		}
		label := branch
		if snapshot.PendingAction == "create_builder_worktree" {
			label = "threaddock-" + string(snapshot.RunID)
		}
		if snapshot.PendingAction == "open_reviewer_worktree" {
			label = "threaddock-review-" + string(snapshot.RunID)
		}
		found, exists, err := locator.FindWorktree(ctx, snapshot.RepositoryPath, path, label)
		if err != nil {
			return ErrPendingReconcile
		}
		if !exists {
			if snapshot.PendingAction == "open_reviewer_worktree" {
				return o.markNotExecuted(ctx, snapshot, "Reviewer Worktree 미발견 확인; 다음 Advance에서 연결")
			}
			return o.markNotExecuted(ctx, snapshot, "Builder Worktree 미발견 확인; 다음 Advance에서 생성")
		}
		if snapshot.PendingAction == "create_builder_worktree" {
			snapshot.BuilderWorktree = state.WorktreeState{Path: found.Path, WorkspaceID: found.WorkspaceID, PaneID: found.PaneID, Branch: branch}
			snapshot.Phase = contract.PhaseBuilding
			snapshot.ActionCursor = 0
			return o.finish(ctx, snapshot, "기존 Builder Worktree reconcile 완료", true)
		}
		if snapshot.PendingAction == "open_reviewer_worktree" {
			snapshot.ReviewerWorktree = state.WorktreeState{Path: found.Path, WorkspaceID: found.WorkspaceID, PaneID: found.PaneID, Branch: branch}
			return o.finish(ctx, snapshot, "기존 Reviewer Worktree reconcile 완료", false)
		}
		snapshot.Integration = state.WorktreeState{Path: found.Path, WorkspaceID: found.WorkspaceID, PaneID: found.PaneID, Branch: branch}
		return o.finish(ctx, snapshot, "기존 integration Worktree reconcile 완료", false)
	case "create_integration_worktree":
		locator, ok := o.deps.Worktree.(IntegrationWorktreeLocator)
		if !ok {
			return ErrPendingReconcile
		}
		found, err := locator.ReconcileIntegrationWorktree(ctx, snapshot.Integration.Path, snapshot.Integration.Branch, runtime.contract.BaseCommit)
		if err != nil {
			return ErrPendingReconcile
		}
		if !found {
			return o.markNotExecuted(ctx, snapshot, "integration Worktree 미발견 확인; 다음 Advance에서 생성")
		}
		return o.finish(ctx, snapshot, "기존 integration Worktree Git reconcile 완료", false)
	case "start_builder", "start_reviewer":
		locator, ok := o.deps.Herdr.(AgentLocator)
		if !ok {
			return ErrPendingReconcile
		}
		role := "builder"
		name := snapshot.Builder.Name
		if snapshot.PendingAction == "start_reviewer" {
			role = "reviewer"
			name = snapshot.Reviewer.Name
		}
		legacyName := role + "-" + string(snapshot.RunID)
		if name == legacyName && name != agentName(role, snapshot.RunID) {
			if role == "reviewer" {
				snapshot.Reviewer.Name = agentName(role, snapshot.RunID)
			} else {
				snapshot.Builder.Name = agentName(role, snapshot.RunID)
			}
			return o.markNotExecuted(ctx, snapshot, "legacy Agent name migrated; next Advance retries start")
		}
		info, err := locator.GetInfo(ctx, name)
		if errors.Is(err, herdr.ErrAgentNotFound) {
			return o.markNotExecuted(ctx, snapshot, "Agent 미발견 확인; 다음 Advance에서 start 실행")
		}
		if err != nil || info.Name == "" {
			return ErrPendingReconcile
		}
		work := snapshot.BuilderWorktree
		if snapshot.PendingAction == "start_reviewer" {
			work = snapshot.ReviewerWorktree
		}
		if !matchesAgentIdentity(info, name, work) {
			return ErrPendingReconcile
		}
		if strings.HasPrefix(snapshot.PendingAction, "start_") {
			if snapshot.PendingAction == "start_builder" {
				snapshot.Builder.SessionID = info.SessionID
			} else {
				snapshot.Reviewer.SessionID = info.SessionID
			}
		}
		return o.finish(ctx, snapshot, "기존 Agent action reconcile 완료", false)
	case "baseline_builder_prompt", "baseline_reviewer_prompt":
		locator, ok := o.deps.Herdr.(AgentLocator)
		if !ok {
			return ErrPendingReconcile
		}
		name := snapshot.Builder.Name
		receipt := &snapshot.BuilderPrompt
		if snapshot.PendingAction == "baseline_reviewer_prompt" {
			name = snapshot.Reviewer.Name
			receipt = &snapshot.ReviewerPrompt
		}
		info, err := locator.GetInfo(ctx, name)
		work := snapshot.BuilderWorktree
		if snapshot.PendingAction == "baseline_reviewer_prompt" {
			work = snapshot.ReviewerWorktree
		}
		if err != nil || info.StateChangeSeq <= 0 || !matchesAgentIdentity(info, name, work) {
			return ErrPendingReconcile
		}
		if snapshot.PendingAction == "baseline_reviewer_prompt" {
			snapshot.Reviewer.SessionID = info.SessionID
		} else {
			snapshot.Builder.SessionID = info.SessionID
		}
		receipt.BaselineSeq = info.StateChangeSeq
		return o.finish(ctx, snapshot, "prompt baseline reconcile 완료", false)
	case "prompt_builder", "prompt_reviewer":
		reader, ok := o.deps.Herdr.(PromptReceiptReader)
		if !ok {
			return ErrPendingReconcile
		}
		name := snapshot.Builder.Name
		receipt := snapshot.BuilderPrompt
		if snapshot.PendingAction == "prompt_reviewer" {
			name = snapshot.Reviewer.Name
			receipt = snapshot.ReviewerPrompt
		}
		info, observed, err := reader.ReadPromptReceipt(ctx, name, receipt.RequestID)
		if err != nil || info.Name == "" || info.StateChangeSeq <= 0 {
			return ErrPendingReconcile
		}
		if observed {
			return o.finish(ctx, snapshot, "prompt receipt reconcile 완료", false)
		}
		if info.StateChangeSeq == receipt.BaselineSeq {
			return o.markNotExecuted(ctx, snapshot, "prompt 미실행 확인; 다음 Advance에서 재전송")
		}
		return ErrPendingReconcile
	case "collect_builder_evidence":
		reader, ok := o.deps.Herdr.(EvidenceReader)
		if !ok {
			return ErrPendingReconcile
		}
		evidence, err := reader.ReadEvidence(ctx, snapshot.Builder.Name)
		if err != nil || evidence.RequestID != snapshot.BuilderPrompt.RequestID || !validCommitSHA(evidence.CommitSHA) || len(evidence.Verification) == 0 || !verificationMatchesTask(evidence.Verification, runtime.builderTask.Verification) {
			return ErrPendingReconcile
		}
		for _, check := range evidence.Verification {
			if strings.ToLower(strings.TrimSpace(check.Outcome)) != "passed" || strings.TrimSpace(check.Command) == "" || strings.TrimSpace(check.Duration) == "" {
				return ErrPendingReconcile
			}
			if _, err := time.ParseDuration(strings.TrimSpace(check.Duration)); err != nil {
				return ErrPendingReconcile
			}
		}
		snapshot.Builder.CommitSHA = strings.ToLower(evidence.CommitSHA)
		snapshot.Builder.Verification = nil
		snapshot.Builder.VerificationEvidence = nil
		for _, check := range evidence.Verification {
			command := strings.TrimSpace(check.Command)
			snapshot.Builder.Verification = append(snapshot.Builder.Verification, command)
			snapshot.Builder.VerificationEvidence = append(snapshot.Builder.VerificationEvidence, state.VerificationEvidence{Command: command, Outcome: "passed", Duration: strings.TrimSpace(check.Duration)})
		}
		snapshot.Phase = contract.PhaseIntegrating
		snapshot.ActionCursor = 0
		return o.finish(ctx, snapshot, "Builder evidence reconcile 완료", true)
	case "inspect_builder_commit":
		inspector, ok := o.deps.Worktree.(WorktreeInspector)
		if !ok {
			return ErrPendingReconcile
		}
		inspection, err := inspector.InspectCommit(ctx, snapshot.BuilderWorktree.Path, runtime.contract.BaseCommit, runtime.builderBranch, snapshot.Builder.CommitSHA)
		if err != nil || validateInspection(inspection, snapshot.Builder.CommitSHA, runtime.builderBranch, runtime.builderTask.AllowedPaths) != nil {
			return ErrPendingReconcile
		}
		if containsCredential(inspection.Patch) {
			return o.blockSensitiveInspection(ctx, snapshot)
		}
		snapshot.Builder.Branch = inspection.Branch
		snapshot.Builder.ChangedFiles = append([]string(nil), inspection.ChangedFiles...)
		snapshot.Builder.Patch = inspection.Patch
		return o.finish(ctx, snapshot, "Builder patch reconcile 완료", false)
	case "merge_verified_commit":
		locator, ok := o.deps.Worktree.(CurrentCommitLocator)
		if !ok {
			return ErrPendingReconcile
		}
		current, err := locator.CurrentCommit(ctx, snapshot.Integration.Path)
		if err != nil || current != snapshot.Builder.CommitSHA {
			return ErrPendingReconcile
		}
		snapshot.Phase = contract.PhaseReviewing
		snapshot.ActionCursor = 0
		return o.finish(ctx, snapshot, "기존 immutable merge reconcile 완료", true)
	default:
		return ErrPendingReconcile
	}
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
	integrationBranch := snapshot.Integration.Branch
	if integrationBranch == "" {
		integrationBranch = "agent/" + safeBranchPart(c.Parent.Key) + "-integration"
	}
	runtime = &runRuntime{contract: c, builderTask: builder, builderBranch: builder.Branch, integrationPath: snapshot.IntegrationPath, integrationBranch: integrationBranch, stage: stageFromSnapshot(snapshot), worktree: herdr.Worktree{Path: snapshot.BuilderWorktree.Path, WorkspaceID: snapshot.BuilderWorktree.WorkspaceID, PaneID: snapshot.BuilderWorktree.PaneID}}
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

func safeBranchPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	return strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(value)
}

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
