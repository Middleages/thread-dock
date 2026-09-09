package herdr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/runner"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

type runtimeScriptRunner struct {
	responses  []string
	calls      [][]string
	failAt     int
	failResult runner.Result
	notFoundAt int
}

func (r *runtimeScriptRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	r.calls = append(r.calls, append([]string{executable}, args...))
	if r.notFoundAt > 0 && len(r.calls) == r.notFoundAt {
		return runner.Result{Stdout: `{"error":{"code":"agent_not_found","message":"missing"}}`, ExitCode: 1}, errors.New("agent missing")
	}
	if r.failAt > 0 && len(r.calls) == r.failAt {
		if r.failResult.ExitCode != 0 || r.failResult.Stdout != "" || r.failResult.Stderr != "" {
			return r.failResult, errors.New("prompt timeout")
		}
		return runner.Result{ExitCode: 1}, errors.New("prompt timeout")
	}
	if len(r.responses) == 0 {
		return runner.Result{ExitCode: 1}, errors.New("unexpected command")
	}
	out := r.responses[0]
	r.responses = r.responses[1:]
	return runner.Result{Stdout: out}, nil
}

func builderRuntimeFixture(t *testing.T) (*Runtime, statev2.InvocationState, runtimecontract.Invocation, *runtimeScriptRunner) {
	t.Helper()
	r := &runtimeScriptRunner{notFoundAt: 1, responses: []string{
		readFixture(t, "testdata/v0.8.2/runtime-worktree-open.txt"),
		readFixture(t, "testdata/v0.8.2/runtime-pane-list.txt"),
		`{}`,
		readFixture(t, "testdata/v0.8.2/runtime-agent-get.txt"),
		readFixture(t, "testdata/v0.8.2/runtime-evidence.txt"),
	}}
	cli := NewCLI(r, "herdr")
	rt := NewBuilderRuntime(cli, map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", LogicalWorkID: "logical-1", LaunchRequested: true}
	inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, Packet: json.RawMessage(`{"taskId":"task-1"}`)}
	return rt, state, inv, r
}

func TestBuilderRuntimeLaunchUsesExactIdentityAndPromptsOnce(t *testing.T) {
	rt, state, inv, runner := builderRuntimeFixture(t)
	got, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git", Branch: "agent/inv-1", BaseSHA: strings.Repeat("a", 40)}, inv)
	if err != nil {
		t.Logf("calls=%#v err=%v", runner.calls, err)
		t.Fatal(err)
	}
	if got == "" || !strings.Contains(got, "session-1") || !strings.Contains(got, "ws-1") || !strings.Contains(got, "/repo/worktree") {
		t.Fatalf("provider identity=%q", got)
	}
	if len(runner.calls) != 6 {
		t.Fatalf("calls=%#v", runner.calls)
	}
	if !strings.Contains(runner.calls[5][4], EvidenceSchemaExample) || !strings.Contains(runner.calls[5][4], "requestId=inv-1") || !strings.Contains(runner.calls[5][4], `{"taskId":"task-1"}`) {
		t.Fatalf("prompt=%q", runner.calls[5][4])
	}
}

