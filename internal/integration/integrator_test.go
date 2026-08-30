package integration

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

const (
	apiSHA   = "0123456789abcdef0123456789abcdef01234567"
	testsSHA = "89abcdef0123456789abcdef0123456789abcdef"
)

func TestValidateResultRejectsChangedPathOutsideOwnership(t *testing.T) {
	owned := contract.Task{ID: "api", Branch: "agent/api", AllowedPaths: []string{"src/payments/**"}, Verification: []string{"go test ./..."}}
	evidence := validEvidence(apiSHA)
	inspection := worktree.CommitInspection{CommitSHA: apiSHA, Branch: owned.Branch, ChangedFiles: []string{"src/auth/token.go"}, Patch: "bounded"}
	if err := ValidateResult(owned, evidence, inspection); !errors.Is(err, ErrPathOwnership) {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateResultRejectsUnsafeSHAOrBranch(t *testing.T) {
	tests := []struct {
		name       string
		sha        string
		branch     string
		inspection string
	}{
		{name: "uppercase sha", sha: strings.ToUpper(apiSHA), branch: "agent/api", inspection: apiSHA},
		{name: "short sha", sha: "deadbeef", branch: "agent/api", inspection: apiSHA},
		{name: "branch mismatch", sha: apiSHA, branch: "agent/wrong", inspection: apiSHA},
		{name: "inspection sha mismatch", sha: apiSHA, branch: "agent/api", inspection: testsSHA},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := contract.Task{ID: "api", Branch: "agent/api", AllowedPaths: []string{"src/**"}, Verification: []string{"go test ./..."}}
			evidence := validEvidence(tc.sha)
			inspection := worktree.CommitInspection{CommitSHA: tc.inspection, Branch: tc.branch, ChangedFiles: []string{"src/main.go"}, Patch: "bounded"}
			if err := ValidateResult(task, evidence, inspection); err == nil {
				t.Fatal("expected unsafe result rejection")
			}
		})
	}
}

func TestValidateResultRequiresExactVerificationEvidence(t *testing.T) {
	task := contract.Task{ID: "api", Branch: "agent/api", AllowedPaths: []string{"src/**"}, Verification: []string{"go test ./...", "go vet ./..."}}
	base := validEvidence(apiSHA)
	tests := []struct {
		name   string
		events []state.VerificationEvidence
	}{
		{name: "missing command", events: base.VerificationEvidence[:1]},
		{name: "wrong outcome", events: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "failed", Duration: "1s"}, {Command: "go vet ./...", Outcome: "passed", Duration: "1s"}}},
		{name: "invalid duration", events: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "soon"}, {Command: "go vet ./...", Outcome: "passed", Duration: "1s"}}},
		{name: "unknown command", events: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}, {Command: "go test ./unit", Outcome: "passed", Duration: "1s"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evidence := base
			evidence.VerificationEvidence = tc.events
			inspection := worktree.CommitInspection{CommitSHA: apiSHA, Branch: task.Branch, ChangedFiles: []string{"src/main.go"}, Patch: "bounded"}
			if err := ValidateResult(task, evidence, inspection); err == nil {
				t.Fatal("expected evidence rejection")
			}
		})
	}
}

func TestValidateResultRejectsNonCanonicalBranchEvidenceAndDurations(t *testing.T) {
	baseTask := contract.Task{ID: "api", Branch: "agent/api", AllowedPaths: []string{"src/**"}, Verification: []string{"go test ./..."}}
	baseInspection := worktree.CommitInspection{CommitSHA: apiSHA, Branch: "agent/api", ChangedFiles: []string{"src/main.go"}, Patch: "bounded"}
	tests := []struct {
		name     string
		task     contract.Task
		inspect  worktree.CommitInspection
		evidence state.AgentEvidence
	}{
		{name: "task branch leading whitespace", task: func() contract.Task { task := baseTask; task.Branch = " agent/api"; return task }(), inspect: baseInspection, evidence: validEvidence(apiSHA)},
		{name: "inspection branch trailing whitespace", task: baseTask, inspect: func() worktree.CommitInspection {
			inspection := baseInspection
			inspection.Branch = "agent/api "
			return inspection
		}(), evidence: validEvidence(apiSHA)},
		{name: "evidence command leading whitespace", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: " go test ./...", Outcome: "passed", Duration: "1s"}}}},
		{name: "outcome whitespace", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: " passed", Duration: "1s"}}}},
		{name: "outcome case", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "PASSED", Duration: "1s"}}}},
		{name: "duration leading whitespace", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: " 1s"}}}},
		{name: "duration trailing whitespace", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s "}}}},
		{name: "zero duration", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "0s"}}}},
		{name: "negative duration", task: baseTask, inspect: baseInspection, evidence: state.AgentEvidence{CommitSHA: apiSHA, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "-1s"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateResult(tc.task, tc.evidence, tc.inspect); err == nil {
				t.Fatal("expected non-canonical evidence rejection")
			}
		})
	}
}

