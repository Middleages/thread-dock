package workrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

type preparationStateFake struct {
	snapshot statev2.WorkSnapshot
	applies  []statev2.TransitionRequest
	next     []statev2.WorkSnapshot
	err      error
	loads    int
}

func (f *preparationStateFake) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	f.loads++
	return f.snapshot, nil
}
func (f *preparationStateFake) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	f.applies = append(f.applies, req)
	if f.err != nil {
		return statev2.WorkSnapshot{}, f.err
	}
	if len(f.next) != 0 {
		result := f.next[0]
		f.next = f.next[1:]
		return result, nil
	}
	return f.snapshot, nil
}

type preparationGitFake struct {
	inspect     []worktree.TaskWorktreeInspection
	createCalls int
	createErr   error
	calls       []string
	inspectErr  error
}

type concurrentPreparationState struct {
	mu       sync.Mutex
	snapshot statev2.WorkSnapshot
	beginWon bool
	applies  int
}

func (s *concurrentPreparationState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot, nil
}

func (s *concurrentPreparationState) Apply(_ context.Context, request statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applies++
	if request.Task.Action == statev2.TaskBeginWorktreePreparation {
		if s.beginWon {
			return statev2.WorkSnapshot{}, &statev2.StaleRevisionError{CurrentRevision: s.snapshot.Revision + 1, CurrentState: statev2.StateRunning}
		}
		s.beginWon = true
		task := s.snapshot.TaskStates[request.Task.TaskID]
		task.Status, task.BuilderAttempt = statev2.TaskWorktreePreparing, 1
		task.Worktree = request.Task.Worktree
		task.Invocation = request.Task.Invocation
		task.Invocation.InvocationID, task.Invocation.LogicalWorkID, task.Invocation.Role, task.Invocation.ReturnStage = request.Task.InvocationID, request.Task.LogicalWorkID, "builder", statev2.TaskPending
		task.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: request.Task.LogicalWorkID, Role: "builder", BuilderAttempt: 1, LogicalProfile: request.Task.Invocation.LogicalProfile, RuntimeFingerprint: request.Task.Invocation.RuntimeFingerprint, Worktree: request.Task.Worktree}
		task.InvocationHistory = []statev2.InvocationID{request.Task.InvocationID}
		s.snapshot.TaskStates[request.Task.TaskID], s.snapshot.Revision = task, s.snapshot.Revision+1
		return s.snapshot, nil
	}
	if request.Task.Action == statev2.TaskReconcileWorktreePreparation {
		task := s.snapshot.TaskStates[request.Task.TaskID]
		task.Status = statev2.TaskInvocationReserved
		s.snapshot.TaskStates[request.Task.TaskID], s.snapshot.Revision = task, s.snapshot.Revision+1
		return s.snapshot, nil
	}
	return statev2.WorkSnapshot{}, errors.New("unexpected transition")
}

type concurrentPreparationGit struct {
	mu           sync.Mutex
	inspectCount int
	createCalls  int
}

func (g *concurrentPreparationGit) InspectTaskWorktree(_ context.Context, _ string, path string, _ string, _ string) (worktree.TaskWorktreeInspection, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inspectCount++
	if g.inspectCount <= 2 {
		return worktree.TaskWorktreeInspection{CanonicalPath: path, GitCommonDir: "/repo/.git"}, nil
	}
	return worktree.TaskWorktreeInspection{CanonicalPath: path, GitCommonDir: "/repo/.git", Exists: true, IdentityMatches: true}, nil
}

func (g *concurrentPreparationGit) CreateManagedWorktree(context.Context, string, string, string, string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.createCalls++
	return nil
}

func (f *preparationGitFake) InspectTaskWorktree(context.Context, string, string, string, string) (worktree.TaskWorktreeInspection, error) {
	f.calls = append(f.calls, "inspect")
	if f.inspectErr != nil {
		return worktree.TaskWorktreeInspection{}, f.inspectErr
	}
	if len(f.inspect) == 0 {
		return worktree.TaskWorktreeInspection{}, errors.New("missing scripted observation")
	}
	g := f.inspect[0]
	f.inspect = f.inspect[1:]
	return g, nil
}
func (f *preparationGitFake) CreateManagedWorktree(context.Context, string, string, string, string) error {
	f.createCalls++
	f.calls = append(f.calls, "create")
	return f.createErr
}

