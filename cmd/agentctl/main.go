package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"thread-dock/internal/cli"
	"thread-dock/internal/config"
	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/orchestrator"
	"thread-dock/internal/registry"
	"thread-dock/internal/revert"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/workflow"
	"thread-dock/internal/worktree"
)

func main() {
	args := os.Args[1:]
	if cli.NeedsWorkflowDependencies(args) {
		deps, err := productionWorkflowDependencies()
		if err != nil {
			fmt.Fprintln(os.Stderr, "agentctl 워크플로 저장소를 준비하지 못했습니다")
			os.Exit(1)
		}
		os.Exit(cli.RunWithDependencies(context.Background(), args, os.Stdout, os.Stderr, deps))
	}
	// These commands and malformed/unknown invocations are deliberately
	// dependency-free. This preserves usage and the foundation contract
	// interface on a machine not configured for GHES.
	if !cli.NeedsProductionDependencies(args) {
		os.Exit(cli.Run(context.Background(), args, os.Stdout, os.Stderr))
	}

	deps, err := productionDependencies(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentctl 설정을 준비하지 못했습니다: %v\n", err)
		os.Exit(1)
	}
	os.Exit(cli.RunWithDependencies(context.Background(), args, os.Stdout, os.Stderr, deps))
}

// productionWorkflowDependencies wires only the provider-neutral v2 stores
// and workflow service. Project/work commands must remain usable without a
// legacy config file, Git checkout, credentials, or runtime adapters.
func productionWorkflowDependencies() (cli.Dependencies, error) {
	root := os.Getenv("THREADDOCK_STATE_DIR")
	if root == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return cli.Dependencies{}, fmt.Errorf("기본 상태 디렉터리를 확인할 수 없습니다: %w", err)
		}
		root = filepath.Join(configDir, "threaddock")
	}
	projects := registry.NewStore(root)
	works := statev2.NewStore(root)
	return cli.Dependencies{Workflow: workflow.New(projects, works)}, nil
}