func TestValidateResultUsesVerificationMultiset(t *testing.T) {
	task := contract.Task{ID: "api", Branch: "agent/api", AllowedPaths: []string{"src/**"}, Verification: []string{"go test ./...", "go test ./..."}}
	evidence := validEvidence(apiSHA)
	evidence.VerificationEvidence = []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}, {Command: "go test ./...", Outcome: "passed", Duration: "2s"}}
	inspection := worktree.CommitInspection{CommitSHA: apiSHA, Branch: task.Branch, ChangedFiles: []string{"src/main.go"}, Patch: "bounded"}
	if err := ValidateResult(task, evidence, inspection); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateResultIgnoresAgentReportedPathsAndPatch(t *testing.T) {
	task := contract.Task{ID: "api", Branch: "agent/api", AllowedPaths: []string{"src/**"}, Verification: []string{"go test ./...", "go vet ./..."}}
	evidence := validEvidence(apiSHA)
	evidence.ChangedFiles = []string{"outside/agent-report.go"}
	evidence.Patch = "agent supplied raw output"
	inspection := worktree.CommitInspection{CommitSHA: apiSHA, Branch: task.Branch, ChangedFiles: []string{"src/main.go"}, Patch: "git-derived"}
	if err := ValidateResult(task, evidence, inspection); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestMergeResultsProcessesImmutableCommitsInOrder(t *testing.T) {
	git := &fakeGit{currentCommit: testsSHA, checks: []worktree.VerificationCheck{{Command: "go test ./...", Outcome: "passed", Duration: "1ms", ExitCode: 0}}}
	integrator := New(git)
	results := []Result{{TaskID: "api", CommitSHA: apiSHA}, {TaskID: "tests", CommitSHA: testsSHA}}
	got, err := integrator.MergeResults(context.Background(), "/work/integration", results, []string{"go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(git.merged, []string{apiSHA, testsSHA}) {
		t.Fatalf("merged=%v", git.merged)
	}
	if got.CommitSHA != testsSHA || len(got.Verification) != 1 || got.Verification[0].Command != "go test ./..." {
		t.Fatalf("result=%#v", got)
	}
}

func TestMergeResultsStopsOnConfirmedConflictAndAborts(t *testing.T) {
	git := &fakeGit{mergeErr: worktree.ErrConflict}
	_, err := New(git).MergeResults(context.Background(), "/work/integration", []Result{{TaskID: "api", CommitSHA: apiSHA}, {TaskID: "tests", CommitSHA: testsSHA}}, nil)
	if !errors.Is(err, ErrBlockedConflict) || len(git.merged) != 1 || git.aborts != 1 {
		t.Fatalf("err=%v merged=%v aborts=%d", err, git.merged, git.aborts)
	}
}

func TestMergeResultsReturnsAbortFailure(t *testing.T) {
	abortErr := errors.New("abort failed")
	git := &fakeGit{mergeErr: worktree.ErrConflict, abortErr: abortErr}
	_, err := New(git).MergeResults(context.Background(), "/work/integration", []Result{{TaskID: "api", CommitSHA: apiSHA}}, nil)
	if !errors.Is(err, abortErr) || !errors.Is(err, ErrBlockedConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestMergeResultsReturnsFailedCheckWithoutRawOutput(t *testing.T) {
	git := &fakeGit{currentCommit: testsSHA, checks: []worktree.VerificationCheck{{Command: "go test ./...", Outcome: "failed", Duration: "2s", ExitCode: 1}}}
	got, err := New(git).MergeResults(context.Background(), "/work/integration", []Result{{TaskID: "api", CommitSHA: apiSHA}}, []string{"go test ./..."})
	if err != nil || len(got.Verification) != 1 || got.Verification[0].Outcome != "failed" || got.Verification[0].ExitCode != 1 {
		t.Fatalf("result=%#v err=%v", got, err)
	}
	if strings.Contains(strings.Join([]string{got.Verification[0].Command, got.Verification[0].Outcome, got.Verification[0].Duration}, " "), "secret") {
		t.Fatal("raw output leaked into result")
	}
}

func TestMergeResultsReturnsCheckInfrastructureError(t *testing.T) {
	checkErr := errors.New("runner unavailable")
	git := &fakeGit{checkErr: checkErr}
	_, err := New(git).MergeResults(context.Background(), "/work/integration", []Result{{TaskID: "api", CommitSHA: apiSHA}}, []string{"go test ./..."})
	if !errors.Is(err, checkErr) {
		t.Fatalf("err=%v", err)
	}
}

func validEvidence(sha string) state.AgentEvidence {
	return state.AgentEvidence{CommitSHA: sha, VerificationEvidence: []state.VerificationEvidence{{Command: "go test ./...", Outcome: "passed", Duration: "1s"}, {Command: "go vet ./...", Outcome: "passed", Duration: "1s"}}}
}

type fakeGit struct {
	mergeErr, abortErr, checkErr error
	merged                       []string
	aborts                       int
	checks                       []worktree.VerificationCheck
	currentCommit                string
}

func (f *fakeGit) MergeCommitNoFF(_ context.Context, _ string, sha string) error {
	f.merged = append(f.merged, sha)
	return f.mergeErr
}

func (f *fakeGit) AbortMerge(context.Context, string) error {
	f.aborts++
	return f.abortErr
}

func (f *fakeGit) RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error) {
	return f.checks, f.checkErr
}

func (f *fakeGit) CurrentCommit(context.Context, string) (string, error) {
	return f.currentCommit, nil
}
