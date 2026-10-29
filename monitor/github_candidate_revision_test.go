package main

import (
	"context"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

func TestFetchPullRequestHeadSHA(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{Stdout: "abc123456789\n"}}
	runner := newWSLCommandRunner(fake, "Ubuntu", time.Second)

	sha, err := fetchPullRequestHeadSHA(runner, 42, "https://github.com/acme/app/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	if sha != "abc123456789" {
		t.Fatalf("sha=%q", sha)
	}
	want := []string{"", "wsl.exe", "--distribution", "Ubuntu", "--exec", "gh", "pr", "view", "https://github.com/acme/app/pull/42", "--json", "headRefOid", "--jq", ".headRefOid"}
	if got := fake.args; len(got) != len(want) {
		t.Fatalf("args=%q want=%q", got, want)
	}
	for i := range want {
		if fake.args[i] != want[i] {
			t.Fatalf("args=%q want=%q", fake.args, want)
		}
	}
}

func TestEnrichAgentCandidateRevisionsUpdatesVerificationAnchor(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{Stdout: "deadbeef1234567\n"}}
	number := 42
	snapshot := Snapshot{Projects: []Project{{WorkItems: []WorkItem{{
		GitHub: &GitHubWork{
			Kind: "pull_request",
			Number: &number,
			URL: "https://github.com/acme/app/pull/42",
			Fields: map[string]string{},
		},
		AgentWorkflow: &AgentWorkflow{VerificationState: VerificationState{Status: VerificationUnverified}},
	}}}}}

	notices := enrichAgentCandidateRevisions(&snapshot, fake, "Ubuntu", time.Second)
	if len(notices) != 0 {
		t.Fatalf("notices=%v", notices)
	}
	if got := snapshot.Projects[0].WorkItems[0].GitHub.Fields["headSha"]; got != "deadbeef1234567" {
		t.Fatalf("headSha=%q", got)
	}
}

func TestEnrichAgentCandidateRevisionsLeavesUnresolvedRevisionUnverified(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{ExitCode: 1}, err: context.Canceled}
	number := 42
	snapshot := Snapshot{Projects: []Project{{WorkItems: []WorkItem{{
		GitHub: &GitHubWork{Kind: "pull_request", Number: &number, URL: "https://github.com/acme/app/pull/42", Fields: map[string]string{}},
		AgentWorkflow: &AgentWorkflow{VerificationState: VerificationState{Status: VerificationUnverified, Codes: []string{"reviewer_missing"}}},
	}}}}}

	notices := enrichAgentCandidateRevisions(&snapshot, fake, "Ubuntu", time.Second)
	if len(notices) != 1 {
		t.Fatalf("notices=%v", notices)
	}
	if got := snapshot.Projects[0].WorkItems[0].GitHub.Fields["headSha"]; got != "" {
		t.Fatalf("headSha=%q", got)
	}

	normalizeMissingCandidateRevision(&snapshot)
	state := snapshot.Projects[0].WorkItems[0].AgentWorkflow.VerificationState
	if state.Status != VerificationUnverified {
		t.Fatalf("status=%q", state.Status)
	}
	if len(state.Codes) != 1 || state.Codes[0] != "candidate_revision_missing" {
		t.Fatalf("codes=%v", state.Codes)
	}
}
