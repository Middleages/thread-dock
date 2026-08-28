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
}

func (r *recordingRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	call := append([]string{executable}, args...)
	r.calls = append(r.calls, call)
	key := strings.Join(call, "\x00")
	response, ok := r.responses[key]
	if !ok {
		return runner.Result{ExitCode: 1}, &testError{"unexpected command: " + strings.Join(call, " ")}
	}
	return runner.Result{Stdout: response, ExitCode: 0}, nil
}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func TestCreateWorktreeReturnsActualIDsAndUsesExplicitArguments(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00worktree\x00create\x00--cwd\x00/repo\x00--branch\x00agent/184-integration\x00--base\x00main\x00--label\x00issue-184-integration\x00--no-focus": readFixture(t, "testdata/v0.8.2/worktree-create.txt"),
		"herdr\x00pane\x00list\x00--workspace\x00w5": readFixture(t, "testdata/v0.8.2/pane-list.txt"),
	})
	cli := NewCLI(r, "herdr")

	got, err := cli.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Cwd: "/repo", Branch: "agent/184-integration", Base: "main", Label: "issue-184-integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != "w5" || got.PaneID != "w5:p1" {
		t.Fatalf("result=%#v", got)
	}
	want := [][]string{
		{"herdr", "worktree", "create", "--cwd", "/repo", "--branch", "agent/184-integration", "--base", "main", "--label", "issue-184-integration", "--no-focus"},
		{"herdr", "pane", "list", "--workspace", "w5"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
}

func TestAgentLifecycleCommandsUseStructuredArguments(t *testing.T) {
	r := fixtureRunner(t, map[string]string{
		"herdr\x00agent\x00start\x00builder_api\x00--kind\x00opencode\x00--pane\x00w5:p1":                                   `{}`,
		"herdr\x00agent\x00prompt\x00builder_api\x00packet with spaces\n$(not-a-command)\x00--wait\x00--timeout\x003600000": `{}`,
		"herdr\x00agent\x00get\x00builder_api":                                                    readFixture(t, "testdata/v0.8.2/agent-get.txt"),
		"herdr\x00agent\x00read\x00builder_api\x00--source\x00recent-unwrapped\x00--lines\x00120": `{"id":"cli:agent:read","result":{"output":"recent output"}}`,
	})
	cli := NewCLI(r, "herdr")

	if err := cli.StartAgent(context.Background(), StartAgentRequest{Name: "builder_api", Kind: "opencode", PaneID: "w5:p1"}); err != nil {
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
	if err == nil || !strings.Contains(err.Error(), "agent_status") {
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