func TestBuilderRuntimeRejectsInvalidInvocationBeforeCLI(t *testing.T) {
	r := &runtimeScriptRunner{}
	rt := NewBuilderRuntime(NewCLI(r, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	state := statev2.InvocationState{InvocationID: "inv-1", Role: "reviewer", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1"}
	inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleReviewer, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, ReadOnly: true, Packet: json.RawMessage(`{}`)}
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree"}, inv); err == nil {
		t.Fatal("expected validation error")
	}
	if len(r.calls) != 0 {
		t.Fatalf("invalid launch mutated CLI: %#v", r.calls)
	}
}

func TestRuntimeInterfaceCompileCheck(t *testing.T) {
	var _ coordinator.Runtime = (*Runtime)(nil)
}

func TestBuilderRuntimeObserveEndsOnlyForCurrentMarkedEvidence(t *testing.T) {
	name := deterministicAgentName("inv-1", "/repo/worktree")
	tests := []struct {
		name         string
		recent       string
		state        AgentState
		wantState    string
		wantErr      bool
		wantArtifact bool
	}{
		{"idle without evidence", "plain transcript", AgentStateIdle, coordinator.RuntimeObservationUnknown, false, false},
		{"done with stale evidence", EvidenceBeginMarker + "\n{" + "\"requestId\":\"old\",\"commitSha\":\"0123456789abcdef0123456789abcdef01234567\",\"verification\":[{\"command\":\"go test\",\"outcome\":\"passed\",\"duration\":\"1s\"}]}\n" + EvidenceEndMarker, AgentStateDone, coordinator.RuntimeObservationUnknown, false, false},
		{"done with current evidence", EvidenceBeginMarker + "\n{" + "\"requestId\":\"inv-1\",\"commitSha\":\"0123456789abcdef0123456789abcdef01234567\",\"verification\":[{\"command\":\"go test\",\"outcome\":\"passed\",\"duration\":\"1s\"}]}\n" + EvidenceEndMarker, AgentStateDone, coordinator.RuntimeObservationEnded, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &runtimeScriptRunner{responses: []string{
				`{"id":"info","result":{"agent":{"name":"` + name + `","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"` + string(tc.state) + `","agent_session":{"value":"session-1"}}}}`,
				tc.recent,
				`{"id":"info","result":{"agent":{"name":"` + name + `","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"` + string(tc.state) + `","agent_session":{"value":"session-1"}}}}`,
			}}
			rt := NewBuilderRuntime(NewCLI(r, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
			state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", LogicalWorkID: "logical-1", LaunchRequested: true}
			inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, Packet: json.RawMessage(`{"taskId":"task-1"}`)}
			got, err := rt.Observe(context.Background(), state, inv)
			if tc.wantErr != (err != nil) || got.State != tc.wantState || (got.Artifact != nil) != tc.wantArtifact {
				t.Fatalf("observation=%#v err=%v", got, err)
			}
			if len(r.calls) != 3 {
				t.Fatalf("calls=%#v", r.calls)
			}
		})
	}
}

func TestBuilderRuntimeObserveRejectsPersistedIdentityChange(t *testing.T) {
	name := deterministicAgentName("inv-1", "/repo/worktree")
	r := &runtimeScriptRunner{responses: []string{
		`{"id":"info","result":{"agent":{"name":"` + name + `","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"idle","agent_session":{"value":"session-1"}}}}`,
	}}
	rt := NewBuilderRuntime(NewCLI(r, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", ProviderIdentity: `{"name":"` + name + `","session":"session-other","pane":"pane-1","workspace":"ws-1","path":"/repo/worktree"}`}
	inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, Packet: json.RawMessage(`{"taskId":"task-1"}`)}
	got, err := rt.Observe(context.Background(), state, inv)
	if err != nil || got.State != coordinator.RuntimeObservationUnknown || got.Artifact != nil || len(r.calls) != 1 {
		t.Fatalf("observation=%#v err=%v calls=%#v", got, err, r.calls)
	}
}

func TestBuilderRuntimePromptFailureDoesNotRetry(t *testing.T) {
	rt, state, inv, r := builderRuntimeFixture(t)
	r.failAt = 6
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}, inv); !errors.Is(err, ErrRuntimePrompt) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 6 {
		t.Fatalf("prompt was retried: %#v", r.calls)
	}
}

func TestBuilderRuntimePreservesValidatedPromptCodeWhileKeepingStaticSentinel(t *testing.T) {
	r := &runtimeScriptRunner{notFoundAt: 1, responses: []string{
		readFixture(t, "testdata/v0.8.2/runtime-worktree-open.txt"),
		readFixture(t, "testdata/v0.8.2/runtime-pane-list.txt"),
		`{}`,
		readFixture(t, "testdata/v0.8.2/runtime-agent-get.txt"),
	}}
	// The scripted runner's final response is deliberately an error returned
	// after the provider has already accepted all launch setup calls.
	r.failAt = 6
	r.failResult = runner.Result{ExitCode: 1, Stderr: `{"id":"prompt-1","error":{"code":"agent_prompt_stalled","message":"provider secret"}}`}
	rt, state, inv, _ := builderRuntimeFixture(t)
	rt.client = NewCLI(r, "herdr")
	_, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git", Branch: "agent/inv-1", BaseSHA: strings.Repeat("a", 40)}, inv)
	if !errors.Is(err, ErrRuntimePrompt) {
		t.Fatalf("error=%v, want ErrRuntimePrompt", err)
	}
	var promptErr *PromptError
	if !errors.As(err, &promptErr) || promptErr.Code() != "agent_prompt_stalled" {
		t.Fatalf("error=%v, prompt error=%#v", err, promptErr)
	}
}

