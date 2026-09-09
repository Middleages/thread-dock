package workrun

import (
	"context"
	"errors"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	statev2 "thread-dock/internal/state/v2"
)

func TestReviewIntegrationServiceRequiresDependencies(t *testing.T) {
	service := NewReviewIntegrationService(nil, nil, nil, "operator", nil, ReviewIntegrationBinding{})
	_, err := service.Advance(context.Background(), statev2.WorkSnapshot{WorkID: contractv2.WorkID("work-1")}, "task-1", "request-1")
	if err == nil {
		t.Fatal("missing integration dependencies unexpectedly accepted")
	}
}

func TestReviewIntegrationLaunchesReviewerAndMergesExactCandidate(t *testing.T) {
	const candidate = "0123456789abcdef0123456789abcdef01234567"
	const tree = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	const base = "1111111111111111111111111111111111111111"
	snapshot := statev2.WorkSnapshot{WorkID: "work-1", Revision: 3, Contract: contractv2.WorkItemContract{
		WorkID: "work-1", AcceptanceCriteria: []string{"work"},
		RepositoryPlans:   []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base}},
		Tasks:             []contractv2.Task{{TaskID: "task-1", RepoKey: "repo", AcceptanceCriteria: []string{"task"}}},
		ExecutionProfiles: contractv2.ExecutionProfiles{Reviewer: "reviewer"},
	}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	wt := &statev2.WorktreeIdentity{CanonicalPath: "/review", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: base}
	snapshot.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskGatePassed, BuilderAttempt: 1, Worktree: wt, LogicalWork: &statev2.LogicalWorkState{LogicalWorkID: "builder", Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "builder-runtime", Worktree: wt}, Candidate: &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, TreeSHA: tree, ChangedFiles: []string{"internal/x.go"}}, Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: time.Now().UTC()}, InvocationHistory: []statev2.InvocationID{}}
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state}
	git := &reviewIntegrationGitFake{head: "2222222222222222222222222222222222222222"}
	service := NewReviewIntegrationService(state, runtime, git, "operator", func() time.Time { return time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC) }, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "caller-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskStates["task-1"].Status != statev2.TaskIntegrated || runtime.activate != 1 || runtime.submit != 1 || git.mergedSHA != candidate || git.create != 1 || git.abort != 0 {
		t.Fatalf("status=%q runtime=%d/%d git create/merge/abort=%d/%s/%d", got.TaskStates["task-1"].Status, runtime.activate, runtime.submit, git.create, git.mergedSHA, git.abort)
	}
	if _, err := service.Advance(context.Background(), got, "task-1", "caller-1"); err != nil {
		t.Fatal(err)
	}
	if runtime.activate != 1 || runtime.submit != 1 || git.create != 1 || git.mergedSHA != candidate {
		t.Fatalf("integrated replay mutated runtime/git: runtime=%d/%d git=%d/%s", runtime.activate, runtime.submit, git.create, git.mergedSHA)
	}
}

type reviewIntegrationState struct {
	snapshot statev2.WorkSnapshot
}

func (s *reviewIntegrationState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}
func (s *reviewIntegrationState) Apply(_ context.Context, request statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	if request.Task == nil {
		return s.snapshot, errors.New("task transition required")
	}
	task := s.snapshot.TaskStates[request.Task.TaskID]
	switch request.Task.Action {
	case statev2.TaskReserveInvocation:
		task.Status = statev2.TaskInvocationReserved
		task.Invocation = &statev2.InvocationState{InvocationID: request.Task.InvocationID, LogicalWorkID: request.Task.LogicalWorkID, Role: "reviewer", ReturnStage: statev2.TaskGatePassed, LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime"}
		task.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: request.Task.LogicalWorkID, Role: "reviewer", BuilderAttempt: 1, LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime", Worktree: request.Task.Worktree}
		task.Worktree = request.Task.Worktree
	case statev2.TaskRecordIntegration:
		task.Status = statev2.TaskIntegrated
		task.Integration = request.Task.Integration
	default:
		return s.snapshot, errors.New("unexpected transition")
	}
	s.snapshot.TaskStates[request.Task.TaskID] = task
	s.snapshot.Revision++
	return s.snapshot, nil
}

type reviewIntegrationRuntimeFake struct {
	state            *reviewIntegrationState
	activate, submit int
}

func (r *reviewIntegrationRuntimeFake) Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error) {
	r.activate++
	return coordinator.ReconcileResult{}, nil
}
func (r *reviewIntegrationRuntimeFake) SubmitRuntime(_ context.Context, _ contractv2.WorkID, taskID contractv2.TaskID, _ statev2.InvocationID) <-chan coordinator.CommandResult {
	r.submit++
	task := r.state.snapshot.TaskStates[taskID]
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	task.Status = statev2.TaskAccepted
	task.Invocation.TerminationConfirmed = true
	task.Invocation.EndedAt = &at
	task.Review = &statev2.ReviewEvidence{ReviewerInvocationID: task.Invocation.InvocationID, BuilderAttempt: 1, CandidateSHA: task.Candidate.CandidateSHA, ReviewSHA: task.Candidate.CandidateSHA, Accepted: true, Findings: []statev2.ReviewFinding{}, ObservedAt: at}
	r.state.snapshot.TaskStates[taskID] = task
	result := make(chan coordinator.CommandResult, 1)
	result <- coordinator.CommandResult{Snapshot: r.state.snapshot}
	return result
}
func (r *reviewIntegrationRuntimeFake) Close(context.Context) error { return nil }

type reviewIntegrationGitFake struct {
	create, abort   int
	mergedSHA, head string
}

func (g *reviewIntegrationGitFake) CreateManagedWorktree(context.Context, string, string, string, string) error {
	g.create++
	return nil
}
func (g *reviewIntegrationGitFake) ReconcileIntegrationWorktree(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (g *reviewIntegrationGitFake) MergeCommitNoFF(_ context.Context, _ string, sha string) error {
	g.mergedSHA = sha
	return nil
}
func (g *reviewIntegrationGitFake) AbortMerge(context.Context, string) error { g.abort++; return nil }
func (g *reviewIntegrationGitFake) CurrentCommit(context.Context, string) (string, error) {
	return g.head, nil
}
func (g *reviewIntegrationGitFake) IsAncestor(context.Context, string, string) (bool, error) {
	return true, nil
}
