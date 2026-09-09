package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	applyErr error
}

func (f *verificationStateFake) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return f.snapshot, nil
}

func (f *verificationStateFake) Apply(_ context.Context, request statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	f.applyN++
	f.request = request
	if f.applyErr != nil {
		return statev2.WorkSnapshot{}, f.applyErr
	}
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
	errs        []error
	calls       []string
}

func (f *verificationGitFake) InspectTaskWorktree(_ context.Context, repositoryPath, worktreePath, branch, candidate string) (worktree.TaskWorktreeInspection, error) {
	f.calls = append(f.calls, strings.Join([]string{repositoryPath, worktreePath, branch, candidate}, "|"))
	if len(f.inspections) == 0 {
		if len(f.errs) > 0 {
			err := f.errs[0]
			f.errs = f.errs[1:]
			return worktree.TaskWorktreeInspection{}, err
		}
		return worktree.TaskWorktreeInspection{}, errors.New("unexpected inspection")
	}
	inspection := f.inspections[0]
	f.inspections = f.inspections[1:]
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return inspection, err
	}
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
	if got := state.request.Task.Gate; got == nil || got.BuilderAttempt != 4 || got.CandidateSHA != candidateSHA || !got.Passed || got.ObservedAt != now || len(got.Commands) != 2 || !strings.HasPrefix(got.Commands[0], "argv:{") || !strings.HasPrefix(got.Commands[1], "shell:{") || strings.Contains(strings.Join(got.Commands, "|"), "go test") || strings.Join(got.Outcomes, "|") != "passed|passed" || strings.Contains(got.Diagnostic, "secret") {
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

func TestVerificationServiceRejectsPersistedBranchMismatchBeforeProcess(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	snapshot.TaskStates["task-1"].Worktree.Branch = "agent/other"
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	service := NewVerificationService(state, process, &verificationGitFake{}, "operator-1", func() time.Time { return now })
	if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err == nil {
		t.Fatal("persisted branch mismatch was accepted")
	}
	if len(process.calls) != 0 {
		t.Fatalf("process calls = %d", len(process.calls))
	}
}

func TestVerificationServiceRejectsPersistedBaseMismatchBeforeProcess(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	snapshot.TaskStates["task-1"].Worktree.BaseSHA = strings.Repeat("d", 40)
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	service := NewVerificationService(state, process, &verificationGitFake{}, "operator-1", func() time.Time { return now })
	if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err == nil {
		t.Fatal("persisted base mismatch was accepted")
	}
	if len(process.calls) != 0 {
		t.Fatalf("process calls = %d", len(process.calls))
	}
}

func TestVerificationServicePreNormalNegativeRecordsFailedGateWithoutError(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{
		{CanonicalPath: "/managed/work-1", Exists: false, IdentityMatches: false},
		{CanonicalPath: "/managed/work-1", Exists: false, IdentityMatches: false},
	}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err != nil || got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.request.Task.Gate.Passed {
		t.Fatalf("got=%#v err=%v gate=%#v", got, err, state.request.Task.Gate)
	}
	if len(process.calls) != 0 || len(git.calls) != 2 {
		t.Fatalf("process/git calls=%d/%d", len(process.calls), len(git.calls))
	}
}

func TestVerificationServicePreInspectorErrorRecordsFailedGateAndReturnsBoundedError(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true}, {CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true}}, errs: []error{errors.New("secret inspector failure")}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err == nil || err.Error() != "verification worktree inspection failed" || got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.request.Task.Gate.Passed {
		t.Fatalf("got=%#v err=%v gate=%#v", got, err, state.request.Task.Gate)
	}
	if strings.Contains(err.Error(), "secret") || len(process.calls) != 0 || len(git.calls) != 2 {
		t.Fatalf("secret or I/O leak: err=%v process/git=%d/%d", err, len(process.calls), len(git.calls))
	}
}

func TestVerificationServicePostNormalNegativeAfterPassingCommandsRecordsFailedGateWithoutError(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true},
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: false},
	}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err != nil || got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.request.Task.Gate.Passed {
		t.Fatalf("got=%#v err=%v gate=%#v", got, err, state.request.Task.Gate)
	}
	if len(process.calls) != 2 {
		t.Fatalf("process calls = %d", len(process.calls))
	}
}

