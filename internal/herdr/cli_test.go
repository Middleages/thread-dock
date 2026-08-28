package herdr

import (
	"context"
	"embed"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/runner"
)

// testFS contains captured Herdr v0.8.2 wire responses.
//
//go:embed testdata/v0.8.2/*.txt
var testFS embed.FS

type recordingRunner struct {
	responses map[string]string
	calls     [][]string
	fail      *runnerFailure
}

func (r *recordingRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	call := append([]string{executable}, args...)
	r.calls = append(r.calls, call)
	if r.fail != nil {
		return runner.Result{Stderr: r.fail.stderr, ExitCode: r.fail.exitCode}, &testError{r.fail.err}
	}
	key := strings.Join(call, "\x00")
	response, ok := r.responses[key]
	if !ok {
		return runner.Result{ExitCode: 1}, &testError{"unexpected command: " + strings.Join(call, " ")}
	}
	return runner.Result{Stdout: response, ExitCode: 0}, nil
}

type runnerFailure struct {
	stderr   string
	err      string
	exitCode int
}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func TestCreateWorktreeReturnsActualIDsAndUsesExplicitArguments(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00agent/184-integration\x00--base\x00main\x00--label\x00issue-184-integration\x00--no-focus": readFixture(t, "testdata/v0.8.2/worktree-create.txt"),
		"herdr\x00pane\x00list\x00--workspace\x00workspace-redacted": readFixture(t, "testdata/v0.8.2/pane-list.txt"),
	})
	cli := NewCLI(r, "herdr")

	got, err := cli.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Cwd: "/repo", Branch: "agent/184-integration", Base: "main", Label: "issue-184-integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != "workspace-redacted" || got.PaneID != "pane-redacted" {
		t.Fatalf("result=%#v", got)
	}
	if got.Path != "/redacted/worktree" {
		t.Fatalf("path=%q", got.Path)
	}
	want := [][]string{
		{"herdr", "worktree", "create", "--cwd", "/repo", "--branch", "agent/184-integration", "--base", "main", "--label", "issue-184-integration", "--no-focus"},
		{"herdr", "pane", "list", "--workspace", "workspace-redacted"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
}

func TestAgentLifecycleCommandsUseStructuredArguments(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00start\x00builder_api\x00--kind\x00opencode\x00--pane\x00pane-redacted":                           `{}`,
		"herdr\x00agent\x00prompt\x00builder_api\x00packet with spaces\n$(not-a-command)\x00--wait\x00--timeout\x003600000": `{}`,
		"herdr\x00agent\x00get\x00builder_api":                                                    readFixture(t, "testdata/v0.8.2/agent-get.txt"),
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": "recent output",
	})
	cli := NewCLI(r, "herdr")

	if err := cli.StartAgent(context.Background(), StartAgentRequest{Name: "builder_api", PaneID: "pane-redacted"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.Prompt(context.Background(), "builder_api", "packet with spaces\n$(not-a-command)"); err != nil {
		t.Fatal(err)
	}
	state, err := cli.Get(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if state != AgentStateWorking {
		t.Fatalf("state=%q", state)
	}
	recent, err := cli.ReadRecent(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if recent != "recent output" {
		t.Fatalf("recent=%q", recent)
	}
}

func TestRunnerFailureDoesNotExposeSecrets(t *testing.T) {
	packetSecret := "packet-secret-184"
	r := &recordingRunner{fail: &runnerFailure{
		stderr: "stderr-secret-184", err: "runner-secret-184", exitCode: 17,
	}}
	err := NewCLI(r, "herdr").Prompt(context.Background(), "builder_api", packetSecret)
	if err == nil {
		t.Fatal("expected prompt error")
	}
	message := err.Error()
	for _, secret := range []string{packetSecret, "stderr-secret-184", "runner-secret-184", "builder_api"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error contains secret %q: %q", secret, message)
		}
	}
	if message != "herdr agent prompt failed (exit code 17)" {
		t.Fatalf("error=%q", message)
	}
}

func TestReadRecentReturnsRawStdout(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": readFixture(t, "testdata/v0.8.2/recent-output.txt"),
	})
	got, err := NewCLI(r, "herdr").ReadRecent(context.Background(), "builder_api")
	if err != nil {
		t.Fatal(err)
	}
	if got != readFixture(t, "testdata/v0.8.2/recent-output.txt") {
		t.Fatalf("output=%q", got)
	}
}

