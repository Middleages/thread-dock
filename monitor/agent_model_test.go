package main

import (
	"fmt"
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
				{Name: "test", HeadSHA: "abcdef1234567890abcdef1234567890abcdef12", Conclusion: "failure"},
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
				{Name: "test", HeadSHA: "1234567890abcdef1234567890abcdef12345678", Status: "in_progress"},
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

func TestReceiptAsExecutionEvidenceDoesNotVerify(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		WorkID: "pr:101",
		Title:  "Refactor agent evidence",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(101),
			URL:    "https://github.com/acme/app/pull/101",
			Fields: map[string]string{
				"headSha": "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d",
			},
			Checks: []GitHubCheck{
				{Name: "test", Conclusion: "success"},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/101",
				Session:     "impl-session",
				AgentName:   "Implementer",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.ExecutionState != ExecutionCompleted {
		t.Fatalf("executionState = %q, want completed", workflow.ExecutionState)
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("verification status = %q, want unverified (receipt alone must not verify)", workflow.VerificationState.Status)
	}
	hasReviewerMissing := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasReviewerMissing = true
			break
		}
	}
	if !hasReviewerMissing {
		t.Fatalf("codes = %v, want reviewer_missing", workflow.VerificationState.Codes)
	}
	if len(workflow.Results) != 1 {
		t.Fatalf("results count = %d, want 1", len(workflow.Results))
	}
	if workflow.Results[0].Status != ExecutionCompleted {
		t.Fatalf("result status = %q, want completed", workflow.Results[0].Status)
	}
	if workflow.Results[0].Verification != "" {
		t.Fatalf("result verification = %q, want empty (no per-result evidence)", workflow.Results[0].Verification)
	}
}

func TestMatchingReviewEvidenceVerifies(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"security": {"security-reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:102",
		Title:  "Implement Task 3",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(102),
			URL:    "https://github.com/acme/app/pull/102",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Checks: []GitHubCheck{
				{Name: "ci/build", HeadSHA: headSHA, Conclusion: "success"},
				{Name: "ci/test", HeadSHA: headSHA, Conclusion: "success"},
			},
			Comments: []GitHubComment{
				{
					Author:     "security-reviewer",
					Body:       "LGTM!\n<!-- evidence: reviewer=security sha=8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d verdict=ACCEPT -->",
					URL:        "https://github.com/acme/app/pull/102#issuecomment-1",
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/102",
				Session:     "task3-session",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationVerified {
		t.Fatalf("verification status = %q, want verified", workflow.VerificationState.Status)
	}
	hasReviewerAccepted, hasCIPassed := false, false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_accepted" {
			hasReviewerAccepted = true
		}
		if c == "ci_passed" {
			hasCIPassed = true
		}
	}
	if !hasReviewerAccepted || !hasCIPassed {
		t.Fatalf("codes = %v, want reviewer_accepted and ci_passed", workflow.VerificationState.Codes)
	}
	if !strings.Contains(workflow.VerificationState.Summary, "검증 완료") {
		t.Fatalf("summary = %q, want contains 검증 완료", workflow.VerificationState.Summary)
	}
	if len(workflow.Results) != 1 || workflow.Results[0].Verification != "" {
		t.Fatalf("results = %#v, want one result without copied verification", workflow.Results)
	}
}

func TestStaleReviewEvidenceProducesStaleSHA(t *testing.T) {
	now := time.Now()
	latestHeadSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	olderSHA := "a9b64e956dd289df17d2a00cb4f86895ae178167"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"code-reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:103",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(103),
			URL:    "https://github.com/acme/app/pull/103",
			Fields: map[string]string{
				"headSha": latestHeadSHA,
			},
			Checks: []GitHubCheck{
				{Name: "ci/test", HeadSHA: latestHeadSHA, Conclusion: "success"},
			},
			Comments: []GitHubComment{
				{
					Author:     "code-reviewer",
					Body:       fmt.Sprintf("Reviewed older commit:\n<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", olderSHA),
					URL:        "https://github.com/acme/app/pull/103#issuecomment-2",
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/103",
				Session:     "sess-103",
				AgentName:   "Developer",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("verification status = %q, want unverified on stale review", workflow.VerificationState.Status)
	}
	hasStaleSHA := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "stale_sha" {
			hasStaleSHA = true
		}
	}
	if !hasStaleSHA {
		t.Fatalf("codes = %v, want stale_sha", workflow.VerificationState.Codes)
	}
	if !strings.Contains(workflow.VerificationState.Summary, "이전 커밋") {
		t.Fatalf("summary = %q, want notice of older commit review", workflow.VerificationState.Summary)
	}
}

func TestReviewerVerdictRejectProducesReviewerNotAccepted(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:104",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(104),
			URL:    "https://github.com/acme/app/pull/104",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "reviewer",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=REJECT -->", headSHA),
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/104",
				Session:     "sess-104",
				AgentName:   "Coder",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("verification status = %q, want unverified on reject", workflow.VerificationState.Status)
	}
	hasNotAccepted := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_not_accepted" {
			hasNotAccepted = true
		}
	}
	if !hasNotAccepted {
		t.Fatalf("codes = %v, want reviewer_not_accepted", workflow.VerificationState.Codes)
	}
}