func TestVerificationServicePostInspectorErrorAfterPassingCommandsReturnsBoundedError(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true}}, errs: []error{nil, errors.New("secret post inspector failure")}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err == nil || err.Error() != "verification worktree inspection failed" || got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.request.Task.Gate.Passed {
		t.Fatalf("got=%#v err=%v gate=%#v", got, err, state.request.Task.Gate)
	}
	if strings.Contains(err.Error(), "secret") || len(process.calls) != 2 {
		t.Fatalf("secret or process leak: err=%v process=%d", err, len(process.calls))
	}
}

func TestVerificationServiceApplyErrorPrecedesInspectorError(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot, applyErr: errors.New("apply precedence")}
	process := &verificationRunnerFake{}
	git := &verificationGitFake{errs: []error{errors.New("inspector precedence"), errors.New("inspector precedence")}}
	service := NewVerificationService(state, process, git, "operator-1", func() time.Time { return now })
	_, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err == nil || err.Error() != "apply precedence" {
		t.Fatalf("err=%v", err)
	}
}

func TestVerificationServicePersistsArgvDigestShape(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	spec := contractv2.CommandSpec{Argv: []string{"go", "test", "./internal/foo"}, CwdRepoKey: "app", TimeoutSeconds: 19}
	snapshot.Contract.Tasks[0].Verification = []contractv2.CommandSpec{spec}
	state := &verificationStateFake{snapshot: snapshot}
	git := matchingVerificationGit()
	service := NewVerificationService(state, &verificationRunnerFake{}, git, "operator-1", func() time.Time { return now })
	if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err != nil {
		t.Fatal(err)
	}
	label := state.request.Task.Gate.Commands[0]
	if !strings.HasPrefix(label, "argv:") {
		t.Fatalf("label=%q", label)
	}
	var shape struct {
		SHA            string `json:"sha256"`
		Argc           int    `json:"argc"`
		TimeoutSeconds uint32 `json:"timeoutSeconds"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(label, "argv:")), &shape); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(spec)
	sum := sha256.Sum256(encoded)
	if shape.SHA != hex.EncodeToString(sum[:]) || shape.Argc != len(spec.Argv) || shape.TimeoutSeconds != spec.TimeoutSeconds {
		t.Fatalf("shape=%#v digest=%s", shape, hex.EncodeToString(sum[:]))
	}
}

func TestVerificationServicePersistsShellDigestShape(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	spec := contractv2.CommandSpec{ShellScript: "go test ./internal/foo", CwdRepoKey: "app", TimeoutSeconds: 29}
	snapshot.Contract.Tasks[0].Verification = []contractv2.CommandSpec{spec}
	state := &verificationStateFake{snapshot: snapshot}
	service := NewVerificationService(state, &verificationRunnerFake{}, matchingVerificationGit(), "operator-1", func() time.Time { return now })
	if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err != nil {
		t.Fatal(err)
	}
	label := state.request.Task.Gate.Commands[0]
	if !strings.HasPrefix(label, "shell:") {
		t.Fatalf("label=%q", label)
	}
	var shape struct {
		SHA            string `json:"sha256"`
		Bytes          int    `json:"bytes"`
		TimeoutSeconds uint32 `json:"timeoutSeconds"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(label, "shell:")), &shape); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(spec)
	sum := sha256.Sum256(encoded)
	if shape.SHA != hex.EncodeToString(sum[:]) || shape.Bytes != len([]byte(spec.ShellScript)) || shape.TimeoutSeconds != spec.TimeoutSeconds {
		t.Fatalf("shape=%#v digest=%s", shape, hex.EncodeToString(sum[:]))
	}
}

func TestVerificationServiceBoundsOversizedDurableCommandLabels(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec contractv2.CommandSpec
	}{
		{name: "argv", spec: contractv2.CommandSpec{Argv: []string{"go", strings.Repeat("x", statev2.MaxDiagnosticBytes*2)}, CwdRepoKey: "app", TimeoutSeconds: 1}},
		{name: "shell", spec: contractv2.CommandSpec{ShellScript: strings.Repeat("x", statev2.MaxDiagnosticBytes*2), CwdRepoKey: "app", TimeoutSeconds: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, now := verificationCandidateSnapshot()
			snapshot.Contract.Tasks[0].Verification = []contractv2.CommandSpec{tc.spec}
			state := &verificationStateFake{snapshot: snapshot}
			service := NewVerificationService(state, &verificationRunnerFake{}, matchingVerificationGit(), "operator-1", func() time.Time { return now })
			if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err != nil {
				t.Fatal(err)
			}
			for _, label := range state.request.Task.Gate.Commands {
				if len(label) > statev2.MaxDiagnosticBytes {
					t.Fatalf("label length=%d", len(label))
				}
			}
		})
	}
}

