package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

type parallelHarness struct {
	*harness
	reviewFailures          int
	ciFailures              int
	changedFiles            []string
	mergeConflict           bool
	stallRecoveries         int
	stallMode               bool
	protectedRiskCategories []string
	parallelGH              *parallelGitHub
	parallelHD              *parallelHerdr
	parallelGit             *parallelGit
}

func (h *parallelHarness) mustLoadRun() state.RunSnapshot {
	h.t.Helper()
	for id := range h.orchestrator.runs {
		return h.mustLoad(id)
	}
	h.t.Fatal("run missing")
	return state.RunSnapshot{}
}

type parallelHerdr struct {
	*fakeHerdr
	harness         *parallelHarness
	reviewerPath    string
	evidenceReads   int
	promptIDs       map[string]string
	evidenceByAgent map[string]int
	forceWorking    bool
	createErr       error
	resumes         []herdr.ResumeAgentRequest
}

type parallelGit struct {
	*fakeGit
	harness     *parallelHarness
	remoteSHA   string
	abortMerges int
}

type parallelGitHub struct {
	*fakeGitHub
	harness                   *parallelHarness
	pr                        github.PullRequest
	projectStatuses           []string
	projectStatus             string
	projectItemID             string
	draftCalls                int
	mergeCalls                int
	readyCalls                int
	mergeableUnknown          int
	projectObserveCalls       int
	projectAddCalls           int
	projectUpdateCalls        int
	checkCalls                int
	projectAddResponseLost    bool
	projectUpdateResponseLost bool
	projectObservation        *github.ProjectStatus
	protectedComments         []string
}

func newParallelHarness(t *testing.T) *parallelHarness {
	t.Helper()
	base := newHarness(t)
	ph := &parallelHarness{harness: base, changedFiles: []string{"src/payments/retry.go", "tests/payments/retry_test.go"}}
	ph.herdr = &fakeHerdr{findWorktreeExists: true, recent: builderTranscriptSecret + " arbitrary transcript", evidence: herdr.Evidence{CommitSHA: validSHA, Verification: []herdr.VerificationCheck{{Command: "go test ./internal/payments", Outcome: "passed", Duration: "1.2s"}}}}
	ph.git = &fakeGit{reconcileExists: true, inspection: worktree.CommitInspection{CommitSHA: validSHA, Branch: "agent/api", ChangedFiles: append([]string(nil), ph.changedFiles...), Patch: "bounded patch"}}
	ph.github = &fakeGitHub{}
	ph.Deps = base.Deps
	hd := &parallelHerdr{fakeHerdr: ph.herdr, harness: ph}
	gg := &parallelGit{fakeGit: ph.git, harness: ph}
	gh := &parallelGitHub{fakeGitHub: ph.github, harness: ph}
	ph.Deps.Herdr, ph.Deps.Worktree, ph.Deps.Git, ph.Deps.GitHub = hd, gg, gg, gh
	ph.parallelGH = gh
	ph.parallelHD = hd
	ph.parallelGit = gg
	ph.herdr = hd.fakeHerdr
	ph.git = gg.fakeGit
	ph.github = gh.fakeGitHub
	ph.orchestrator = NewParallel(ph.Deps)
	return ph
}

func (h *parallelHarness) runToStable() contract.RunPhase {
	h.t.Helper()
	h.stallMode = h.stallRecoveries > 0
	if len(h.protectedRiskCategories) > 0 {
		file, err := os.Open(h.contractPath)
		if err != nil {
			h.t.Fatal(err)
		}
		c, err := contract.Read(file)
		_ = file.Close()
		if err != nil {
			h.t.Fatal(err)
		}
		c.RiskCategories = append([]string(nil), h.protectedRiskCategories...)
		var encoded bytes.Buffer
		if err := contract.Write(&encoded, c); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(h.contractPath, encoded.Bytes(), 0600); err != nil {
			h.t.Fatal(err)
		}
	}
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		h.t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		if err := h.orchestrator.Advance(context.Background(), id); err != nil {
			h.t.Fatal(err)
		}
		snapshot := h.mustLoad(id)
		if snapshot.Phase == contract.PhaseCompleted || snapshot.Phase == contract.PhaseBlocked || snapshot.Phase == contract.PhaseNeedsOperator {
			return snapshot.Phase
		}
	}
	h.t.Fatal("120 transitions 안에 안정 상태에 도달하지 못함")
	return ""
}

