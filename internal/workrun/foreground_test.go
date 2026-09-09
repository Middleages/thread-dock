package workrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	statev2 "thread-dock/internal/state/v2"
)

type foregroundStateFake struct{ snapshot statev2.WorkSnapshot }

func (f *foregroundStateFake) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return f.snapshot, nil
}
func (f *foregroundStateFake) Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	return f.snapshot, errors.New("unexpected state mutation")
}

type foregroundPreparerFake struct {
	calls  int
	last   PreparationRequest
	state  *foregroundStateFake
	events *[]string
}

func (f *foregroundPreparerFake) Prepare(_ context.Context, request PreparationRequest) (statev2.WorkSnapshot, error) {
	f.calls++
	if f.events != nil {
		*f.events = append(*f.events, "prepare")
	}
	f.last = request
	task := f.state.snapshot.TaskStates[request.TaskID]
	task.Status = statev2.TaskInvocationReserved
	task.BuilderAttempt = 1
	task.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: request.LogicalWorkID, Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: request.RuntimeFingerprint, Worktree: &statev2.WorktreeIdentity{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git"), Branch: request.Branch, BaseSHA: request.BaseSHA}}
	task.Invocation = &statev2.InvocationState{InvocationID: request.InvocationID, LogicalWorkID: request.LogicalWorkID, Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: request.RuntimeFingerprint}
	task.Worktree = task.LogicalWork.Worktree
	task.InvocationHistory = []statev2.InvocationID{request.InvocationID}
	f.state.snapshot.TaskStates[request.TaskID] = task
	return f.state.snapshot, nil
}

type foregroundRuntimeFake struct {
	activate int
	submit   int
	close    int
	state    *foregroundStateFake
	events   *[]string
}

type foregroundBindingInspectorFake struct {
	calls  int
	common string
	err    error
	events *[]string
}

func (f *foregroundBindingInspectorFake) InspectRepositoryBinding(context.Context, string) (string, error) {
	f.calls++
	if f.events != nil {
		*f.events = append(*f.events, "inspect-binding")
	}
	return f.common, f.err
}

func (f *foregroundRuntimeFake) Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error) {
	f.activate++
	if f.events != nil {
		*f.events = append(*f.events, "activate")
	}
	return coordinator.ReconcileResult{}, nil
}
func (f *foregroundRuntimeFake) SubmitRuntime(_ context.Context, _ contractv2.WorkID, taskID contractv2.TaskID, invocationID statev2.InvocationID) <-chan coordinator.CommandResult {
	f.submit++
	if f.events != nil {
		*f.events = append(*f.events, "submit")
	}
	task := f.state.snapshot.TaskStates[taskID]
	task.Status = statev2.TaskRunning
	task.Invocation.InvocationID = invocationID
	f.state.snapshot.TaskStates[taskID] = task
	result := make(chan coordinator.CommandResult, 1)
	result <- coordinator.CommandResult{Snapshot: f.state.snapshot}
	return result
}
func (f *foregroundRuntimeFake) Close(context.Context) error {
	f.close++
	if f.events != nil {
		*f.events = append(*f.events, "close")
	}
	return nil
}

func foregroundSnapshot(t *testing.T, repo, worktreeRoot string) (*foregroundStateFake, *foregroundPreparerFake, *foregroundRuntimeFake) {
	t.Helper()
	base := strings.Repeat("a", 40)
	snapshot := statev2.WorkSnapshot{
		SchemaVersion: 2, ProjectID: "project-1", WorkID: "work-1", Revision: 3, State: statev2.StateQueued,
		ContractHash: "contract-hash", SyncStatus: "local", NextAction: "run", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{},
		Control:    statev2.WorkControl{ApprovedContractHash: "contract-hash", ApprovalRef: "approval"},
		Contract:   contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", Branch: "agent/task-1", AllowedPaths: []string{"internal"}}}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder"}},
		TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{},
	}
	state := &foregroundStateFake{snapshot: snapshot}
	preparer := &foregroundPreparerFake{state: state}
	runtime := &foregroundRuntimeFake{state: state}
	return state, preparer, runtime
}

func TestForegroundRunActivatesPreparesSubmitsOnceClosesAndReplays(t *testing.T) {
	repo := t.TempDir()
	root := t.TempDir()
	state, preparer, runtime := foregroundSnapshot(t, repo, root)
	events := []string{}
	inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git"), events: &events}
	preparer.events, runtime.events = &events, &events
	service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
	first, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskStates["task-1"].Status != statev2.TaskRunning || inspector.calls != 1 || runtime.activate != 1 || preparer.calls != 1 || runtime.submit != 1 || runtime.close != 1 {
		t.Fatalf("first snapshot/status calls=%#v/%d/%d/%d/%d/%d", first.TaskStates["task-1"].Status, inspector.calls, runtime.activate, preparer.calls, runtime.submit, runtime.close)
	}
	if got, want := strings.Join(events, ","), "inspect-binding,activate,prepare,submit,close"; got != want {
		t.Fatalf("fresh event order=%q want=%q", got, want)
	}
	second, err := service.RunWork(context.Background(), "work-1", 99, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if second.TaskStates["task-1"].Status != statev2.TaskRunning || inspector.calls != 2 || runtime.activate != 1 || preparer.calls != 1 || runtime.submit != 1 || runtime.close != 1 {
		t.Fatalf("replay performed lifecycle I/O: status=%q inspect=%d activate=%d prepare=%d submit=%d close=%d", second.TaskStates["task-1"].Status, inspector.calls, runtime.activate, preparer.calls, runtime.submit, runtime.close)
	}
}

func TestForegroundRunRejectsNoncanonicalBindingBeforeActivation(t *testing.T) {
	repo := t.TempDir()
	root := t.TempDir()
	state, preparer, runtime := foregroundSnapshot(t, repo, root)
	inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}
	service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo + string(os.PathSeparator) + ".", WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
	if _, err := service.RunWork(context.Background(), "work-1", 3, "request-1"); err == nil {
		t.Fatal("noncanonical repository path was accepted")
	}
	if runtime.activate != 0 || preparer.calls != 0 {
		t.Fatalf("provider calls before validation: activate=%d prepare=%d", runtime.activate, preparer.calls)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Fatal(err)
	}
}

func TestForegroundRunRejectsPersistedRepositoryBindingBeforeReplayAndCoordinator(t *testing.T) {
	repo := t.TempDir()
	root := t.TempDir()
	state, preparer, runtime := foregroundSnapshot(t, repo, root)
	task := state.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	task.Invocation = &statev2.InvocationState{InvocationID: "old-invocation", LogicalWorkID: "old-logical", Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-1"}
	task.Worktree = &statev2.WorktreeIdentity{CanonicalPath: filepath.Join(root, "old"), GitCommonDir: filepath.Join(t.TempDir(), ".git"), Branch: "agent/task-1", BaseSHA: strings.Repeat("a", 40)}
	task.InvocationHistory = []statev2.InvocationID{"old-invocation"}
	state.snapshot.TaskStates["task-1"] = task
	inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}
	service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
	if _, err := service.RunWork(context.Background(), "work-1", 3, "request-1"); err == nil {
		t.Fatal("mismatched persisted repository binding was accepted")
	}
	if inspector.calls != 1 || runtime.activate != 0 || preparer.calls != 0 || runtime.submit != 0 || runtime.close != 0 {
		t.Fatalf("binding mismatch allowed side effects: inspect=%d activate=%d prepare=%d submit=%d close=%d", inspector.calls, runtime.activate, preparer.calls, runtime.submit, runtime.close)
	}
}
