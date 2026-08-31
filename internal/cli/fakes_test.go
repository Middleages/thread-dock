package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/herdr"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
)

type fakeRunService struct {
	view       contract.StatusView
	startID    contract.RunID
	startErr   error
	statusErr  error
	stopErr    error
	resumeErr  error
	cleanupErr error
	calls      []string
}

func (f *fakeRunService) Start(_ context.Context, path string) (contract.RunID, error) {
	f.calls = append(f.calls, "start:"+path)
	if f.startID == "" {
		f.startID = "run-184"
	}
	if f.startErr != nil {
		return f.startID, f.startErr
	}
	return f.startID, nil
}

func (f *fakeRunService) Status(_ context.Context, id contract.RunID) (contract.StatusView, error) {
	f.calls = append(f.calls, "status:"+string(id))
	if f.statusErr != nil {
		return contract.StatusView{}, f.statusErr
	}
	view := f.view
	if view.RunID == "" {
		view.RunID = id
	}
	return view, nil
}

func (f *fakeRunService) Stop(_ context.Context, id contract.RunID) error {
	f.calls = append(f.calls, "stop:"+string(id))
	return f.stopErr
}

func (f *fakeRunService) Resume(_ context.Context, id contract.RunID) error {
	f.calls = append(f.calls, "resume:"+string(id))
	return f.resumeErr
}

func (f *fakeRunService) Cleanup(_ context.Context, id contract.RunID) error {
	f.calls = append(f.calls, "cleanup:"+string(id))
	return f.cleanupErr
}

type cliHarness struct {
	service *fakeRunService
}

func newCLIHarness(t *testing.T) *cliHarness {
	t.Helper()
	return &cliHarness{service: &fakeRunService{view: contract.StatusView{
		ContractVersion: 1,
		RunID:           "run-184",
		Phase:           contract.PhaseReviewing,
		Summary:         "독립 확인 중",
		NextAction:      &contract.NextAction{Kind: "review", Label: "Reviewer 결과 확인"},
		Agents:          []contract.AgentView{{Name: "reviewer-184", Role: "reviewer", State: "working", Summary: "확인 중"}},
		GitHub:          contract.GitHubView{ParentIssue: 184, PullRequest: 185, URL: "https://github.example/184", CI: "pending"},
		UpdatedAt:       time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC),
	}}}
}

func (h *cliHarness) Run(args []string, out, errOut interface{ Write([]byte) (int, error) }) int {
	return RunWithDependencies(context.Background(), args, out, errOut, Dependencies{Runs: h.service})
}

var errFakeRun = errors.New("fake run failure")

type fakeCoordinator struct {
	advanceCalls int
	stopCalls    int
	advanceErr   error
}

func (f *fakeCoordinator) Start(context.Context, string) (contract.RunID, error) {
	return "run-184", nil
}

func (f *fakeCoordinator) Advance(context.Context, contract.RunID) error {
	f.advanceCalls++
	return f.advanceErr
}

func (f *fakeCoordinator) Stop(context.Context, contract.RunID) error {
	f.stopCalls++
	return nil
}

type fakeStateStore struct {
	snapshot  state.RunSnapshot
	saves     []state.RunSnapshot
	events    []state.Event
	runs      []state.RunSnapshot
	appendErr error
	saveErr   error
}

func (f *fakeStateStore) Load(context.Context, contract.RunID) (state.RunSnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeStateStore) Save(_ context.Context, snapshot state.RunSnapshot) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.snapshot = snapshot
	f.saves = append(f.saves, snapshot)
	return nil
}

func (f *fakeStateStore) Append(_ context.Context, _ contract.RunID, event state.Event) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.events = append(f.events, event)
	return nil
}

func (f *fakeStateStore) ListRecoverable(context.Context) ([]state.RunSnapshot, error) {
	return f.runs, nil
}

type fakeCleanup struct {
	statuses       map[string]string
	statusCalls    []string
	removed        []string
	herdrRemoved   []string
	herdrRemoveErr error
	events         []string
	foundWorktrees map[string]herdr.Worktree
	findErr        error
	findCalls      []findWorktreeCall
}

type findWorktreeCall struct {
	cwd, path, label string
}

func (f *fakeCleanup) Status(_ context.Context, path string) (string, error) {
	f.statusCalls = append(f.statusCalls, path)
	f.events = append(f.events, "status:"+path)
	return f.statuses[path], nil
}

func (f *fakeCleanup) RemoveSafe(_ context.Context, _, _, path string) error {
	f.removed = append(f.removed, path)
	f.events = append(f.events, "remove-git:"+path)
	return nil
}

func (f *fakeCleanup) RemoveHerdr(_ context.Context, workspaceID string) error {
	f.herdrRemoved = append(f.herdrRemoved, workspaceID)
	f.events = append(f.events, "remove-herdr:"+workspaceID)
	return f.herdrRemoveErr
}

func (f *fakeCleanup) FindWorktree(_ context.Context, cwd, path, label string) (herdr.Worktree, bool, error) {
	f.findCalls = append(f.findCalls, findWorktreeCall{cwd: cwd, path: path, label: label})
	f.events = append(f.events, "find:"+path)
	if f.findErr != nil {
		return herdr.Worktree{}, false, f.findErr
	}
	if f.foundWorktrees == nil {
		return herdr.Worktree{}, false, nil
	}
	work, ok := f.foundWorktrees[path]
	if !ok {
		return herdr.Worktree{}, false, nil
	}
	return work, true, nil
}

func (f *fakeCleanup) TrustedHerdrWorktreeRoot() string {
	return "/home/operator/.herdr/worktrees"
}

type recordedProcessCall struct {
	cwd, executable string
	args            []string
}

type recordedProcessRunner struct {
	calls  []recordedProcessCall
	err    error
	result runner.Result
}

func (r *recordedProcessRunner) Run(_ context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	r.calls = append(r.calls, recordedProcessCall{cwd: cwd, executable: executable, args: append([]string(nil), args...)})
	if r.result.ExitCode == 0 && r.result.Stdout == "" && r.result.Stderr == "" {
		r.result.ExitCode = 0
	}
	return r.result, r.err
}

type fakeStateRemover struct {
	calls []contract.RunID
}

func (f *fakeStateRemover) Remove(_ context.Context, id contract.RunID) error {
	f.calls = append(f.calls, id)
	return nil
}
