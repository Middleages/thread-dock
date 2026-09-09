package workrun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/runner"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

type verificationStateFake struct {
	snapshot statev2.WorkSnapshot
	request  statev2.TransitionRequest
	applyN   int
}

func (f *verificationStateFake) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return f.snapshot, nil
}

func (f *verificationStateFake) Apply(_ context.Context, request statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	f.applyN++
	f.request = request
	if request.Task == nil || request.Task.Gate == nil {
		return statev2.WorkSnapshot{}, errors.New("expected gate transition")
	}
	task := f.snapshot.TaskStates[request.Task.TaskID]
	task.Gate = request.Task.Gate
	if request.Task.Gate.Passed {
		task.Status = statev2.TaskGatePassed
	} else {
		task.Status = statev2.TaskGateFailed
	}
	f.snapshot.TaskStates[request.Task.TaskID] = task
	f.snapshot.Revision++
	return f.snapshot, nil
}

type verificationRunnerFake struct {
	calls   []verificationRunnerCall
	results []runner.Result
	errs    []error
}

type verificationRunnerCall struct {
	cwd        string
	executable string
	args       []string
	timeout    time.Duration
}

func (f *verificationRunnerFake) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	deadline, _ := ctx.Deadline()
	f.calls = append(f.calls, verificationRunnerCall{cwd: cwd, executable: executable, args: append([]string(nil), args...), timeout: time.Until(deadline)})
	result := runner.Result{ExitCode: 0, Stdout: "secret stdout", Stderr: "secret stderr"}
	if len(f.results) > 0 {
		result = f.results[0]
		f.results = f.results[1:]
	}
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return result, err
	}
	return result, nil
}

type verificationGitFake struct {
	inspections []worktree.TaskWorktreeInspection
	calls       []string
}

func (f *verificationGitFake) InspectTaskWorktree(_ context.Context, repositoryPath, worktreePath, branch, candidate string) (worktree.TaskWorktreeInspection, error) {
	f.calls = append(f.calls, strings.Join([]string{repositoryPath, worktreePath, branch, candidate}, "|"))
	if len(f.inspections) == 0 {
		return worktree.TaskWorktreeInspection{}, errors.New("unexpected inspection")
	}
	inspection := f.inspections[0]
	f.inspections = f.inspections[1:]
	return inspection, nil
}

func TestVerificationServiceRunsApprovedTaskCommandsAndRecordsSecretFreeGate(t *testing.T) {
	now := time.Date(2026, time.Month(9), 9, 1, 2, 3, 0, time.UTC)
	repository := "/repo"
	worktreePath := "/managed/work-1"
	candidateSHA := strings.Repeat("b", 40)
	baseSHA := strings.Repeat("a", 40)
	contract := contractv2.WorkItemContract{
		Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1,
		RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: baseSHA}},
		Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", Branch: "agent/task-1", Verification: []contractv2.CommandSpec{
			{Argv: []string{"go", "test", "./internal/foo bar"}, CwdRepoKey: "app", TimeoutSeconds: 17},
			{ShellScript: "go test ./internal/bar", CwdRepoKey: "app", TimeoutSeconds: 23},
		}}},
	}
	snapshot := statev2.WorkSnapshot{
		WorkID: "work-1", Revision: 9, Contract: contract, ContractHash: "contract-hash",
		Control: statev2.WorkControl{ApprovedContractHash: "contract-hash", ApprovalRef: "approval-1"},
		TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {
			TaskID: "task-1", Status: statev2.TaskCandidateReady, BuilderAttempt: 4,
			Candidate: &statev2.CandidateEvidence{BuilderAttempt: 4, CandidateSHA: candidateSHA, TreeSHA: strings.Repeat("c", 40), ChangedFiles: []string{}},
			Worktree:  &statev2.WorktreeIdentity{CanonicalPath: worktreePath, GitCommonDir: repository + "/.git", Branch: "agent/task-1", BaseSHA: baseSHA},
		}},
	}
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{
		{CanonicalPath: worktreePath, Exists: true, IdentityMatches: true},
		{CanonicalPath: worktreePath, Exists: true, IdentityMatches: true},
	}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })

	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskStates["task-1"].Status != statev2.TaskGatePassed {
		t.Fatalf("status = %q", got.TaskStates["task-1"].Status)
	}
	if len(process.calls) != 2 || process.calls[0].executable != "go" || strings.Join(process.calls[0].args, "|") != "test|./internal/foo bar" || process.calls[0].cwd != worktreePath {
		t.Fatalf("argv call = %#v", process.calls)
	}
	if process.calls[0].timeout < 16*time.Second || process.calls[0].timeout > 17*time.Second || process.calls[1].timeout < 22*time.Second || process.calls[1].timeout > 23*time.Second {
		t.Fatalf("deadlines = %#v", process.calls)
	}
	if process.calls[1].executable != "bash" || strings.Join(process.calls[1].args, "|") != "-lc|go test ./internal/bar" {
		t.Fatalf("shell call = %#v", process.calls[1])
	}
	if got := state.request.Task.Gate; got == nil || got.BuilderAttempt != 4 || got.CandidateSHA != candidateSHA || !got.Passed || got.ObservedAt != now || strings.Join(got.Commands, "|") != "argv:[\"go\",\"test\",\"./internal/foo bar\"]|shell:\"go test ./internal/bar\"" || strings.Join(got.Outcomes, "|") != "passed|passed" || strings.Contains(got.Diagnostic, "secret") {
		t.Fatalf("gate evidence = %#v", got)
	}
}

