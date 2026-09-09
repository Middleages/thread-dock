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

type foregroundStateFake struct {
	snapshot   statev2.WorkSnapshot
	loads      int
	failLoadAt int
	loadErr    error
}

func (f *foregroundStateFake) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	f.loads++
	if f.failLoadAt > 0 && f.loads == f.failLoadAt {
		return statev2.WorkSnapshot{}, f.loadErr
	}
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
	result *statev2.WorkSnapshot
	err    error
}

func (f *foregroundPreparerFake) Prepare(_ context.Context, request PreparationRequest) (statev2.WorkSnapshot, error) {
	f.calls++
	if f.events != nil {
		*f.events = append(*f.events, "prepare")
	}
	f.last = request
	if f.err != nil {
		return statev2.WorkSnapshot{}, f.err
	}
	if f.result != nil {
		return *f.result, nil
	}
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
	activate         int
	submit           int
	close            int
	state            *foregroundStateFake
	events           *[]string
	activateMutation func(*statev2.WorkSnapshot)
	submitResult     *coordinator.CommandResult
	closeErr         error
	closedChannel    bool
	nilChannel       bool
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
	if f.activateMutation != nil {
		f.activateMutation(&f.state.snapshot)
	}
	return coordinator.ReconcileResult{}, nil
}
func (f *foregroundRuntimeFake) SubmitRuntime(_ context.Context, _ contractv2.WorkID, taskID contractv2.TaskID, invocationID statev2.InvocationID) <-chan coordinator.CommandResult {
	f.submit++
	if f.events != nil {
		*f.events = append(*f.events, "submit")
	}
	if f.submitResult != nil {
		if f.submitResult.Snapshot.WorkID != "" {
			f.state.snapshot = f.submitResult.Snapshot
		}
		result := make(chan coordinator.CommandResult, 1)
		result <- *f.submitResult
		return result
	}
	if f.closedChannel {
		result := make(chan coordinator.CommandResult)
		close(result)
		return result
	}
	if f.nilChannel {
		return nil
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
	return f.closeErr
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

func TestForegroundActivationObservedLifecycleMatrixNeverPreparesOrSubmits(t *testing.T) {
	for _, status := range []statev2.TaskStatus{statev2.TaskWorktreePreparing, statev2.TaskInvocationReserved, statev2.TaskRunning, statev2.TaskTerminationPending, statev2.TaskTerminated, statev2.TaskCandidateReady, statev2.TaskNeedsOperator} {
		t.Run(string(status), func(t *testing.T) {
			repo, root := t.TempDir(), t.TempDir()
			state, preparer, runtime := foregroundSnapshot(t, repo, root)
			runtime.activateMutation = func(snapshot *statev2.WorkSnapshot) {
				task := snapshot.TaskStates["task-1"]
				task.Status = status
				if status != statev2.TaskNeedsOperator {
					task.Invocation = &statev2.InvocationState{InvocationID: "existing", LogicalWorkID: "logical", Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-1"}
					task.InvocationHistory = []statev2.InvocationID{"existing"}
					task.Worktree = &statev2.WorktreeIdentity{CanonicalPath: filepath.Join(root, "existing"), GitCommonDir: filepath.Join(repo, ".git"), Branch: "agent/task-1", BaseSHA: strings.Repeat("a", 40)}
				}
				snapshot.TaskStates["task-1"] = task
			}
			inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}
			service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
			if _, err := service.RunWork(context.Background(), "work-1", 3, "request-1"); err != nil {
				t.Fatalf("status=%q err=%v", status, err)
			}
			if runtime.activate != 1 || preparer.calls != 0 || runtime.submit != 0 || runtime.close != 1 {
				t.Fatalf("status=%q calls activate/prepare/submit/close=%d/%d/%d/%d", status, runtime.activate, preparer.calls, runtime.submit, runtime.close)
			}
		})
	}
}

func TestForegroundContractOrderAndExactIntegratedDependencySelection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		firstReady bool
		laterReady bool
		depStatus  statev2.TaskStatus
		wantTask   contractv2.TaskID
		wantErr    bool
	}{
		{name: "first ready", firstReady: true, wantTask: "task-1"},
		{name: "later ready after unmet earlier", firstReady: false, laterReady: true, wantTask: "task-2"},
		{name: "non-integrated dependency is unmet", firstReady: false, depStatus: statev2.TaskGateFailed, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, root := t.TempDir(), t.TempDir()
			state, preparer, runtime := foregroundSnapshot(t, repo, root)
			base := strings.Repeat("a", 40)
			state.snapshot.Contract.Tasks = []contractv2.Task{
				{TaskID: "task-1", RepoKey: "repo", Branch: "agent/task-1", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}, DependsOn: func() []contractv2.TaskID {
					if tc.firstReady {
						return nil
					}
					return []contractv2.TaskID{"dependency"}
				}()},
				{TaskID: "task-2", RepoKey: "repo", Branch: "agent/task-2", AllowedPaths: []string{"docs"}, AcceptanceCriteria: []string{"done"}, DependsOn: func() []contractv2.TaskID {
					if tc.firstReady || tc.laterReady {
						return nil
					}
					return []contractv2.TaskID{"dependency"}
				}()},
			}
			state.snapshot.Contract.RepositoryPlans[0].BaseSHA = base
			state.snapshot.TaskStates = map[contractv2.TaskID]statev2.TaskExecutionState{
				"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, InvocationHistory: []statev2.InvocationID{}},
				"task-2": {TaskID: "task-2", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, InvocationHistory: []statev2.InvocationID{}},
			}
			if !tc.firstReady {
				state.snapshot.TaskStates["dependency"] = statev2.TaskExecutionState{TaskID: "dependency", Status: tc.depStatus, RepairLimit: 2, RecoveryLimit: 1, InvocationHistory: []statev2.InvocationID{}}
				state.snapshot.Contract.Tasks[0].DependsOn = []contractv2.TaskID{"dependency"}
			}
			inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}
			service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
			_, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
			if tc.wantErr {
				if err == nil || preparer.calls != 0 || runtime.submit != 0 {
					t.Fatalf("err=%v prepare=%d submit=%d", err, preparer.calls, runtime.submit)
				}
				return
			}
			if err != nil || preparer.last.TaskID != tc.wantTask {
				t.Fatalf("err=%v selected=%q want=%q", err, preparer.last.TaskID, tc.wantTask)
			}
		})
	}
}

