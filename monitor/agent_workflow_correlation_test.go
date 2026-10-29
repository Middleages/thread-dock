package main

import (
	"context"
	"testing"
	"time"
)

type agentWorkflowSnapshotSource struct {
	snapshot Snapshot
}

func (s agentWorkflowSnapshotSource) FetchAll(context.Context) (Snapshot, error) {
	return s.snapshot, nil
}

func TestPRCorrelatesThroughClosingIssue(t *testing.T) {
	now := time.Now()
	work := WorkItem{
		WorkID: "pr:7",
		GitHub: &GitHubWork{
			Kind:             "pull_request",
			URL:              "https://github.com/acme/app/pull/7",
			RelatedIssueURLs: []string{"https://github.com/acme/app/issues/42"},
		},
	}
	herdr := HerdrSnapshot{
		ObservedAt: now,
		Connections: []HerdrConnection{{
			IssueURL:    "https://github.com/acme/app/issues/42",
			Session:     "pr-7",
			AgentName:   "Builder",
			AgentStatus: "working",
		}},
	}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil {
		t.Fatal("PR should correlate through its closing issue")
	}
	if workflow.Provenance.Scope != "related_issue" {
		t.Fatalf("scope = %q, want related_issue", workflow.Provenance.Scope)
	}
}

func TestCoordinatorFallbackIsPartialAndFallback(t *testing.T) {
	project := Project{Links: []Link{{Kind: "github", URL: "https://github.com/acme/app/issues"}}}
	work := WorkItem{WorkID: "issue:1", State: "open"}
	herdr := HerdrSnapshot{
		ObservedAt: time.Now(),
		Connections: []HerdrConnection{{
			Repository:  "acme/app",
			Role:        "coordinator",
			Session:     "main",
			AgentName:   "Coordinator",
			AgentStatus: "working",
		}},
	}

	workflow := CorrelateAgentWorkflow(&project, &work, &herdr)
	if workflow == nil {
		t.Fatal("expected coordinator fallback")
	}
	if !workflow.Provenance.Fallback || !workflow.Provenance.Partial {
		t.Fatalf("provenance = %#v, want fallback+partial", workflow.Provenance)
	}
	if workflow.VerificationState.Status != VerificationPartial {
		t.Fatalf("verification = %q, want partial", workflow.VerificationState.Status)
	}
}

func TestDuplicateAgentNamesUseDistinctLogicalIDs(t *testing.T) {
	work := WorkItem{GitHub: &GitHubWork{URL: "https://github.com/acme/app/issues/5"}}
	herdr := HerdrSnapshot{Connections: []HerdrConnection{
		{IssueURL: "https://github.com/acme/app/issues/5", Session: "a", PaneID: "1", AgentName: "Codex", AgentStatus: "working"},
		{IssueURL: "https://github.com/acme/app/issues/5", Session: "b", PaneID: "2", AgentName: "Codex", AgentStatus: "working"},
	}}

	workflow := CorrelateAgentWorkflow(nil, &work, &herdr)
	if workflow == nil || len(workflow.Agents) != 2 {
		t.Fatalf("workflow agents = %#v", workflow)
	}
	if workflow.Agents[0].AgentID == workflow.Agents[1].AgentID {
		t.Fatalf("duplicate agent names must not collapse identities: %#v", workflow.Agents)
	}
}

func TestAppEnrichesReturnedSnapshot(t *testing.T) {
	now := time.Now()
	snapshot := Snapshot{
		Projects: []Project{{
			Links: []Link{{Kind: "github", URL: "https://github.com/acme/app/issues"}},
			WorkItems: []WorkItem{{
				WorkID: "issue:1",
				GitHub:  &GitHubWork{URL: "https://github.com/acme/app/issues/1"},
			}},
		}},
		Herdr: &HerdrSnapshot{
			ObservedAt: now,
			Connections: []HerdrConnection{{
				IssueURL:    "https://github.com/acme/app/issues/1",
				Session:     "agent-1",
				AgentName:   "Codex",
				AgentStatus: "working",
			}},
		},
	}

	app := NewApp(agentWorkflowSnapshotSource{snapshot: snapshot})
	got, err := app.GetMonitorSnapshot()
	if err != nil {
		t.Fatalf("GetMonitorSnapshot() error = %v", err)
	}
	if got.Projects[0].WorkItems[0].AgentWorkflow == nil {
		t.Fatal("returned snapshot should expose AgentWorkflow")
	}
}
