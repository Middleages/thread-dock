package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

type harness struct {
	t            *testing.T
	dir          string
	store        *state.Store
	orchestrator *Orchestrator
	github       *fakeGitHub
	herdr        *fakeHerdr
	git          *fakeGit
	clock        *fakeClock
	contractPath string
	Deps         Dependencies
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	contractPath := filepath.Join(dir, "contract.json")
	writeFixtureContract(t, contractPath)
	store := state.NewStore(filepath.Join(dir, "state"))
	gh := &fakeGitHub{}
	hd := &fakeHerdr{findWorktreeExists: true, recent: builderTranscriptSecret + " arbitrary transcript", evidence: herdr.Evidence{
		CommitSHA:    validSHA,
		Verification: []herdr.VerificationCheck{{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1.2s"}},
	}}
	git := &fakeGit{reconcileExists: true, inspection: worktree.CommitInspection{CommitSHA: validSHA, Branch: "agent/api", ChangedFiles: []string{"src/payments/retry.go"}, Patch: "diff --git a/src/payments/retry.go b/src/payments/retry.go\n+bounded patch\n"}}
	clock := &fakeClock{now: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)}
	deps := Dependencies{Store: store, GitHub: gh, Herdr: hd, Git: git, Clock: clock}
	return &harness{
		t:            t,
		dir:          dir,
		store:        store,
		orchestrator: New(deps),
		github:       gh,
		herdr:        hd,
		git:          git,
		clock:        clock,
		contractPath: contractPath,
		Deps:         deps,
	}
}

func (h *harness) mustLoad(id contract.RunID) state.RunSnapshot {
	h.t.Helper()
	snapshot, err := h.store.Load(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return snapshot
}

func (h *harness) events(id contract.RunID) []state.Event {
	h.t.Helper()
	file, err := os.Open(filepath.Join(h.dir, "state", "runs", string(id), "events.jsonl"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer file.Close()
	var events []state.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event state.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			h.t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		h.t.Fatal(err)
	}
	return events
}

func hasEventMessage(events []state.Event, want string) bool {
	for _, event := range events {
		if event.Message == want || contains(event.Message, want) {
			return true
		}
	}
	return false
}

func contains(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}

func sameDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type fakeGitHub struct {
	issueCalls        int
	temporaryFailures int
	issueErr          error
}

func (f *fakeGitHub) FindIssueBundle(context.Context, github.Repository, string) (github.IssueBundle, bool, error) {
	return github.IssueBundle{}, false, nil
}

func (f *fakeGitHub) CreateIssueBundle(context.Context, github.Repository, contract.TaskContract, string) (github.IssueBundle, error) {
	f.issueCalls++
	if f.temporaryFailures > 0 {
		f.temporaryFailures--
		return github.IssueBundle{}, &github.TemporaryError{StatusCode: 503, Message: "temporary"}
	}
	if f.issueErr != nil {
		return github.IssueBundle{}, f.issueErr
	}
	return github.IssueBundle{Parent: github.Issue{Number: 184, NodeID: "issue-node"}}, nil
}

func (f *fakeGitHub) CreateDraftPR(context.Context, github.Repository, github.DraftPRRequest) (github.PullRequest, error) {
	return github.PullRequest{}, errors.New("unexpected draft PR")
}

func (f *fakeGitHub) UpdateIssueState(context.Context, github.Repository, int, string) error {
	return errors.New("unexpected issue state update")
}

func (f *fakeGitHub) SetProjectStatus(context.Context, github.ProjectRef, string, string) error {
	return errors.New("unexpected project status update")
}

func (f *fakeGitHub) GetPullRequest(context.Context, github.Repository, int) (github.PullRequest, error) {
	return github.PullRequest{}, errors.New("unexpected pull request lookup")
}

type fakeHerdr struct {
	worktrees           int
	starts              []herdr.StartAgentRequest
	prompts             []string
	recent              string
	evidence            herdr.Evidence
	startEntered        chan struct{}
	releaseStart        chan struct{}
	startCount          int
	findWorktreeCalls   int
	findWorktreeCWD     string
	agentSeq            int64
	agentInfoErr        error
	agentInfoOverride   *herdr.AgentInfo
	findWorktreeExists  bool
	promptReceiptReads  int
	lastPromptRequestID string
}

func (f *fakeHerdr) CreateWorktree(context.Context, herdr.CreateWorktreeRequest) (herdr.Worktree, error) {
	f.worktrees++
	return herdr.Worktree{WorkspaceID: "workspace-184", PaneID: "pane-184", Path: "/tmp/builder-184"}, nil
}

func (f *fakeHerdr) StartAgent(_ context.Context, request herdr.StartAgentRequest) error {
	f.starts = append(f.starts, request)
	f.startCount++
	if f.startEntered != nil {
		select {
		case <-f.startEntered:
		default:
			close(f.startEntered)
		}
	}
	if f.releaseStart != nil {
		<-f.releaseStart
	}
	return nil
}

func (f *fakeHerdr) Prompt(_ context.Context, _ string, packet string) error {
	f.prompts = append(f.prompts, packet)
	if marker := "Use requestId="; strings.Contains(packet, marker) {
		f.lastPromptRequestID = strings.TrimSuffix(strings.Fields(strings.TrimPrefix(packet[strings.Index(packet, marker):], marker))[0], ".")
	}
	return nil
}

func (f *fakeHerdr) Get(context.Context, string) (herdr.AgentState, error) {
	return herdr.AgentStateDone, nil
}

func (f *fakeHerdr) ReadRecent(context.Context, string) (string, error) {
	return f.recent, nil
}

func (f *fakeHerdr) ReadEvidence(_ context.Context, name string) (herdr.Evidence, error) {
	if f.evidence.RequestID == "" {
		f.evidence.RequestID = f.lastPromptRequestID
		if f.evidence.RequestID == "" {
			f.evidence.RequestID = strings.TrimPrefix(name, "builder-") + ":builder-prompt"
		}
	}
	return f.evidence, nil
}

func (f *fakeHerdr) OpenWorktree(_ context.Context, request herdr.OpenWorktreeRequest) (herdr.Worktree, error) {
	return herdr.Worktree{WorkspaceID: "workspace-review-184", PaneID: "pane-review-184", Path: request.Path}, nil
}

func (f *fakeHerdr) FindWorktree(_ context.Context, cwd, _, _ string) (herdr.Worktree, bool, error) {
	f.findWorktreeCalls++
	f.findWorktreeCWD = cwd
	if !f.findWorktreeExists {
		return herdr.Worktree{}, false, nil
	}
	return herdr.Worktree{WorkspaceID: "workspace-184", PaneID: "pane-184", Path: "/tmp/integration"}, true, nil
}

func (f *fakeHerdr) GetInfo(_ context.Context, name string) (herdr.AgentInfo, error) {
	if f.agentInfoErr != nil {
		return herdr.AgentInfo{}, f.agentInfoErr
	}
	if f.agentInfoOverride != nil {
		return *f.agentInfoOverride, nil
	}
	seq := f.agentSeq
	if seq == 0 {
		seq = 42
	}
	workspaceID, paneID := "workspace-184", "pane-184"
	if strings.HasPrefix(name, "reviewer-") {
		workspaceID, paneID = "workspace-review-184", "pane-review-184"
	}
	return herdr.AgentInfo{Name: name, SessionID: "session-" + name, PaneID: paneID, WorkspaceID: workspaceID, State: herdr.AgentStateWorking, StateChangeSeq: seq}, nil
}

func (f *fakeHerdr) ReadPromptReceipt(_ context.Context, name, requestID string) (herdr.AgentInfo, bool, error) {
	f.promptReceiptReads++
	seq := f.agentSeq
	if seq == 0 {
		seq = 42
	}
	return herdr.AgentInfo{Name: name, StateChangeSeq: seq}, strings.Contains(f.recent, requestID), nil
}

type fakeGit struct {
	merges          int
	mergedBranch    string
	mergeSHAs       []string
	creates         int
	integrationPath string
	inspection      worktree.CommitInspection
	reconcileExists bool
	reconcileCalls  int
	currentCommit   string
}

func (f *fakeGit) Create(_ context.Context, _, worktreePath, _, _ string) error {
	f.creates++
	f.integrationPath = worktreePath
	return nil
}

func (f *fakeGit) Status(context.Context, string) (string, error) { return "", nil }

func (f *fakeGit) Commit(context.Context, string, string) (string, error) { return validSHA, nil }

func (f *fakeGit) Merge(_ context.Context, _ string, branch string) error {
	f.merges++
	f.mergedBranch = branch
	return nil
}

func (f *fakeGit) InspectCommit(context.Context, string, string, string, string) (worktree.CommitInspection, error) {
	return f.inspection, nil
}

func (f *fakeGit) MergeCommit(_ context.Context, _ string, sha string) error {
	f.merges++
	f.mergeSHAs = append(f.mergeSHAs, sha)
	return nil
}

func (f *fakeGit) CurrentCommit(context.Context, string) (string, error) {
	if f.currentCommit != "" {
		return f.currentCommit, nil
	}
	return validSHA, nil
}

func (f *fakeGit) ReconcileIntegrationWorktree(context.Context, string, string, string) (bool, error) {
	f.reconcileCalls++
	return f.reconcileExists, nil
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(_ context.Context, delay time.Duration) error {
	f.sleeps = append(f.sleeps, delay)
	f.now = f.now.Add(delay)
	return nil
}