func TestForegroundRejectsBeforeProviderMutationMatrix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*statev2.WorkSnapshot, *foregroundBindingInspectorFake)
	}{
		{name: "unapproved", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) { s.Control = statev2.WorkControl{} }},
		{name: "paused", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) { s.Control.PauseRequested = true }},
		{name: "global blocker", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) {
			s.Control.Blocker = &statev2.OperatorBlocker{Kind: statev2.BlockerKindRuntimeUnknown, OperatorRef: "operator", Diagnostic: "blocked", TaskID: "task-1"}
		}},
		{name: "task blocker", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) {
			task := s.TaskStates["task-1"]
			task.Status = statev2.TaskNeedsOperator
			s.TaskStates["task-1"] = task
		}},
		{name: "multi-repo", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) {
			s.Contract.RepositoryPlans = append(s.Contract.RepositoryPlans, contractv2.RepositoryPlan{RepoKey: "other", BaseSHA: strings.Repeat("b", 40), TargetBranch: "main"})
		}},
		{name: "missing branch", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) { s.Contract.Tasks[0].Branch = "" }},
		{name: "no ready task", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) {
			s.Contract.Tasks[0].DependsOn = []contractv2.TaskID{"missing"}
		}},
		{name: "stale revision", mutate: func(_ *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) {}},
		{name: "work identity mismatch", mutate: func(s *statev2.WorkSnapshot, _ *foregroundBindingInspectorFake) {
			s.WorkID = "other-work"
			s.Contract.WorkID = "other-work"
		}},
		{name: "inspector error", mutate: func(_ *statev2.WorkSnapshot, i *foregroundBindingInspectorFake) {
			i.err = errors.New("raw provider secret")
		}},
		{name: "empty common dir", mutate: func(_ *statev2.WorkSnapshot, i *foregroundBindingInspectorFake) { i.common = "" }},
		{name: "noncanonical common dir", mutate: func(_ *statev2.WorkSnapshot, i *foregroundBindingInspectorFake) { i.common = "/repo/../repo/.git" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, root := t.TempDir(), t.TempDir()
			state, preparer, runtime := foregroundSnapshot(t, repo, root)
			inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}
			tc.mutate(&state.snapshot, inspector)
			service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
			expected := contractv2.Revision(3)
			if tc.name == "stale revision" {
				expected = 99
			}
			_, err := service.RunWork(context.Background(), "work-1", expected, "request-1")
			if err == nil || runtime.activate != 0 || preparer.calls != 0 || runtime.submit != 0 || runtime.close != 0 {
				if tc.name == "missing branch" || tc.name == "no ready task" {
					if runtime.close == 1 && runtime.activate == 1 && preparer.calls == 0 && runtime.submit == 0 {
						return
					}
				}
				t.Fatalf("err=%v calls activate/prepare/submit/close=%d/%d/%d/%d", err, runtime.activate, preparer.calls, runtime.submit, runtime.close)
			}
		})
	}
}

