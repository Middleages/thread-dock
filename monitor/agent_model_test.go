package main

import (
	"strings"
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

func TestNormalizeExecutionStateDefaultsUnknown(t *testing.T) {
	if got := NormalizeExecutionState(""); got != ExecutionUnknown {
		t.Fatalf("NormalizeExecutionState(\"\") = %q, want %q", got, ExecutionUnknown)
	}
	if got := NormalizeExecutionState("DONE"); got != ExecutionCompleted {
		t.Fatalf("NormalizeExecutionState(\"DONE\") = %q, want %q", got, ExecutionCompleted)
	}
	if got := NormalizeExecutionState("running"); got != ExecutionRunning {
		t.Fatalf("NormalizeExecutionState(\"running\") = %q, want %q", got, ExecutionRunning)
	}
}

func TestNormalizeVerificationStateDoesNotInferSuccess(t *testing.T) {
	state := NormalizeVerificationState(VerificationState{
		Status:    "",
		Codes:     nil,
		AnchorSHA: "abc1234",
	})
	if state.Status != VerificationUnverified {
		t.Fatalf("status = %q, want %q", state.Status, VerificationUnverified)
	}
	if state.AnchorSHA != "abc1234" {
		t.Fatalf("anchor sha = %q, want abc1234", state.AnchorSHA)
	}
}

func TestVerificationEvidenceMatchesCandidateSHA(t *testing.T) {
	state := VerificationState{
		Status:    VerificationUnverified,
		AnchorSHA: "abc1234567890abcdef1234567890abcdef1234",
	}
	evidence := EvidenceItem{Type: "review", Reference: "comment:1", Revision: "abc1234567890abcdef1234567890abcdef1234", Claim: "ACCEPT"}
	if !EvidenceMatchesCandidate(state, evidence) {
		t.Fatal("matching candidate SHA should match evidence")
	}

	// Short SHA matching full anchor SHA (min 7 chars)
	shortEvidence := EvidenceItem{Type: "review", Reference: "comment:2", Revision: "abc1234", Claim: "ACCEPT"}
	if !EvidenceMatchesCandidate(state, shortEvidence) {
		t.Fatal("7-char short SHA prefix should match full candidate SHA")
	}

	// Case-insensitive matching
	upperEvidence := EvidenceItem{Type: "review", Reference: "comment:3", Revision: "ABC1234", Claim: "ACCEPT"}
	if !EvidenceMatchesCandidate(state, upperEvidence) {
		t.Fatal("uppercase SHA should match lowercase candidate SHA")
	}

	// Sub-7 char prefix must not match
	tooShort := EvidenceItem{Type: "review", Reference: "comment:4", Revision: "abc12", Claim: "ACCEPT"}
	if EvidenceMatchesCandidate(state, tooShort) {
		t.Fatal("sub-7 char prefix must not match candidate SHA")
	}

	// Stale SHA must not match
	stale := evidence
	stale.Revision = "def4567890abcdef1234567890abcdef12345678"
	if EvidenceMatchesCandidate(state, stale) {
		t.Fatal("stale evidence must not match candidate SHA")
	}
}

func TestNormalizeVerificationStateRemovesSuccessCodeAndDedupes(t *testing.T) {
	state := NormalizeVerificationState(VerificationState{
		Status: VerificationVerified,
		Codes:  []string{"fully_verified", "ci_passed", "ci_passed", "reviewer_accepted"},
	})
	for _, code := range state.Codes {
		if code == "fully_verified" {
			t.Fatal("fully_verified must not be a verification reason code")
		}
	}
	if state.Status != VerificationVerified {
		t.Fatalf("status = %q, want verified", state.Status)
	}
	if len(state.Codes) != 2 || state.Codes[0] != "ci_passed" || state.Codes[1] != "reviewer_accepted" {
		t.Fatalf("codes = %v, want [ci_passed reviewer_accepted]", state.Codes)
	}
}

func TestAgentLineagePreservesParentChildIdentity(t *testing.T) {
	root := AgentObservation{AgentID: "root", Role: "coordinator"}
	child := AgentObservation{AgentID: "child", ParentAgentID: root.AgentID, Role: "reviewer"}
	if child.ParentAgentID != root.AgentID {
		t.Fatalf("child parent = %q, want %q", child.ParentAgentID, root.AgentID)
	}
	if child.AgentID == child.ExecutionRef {
		t.Fatal("logical agent identity must remain separate from execution reference")
	}
}

func TestHerdrCompletionDoesNotCompleteGitHubWork(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		WorkID: "issue:42",
		Title:  "Implement agent read model",
		State:  "open",
		GitHub: &GitHubWork{
			Kind:   "issue",
			Number: intPtr(42),
			URL:    "https://github.com/acme/app/issues/42",
			State:  "open",
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/issues/42",
				Session:     "feature-agent",
				AgentName:   "Codex",
				Role:        "implementer",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow should not be nil")
	}
	if workflow.ExecutionState != ExecutionCompleted {
		t.Fatalf("executionState = %q, want %q", workflow.ExecutionState, ExecutionCompleted)
	}
	if workflow.GitHubWorkState != "open" {
		t.Fatalf("githubWorkState = %q, want %q (Herdr completion must not change GitHub work state)", workflow.GitHubWorkState, "open")
	}
	if work.State != "open" {
		t.Fatalf("work.State = %q, want %q", work.State, "open")
	}
	if workflow.VerificationState.Status == VerificationVerified {
		t.Fatal("Herdr execution completion must not infer VerificationVerified")
	}
}

func TestDeterministicIssueAndPRCorrelation(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		WorkID: "issue:42",
		GitHub: &GitHubWork{
			Kind: "issue",
			URL:  "https://github.com/acme/app/issues/42",
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/issues/42",
				Session:     "task-42",
				AgentName:   "Worker",
				AgentStatus: "working",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("expected matched workflow")
	}
	if workflow.Provenance.Scope != "issue" {
		t.Fatalf("scope = %q, want %q", workflow.Provenance.Scope, "issue")
	}
	if len(workflow.Agents) != 1 || workflow.Agents[0].AgentID != "Worker" {
		t.Fatalf("agents = %#v", workflow.Agents)
	}
	if workflow.ExecutionState != ExecutionRunning {
		t.Fatalf("executionState = %q, want %q", workflow.ExecutionState, ExecutionRunning)
	}
}

func TestProjectLevelAndCoordinatorFallback(t *testing.T) {
	now := time.Now()
	project := Project{
		Links: []Link{
			{Kind: "github", URL: "https://github.com/acme/app"},
		},
	}
	work := WorkItem{
		WorkID: "legacy:1",
		State:  "open",
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				Role:        "coordinator",
				Repository:  "acme/app",
				Session:     "main-session",
				AgentName:   "Coordinator",
				AgentStatus: "working",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(&project, &work, &herdr)
	if workflow == nil {
		t.Fatal("coordinator fallback should match repo")
	}
	if workflow.Provenance.Scope != "coordinator" {
		t.Fatalf("scope = %q, want %q", workflow.Provenance.Scope, "coordinator")
	}
	if len(workflow.Agents) != 1 || workflow.Agents[0].Role != "coordinator" {
		t.Fatalf("agents = %#v", workflow.Agents)
	}
}

func TestAmbiguityHandlingMultipleSessions(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		GitHub: &GitHubWork{
			URL: "https://github.com/acme/app/issues/99",
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/issues/99",
				Session:     "session-alpha",
				AgentName:   "AgentA",
				AgentStatus: "working",
			},
			{
				IssueURL:    "https://github.com/acme/app/issues/99",
				Session:     "session-beta",
				AgentName:   "AgentB",
				AgentStatus: "idle",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow should match")
	}
	if len(workflow.Notices) == 0 {
		t.Fatal("expected ambiguity notice for multiple sessions")
	}
	if !strings.Contains(workflow.Notices[0], "모호") {
		t.Fatalf("notice = %q, want ambiguity notice", workflow.Notices[0])
	}
}

func TestLegacyNoAgentFallback(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		WorkID: "task:legacy",
		State:  "open",
	}
	herdr := HerdrSnapshot{
		ObservedAt:  now,
		Connections: []HerdrConnection{},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow != nil {
		t.Fatalf("legacy work without agents must produce nil workflow, got %#v", workflow)
	}
}

func TestAwaitingPRWhenNoCandidateSHA(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		WorkID: "issue:10",
		GitHub: &GitHubWork{
			Kind: "issue",
			URL:  "https://github.com/acme/app/issues/10",
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/issues/10",
				Session:     "dev",
				AgentName:   "DevAgent",
				AgentStatus: "working",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow should not be nil")
	}
	if workflow.VerificationState.AnchorSHA != "" {
		t.Fatalf("anchorSha = %q, want empty", workflow.VerificationState.AnchorSHA)
	}
	foundAwaitingPR := false
	for _, code := range workflow.VerificationState.Codes {
		if code == "awaiting_pr" {
			foundAwaitingPR = true
			break
		}
	}
	if !foundAwaitingPR {
		t.Fatalf("codes = %v, want awaiting_pr", workflow.VerificationState.Codes)
	}
	if !strings.Contains(workflow.VerificationState.Summary, "PR 생성 대기") {
		t.Fatalf("summary = %q, want PR awaiting notice", workflow.VerificationState.Summary)
	}
}

func TestCandidateSHACIPendingAndFailed(t *testing.T) {
	now := time.Now()
	workFailed := WorkItem{
		GitHub: &GitHubWork{
			Kind: "pull_request",
			URL:  "https://github.com/acme/app/pull/1",
			Fields: map[string]string{
				"headSha": "abcdef1234567890abcdef1234567890abcdef12",
			},
			Checks: []GitHubCheck{
				{Name: "test", Conclusion: "failure"},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/1",
				Session:     "pr-session",
				AgentName:   "Builder",
				AgentStatus: "working",
			},
		},
	}

	wfFailed := CorrelateAgentWorkflow(nil, &workFailed, &herdr)
	if wfFailed == nil {
		t.Fatal("workflow should not be nil")
	}
	if wfFailed.VerificationState.AnchorSHA != "abcdef1234567890abcdef1234567890abcdef12" {
		t.Fatalf("anchorSha = %q", wfFailed.VerificationState.AnchorSHA)
	}
	hasCIFailed := false
	for _, code := range wfFailed.VerificationState.Codes {
		if code == "ci_failed" {
			hasCIFailed = true
		}
	}
	if !hasCIFailed {
		t.Fatalf("codes = %v, want ci_failed", wfFailed.VerificationState.Codes)
	}

	// CI Pending case
	workPending := WorkItem{
		GitHub: &GitHubWork{
			Kind: "pull_request",
			URL:  "https://github.com/acme/app/pull/2",
			Fields: map[string]string{
				"headSha": "1234567890abcdef1234567890abcdef12345678",
			},
			Checks: []GitHubCheck{
				{Name: "test", Status: "in_progress"},
			},
		},
	}
	herdrPending := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/2",
				Session:     "pr-session-2",
				AgentName:   "Builder2",
				AgentStatus: "working",
			},
		},
	}

	wfPending := CorrelateAgentWorkflow(nil, &workPending, &herdrPending)
	if wfPending == nil {
		t.Fatal("workflow should not be nil")
	}
	hasCIPending := false
	for _, code := range wfPending.VerificationState.Codes {
		if code == "ci_pending" {
			hasCIPending = true
		}
	}
	if !hasCIPending {
		t.Fatalf("codes = %v, want ci_pending", wfPending.VerificationState.Codes)
	}
}