func TestReviewerMarkerAuthorMismatchRejected(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"security": {"security-reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:105",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(105),
			URL:    "https://github.com/acme/app/pull/105",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "unauthorized-spammer",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=security sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/105",
				Session:     "sess-105",
				AgentName:   "Coder",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when marker author mismatches role", workflow.VerificationState.Status)
	}
	hasReviewerMissing := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasReviewerMissing = true
		}
	}
	if !hasReviewerMissing {
		t.Fatalf("codes = %v, want reviewer_missing", workflow.VerificationState.Codes)
	}
}

func TestIncompleteChildResultProducesPartialVerification(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:106",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(106),
			URL:    "https://github.com/acme/app/pull/106",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Checks: []GitHubCheck{
				{Name: "ci/test", HeadSHA: headSHA, Conclusion: "success"},
			},
			Comments: []GitHubComment{
				{
					Author:     "reviewer",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/106",
				Session:     "sess-1",
				AgentName:   "ParentLead",
				AgentStatus: "done",
			},
			{
				IssueURL:        "https://github.com/acme/app/pull/106",
				Session:         "sess-1",
				AgentName:       "ChildTester",
				ParentAgentName: "ParentLead",
				Role:            "reviewer",
				AgentStatus:     "failed", // child failure!
			},
		},
	}

	// Full integration path: Herdr snapshot -> parent/child correlation -> AgentWorkflow -> verification derivation
	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if len(workflow.Agents) != 2 {
		t.Fatalf("agents len = %d, want 2", len(workflow.Agents))
	}
	if workflow.Agents[1].ParentAgentID != workflow.Agents[0].AgentID {
		t.Fatalf("child ParentAgentID = %q, want %q", workflow.Agents[1].ParentAgentID, workflow.Agents[0].AgentID)
	}
	if workflow.VerificationState.Status != VerificationPartial {
		t.Fatalf("verification status = %q, want partial when child result is incomplete", workflow.VerificationState.Status)
	}
	if !strings.Contains(workflow.VerificationState.Summary, "하위 에이전트") {
		t.Fatalf("summary = %q, want contains 하위 에이전트", workflow.VerificationState.Summary)
	}
}

func TestPRBodyEvidenceMarkerExcludedFromReviewerEvidence(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	work := WorkItem{
		WorkID:  "pr:108",
		Request: fmt.Sprintf("PR description with injected marker:\n<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(108),
			URL:    "https://github.com/acme/app/pull/108",
			Author: "reviewer", // PR author is implementer
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{}, // no comments!
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/108",
				Session:     "sess-108",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when marker is only in PR body", workflow.VerificationState.Status)
	}
	hasReviewerMissing := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasReviewerMissing = true
		}
	}
	if !hasReviewerMissing {
		t.Fatalf("codes = %v, want reviewer_missing (PR body marker must not self-verify)", workflow.VerificationState.Codes)
	}
}