func TestForegroundSubmitErrorBoundariesAndClosePrecedence(t *testing.T) {
	for _, status := range []statev2.TaskStatus{statev2.TaskInvocationReserved, statev2.TaskRunning, statev2.TaskCandidateReady, statev2.TaskNeedsOperator} {
		t.Run(string(status), func(t *testing.T) {
			repo, root := t.TempDir(), t.TempDir()
			state, preparer, runtime := foregroundSnapshot(t, repo, root)
			canonical := matchingReservedSnapshot(state.snapshot, repo, root, status, false)
			runtime.submitResult = &coordinator.CommandResult{Snapshot: canonical, Err: errors.New("scripted runtime error")}
			inspector := &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}
			service := NewForegroundService(state, preparer, inspector, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
			got, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
			if err != nil || got.TaskStates["task-1"].Status != status || runtime.close != 1 {
				t.Fatalf("status=%q got=%#v err=%v close=%d", status, got.TaskStates["task-1"].Status, err, runtime.close)
			}
		})
	}
	t.Run("different invocation is rejected", func(t *testing.T) {
		repo, root := t.TempDir(), t.TempDir()
		state, preparer, runtime := foregroundSnapshot(t, repo, root)
		wrong := matchingReservedSnapshot(state.snapshot, repo, root, statev2.TaskRunning, true)
		runtime.submitResult = &coordinator.CommandResult{Snapshot: wrong, Err: errors.New("runtime error")}
		service := NewForegroundService(state, preparer, &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
		got, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
		if err == nil || got.WorkID != "" || runtime.close != 1 {
			t.Fatalf("different invocation fabricated result=%#v err=%v close=%d", got, err, runtime.close)
		}
	})
}

func TestForegroundPreparationAndCloseErrorsPreservePrimaryError(t *testing.T) {
	repo, root := t.TempDir(), t.TempDir()
	state, preparer, runtime := foregroundSnapshot(t, repo, root)
	preparer.err = errors.New("prepare failed")
	runtime.closeErr = errors.New("close failed")
	service := NewForegroundService(state, preparer, &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
	_, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
	if err == nil || !strings.Contains(err.Error(), "prepare failed") || strings.Contains(err.Error(), "close failed") || runtime.close != 1 {
		t.Fatalf("primary error err=%v close=%d", err, runtime.close)
	}
}

func TestForegroundRejectsPreparationIdentityMismatchAndInvalidSubmitChannels(t *testing.T) {
	for _, tc := range []struct {
		name            string
		prepareMismatch bool
		nilChannel      bool
		closedChannel   bool
	}{
		{name: "wrong preparation identity", prepareMismatch: true},
		{name: "nil submit channel", nilChannel: true},
		{name: "closed submit channel", closedChannel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, root := t.TempDir(), t.TempDir()
			state, preparer, runtime := foregroundSnapshot(t, repo, root)
			if tc.prepareMismatch {
				wrong := matchingReservedSnapshot(state.snapshot, repo, root, statev2.TaskInvocationReserved, true)
				preparer.result = &wrong
			}
			runtime.nilChannel, runtime.closedChannel = tc.nilChannel, tc.closedChannel
			service := NewForegroundService(state, preparer, &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
			got, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
			wantSubmit := 1
			if tc.prepareMismatch {
				wantSubmit = 0
			}
			if err == nil || (!tc.prepareMismatch && got.WorkID != "") || runtime.submit != wantSubmit || runtime.close != 1 {
				t.Fatalf("got=%#v err=%v submit=%d close=%d", got, err, runtime.submit, runtime.close)
			}
		})
	}
}

func TestForegroundTreatsIntegratedTaskEvidenceAsSettledAndSelectsNextTask(t *testing.T) {
	repo, root := t.TempDir(), t.TempDir()
	state, preparer, runtime := foregroundSnapshot(t, repo, root)
	state.snapshot.Contract.Tasks = []contractv2.Task{
		{TaskID: "task-1", RepoKey: "repo", Branch: "agent/task-1", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}},
		{TaskID: "task-2", RepoKey: "repo", Branch: "agent/task-2", AllowedPaths: []string{"docs"}, AcceptanceCriteria: []string{"done"}, DependsOn: []contractv2.TaskID{"task-1"}},
	}
	state.snapshot.TaskStates["task-2"] = statev2.TaskExecutionState{TaskID: "task-2", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, InvocationHistory: []statev2.InvocationID{}}
	task1 := state.snapshot.TaskStates["task-1"]
	task1.Status = statev2.TaskIntegrated
	task1.BuilderAttempt = 1
	task1.Candidate = &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), ChangedFiles: []string{"internal/settled.go"}}
	task1.Invocation = &statev2.InvocationState{InvocationID: "settled-invocation", LogicalWorkID: "settled-logical", Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-1"}
	task1.InvocationHistory = []statev2.InvocationID{"settled-invocation"}
	task1.Worktree = &statev2.WorktreeIdentity{CanonicalPath: filepath.Join(root, "settled"), GitCommonDir: filepath.Join(repo, ".git"), Branch: "agent/task-1", BaseSHA: strings.Repeat("a", 40)}
	task1.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: "settled-logical", Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "runtime-1", Worktree: task1.Worktree}
	task1.Gate = &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true}
	task1.Review = &statev2.ReviewEvidence{ReviewerInvocationID: "review", BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), ReviewSHA: strings.Repeat("c", 40), Accepted: true, Findings: []statev2.ReviewFinding{}}
	task1.Integration = &statev2.IntegrationEvidence{BuilderAttempt: 1, CandidateSHA: strings.Repeat("a", 40), IntegrationHEAD: strings.Repeat("d", 40), RelationVerified: true}
	state.snapshot.TaskStates["task-1"] = task1
	service := NewForegroundService(state, preparer, &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
	if _, err := service.RunWork(context.Background(), "work-1", 3, "request-1"); err != nil {
		t.Fatal(err)
	}
	if preparer.last.TaskID != "task-2" || preparer.calls != 1 || runtime.submit != 1 || runtime.close != 1 {
		t.Fatalf("selected=%q prepare/submit/close=%d/%d/%d", preparer.last.TaskID, preparer.calls, runtime.submit, runtime.close)
	}
	settled := state.snapshot.TaskStates["task-1"]
	if settled.Status != statev2.TaskIntegrated || settled.Candidate == nil || settled.Invocation == nil || settled.Worktree == nil || settled.Gate == nil || settled.Review == nil || settled.Integration == nil {
		t.Fatalf("settled task evidence was changed: %#v", settled)
	}
}

