package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"thread-dock/internal/contract"
	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/herdr"
	"thread-dock/internal/monitor"
	"thread-dock/internal/orchestrator"
	"thread-dock/internal/registry"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
	statev2 "thread-dock/internal/state/v2"
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

// WorkflowService is the provider-neutral v2 project/work command boundary.
// It intentionally exposes only local registry, state and aggregate snapshot
// operations; provider adapters are not part of the CLI contract.
type WorkflowService interface {
	RegisterProject(context.Context, registry.Project, contractv2.Revision, contractv2.RequestID) (registry.Project, error)
	ListProjects(context.Context) ([]registry.Project, error)
	PlanWork(context.Context, string, io.Reader, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	ApproveWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	Status(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error)
	Snapshot(context.Context, time.Time) (monitor.Snapshot, error)
}

// RetirementService is the narrow command boundary for explicit session
// retirement. The bool is true only for an explicit blocked-run request.
// Keeping this separate from RunService preserves compatibility with older
// monitor and test implementations that do not know about retirement.
type RetirementService interface {
	Retire(context.Context, contract.RunID, bool) error
}

// Dependencies are the injectable command dependencies. Contract commands do
// not require Runs, preserving the original agentctl contract interface.
type Dependencies struct {
	Runs       RunService
	Workflow   WorkflowService
	Retirement RetirementService
	Confirmer  ProtectedChangeConfirmer
	Reverter   RevertRunService
}

// NeedsWorkflowDependencies is true only for the accepted v2 command
// shapes. Malformed and unknown invocations remain dependency-free so they
// can print usage without loading configuration or provider adapters.
func NeedsWorkflowDependencies(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[0] {
	case "project":
		switch args[1] {
		case "register":
			_, _, _, ok := parseWorkflowCreateArgs(args[2:])
			return ok
		case "list":
			return len(args) == 3 && args[2] == "--json"
		case "status":
			return len(args) == 4 && args[2] == "--all" && args[3] == "--json"
		}
	case "work":
		switch args[1] {
		case "plan":
			_, _, _, ok := parseWorkflowCreateArgs(args[2:])
			return ok
		case "approve":
			_, _, _, ok := parseWorkflowApproveArgs(args[2:])
			return ok
		case "pause", "resume":
			_, _, _, ok := parseWorkflowApproveArgs(args[2:])
			return ok
		case "reconcile":
			_, ok := parseWorkflowReconcileArgs(args[2:])
			return ok
		case "run":
			_, _, _, ok := parseWorkflowRunArgs(args[2:])
			return ok
		case "status":
			return len(args) == 4 && nonFlagArg(args[2]) && args[3] == "--json"
		}
	}
	return false
}

func nonFlagArg(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.HasPrefix(value, "-")
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
	case "retire":
		return validRetireArgs(args[1:])
	case "confirm":
		return len(args) == 3 && strings.TrimSpace(args[1]) != "" && args[2] == "protected-change"
	case "create-revert":
		return len(args) == 4 && strings.TrimSpace(args[1]) != "" && args[2] == "--reason" && args[3] != ""
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

func runRetire(ctx context.Context, args []string, stdout, stderr io.Writer, service RetirementService) int {
	if !validRetireArgs(args) {
		printUsage(stderr)
		return 2
	}
	if service == nil {
		return reportGenericRunError(stderr)
	}
	blocked := len(args) == 2
	if err := service.Retire(ctx, contract.RunID(args[0]), blocked); err != nil {
		if errors.Is(err, errBlockedRetirementRequiresGuard) {
			fmt.Fprintln(stderr, errBlockedRetirementRequiresGuard)
			return 1
		}
		return reportGenericRunError(stderr)
	}
	return 0
}

func validRetireArgs(args []string) bool {
	if len(args) == 1 {
		return strings.TrimSpace(args[0]) != "" && !strings.HasPrefix(args[0], "-")
	}
	return len(args) == 2 && strings.TrimSpace(args[0]) != "" && !strings.HasPrefix(args[0], "-") && args[1] == "--blocked"
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
	for _, agent := range view.Agents {
		if strings.TrimSpace(agent.Lifecycle) != "" {
			fmt.Fprintf(stdout, "세션 %s: %s\n", agent.Name, agent.Lifecycle)
		}
	}
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
	Name      string `json:"name"`
	Role      string `json:"role"`
	State     string `json:"state"`
	Summary   string `json:"summary"`
	Lifecycle string `json:"lifecycle"`
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
		wire.Agents = append(wire.Agents, agentJSON{Name: agent.Name, Role: agent.Role, State: agent.State, Summary: agent.Summary, Lifecycle: agent.Lifecycle})
	}
	return json.NewEncoder(stdout).Encode(wire)
}

func reportRunError(stderr io.Writer, err error) int {
	if err == nil {
		return 1
	}
	// Provider adapters and filesystem errors may contain command output or
	// credential-shaped values. Keep the historical useful detail for ordinary
	// policy errors, but replace provider/raw diagnostics at this boundary.
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"provider", "herdr", "stderr", "stdout", "token", "secret", "password", "credential", "authorization", "api_key", "private key"} {
		if strings.Contains(message, marker) {
			fmt.Fprintln(stderr, "실행 명령을 처리하지 못했습니다. 상태와 운영 로그를 확인하십시오.")
			return 1
		}
	}
	fmt.Fprintf(stderr, "실행 명령을 처리하지 못했습니다: %v\n", err)
	return 1
}