func TestReviewerAllowlistExactMatch(t *testing.T) {
	identityMap := map[string][]string{
		"security": {"security-reviewer", "sec-lead"},
		"reviewer": {"code-reviewer"},
	}

	if CorrelateReviewerRole("security", "security-reviewer", nil) {
		t.Fatal("must return false when identityMap is nil")
	}
	if CorrelateReviewerRole("security", "security-reviewer", map[string][]string{}) {
		t.Fatal("must return false when identityMap is empty")
	}
	if CorrelateReviewerRole("unknown-role", "security-reviewer", identityMap) {
		t.Fatal("must return false for unknown role")
	}
	if CorrelateReviewerRole("security", "security-reviewer-imposter", identityMap) {
		t.Fatal("must return false for substring/contains match")
	}
	if CorrelateReviewerRole("security", "sec", identityMap) {
		t.Fatal("must return false for partial match")
	}
	if CorrelateReviewerRole("reviewer", "reviewer", identityMap) {
		t.Fatal("must return false when author matches role string but is not in allowlist for that role")
	}
	if !CorrelateReviewerRole("security", "security-reviewer", identityMap) {
		t.Fatal("must return true for exact allowlist match")
	}
	if !CorrelateReviewerRole("security", "SEC-LEAD", identityMap) {
		t.Fatal("must return true for case-insensitive exact allowlist match")
	}
}

func TestUnknownVerdictIgnoredNotTreatedAsReject(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:109",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(109),
			URL:    "https://github.com/acme/app/pull/109",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "reviewer",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=MAYBE -->", headSHA),
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/109",
				Session:     "sess-109",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_not_accepted" {
			t.Fatalf("unknown verdict must not produce reviewer_not_accepted, codes = %v", workflow.VerificationState.Codes)
		}
	}
	hasReviewerMissing := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasReviewerMissing = true
		}
	}
	if !hasReviewerMissing {
		t.Fatalf("codes = %v, want reviewer_missing for unknown verdict", workflow.VerificationState.Codes)
	}
}

func TestCIEvidenceRevisionObservedNotFabricated(t *testing.T) {
	candidateSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	mismatchedSHA := "1111111a35dd7e2bcce9b6ae581ea9954c1fd111"
	work := WorkItem{
		WorkID: "pr:110",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(110),
			URL:    "https://github.com/acme/app/pull/110",
			Fields: map[string]string{
				"headSha": candidateSHA,
			},
			Checks: []GitHubCheck{
				{Name: "ci/unobserved", Conclusion: "success"},
				{Name: "ci/mismatched", HeadSHA: mismatchedSHA, Conclusion: "failure"},
			},
		},
	}

	evidences := ExtractEvidenceItems(&work, nil)
	if len(evidences) != 2 {
		t.Fatalf("evidences count = %d, want 2", len(evidences))
	}
	if evidences[0].Revision != "" {
		t.Fatalf("evidence[0].Revision = %q, want empty (must not copy candidateSHA)", evidences[0].Revision)
	}
	if evidences[1].Revision != mismatchedSHA {
		t.Fatalf("evidence[1].Revision = %q, want %q", evidences[1].Revision, mismatchedSHA)
	}

	vState := DeriveVerificationState(candidateSHA, evidences, work.GitHub.Checks, nil, false, false, false)
	for _, code := range vState.Codes {
		if code == "ci_failed" {
			t.Fatalf("mismatched SHA check failure must not fail candidate verification, got codes = %v", vState.Codes)
		}
	}
}

func TestMalformedEvidenceMarkerLeavesProjectObservable(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	work := WorkItem{
		WorkID: "pr:107",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(107),
			URL:    "https://github.com/acme/app/pull/107",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "reviewer",
					Body:       "<!-- evidence: broken missing all fields -->\nNormal comment text here",
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/107",
				Session:     "sess-107",
				AgentName:   "Dev",
				AgentStatus: "working",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("malformed marker must not crash correlation; workflow must be observable")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified", workflow.VerificationState.Status)
	}
	hasReviewerMissing := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasReviewerMissing = true
		}
	}
	if !hasReviewerMissing {
		t.Fatalf("codes = %v, want reviewer_missing", workflow.VerificationState.Codes)
	}
}

