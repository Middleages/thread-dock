package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"thread-dock/internal/cli"
	"thread-dock/internal/config"
	"thread-dock/internal/contract"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
)

func TestProductionDependenciesRouteOpenCodeRoleAgents(t *testing.T) {
	cfg := config.Config{OpenCodeAgents: config.OpenCodeAgents{
		Builder:  "build",
		Reviewer: "review",
	}}

	builder, reviewer := roleAgentRouting(cfg)
	if builder != "build" || reviewer != "review" {
		t.Fatalf("role agent routing builder=%q reviewer=%q, want build/review", builder, reviewer)
	}
}

func TestRepositoryDiscoveryOnlyRunsForStart(t *testing.T) {
	var calls int
	discover := func(context.Context, runner.Runner, string) (string, error) {
		calls++
		return "/workspace/repo", nil
	}

	path, err := repositoryPathForCommand(context.Background(), []string{"start", "contract.json"}, nil, "git", discover)
	if err != nil || path != "/workspace/repo" || calls != 1 {
		t.Fatalf("start path=%q err=%v calls=%d", path, err, calls)
	}
	for _, command := range []string{"status", "stop", "cleanup"} {
		path, err = repositoryPathForCommand(context.Background(), []string{command, "run-184"}, nil, "git", discover)
		if err != nil || path != "" || calls != 1 {
			t.Fatalf("%s path=%q err=%v calls=%d", command, path, err, calls)
		}
	}
	for _, command := range []string{"resume", "confirm", "create-revert"} {
		path, err = repositoryPathForCommand(context.Background(), []string{command, "run-184"}, nil, "git", discover)
		if err != nil || path != "/workspace/repo" {
			t.Fatalf("%s path=%q err=%v", command, path, err)
		}
	}
	if calls != 4 {
		t.Fatalf("discovery calls=%d, want four", calls)
	}
}

func TestRepositoryDiscoveryPropagatesStartFailure(t *testing.T) {
	want := errors.New("not a git checkout")
	discover := func(context.Context, runner.Runner, string) (string, error) {
		return "", want
	}
	if _, err := repositoryPathForCommand(context.Background(), []string{"start", "contract.json"}, nil, "git", discover); !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}

func TestRetireUsesSnapshotWiringWithoutRepositoryDiscoveryOrGHESCredential(t *testing.T) {
	if requiresGHESCredential([]string{"retire", "run-184"}) {
		t.Fatal("retire must not require a GHES token")
	}
	var calls int
	path, err := repositoryPathForCommand(context.Background(), []string{"retire", "run-184"}, nil, "git", func(context.Context, runner.Runner, string) (string, error) {
		calls++
		return "/wrong/current/checkout", nil
	})
	if err != nil || path != "" || calls != 0 {
		t.Fatalf("retire repository discovery path=%q err=%v calls=%d", path, err, calls)
	}
}

func TestPersistedRepositoryCommandsUseSnapshotPath(t *testing.T) {
	store := state.NewStore(t.TempDir())
	snapshot := state.RunSnapshot{RunID: "persisted-repository", Phase: contract.PhaseCompleted, RepositoryPath: "/snapshot/repository"}
	if err := store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"retire", "cleanup"} {
		got, err := repositoryPathForPersistedCommand(context.Background(), []string{command, string(snapshot.RunID)}, store, "")
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if got != snapshot.RepositoryPath {
			t.Fatalf("%s repository=%q, want %q", command, got, snapshot.RepositoryPath)
		}
	}
}

func TestProductionDependenciesUseSnapshotRepositoryAndCompositeRetirementRoots(t *testing.T) {
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	stateDir := t.TempDir()
	herdrRoot := filepath.Join(t.TempDir(), "herdr-worktrees")
	if err := os.MkdirAll(herdrRoot, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	configData, err := json.Marshal(map[string]any{
		"ghesHost": "https://github.example.test", "stateDir": stateDir,
		"herdrWorktreeRoot": herdrRoot, "projectId": "PVT_1", "projectStatusFieldId": "PVTSSF_1",
		"projectStatusOptions": map[string]string{"Backlog": "opt-1", "Ready": "opt-2", "In Progress": "opt-3", "Review": "opt-4", "Done": "opt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREADDOCK_CONFIG", configPath)
	store := state.NewStore(stateDir)
	snapshot := state.RunSnapshot{RunID: "retire-wiring", Phase: contract.PhaseCompleted, RepositoryPath: "/snapshot/repository"}
	if err := store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	deps, err := productionDependencies([]string{"retire", string(snapshot.RunID)})
	if err != nil {
		t.Fatal(err)
	}
	service, ok := deps.Retirement.(*cli.OrchestratorRunService)
	if !ok {
		t.Fatalf("retirement service=%T", deps.Retirement)
	}
	runtime, err := service.RetirementRuntime(context.Background(), snapshot.RunID)
	if err != nil {
		t.Fatal(err)
	}
	wantManaged := filepath.Join(stateDir, "worktrees")
	if runtime.RepositoryPath != snapshot.RepositoryPath || runtime.ManagedRoot != wantManaged || runtime.HerdrRoot != filepath.Clean(herdrRoot) || !runtime.Composite {
		t.Fatalf("runtime=%#v want repo=%q managed=%q herdr=%q composite=true", runtime, snapshot.RepositoryPath, wantManaged, herdrRoot)
	}
}