func (h *parallelHerdr) CreateWorktree(ctx context.Context, request herdr.CreateWorktreeRequest) (herdr.Worktree, error) {
	h.fakeHerdr.worktrees++
	if h.createErr != nil {
		return herdr.Worktree{}, h.createErr
	}
	branch := strings.ReplaceAll(request.Branch, "/", "-")
	return herdr.Worktree{WorkspaceID: "workspace-" + branch, PaneID: "pane-" + branch, Path: "/tmp/" + branch}, nil
}

func (h *parallelHerdr) GetInfo(ctx context.Context, name string) (herdr.AgentInfo, error) {
	info, err := h.fakeHerdr.GetInfo(ctx, name)
	if err != nil {
		return info, err
	}
	for _, taskID := range []string{"api", "tests"} {
		if strings.Contains(name, "-"+taskID) {
			info.WorkspaceID = "workspace-agent-" + taskID
			info.PaneID = "pane-agent-" + taskID
			info.Path = "/tmp/agent-" + taskID
		}
	}
	if strings.HasPrefix(name, "reviewer-") {
		info.WorkspaceID, info.PaneID, info.Path = "workspace-review-184", "pane-review-184", h.reviewerPath
	}
	if h.harness.stallMode {
		info.State = herdr.AgentStateIdle
	}
	if h.evidenceByAgent[name] > 1 {
		info.State = herdr.AgentStateIdle
	}
	if h.forceWorking {
		info.State = herdr.AgentStateWorking
	}
	return info, nil
}

func (h *parallelHerdr) OpenWorktree(_ context.Context, request herdr.OpenWorktreeRequest) (herdr.Worktree, error) {
	h.reviewerPath = request.Path
	return herdr.Worktree{WorkspaceID: "workspace-review-184", PaneID: "pane-review-184", Path: request.Path}, nil
}

func (h *parallelHerdr) ResumeAgent(ctx context.Context, request herdr.ResumeAgentRequest) error {
	h.resumes = append(h.resumes, request)
	return h.fakeHerdr.StartAgent(ctx, herdr.StartAgentRequest{Name: request.Name, PaneID: request.PaneID})
}

func (h *parallelHerdr) ReadEvidence(ctx context.Context, name string) (herdr.Evidence, error) {
	h.evidenceReads++
	if h.evidenceByAgent == nil {
		h.evidenceByAgent = make(map[string]int)
	}
	h.evidenceByAgent[name]++
	if h.harness.stallRecoveries > 0 && strings.Contains(name, "-api") {
		h.harness.stallRecoveries--
		return herdr.Evidence{}, errors.New("agent made no progress")
	}
	evidence := h.fakeHerdr.evidence
	evidence.CommitSHA = "1111111111111111111111111111111111111111"
	if strings.Contains(name, "-tests") {
		evidence.CommitSHA = "2222222222222222222222222222222222222222"
	}
	if h.evidenceByAgent[name] == 2 {
		evidence.CommitSHA = "3333333333333333333333333333333333333333"
	}
	if h.evidenceByAgent[name] >= 3 {
		evidence.CommitSHA = "4444444444444444444444444444444444444444"
	}
	if strings.Contains(name, "-tests") {
		evidence.Verification = []herdr.VerificationCheck{{Command: "go test ./tests/payments", Outcome: "passed", Duration: "1s"}}
	}
	if h.promptIDs != nil {
		evidence.RequestID = h.promptIDs[name]
	}
	if evidence.RequestID == "" {
		evidence.RequestID = h.fakeHerdr.lastPromptRequestID
	}
	if evidence.RequestID == "" {
		evidence.RequestID = strings.TrimPrefix(name, "builder-") + ":prompt"
	}
	return evidence, nil
}

