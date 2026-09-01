package pilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeRoleAgentRunbookDocumentsRoutingAuthorityAndSnapshotSafety(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "docs", "operator", "opencode-role-agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(contents)
	required := []string{
		"# OpenCode Role Agent Routing",
		"Planner",
		"selected manually",
		"explore",
		"Builder",
		"Reviewer",
		"OpenCode is the model authority",
		"ThreadDock is the name router",
		"opencode agent list",
		"openCodeAgents",
		"builder",
		"reviewer",
		"optional",
		"existing RUNs retain snapshot routing",
		"Go Orchestrator has no model",
	}
	for _, phrase := range required {
		if !strings.Contains(doc, phrase) {
			t.Errorf("OpenCode role-agent runbook missing %q", phrase)
		}
	}
}

func TestReadmeLinksOpenCodeRoleAgentRunbook(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "docs/operator/opencode-role-agents.md") {
		t.Fatal("README must link the OpenCode role-agent runbook")
	}
}
