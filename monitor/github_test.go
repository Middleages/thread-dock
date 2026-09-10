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

var issueFixture = map[string]any{"number": 1, "title": "Fix retries", "url": "https://github.com/acme/app/issues/1", "state": "OPEN", "body": "See https://github.com/acme/app/wiki", "updatedAt": "2026-09-09T10:00:00Z", "author": map[string]any{"login": "sol"}}

type ghFakeRunner struct {
	mu    sync.Mutex
	calls [][]string
	fn    func([]string) (runner.Result, error)
}

func (f *ghFakeRunner) Run(_ context.Context, _ string, _ string, args ...string) (runner.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), args...))
	f.mu.Unlock()
	return f.fn(args)
}

func jsonOutput(value any) string { data, _ := json.Marshal(value); return string(data) }

func TestGitHubMonitorValidatesSetupAndStoresOptionalSessionsPath(t *testing.T) {
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu", "THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json"}, &ghFakeRunner{}, time.Second)
	if monitor.config.err != nil || monitor.sessionsFile != "/tmp/sessions.json" {
		t.Fatalf("config=%#v sessions=%q", monitor.config, monitor.sessionsFile)
	}
	setup := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "bad", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, &ghFakeRunner{}, time.Second)
	snapshot, err := setup.FetchAll(context.Background())
	if err != nil || snapshot.SyncStatus != "setup_required" || !strings.Contains(strings.Join(snapshot.Notices, " "), "THREADDOCK_REPOS") {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	unknown := NewGitHubMonitor(map[string]string{"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu", "THREADDOCK_OTHER": "x"}, &ghFakeRunner{}, time.Second)
	snapshot, _ = unknown.FetchAll(context.Background())
	if snapshot.SyncStatus != "setup_required" || !strings.Contains(strings.Join(snapshot.Notices, " "), "THREADDOCK_OTHER") {
		t.Fatalf("unknown config snapshot=%#v", snapshot)
	}
	for name, env := range map[string]map[string]string{
		"no targets": {"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"},
		"missing distribution": {"THREADDOCK_REPOS": "acme/app"},
		"malformed project": {"THREADDOCK_PROJECTS": "https://github.com/acme/projects/2", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"},
	} {
		snapshot, _ = NewGitHubMonitor(env, &ghFakeRunner{}, time.Second).FetchAll(context.Background())
		if snapshot.SyncStatus != "setup_required" {
			t.Fatalf("%s snapshot=%#v", name, snapshot)
		}
	}
}

func TestGitHubMonitorRejectsMalformedRowsAndRetainsIssueCache(t *testing.T) {
	now := time.Unix(1000, 0)
	malformed := false
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if strings.Contains(strings.Join(args, " "), "issue list") {
			if malformed { return runner.Result{Stdout: `[{}]`}, nil }
			return runner.Result{Stdout: jsonOutput([]any{issueFixture})}, nil
		}
		if strings.Contains(strings.Join(args, " "), "pr list") {
			if malformed { return runner.Result{Stdout: `[{}]`}, nil }
			return runner.Result{Stdout: jsonOutput([]any{map[string]any{"number": 2, "title": "PR", "url": "https://github.com/acme/app/pull/2", "state": "OPEN", "closingIssuesReferences": []any{}, "statusCheckRollup": []any{}})}, nil
		}
		return runner.Result{Stdout: `[]`}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	monitor.now = func() time.Time { return now }
	first, _ := monitor.FetchAll(context.Background())
	if len(first.Projects[0].WorkItems) != 2 { t.Fatalf("first=%#v", first) }
	now = now.Add(61 * time.Second)
	malformed = true
	second, _ := monitor.FetchAll(context.Background())
	if len(second.Projects[0].WorkItems) != 2 || second.Freshness.State != "stale" { t.Fatalf("retained=%#v", second) }
}

func TestGitHubMonitorRejectsMalformedProjectFieldsAndRetainsBoardCache(t *testing.T) {
	now := time.Unix(1000, 0)
	malformed := false
	valid := map[string]any{"totalCount": 1, "items": []any{map[string]any{"id": "item-1", "content": nil, "fieldValues": []any{map[string]any{"field": map[string]any{"name": "Status"}, "name": "Waiting"}}}}}
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if malformed { return runner.Result{Stdout: jsonOutput(map[string]any{"totalCount": 1, "items": []any{map[string]any{"id": "item-1", "content": nil, "fieldValues": []any{map[string]any{"field": nil, "name": "Status", "value": "Waiting"}}}}})}, nil }
		return runner.Result{Stdout: jsonOutput(valid)}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_PROJECTS": "https://github.com/orgs/acme/projects/7", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	monitor.now = func() time.Time { return now }
	first, _ := monitor.FetchAll(context.Background())
	if first.Projects[0].WorkItems[0].GitHub.Fields["Status"] != "Waiting" { t.Fatalf("first=%#v", first) }
	now = now.Add(61 * time.Second)
	malformed = true
	second, _ := monitor.FetchAll(context.Background())
	if len(second.Projects[0].WorkItems) != 1 || second.Freshness.State != "stale" || second.Projects[0].WorkItems[0].GitHub.Fields["Status"] != "Waiting" { t.Fatalf("retained=%#v", second) }
}

func TestGitHubMonitorUsesExactGhArgumentArrays(t *testing.T) {
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if strings.Contains(strings.Join(args, " "), "project item-list") { return runner.Result{Stdout: `{"totalCount":0,"items":[]}`}, nil }
		return runner.Result{Stdout: `[]`}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_PROJECTS": "https://github.com/users/acme/projects/2", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu 24.04"}, fake, time.Second)
	if _, err := monitor.FetchAll(context.Background()); err != nil { t.Fatal(err) }
	want := [][]string{
		{"--distribution", "Ubuntu 24.04", "--exec", "gh", "issue", "list", "--repo", "acme/app", "--state", "all", "--limit", "100", "--json", "number,title,url,state,body,updatedAt,author"},
		{"--distribution", "Ubuntu 24.04", "--exec", "gh", "pr", "list", "--repo", "acme/app", "--state", "all", "--limit", "100", "--json", "number,title,url,state,body,updatedAt,reviewDecision,statusCheckRollup,closingIssuesReferences"},
		{"--distribution", "Ubuntu 24.04", "--exec", "gh", "project", "item-list", "2", "--owner", "acme", "--format", "json", "--limit", "100"},
	}
	if len(fake.calls) != len(want) { t.Fatalf("calls=%d want=%d", len(fake.calls), len(want)) }
	for index := range want {
		if strings.Join(fake.calls[index], "|") != strings.Join(want[index], "|") { t.Fatalf("call %d=%q want=%q", index, fake.calls[index], want[index]) }
	}
}

func TestGitHubMonitorDoesNotInferClosingRelationFromBodyURL(t *testing.T) {
	issue := map[string]any{"number": 1, "title": "Issue", "url": "https://github.com/acme/app/issues/1", "state": "OPEN", "body": "See https://github.com/acme/app/pull/99"}
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if strings.Contains(strings.Join(args, " "), "issue list") { return runner.Result{Stdout: jsonOutput([]any{issue})}, nil }
		return runner.Result{Stdout: jsonOutput([]any{map[string]any{"number": 99, "title": "PR", "url": "https://github.com/acme/app/pull/99", "state": "OPEN", "closingIssuesReferences": []any{}}})}, nil
	}}
	snapshot, _ := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second).FetchAll(context.Background())
	work := snapshot.Projects[0].WorkItems[0]
	if len(work.GitHub.RelatedPullRequestURLs) != 0 || len(work.Links) != 2 { t.Fatalf("work=%#v", work) }
}

