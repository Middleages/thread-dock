package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

type herdrRunner struct {
	mu    sync.Mutex
	calls [][]string
	fn    func([]string) (runner.Result, error)
}

func (r *herdrRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string{executable}, args...))
	r.mu.Unlock()
	return r.fn(append([]string{executable}, args...))
}

func herdrConfigJSON(bindings ...map[string]any) string {
	value := map[string]any{"version": 1, "bindings": bindings}
	data, _ := json.Marshal(value)
	return string(data)
}

func herdrAgent(name, status, workspace, tab, pane, cwd string) map[string]any {
	return map[string]any{
		"name": name, "agent_status": status, "workspace_id": workspace, "tab_id": tab,
		"pane_id": pane, "cwd": cwd, "foreground_cwd": cwd,
		"terminal_id": "secret-terminal", "provider_session": "secret-session", "transcript": "secret-transcript",
	}
}

func herdrList(agents ...map[string]any) string {
	if agents == nil {
		agents = []map[string]any{}
	}
	data, _ := json.Marshal(map[string]any{"result": map[string]any{"type": "agent_list", "agents": agents}})
	return string(data)
}

func TestHerdrMonitorValidatesConfigAndNormalizesV082Rows(t *testing.T) {
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if len(args) >= 5 && args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna", "worktree": "/repo"})}, nil
		}
		return runner.Result{Stdout: herdrList(herdrAgent("Luna", "working", "w1", "t1", "p1", "/repo/src"))}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	monitor.realpath = func(path string) (string, error) { return path, nil }
	snapshot, err := monitor.FetchHerdr(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != "fresh" || len(snapshot.Connections) != 1 || snapshot.Connections[0].Status != "connected" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if snapshot.Sessions[0].Agents[0].Name != "Luna" || snapshot.Sessions[0].Agents[0].ForegroundCWD != "/repo/src" {
		t.Fatalf("agent=%#v", snapshot.Sessions[0].Agents[0])
	}
	serialized, _ := json.Marshal(snapshot)
	if strings.Contains(string(serialized), "secret-") || strings.Contains(string(serialized), "transcript") {
		t.Fatalf("forbidden fields crossed wire: %s", serialized)
	}
	process.mu.Lock()
	defer process.mu.Unlock()
	if len(process.calls) != 2 {
		t.Fatalf("calls=%#v", process.calls)
	}
	wantHerdrArgs := []string{"wsl.exe", "--distribution", "Ubuntu", "--exec", "/bin/sh", "-c", `exec "$HOME/.local/bin/herdr" "$@"`, "threaddock-herdr", "--session", "feature", "agent", "list"}
	if strings.Join(process.calls[1], "\x00") != strings.Join(wantHerdrArgs, "\x00") {
		t.Fatalf("herdr args=%#v want=%#v", process.calls[1], wantHerdrArgs)
	}
}

func TestHerdrMonitorSeparatesSessionsAndRequiresCompleteIdentity(t *testing.T) {
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if len(args) > 4 && args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(
				map[string]any{"repository": "github.com/acme/app", "session": "feature-a", "workspaceId": "w1", "tabId": "t1", "paneId": "same", "agentName": "A", "worktree": "/repo"},
				map[string]any{"repository": "github.com/acme/app", "session": "feature-b", "workspaceId": "w2", "tabId": "t2", "paneId": "same", "agentName": "B", "worktree": "/repo"},
			)}, nil
		}
		name := args[len(args)-3]
		if name == "feature-a" {
			return runner.Result{Stdout: herdrList(herdrAgent("A", "working", "w1", "t1", "same", "/repo/src"))}, nil
		}
		return runner.Result{Stdout: herdrList(herdrAgent("B", "working", "w2", "t2", "same", "/repo/src"))}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	monitor.realpath = func(path string) (string, error) { return path, nil }
	snapshot, err := monitor.FetchHerdr(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(snapshot.Connections); got != 2 || snapshot.Connections[0].Status != "connected" || snapshot.Connections[1].Status != "connected" {
		t.Fatalf("connections=%#v", snapshot.Connections)
	}
	if snapshot.Sessions[0].Agents[0].PaneID != snapshot.Sessions[1].Agents[0].PaneID {
		t.Fatal("fixture must preserve same pane id")
	}

	partial := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "paneId": "p1"})}, nil
		}
		return runner.Result{Stdout: herdrList(herdrAgent("Luna", "working", "w1", "t1", "p1", "/repo"))}, nil
	}}, time.Second)
	partialSnapshot, _ := partial.FetchHerdr(context.Background())
	if partialSnapshot.Connections[0].Status != "unverified" {
		t.Fatalf("partial identity status=%q", partialSnapshot.Connections[0].Status)
	}
}