func TestUnconfiguredProjectRejectsReviewerEvidence(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	work := WorkItem{
		WorkID: "pr:111",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(111),
			URL:    "https://github.com/acme/app/pull/111",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "reviewer",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/111",
				Session:     "sess-111",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	wfNilProject := CorrelateAgentWorkflow(nil, &work, &herdr)
	if wfNilProject == nil {
		t.Fatal("workflow should not be nil")
	}
	if wfNilProject.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when project is nil", wfNilProject.VerificationState.Status)
	}
	hasReviewerMissing := false
	for _, c := range wfNilProject.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasReviewerMissing = true
		}
	}
	if !hasReviewerMissing {
		t.Fatalf("codes = %v, want reviewer_missing without configured identities", wfNilProject.VerificationState.Codes)
	}

	emptyProject := &Project{}
	wfEmptyProject := CorrelateAgentWorkflow(emptyProject, &work, &herdr)
	if wfEmptyProject == nil {
		t.Fatal("workflow should not be nil")
	}
	if wfEmptyProject.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when project has empty identities", wfEmptyProject.VerificationState.Status)
	}
}

func TestUnboundChecksDoNotAddCIPassedPresentation(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"reviewer"},
		},
	}
	work := WorkItem{
		WorkID: "pr:112",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(112),
			URL:    "https://github.com/acme/app/pull/112",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Checks: []GitHubCheck{
				// Checks are present and successful, but have no observed HeadSHA (empty)
				{Name: "ci/unbound-check", Conclusion: "success"},
			},
			Comments: []GitHubComment{
				{
					Author:     "reviewer",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &now,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/112",
				Session:     "sess-112",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	// Reviewer approved, so status is verified
	if workflow.VerificationState.Status != VerificationVerified {
		t.Fatalf("status = %q, want verified", workflow.VerificationState.Status)
	}
	// But unbound checks must NOT produce ci_passed code
	for _, c := range workflow.VerificationState.Codes {
		if c == "ci_passed" {
			t.Fatalf("unbound checks (HeadSHA == '') must not produce ci_passed code, codes = %v", workflow.VerificationState.Codes)
		}
	}
	// And summary must NOT claim "CI 통과"
	if strings.Contains(workflow.VerificationState.Summary, "CI 통과") {
		t.Fatalf("summary = %q, must not contain CI 통과 when checks are unbound", workflow.VerificationState.Summary)
	}
	if workflow.VerificationState.Summary != "검증 완료 (리뷰어 승인)" {
		t.Fatalf("summary = %q, want '검증 완료 (리뷰어 승인)'", workflow.VerificationState.Summary)
	}
}

func TestReviewerChronologyLatestAcceptSupersedesReject(t *testing.T) {
	t1 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 2, 10, 30, 0, 0, time.UTC)
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"alice"},
		},
	}
	work := WorkItem{
		WorkID: "pr:113",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(113),
			URL:    "https://github.com/acme/app/pull/113",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "alice",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=REJECT -->", headSHA),
					ObservedAt: &t1,
				},
				{
					Author:     "alice",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &t2,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: t2,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/113",
				Session:     "sess-113",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationVerified {
		t.Fatalf("status = %q, want verified when latest verdict is ACCEPT", workflow.VerificationState.Status)
	}
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_not_accepted" {
			t.Fatalf("earlier REJECT must not persist when latest is ACCEPT, codes = %v", workflow.VerificationState.Codes)
		}
	}
}

func TestReviewerChronologyLatestRejectSupersedesAccept(t *testing.T) {
	t1 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 2, 10, 30, 0, 0, time.UTC)
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"reviewer": {"alice"},
		},
	}
	work := WorkItem{
		WorkID: "pr:114",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(114),
			URL:    "https://github.com/acme/app/pull/114",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "alice",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &t1,
				},
				{
					Author:     "alice",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=REJECT -->", headSHA),
					ObservedAt: &t2,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: t2,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/114",
				Session:     "sess-114",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when latest verdict is REJECT", workflow.VerificationState.Status)
	}
	hasNotAccepted := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_not_accepted" {
			hasNotAccepted = true
		}
		if c == "reviewer_accepted" {
			t.Fatalf("earlier ACCEPT must not persist when latest is REJECT, codes = %v", workflow.VerificationState.Codes)
		}
	}
	if !hasNotAccepted {
		t.Fatalf("expected reviewer_not_accepted in codes, got %v", workflow.VerificationState.Codes)
	}
}

