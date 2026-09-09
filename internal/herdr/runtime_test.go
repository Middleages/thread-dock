package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"thread-dock/internal/coordinator"
	"thread-dock/internal/runner"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
)

type runtimeScriptRunner struct {
	responses []string
	calls     [][]string
	failAt    int
}

func (r *runtimeScriptRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	r.calls = append(r.calls, append([]string{executable}, args...))
	if r.failAt > 0 && len(r.calls) == r.failAt {
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
	r := &runtimeScriptRunner{responses: []string{
		readFixture(t, "testdata/v0.8.2/runtime-worktree-open.txt"),
		readFixture(t, "testdata/v0.8.2/runtime-pane-list.txt"),
		`{}`,
		readFixture(t, "testdata/v0.8.2/runtime-agent-get.txt"),
		readFixture(t, "testdata/v0.8.2/runtime-evidence.txt"),
	}}
	cli := NewCLI(r, "herdr")
	rt := NewBuilderRuntime(cli, map[string]ProfileBinding{"builder-profile": {OpenCodeAgent: "threaddock-builder", RuntimeFingerprint: "fp-1"}})
	state := statev2.InvocationState{InvocationID: "inv-1", Role: "builder", LogicalProfile: "builder-profile", RuntimeFingerprint: "fp-1", LogicalWorkID: "logical-1"}
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
	if len(runner.calls) != 5 {
		t.Fatalf("calls=%#v", runner.calls)
	}
	if !strings.Contains(runner.calls[4][4], EvidenceSchemaExample) || !strings.Contains(runner.calls[4][4], "requestId=inv-1") || !strings.Contains(runner.calls[4][4], `{"taskId":"task-1"}`) {
		t.Fatalf("prompt=%q", runner.calls[4][4])
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
		{"done with stale evidence", EvidenceBeginMarker + "\n{" + "\"requestId\":\"old\",\"commitSha\":\"0123456789abcdef0123456789abcdef01234567\",\"verification\":[{\"command\":\"go test\",\"outcome\":\"passed\",\"duration\":\"1s\"}]}\n" + EvidenceEndMarker, AgentStateDone, coordinator.RuntimeObservationUnknown, true, false},
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
	if !errors.Is(err, ErrRuntimeIdentity) || got.Artifact != nil || len(r.calls) != 1 {
		t.Fatalf("observation=%#v err=%v calls=%#v", got, err, r.calls)
	}
}

func TestBuilderRuntimePromptFailureDoesNotRetry(t *testing.T) {
	rt, state, inv, r := builderRuntimeFixture(t)
	r.failAt = 5
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}, inv); !errors.Is(err, ErrRuntimePrompt) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 5 {
		t.Fatalf("prompt was retried: %#v", r.calls)
	}
}

func TestBuilderRuntimeLaunchDoesNotPromptAfterIdentityMismatch(t *testing.T) {
	rt, state, inv, r := builderRuntimeFixture(t)
	r.responses[3] = `{"id":"info","result":{"agent":{"name":"other-agent","pane_id":"pane-1","workspace_id":"ws-1","cwd":"/repo/worktree","agent_status":"working","agent_session":{"value":"session-1"}}}}`
	if _, err := rt.Launch(context.Background(), state, statev2.WorktreeIdentity{CanonicalPath: "/repo/worktree", GitCommonDir: "/repo/.git"}, inv); !errors.Is(err, ErrRuntimeIdentity) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 4 {
		t.Fatalf("unexpected calls=%#v", r.calls)
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
	if !errors.Is(err, ErrRuntimeEvidence) || got.Artifact != nil || got.State != coordinator.RuntimeObservationUnknown {
		t.Fatalf("observation=%#v err=%v", got, err)
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
