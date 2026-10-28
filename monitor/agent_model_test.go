package main

import "testing"

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