func TestMultipleReviewerRolesRequireAllAccepted(t *testing.T) {
	t1 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 2, 10, 15, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 2, 10, 45, 0, 0, time.UTC)
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"security": {"sec-alice"},
			"qa":       {"qa-bob"},
		},
	}
	work := WorkItem{
		WorkID: "pr:115",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(115),
			URL:    "https://github.com/acme/app/pull/115",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "sec-alice",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=security sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &t1,
				},
				{
					Author:     "qa-bob",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=qa sha=%s verdict=REJECT -->", headSHA),
					ObservedAt: &t2,
				},
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: t2,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/115",
				Session:     "sess-115",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when any role rejects", workflow.VerificationState.Status)
	}

	work.GitHub.Comments = append(work.GitHub.Comments, GitHubComment{
		Author:     "qa-bob",
		Body:       fmt.Sprintf("<!-- evidence: reviewer=qa sha=%s verdict=ACCEPT -->", headSHA),
		ObservedAt: &t3,
	})

	workflow2 := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow2 == nil {
		t.Fatal("workflow2 must not be nil")
	}
	if workflow2.VerificationState.Status != VerificationVerified {
		t.Fatalf("status = %q, want verified when all roles have latest ACCEPT", workflow2.VerificationState.Status)
	}
}

func TestMultipleReviewerRolesMissingOneRoleLeavesUnverified(t *testing.T) {
	t1 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{
		ReviewerIdentities: map[string][]string{
			"security": {"sec-alice"},
			"qa":       {"qa-bob"},
		},
	}
	work := WorkItem{
		WorkID: "pr:116",
		GitHub: &GitHubWork{
			Kind:   "pull_request",
			Number: intPtr(116),
			URL:    "https://github.com/acme/app/pull/116",
			Fields: map[string]string{
				"headSha": headSHA,
			},
			Comments: []GitHubComment{
				{
					Author:     "sec-alice",
					Body:       fmt.Sprintf("<!-- evidence: reviewer=security sha=%s verdict=ACCEPT -->", headSHA),
					ObservedAt: &t1,
				},
				// qa-bob has NOT reviewed (no comment / no evidence)
			},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: t1,
		Connections: []HerdrConnection{
			{
				IssueURL:    "https://github.com/acme/app/pull/116",
				Session:     "sess-116",
				AgentName:   "Worker",
				AgentStatus: "done",
			},
		},
	}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	// qa role hasn't reviewed yet, so status must remain unverified
	if workflow.VerificationState.Status != VerificationUnverified {
		t.Fatalf("status = %q, want unverified when a configured role has no review evidence", workflow.VerificationState.Status)
	}
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_accepted" {
			t.Fatalf("reviewer_accepted must not be present when qa role is missing, codes = %v", workflow.VerificationState.Codes)
		}
	}
	hasMissing := false
	for _, c := range workflow.VerificationState.Codes {
		if c == "reviewer_missing" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Fatalf("reviewer_missing code expected, got %v", workflow.VerificationState.Codes)
	}
	if !strings.Contains(workflow.VerificationState.Summary, "승인 대기 중") {
		t.Fatalf("summary = %q, want summary indicating waiting for approval", workflow.VerificationState.Summary)
	}
}

func TestIdleChildDoesNotBlockVerification(t *testing.T) {
	if got := NormalizeExecutionState("idle"); got != ExecutionUnknown {
		t.Fatalf("idle = %q, want unknown", got)
	}
	if got := NormalizeExecutionState("blocked"); got != ExecutionWaiting {
		t.Fatalf("blocked = %q, want waiting", got)
	}
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{ReviewerIdentities: map[string][]string{"reviewer": {"reviewer"}}}
	work := WorkItem{
		WorkID: "pr:120",
		GitHub: &GitHubWork{
			Kind: "pull_request", URL: "https://github.com/acme/app/pull/120",
			Fields: map[string]string{"headSha": headSHA},
			Comments: []GitHubComment{{
				Author: "reviewer", ObservedAt: &now,
				Body: fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
			}},
		},
	}
	herdr := HerdrSnapshot{ObservedAt: now, Connections: []HerdrConnection{
		{IssueURL: work.GitHub.URL, Session: "s", AgentName: "Lead", AgentStatus: "done"},
		{IssueURL: work.GitHub.URL, Session: "s", AgentName: "Child", ParentAgentName: "Lead", AgentStatus: "idle"},
	}}
	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	if workflow == nil || workflow.VerificationState.Status != VerificationVerified {
		t.Fatalf("workflow = %#v, want verified with idle child", workflow)
	}
}

