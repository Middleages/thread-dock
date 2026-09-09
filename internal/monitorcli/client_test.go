package monitorcli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

type fakeRunner struct {
	calls []runnerCall
	steps []runnerStep
}

type runnerCall struct {
	cwd        string
	executable string
	args       []string
	deadline   time.Time
}

type runnerStep struct {
	result runner.Result
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	deadline, _ := ctx.Deadline()
	f.calls = append(f.calls, runnerCall{cwd: cwd, executable: executable, args: append([]string(nil), args...), deadline: deadline})
	if len(f.steps) == 0 {
		return runner.Result{}, errors.New("unexpected runner call")
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step.result, step.err
}

func TestFetchAllInvokesExactWSLCommand(t *testing.T) {
	fake := &fakeRunner{steps: []runnerStep{{result: runner.Result{Stdout: validJSON()}}}}
	client := New(fake, 2*time.Second)

	if _, err := client.FetchAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.calls))
	}
	call := fake.calls[0]
	if call.executable != "wsl.exe" {
		t.Fatalf("executable = %q, want wsl.exe", call.executable)
	}
	if strings.Join(call.args, " ") != "--exec agentctl project status --all --json" {
		t.Fatalf("args = %#v", call.args)
	}
}

func TestFetchAllBoundsRunnerContext(t *testing.T) {
	fake := &fakeRunner{steps: []runnerStep{{result: runner.Result{Stdout: validJSON()}}}}
	client := New(fake, 150*time.Millisecond)

	if _, err := client.FetchAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := fake.calls[0].deadline
	if deadline.IsZero() {
		t.Fatal("runner context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 150*time.Millisecond {
		t.Fatalf("remaining timeout = %s", remaining)
	}
}

func TestFetchAllRejectsNonzeroExit(t *testing.T) {
	fake := &fakeRunner{steps: []runnerStep{{result: runner.Result{ExitCode: 7, Stderr: "failed"}, err: errors.New("exit status 7")}}}
	client := New(fake, time.Second)

	if _, err := client.FetchAll(context.Background()); err == nil || !strings.Contains(err.Error(), "exit 7") {
		t.Fatalf("err = %v, want nonzero exit error", err)
	}
}

func TestFetchAllRejectsMalformedUnknownAndInvalidSchema(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{name: "malformed", json: "{"},
		{name: "trailing", json: validJSON() + " {}"},
		{name: "unknown field", json: strings.Replace(validJSON(), "\"schemaVersion\":2", "\"schemaVersion\":2,\"unexpected\":true", 1)},
		{name: "wrong schema", json: strings.Replace(validJSON(), "\"schemaVersion\":2", "\"schemaVersion\":1", 1)},
		{name: "missing projects", json: `{"schemaVersion":2,"revision":3,"observedAt":"2026-09-07T00:00:00Z","freshness":{"state":"fresh","syncStatus":"synced"},"state":"running","syncStatus":"synced","nextAction":"review","evidenceRefs":[]}`},
		{name: "missing evidence refs", json: `{"schemaVersion":2,"revision":3,"observedAt":"2026-09-07T00:00:00Z","freshness":{"state":"fresh","syncStatus":"synced"},"state":"running","syncStatus":"synced","nextAction":"review","projects":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRunner{steps: []runnerStep{{result: runner.Result{Stdout: tc.json}}}}
			if _, err := New(fake, time.Second).FetchAll(context.Background()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestFetchAllReturnsErrorBeforeFirstSuccessfulSnapshot(t *testing.T) {
	fake := &fakeRunner{steps: []runnerStep{{result: runner.Result{Stdout: "{"}, err: errors.New("malformed")}}}
	client := New(fake, time.Second)

	if snapshot, err := client.FetchAll(context.Background()); err == nil || len(snapshot.Projects) != 0 {
		t.Fatalf("snapshot=%#v err=%v, want empty snapshot and error", snapshot, err)
	}
}

func TestFetchAllReturnsCopiedLastGoodSnapshotAsStaleOfflineAfterFailure(t *testing.T) {
	fake := &fakeRunner{steps: []runnerStep{
		{result: runner.Result{Stdout: validJSON()}},
		{result: runner.Result{ExitCode: 1, Stderr: "offline"}, err: errors.New("exit status 1")},
	}}
	client := New(fake, time.Second)

	good, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	good.Projects[0].Name = "mutated caller copy"

	degraded, err := client.FetchAll(context.Background())
	if err == nil {
		t.Fatal("expected degraded fetch to preserve runner error")
	}
	if degraded.Projects[0].Name != "Payments" {
		t.Fatalf("degraded project = %#v, want retained last-good copy", degraded.Projects[0])
	}
	if degraded.Freshness.State != "stale" || degraded.Freshness.SyncStatus != "offline" || degraded.State != "stale" || degraded.SyncStatus != "offline" {
		t.Fatalf("degraded freshness = %#v snapshot state=%q sync=%q", degraded.Freshness, degraded.State, degraded.SyncStatus)
	}
	degraded.Projects[0].EvidenceRefs[0] = "mutated degraded copy"

	fake.steps = []runnerStep{{result: runner.Result{Stdout: validJSON()}}}
	latest, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest.Projects[0].EvidenceRefs[0] != "state://project-1" {
		t.Fatalf("retained snapshot was mutated = %#v", latest.Projects[0].EvidenceRefs)
	}
}

func validJSON() string {
	return `{"schemaVersion":2,"revision":3,"observedAt":"2026-09-07T00:00:00Z","freshness":{"state":"fresh","syncStatus":"synced"},"state":"running","syncStatus":"synced","nextAction":"review","evidenceRefs":[],"projects":[{"projectId":"project-1","name":"Payments","state":"running","syncStatus":"synced","nextAction":"review","evidenceRefs":["state://project-1"],"workItems":[{"workId":"work-1","title":"Retry payment failures","request":"Reduce duplicate retries","state":"needs_operator","syncStatus":"synced","nextAction":"review","evidenceRefs":["state://work-1"],"tasks":[{"taskId":"task-1","repoKey":"app","state":"verified","verification":"passed","review":"approved","merge":"ready"}],"publications":[{"intentId":"intent-1","key":"parent-issue","generation":1,"kind":"issue","status":"published","attempts":1,"url":"https://github.com/acme/app/issues/1"}],"decisions":[{"decisionId":"decision-1","summary":"Use bounded retries","status":"accepted"}],"handoffs":[{"summary":"Review the change","nextAction":"approve","evidenceRefs":["handoff://1"]}],"links":[{"kind":"github","label":"Parent Issue","url":"https://github.com/acme/app/issues/1"}]}]}]}`
}
