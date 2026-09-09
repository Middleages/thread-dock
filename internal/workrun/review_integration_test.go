package workrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/runner"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
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

func TestReviewIntegrationBlockDoesNotMerge(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	task := snapshot.TaskStates["task-1"]
	task.Status, task.Review, task.Integration = statev2.TaskGatePassed, nil, nil
	snapshot.TaskStates["task-1"] = task
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state, decision: "block"}
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40)}
	service := NewReviewIntegrationService(state, runtime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "caller")
	if err == nil || got.TaskStates["task-1"].Status != statev2.TaskReviewBlocked || git.mergedSHA != "" {
		t.Fatalf("status=%q err=%v merge=%q", got.TaskStates["task-1"].Status, err, git.mergedSHA)
	}
}

func TestReviewIntegrationConflictAbortsAndPersistsBlocker(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	state := &reviewIntegrationState{snapshot: snapshot}
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40), mergeErr: worktree.ErrConflict}
	service := NewReviewIntegrationService(state, &reviewIntegrationRuntimeFake{state: state}, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "caller")
	if git.abort != 1 || got.TaskStates["task-1"].Status != statev2.TaskNeedsOperator || got.Control.Blocker == nil || got.TaskStates["task-1"].Integration != nil {
		t.Fatalf("status=%q err=%v abort=%d blocker=%#v", got.TaskStates["task-1"].Status, err, git.abort, got.Control.Blocker)
	}
}

func TestReviewIntegrationRejectsStaleSuppliedSnapshotBeforeRuntimeOrGit(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state}
	git := &reviewIntegrationGitFake{}
	service := NewReviewIntegrationService(state, runtime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	stale := snapshot
	stale.Revision = 0
	if _, err := service.Advance(context.Background(), stale, "task-1", "caller"); err == nil || runtime.activate != 0 || runtime.submit != 0 || git.create != 0 || git.mergedSHA != "" {
		t.Fatalf("stale advance err=%v runtime=%d/%d git=%d/%s", err, runtime.activate, runtime.submit, git.create, git.mergedSHA)
	}
}

func TestReviewIntegrationReplayUsesExistingReviewerInvocation(t *testing.T) {
	snapshot := acceptedReviewSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40))
	task := snapshot.TaskStates["task-1"]
	task.Status, task.Review, task.Integration = statev2.TaskInvocationReserved, nil, nil
	snapshot.TaskStates["task-1"] = task
	state := &reviewIntegrationState{snapshot: snapshot}
	runtime := &reviewIntegrationRuntimeFake{state: state}
	git := &reviewIntegrationGitFake{head: strings.Repeat("3", 40)}
	service := NewReviewIntegrationService(state, runtime, git, "operator", time.Now, ReviewIntegrationBinding{RepositoryPath: "/repo", IntegrationPath: "/integration", IntegrationBranch: "main", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), snapshot, "task-1", "fresh-caller")
	if err != nil || runtime.lastInvocation != "review-inv" || state.reserves != 0 || got.TaskStates["task-1"].Status != statev2.TaskIntegrated {
		t.Fatalf("status=%q err=%v invocation=%q reserves=%d", got.TaskStates["task-1"].Status, err, runtime.lastInvocation, state.reserves)
	}
}

func TestReviewIntegrationAcceptUsesRealManagedGitAndPersistsEvidence(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGit(t, "", "init", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "ThreadDock Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	runGit(t, repo, "checkout", "-b", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "change.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "change.txt")
	runGit(t, repo, "commit", "-m", "candidate")
	candidate := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	state := &reviewIntegrationState{snapshot: acceptedReviewSnapshot(base, candidate)}
	realGit := worktree.New(runner.OSRunner{}, "git", root, repo)
	service := NewReviewIntegrationService(state, &reviewIntegrationRuntimeFake{state: state}, realGit, "operator", func() time.Time { return time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC) }, ReviewIntegrationBinding{RepositoryPath: repo, IntegrationPath: filepath.Join(root, "integration"), IntegrationBranch: "integration", RuntimeFingerprint: "review-runtime"})
	got, err := service.Advance(context.Background(), state.snapshot, "task-1", "caller")
	if err != nil {
		t.Fatal(err)
	}
	task := got.TaskStates["task-1"]
	if task.Status != statev2.TaskIntegrated || task.Integration == nil || task.Integration.CandidateSHA != candidate || task.Integration.IntegrationHEAD == candidate || !task.Integration.RelationVerified {
		t.Fatalf("task=%#v", task)
	}
	if !strings.Contains(runGit(t, filepath.Join(root, "integration"), "log", "--format=%s"), "Merge") {
		t.Fatal("integration branch did not record a merge commit")
	}
}