func TestForegroundSubmitErrorReloadFailureKeepsOriginalRuntimeError(t *testing.T) {
	repo, root := t.TempDir(), t.TempDir()
	state, preparer, runtime := foregroundSnapshot(t, repo, root)
	canonical := matchingReservedSnapshot(state.snapshot, repo, root, statev2.TaskRunning, false)
	runtime.submitResult = &coordinator.CommandResult{Snapshot: canonical, Err: errors.New("original runtime error")}
	state.failLoadAt = 3
	state.loadErr = errors.New("post-submit load failed")
	service := NewForegroundService(state, preparer, &foregroundBindingInspectorFake{common: filepath.Join(repo, ".git")}, runtime, ForegroundBinding{RepositoryPath: repo, WorktreeRoot: root, RuntimeFingerprint: "runtime-1"})
	got, err := service.RunWork(context.Background(), "work-1", 3, "request-1")
	if err == nil || !strings.Contains(err.Error(), "original runtime error") || strings.Contains(err.Error(), "post-submit load failed") || got.WorkID != "" || runtime.close != 1 {
		t.Fatalf("reload failure result=%#v err=%v close=%d loads=%d", got, err, runtime.close, state.loads)
	}
}

func matchingReservedSnapshot(snapshot statev2.WorkSnapshot, repo, root string, status statev2.TaskStatus, wrongInvocation bool) statev2.WorkSnapshot {
	states := make(map[contractv2.TaskID]statev2.TaskExecutionState, len(snapshot.TaskStates))
	for id, task := range snapshot.TaskStates {
		states[id] = task
	}
	snapshot.TaskStates = states
	invocationID, logicalID := foregroundIDs(snapshot.WorkID, "request-1")
	if wrongInvocation {
		invocationID = "different-invocation"
	}
	worktreePath := filepath.Join(root, foregroundWorktreeID(snapshot.WorkID, "task-1"))
	task := snapshot.TaskStates["task-1"]
	task.Status = status
	task.BuilderAttempt = 1
	task.InvocationHistory = []statev2.InvocationID{invocationID}
	task.Worktree = &statev2.WorktreeIdentity{CanonicalPath: worktreePath, GitCommonDir: filepath.Join(repo, ".git"), Branch: "agent/task-1", BaseSHA: strings.Repeat("a", 40)}
	task.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: logicalID, Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "runtime-1", Worktree: task.Worktree}
	task.Invocation = &statev2.InvocationState{InvocationID: invocationID, LogicalWorkID: logicalID, Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-1"}
	snapshot.TaskStates["task-1"] = task
	return snapshot
}