func TestPrepareRequiresApprovedWorkAndTaskIdentity(t *testing.T) {
	state := &preparationStateFake{}
	git := &preparationGitFake{}
	service := NewPreparationService(state, git, "operator", func() time.Time { return time.Unix(10, 0).UTC() })
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	managed := filepath.Join(root, "managed")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := service.Prepare(context.Background(), PreparationRequest{WorkID: "work-1", TaskID: "task-1", InvocationID: "inv", LogicalWorkID: "logical", RepositoryPath: repo, WorktreePath: filepath.Join(managed, "task"), Branch: "main", BaseSHA: strings.Repeat("a", 40), RuntimeFingerprint: "runtime"})
	if err == nil || !strings.Contains(err.Error(), "work snapshot") {
		t.Fatalf("err=%v", err)
	}
}

func TestPrepareCreatesOnlyAfterDurableBeginAndAdoptsMatchingObservation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	managed := filepath.Join(root, "managed")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "prepare", AcceptanceCriteria: []string{"ready"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "integration-main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", Branch: "agent/task-1", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"ready"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var data bytes.Buffer
	if err := contractv2.Write(&data, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: contract, ContractHash: hex.EncodeToString(sum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit, PriorAttempts: []statev2.AttemptSummary{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	store := statev2.NewStore(filepath.Join(root, "state"))
	if _, err := store.CreatePlan(ctx, snapshot, "create", "payload"); err != nil {
		t.Fatal(err)
	}
	approved, err := store.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	approve := statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: approved.ContractHash, At: time.Unix(1, 0).UTC()}
	request := statev2.TransitionRequest{WorkID: approved.WorkID, ExpectedRevision: approved.Revision, RequestID: "approve", Work: &approve}
	request.PayloadHash, _ = statev2.TransitionPayloadHash(request)
	approved, err = store.Apply(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(managed, "task-1")
	git := &preparationGitFake{inspect: []worktree.TaskWorktreeInspection{{CanonicalPath: target, GitCommonDir: filepath.Join(repo, ".git")}, {CanonicalPath: target, GitCommonDir: filepath.Join(repo, ".git"), Exists: true, IdentityMatches: true}}}
	service := NewPreparationService(store, git, "operator", func() time.Time { return time.Unix(2, 0).UTC() })
	prepared, err := service.Prepare(ctx, PreparationRequest{WorkID: "work-1", TaskID: "task-1", InvocationID: "inv-1", LogicalWorkID: "logical-1", RepositoryPath: repo, WorktreePath: target, Branch: "agent/task-1", BaseSHA: base, RuntimeFingerprint: "runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if git.createCalls != 1 || prepared.TaskStates["task-1"].Status != statev2.TaskInvocationReserved {
		t.Fatalf("create=%d state=%#v", git.createCalls, prepared.TaskStates["task-1"])
	}
}

func preparationFixture(t *testing.T, observations []worktree.TaskWorktreeInspection, createErr error) (*PreparationService, *preparationGitFake, statev2.Store, statev2.WorkSnapshot, PreparationRequest) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	repo, managed := filepath.Join(root, "repo"), filepath.Join(root, "managed")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "prepare", AcceptanceCriteria: []string{"ready"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "integration-main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", Branch: "agent/task-1", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"ready"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var data bytes.Buffer
	if err := contractv2.Write(&data, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: contract, ContractHash: hex.EncodeToString(sum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit, PriorAttempts: []statev2.AttemptSummary{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	store := statev2.NewStore(filepath.Join(root, "state"))
	if _, err := store.CreatePlan(ctx, snapshot, "create", "payload"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, "work-1")
	if err != nil {
		t.Fatal(err)
	}
	approve := statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: snapshot.ContractHash, At: time.Unix(1, 0).UTC()}
	approval := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: "approve", Work: &approve}
	approval.PayloadHash, _ = statev2.TransitionPayloadHash(approval)
	snapshot, err = store.Apply(ctx, approval)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(managed, "task-1")
	git := &preparationGitFake{inspect: observations, createErr: createErr}
	service := NewPreparationService(store, git, "operator", func() time.Time { return time.Unix(2, 0).UTC() })
	return service, git, store, snapshot, PreparationRequest{WorkID: "work-1", TaskID: "task-1", InvocationID: "inv-1", LogicalWorkID: "logical-1", RepositoryPath: repo, WorktreePath: target, Branch: "agent/task-1", BaseSHA: base, RuntimeFingerprint: "runtime"}
}

func TestPrepareLostSuccessErrorAdoptsMatchingWorktree(t *testing.T) {
	service, git, store, _, request := preparationFixture(t, nil, errors.New("create response lost"))
	common := filepath.Join(request.RepositoryPath, ".git")
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: common}, {CanonicalPath: request.WorktreePath, GitCommonDir: common, Exists: true, IdentityMatches: true}}
	got, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if git.createCalls != 1 || got.TaskStates[request.TaskID].Status != statev2.TaskInvocationReserved {
		t.Fatalf("creates=%d state=%#v", git.createCalls, got.TaskStates[request.TaskID])
	}
	loaded, loadErr := store.Load(context.Background(), request.WorkID)
	if loadErr != nil || loaded.TaskStates[request.TaskID].Status != statev2.TaskInvocationReserved {
		t.Fatalf("loaded=%#v err=%v", loaded, loadErr)
	}
}

func TestPrepareExistingExactWorktreeAdoptsWithoutCreate(t *testing.T) {
	service, git, _, _, request := preparationFixture(t, nil, nil)
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git"), Exists: true, IdentityMatches: true}}
	got, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if git.createCalls != 0 || got.TaskStates[request.TaskID].Status != statev2.TaskInvocationReserved || len(git.calls) != 1 {
		t.Fatalf("creates=%d calls=%v state=%#v", git.createCalls, git.calls, got.TaskStates[request.TaskID])
	}
}

func TestPrepareCreateFailureMissingReturnsRetryablePending(t *testing.T) {
	// Covered by the same state-first contract as lost-success; the table below
	// uses a scripted inspector to keep the failure surface deterministic.
	service, git, store, _, request := preparationFixture(t, nil, errors.New("create failed"))
	request.WorktreePath = filepath.Join(filepath.Dir(request.WorktreePath), "task-1")
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git")}, {CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git")}}
	_, err := service.Prepare(context.Background(), request)
	if err == nil || strings.Contains(err.Error(), "create failed") {
		t.Fatal("expected preparation error")
	}
	got, loadErr := store.Load(context.Background(), request.WorkID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if got.TaskStates[request.TaskID].Status != statev2.TaskPending || got.TaskStates[request.TaskID].LogicalWork == nil || git.createCalls != 1 {
		t.Fatalf("state=%#v creates=%d", got.TaskStates[request.TaskID], git.createCalls)
	}
}

func TestPreparePreInspectionMismatchPersistsBoundedBlockerWithoutCreate(t *testing.T) {
	service, git, store, _, request := preparationFixture(t, nil, nil)
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git"), Exists: true, IdentityMatches: false}}
	_, err := service.Prepare(context.Background(), request)
	if err == nil {
		t.Fatal("expected blocker error")
	}
	got, loadErr := store.Load(context.Background(), request.WorkID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if got.TaskStates[request.TaskID].Status != statev2.TaskNeedsOperator || got.Control.Blocker == nil || got.Control.Blocker.Kind != "evidence_mismatch" || strings.Contains(got.Control.Blocker.Diagnostic, "raw") || git.createCalls != 0 {
		t.Fatalf("state=%#v creates=%d", got, git.createCalls)
	}
}

func TestPrepareRejectsInvalidBranchesBeforeLoadingState(t *testing.T) {
	root := t.TempDir()
	repo, managed := filepath.Join(root, "repo"), filepath.Join(root, "managed")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"foo.lock", ".branch", "foo//bar", "foo@{bar}"} {
		state := &preparationStateFake{}
		git := &preparationGitFake{}
		service := NewPreparationService(state, git, "operator", nil)
		_, err := service.Prepare(context.Background(), PreparationRequest{WorkID: "work", TaskID: "task", InvocationID: "inv", LogicalWorkID: "logical", RepositoryPath: repo, WorktreePath: filepath.Join(managed, "task"), Branch: branch, BaseSHA: strings.Repeat("a", 40), RuntimeFingerprint: "runtime"})
		if err == nil || state.loads != 0 || len(git.calls) != 0 {
			t.Fatalf("branch=%q err=%v loads=%d calls=%v", branch, err, state.loads, git.calls)
		}
	}
}

func TestPrepareReservedReplayIsNoOp(t *testing.T) {
	common := filepath.Join(t.TempDir(), ".git")
	service, git, _, _, request := preparationFixture(t, nil, nil)
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: common}, {CanonicalPath: request.WorktreePath, GitCommonDir: common, Exists: true, IdentityMatches: true}}
	prepared, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if git.createCalls != 1 {
		t.Fatalf("initial preparation creates=%d", git.createCalls)
	}
	before := prepared.Revision
	git.calls = nil
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: common, Exists: true, IdentityMatches: true}}
	request.InvocationID = "replay-invocation"
	got, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != before || !reflect.DeepEqual(got.TaskStates[request.TaskID], prepared.TaskStates[request.TaskID]) || !reflect.DeepEqual(git.calls, []string{"inspect"}) {
		t.Fatalf("replay mutated state: before=%d after=%d calls=%v", before, got.Revision, git.calls)
	}
	otherRepo := filepath.Join(filepath.Dir(request.RepositoryPath), "repo-b")
	if err := os.MkdirAll(otherRepo, 0700); err != nil {
		t.Fatal(err)
	}
	request.RepositoryPath = otherRepo
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(otherRepo, ".git"), Exists: true, IdentityMatches: true}}
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("expected current repository binding mismatch")
	}
	if git.createCalls != 1 {
		t.Fatalf("replay created worktree: %d", git.createCalls)
	}
}

