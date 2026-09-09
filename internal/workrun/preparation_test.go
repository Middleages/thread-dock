package workrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

type preparationStateFake struct {
	snapshot statev2.WorkSnapshot
	applies  []statev2.TransitionRequest
}

func (f *preparationStateFake) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return f.snapshot, nil
}
func (f *preparationStateFake) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	f.applies = append(f.applies, req)
	return f.snapshot, nil
}

type preparationGitFake struct {
	inspect     []worktree.TaskWorktreeInspection
	createCalls int
}

func (f *preparationGitFake) InspectTaskWorktree(context.Context, string, string, string, string) (worktree.TaskWorktreeInspection, error) {
	if len(f.inspect) == 0 {
		return worktree.TaskWorktreeInspection{}, errors.New("missing scripted observation")
	}
	g := f.inspect[0]
	f.inspect = f.inspect[1:]
	return g, nil
}
func (f *preparationGitFake) CreateManagedWorktree(context.Context, string, string, string, string) error {
	f.createCalls++
	return nil
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
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "prepare", AcceptanceCriteria: []string{"ready"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"ready"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
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
	prepared, err := service.Prepare(ctx, PreparationRequest{WorkID: "work-1", TaskID: "task-1", InvocationID: "inv-1", LogicalWorkID: "logical-1", RepositoryPath: repo, WorktreePath: target, Branch: "main", BaseSHA: base, RuntimeFingerprint: "runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if git.createCalls != 1 || prepared.TaskStates["task-1"].Status != statev2.TaskInvocationReserved {
		t.Fatalf("create=%d state=%#v", git.createCalls, prepared.TaskStates["task-1"])
	}
}