func TestBuilderRuntimeLaunchDoesNotPromptAfterIdentityMismatch(t *testing.T) {
	rt, state, inv, r := builderRuntimeFixture(t)
	r.responses[3] = `{"id":"info","result":{"agent":{"name":"other-agent","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"working","agent_session":{"value":"session-1"}}}}`
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}, inv); !errors.Is(err, ErrRuntimeIdentity) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 5 {
		t.Fatalf("unexpected calls=%#v", r.calls)
	}
}

func TestBuilderRuntimeLaunchRequiresMarkedLaunchAndPreflightAbsence(t *testing.T) {
	rt, state, inv, r := builderRuntimeFixture(t)
	state.LaunchRequested = false
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}, inv); !errors.Is(err, ErrRuntimeConfiguration) {
		t.Fatalf("unmarked launch err=%v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("unmarked launch called CLI: %#v", r.calls)
	}

	state.LaunchRequested = true
	r.notFoundAt = 0
	r.responses[0] = `{"id":"info","result":{"agent":{"name":"other","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"working","agent_session":{"value":"session-1"}}}}`
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}, inv); !errors.Is(err, ErrRuntimeIdentity) {
		t.Fatalf("collision err=%v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("collision mutated CLI: %#v", r.calls)
	}
}

func TestBuilderRuntimeInvalidInputsDoNotCallCLI(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*statev2.InvocationState, *runtimecontract.Invocation, *statev2.WorktreeIdentity, map[string]ProfileBinding)
	}{
		{"role", func(s *statev2.InvocationState, i *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			s.Role = "reviewer"
			i.Role = runtimecontract.RoleReviewer
		}},
		{"schema", func(_ *statev2.InvocationState, i *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			i.OutputSchema = "wrong"
		}},
		{"read only", func(_ *statev2.InvocationState, i *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			i.ReadOnly = true
		}},
		{"path", func(_ *statev2.InvocationState, i *runtimecontract.Invocation, w *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			i.Worktree = "/repo/../worktree"
			w.CanonicalPath = i.Worktree
		}},
		{"profile", func(_ *statev2.InvocationState, i *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			i.ProfileID = "missing"
		}},
		{"profile name", func(_ *statev2.InvocationState, _ *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, p map[string]ProfileBinding) {
			p["builder-profile"] = ProfileBinding{OpenCodeAgent: strings.Repeat("x", 65), RuntimeFingerprint: "fp-1"}
		}},
		{"fingerprint", func(s *statev2.InvocationState, _ *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			s.RuntimeFingerprint = "other"
		}},
		{"packet", func(_ *statev2.InvocationState, i *runtimecontract.Invocation, _ *statev2.WorktreeIdentity, _ map[string]ProfileBinding) {
			i.Packet = json.RawMessage(`{"taskId":`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runtimeScriptRunner{}
			profiles := map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}}
			state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", LaunchRequested: true}
			worktree := statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}
			inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, Packet: json.RawMessage(`{"taskId":"task-1"}`)}
			tc.mutate(&state, &inv, &worktree, profiles)
			rt := NewBuilderRuntime(NewCLI(r, "herdr"), profiles)
			if _, err := rt.Launch(context.Background(), state, worktree, inv); err == nil {
				t.Fatal("expected validation error")
			}
			if len(r.calls) != 0 {
				t.Fatalf("invalid input called CLI: %#v", r.calls)
			}
		})
	}
}