func TestVerificationServiceDoesNotPersistCredentialShapedCommandValues(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	snapshot.Contract.Tasks[0].Verification = []contractv2.CommandSpec{
		{Argv: []string{"tool", "--token", "sentinel-token", "password=sentinel-password", "secret=sentinel-secret"}, CwdRepoKey: "app", TimeoutSeconds: 1},
		{ShellScript: "tool --token sentinel-token password=sentinel-password secret=sentinel-secret", CwdRepoKey: "app", TimeoutSeconds: 1},
	}
	state := &verificationStateFake{snapshot: snapshot}
	service := NewVerificationService(state, &verificationRunnerFake{}, matchingVerificationGit(), "operator-1", func() time.Time { return now })
	if _, err := service.VerifyCandidate(context.Background(), snapshot, "task-1"); err != nil {
		t.Fatal(err)
	}
	serialized := strings.Join(append(state.request.Task.Gate.Commands, state.request.Task.Gate.Diagnostic), "|")
	for _, secret := range []string{"--token", "password=", "secret", "sentinel-token", "sentinel-password", "sentinel-secret"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("persisted secret-shaped value %q in %q", secret, serialized)
		}
	}
}

func TestVerificationServiceInvalidCommandBindingsRecordFailedGateWithoutProcess(t *testing.T) {
	tests := []struct {
		name string
		spec contractv2.CommandSpec
	}{
		{name: "cwd repo mismatch", spec: contractv2.CommandSpec{Argv: []string{"go", "test"}, CwdRepoKey: "other", TimeoutSeconds: 1}},
		{name: "zero timeout", spec: contractv2.CommandSpec{Argv: []string{"go", "test"}, CwdRepoKey: "app"}},
		{name: "argv and shell", spec: contractv2.CommandSpec{Argv: []string{"go", "test"}, ShellScript: "go test", CwdRepoKey: "app", TimeoutSeconds: 1}},
		{name: "neither argv nor shell", spec: contractv2.CommandSpec{CwdRepoKey: "app", TimeoutSeconds: 1}},
		{name: "blank executable", spec: contractv2.CommandSpec{Argv: []string{"   ", "test"}, CwdRepoKey: "app", TimeoutSeconds: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, now := verificationCandidateSnapshot()
			snapshot.Contract.Tasks[0].Verification = []contractv2.CommandSpec{tc.spec}
			state := &verificationStateFake{snapshot: snapshot}
			process := &verificationRunnerFake{}
			service := NewVerificationService(state, process, matchingVerificationGit(), "operator-1", func() time.Time { return now })
			got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
			if err != nil || got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.request.Task.Gate.Passed {
				t.Fatalf("got=%#v err=%v gate=%#v", got, err, state.request.Task.Gate)
			}
			if len(process.calls) != 0 {
				t.Fatalf("process calls=%d", len(process.calls))
			}
		})
	}
}

func TestVerificationServiceTimeoutStopsLaterCommandsAndRecordsNotRun(t *testing.T) {
	snapshot, now := verificationCandidateSnapshot()
	state := &verificationStateFake{snapshot: snapshot}
	process := &verificationRunnerFake{errs: []error{context.DeadlineExceeded}}
	service := NewVerificationService(state, process, matchingVerificationGit(), "operator-1", func() time.Time { return now })
	got, err := service.VerifyCandidate(context.Background(), snapshot, "task-1")
	if err != nil || got.TaskStates["task-1"].Status != statev2.TaskGateFailed || state.request.Task.Gate.Passed {
		t.Fatalf("got=%#v err=%v gate=%#v", got, err, state.request.Task.Gate)
	}
	if len(process.calls) != 1 || strings.Join(state.request.Task.Gate.Outcomes, "|") != "failed|not_run" {
		t.Fatalf("process calls=%d outcomes=%v", len(process.calls), state.request.Task.Gate.Outcomes)
	}
}

func matchingVerificationGit() *verificationGitFake {
	return &verificationGitFake{inspections: []worktree.TaskWorktreeInspection{
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true},
		{CanonicalPath: "/managed/work-1", Exists: true, IdentityMatches: true},
	}}
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