func beginPreparingForTest(t *testing.T, store statev2.Store, snapshot statev2.WorkSnapshot, request PreparationRequest) statev2.WorkSnapshot {
	t.Helper()
	transition := statev2.TaskTransition{TaskID: request.TaskID, Action: statev2.TaskBeginWorktreePreparation, InvocationID: request.InvocationID, LogicalWorkID: request.LogicalWorkID, Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Unix(3, 0).UTC(), Worktree: &statev2.WorktreeIdentity{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git"), Branch: request.Branch, BaseSHA: request.BaseSHA, IntegratedDependencies: map[contractv2.TaskID]string{}}, Invocation: &statev2.InvocationState{LogicalProfile: snapshot.Contract.ExecutionProfiles.Builder, RuntimeFingerprint: request.RuntimeFingerprint}}
	apply := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: "begin-direct", Task: &transition}
	var err error
	apply.PayloadHash, err = statev2.TransitionPayloadHash(apply)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := store.Apply(context.Background(), apply)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func TestPrepareRestartFromPreparingOnlyInspectsAndReconciles(t *testing.T) {
	service, git, store, snapshot, request := preparationFixture(t, nil, nil)
	prepared := beginPreparingForTest(t, store, snapshot, request)
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git"), Exists: true, IdentityMatches: true}}
	request.InvocationID = "new-proposed-invocation"
	got, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	state := got.TaskStates[request.TaskID]
	if git.createCalls != 0 || state.Status != statev2.TaskInvocationReserved || state.Invocation.InvocationID != prepared.TaskStates[request.TaskID].Invocation.InvocationID || len(git.calls) != 1 || git.calls[0] != "inspect" {
		t.Fatalf("calls=%v creates=%d state=%#v", git.calls, git.createCalls, state)
	}
}

func TestPrepareRestartMismatchPersistsActiveInvocationBlocker(t *testing.T) {
	service, git, store, snapshot, request := preparationFixture(t, nil, nil)
	beginPreparingForTest(t, store, snapshot, request)
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(request.RepositoryPath, ".git"), Exists: true, IdentityMatches: false}}
	_, err := service.Prepare(context.Background(), request)
	if err == nil {
		t.Fatal("expected blocker error")
	}
	got, loadErr := store.Load(context.Background(), request.WorkID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	state := got.TaskStates[request.TaskID]
	if state.Status != statev2.TaskNeedsOperator || got.Control.Blocker == nil || got.Control.Blocker.InvocationID != "inv-1" {
		t.Fatalf("state=%#v blocker=%#v", state, got.Control.Blocker)
	}
}

func TestPreparePreparingRequiresPersistedRepositoryCommonDir(t *testing.T) {
	service, git, store, snapshot, request := preparationFixture(t, nil, nil)
	beginPreparingForTest(t, store, snapshot, request)
	otherRepo := filepath.Join(filepath.Dir(request.RepositoryPath), "repo-b")
	if err := os.MkdirAll(otherRepo, 0700); err != nil {
		t.Fatal(err)
	}
	request.RepositoryPath = otherRepo
	git.inspect = []worktree.TaskWorktreeInspection{{CanonicalPath: request.WorktreePath, GitCommonDir: filepath.Join(otherRepo, ".git"), Exists: true, IdentityMatches: true}}
	_, err := service.Prepare(context.Background(), request)
	if err == nil {
		t.Fatal("expected repository identity blocker")
	}
	got, loadErr := store.Load(context.Background(), request.WorkID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if got.TaskStates[request.TaskID].Status != statev2.TaskNeedsOperator || got.Control.Blocker == nil || got.Control.Blocker.InvocationID != "inv-1" {
		t.Fatalf("snapshot=%#v", got)
	}
}

func TestPrepareInspectionErrorPersistsBlockerWithoutRawOutput(t *testing.T) {
	service, git, store, _, request := preparationFixture(t, nil, nil)
	git.inspectErr = errors.New("raw git stderr secret")
	_, err := service.Prepare(context.Background(), request)
	if err == nil {
		t.Fatal("expected blocker error")
	}
	got, loadErr := store.Load(context.Background(), request.WorkID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if got.Control.Blocker == nil || got.Control.Blocker.Kind != "evidence_mismatch" || strings.Contains(got.Control.Blocker.Diagnostic, "secret") || git.createCalls != 0 {
		t.Fatalf("blocker=%#v creates=%d", got.Control.Blocker, git.createCalls)
	}
}

func TestPrepareStaleBeginDoesNotCreate(t *testing.T) {
	root := t.TempDir()
	repo, managed := filepath.Join(root, "repo"), filepath.Join(root, "managed")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "prepare", AcceptanceCriteria: []string{"ready"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "integration"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", Branch: "agent/task", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"ready"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var data bytes.Buffer
	if err := contractv2.Write(&data, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 4, State: statev2.StateQueued, Contract: contract, ContractHash: hex.EncodeToString(sum[:]), Control: statev2.WorkControl{ApprovedContractHash: hex.EncodeToString(sum[:]), ApprovalRef: "approval"}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	state := &preparationStateFake{snapshot: snapshot, err: &statev2.StaleRevisionError{CurrentRevision: 5, CurrentState: statev2.StateQueued}}
	target := filepath.Join(managed, "task")
	git := &preparationGitFake{inspect: []worktree.TaskWorktreeInspection{{CanonicalPath: target, GitCommonDir: filepath.Join(repo, ".git")}}}
	service := NewPreparationService(state, git, "operator", nil)
	_, err := service.Prepare(context.Background(), PreparationRequest{WorkID: "work-1", TaskID: "task-1", InvocationID: "inv", LogicalWorkID: "logical", RepositoryPath: repo, WorktreePath: target, Branch: "agent/task", BaseSHA: base, RuntimeFingerprint: "runtime"})
	if err == nil || git.createCalls != 0 {
		t.Fatalf("err=%v creates=%d", err, git.createCalls)
	}
}

func TestPrepareConcurrentCallsOnlyOneBeginAndCreateWins(t *testing.T) {
	service, _, _, snapshot, request := preparationFixture(t, nil, nil)
	state := &concurrentPreparationState{snapshot: snapshot}
	git := &concurrentPreparationGit{}
	service.state, service.git = state, git
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = service.Prepare(context.Background(), request) }()
	}
	wg.Wait()
	git.mu.Lock()
	creates, inspections := git.createCalls, git.inspectCount
	git.mu.Unlock()
	state.mu.Lock()
	beginWon, applies := state.beginWon, state.applies
	finalStatus := state.snapshot.TaskStates[request.TaskID].Status
	state.mu.Unlock()
	if !beginWon || creates != 1 || applies < 2 || inspections < 3 || finalStatus != statev2.TaskInvocationReserved {
		t.Fatalf("begin=%v creates=%d applies=%d inspections=%d status=%s", beginWon, creates, applies, inspections, finalStatus)
	}
}

func TestPrepareDerivesIntegratedDependencyHeads(t *testing.T) {
	root := t.TempDir()
	repo, managed := filepath.Join(root, "repo"), filepath.Join(root, "managed")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(managed, 0700); err != nil {
		t.Fatal(err)
	}
	base, depHead := strings.Repeat("a", 40), strings.Repeat("b", 40)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work", ProjectID: "project", Revision: 1, Request: "prepare", AcceptanceCriteria: []string{"ready"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "integration"}}, Tasks: []contractv2.Task{{TaskID: "dep", RepoKey: "repo", Branch: "agent/dep", AllowedPaths: []string{"dep"}, AcceptanceCriteria: []string{"ready"}}, {TaskID: "task", RepoKey: "repo", Branch: "agent/task", DependsOn: []contractv2.TaskID{"dep"}, AllowedPaths: []string{"task"}, AcceptanceCriteria: []string{"ready"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var data bytes.Buffer
	if err := contractv2.Write(&data, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateQueued, Contract: contract, ContractHash: hex.EncodeToString(sum[:]), Control: statev2.WorkControl{ApprovedContractHash: hex.EncodeToString(sum[:]), ApprovalRef: "approval"}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"dep": {TaskID: "dep", Status: statev2.TaskIntegrated, RepairLimit: 1, RecoveryLimit: 1, Integration: &statev2.IntegrationEvidence{IntegrationHEAD: depHead}}, "task": {TaskID: "task", Status: statev2.TaskPending, RepairLimit: 1, RecoveryLimit: 1}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	target := filepath.Join(managed, "task")
	state := &preparationStateFake{snapshot: snapshot}
	git := &preparationGitFake{inspect: []worktree.TaskWorktreeInspection{{CanonicalPath: target, GitCommonDir: filepath.Join(repo, ".git")}, {CanonicalPath: target, GitCommonDir: filepath.Join(repo, ".git"), Exists: true, IdentityMatches: true}}}
	preparing := snapshot
	preparing.TaskStates = map[contractv2.TaskID]statev2.TaskExecutionState{"dep": snapshot.TaskStates["dep"], "task": {TaskID: "task", Status: statev2.TaskWorktreePreparing, BuilderAttempt: 1, RepairLimit: 1, RecoveryLimit: 1, Worktree: &statev2.WorktreeIdentity{CanonicalPath: target, GitCommonDir: filepath.Join(repo, ".git"), Branch: "agent/task", BaseSHA: base, IntegratedDependencies: map[contractv2.TaskID]string{"dep": depHead}}, Invocation: &statev2.InvocationState{InvocationID: "inv", LogicalWorkID: "logical", Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime"}}}
	reserved := preparing
	reserved.TaskStates["task"] = reserved.TaskStates["task"]
	state.next = []statev2.WorkSnapshot{preparing, reserved}
	service := NewPreparationService(state, git, "operator", func() time.Time { return time.Unix(2, 0).UTC() })
	_, err := service.Prepare(context.Background(), PreparationRequest{WorkID: "work", TaskID: "task", InvocationID: "inv", LogicalWorkID: "logical", RepositoryPath: repo, WorktreePath: target, Branch: "agent/task", BaseSHA: base, RuntimeFingerprint: "runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.applies) == 0 || state.applies[0].Task.Worktree.IntegratedDependencies["dep"] != depHead {
		t.Fatalf("begin=%#v", state.applies)
	}
}
