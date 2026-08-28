package pilot

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPilotCheckPrintsKoreanPassSummaryAndWritesEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("WSL pilot script is exercised on Unix-like builders")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("pilot runtime prerequisite jq is not installed")
	}

	repoRoot := repositoryRoot(t)
	temp := t.TempDir()
	stateDir := filepath.Join(temp, "state")
	runID := "td-test-run"
	runDir := filepath.Join(stateDir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainSHA := "0123456789abcdef0123456789abcdef01234567"
	snapshot := `{
  "runId":"td-test-run",
  "phase":"reviewing",
  "pendingAction":"",
  "parentIssue":101,
  "integration":{"path":"/managed/integration","branch":"agent/parent-integration"},
  "builder":{"name":"builder-td-test-run","commitSha":"abcdef0123456789abcdef0123456789abcdef01"},
  "reviewer":{"name":"reviewer-td-test-run","verification":["review schema sent"]},
  "builderWorktree":{"path":"/managed/builder","workspaceId":"ws-builder","paneId":"pane-builder"},
  "reviewerWorktree":{"path":"/managed/integration","workspaceId":"ws-reviewer","paneId":"pane-reviewer"},
  "builderPrompt":{"requestId":"td-test-run:builder-prompt"},
  "reviewerPrompt":{"requestId":"td-test-run:reviewer-prompt"}
}`
	writeFile(t, filepath.Join(runDir, "run.json"), snapshot, 0o600)
	contractPath := filepath.Join(temp, "pilot.json")
	writeFile(t, contractPath, `{"repository":{"owner":"PDX","name":"pilot-product"}}`, 0o600)
	configPath := filepath.Join(temp, "config.json")
	writeFile(t, configPath, `{"ghesHost":"https://github.example.test"}`, 0o600)

	fakeBin := filepath.Join(temp, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(fakeBin, "git"), `#!/usr/bin/env bash
case "$*" in
  "rev-parse HEAD") printf '%s\n' "${PILOT_TEST_MAIN_SHA}" ;;
  "status --short") exit 0 ;;
  "rev-parse --show-toplevel") printf '%s\n' "${PILOT_TEST_REPO_ROOT}" ;;
  *) printf 'unexpected git arguments: %s\n' "$*" >&2; exit 9 ;;
esac
`)
	writeExecutable(t, filepath.Join(fakeBin, "herdr"), `#!/usr/bin/env bash
printf '%s\n' '{"result":{"worktrees":[{"path":"/managed/builder","branch":"agent/pilot"},{"path":"/managed/integration","branch":"agent/parent-integration"}]}}'
`)
	writeExecutable(t, filepath.Join(fakeBin, "gh"), `#!/usr/bin/env bash
printf '%s\n' '[{"number":101,"body":"<!-- threaddock:td:td-test-run -->","html_url":"https://github.example.test/PDX/pilot-product/issues/101"},{"number":102,"body":"<!-- threaddock:td:td-test-run -->","html_url":"https://github.example.test/PDX/pilot-product/issues/102"}]'
`)

	sessionPath := filepath.Join(temp, "pilot-session.json")
	writeFile(t, sessionPath, `{"run":"`+runID+`","stateDir":"`+stateDir+`","mainBefore":"`+mainSHA+`","checkoutBefore":"","repoRoot":"`+repoRoot+`","contractPath":"`+contractPath+`","outagePassed":true,"wslRestartPassed":true,"snapshotDiffPassed":true}`, 0o600)

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "single-run-pilot.sh"), "--check")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
		"THREADDOCK_PILOT_SESSION="+sessionPath,
		"THREADDOCK_CONFIG="+configPath,
		"PILOT_TEST_MAIN_SHA="+mainSHA,
		"PILOT_TEST_REPO_ROOT="+repoRoot,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pilot check failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"Single-run 파일럿 결과",
		"GHES Issue 중복 방지",
		"Herdr Worktree 중복 방지",
		"main 브랜치 보호",
		"전체 결과: PASS",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}

	evidencePath := filepath.Join(stateDir, "pilot-results", runID+".md")
	evidence, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(evidence), "THREADDOCK_GH_TOKEN") ||
		!strings.Contains(string(evidence), "전체 결과: PASS") ||
		!strings.Contains(string(evidence), "ws-reviewer/pane-reviewer") {
		t.Fatalf("unexpected evidence:\n%s", evidence)
	}
}

func TestPilotRejectsCheckWhenReviewerIsNotReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("WSL pilot script is exercised on Unix-like builders")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("pilot runtime prerequisite jq is not installed")
	}

	repoRoot := repositoryRoot(t)
	temp := t.TempDir()
	stateDir := filepath.Join(temp, "state")
	runID := "td-not-ready"
	runDir := filepath.Join(stateDir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(runDir, "run.json"), `{"runId":"td-not-ready","phase":"building","pendingAction":"prompt_builder"}`, 0o600)
	contractPath := filepath.Join(temp, "pilot.json")
	writeFile(t, contractPath, `{"repository":{"owner":"PDX","name":"pilot-product"}}`, 0o600)
	configPath := filepath.Join(temp, "config.json")
	writeFile(t, configPath, `{"ghesHost":"https://github.example.test"}`, 0o600)
	sessionPath := filepath.Join(temp, "pilot-session.json")
	writeFile(t, sessionPath, `{"run":"`+runID+`","stateDir":"`+stateDir+`","mainBefore":"0123456789abcdef0123456789abcdef01234567","checkoutBefore":"","repoRoot":"`+repoRoot+`","contractPath":"`+contractPath+`","outagePassed":true,"wslRestartPassed":true,"snapshotDiffPassed":true}`, 0o600)
	fakeBin := filepath.Join(temp, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(fakeBin, "git"), `#!/usr/bin/env bash
case "$*" in
  "rev-parse HEAD") printf '%s\n' '0123456789abcdef0123456789abcdef01234567' ;;
  "status --short") exit 0 ;;
  *) exit 9 ;;
esac
`)
	writeExecutable(t, filepath.Join(fakeBin, "herdr"), `#!/usr/bin/env bash
printf '%s\n' '{"result":{"worktrees":[]}}'
`)
	writeExecutable(t, filepath.Join(fakeBin, "gh"), `#!/usr/bin/env bash
printf '%s\n' '[{"body":"<!-- threaddock:td:td-not-ready -->"},{"body":"<!-- threaddock:td:td-not-ready -->"}]'
`)

	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "single-run-pilot.sh"), "--check")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "THREADDOCK_PILOT_SESSION="+sessionPath, "THREADDOCK_CONFIG="+configPath)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure, output:\n%s", output)
	}
	if !strings.Contains(string(output), "Reviewer 준비 상태") || !strings.Contains(string(output), "FAIL") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source path is unavailable")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(filename), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	writeFile(t, path, contents, 0o755)
}