func TestProvenanceCaveatsOnlyDowngradeVerification(t *testing.T) {
	sha := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	accept := []EvidenceItem{{Type: "review", Role: "reviewer", Revision: sha, Claim: "ACCEPT"}}
	roles := []string{"reviewer"}

	// No review: caveats must not turn unverified into partial.
	got := DeriveVerificationState(sha, nil, nil, roles, true, true, true)
	if got.Status != VerificationUnverified {
		t.Fatalf("no review: status = %q, want unverified", got.Status)
	}
	// Failed CI stays unverified and keeps the CI summary.
	failed := []GitHubCheck{{Name: "ci", HeadSHA: sha, Conclusion: "failure"}}
	got = DeriveVerificationState(sha, accept, failed, roles, false, false, true)
	if got.Status != VerificationUnverified || got.Summary != "CI 검증 실패" {
		t.Fatalf("ci failed: %#v", got)
	}
	// Cancelled counts as failed, waiting as pending.
	got = DeriveVerificationState(sha, accept, []GitHubCheck{{Name: "ci", HeadSHA: sha, Conclusion: "cancelled"}}, roles, false, false, false)
	if got.Status != VerificationUnverified || got.Codes[0] != "ci_failed" {
		t.Fatalf("cancelled: %#v", got)
	}
	got = DeriveVerificationState(sha, accept, []GitHubCheck{{Name: "ci", HeadSHA: sha, Status: "waiting"}}, roles, false, false, false)
	if got.Status != VerificationUnverified || got.Codes[0] != "ci_pending" {
		t.Fatalf("waiting: %#v", got)
	}
	// Fully verified evidence is downgraded by each caveat.
	for _, c := range [][3]bool{{true, false, false}, {false, true, false}, {false, false, true}} {
		got = DeriveVerificationState(sha, accept, nil, roles, c[0], c[1], c[2])
		if got.Status != VerificationPartial || got.Summary == "" {
			t.Fatalf("caveat %v: %#v", c, got)
		}
	}
	got = DeriveVerificationState(sha, accept, nil, roles, false, false, false)
	if got.Status != VerificationVerified {
		t.Fatalf("clean: %#v", got)
	}
}

func TestMultipleAgentsGetUniqueTaskIDs(t *testing.T) {
	work := WorkItem{WorkID: "issue:7", GitHub: &GitHubWork{URL: "https://github.com/acme/app/issues/7"}}
	herdr := HerdrSnapshot{Connections: []HerdrConnection{
		{IssueURL: work.GitHub.URL, Session: "s", AgentName: "A", AgentStatus: "done"},
		{IssueURL: work.GitHub.URL, Session: "s", AgentName: "B", AgentStatus: "done"},
	}}
	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil || len(workflow.Tasks) != 2 || workflow.Tasks[0].TaskID == workflow.Tasks[1].TaskID {
		t.Fatalf("tasks = %#v, want unique ids", workflow)
	}
	for _, r := range workflow.Results {
		if len(r.Findings) != 0 {
			t.Fatalf("findings = %v, want none", r.Findings)
		}
	}
}