func TestBuilderRuntimeObserveRejectsMalformedMarkedEvidence(t *testing.T) {
	name := deterministicAgentName("inv-1", "/repo/worktree")
	info := `{"id":"info","result":{"agent":{"name":"` + name + `","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"done","agent_session":{"value":"session-1"}}}}`
	r := &runtimeScriptRunner{responses: []string{info, EvidenceBeginMarker + "\n{not-json}\n" + EvidenceEndMarker, info}}
	rt := NewBuilderRuntime(NewCLI(r, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", ProviderIdentity: `{"name":"` + name + `","session":"session-1","pane":"pane-1","workspace":"ws-1","path":"/repo/worktree"}`}
	inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, Packet: json.RawMessage(`{"taskId":"task-1"}`)}
	got, err := rt.Observe(context.Background(), state, inv)
	if err != nil || got.Artifact != nil || got.State != coordinator.RuntimeObservationUnknown {
		t.Fatalf("observation=%#v err=%v", got, err)
	}
}

func TestBuilderRuntimeObservePostStatePreventsEnded(t *testing.T) {
	name := deterministicAgentName("inv-1", "/repo/worktree")
	info := func(state string) string {
		return `{"id":"info","result":{"agent":{"name":"` + name + `","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"` + state + `","agent_session":{"value":"session-1"}}}}`
	}
	evidence := EvidenceBeginMarker + `
{"requestId":"inv-1","commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}
` + EvidenceEndMarker
	for _, postState := range []string{"working", "blocked"} {
		t.Run(postState, func(t *testing.T) {
			r := &runtimeScriptRunner{responses: []string{info("idle"), evidence, info(postState)}}
			rt := NewBuilderRuntime(NewCLI(r, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
			state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", ProviderIdentity: `{"name":"` + name + `","session":"session-1","pane":"pane-1","workspace":"ws-1","path":"/repo/worktree"}`}
			inv := runtimecontract.Invocation{RequestID: "inv-1", Role: runtimecontract.RoleBuilder, ProfileID: "builder-profile", Worktree: "/repo/worktree", OutputSchema: runtimecontract.BuilderOutputSchema, Packet: json.RawMessage(`{"taskId":"task-1"}`)}
			got, err := rt.Observe(context.Background(), state, inv)
			if err != nil || got.State != coordinator.RuntimeObservationActive || got.Artifact != nil {
				t.Fatalf("observation=%#v err=%v", got, err)
			}
		})
	}
}

func TestBuilderRuntimeTerminateIsUnsupportedWithoutCLIEffects(t *testing.T) {
	r := &runtimeScriptRunner{}
	rt := NewBuilderRuntime(NewCLI(r, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	if err := rt.Terminate(context.Background(), statev2.InvocationState{}); !errors.Is(err, ErrExactTerminationUnsupported) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("terminate called CLI: %#v", r.calls)
	}
}

type coordinatorHerdrRunner struct {
	path          string
	name          string
	candidate     string
	malformed     bool
	promptFailure bool
	gets          int
	prompts       int
	calls         [][]string
}

func (r *coordinatorHerdrRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	r.calls = append(r.calls, append([]string{executable}, args...))
	switch {
	case len(args) >= 3 && args[0] == "agent" && args[1] == "get":
		r.gets++
		if r.gets == 1 {
			return runner.Result{Stdout: `{"error":{"code":"agent_not_found","message":"missing"}}`, ExitCode: 1}, errors.New("agent missing")
		}
		status := "working"
		if r.gets > 2 {
			status = "idle"
		}
		return runner.Result{Stdout: fmt.Sprintf(`{"id":"info","result":{"agent":{"name":%q,"pane_id":"pane-1","workspace_id":"ws-1","cwd":%q,"agent_status":%q,"agent_session":{"value":"session-1"}}}}`, r.name, r.path, status)}, nil
	case len(args) >= 2 && args[0] == "worktree" && args[1] == "open":
		return runner.Result{Stdout: fmt.Sprintf(`{"id":"open","result":{"root_pane":{"pane_id":"pane-1","workspace_id":"ws-1","cwd":%q}}}`, r.path)}, nil
	case len(args) >= 2 && args[0] == "pane" && args[1] == "list":
		return runner.Result{Stdout: `{"id":"panes","result":{"panes":[{"pane_id":"pane-1","workspace_id":"ws-1"}]}}`}, nil
	case len(args) >= 2 && args[0] == "agent" && args[1] == "start":
		return runner.Result{Stdout: `{}`}, nil
	case len(args) >= 2 && args[0] == "agent" && args[1] == "prompt":
		r.prompts++
		if r.promptFailure {
			return runner.Result{Stderr: `{"id":"prompt-1","error":{"code":"agent_prompt_stalled","message":"prompt-provider-secret"}}`, ExitCode: 1}, errors.New("prompt provider failed")
		}
		return runner.Result{Stdout: `{}`}, nil
	case len(args) >= 2 && args[0] == "agent" && args[1] == "read":
		if r.malformed {
			return runner.Result{Stdout: EvidenceBeginMarker + "\n{not-json}\n" + EvidenceEndMarker}, nil
		}
		return runner.Result{Stdout: EvidenceBeginMarker + "\n" + fmt.Sprintf(`{"requestId":"inv-1","commitSha":%q,"verification":[{"command":"go test ./internal/herdr","outcome":"passed","duration":"1s"}]}`, r.candidate) + "\n" + EvidenceEndMarker}, nil
	default:
		return runner.Result{ExitCode: 1}, fmt.Errorf("unexpected Herdr command: %v", args)
	}
}

func TestBuilderRuntimeCoordinatorStoreSmoke(t *testing.T) {
	runBuilderRuntimeCoordinatorStoreSmoke(t, coordinatorStoreSmokeConfig{})
}

func TestBuilderRuntimeCoordinatorMalformedEvidencePersistsBlocker(t *testing.T) {
	runBuilderRuntimeCoordinatorStoreSmoke(t, coordinatorStoreSmokeConfig{malformed: true})
}

func TestBuilderRuntimeCoordinatorStorePromptFailurePersistsBoundedBlocker(t *testing.T) {
	runBuilderRuntimeCoordinatorStoreSmoke(t, coordinatorStoreSmokeConfig{promptFailure: true})
}

type coordinatorStoreSmokeConfig struct {
	malformed     bool
	promptFailure bool
}

func runBuilderRuntimeCoordinatorStoreSmoke(t *testing.T, config coordinatorStoreSmokeConfig) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	worktreePath := filepath.Join(root, "worktree")
	git := runner.OSRunner{}
	if result, err := git.Run(ctx, root, "git", "init", "-b", "main", repo); err != nil || result.ExitCode != 0 {
		t.Fatalf("git init: %v/%#v", err, result)
	}
	for _, args := range [][]string{{"config", "user.email", "test@example.invalid"}, {"config", "user.name", "ThreadDock Test"}} {
		if result, err := git.Run(ctx, repo, "git", args...); err != nil || result.ExitCode != 0 {
			t.Fatalf("git config: %v/%#v", err, result)
		}
	}
	if result, err := git.Run(ctx, repo, "git", "commit", "--allow-empty", "-m", "base"); err != nil || result.ExitCode != 0 {
		t.Fatalf("base commit: %v/%#v", err, result)
	}
	baseResult, err := git.Run(ctx, repo, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	baseSHA := strings.TrimSpace(baseResult.Stdout)
	if result, err := git.Run(ctx, root, "git", "-C", repo, "worktree", "add", "-b", "agent/task-1", worktreePath, baseSHA); err != nil || result.ExitCode != 0 {
		t.Fatalf("worktree add: %v/%#v", err, result)
	}
	if err := os.MkdirAll(filepath.Join(worktreePath, "internal", "herdr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "internal", "herdr", "result.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result, err := git.Run(ctx, worktreePath, "git", "-C", worktreePath, "add", "internal/herdr/result.txt"); err != nil || result.ExitCode != 0 {
		t.Fatalf("candidate add: %v/%#v", err, result)
	}
	if result, err := git.Run(ctx, worktreePath, "git", "-C", worktreePath, "commit", "-m", "candidate"); err != nil || result.ExitCode != 0 {
		t.Fatalf("candidate commit: %v/%#v", err, result)
	}
	candidateResult, err := git.Run(ctx, worktreePath, "git", "-C", worktreePath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	candidateSHA := strings.TrimSpace(candidateResult.Stdout)

	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-smoke", ProjectID: "project-smoke", Revision: 1, Request: "builder smoke", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: baseSHA, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", Branch: "agent/task-1", AllowedPaths: []string{"internal/herdr/**"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "not required"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder-profile", Reviewer: "reviewer", Documenter: "documenter"}}
	var canonical bytes.Buffer
	if err := contractv2.Write(&canonical, contract); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(canonical.Bytes())
	contractHash := fmt.Sprintf("%x", hash[:])
	store := statev2.NewStore(root)
	planned := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: contractHash, Contract: contract, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, Control: statev2.WorkControl{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit, PriorAttempts: []statev2.AttemptSummary{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	planned, err = store.CreatePlan(ctx, planned, "plan-smoke", "payload-smoke")
	if err != nil {
		t.Fatal(err)
	}
	approve := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: planned.Revision, RequestID: "approve-smoke", Work: &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approve-smoke", ContractHash: contractHash}}
	approve.PayloadHash, err = statev2.TransitionPayloadHash(approve)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := store.Apply(ctx, approve)
	if err != nil {
		t.Fatal(err)
	}
	name := deterministicAgentName("inv-1", worktreePath)
	provider := &coordinatorHerdrRunner{path: worktreePath, name: name, candidate: candidateSHA, malformed: config.malformed, promptFailure: config.promptFailure}
	rt := NewBuilderRuntime(NewCLI(provider, "herdr"), map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	inspector := worktree.New(runner.OSRunner{}, repo)
	c := coordinator.NewCoordinator(store, rt, nil, inspector, coordinator.NewOwnerLocker(root), "owner-smoke", 41, time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if _, err := c.Activate(ctx, contract.WorkID); err != nil {
		t.Fatal(err)
	}
	reserve := statev2.TaskTransition{TaskID: "task-1", Action: statev2.TaskReserveInvocation, InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, BuilderAttempt: 1, At: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC), Worktree: &statev2.WorktreeIdentity{CanonicalPath: worktreePath, GitCommonDir: filepath.Join(repo, ".git"), Branch: "agent/task-1", BaseSHA: baseSHA}, Invocation: &statev2.InvocationState{LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1"}}
	reserveRequest := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: approved.Revision, RequestID: "reserve-smoke", Task: &reserve}
	reserveRequest.PayloadHash, err = statev2.TransitionPayloadHash(reserveRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, reserveRequest); err != nil {
		t.Fatal(err)
	}

	first := <-c.SubmitRuntime(ctx, contract.WorkID, "task-1", "inv-1")
	if config.promptFailure {
		if first.Err == nil || !errors.Is(first.Err, ErrRuntimePrompt) {
			t.Fatalf("prompt failure result=%v, want ErrRuntimePrompt", first.Err)
		}
		var promptErr *PromptError
		if !errors.As(first.Err, &promptErr) || promptErr.Code() != "agent_prompt_stalled" {
			t.Fatalf("prompt failure result=%v prompt error=%#v", first.Err, promptErr)
		}
		blocked, loadErr := store.Load(ctx, contract.WorkID)
		if loadErr != nil || blocked.State != statev2.StateNeedsOperator || blocked.TaskStates["task-1"].Status != statev2.TaskNeedsOperator || blocked.TaskStates["task-1"].Candidate != nil || !blocked.TaskStates["task-1"].Invocation.LaunchRequested {
			t.Fatalf("prompt failure state=%#v err=%v", blocked, loadErr)
		}
		if blocked.Control.Blocker == nil || blocked.Control.Blocker.Kind != statev2.BlockerKindRuntimeUnknown || strings.Contains(blocked.Control.Blocker.Diagnostic, "prompt-provider-secret") {
			t.Fatalf("prompt failure blocker=%#v, provider secret leaked or blocker missing", blocked.Control.Blocker)
		}
		calls := len(provider.calls)
		second := <-c.SubmitRuntime(ctx, contract.WorkID, "task-1", "inv-1")
		if !errors.Is(second.Err, coordinator.ErrRuntimeBlocked) || len(provider.calls) != calls || provider.prompts != 1 {
			t.Fatalf("replay result=%#v calls=%d prompts=%d, want blocked without provider retry", second, len(provider.calls), provider.prompts)
		}
		if err := c.Close(ctx); err != nil {
			t.Fatal(err)
		}
		return
	}
	if first.Err != nil {
		t.Fatalf("launch result: %v", first.Err)
	}
	running, err := store.Load(ctx, contract.WorkID)
	if err != nil || running.TaskStates["task-1"].Status != statev2.TaskRunning {
		t.Fatalf("running snapshot=%#v err=%v", running.TaskStates["task-1"], err)
	}
	second := <-c.SubmitRuntime(ctx, contract.WorkID, "task-1", "inv-1")
	if config.malformed {
		if second.Err == nil {
			t.Fatal("malformed evidence unexpectedly settled")
		}
		blocked, loadErr := store.Load(ctx, contract.WorkID)
		if loadErr != nil || blocked.State != statev2.StateNeedsOperator || blocked.Control.Blocker == nil || blocked.Control.Blocker.Kind != statev2.BlockerKindRuntimeUnknown {
			t.Fatalf("malformed state=%#v err=%v", blocked, loadErr)
		}
		if err := c.Close(ctx); err != nil {
			t.Fatal(err)
		}
		return
	}
	if second.Err != nil {
		t.Fatalf("observe result: %v", second.Err)
	}
	final, err := store.Load(ctx, contract.WorkID)
	if err != nil || final.TaskStates["task-1"].Status != statev2.TaskCandidateReady || final.TaskStates["task-1"].Candidate == nil || final.TaskStates["task-1"].Candidate.CandidateSHA != candidateSHA {
		t.Fatalf("candidate snapshot=%#v err=%v", final.TaskStates["task-1"], err)
	}
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
