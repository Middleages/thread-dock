package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

type parallelGHFakeRunner struct {
	*ghFakeRunner
}

func (r *parallelGHFakeRunner) SupportsConcurrentRuns() bool { return true }

func TestHostedGitHubMonitorOverlapsIndependentRepoAndProjectReads(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	base := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "issue list"):
			entered <- "repo"
			<-release
			return runner.Result{Stdout: `[]`}, nil
		case strings.Contains(joined, "pr list"):
			return runner.Result{Stdout: `[]`}, nil
		case strings.Contains(joined, "project item-list"):
			entered <- "project"
			<-release
			return runner.Result{Stdout: `{"totalCount":0,"items":[]}`}, nil
		default:
			return runner.Result{Stdout: `[]`}, nil
		}
	}}
	fake := &parallelGHFakeRunner{ghFakeRunner: base}

	monitor := NewHostedGitHubMonitor(map[string]string{
		"THREADDOCK_REPOS":            "acme/app",
		"THREADDOCK_PROJECTS":         "https://github.com/orgs/acme/projects/4",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}, fake, time.Second)

	done := make(chan struct{})
	go func() {
		_, _ = monitor.FetchAll(context.Background())
		close(done)
	}()

	seen := map[string]bool{}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for len(seen) < 2 {
		select {
		case kind := <-entered:
			seen[kind] = true
		case <-timer.C:
			close(release)
			t.Fatalf("repository and Project reads did not overlap, entered=%v", seen)
		}
	}
	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GitHub aggregate did not finish after concurrent reads were released")
	}
}

func TestHostedGitHubMonitorSurfacesClassifiedGitHubCLIDiagnostic(t *testing.T) {
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if strings.Contains(strings.Join(args, " "), "project item-list") {
			return runner.Result{ExitCode: 1, Stderr: "error: your authentication token is missing required scopes [project]\nTo request it, run: gh auth refresh -s project\n"}, errors.New("process failed")
		}
		return runner.Result{Stdout: `[]`}, nil
	}}
	monitor := NewHostedGitHubMonitor(map[string]string{
		"THREADDOCK_PROJECTS":         "https://github.com/orgs/acme/projects/4",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}, fake, time.Second)

	snapshot, err := monitor.FetchAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(snapshot.Notices, "\n")
	if !strings.Contains(joined, "GitHub Project 조회 실패") || !strings.Contains(joined, "GitHub 인증 scope가 부족합니다: project") {
		t.Fatalf("notices=%q", joined)
	}
	if strings.Contains(joined, "authentication token") || strings.Contains(joined, "gh auth refresh") {
		t.Fatalf("raw stderr must not reach operator notices: %q", joined)
	}
}

func TestHostedGitHubMonitorOrdersParallelFailureNoticesByConfiguredTarget(t *testing.T) {
	enteredIssue := make(chan struct{})
	enteredProject := make(chan struct{})
	projectReturned := make(chan struct{})
	releaseIssue := make(chan struct{})
	releaseProject := make(chan struct{})
	fake := &parallelGHFakeRunner{ghFakeRunner: &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "issue list"):
			close(enteredIssue)
			<-releaseIssue
			return runner.Result{ExitCode: 1, Stderr: "error: missing required scopes [project]"}, errors.New("process failed")
		case strings.Contains(joined, "project item-list"):
			close(enteredProject)
			<-releaseProject
			close(projectReturned)
			return runner.Result{ExitCode: 1, Stderr: "error: authentication required"}, errors.New("process failed")
		default:
			return runner.Result{Stdout: `[]`}, nil
		}
	}}}
	monitor := NewHostedGitHubMonitor(map[string]string{
		"THREADDOCK_REPOS":            "acme/app",
		"THREADDOCK_PROJECTS":         "https://github.com/orgs/acme/projects/4",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}, fake, time.Second)

	done := make(chan Snapshot)
	go func() {
		snapshot, _ := monitor.FetchAll(context.Background())
		done <- snapshot
	}()
	select {
	case <-enteredIssue:
	case <-time.After(time.Second):
		t.Fatal("repository issue read did not start")
	}
	select {
	case <-enteredProject:
	case <-time.After(time.Second):
		t.Fatal("project read did not start")
	}
	close(releaseProject)
	select {
	case <-projectReturned:
	case <-time.After(time.Second):
		t.Fatal("project failure did not return")
	}
	close(releaseIssue)

	var snapshot Snapshot
	select {
	case snapshot = <-done:
	case <-time.After(time.Second):
		t.Fatal("GitHub aggregate did not finish after failures were released")
	}
	var diagnosticOrder []string
	for _, notice := range snapshot.Notices {
		if strings.Contains(notice, "GitHub Issue 조회 실패") || strings.Contains(notice, "GitHub Project 조회 실패") {
			diagnosticOrder = append(diagnosticOrder, notice)
		}
	}
	want := []string{
		"GitHub Issue 조회 실패: GitHub 인증 scope가 부족합니다: project",
		"GitHub Project 조회 실패: GitHub 인증 상태를 확인하세요",
	}
	if strings.Join(diagnosticOrder, "\n") != strings.Join(want, "\n") {
		t.Fatalf("diagnostic notices=%q want=%q all=%q", diagnosticOrder, want, snapshot.Notices)
	}
}

func TestOpenOnlyRunnerPreservesConcurrentCapability(t *testing.T) {
	fake := &parallelGHFakeRunner{ghFakeRunner: &ghFakeRunner{fn: func([]string) (runner.Result, error) { return runner.Result{}, nil }}}
	if !supportsConcurrentRuns(openOnlyCommandRunner{base: fake}) {
		t.Fatal("open-only wrapper must preserve the production runner concurrency capability")
	}
}