func TestEnrichSnapshotMarksPRWithoutHeadSHAAsRevisionMissing(t *testing.T) {
	number := 42
	url := "https://github.com/acme/app/pull/42"
	snapshot := Snapshot{Projects: []Project{{WorkItems: []WorkItem{{
		WorkID: "pr:42",
		GitHub: &GitHubWork{Kind: "pull_request", Number: &number, URL: url, Fields: map[string]string{}},
	}}}}}
	herdr := HerdrSnapshot{ObservedAt: time.Now(), Connections: []HerdrConnection{{IssueURL: url, Session: "s", AgentName: "A", AgentStatus: "working"}}}

	enrichSnapshotWithAgentWorkflow(&snapshot, &herdr)

	state := snapshot.Projects[0].WorkItems[0].AgentWorkflow.VerificationState
	if state.Status != VerificationUnverified || len(state.Codes) != 1 || state.Codes[0] != "candidate_revision_missing" {
		t.Fatalf("state = %#v, want candidate_revision_missing only", state)
	}
}

func TestTruncatedCommentsDowngradeVerifiedToPartial(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{ReviewerIdentities: map[string][]string{"reviewer": {"reviewer"}}}
	comments := []GitHubComment{{
		Author: "reviewer", ObservedAt: &now,
		Body: fmt.Sprintf("<!-- evidence: reviewer=reviewer sha=%s verdict=ACCEPT -->", headSHA),
	}}
	for len(comments) < githubCommentCap {
		comments = append(comments, GitHubComment{Author: "someone", Body: "chat"})
	}
	work := WorkItem{WorkID: "pr:130", GitHub: &GitHubWork{
		Kind: "pull_request", URL: "https://github.com/acme/app/pull/130",
		Fields: map[string]string{"headSha": headSHA}, Comments: comments, CommentsTruncated: true,
	}}
	herdr := HerdrSnapshot{ObservedAt: now, Connections: []HerdrConnection{{IssueURL: work.GitHub.URL, Session: "s", AgentName: "A", AgentStatus: "done"}}}

	workflow := CorrelateAgentWorkflow(project, &work, &herdr)
	state := workflow.VerificationState
	if state.Status != VerificationPartial || !strings.Contains(state.Summary, "일부만 조회") || len(workflow.Notices) == 0 {
		t.Fatalf("workflow = %#v, want partial with truncation notice", workflow)
	}
	found := false
	for _, c := range state.Codes {
		found = found || c == "comments_truncated"
	}
	if !found {
		t.Fatalf("codes = %v", state.Codes)
	}
	work.GitHub.CommentsTruncated = false
	if got := CorrelateAgentWorkflow(project, &work, &herdr).VerificationState.Status; got != VerificationVerified {
		t.Fatalf("untruncated = %q, want verified", got)
	}
}

func TestSameAccountReviewerIsFlaggedButStaysVerified(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	project := &Project{ReviewerIdentities: map[string][]string{"td_reviewer": {"alice", "bob"}}}
	build := func(prAuthor, commenter string) *AgentWorkflow {
		work := WorkItem{WorkID: "pr:140", GitHub: &GitHubWork{
			Kind: "pull_request", URL: "https://github.com/acme/app/pull/140", Author: prAuthor,
			Fields: map[string]string{"headSha": headSHA},
			Comments: []GitHubComment{{
				Author: commenter, ObservedAt: &now,
				Body: fmt.Sprintf("<!-- evidence: reviewer=td_reviewer sha=%s verdict=ACCEPT -->", headSHA),
			}},
		}}
		herdr := HerdrSnapshot{ObservedAt: now, Connections: []HerdrConnection{{IssueURL: work.GitHub.URL, Session: "s", AgentName: "A", AgentStatus: "done"}}}
		return CorrelateAgentWorkflow(project, &work, &herdr)
	}
	has := func(w *AgentWorkflow) bool {
		for _, c := range w.VerificationState.Codes {
			if c == "reviewer_self_declared" {
				return true
			}
		}
		return false
	}

	same := build("Alice", "alice")
	if same.VerificationState.Status != VerificationVerified || !has(same) || !strings.Contains(same.VerificationState.Summary, "같은 계정 선언") || len(same.Notices) == 0 {
		t.Fatalf("same author: %#v", same)
	}
	if other := build("alice", "bob"); other.VerificationState.Status != VerificationVerified || has(other) || len(other.Notices) != 0 {
		t.Fatalf("different author: %#v", other)
	}
	if unknown := build("", "alice"); unknown.VerificationState.Status != VerificationVerified || has(unknown) {
		t.Fatalf("unknown author: %#v", unknown)
	}
}
