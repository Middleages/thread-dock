package main

import "testing"

func TestNormalizeExecutionStateDefaultsUnknown(t *testing.T) {
	if got := normalizeExecutionState(""); got != ExecutionUnknown {
		t.Fatalf("normalizeExecutionState(\"\") = %q, want %q", got, ExecutionUnknown)
	}
	if got := normalizeExecutionState("done"); got != ExecutionCompleted {
		t.Fatalf("normalizeExecutionState(\"done\") = %q, want %q", got, ExecutionCompleted)
	}
}

func TestNormalizeVerificationStateDoesNotInferSuccess(t *testing.T) {
	state := NormalizeVerificationState(VerificationState{
		Status:    "",
		Codes:     nil,
		AnchorSHA: "abc123",
	})
	if state.Status != VerificationUnverified {
		t.Fatalf("status = %q, want %q", state.Status, VerificationUnverified)
	}
	if state.AnchorSHA != "abc123" {
		t.Fatalf("anchor sha = %q, want abc123", state.AnchorSHA)
	}
}

func TestVerificationEvidenceMatchesCandidateSHA(t *testing.T) {
	state := VerificationState{
		Status:    VerificationUnverified,
		AnchorSHA: "abc123",
	}
	evidence := EvidenceItem{Type: "review", Reference: "comment:1", Revision: "abc123", Claim: "ACCEPT"}
	if !evidenceMatchesCandidate(state, evidence) {
		t.Fatal("matching candidate SHA should match evidence")
	}
	stale := evidence
	stale.Revision = "old456"
	if evidenceMatchesCandidate(state, stale) {
		t.Fatal("stale evidence must not match candidate SHA")
	}
}

func TestNormalizeVerificationStateRemovesSuccessCode(t *testing.T) {
	state := NormalizeVerificationState(VerificationState{
		Status: VerificationVerified,
		Codes:  []string{"fully_verified", "ci_passed"},
	})
	for _, code := range state.Codes {
		if code == "fully_verified" {
			t.Fatal("fully_verified must not be a verification reason code")
		}
	}
	if state.Status != VerificationVerified {
		t.Fatalf("status = %q, want verified", state.Status)
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
