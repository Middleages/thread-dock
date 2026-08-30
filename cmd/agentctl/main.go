package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"thread-dock/internal/cli"
	"thread-dock/internal/config"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/orchestrator"
	"thread-dock/internal/revert"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

func main() {
	args := os.Args[1:]
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
	repositoryPath, err := repositoryPathForCommand(context.Background(), args, process, cfg.GitBinary, cli.DiscoverRepositoryPath)
	if err != nil {
		return cli.Dependencies{}, err
	}
	worktreeRoot := filepath.Join(cfg.StateDir, "worktrees")
	store := state.NewStore(cfg.StateDir)
	// Configure both roots so managed integration/revert operations can prove
	// ownership before touching a checkout.
	git := worktree.New(process, cfg.GitBinary, worktreeRoot, repositoryPath)
	ghes := github.NewRESTClient(cfg.APIBase, token, cfg.APIVersion, nil)
	herdrClient := herdr.NewCLI(process, cfg.HerdrBinary)
	orch := orchestrator.NewAuto(orchestrator.Dependencies{
		Store:                    store,
		GitHub:                   ghes,
		Herdr:                    herdrClient,
		Git:                      git,
		Worktree:                 git,
		RepositoryPath:           repositoryPath,
		WorktreeRoot:             worktreeRoot,
		ProjectAutomationEnabled: cfg.ProjectAutomationEnabled,
		Project:                  github.ProjectRef{ID: cfg.ProjectID, StatusFieldID: cfg.ProjectStatusFieldID, StatusOptions: cfg.ProjectStatusOptions},
	})
	service := cli.NewOrchestratorRunService(
		orch,
		store,
		cli.SafeWorktreeCleanup{Runner: process, Binary: cfg.GitBinary, HerdrBinary: cfg.HerdrBinary, HerdrLocator: herdrClient},
		cli.NewRunStateRemover(cfg.StateDir),
		nil,
		worktreeRoot,
	)
	return cli.Dependencies{Runs: service, Confirmer: orch, Reverter: cli.NewRevertRunService(store, revert.New(git, ghes), worktreeRoot)}, nil
}

type repositoryDiscoverer func(context.Context, runner.Runner, string) (string, error)

func repositoryPathForCommand(ctx context.Context, args []string, process runner.Runner, binary string, discover repositoryDiscoverer) (string, error) {
	if len(args) == 0 || args[0] != "start" {
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
