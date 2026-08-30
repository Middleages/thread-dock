package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/worktree"
)

type parallelHarness struct {
	*harness
	reviewFailures  int
	ciFailures      int
	changedFiles    []string
	mergeConflict   bool
	stallRecoveries int
	parallelGH      *parallelGitHub
}

type parallelHerdr struct {
	*fakeHerdr
	harness      *parallelHarness
	reviewerPath string
}

type parallelGit struct {
	*fakeGit
	harness *parallelHarness
}

type parallelGitHub struct {
	*fakeGitHub
	harness         *parallelHarness
	pr              github.PullRequest
	projectStatuses []string
	draftCalls      int
	mergeCalls      int
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
	ph.herdr = hd.fakeHerdr
	ph.git = gg.fakeGit
	ph.github = gh.fakeGitHub
	ph.orchestrator = NewParallel(ph.Deps)
	return ph
}

func (h *parallelHarness) runToStable() contract.RunPhase {
	h.t.Helper()
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		h.t.Fatal(err)
	}
	if len(h.changedFiles) == 1 && strings.HasPrefix(h.changedFiles[0], "authentication/") {
		for _, runtime := range h.orchestrator.runs {
			for index := range runtime.contract.Tasks {
				runtime.contract.Tasks[index].AllowedPaths = append(runtime.contract.Tasks[index].AllowedPaths, "authentication/**")
			}
		}
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
	return info, nil
}

func (h *parallelHerdr) OpenWorktree(_ context.Context, request herdr.OpenWorktreeRequest) (herdr.Worktree, error) {
	h.reviewerPath = request.Path
	return herdr.Worktree{WorkspaceID: "workspace-review-184", PaneID: "pane-review-184", Path: request.Path}, nil
}

func (h *parallelHerdr) ReadEvidence(ctx context.Context, name string) (herdr.Evidence, error) {
	if h.harness.stallRecoveries > 0 {
		h.harness.stallRecoveries--
		return herdr.Evidence{}, errors.New("agent made no progress")
	}
	evidence := h.fakeHerdr.evidence
	if strings.Contains(name, "-tests") {
		evidence.Verification = []herdr.VerificationCheck{{Command: "go test ./tests/payments", Outcome: "passed", Duration: "1s"}}
	}
	evidence.RequestID = h.fakeHerdr.lastPromptRequestID
	if evidence.RequestID == "" {
		evidence.RequestID = strings.TrimPrefix(name, "builder-") + ":prompt"
	}
	return evidence, nil
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

func (h *parallelGit) AbortMerge(context.Context, string) error { return nil }

func (h *parallelGit) RunChecks(_ context.Context, _ string, commands []string) ([]worktree.VerificationCheck, error) {
	checks := make([]worktree.VerificationCheck, 0, len(commands))
	for _, command := range commands {
		checks = append(checks, worktree.VerificationCheck{Command: command, Outcome: "passed", Duration: "1s", ExitCode: 0})
	}
	return checks, nil
}

func (h *parallelGit) PushBranch(context.Context, string, string, string) error { return nil }

func (h *parallelGitHub) CreateDraftPR(_ context.Context, _ github.Repository, request github.DraftPRRequest) (github.PullRequest, error) {
	h.draftCalls++
	h.pr = github.PullRequest{Number: 185, HTMLURL: "https://github.example/185", State: "open", Draft: true, Head: request.Head, HeadSHA: validSHA, Base: request.Base}
	return h.pr, nil
}

func (h *parallelGitHub) GetPullRequest(context.Context, github.Repository, int) (github.PullRequest, error) {
	pr := h.pr
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

func (h *parallelGitHub) MarkReadyForReview(context.Context, github.Repository, int) (github.PullRequest, error) {
	h.pr.Draft = false
	return h.pr, nil
}

func (h *parallelGitHub) GetChecks(context.Context, github.Repository, string) ([]github.CheckState, error) {
	if h.harness.ciFailures > 0 {
		h.harness.ciFailures--
		return []github.CheckState{{Name: "ci", State: "failure"}}, nil
	}
	return []github.CheckState{{Name: "ci", State: "success"}}, nil
}

func (h *parallelGitHub) MergePullRequest(context.Context, github.Repository, int, string, string) (github.MergePullRequestResult, error) {
	h.mergeCalls++
	return github.MergePullRequestResult{SHA: fmt.Sprintf("%040x", 2), Merged: true}, nil
}

func (h *parallelGitHub) SetProjectStatus(context.Context, github.ProjectRef, string, string) error {
	h.projectStatuses = append(h.projectStatuses, "status")
	return nil
}
