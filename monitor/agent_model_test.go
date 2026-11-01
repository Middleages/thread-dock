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
	if workflow.Results[0].Verification != VerificationUnverified {
		t.Fatalf("result verification = %q, want unverified", workflow.Results[0].Verification)
	}
}

func TestMatchingReviewEvidenceVerifies(t *testing.T) {
	now := time.Now()
	headSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
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
				{Name: "ci/build", Conclusion: "success"},
				{Name: "ci/test", Conclusion: "success"},
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

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
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
	if len(workflow.Results) != 1 || workflow.Results[0].Verification != VerificationVerified {
		t.Fatalf("results = %#v, want verified result", workflow.Results)
	}
}

func TestStaleReviewEvidenceProducesStaleSHA(t *testing.T) {
	now := time.Now()
	latestHeadSHA := "8e54c92a35dd7e2bcce9b6ae581ea9954c1fd66d"
	olderSHA := "a9b64e956dd289df17d2a00cb4f86895ae178167"
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
				{Name: "ci/test", Conclusion: "success"},
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

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
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

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
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

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
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
				{Name: "ci/test", Conclusion: "success"},
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
				Session:     "sess-parent",
				AgentName:   "ParentLead",
				AgentStatus: "done",
			},
			{
				IssueURL:    "https://github.com/acme/app/pull/106",
				Session:     "sess-child",
				AgentName:   "ChildTester",
				Role:        "reviewer",
				AgentStatus: "failed", // child failure!
			},
		},
	}

	// Connect parent-child
	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("workflow must not be nil")
	}
	// Manually simulate child parentage check in unit test
	workflow.Agents[1].ParentAgentID = workflow.Agents[0].AgentID
	// re-derive with hasChildIncomplete
	vState := DeriveVerificationState(headSHA, ExtractEvidenceItems(&work, headSHA, nil), work.GitHub.Checks, false, false, true)
	if vState.Status != VerificationPartial {
		t.Fatalf("verification status = %q, want partial when child result is incomplete", vState.Status)
	}
	if !strings.Contains(vState.Summary, "부분 확인") {
		t.Fatalf("summary = %q, want contains 부분 확인", vState.Summary)
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