func (h *parallelHerdr) Prompt(ctx context.Context, name, packet string) error {
	if h.promptIDs == nil {
		h.promptIDs = make(map[string]string)
	}
	if marker := "Use requestId="; strings.Contains(packet, marker) {
		value := strings.TrimPrefix(packet[strings.Index(packet, marker):], marker)
		h.promptIDs[name] = strings.TrimSuffix(strings.Fields(value)[0], ".")
	}
	return h.fakeHerdr.Prompt(ctx, name, packet)
}

func (h *parallelHerdr) ReadReviewEvidence(context.Context, string, string) (herdr.ReviewEvidence, error) {
	if h.harness.reviewFailures > 0 {
		h.harness.reviewFailures--
		return herdr.ReviewEvidence{Decision: "block", BlockingFindings: []herdr.ReviewFinding{{ID: "review", Summary: "fix", Paths: []string{"src/payments/retry.go"}}}}, nil
	}
	return herdr.ReviewEvidence{Decision: "accept", BlockingFindings: []herdr.ReviewFinding{}, RiskCategories: []string{}}, nil
}

func (h *parallelGit) InspectCommit(ctx context.Context, path, base, branch, sha string) (worktree.CommitInspection, error) {
	inspection := h.fakeGit.inspection
	inspection.CommitSHA, inspection.Branch = sha, branch
	inspection.ChangedFiles = []string{"src/payments/retry.go"}
	if strings.Contains(branch, "tests") {
		inspection.ChangedFiles = []string{"tests/payments/retry_test.go"}
	}
	if len(h.harness.changedFiles) == 1 {
		inspection.ChangedFiles = append([]string(nil), h.harness.changedFiles...)
	}
	if len(inspection.ChangedFiles) == 0 {
		inspection.ChangedFiles = []string{"src/payments/retry.go"}
	}
	return inspection, nil
}

func (h *parallelGit) MergeCommitNoFF(context.Context, string, string) error {
	if h.harness.mergeConflict {
		return worktree.ErrConflict
	}
	h.fakeGit.merges++
	return nil
}

func (h *parallelGit) AbortMerge(context.Context, string) error { h.abortMerges++; return nil }

func (h *parallelGit) RunChecks(_ context.Context, _ string, commands []string) ([]worktree.VerificationCheck, error) {
	checks := make([]worktree.VerificationCheck, 0, len(commands))
	for _, command := range commands {
		checks = append(checks, worktree.VerificationCheck{Command: command, Outcome: "passed", Duration: "1s", ExitCode: 0})
	}
	return checks, nil
}

func (h *parallelGit) PushBranch(context.Context, string, string, string) error { return nil }

func (h *parallelGit) FetchRemoteHead(context.Context, string, string, string) (string, error) {
	if h.remoteSHA == "" {
		return validSHA, nil
	}
	return h.remoteSHA, nil
}

func (h *parallelGit) IsAncestor(context.Context, string, string) (bool, error) { return true, nil }

func (h *parallelGitHub) CreateDraftPR(_ context.Context, _ github.Repository, request github.DraftPRRequest) (github.PullRequest, error) {
	h.draftCalls++
	h.pr = github.PullRequest{Number: 185, NodeID: "PR_node", HTMLURL: "https://github.example/185", State: "open", Draft: true, Head: request.Head, HeadSHA: validSHA, Base: request.Base}
	return h.pr, nil
}