func acceptedReviewSnapshot(base, candidate string) statev2.WorkSnapshot {
	wt := &statev2.WorktreeIdentity{CanonicalPath: "/candidate", GitCommonDir: "/repo/.git", Branch: "candidate", BaseSHA: base}
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	return statev2.WorkSnapshot{WorkID: "work-1", Revision: 1, Contract: contractv2.WorkItemContract{WorkID: "work-1", RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "repo"}}}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskAccepted, BuilderAttempt: 1, Worktree: wt, Invocation: &statev2.InvocationState{InvocationID: "review-inv", LogicalWorkID: "review-logical", Role: "reviewer", ReturnStage: statev2.TaskGatePassed, TerminationConfirmed: true, EndedAt: &at}, Candidate: &statev2.CandidateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, TreeSHA: strings.Repeat("a", 40), ChangedFiles: []string{"change.txt"}}, Gate: &statev2.GateEvidence{BuilderAttempt: 1, CandidateSHA: candidate, Commands: []string{"check"}, Outcomes: []string{"passed"}, Passed: true, ObservedAt: at}, Review: &statev2.ReviewEvidence{ReviewerInvocationID: "review-inv", BuilderAttempt: 1, CandidateSHA: candidate, ReviewSHA: candidate, Accepted: true, Findings: []statev2.ReviewFinding{}, ObservedAt: at}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
}

type reviewIntegrationState struct {
	snapshot statev2.WorkSnapshot
	reserves int
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
		s.reserves++
		task.Status = statev2.TaskInvocationReserved
		task.Invocation = &statev2.InvocationState{InvocationID: request.Task.InvocationID, LogicalWorkID: request.Task.LogicalWorkID, Role: "reviewer", ReturnStage: statev2.TaskGatePassed, LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime"}
		task.LogicalWork = &statev2.LogicalWorkState{LogicalWorkID: request.Task.LogicalWorkID, Role: "reviewer", BuilderAttempt: 1, LogicalProfile: "reviewer", RuntimeFingerprint: "review-runtime", Worktree: request.Task.Worktree}
		task.Worktree = request.Task.Worktree
	case statev2.TaskRecordIntegration:
		task.Status = statev2.TaskIntegrated
		task.Integration = request.Task.Integration
	case statev2.TaskNeedsOperatorAction:
		task.Status = statev2.TaskNeedsOperator
		s.snapshot.Control.Blocker = request.Task.Blocker
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
	decision         string
	lastInvocation   statev2.InvocationID
}

func (r *reviewIntegrationRuntimeFake) Activate(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error) {
	if invocation := r.state.snapshot.TaskStates["task-1"].Invocation; invocation != nil && invocation.Role != "reviewer" {
		return coordinator.ReconcileResult{}, errors.New("reviewer was reserved before activation")
	}
	r.activate++
	return coordinator.ReconcileResult{}, nil
}
func (r *reviewIntegrationRuntimeFake) SubmitRuntime(_ context.Context, _ contractv2.WorkID, taskID contractv2.TaskID, _ statev2.InvocationID) <-chan coordinator.CommandResult {
	r.submit++
	task := r.state.snapshot.TaskStates[taskID]
	r.lastInvocation = task.Invocation.InvocationID
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	task.Status = statev2.TaskAccepted
	if r.decision == "block" {
		task.Status = statev2.TaskReviewBlocked
	}
	task.Invocation.TerminationConfirmed = true
	task.Invocation.EndedAt = &at
	task.Review = &statev2.ReviewEvidence{ReviewerInvocationID: task.Invocation.InvocationID, BuilderAttempt: 1, CandidateSHA: task.Candidate.CandidateSHA, ReviewSHA: task.Candidate.CandidateSHA, Accepted: r.decision != "block", Findings: []statev2.ReviewFinding{}, ObservedAt: at}
	if r.decision == "block" {
		task.Review.Findings = []statev2.ReviewFinding{{Code: "blocked", Severity: "blocking", Diagnostic: "blocked"}}
	}
	r.state.snapshot.TaskStates[taskID] = task
	result := make(chan coordinator.CommandResult, 1)
	result <- coordinator.CommandResult{Snapshot: r.state.snapshot}
	return result
}
func (r *reviewIntegrationRuntimeFake) Close(context.Context) error { return nil }

type reviewIntegrationGitFake struct {
	create, abort   int
	mergeErr        error
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
	return g.mergeErr
}
func (g *reviewIntegrationGitFake) AbortMerge(context.Context, string) error { g.abort++; return nil }
func (g *reviewIntegrationGitFake) CurrentCommit(context.Context, string) (string, error) {
	return g.head, nil
}
func (g *reviewIntegrationGitFake) IsAncestor(context.Context, string, string) (bool, error) {
	return true, nil
}