func TestHerdrMonitorAllowsCoordinatorWithoutWorktreeAndChecksCanonicalCWD(t *testing.T) {
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(
				map[string]any{"repository": "github.com/acme/app", "session": "central", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Sol", "role": "coordinator"},
				map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p2", "agentName": "Luna", "worktree": "/repo"},
			)}, nil
		}
		if strings.Contains(strings.Join(args, " "), "central") {
			return runner.Result{Stdout: herdrList(herdrAgent("Sol", "idle", "w1", "t1", "p1", "/elsewhere"))}, nil
		}
		return runner.Result{Stdout: herdrList(herdrAgent("Luna", "working", "w1", "t1", "p2", "/repo/src"))}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	monitor.realpath = func(path string) (string, error) {
		if path == "/repo" || path == "/repo/src" {
			return "/mnt/repo", nil
		}
		return path, nil
	}
	snapshot, err := monitor.FetchHerdr(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Connections[0].Status != "connected" || snapshot.Connections[1].Status != "connected" {
		t.Fatalf("connections=%#v", snapshot.Connections)
	}
}

func TestHerdrMonitorDistinguishesEmptyCacheFromMissingAfterFailure(t *testing.T) {
	now := time.Unix(1000, 0)
	fail := false
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna", "worktree": "/repo"})}, nil
		}
		if fail {
			return runner.Result{}, errors.New("secret stderr")
		}
		return runner.Result{Stdout: herdrList()}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	monitor.now = func() time.Time { return now }
	monitor.realpath = func(path string) (string, error) { return path, nil }
	first, _ := monitor.FetchHerdr(context.Background())
	if first.Connections[0].Status != "missing" {
		t.Fatalf("first=%#v", first.Connections)
	}
	now = now.Add(6 * time.Second)
	fail = true
	second, _ := monitor.FetchHerdr(context.Background())
	if second.Status != "cached" || second.Connections[0].Status != "cached" || strings.Contains(strings.Join(second.Notices, " "), "secret") {
		t.Fatalf("second=%#v notices=%#v", second.Connections, second.Notices)
	}
}

func TestHerdrMonitorTreatsMalformedAgentRowsAsFailedRefresh(t *testing.T) {
	now := time.Unix(1000, 0)
	malformed := false
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna", "worktree": "/repo"})}, nil
		}
		if malformed {
			return runner.Result{Stdout: `{"result":{"type":"agent_list","agents":[null]}}`}, nil
		}
		return runner.Result{Stdout: herdrList(herdrAgent("Luna", "working", "w1", "t1", "p1", "/repo"))}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	monitor.now = func() time.Time { return now }
	monitor.realpath = func(path string) (string, error) { return path, nil }
	first, _ := monitor.FetchHerdr(context.Background())
	if first.Status != "fresh" || first.Connections[0].Status != "connected" {
		t.Fatalf("first=%#v", first)
	}
	now = now.Add(6 * time.Second)
	malformed = true
	second, _ := monitor.FetchHerdr(context.Background())
	if second.Status != "cached" || second.Connections[0].Status != "cached" || len(second.Sessions[0].Agents) != 1 {
		t.Fatalf("malformed refresh erased cache: %#v", second)
	}
}

func TestHerdrMonitorRejectsMalformedAgentRowsWithoutCache(t *testing.T) {
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna"})}, nil
		}
		return runner.Result{Stdout: `{"result":{"type":"agent_list","agents":[{"name":42}]}}`}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	snapshot, _ := monitor.FetchHerdr(context.Background())
	if snapshot.Status != "offline" || snapshot.Connections[0].Status != "offline" {
		t.Fatalf("malformed first observation was treated as current: %#v", snapshot)
	}
}