func TestGitHubMonitorBoardFailurePreservesRepositoryWithDeterministicBarrier(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if strings.Contains(strings.Join(args, " "), "project item-list") { return runner.Result{ExitCode: 7}, errors.New("board down") }
		if strings.Contains(strings.Join(args, " "), "issue list") { close(started); <-release; return runner.Result{Stdout: jsonOutput([]any{issueFixture})}, nil }
		return runner.Result{Stdout: `[]`}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_PROJECTS": "https://github.com/users/acme/projects/2", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	first := make(chan Snapshot, 1)
	go func() { snapshot, _ := monitor.FetchAll(context.Background()); first <- snapshot }()
	select { case <-started: case <-time.After(time.Second): t.Fatal("aggregate did not reach repository read") }
	second := make(chan Snapshot, 1)
	go func() { snapshot, _ := monitor.FetchAll(context.Background()); second <- snapshot }()
	close(release)
	a, b := <-first, <-second
	if a.Revision != b.Revision || len(a.Projects[0].WorkItems) != 1 || a.Projects[1].SyncStatus != "degraded" { t.Fatalf("a=%#v b=%#v", a, b) }
}

func TestGitHubMonitorMapsRepositoryAndProjectItemsWithoutIssueNumberCollisions(t *testing.T) {
	projectPayload := map[string]any{"totalCount": 1, "items": []any{map[string]any{"id": "draft-1", "content": nil, "status": "Waiting", "priority": map[string]any{"name": "P2"}}}}
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "issue list") && strings.Contains(joined, "acme/app"): return runner.Result{Stdout: jsonOutput([]any{issueFixture})}, nil
		case strings.Contains(joined, "issue list") && strings.Contains(joined, "acme/api"): return runner.Result{Stdout: jsonOutput([]any{map[string]any{"number": 1, "title": "API", "url": "https://github.com/acme/api/issues/1", "state": "OPEN"}})}, nil
		case strings.Contains(joined, "pr list"): return runner.Result{Stdout: jsonOutput([]any{map[string]any{"number": 2, "title": "Fix", "url": "https://github.com/acme/app/pull/2", "state": "OPEN", "reviewDecision": "REVIEW_REQUIRED", "statusCheckRollup": []any{map[string]any{"name": "build", "status": "COMPLETED", "conclusion": "SUCCESS", "detailsUrl": "https://github.com/acme/app/actions/1"}}, "closingIssuesReferences": []any{map[string]any{"url": issueFixture["url"]}}}})}, nil
		case strings.Contains(joined, "project item-list"): return runner.Result{Stdout: jsonOutput(projectPayload)}, nil
		default: return runner.Result{Stdout: "[]"}, nil
		}
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app,acme/api", "THREADDOCK_PROJECTS": "https://github.com/orgs/acme/projects/7", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	snapshot, err := monitor.FetchAll(context.Background())
	if err != nil || len(snapshot.Projects) != 3 {
		t.Fatalf("projects=%d err=%v", len(snapshot.Projects), err)
	}
	app := snapshot.Projects[0]
	if app.ProjectID != "repo:acme/app" || len(app.WorkItems) != 2 || app.Links[2].URL != "https://github.com/acme/app/wiki" {
		t.Fatalf("app project=%#v", app)
	}
	if snapshot.Projects[1].WorkItems[0].WorkID == app.WorkItems[0].WorkID {
		t.Fatal("same issue number collided across repositories")
	}
	pr := app.WorkItems[1]
	if pr.GitHub == nil || len(pr.GitHub.RelatedIssueURLs) != 1 || pr.GitHub.Checks[0].URL == "" || pr.GitHub.ReviewDecision != "REVIEW_REQUIRED" {
		t.Fatalf("pr github=%#v", pr.GitHub)
	}
	board := snapshot.Projects[2]
	if board.WorkItems[0].GitHub == nil || board.WorkItems[0].GitHub.ContentAvailable || board.WorkItems[0].GitHub.Fields["status"] != "Waiting" || board.WorkItems[0].GitHub.Fields["priority"] != "P2" {
		t.Fatalf("board item=%#v", board.WorkItems[0])
	}
	if !strings.Contains(strings.Join(snapshot.Notices, " "), "내용을 확인할 수 없습니다") {
		t.Fatalf("notices=%v", snapshot.Notices)
	}
}

