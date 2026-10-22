package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

func TestGitHubMonitorAcceptsEnterpriseProjectURLAndRoutesGhToConfiguredHost(t *testing.T) {
	const host = "github.samsungds.net"
	issue := map[string]any{
		"number":    1,
		"title":     "Enterprise issue",
		"url":       "https://" + host + "/FDYPhotoDX/app/issues/1",
		"state":     "OPEN",
		"body":      "See https://" + host + "/FDYPhotoDX/app/wiki",
		"updatedAt": "2026-09-11T00:00:00Z",
		"author":    map[string]any{"login": "sol"},
	}
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "issue list"):
			return runner.Result{Stdout: jsonOutput([]any{issue})}, nil
		case strings.Contains(joined, "pr list"):
			return runner.Result{Stdout: `[]`}, nil
		case strings.Contains(joined, "project item-list"):
			return runner.Result{Stdout: `{"totalCount":0,"items":[]}`}, nil
		default:
			return runner.Result{Stdout: `[]`}, nil
		}
	}}

	monitor := NewHostedGitHubMonitor(map[string]string{
		"THREADDOCK_GITHUB_HOST":      host,
		"THREADDOCK_REPOS":            "FDYPhotoDX/app",
		"THREADDOCK_PROJECTS":         "https://" + host + "/orgs/FDYPhotoDX/projects/4",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}, fake, time.Second)

	snapshot, err := monitor.FetchAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Projects) != 2 {
		t.Fatalf("projects=%#v", snapshot.Projects)
	}
	if got := snapshot.Projects[0].EvidenceRefs[0]; got != "https://"+host+"/FDYPhotoDX/app" {
		t.Fatalf("repository evidence=%q", got)
	}
	if got := snapshot.Projects[0].WorkItems[0].GitHub.URL; got != "https://"+host+"/FDYPhotoDX/app/issues/1" {
		t.Fatalf("issue url=%q", got)
	}
	if got := snapshot.Projects[1].Links[0].URL; got != "https://"+host+"/orgs/FDYPhotoDX/projects/4" {
		t.Fatalf("project url=%q", got)
	}

	wantPrefix := []string{"--distribution", "Ubuntu", "--exec", "/usr/bin/env", "GH_HOST=" + host, "gh"}
	if len(fake.calls) != 3 {
		t.Fatalf("calls=%#v", fake.calls)
	}
	for index, call := range fake.calls {
		if len(call) < len(wantPrefix) || strings.Join(call[:len(wantPrefix)], "|") != strings.Join(wantPrefix, "|") {
			t.Fatalf("call %d=%q does not route gh via GH_HOST", index, call)
		}
	}
	if joined := strings.Join(fake.calls[0], "|"); !strings.Contains(joined, "--repo|"+host+"/FDYPhotoDX/app") {
		t.Fatalf("enterprise repo argument missing host: %q", fake.calls[0])
	}
}

func TestGitHubMonitorRejectsProjectURLFromDifferentHost(t *testing.T) {
	monitor := NewHostedGitHubMonitor(map[string]string{
		"THREADDOCK_GITHUB_HOST":      "github.samsungds.net",
		"THREADDOCK_PROJECTS":         "https://github.com/orgs/FDYPhotoDX/projects/4",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}, &ghFakeRunner{}, time.Second)
	if _, err := monitor.FetchAll(context.Background()); err == nil {
		t.Fatal("project URL on a different GitHub host should be rejected")
	}
}

func TestHostedHerdrKeepsConfiguredHostAndRejectsDotComLocator(t *testing.T) {
	const host = "ghe.example:8443"
	valid := herdrConfigJSON(map[string]any{
		"issueUrl":   "https://" + host + "/acme/app/issues/7",
		"projectUrl": "https://" + host + "/orgs/acme/projects/4",
		"repository": host + "/acme/app",
		"session":    "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna", "role": "coordinator",
	})
	config := valid
	process := &herdrRunner{fn: func(args []string) (runner.Result, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "/bin/cat"):
			return runner.Result{Stdout: config}, nil
		case strings.Contains(joined, "agent list"):
			return runner.Result{Stdout: herdrList(herdrAgent("Luna", "working", "w1", "t1", "p1", "/repo/src"))}, nil
		case strings.Contains(joined, "workspace list"):
			return runner.Result{Stdout: herdrWorkspaceList()}, nil
		default:
			return runner.Result{Stdout: `[]`}, nil
		}
	}}
	env := map[string]string{"THREADDOCK_GITHUB_HOST": host, "THREADDOCK_SESSIONS_FILE": "/tmp/sessions.json", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}
	monitor := NewHostedHerdrMonitor(env, process, time.Second)
	snapshot, err := monitor.FetchHerdr(context.Background())
	if err != nil || snapshot.Status != "fresh" || len(snapshot.Connections) != 1 {
		t.Fatalf("valid GHES snapshot=%#v err=%v", snapshot, err)
	}
	connection := snapshot.Connections[0]
	if connection.IssueURL != "https://"+host+"/acme/app/issues/7" || connection.ProjectURL != "https://"+host+"/orgs/acme/projects/4" || connection.Repository != host+"/acme/app" {
		t.Fatalf("valid GHES locator=%#v", connection)
	}

	config = herdrConfigJSON(map[string]any{
		"issueUrl": "https://github.com/acme/app/issues/7",
		"session":  "feature", "workspaceId": "w1", "tabId": "t1", "paneId": "p1", "agentName": "Luna", "role": "coordinator",
	})
	wrong, err := monitor.FetchHerdr(context.Background())
	if err != nil || wrong.Status != "unverified" || wrong.SyncStatus != "setup_required" || len(wrong.Connections) != 0 {
		t.Fatalf("wrong-host locator promoted: %#v err=%v", wrong, err)
	}
}