func (h *parallelGitHub) GetPullRequest(context.Context, github.Repository, int) (github.PullRequest, error) {
	pr := h.pr
	if pr.Number > 0 && pr.NodeID == "" {
		pr.NodeID = "PR_node"
	}
	if h.mergeableUnknown > 0 {
		h.mergeableUnknown--
		return pr, nil
	}
	mergeable := true
	pr.Mergeable = &mergeable
	return pr, nil
}

func (h *parallelGitHub) FindOpenPullRequest(context.Context, github.Repository, string, string) (github.PullRequest, bool, error) {
	if h.pr.Number == 0 {
		return github.PullRequest{}, false, nil
	}
	return h.pr, true, nil
}

func (h *parallelGitHub) MarkReadyForReview(context.Context, github.Repository, string) (github.PullRequest, error) {
	h.readyCalls++
	h.pr.Draft = false
	return h.pr, nil
}

func (h *parallelGitHub) CreateIssueComment(_ context.Context, _ github.Repository, _ int, body string) error {
	h.protectedComments = append(h.protectedComments, body)
	return nil
}

func (h *parallelGitHub) FindIssueComment(_ context.Context, _ github.Repository, _ int, marker string) (bool, error) {
	for _, body := range h.protectedComments {
		if strings.Contains(body, marker) {
			return true, nil
		}
	}
	return false, nil
}

func (h *parallelGitHub) GetChecks(context.Context, github.Repository, string) ([]github.CheckState, error) {
	h.checkCalls++
	if h.harness.ciFailures > 0 {
		h.harness.ciFailures--
		return []github.CheckState{{Name: "go test ./internal/payments", State: "failure"}}, nil
	}
	return []github.CheckState{{Name: "go test ./internal/payments", State: "success"}}, nil
}

func (h *parallelGitHub) MergePullRequest(context.Context, github.Repository, int, string, string) (github.MergePullRequestResult, error) {
	h.mergeCalls++
	sha := fmt.Sprintf("%040x", 2)
	h.pr.Merged, h.pr.State, h.pr.MergeCommitSHA = true, "closed", sha
	return github.MergePullRequestResult{SHA: sha, Merged: true}, nil
}

func (h *parallelGitHub) SetProjectStatus(_ context.Context, _ github.ProjectRef, _ string, status string) error {
	h.projectStatuses = append(h.projectStatuses, "status")
	h.projectStatus = status
	return nil
}

func (h *parallelGitHub) ReadProjectStatus(context.Context, github.ProjectRef, string) (github.ProjectStatus, error) {
	h.projectObserveCalls++
	if h.projectObservation != nil {
		return *h.projectObservation, nil
	}
	if h.projectItemID == "" {
		return github.ProjectStatus{Found: false, ItemPresent: false, StatusPresent: false}, nil
	}
	if h.projectStatus == "" {
		return github.ProjectStatus{Found: false, ItemPresent: true, StatusPresent: false, ItemID: h.projectItemID}, nil
	}
	return github.ProjectStatus{Found: true, ItemPresent: true, StatusPresent: true, ItemID: h.projectItemID, Status: h.projectStatus}, nil
}

func (h *parallelGitHub) AddProjectItem(context.Context, github.ProjectRef, string) (string, error) {
	h.projectAddCalls++
	h.projectItemID = "ITEM_PROJECT"
	if h.projectAddResponseLost {
		return "", errors.New("project add response lost")
	}
	return h.projectItemID, nil
}

func (h *parallelGitHub) UpdateProjectStatus(_ context.Context, _ github.ProjectRef, itemID, status string) error {
	h.projectUpdateCalls++
	if itemID == "" {
		return errors.New("missing project item")
	}
	h.projectItemID, h.projectStatus = itemID, status
	if h.projectUpdateResponseLost {
		return errors.New("project update response lost")
	}
	return nil
}

func (h *parallelGitHub) GetProjectStatus(context.Context, github.ProjectRef, string) (string, error) {
	if len(h.projectStatuses) == 0 {
		return "", errors.New("project status unavailable")
	}
	return h.projectStatus, nil
}