func TestHerdrMonitorRejectsMalformedLocationRowsOnColdCache(t *testing.T) {
	for _, row := range []string{
		`{}`,
		`{"workspace_id":"w1","tab_id":"t1"}`,
		`{"workspace_id":"w1","tab_id":"t1","pane_id":"p1","name":null}`,
	} {
		t.Run(row, func(t *testing.T) {
			process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
				if args[4] == "/bin/cat" {
					return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna"})}, nil
				}
				return runner.Result{Stdout: `{"result":{"type":"agent_list","agents":[` + row + `]}}`}, nil
			}}
			monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
			snapshot, _ := monitor.FetchHerdr(context.Background())
			if snapshot.Status != "offline" || snapshot.Connections[0].Status != "offline" {
				t.Fatalf("malformed row became current: row=%s snapshot=%#v", row, snapshot)
			}
		})
	}
}

func TestHerdrMonitorAcceptsUnnamedAgentWithCompleteLocationAsUnconnected(t *testing.T) {
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p2", "agentName": "Luna"})}, nil
		}
		return runner.Result{Stdout: `{"result":{"type":"agent_list","agents":[{"agent_status":"working","workspace_id":"w1","tab_id":"t1","pane_id":"p1","cwd":"/repo"}]}}`}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	snapshot, _ := monitor.FetchHerdr(context.Background())
	if snapshot.Status != "fresh" || snapshot.Connections[0].Status != "missing" || len(snapshot.UnconnectedAgents) != 1 || snapshot.UnconnectedAgents[0].Name != "" {
		t.Fatalf("unnamed location agent=%#v connections=%#v", snapshot.UnconnectedAgents, snapshot.Connections)
	}
}

func TestHerdrMonitorTracksUnconnectedIdentityBeyondPaneID(t *testing.T) {
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		if args[4] == "/bin/cat" {
			return runner.Result{Stdout: herdrConfigJSON(map[string]any{"repository": "github.com/acme/app", "session": "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "same", "agentName": "A", "worktree": "/repo"})}, nil
		}
		return runner.Result{Stdout: herdrList(
			herdrAgent("A", "working", "w1", "t1", "same", "/repo"),
			herdrAgent("B", "idle", "w2", "t2", "same", "/repo"),
		)}, nil
	}}
	monitor := NewHerdrMonitor(map[string]string{"THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, process, time.Second)
	monitor.realpath = func(path string) (string, error) { return path, nil }
	snapshot, _ := monitor.FetchHerdr(context.Background())
	if snapshot.Connections[0].Status != "connected" || len(snapshot.UnconnectedAgents) != 1 || snapshot.UnconnectedAgents[0].Name != "B" {
		t.Fatalf("identity tracking=%#v connections=%#v", snapshot.UnconnectedAgents, snapshot.Connections)
	}
}

func TestCombinedMonitorKeepsSourcesIndependent(t *testing.T) {
	github := &fakeSnapshotSource{snapshot: Snapshot{SchemaVersion: 2, Projects: []Project{{ProjectID: "repo:acme/app"}}, SyncStatus: "synced"}}
	herdr := &fakeHerdrSource{snapshot: HerdrSnapshot{Status: "fresh", SyncStatus: "synced"}}
	source := NewCombinedMonitor(github, herdr)
	snapshot, err := source.FetchAll(context.Background())
	if err != nil || len(snapshot.Projects) != 1 || snapshot.Herdr == nil || snapshot.Herdr.Status != "fresh" {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	github.err = errors.New("github offline")
	herdr.err = errors.New("herdr offline")
	snapshot, err = source.FetchAll(context.Background())
	if err != nil || snapshot.Herdr == nil || snapshot.Herdr.Status != "offline" || len(snapshot.Projects) != 0 || snapshot.SyncStatus != "offline" {
		t.Fatalf("failed aggregate=%#v err=%v", snapshot, err)
	}
}

type fakeHerdrSource struct {
	snapshot HerdrSnapshot
	err      error
}

func (f *fakeHerdrSource) FetchHerdr(context.Context) (HerdrSnapshot, error) {
	if f.err != nil {
		return HerdrSnapshot{}, f.err
	}
	return f.snapshot, nil
}
