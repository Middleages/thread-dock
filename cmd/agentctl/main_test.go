package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"thread-dock/internal/cli"
	"thread-dock/internal/config"
	"thread-dock/internal/contract"
	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/registry"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
	statev2 "thread-dock/internal/state/v2"
)

func TestRuntimeWorkflowCommandOnlyAcceptsExactRuntimeShapes(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want bool
	}{
		{[]string{"work", "run", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, true},
		{[]string{"work", "reconcile", "work-1", "--json"}, true},
		{[]string{"work", "status", "work-1", "--json"}, false},
		{[]string{"project", "status", "--all", "--json"}, false},
		{[]string{"work", "plan", "contract.json", "--expected-revision", "0", "--request-id", "request-1"}, false},
		{[]string{"work", "approve", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, false},
		{[]string{"work", "pause", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, false},
		{[]string{"work", "resume", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, false},
		{[]string{"work", "run", "work-1", "--expected-revision", "0", "--request-id", "request-1"}, false},
		{[]string{"work", "reconcile", "work-1"}, false},
	} {
		if got := runtimeWorkflowCommand(tt.args); got != tt.want {
			t.Errorf("args=%v runtime=%t want=%t", tt.args, got, tt.want)
		}
	}
}

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

func TestProductionWorkflowDependenciesUseStateDirWithoutLegacySetup(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("THREADDOCK_STATE_DIR", stateDir)
	t.Setenv("THREADDOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	deps, err := productionWorkflowDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if deps.Workflow == nil || deps.Runs != nil || deps.Confirmer != nil || deps.Reverter != nil {
		t.Fatalf("workflow deps=%#v", deps)
	}
	project := registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	if _, err := deps.Workflow.RegisterProject(context.Background(), project, 0, "request-project"); err != nil {
		t.Fatal(err)
	}
	projects, err := deps.Workflow.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].ProjectID != project.ProjectID {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "v2", "projects", "project-1.json")); err != nil {
		t.Fatalf("v2 project state was not written under THREADDOCK_STATE_DIR: %v", err)
	}
}

func TestProductionWorkflowDependenciesStatusStaysProviderNeutralWithoutConfig(t *testing.T) {
	t.Setenv("THREADDOCK_STATE_DIR", t.TempDir())
	t.Setenv("THREADDOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	deps, err := productionWorkflowDependencies([]string{"work", "status", "work-1", "--json"})
	if err != nil || deps.Workflow == nil {
		t.Fatalf("deps=%#v err=%v", deps, err)
	}
	if _, ok := deps.Workflow.(interface {
		RunWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	}); ok {
		t.Fatal("status unexpectedly constructed runtime runner")
	}
}

func TestProductionWorkflowDependenciesRuntimeBindingAndRequiredProfiles(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	if result, err := (runner.OSRunner{}).Run(context.Background(), "", "git", "init", repository); err != nil || result.ExitCode != 0 {
		t.Fatalf("git init exit=%d err=%v", result.ExitCode, err)
	}
	stateRoot := filepath.Join(root, "state")
	if err := os.Mkdir(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	workContract := contractv2.WorkItemContract{Version: 2, WorkID: "work-production", ProjectID: "project-production", Revision: 1, Request: "run", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-production", RepoKey: "repo", Branch: "agent/task-production", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder-logical", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, workContract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	store := statev2.NewStore(stateRoot)
	initial := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: workContract.ProjectID, WorkID: workContract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: workContract, ContractHash: hex.EncodeToString(sum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-production": {TaskID: "task-production", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	if _, err := store.CreatePlan(context.Background(), initial, "plan-production", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	approval := statev2.TransitionRequest{WorkID: workContract.WorkID, ExpectedRevision: 1, RequestID: "approve-production", Work: &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: initial.ContractHash}}
	approval.PayloadHash, _ = statev2.TransitionPayloadHash(approval)
	if _, err := store.Apply(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	configData := `{"ghesHost":"https://github.example.test","stateDir":"` + stateRoot + `","openCodeAgents":{"builder":"builder-native"},"builderRuntimeFingerprint":"  fingerprint-production  ","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREADDOCK_STATE_DIR", stateRoot)
	t.Setenv("THREADDOCK_CONFIG", configPath)
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	runArgs := []string{"work", "run", string(workContract.WorkID), "--expected-revision", "2", "--request-id", "request-production"}
	deps, err := productionWorkflowDependencies(runArgs)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deps.Workflow.(interface {
		RunWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	}); !ok {
		t.Fatal("run workflow lacks runner capability")
	}
	if _, ok := deps.Workflow.(interface {
		ReconcileWork(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error)
	}); !ok {
		t.Fatal("run workflow lacks reconcile capability")
	}
	reconcileDeps, err := productionWorkflowDependencies([]string{"work", "reconcile", string(workContract.WorkID), "--json"})
	if err != nil || reconcileDeps.Workflow == nil {
		t.Fatalf("reconcile deps=%#v err=%v", reconcileDeps, err)
	}
	for _, data := range []string{
		strings.Replace(configData, `,"builderRuntimeFingerprint":"  fingerprint-production  "`, "", 1),
		strings.Replace(configData, `,"openCodeAgents":{"builder":"builder-native"}`, "", 1),
	} {
		badPath := filepath.Join(root, "bad-config.json")
		if err := os.WriteFile(badPath, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("THREADDOCK_CONFIG", badPath)
		if _, err := productionWorkflowDependencies(runArgs); err == nil || strings.Contains(err.Error(), "fingerprint-production") {
			t.Fatalf("bad runtime config err=%v", err)
		}
	}
}
