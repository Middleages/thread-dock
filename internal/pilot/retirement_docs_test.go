package pilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionRetirementRunbookDocumentsCommandsAndGuards(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "docs", "operator", "session-retirement.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(contents)
	required := []string{
		"# Session Retirement Runbook",
		"agentctl status RUN",
		"agentctl status RUN --json",
		"agentctl retire RUN",
		"agentctl retire RUN --blocked",
		"agentctl resume RUN",
		"agentctl cleanup RUN",
		"autoRetireCompletedSessions",
		"true",
		"false",
		"PhaseRetiring",
		"retiring",
		"retired",
		"blocked",
		"needs_operator",
		"blocked retirement requires --blocked",
		"response loss",
		"Workspace absence",
		"Worktree",
		"run.json",
		"events",
		"seven days",
		"secret scan",
		"--force",
		"Herdr 0.8.2",
		"Workspace ID",
		"worktree remove",
		"evidence awaiting acceptance",
		"run-before-cleanup.json",
		"original pilot config",
	}
	for _, phrase := range required {
		if !strings.Contains(doc, phrase) {
			t.Errorf("session retirement runbook missing %q", phrase)
		}
	}
	if strings.Contains(doc, "config-cleanup.json") || strings.Contains(doc, "CLEANUP_STATE") {
		t.Fatal("session retirement runbook must not rebind managed Worktree trust by changing stateDir")
	}
}

func TestParallelPilotRunbookProtectsUnacceptedEvidenceFromRetirement(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "docs", "operator", "parallel-pilot.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(contents)
	required := []string{
		"retirement",
		"accepted",
		"diagnostic",
		"agentctl retire RUN",
		"agentctl cleanup RUN",
		"never retire",
		"never clean",
		"Workspace",
		"Worktree",
		"run.json",
		"secret scan",
	}
	for _, phrase := range required {
		if !strings.Contains(doc, phrase) {
			t.Errorf("parallel pilot runbook missing %q", phrase)
		}
	}
}