func TestGitHubMonitorRetainsSuccessfulSourcesAndCoalescesConcurrentFetches(t *testing.T) {
	var now = time.Unix(1000, 0)
	var failing bool
	var calls int
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		calls++
		if failing { return runner.Result{ExitCode: 7, Stderr: "private"}, errors.New("failed") }
		if strings.Contains(strings.Join(args, " "), "issue list") { return runner.Result{Stdout: jsonOutput([]any{issueFixture})}, nil }
		return runner.Result{Stdout: "[]"}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	monitor.now = func() time.Time { return now }
	first, second := make(chan Snapshot, 1), make(chan Snapshot, 1)
	go func() { s, _ := monitor.FetchAll(context.Background()); first <- s }()
	go func() { s, _ := monitor.FetchAll(context.Background()); second <- s }()
	a, b := <-first, <-second
	if calls != 2 || a.Revision != b.Revision { t.Fatalf("calls=%d revisions=%d,%d", calls, a.Revision, b.Revision) }
	now = now.Add(61 * time.Second)
	failing = true
	stale, _ := monitor.FetchAll(context.Background())
	if stale.Freshness.State != "stale" || len(stale.Projects[0].WorkItems) != 1 || !strings.Contains(strings.Join(stale.Notices, " "), "acme/app") { t.Fatalf("stale=%#v", stale) }
	if strings.Contains(strings.Join(stale.Notices, " "), "private") { t.Fatal("raw runner error leaked") }
}

func TestGitHubMonitorEmitsLimitNoticeAndRejectsMalformedResponsesWithoutReplacingCache(t *testing.T) {
	now := time.Unix(1000, 0)
	malformed := false
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if malformed { return runner.Result{Stdout: `{`}, nil }
		if strings.Contains(strings.Join(args, " "), "issue list") { return runner.Result{Stdout: jsonOutput(make([]any, 100))}, nil }
		return runner.Result{Stdout: "[]"}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	monitor.now = func() time.Time { return now }
	first, _ := monitor.FetchAll(context.Background())
	if !strings.Contains(strings.Join(first.Notices, " "), "100개") { t.Fatalf("notices=%v", first.Notices) }
	now = now.Add(61 * time.Second)
	malformed = true
	second, _ := monitor.FetchAll(context.Background())
	if len(second.Projects[0].WorkItems) != 100 || second.Freshness.State != "stale" { t.Fatalf("retained=%#v", second) }
}