func TestVerificationServiceStopsAtFirstFailedCommandAndPostMutationCannotPass(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{results: []runner.Result{{ExitCode: 9, Stdout: "private", Stderr: "private"}}, errs: []error{errors.New("exit status 9")}}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true},
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: false},
	}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })

	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.applyN != 1 {
		t.Fatalf("status=%q applies=%d", got.TaskStates["task-1"].Status, state.applyN)
	}
	if len(process.calls) != 1 || len(git.calls) != 2 {
		t.Fatalf("process calls=%d git calls=%d", len(process.calls), len(git.calls))
	}
	gate := state.request.Task.Gate
	if gate.Passed || strings.Join(gate.Outcomes, "|") != "failed|not_run" || strings.Contains(gate.Diagnostic, "private") {
		t.Fatalf("gate=%#v", gate)
	}
}

func TestVerificationServiceDoesNotReturnRawProcessError(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{errs: []error{errors.New("credential=secret")}}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true},
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true},
	}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	_, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err == nil || strings.Contains(err.Error(), "secret") || state.request.Task == nil || state.request.Task.Gate.Passed {
		t.Fatalf("raw process error or successful gate: err=%v request=%#v", err, state.request)
	}
}

func TestVerificationServiceRejectsEmptyCommandsBeforeAnyProcess(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	snapshot.Contract.Tasks[0].Verification = nil
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err == nil {
		t.Fatal("empty verification commands were accepted")
	}
	if len(process.calls) != 0 || len(git.calls) != 0 || state.applyN != 0 {
		t.Fatalf("empty gate performed I/O: process=%d git=%d apply=%d", len(process.calls), len(git.calls), state.applyN)
	}
}

func verificationCandidateSnapshot() (statev2.WorkSnapshot, time.Time) {
	now := time.Date(2026, time.Month(9), 9, 1, 2, 3, 0, time.UTC)
	baseSHA := strings.Repeat("a", 40)
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1,
		RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: baseSHA}},
		Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", Branch: "agent/task-1", Verification: []contractv2.CommandSpec{
			{Argv: []string{"go", "test"}, CwdRepoKey: "app", TimeoutSeconds: 10},
			{Argv: []string{"go", "vet"}, CwdRepoKey: "app", TimeoutSeconds: 10},
		}}}}
	snapshot := statev2.WorkSnapshot{WorkID: "work-1", Revision: 9, Contract: contract, ContractHash: "contract-hash", Control: statev2.WorkControl{ApprovedContractHash: "contract-hash", ApprovalRef: "approval-1"}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {
		TaskID: "task-1", Status: statev2.TaskCandidateReady, BuilderAttempt: 4,
		Candidate: &statev2.CandidateEvidence{BuilderAttempt: 4, CandidateSHA: strings.Repeat("b", 40), TreeSHA: strings.Repeat("c", 40), ChangedFiles: []string{}},
		Worktree:  &statev2.WorktreeIdentity{CanonicalPath: "/managed/work-1", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: baseSHA},
	}}}
	return snapshot, now
}