func reportGenericRunError(stderr io.Writer) int {
	fmt.Fprintln(stderr, "실행 명령을 처리하지 못했습니다. 상태와 운영 로그를 확인하십시오.")
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

// RetiredWorktreeCleanup removes a previously closed Workspace's checkout by
// verified Git path. The target carries the durable proof, so callers never
// need to reconstruct a branch or HEAD from provider output.
type RetiredWorktreeCleanup interface {
	RemoveRetired(context.Context, string, string, state.RetirementTarget) error
}

// RetiredWorktreeInspector is the read-only half of RetiredWorktreeCleanup.
// It lets Cleanup complete every proof check before issuing its first remove.
// Adapters that do not expose this optional seam still have their durable
// proof fields validated by the service; the production adapter implements
// this with Git's strict InspectRetirementTarget operation.
type RetiredWorktreeInspector interface {
	InspectRetired(context.Context, string, string, state.RetirementTarget) error
}

type herdrWorktreeRootProvider interface {
	TrustedHerdrWorktreeRoot() string
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
	trustedHerdrRoot   string
}

// RetirementRuntime captures the immutable repository and trusted cleanup
// roots used by one retirement invocation. It is intentionally narrow so
// production wiring tests can verify that retirement is snapshot-based without
// invoking Herdr or Git mutations.
type RetirementRuntime struct {
	RepositoryPath string
	ManagedRoot    string
	HerdrRoot      string
	Composite      bool
}

func (s *OrchestratorRunService) RetirementRuntime(ctx context.Context, id contract.RunID) (RetirementRuntime, error) {
	if s == nil || s.store == nil {
		return RetirementRuntime{}, errRunServiceMissing
	}
	snapshot, err := s.store.Load(ctx, id)
	if err != nil {
		return RetirementRuntime{}, err
	}
	herdrRoot := s.trustedHerdrRoot
	composite := false
	if safe, ok := s.cleanup.(SafeWorktreeCleanup); ok {
		if herdrRoot == "" {
			herdrRoot = safe.HerdrWorktreeRoot
		}
		_, composite = safe.RetirementAdapter.(*worktree.CompositeRetirementInspector)
	}
	return RetirementRuntime{RepositoryPath: snapshot.RepositoryPath, ManagedRoot: s.trustedManagedRoot, HerdrRoot: herdrRoot, Composite: composite}, nil
}

func NewOrchestratorRunService(coordinator RunCoordinator, store RunStateStore, cleanup WorktreeCleanup, removeState StateRemover, now func() time.Time, trustedManagedRoot string, trustedHerdrRoot ...string) *OrchestratorRunService {
	if now == nil {
		now = time.Now
	}
	herdrRoot := ""
	if len(trustedHerdrRoot) > 0 {
		herdrRoot = trustedHerdrRoot[0]
	}
	return &OrchestratorRunService{coordinator: coordinator, store: store, cleanup: cleanup, removeState: removeState, now: now, trustedManagedRoot: trustedManagedRoot, trustedHerdrRoot: herdrRoot}
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
	// A schema receipt is the terminal handoff for the historical Single-run
	// reviewer. Parallel runs must advance so the coordinator can collect
	// strict ReviewEvidence and continue through PR, CI, and merge gates.
	if snapshot.Strategy == "parallel" {
		return false
	}
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
	if strings.TrimSpace(snapshot.RepositoryPath) == "" {
		return errors.New("repository path가 없어 정리를 중단했습니다")
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
	var retiredCleanup RetiredWorktreeCleanup
	var retiredInspector RetiredWorktreeInspector
	for _, target := range targets {
		var ok bool
		switch target.kind {
		case cleanupHerdr:
			herdrCleanup, ok = s.cleanup.(HerdrWorktreeCleanup)
			if !ok {
				return errors.New("Herdr Worktree 제거 기능이 구성되지 않아 정리를 중단했습니다")
			}
			herdrLocator, ok = s.cleanup.(HerdrWorktreeLocator)
			if !ok {
				return errors.New("Herdr Worktree 식별 확인 기능이 구성되지 않아 정리를 중단했습니다")
			}
		case cleanupRetired:
			retiredCleanup, ok = s.cleanup.(RetiredWorktreeCleanup)
			if !ok {
				return errors.New("retired Worktree 제거 기능이 구성되지 않아 정리를 중단했습니다")
			}
			retiredInspector, ok = s.cleanup.(RetiredWorktreeInspector)
			if !ok {
				return errors.New("retired Worktree 식별 확인 기능이 구성되지 않아 정리를 중단했습니다")
			}
		}
	}
	managedRoot := filepath.Clean(s.trustedManagedRoot)
	if strings.TrimSpace(s.trustedManagedRoot) == "" {
		return errors.New("신뢰된 Worktree 루트가 구성되지 않아 정리를 중단했습니다")
	}
	if managedRoot == "." || managedRoot == string(filepath.Separator) {
		return errors.New("관리 대상 Worktree 루트가 안전하지 않습니다")
	}
	herdrRoot := s.trustedHerdrRoot
	if strings.TrimSpace(herdrRoot) == "" {
		if provider, ok := s.cleanup.(herdrWorktreeRootProvider); ok {
			herdrRoot = provider.TrustedHerdrWorktreeRoot()
		}
	}
	for _, target := range targets {
		if err := validateCleanupTargetContainment(snapshot, target, managedRoot, herdrRoot); err != nil {
			return err
		}
		if target.kind == cleanupRetired {
			if err := validateRetiredProof(target.retirement, target.path); err != nil {
				return err
			}
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
		if target.kind == cleanupRetired {
			proofRoot := cleanupProofRoot(snapshot, target, managedRoot, herdrRoot)
			if err := retiredInspector.InspectRetired(ctx, snapshot.RepositoryPath, proofRoot, target.retirement); err != nil {
				return errors.New("retired Worktree Git proof가 persisted 상태와 달라 정리를 중단했습니다")
			}
			continue
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
		if target.kind == cleanupRetired {
			proofRoot := cleanupProofRoot(snapshot, target, managedRoot, herdrRoot)
			if err := retiredCleanup.RemoveRetired(ctx, snapshot.RepositoryPath, proofRoot, target.retirement); err != nil {
				return errors.New("retired Worktree 정리에 실패했습니다")
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

func validateRetiredProof(proof state.RetirementTarget, path string) error {
	if proof.Status != "retired" || strings.TrimSpace(proof.Path) == "" || !sameCleanPath(proof.Path, path) || !filepath.IsAbs(proof.Path) || filepath.Clean(proof.Path) != proof.Path || strings.TrimSpace(proof.RepositoryCommonDir) == "" || !filepath.IsAbs(proof.RepositoryCommonDir) || filepath.Clean(proof.RepositoryCommonDir) != proof.RepositoryCommonDir || strings.TrimSpace(proof.Branch) == "" || strings.TrimSpace(proof.HeadSHA) == "" {
		return errors.New("retired Worktree의 Git proof가 불완전하여 정리를 거부했습니다")
	}
	return nil
}

func validateCleanupTargetContainment(snapshot state.RunSnapshot, target cleanupTarget, managedRoot, herdrRoot string) error {
	if target.kind == cleanupIntegration {
		if !strictPathWithinRoot(managedRoot, target.path) {
			return errors.New("관리 대상 루트 밖의 Worktree가 있어 정리를 거부했습니다")
		}
	} else if target.kind == cleanupHerdr {
		sharedReviewer := target.role == "reviewer" && sameCleanPath(target.path, snapshot.Integration.Path)
		if sharedReviewer {
			if !sameCleanPath(target.path, snapshot.Integration.Path) {
				return errors.New("Reviewer Integration 경로가 persisted 상태와 달라 정리를 거부했습니다")
			}
			if !strictPathWithinRoot(managedRoot, target.path) {
				return errors.New("shared Reviewer/Integration Worktree가 관리 대상 루트 밖에 있어 정리를 거부했습니다")
			}
			integrationIdentity := cleanupTarget{branch: strings.TrimSpace(snapshot.Integration.Branch), headSHA: cleanupIntegrationHeadSHA(snapshot)}
			if (snapshot.Strategy == "parallel" || integrationIdentity.branch != "") && (integrationIdentity.branch == "" || target.branch == "") {
				return errors.New("shared Reviewer/Integration Worktree branch identity가 불완전하여 정리를 거부했습니다")
			}
			if (snapshot.Strategy == "parallel" && integrationIdentity.headSHA == "") || (integrationIdentity.headSHA != "" && target.headSHA == "") {
				return errors.New("shared Reviewer/Integration Worktree HEAD identity가 불완전하여 정리를 거부했습니다")
			}
			if !compatibleBranchHead(integrationIdentity, target) {
				return errors.New("shared Reviewer/Integration Worktree의 branch 또는 HEAD가 달라 정리를 거부했습니다")
			}
		} else {
			if strings.TrimSpace(herdrRoot) == "" || !strictPathWithinRoot(herdrRoot, target.path) {
				return errors.New("Builder/Reviewer Worktree가 관리 대상 Herdr 루트 밖에 있어 정리를 거부했습니다")
			}
		}
	} else {
		proofRoot := cleanupProofRoot(snapshot, target, managedRoot, herdrRoot)
		if strings.TrimSpace(proofRoot) == "" || !strictPathWithinRoot(proofRoot, target.path) {
			return errors.New("retired Worktree가 관리 대상 루트 밖에 있어 정리를 거부했습니다")
		}
	}
	if sameCleanPath(target.path, snapshot.RepositoryPath) || isFilesystemRoot(filepath.Clean(target.path)) {
		return errors.New("repository 또는 filesystem root를 Worktree로 정리할 수 없습니다")
	}
	if home, err := os.UserHomeDir(); err == nil && sameCleanPath(target.path, home) {
		return errors.New("home 디렉터리를 Worktree로 정리할 수 없습니다")
	}
	return nil
}

func cleanupProofRoot(snapshot state.RunSnapshot, target cleanupTarget, managedRoot, herdrRoot string) string {
	if (target.role == "reviewer" || target.role == "integration") && sameCleanPath(target.path, snapshot.Integration.Path) {
		return managedRoot
	}
	return herdrRoot
}

// strictPathWithinRoot uses canonical paths when the filesystem entries are
// available, catching symlink escapes. Test doubles may use virtual paths;
// for those, the lexical containment check preserves the adapter seam.
func strictPathWithinRoot(root, target string) bool {
	cleanRoot := filepath.Clean(root)
	if cleanRoot == "." || cleanRoot == string(filepath.Separator) {
		return false
	}
	if !pathWithinRoot(root, target) {
		return false
	}
	rootAbs, rootErr := filepath.Abs(filepath.Clean(root))
	targetAbs, targetErr := filepath.Abs(filepath.Clean(target))
	if rootErr != nil || targetErr != nil {
		return false
	}
	if _, err := os.Stat(rootAbs); err != nil {
		return true
	}
	canonicalRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return false
	}
	if filepath.Clean(canonicalRoot) != filepath.Clean(rootAbs) {
		return false
	}
	if _, err := os.Stat(targetAbs); err != nil {
		// The adapter's read-only status/proof operation will report a missing
		// checkout. Keep virtual test doubles on the same lexical boundary while
		// still canonicalizing every existing target for symlink escape checks.
		return pathWithinRoot(canonicalRoot, targetAbs)
	}
	canonicalTarget, err := filepath.EvalSymlinks(targetAbs)
	if err != nil {
		return false
	}
	return pathWithinRoot(canonicalRoot, canonicalTarget)
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
		GitHub:          contract.GitHubView{ParentIssue: snapshot.ParentIssue, PullRequest: snapshot.PullRequest, URL: snapshot.PullRequestURL, CI: snapshot.CIState},
		UpdatedAt:       snapshot.UpdatedAt,
	}
	if snapshot.PendingAction != "" {
		view.NextAction = &contract.NextAction{Kind: snapshot.PendingAction, Label: snapshot.Summary}
	} else if snapshot.Phase == contract.PhasePaused {
		view.NextAction = &contract.NextAction{Kind: "resume", Label: "실행 재개"}
	} else if snapshot.Phase != contract.PhaseCompleted && snapshot.Phase != contract.PhaseBlocked && snapshot.Phase != contract.PhasePaused {
		view.NextAction = &contract.NextAction{Kind: "advance", Label: "다음 단계 진행"}
	}
	seenAgents := make(map[string]bool)
	if snapshot.Builder.Name != "" {
		view.Agents = append(view.Agents, contract.AgentView{Name: snapshot.Builder.Name, Role: "builder", State: "unknown", Summary: snapshot.Summary, Lifecycle: sessionLifecycleForTarget(snapshot, "builder", "", snapshot.BuilderWorktree.Path)})
		seenAgents[snapshot.Builder.Name] = true
	}
	if snapshot.Reviewer.Name != "" {
		view.Agents = append(view.Agents, contract.AgentView{Name: snapshot.Reviewer.Name, Role: "reviewer", State: "unknown", Summary: snapshot.Summary, Lifecycle: sessionLifecycleForTarget(snapshot, "reviewer", "", snapshot.ReviewerWorktree.Path)})
		seenAgents[snapshot.Reviewer.Name] = true
	}
	taskIDs := append([]string(nil), snapshot.TaskOrder...)
	seenTaskIDs := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		seenTaskIDs[id] = true
	}
	remainingTaskIDs := make([]string, 0, len(snapshot.Tasks))
	for id := range snapshot.Tasks {
		if !seenTaskIDs[id] {
			remainingTaskIDs = append(remainingTaskIDs, id)
		}
	}
	sort.Strings(remainingTaskIDs)
	taskIDs = append(taskIDs, remainingTaskIDs...)
	for _, id := range taskIDs {
		task, ok := snapshot.Tasks[id]
		if !ok || task.Agent.Name == "" || seenAgents[task.Agent.Name] {
			continue
		}
		role := "builder"
		if task.Agent.Name == snapshot.Reviewer.Name {
			role = "reviewer"
		}
		stateName := task.State
		if stateName == "" {
			stateName = "unknown"
		}
		view.Agents = append(view.Agents, contract.AgentView{Name: task.Agent.Name, Role: role, State: stateName, Summary: snapshot.Summary, Lifecycle: sessionLifecycleForTarget(snapshot, role, id, task.Worktree.Path)})
		seenAgents[task.Agent.Name] = true
	}
	return view
}

func sessionLifecycleForTarget(snapshot state.RunSnapshot, role, taskID, path string) string {
	fallback := sessionLifecycle(snapshot)
	for _, target := range snapshot.Retirement.Targets {
		if target.Role != role {
			continue
		}
		if taskID != "" && target.TaskID != taskID {
			continue
		}
		if taskID == "" && path != "" && !sameCleanPath(target.Path, path) {
			continue
		}
		if target.Status == "retired" {
			return "retired"
		}
		return "retiring"
	}
	return fallback
}

func resumablePhase(phase contract.RunPhase) bool {
	switch phase {
	case contract.PhaseRegistered, contract.PhaseAnalyzing, contract.PhaseBuilding, contract.PhaseIntegrating, contract.PhaseReviewing:
		return true
	case contract.PhaseCI, contract.PhaseMerging, contract.PhaseRetiring:
		return true
	default:
		return false
	}
}

type cleanupTargetKind uint8

const (
	cleanupIntegration cleanupTargetKind = iota
	cleanupHerdr
	cleanupRetired
)

type cleanupTarget struct {
	kind        cleanupTargetKind
	path        string
	workspaceID string
	paneID      string
	role        string
	taskID      string
	branch      string
	headSHA     string
	retirement  state.RetirementTarget
}

func cleanupTargets(snapshot state.RunSnapshot) ([]cleanupTarget, error) {
	integrationPath := strings.TrimSpace(snapshot.Integration.Path)
	if strings.TrimSpace(snapshot.IntegrationPath) == "" || integrationPath == "" {
		return nil, errors.New("Integration Worktree 정보가 없어 정리할 수 없습니다")
	}
	if !sameCleanPath(strings.TrimSpace(snapshot.IntegrationPath), integrationPath) {
		return nil, errors.New("Integration Worktree 경로가 persisted 상태와 달라 정리를 거부했습니다")
	}
	targets := []cleanupTarget{{kind: cleanupIntegration, path: integrationPath, role: "integration", branch: strings.TrimSpace(snapshot.Integration.Branch), headSHA: cleanupIntegrationHeadSHA(snapshot)}}

	// Keep the historical top-level Builder/Reviewer identities first. These
	// fields are still populated by Single runs and by the parallel Reviewer.
	if err := addCleanupWorktreeTarget(&targets, cleanupTargetForWorktree(snapshot, snapshot.BuilderWorktree, "builder", ""), false); err != nil {
		return nil, err
	}
	if err := addCleanupWorktreeTarget(&targets, cleanupTargetForWorktree(snapshot, snapshot.ReviewerWorktree, "reviewer", ""), true); err != nil {
		return nil, err
	}

	// Parallel task worktrees are durable under Tasks, not the legacy Builder
	// fields. Follow TaskOrder and then append any map-only IDs in sorted order
	// so cleanup never drops a Builder or depends on map iteration order.
	taskIDs := append([]string(nil), snapshot.TaskOrder...)
	seenTaskIDs := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		seenTaskIDs[id] = true
	}
	remainingTaskIDs := make([]string, 0, len(snapshot.Tasks))
	for id := range snapshot.Tasks {
		if !seenTaskIDs[id] {
			remainingTaskIDs = append(remainingTaskIDs, id)
		}
	}
	sort.Strings(remainingTaskIDs)
	taskIDs = append(taskIDs, remainingTaskIDs...)
	for _, id := range taskIDs {
		task, ok := snapshot.Tasks[id]
		if !ok {
			return nil, fmt.Errorf("parallel task %q is missing durable state", id)
		}
		if isEmptyTaskRunState(task) && snapshot.Strategy != "parallel" && len(snapshot.Tasks) == 0 {
			continue
		}
		if strings.TrimSpace(task.Agent.Name) == "" || !hasAnyWorktreeIdentity(task.Worktree) {
			return nil, fmt.Errorf("parallel Builder target %q is incomplete", id)
		}
		if err := addCleanupWorktreeTarget(&targets, cleanupTargetForWorktree(snapshot, task.Worktree, "builder", id), false); err != nil {
			return nil, err
		}
	}

	// Retirement proof is durable state. If a historical snapshot retained a
	// target after its top-level or task fields were compacted, include it by
	// exact proof rather than reconstructing identity from prose or a path.
	for _, durable := range snapshot.Retirement.Targets {
		if strings.TrimSpace(durable.Path) == "" {
			return nil, errors.New("retirement target is missing a Worktree path")
		}
		target, err := cleanupTargetForRetirement(durable)
		if err != nil {
			return nil, err
		}
		allowShared := durable.Role == "reviewer" && sameCleanPath(durable.Path, integrationPath)
		if err := addCleanupWorktreeTarget(&targets, target, allowShared); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func cleanupTargetForWorktree(snapshot state.RunSnapshot, work state.WorktreeState, role, taskID string) cleanupTarget {
	path := strings.TrimSpace(work.Path)
	if path == "" && strings.TrimSpace(work.WorkspaceID) == "" && strings.TrimSpace(work.PaneID) == "" {
		return cleanupTarget{}
	}
	for _, durable := range snapshot.Retirement.Targets {
		if durable.Status != "retired" || durable.Role != role || !sameCleanPath(durable.Path, path) {
			continue
		}
		if taskID != "" && durable.TaskID != "" && durable.TaskID != taskID {
			continue
		}
		return cleanupTarget{kind: cleanupRetired, path: path, workspaceID: strings.TrimSpace(work.WorkspaceID), paneID: strings.TrimSpace(work.PaneID), role: role, taskID: taskID, branch: strings.TrimSpace(durable.Branch), headSHA: strings.TrimSpace(durable.HeadSHA), retirement: durable}
	}
	target := cleanupTarget{kind: cleanupHerdr, path: path, workspaceID: strings.TrimSpace(work.WorkspaceID), paneID: strings.TrimSpace(work.PaneID), role: role, taskID: taskID, branch: strings.TrimSpace(work.Branch)}
	if role == "reviewer" && sameCleanPath(path, snapshot.Integration.Path) {
		target.headSHA = cleanupIntegrationHeadSHA(snapshot)
	}
	return target
}

func cleanupIntegrationHeadSHA(snapshot state.RunSnapshot) string {
	if strings.TrimSpace(snapshot.FinalSHA) != "" {
		return strings.TrimSpace(snapshot.FinalSHA)
	}
	return strings.TrimSpace(snapshot.IntegrationSHA)
}

func cleanupTargetForRetirement(durable state.RetirementTarget) (cleanupTarget, error) {
	if durable.Status == "retired" {
		if err := validateRetiredProof(durable, durable.Path); err != nil {
			return cleanupTarget{}, err
		}
		return cleanupTarget{kind: cleanupRetired, path: strings.TrimSpace(durable.Path), workspaceID: strings.TrimSpace(durable.WorkspaceID), paneID: strings.TrimSpace(durable.PaneID), role: durable.Role, taskID: durable.TaskID, branch: strings.TrimSpace(durable.Branch), headSHA: strings.TrimSpace(durable.HeadSHA), retirement: durable}, nil
	}
	if durable.Status != "active" && durable.Status != "pending" && durable.Status != "workspace_observed" && durable.Status != "git_proven" {
		return cleanupTarget{}, errors.New("retirement target status is not removable")
	}
	if strings.TrimSpace(durable.WorkspaceID) == "" || strings.TrimSpace(durable.PaneID) == "" {
		return cleanupTarget{}, errors.New("active retirement target identity is incomplete")
	}
	return cleanupTarget{kind: cleanupHerdr, path: strings.TrimSpace(durable.Path), workspaceID: strings.TrimSpace(durable.WorkspaceID), paneID: strings.TrimSpace(durable.PaneID), role: durable.Role, taskID: durable.TaskID, branch: strings.TrimSpace(durable.Branch), headSHA: strings.TrimSpace(durable.HeadSHA)}, nil
}

func addCleanupWorktreeTarget(targets *[]cleanupTarget, target cleanupTarget, allowSharedReviewer bool) error {
	if target.path == "" && target.workspaceID == "" && target.paneID == "" {
		return nil
	}
	if target.path == "" || (target.kind != cleanupRetired && (target.workspaceID == "" || target.paneID == "")) {
		return errors.New("Builder/Reviewer Worktree 식별자가 불완전하여 정리를 거부했습니다")
	}
	for index, existing := range *targets {
		if !sameCleanPath(existing.path, target.path) {
			continue
		}
		if allowSharedReviewer && target.role == "reviewer" && existing.role == "integration" {
			if err := compatibleSharedOwnership(existing, target); err != nil {
				return err
			}
			(*targets)[index] = target
			return nil
		}
		if existing.role == "reviewer" && target.role == "integration" {
			if err := compatibleSharedOwnership(existing, target); err != nil {
				return err
			}
			return nil
		}
		if sameCleanupOwnership(existing, target) {
			return nil
		}
		return errors.New("Worktree 경로가 서로 다른 소유자 또는 retirement 상태와 충돌하여 정리를 거부했습니다")
	}
	*targets = append(*targets, target)
	return nil
}

func isEmptyTaskRunState(task state.TaskRunState) bool {
	return strings.TrimSpace(task.Agent.Name) == "" && strings.TrimSpace(task.Worktree.Path) == "" && strings.TrimSpace(task.Worktree.WorkspaceID) == "" && strings.TrimSpace(task.Worktree.PaneID) == ""
}

func hasAnyWorktreeIdentity(work state.WorktreeState) bool {
	return strings.TrimSpace(work.Path) != "" || strings.TrimSpace(work.WorkspaceID) != "" || strings.TrimSpace(work.PaneID) != ""
}

func sameCleanupOwnership(left, right cleanupTarget) bool {
	if left.kind != right.kind || left.role != right.role || left.taskID != right.taskID || !compatibleIdentity(left.workspaceID, right.workspaceID) || !compatibleIdentity(left.paneID, right.paneID) || !compatibleBranchHead(left, right) {
		return false
	}
	if left.kind == cleanupRetired {
		return sameRetirementProof(left.retirement, right.retirement)
	}
	return true
}

func compatibleIdentity(left, right string) bool {
	return left == "" || right == "" || left == right
}

func compatibleSharedOwnership(left, right cleanupTarget) error {
	if left.workspaceID != "" && right.workspaceID != "" && left.workspaceID != right.workspaceID {
		return errors.New("shared Reviewer/Integration Worktree가 서로 다른 Workspace ID를 가져 정리를 거부했습니다")
	}
	if !compatibleBranchHead(left, right) {
		return errors.New("shared Reviewer/Integration Worktree의 branch 또는 HEAD가 달라 정리를 거부했습니다")
	}
	if left.kind == cleanupRetired && right.kind == cleanupRetired && !sameRetirementProof(left.retirement, right.retirement) {
		return errors.New("shared Reviewer/Integration Worktree의 Git proof가 달라 정리를 거부했습니다")
	}
	return nil
}

func sameRetirementProof(left, right state.RetirementTarget) bool {
	return left.Path == right.Path && left.Branch == right.Branch && left.HeadSHA == right.HeadSHA && left.RepositoryCommonDir == right.RepositoryCommonDir
}

func compatibleBranchHead(left, right cleanupTarget) bool {
	return (left.branch == "" || right.branch == "" || left.branch == right.branch) && (left.headSHA == "" || right.headSHA == "" || left.headSHA == right.headSHA)
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
	Runner            runner.Runner
	Binary            string
	HerdrBinary       string
	HerdrWorktreeRoot string
	RetirementAdapter interface {
		InspectRetirementTarget(context.Context, string, string, string, string) (worktree.RetirementProof, error)
		RemoveRetired(context.Context, string, string, worktree.RetirementProof) error
	}
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

// InspectRetired performs the complete read-only Git proof immediately before
// cleanup's removal phase. It intentionally accepts the durable target as a
// value so callers cannot substitute a provider-derived path or branch.
func (c SafeWorktreeCleanup) InspectRetired(ctx context.Context, repositoryPath, herdrRoot string, target state.RetirementTarget) error {
	if c.Runner == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(herdrRoot) == "" {
		return errors.New("retired Worktree Git proof를 확인할 수 없습니다")
	}
	if strings.TrimSpace(target.RepositoryCommonDir) == "" || strings.TrimSpace(target.Path) == "" || strings.TrimSpace(target.Branch) == "" || strings.TrimSpace(target.HeadSHA) == "" {
		return errors.New("retired Worktree Git proof가 불완전합니다")
	}
	if c.RetirementAdapter != nil {
		proof, err := c.RetirementAdapter.InspectRetirementTarget(ctx, repositoryPath, target.Path, target.Branch, target.HeadSHA)
		if err != nil || proof.RepositoryCommonDir != target.RepositoryCommonDir || proof.Path != target.Path || proof.Branch != target.Branch || proof.HeadSHA != target.HeadSHA {
			return errors.New("retired Worktree Git proof가 일치하지 않습니다")
		}
		return nil
	}
	git := worktree.New(c.Runner, c.Binary, herdrRoot, repositoryPath)
	proof, err := git.InspectRetirementTarget(ctx, repositoryPath, target.Path, target.Branch, target.HeadSHA)
	if err != nil || proof.RepositoryCommonDir != target.RepositoryCommonDir || proof.Path != target.Path || proof.Branch != target.Branch || proof.HeadSHA != target.HeadSHA {
		return errors.New("retired Worktree Git proof가 일치하지 않습니다")
	}
	return nil
}

// RemoveRetired removes only a verified retired checkout. Unlike Herdr
// removal it never passes a Workspace ID; Herdr has forgotten that identity
// by the time this cleanup path is reached.
func (c SafeWorktreeCleanup) RemoveRetired(ctx context.Context, repositoryPath, herdrRoot string, target state.RetirementTarget) error {
	if c.Runner == nil || strings.TrimSpace(repositoryPath) == "" || strings.TrimSpace(herdrRoot) == "" {
		return errors.New("retired Worktree 정리에 실패했습니다")
	}
	if err := validateRetiredProof(target, target.Path); err != nil {
		return err
	}
	proof := worktree.RetirementProof{RepositoryCommonDir: target.RepositoryCommonDir, Path: target.Path, Branch: target.Branch, HeadSHA: target.HeadSHA}
	if c.RetirementAdapter != nil {
		return c.RetirementAdapter.RemoveRetired(ctx, repositoryPath, herdrRoot, proof)
	}
	git := worktree.New(c.Runner, c.Binary, herdrRoot, repositoryPath)
	return git.RemoveRetired(ctx, repositoryPath, herdrRoot, proof)
}

func (c SafeWorktreeCleanup) TrustedHerdrWorktreeRoot() string { return c.HerdrWorktreeRoot }

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
var _ RetiredWorktreeCleanup = SafeWorktreeCleanup{}
var _ RetiredWorktreeInspector = SafeWorktreeCleanup{}
