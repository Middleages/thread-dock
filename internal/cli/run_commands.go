package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/orchestrator"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

// RunService is the process-stable service contract behind run commands.
type RunService interface {
	Start(context.Context, string) (contract.RunID, error)
	Status(context.Context, contract.RunID) (contract.StatusView, error)
	Stop(context.Context, contract.RunID) error
	Resume(context.Context, contract.RunID) error
	Cleanup(context.Context, contract.RunID) error
}

// Dependencies are the injectable command dependencies. Contract commands do
// not require Runs, preserving the original agentctl contract interface.
type Dependencies struct {
	Runs RunService
}

var errRunServiceMissing = errors.New("실행 서비스가 구성되지 않았습니다")

// NeedsProductionDependencies keeps malformed and unknown invocations on the
// dependency-free CLI path so usage remains exit code 2 before config loading.
func NeedsProductionDependencies(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "start", "stop", "resume", "cleanup":
		return len(args) == 2 && strings.TrimSpace(args[1]) != ""
	case "status":
		_, _, ok := parseStatusArgs(args[1:])
		return ok
	default:
		return false
	}
}

// DiscoverRepositoryPath invokes Git with explicit arguments and no shell.
func DiscoverRepositoryPath(ctx context.Context, process runner.Runner, binary string) (string, error) {
	if process == nil {
		return "", errors.New("Git 실행기가 구성되지 않았습니다")
	}
	if strings.TrimSpace(binary) == "" {
		binary = "git"
	}
	result, err := process.Run(ctx, "", binary, "rev-parse", "--show-toplevel")
	if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
		return "", errors.New("Git 저장소 root를 확인할 수 없습니다")
	}
	return strings.TrimSpace(result.Stdout), nil
}

func runCommand(ctx context.Context, args []string, stdout, stderr io.Writer, runs RunService) int {
	if ctx == nil {
		ctx = context.Background()
	}

	command := args[0]
	switch command {
	case "start":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			printUsage(stderr)
			return 2
		}
		if runs == nil {
			return reportRunError(stderr, errRunServiceMissing)
		}
		id, err := runs.Start(ctx, args[1])
		if strings.TrimSpace(string(id)) != "" {
			fmt.Fprintln(stdout, id)
		}
		if err != nil {
			return reportRunError(stderr, err)
		}
		if strings.TrimSpace(string(id)) == "" {
			return reportRunError(stderr, errors.New("실행 ID가 반환되지 않았습니다"))
		}
		return 0
	case "status":
		id, jsonOutput, ok := parseStatusArgs(args[1:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		if runs == nil {
			return reportRunError(stderr, errRunServiceMissing)
		}
		view, err := runs.Status(ctx, id)
		if err != nil {
			return reportRunError(stderr, err)
		}
		if jsonOutput {
			if err := writeStatusJSON(stdout, view); err != nil {
				return reportRunError(stderr, err)
			}
			return 0
		}
		writeHumanStatus(stdout, view)
		return 0
	case "stop", "resume", "cleanup":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			printUsage(stderr)
			return 2
		}
		if runs == nil {
			return reportRunError(stderr, errRunServiceMissing)
		}
		id := contract.RunID(args[1])
		var err error
		switch command {
		case "stop":
			err = runs.Stop(ctx, id)
		case "resume":
			err = runs.Resume(ctx, id)
		case "cleanup":
			err = runs.Cleanup(ctx, id)
		}
		if err != nil {
			return reportRunError(stderr, err)
		}
		return 0
	default:
		printUsage(stderr)
		return 2
	}
}

func parseStatusArgs(args []string) (contract.RunID, bool, bool) {
	var id contract.RunID
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			if jsonOutput {
				return "", false, false
			}
			jsonOutput = true
		default:
			if strings.TrimSpace(arg) == "" || strings.HasPrefix(arg, "-") || id != "" {
				return "", false, false
			}
			id = contract.RunID(arg)
		}
	}
	return id, jsonOutput, true
}

