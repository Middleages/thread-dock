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
		"git clone --no-local",
		"go test -race ./...",
		"make check",
		"go build -trimpath",
		"worktree list --porcelain",
		"rev-parse --show-toplevel",
		"rev-parse --abbrev-ref HEAD",
		"rev-parse HEAD",
		"status --porcelain=v1",
		"herdr workspace get",
		"! herdr workspace get \"$WORKSPACE_ID\"",
		"herdr pane list --workspace",
		"jq --arg updatedAt",
		"test ! -e \"$WORKTREE_PATH\"",
		"test ! -e \"$PILOT_STATE/runs/$RUN\"",
		"git -C \"$PILOT_REPO\" worktree list --porcelain | grep -F -- \"$WORKTREE_PATH\"",
		"! git -C \"$PILOT_REPO\" worktree list --porcelain | grep -F -- \"$WORKTREE_PATH\"",
		"gitleaks detect --no-banner --redact --source \"$PILOT_ROOT\"",
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