func TestReadEvidenceAcceptsOnlyStructuredResultsWithDuration(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": `{"commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"2.3s"}]}`,
	})
	got, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api")
	if err != nil || got.CommitSHA == "" || got.Verification[0].Duration != "2.3s" {
		t.Fatalf("evidence=%#v err=%v", got, err)
	}
}

func TestFindWorktreeReconcilesPathWorkspaceAndPane(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00list":                             `{"result":{"worktrees":[{"branch":"agent/run-integration","label":"run","open_workspace_id":"workspace-run","path":"/work/run"}]}}`,
		"herdr\x00pane\x00list\x00--workspace\x00workspace-run": `{"result":{"panes":[{"pane_id":"pane-run","workspace_id":"workspace-run"}]}}`,
	})
	got, found, err := NewCLI(r, "herdr").FindWorktree(context.Background(), "/work/run", "run")
	if err != nil || !found || got.Path != "/work/run" || got.WorkspaceID != "workspace-run" || got.PaneID != "pane-run" {
		t.Fatalf("worktree=%#v found=%v err=%v", got, found, err)
	}
}

func TestReadEvidenceRejectsRawTranscriptAndMissingDuration(t *testing.T) {
	for name, output := range map[string]string{
		"raw":      "commit_sha: 0123456789abcdef0123456789abcdef01234567",
		"duration": `{"commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed"}]}`,
		"trailing": `{"commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test ./...","outcome":"passed","duration":"1s"}]}\nnot-json`,
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureRunner(t, map[string]string{
				"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": output,
			})
			if _, err := NewCLI(r, "herdr").ReadEvidence(context.Background(), "builder_api"); err == nil {
				t.Fatal("accepted untrusted evidence")
			}
		})
	}
}

func TestStartAgentAlwaysStartsOpenCode(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00start\x00builder_api\x00--kind\x00opencode\x00--pane\x00pane-redacted": `{}`,
	})
	if err := NewCLI(r, "herdr").StartAgent(context.Background(), StartAgentRequest{Name: "builder_api", PaneID: "pane-redacted"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateWorktreeRejectsMismatchedWorkspaceRelationships(t *testing.T) {
	create := `{"result":{"root_pane":{"pane_id":"pane-a","workspace_id":"workspace-b"},"workspace":{"workspace_id":"workspace-a"}}}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00branch\x00--base\x00main\x00--label\x00label\x00--no-focus": create,
	})
	_, err := NewCLI(r, "herdr").CreateWorktree(context.Background(), CreateWorktreeRequest{Cwd: "/repo", Branch: "branch", Base: "main", Label: "label"})
	if err == nil || err.Error() != "herdr worktree create failed (exit code 0)" {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateWorktreeRequiresPaneWorkspaceMatch(t *testing.T) {
	create := `{"result":{"root_pane":{"pane_id":"pane-a","workspace_id":"workspace-a"},"workspace":{"workspace_id":"workspace-a"}}}`
	panes := `{"result":{"panes":[{"pane_id":"pane-a","workspace_id":"workspace-b"}]}}`
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00branch\x00--base\x00main\x00--label\x00label\x00--no-focus": create,
		"herdr\x00pane\x00list\x00--workspace\x00workspace-a":                                                                     panes,
	})
	_, err := NewCLI(r, "herdr").CreateWorktree(context.Background(), CreateWorktreeRequest{Cwd: "/repo", Branch: "branch", Base: "main", Label: "label"})
	if err == nil || err.Error() != "herdr pane list failed (exit code 0)" {
		t.Fatalf("err=%v", err)
	}
}

func TestAgentStatesAreLifecycleOnly(t *testing.T) {
	for input, want := range map[string]AgentState{
		"working": AgentStateWorking, "blocked": AgentStateBlocked, "idle": AgentStateIdle,
		"done": AgentStateDone, "unknown": AgentStateUnknown, "other": AgentStateUnknown,
	} {
		if got := ParseAgentState(input); got != want {
			t.Errorf("ParseAgentState(%q)=%q want %q", input, got, want)
		}
	}
}

func TestMalformedHerdrOutputReturnsError(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00get\x00builder_api": `{"id":"cli:agent:get","result":{}}`,
	})
	_, err := NewCLI(r, "herdr").Get(context.Background(), "builder_api")
	if err == nil || err.Error() != "herdr agent get failed (exit code 0)" {
		t.Fatalf("err=%v", err)
	}
}

func fixtureRunner(t *testing.T, responses map[string]string) *recordingRunner {
	t.Helper()
	return &recordingRunner{responses: responses}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := testFS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