func writeHumanStatus(stdout io.Writer, view contract.StatusView) {
	next := "없음"
	if view.NextAction != nil {
		next = view.NextAction.Label
		if strings.TrimSpace(next) == "" {
			next = view.NextAction.Kind
		}
	}
	updated := "없음"
	if !view.UpdatedAt.IsZero() {
		updated = view.UpdatedAt.Format(time.RFC3339)
	}
	fmt.Fprintf(stdout, "실행 ID: %s\n단계: %s\n요약: %s\n최근 갱신: %s\n다음 작업: %s\n", view.RunID, view.Phase, view.Summary, updated, next)
}

type statusJSON struct {
	ContractVersion int               `json:"contractVersion"`
	RunID           contract.RunID    `json:"runId"`
	Phase           contract.RunPhase `json:"phase"`
	Summary         string            `json:"summary"`
	NextAction      *nextActionJSON   `json:"nextAction,omitempty"`
	Agents          []agentJSON       `json:"agents"`
	GitHub          githubJSON        `json:"github"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type nextActionJSON struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type agentJSON struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	State   string `json:"state"`
	Summary string `json:"summary"`
}

type githubJSON struct {
	ParentIssue int    `json:"parentIssue"`
	PullRequest int    `json:"pullRequest"`
	URL         string `json:"url"`
	CI          string `json:"ci"`
}

func writeStatusJSON(stdout io.Writer, view contract.StatusView) error {
	wire := statusJSON{
		ContractVersion: view.ContractVersion,
		RunID:           view.RunID,
		Phase:           view.Phase,
		Summary:         view.Summary,
		Agents:          make([]agentJSON, 0, len(view.Agents)),
		GitHub:          githubJSON{ParentIssue: view.GitHub.ParentIssue, PullRequest: view.GitHub.PullRequest, URL: view.GitHub.URL, CI: view.GitHub.CI},
		UpdatedAt:       view.UpdatedAt,
	}
	if view.NextAction != nil {
		wire.NextAction = &nextActionJSON{Kind: view.NextAction.Kind, Label: view.NextAction.Label}
	}
	for _, agent := range view.Agents {
		wire.Agents = append(wire.Agents, agentJSON{Name: agent.Name, Role: agent.Role, State: agent.State, Summary: agent.Summary})
	}
	return json.NewEncoder(stdout).Encode(wire)
}

func reportRunError(stderr io.Writer, err error) int {
	if err == nil {
		return 1
	}
	fmt.Fprintf(stderr, "실행 명령을 처리하지 못했습니다: %v\n", err)
	return 1
}

// RunCoordinator is the subset of the orchestrator needed by the CLI.
type RunCoordinator interface {
	Start(context.Context, string) (contract.RunID, error)
	Advance(context.Context, contract.RunID) error
	Stop(context.Context, contract.RunID) error
}

// RunStateStore is the local persistence seam used for status, resume and
// cleanup. It intentionally mirrors existing state.Store methods.
type RunStateStore interface {
	Load(context.Context, contract.RunID) (state.RunSnapshot, error)
	Save(context.Context, state.RunSnapshot) error
	Append(context.Context, contract.RunID, state.Event) error
	ListRecoverable(context.Context) ([]state.RunSnapshot, error)
}

// WorktreeCleanup provides only the read-only preflight and safe, non-force
// removal needed by cleanup. It is kept outside orchestrator's public ports.
type WorktreeCleanup interface {
	Status(context.Context, string) (string, error)
	RemoveSafe(context.Context, string, string, string) error
}

// HerdrWorktreeCleanup removes an already-reconciled Herdr workspace by its
// persisted workspace ID. It is separate from Git removal because Herdr
// worktrees may live outside the Integration managed root.
type HerdrWorktreeCleanup interface {
	RemoveHerdr(context.Context, string) error
}

type HerdrWorktreeLocator interface {
	FindWorktree(context.Context, string, string, string) (herdr.Worktree, bool, error)
}

// StateRemover is deliberately injected because state.Store's existing public
// interface does not include deletion.
type StateRemover interface {
	Remove(context.Context, contract.RunID) error
}

type StateRemoverFunc func(context.Context, contract.RunID) error

func (f StateRemoverFunc) Remove(ctx context.Context, id contract.RunID) error {
	return f(ctx, id)
}

// OrchestratorRunService adapts the existing state machine to RunService.
type OrchestratorRunService struct {
	coordinator        RunCoordinator
	store              RunStateStore
	cleanup            WorktreeCleanup
	removeState        StateRemover
	now                func() time.Time
	trustedManagedRoot string
}

func NewOrchestratorRunService(coordinator RunCoordinator, store RunStateStore, cleanup WorktreeCleanup, removeState StateRemover, now func() time.Time, trustedManagedRoot string) *OrchestratorRunService {
	if now == nil {
		now = time.Now
	}
	return &OrchestratorRunService{coordinator: coordinator, store: store, cleanup: cleanup, removeState: removeState, now: now, trustedManagedRoot: trustedManagedRoot}
}

func (s *OrchestratorRunService) Start(ctx context.Context, path string) (contract.RunID, error) {
	if s == nil || s.coordinator == nil {
		return "", errRunServiceMissing
	}
	return s.coordinator.Start(ctx, path)
}

func (s *OrchestratorRunService) Status(ctx context.Context, id contract.RunID) (contract.StatusView, error) {
	if s == nil || s.store == nil {
		return contract.StatusView{}, errRunServiceMissing
	}
	snapshot, err := s.loadStatusSnapshot(ctx, id)
	if err != nil {
		return contract.StatusView{}, err
	}
	return statusView(snapshot), nil
}

func (s *OrchestratorRunService) Stop(ctx context.Context, id contract.RunID) error {
	if s == nil || s.coordinator == nil {
		return errRunServiceMissing
	}
	return s.coordinator.Stop(ctx, id)
}

func (s *OrchestratorRunService) Resume(ctx context.Context, id contract.RunID) error {
	if s == nil || s.coordinator == nil || s.store == nil {
		return errRunServiceMissing
	}
	snapshot, err := s.store.Load(ctx, id)
	if err != nil {
		return err
	}
	ready := reviewerReady(snapshot)
	if snapshot.Phase == contract.PhasePaused {
		if !resumablePhase(snapshot.PreviousPhase) {
			return errors.New("재개할 이전 실행 단계가 없습니다")
		}
		resumeAt := s.now()
		if err := s.store.Append(ctx, id, state.Event{Type: "intent", Kind: "resume", Phase: snapshot.PreviousPhase, Message: "실행 재개", At: resumeAt}); err != nil {
			return err
		}
		snapshot.Phase = snapshot.PreviousPhase
		snapshot.UpdatedAt = resumeAt
		if err := s.store.Save(ctx, snapshot); err != nil {
			return err
		}
	} else if !resumablePhase(snapshot.Phase) {
		return errors.New("진행 중인 실행만 계속할 수 있습니다")
	}
	if ready {
		return nil
	}
	// Paused and already-active runs both advance exactly once. The paused
	// snapshot keeps PreviousPhase, cursor, receipts and all external IDs.
	return s.coordinator.Advance(ctx, id)
}

func reviewerReady(snapshot state.RunSnapshot) bool {
	reviewing := snapshot.Phase == contract.PhaseReviewing ||
		(snapshot.Phase == contract.PhasePaused && snapshot.PreviousPhase == contract.PhaseReviewing)
	hasSchemaReceipt := false
	for _, verification := range snapshot.Reviewer.Verification {
		if verification == "review schema sent" {
			hasSchemaReceipt = true
			break
		}
	}
	return reviewing && snapshot.PendingAction == "" &&
		strings.TrimSpace(snapshot.Reviewer.Name) != "" &&
		snapshot.ReviewerPrompt.RequestID != "" &&
		hasSchemaReceipt &&
		snapshot.ReviewerWorktree.Path != "" &&
		snapshot.ReviewerWorktree.WorkspaceID != "" &&
		snapshot.ReviewerWorktree.PaneID != ""
}

func (s *OrchestratorRunService) Cleanup(ctx context.Context, id contract.RunID) error {
	if s == nil || s.store == nil || s.cleanup == nil || s.removeState == nil {
		return errRunServiceMissing
	}
	snapshot, err := s.store.Load(ctx, id)
	if err != nil {
		return err
	}
	cutoff := s.now().Add(-7 * 24 * time.Hour)
	if snapshot.Phase != contract.PhaseCompleted {
		return errors.New("완료된 실행만 정리할 수 있습니다")
	}
	if !snapshot.UpdatedAt.Before(cutoff) {
		return errors.New("완료 후 7일이 지나지 않은 실행은 정리할 수 없습니다")
	}
	targets, err := cleanupTargets(snapshot)
	if err != nil {
		return err
	}
	var herdrCleanup HerdrWorktreeCleanup
	var herdrLocator HerdrWorktreeLocator
	for _, target := range targets {
		if target.kind != cleanupHerdr {
			continue
		}
		var ok bool
		herdrCleanup, ok = s.cleanup.(HerdrWorktreeCleanup)
		if !ok {
			return errors.New("Herdr Worktree 제거 기능이 구성되지 않아 정리를 중단했습니다")
		}
		herdrLocator, ok = s.cleanup.(HerdrWorktreeLocator)
		if !ok {
			return errors.New("Herdr Worktree 식별 확인 기능이 구성되지 않아 정리를 중단했습니다")
		}
		break
	}
	managedRoot := filepath.Clean(s.trustedManagedRoot)
	if strings.TrimSpace(s.trustedManagedRoot) == "" {
		return errors.New("신뢰된 Worktree 루트가 구성되지 않아 정리를 중단했습니다")
	}
	if managedRoot == "." || managedRoot == string(filepath.Separator) {
		return errors.New("관리 대상 Worktree 루트가 안전하지 않습니다")
	}
	for _, target := range targets {
		if target.kind == cleanupIntegration && !pathWithinRoot(managedRoot, target.path) {
			return errors.New("관리 대상 루트 밖의 Worktree가 있어 정리를 거부했습니다")
		}
	}

	// Complete all read-only checks before the first removal. A dirty or
	// inaccessible Worktree therefore leaves every resource untouched.
	for _, target := range targets {
		if target.kind == cleanupHerdr {
			actual, found, locateErr := herdrLocator.FindWorktree(ctx, snapshot.RepositoryPath, target.path, "")
			if locateErr != nil || !found || actual.Path != target.path || actual.WorkspaceID != target.workspaceID || actual.PaneID != target.paneID {
				return errors.New("Herdr Worktree 식별자가 persisted 상태와 달라 정리를 중단했습니다")
			}
		}
		status, statusErr := s.cleanup.Status(ctx, target.path)
		if statusErr != nil {
			return fmt.Errorf("Worktree 상태를 확인할 수 없어 정리를 중단했습니다: %w", statusErr)
		}
		if strings.TrimSpace(status) != "" {
			return fmt.Errorf("Worktree가 깨끗하지 않아 정리를 거부했습니다: %s", strings.TrimSpace(status))
		}
	}
	for _, target := range targets {
		if target.kind == cleanupIntegration {
			if err := s.cleanup.RemoveSafe(ctx, snapshot.RepositoryPath, managedRoot, target.path); err != nil {
				return fmt.Errorf("Integration Worktree 정리에 실패했습니다: %w", err)
			}
			continue
		}
		if err := herdrCleanup.RemoveHerdr(ctx, target.workspaceID); err != nil {
			// Herdr adapters must not expose command output, which may contain
			// credentials or unrelated terminal diagnostics.
			return errors.New("Herdr Worktree 정리에 실패했습니다")
		}
	}
	return s.removeState.Remove(ctx, id)
}

func (s *OrchestratorRunService) loadStatusSnapshot(ctx context.Context, id contract.RunID) (state.RunSnapshot, error) {
	if id != "" {
		return s.store.Load(ctx, id)
	}
	runs, err := s.store.ListRecoverable(ctx)
	if err != nil {
		return state.RunSnapshot{}, err
	}
	if len(runs) == 0 {
		return state.RunSnapshot{}, errors.New("조회할 실행이 없습니다")
	}
	return runs[0], nil
}

func statusView(snapshot state.RunSnapshot) contract.StatusView {
	version := snapshot.ContractVersion
	if version == 0 {
		version = contract.CurrentVersion
	}
	view := contract.StatusView{
		ContractVersion: version,
		RunID:           snapshot.RunID,
		Phase:           snapshot.Phase,
		Summary:         snapshot.Summary,
		Agents:          make([]contract.AgentView, 0, 2),
		GitHub:          contract.GitHubView{ParentIssue: snapshot.ParentIssue},
		UpdatedAt:       snapshot.UpdatedAt,
	}
	if snapshot.PendingAction != "" {
		view.NextAction = &contract.NextAction{Kind: snapshot.PendingAction, Label: snapshot.Summary}
	} else if snapshot.Phase == contract.PhasePaused {
		view.NextAction = &contract.NextAction{Kind: "resume", Label: "실행 재개"}
	} else if snapshot.Phase != contract.PhaseCompleted && snapshot.Phase != contract.PhaseBlocked && snapshot.Phase != contract.PhasePaused {
		view.NextAction = &contract.NextAction{Kind: "advance", Label: "다음 단계 진행"}
	}
	if snapshot.Builder.Name != "" {
		view.Agents = append(view.Agents, contract.AgentView{Name: snapshot.Builder.Name, Role: "builder", State: "unknown", Summary: snapshot.Summary})
	}
	if snapshot.Reviewer.Name != "" {
		view.Agents = append(view.Agents, contract.AgentView{Name: snapshot.Reviewer.Name, Role: "reviewer", State: "unknown", Summary: snapshot.Summary})
	}
	return view
}

func resumablePhase(phase contract.RunPhase) bool {
	switch phase {
	case contract.PhaseRegistered, contract.PhaseAnalyzing, contract.PhaseBuilding, contract.PhaseIntegrating, contract.PhaseReviewing:
		return true
	default:
		return false
	}
}

type cleanupTargetKind uint8

const (
	cleanupIntegration cleanupTargetKind = iota
	cleanupHerdr
)

type cleanupTarget struct {
	kind        cleanupTargetKind
	path        string
	workspaceID string
	paneID      string
}

func cleanupTargets(snapshot state.RunSnapshot) ([]cleanupTarget, error) {
	if strings.TrimSpace(snapshot.IntegrationPath) == "" || strings.TrimSpace(snapshot.Integration.Path) == "" {
		return nil, errors.New("Integration Worktree 정보가 없어 정리할 수 없습니다")
	}
	targets := []cleanupTarget{{kind: cleanupIntegration, path: strings.TrimSpace(snapshot.Integration.Path)}}
	for _, work := range []state.WorktreeState{snapshot.BuilderWorktree, snapshot.ReviewerWorktree} {
		if strings.TrimSpace(work.Path) == "" && strings.TrimSpace(work.WorkspaceID) == "" && strings.TrimSpace(work.PaneID) == "" {
			continue
		}
		if strings.TrimSpace(work.Path) == "" || strings.TrimSpace(work.WorkspaceID) == "" || strings.TrimSpace(work.PaneID) == "" {
			return nil, errors.New("Builder/Reviewer Worktree 식별자가 불완전하여 정리를 거부했습니다")
		}
		targets = append(targets, cleanupTarget{kind: cleanupHerdr, path: strings.TrimSpace(work.Path), workspaceID: strings.TrimSpace(work.WorkspaceID), paneID: strings.TrimSpace(work.PaneID)})
	}
	return targets, nil
}

func pathWithinRoot(root, target string) bool {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	target, err = filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// SafeWorktreeCleanup adapts worktree.Git's existing safe removal operation
// while allowing each run to supply its persisted repository and managed root.
type SafeWorktreeCleanup struct {
	Runner       runner.Runner
	Binary       string
	HerdrBinary  string
	HerdrLocator HerdrWorktreeLocator
}

func (c SafeWorktreeCleanup) Status(ctx context.Context, path string) (string, error) {
	return worktree.New(c.Runner, c.Binary).Status(ctx, path)
}

func (c SafeWorktreeCleanup) RemoveSafe(ctx context.Context, repositoryPath, managedRoot, path string) error {
	return worktree.New(c.Runner, c.Binary, managedRoot, repositoryPath).RemoveSafe(ctx, path)
}

func (c SafeWorktreeCleanup) RemoveHerdr(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" || c.Runner == nil {
		return errors.New("Herdr Worktree 정리에 실패했습니다")
	}
	binary := strings.TrimSpace(c.HerdrBinary)
	if binary == "" {
		binary = "herdr"
	}
	result, err := c.Runner.Run(ctx, "", binary, "worktree", "remove", "--workspace", workspaceID)
	if err != nil || result.ExitCode != 0 {
		return errors.New("Herdr Worktree 정리에 실패했습니다")
	}
	return nil
}

func (c SafeWorktreeCleanup) FindWorktree(ctx context.Context, cwd, path, label string) (herdr.Worktree, bool, error) {
	if c.HerdrLocator == nil {
		return herdr.Worktree{}, false, errors.New("Herdr Worktree 식별 확인에 실패했습니다")
	}
	return c.HerdrLocator.FindWorktree(ctx, cwd, path, label)
}

// NewRunStateRemover creates the production state deletion port. It only
// removes one validated child under root/runs and never follows a symlink.
func NewRunStateRemover(root string) StateRemover {
	return StateRemoverFunc(func(ctx context.Context, id contract.RunID) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		if !safeRunID(id) {
			return errors.New("실행 상태 경로가 안전하지 않습니다")
		}
		absoluteRoot, err := filepath.Abs(filepath.Clean(root))
		if err != nil {
			return err
		}
		if strings.TrimSpace(root) == "" || isFilesystemRoot(absoluteRoot) {
			return errors.New("실행 상태 경로가 안전하지 않습니다")
		}
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			homeAbs, absErr := filepath.Abs(filepath.Clean(home))
			if absErr == nil && sameCleanPath(absoluteRoot, homeAbs) {
				return errors.New("실행 상태 경로가 안전하지 않습니다")
			}
		}
		rootInfo, err := os.Lstat(absoluteRoot)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
			return errors.New("실행 상태 디렉터리가 올바르지 않습니다")
		}
		canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
		if err != nil || !sameCleanPath(canonicalRoot, absoluteRoot) {
			return errors.New("실행 상태 경로의 심볼릭 링크는 정리할 수 없습니다")
		}
		runsRoot := filepath.Join(absoluteRoot, "runs")
		info, err := os.Lstat(runsRoot)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("실행 상태 디렉터리가 올바르지 않습니다")
		}
		canonicalRunsRoot, err := filepath.EvalSymlinks(runsRoot)
		if err != nil || !sameCleanPath(canonicalRunsRoot, runsRoot) {
			return errors.New("실행 상태 경로의 심볼릭 링크는 정리할 수 없습니다")
		}
		target := filepath.Join(runsRoot, string(id))
		relative, err := filepath.Rel(canonicalRunsRoot, target)
		if err != nil || relative == "." || relative == ".." || filepath.Base(relative) != relative || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("실행 상태 대상이 runs의 직접 자식이 아닙니다")
		}
		entry, err := os.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("실행 상태 심볼릭 링크는 정리할 수 없습니다")
		}
		return os.RemoveAll(target)
	})
}

func safeRunID(id contract.RunID) bool {
	value := string(id)
	return strings.TrimSpace(value) != "" && value != "." && value != ".." && filepath.Base(value) == value && !filepath.IsAbs(value) && !strings.ContainsAny(value, `/\\`)
}

func sameCleanPath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	return clean == filepath.VolumeName(clean)+string(filepath.Separator)
}

func contextDone(ctx context.Context) error {
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

var _ RunService = (*OrchestratorRunService)(nil)
var _ RunCoordinator = (*orchestrator.Orchestrator)(nil)
var _ RunStateStore = (*state.Store)(nil)
var _ WorktreeCleanup = SafeWorktreeCleanup{}
var _ HerdrWorktreeCleanup = SafeWorktreeCleanup{}
var _ HerdrWorktreeLocator = SafeWorktreeCleanup{}