func productionDependencies(args []string) (cli.Dependencies, error) {
	configPath := os.Getenv("THREADDOCK_CONFIG")
	if configPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return cli.Dependencies{}, fmt.Errorf("기본 설정 디렉터리를 확인할 수 없습니다: %w", err)
		}
		configPath = filepath.Join(configDir, "threaddock", "config.json")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return cli.Dependencies{}, fmt.Errorf("설정 파일 %s를 확인하십시오: %w", configPath, err)
	}
	token := os.Getenv("THREADDOCK_GH_TOKEN")
	if requiresGHESCredential(args) && token == "" {
		return cli.Dependencies{}, errors.New("THREADDOCK_GH_TOKEN이 없습니다. GHES 자격 증명을 agentctl 프로세스 환경에 설정한 뒤 다시 실행하십시오")
	}

	process := runner.OSRunner{}
	store := state.NewStore(cfg.StateDir)
	repositoryPath, err := repositoryPathForCommand(context.Background(), args, process, cfg.GitBinary, cli.DiscoverRepositoryPath)
	if err != nil {
		return cli.Dependencies{}, err
	}
	repositoryPath, err = repositoryPathForPersistedCommand(context.Background(), args, store, repositoryPath)
	if err != nil {
		return cli.Dependencies{}, err
	}
	worktreeRoot := filepath.Join(cfg.StateDir, "worktrees")
	// Configure both roots so managed integration/revert operations can prove
	// ownership before touching a checkout.
	git := worktree.New(process, cfg.GitBinary, worktreeRoot, repositoryPath)
	retirementGit := worktree.NewCompositeRetirementInspector(
		worktree.New(process, cfg.GitBinary, worktreeRoot, repositoryPath),
		worktree.New(process, cfg.GitBinary, cfg.HerdrWorktreeRoot, repositoryPath),
		worktreeRoot,
		cfg.HerdrWorktreeRoot,
	)
	ghes := github.NewRESTClient(cfg.APIBase, token, cfg.APIVersion, nil)
	herdrClient := herdr.NewCLI(process, cfg.HerdrBinary)
	builderOpenCodeAgent, reviewerOpenCodeAgent := roleAgentRouting(cfg)
	orch := orchestrator.NewAuto(orchestrator.Dependencies{
		Store:                       store,
		GitHub:                      ghes,
		Herdr:                       herdrClient,
		Git:                         git,
		Worktree:                    git,
		RepositoryPath:              repositoryPath,
		WorktreeRoot:                worktreeRoot,
		ProjectAutomationEnabled:    cfg.ProjectAutomationEnabled,
		Project:                     github.ProjectRef{ID: cfg.ProjectID, StatusFieldID: cfg.ProjectStatusFieldID, StatusOptions: cfg.ProjectStatusOptions},
		WorkingWait:                 cfg.WorkingWait,
		RecoveryLimit:               cfg.RecoveryLimit,
		Remote:                      "origin",
		WorkspaceReader:             herdrClient,
		WorkspaceCloser:             herdrClient,
		RetirementGitInspector:      retirementGit,
		AutoRetireCompletedSessions: cfg.AutoRetireCompletedSessions,
		HerdrWorktreeRoot:           cfg.HerdrWorktreeRoot,
		BuilderOpenCodeAgent:        builderOpenCodeAgent,
		ReviewerOpenCodeAgent:       reviewerOpenCodeAgent,
	})
	service := cli.NewOrchestratorRunService(
		orch,
		store,
		cli.SafeWorktreeCleanup{Runner: process, Binary: cfg.GitBinary, HerdrBinary: cfg.HerdrBinary, HerdrWorktreeRoot: cfg.HerdrWorktreeRoot, RetirementAdapter: retirementGit, HerdrLocator: herdrClient},
		cli.NewRunStateRemover(cfg.StateDir),
		nil,
		worktreeRoot,
		cfg.HerdrWorktreeRoot,
	)
	return cli.Dependencies{Runs: service, Retirement: service, Confirmer: orch, Reverter: cli.NewRevertRunService(store, revert.New(git, ghes), worktreeRoot)}, nil
}

func roleAgentRouting(cfg config.Config) (builder, reviewer string) {
	return cfg.OpenCodeAgents.Builder, cfg.OpenCodeAgents.Reviewer
}

type persistedRunLoader interface {
	Load(context.Context, contract.RunID) (state.RunSnapshot, error)
}

// repositoryPathForPersistedCommand binds delayed destructive operations to
// the repository captured by the RUN. The caller's current checkout is not
// authoritative after a restart.
func repositoryPathForPersistedCommand(ctx context.Context, args []string, store persistedRunLoader, current string) (string, error) {
	if len(args) == 0 || (args[0] != "retire" && args[0] != "cleanup") {
		return current, nil
	}
	if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
		return "", errors.New("persisted command run ID is required")
	}
	snapshot, err := store.Load(ctx, contract.RunID(args[1]))
	if err != nil {
		return "", errors.New("persisted command run state could not be loaded")
	}
	repositoryPath := strings.TrimSpace(snapshot.RepositoryPath)
	if repositoryPath == "" {
		return "", errors.New("persisted command run has no repository path")
	}
	return repositoryPath, nil
}

type repositoryDiscoverer func(context.Context, runner.Runner, string) (string, error)

func repositoryPathForCommand(ctx context.Context, args []string, process runner.Runner, binary string, discover repositoryDiscoverer) (string, error) {
	if len(args) == 0 || (args[0] != "start" && args[0] != "resume" && args[0] != "confirm" && args[0] != "create-revert") {
		return "", nil
	}
	if discover == nil {
		return "", errors.New("Git 저장소 root discovery가 구성되지 않았습니다")
	}
	return discover(ctx, process, binary)
}

func requiresGHESCredential(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "start" || args[0] == "resume" || args[0] == "confirm" || args[0] == "create-revert"
}
