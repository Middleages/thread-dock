package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

func TestCanonicalPullRequestURLRequiresConfiguredHostAndNormalizesSlash(t *testing.T) {
	canonical, repo, number, err := canonicalPullRequestURL("https://ghe.example:8443/Acme/App/pulls/7/", "ghe.example:8443")
	if err != nil || canonical != "https://ghe.example:8443/Acme/App/pull/7" || repo.owner != "Acme" || repo.repo != "App" || number != 7 {
		t.Fatalf("canonical=%q repo=%#v number=%d err=%v", canonical, repo, number, err)
	}
	for _, raw := range []string{
		"https://github.com/Acme/App/pull/7",
		"http://ghe.example:8443/Acme/App/pull/7",
		"https://ghe.example:8443/Acme/App/pull/0",
		"https://ghe.example:8443/Acme/App/pull/7?tab=files",
	} {
		if _, _, _, err := canonicalPullRequestURL(raw, "ghe.example:8443"); err == nil {
			t.Fatalf("invalid URL accepted: %s", raw)
		}
	}
}

func TestPullRequestDetailUsesProjectsOnlyAndCoalescesCachedReads(t *testing.T) {
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if !strings.Contains(strings.Join(args, " "), "pr view") {
			return runner.Result{Stdout: `[]`}, nil
		}
		return runner.Result{Stdout: `{"number":7,"url":"https://ghe.example:8443/acme/app/pull/7","state":"OPEN","reviewDecision":"APPROVED","statusCheckRollup":[],"closingIssuesReferences":[{"url":"https://ghe.example:8443/acme/app/issues/2"}]}`}, nil
	}}
	monitor := NewHostedGitHubMonitor(map[string]string{
		"THREADDOCK_GITHUB_HOST":      "ghe.example:8443",
		"THREADDOCK_PROJECTS":         "https://ghe.example:8443/orgs/acme/projects/1",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}, fake, time.Second)
	source := monitor.(PullRequestDetailSource)
	first, err := source.GetPullRequestDetail(context.Background(), "https://GHE.EXAMPLE:8443/acme/app/pull/7/")
	if err != nil || first.GitHub == nil || first.GitHub.URL != "https://ghe.example:8443/acme/app/pull/7" || first.GitHub.RelatedIssueURLs[0] != "https://ghe.example:8443/acme/app/issues/2" {
		t.Fatalf("detail=%#v err=%v", first, err)
	}
	second, err := source.GetPullRequestDetail(context.Background(), "https://ghe.example:8443/acme/app/pulls/7")
	if err != nil || second.GitHub == nil || len(fake.calls) != 1 {
		t.Fatalf("cached detail=%#v calls=%d err=%v", second, len(fake.calls), err)
	}
}

func TestPullRequestDetailReturnsStaleCacheAndSanitizedNotice(t *testing.T) {
	failing := false
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if failing {
			return runner.Result{ExitCode: 1, Stderr: "private token abc"}, errors.New("private token abc")
		}
		return runner.Result{Stdout: `{"number":7,"url":"https://github.com/acme/app/pull/7","state":"OPEN","statusCheckRollup":[],"closingIssuesReferences":[]}`}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	now := time.Unix(100, 0)
	monitor.now = func() time.Time { return now }
	first, _ := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/7")
	if first.GitHub == nil {
		t.Fatal("first detail missing")
	}
	now = now.Add(githubTTL + time.Second)
	failing = true
	stale, err := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/7/")
	if err != nil || stale.GitHub == nil || !stale.GitHub.Stale || stale.Notice == "" || strings.Contains(stale.Notice, "private token") {
		t.Fatalf("stale=%#v err=%v", stale, err)
	}
}

func TestProjectItemTypeURLContradictionRemainsUnavailable(t *testing.T) {
	item := map[string]any{"id": "item-1", "content": map[string]any{
		"type": "PullRequest", "url": "https://github.com/acme/app/issues/7", "number": 7, "title": "Contradiction",
	}}
	work := mapProjectItem(item, projectTarget{owner: "acme", number: "1", url: "https://github.com/orgs/acme/projects/1"}, 0, sourceRead{})
	if work.GitHub == nil || work.GitHub.Kind != "project_item" || work.GitHub.ContentAvailable {
		t.Fatalf("contradictory item=%#v", work)
	}
}

func TestHostedRepositoryRelationsRejectWrongHostAndNormalizeCase(t *testing.T) {
	const host = "ghe.example:8443"
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "issue list"):
			return runner.Result{Stdout: `[{"number":2,"title":"Issue","url":"https://ghe.example:8443/acme/app/issues/2","state":"OPEN"}]`}, nil
		case strings.Contains(joined, "pr list"):
			return runner.Result{Stdout: `[{"number":7,"title":"PR","url":"https://GHE.EXAMPLE:8443/ACME/APP/pull/7","state":"OPEN","closingIssuesReferences":[{"url":"https://GHE.EXAMPLE:8443/ACME/APP/issues/2"},{"url":"https://GHE.EXAMPLE:8443/OTHER/REPO/issues/3"},{"url":"https://github.com/acme/app/issues/2"}]}]`}, nil
		default:
			return runner.Result{Stdout: `[]`}, nil
		}
	}}
	monitor := NewHostedGitHubMonitor(map[string]string{"THREADDOCK_GITHUB_HOST": host, "THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	snapshot, err := monitor.FetchAll(context.Background())
	if err != nil || len(snapshot.Projects) != 1 || len(snapshot.Projects[0].WorkItems) != 2 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	pr := snapshot.Projects[0].WorkItems[1]
	if len(pr.GitHub.RelatedIssueURLs) != 2 || pr.GitHub.RelatedIssueURLs[0] != "https://"+host+"/ACME/APP/issues/2" || pr.GitHub.RelatedIssueURLs[1] != "https://"+host+"/OTHER/REPO/issues/3" {
		t.Fatalf("wrong-host relation accepted: %#v", pr.GitHub)
	}
}

func TestPullRequestDetailNegativeCacheAndResponseIdentity(t *testing.T) {
	failing := false
	wrongIdentity := false
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if failing {
			return runner.Result{ExitCode: 1}, errors.New("offline")
		}
		if wrongIdentity {
			return runner.Result{Stdout: `{"number":7,"url":"https://github.com/acme/app/pull/8","state":"OPEN","statusCheckRollup":[],"closingIssuesReferences":[]}`}, nil
		}
		return runner.Result{Stdout: `{"number":7,"url":"https://github.com/ACME/APP/pull/7","state":"OPEN","statusCheckRollup":[],"closingIssuesReferences":[]}`}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_REPOS": "acme/app", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	now := time.Unix(100, 0)
	monitor.now = func() time.Time { return now }
	first, err := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/7")
	if err != nil || first.GitHub == nil {
		t.Fatalf("case-equivalent response rejected: %#v err=%v", first, err)
	}
	wrongIdentity = true
	now = now.Add(githubTTL + time.Second)
	wrong, err := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/8")
	if err != nil || wrong.GitHub != nil || wrong.Notice == "" {
		t.Fatalf("wrong identity accepted: %#v err=%v", wrong, err)
	}
	failing = true
	now = now.Add(githubTTL + time.Second)
	uncached, _ := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/9")
	if uncached.GitHub != nil || uncached.Notice == "" {
		t.Fatalf("uncached failure=%#v", uncached)
	}
	calls := len(fake.calls)
	retry, _ := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/9")
	if retry.GitHub != nil || retry.Notice == "" || len(fake.calls) != calls {
		t.Fatalf("negative cache retry=%#v calls=%d want=%d", retry, len(fake.calls), calls)
	}
	now = now.Add(githubTTL + time.Second)
	_, _ = monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/9")
	if len(fake.calls) != calls+1 {
		t.Fatalf("expired negative cache calls=%d want=%d", len(fake.calls), calls+1)
	}
}

func TestPullRequestDetailCoalescesConcurrentReads(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fake := &ghFakeRunner{fn: func(args []string) (runner.Result, error) {
		if strings.Contains(strings.Join(args, " "), "pr view") {
			close(entered)
			<-release
			return runner.Result{Stdout: `{"number":7,"url":"https://github.com/acme/app/pull/7","state":"OPEN","statusCheckRollup":[],"closingIssuesReferences":[]}`}, nil
		}
		return runner.Result{Stdout: `[]`}, nil
	}}
	monitor := NewGitHubMonitor(map[string]string{"THREADDOCK_PROJECTS": "https://github.com/orgs/acme/projects/1", "THREADDOCK_WSL_DISTRIBUTION": "Ubuntu"}, fake, time.Second)
	results := make(chan PullRequestDetail, 2)
	for range 2 {
		go func() {
			detail, _ := monitor.GetPullRequestDetail(context.Background(), "https://github.com/acme/app/pull/7")
			results <- detail
		}()
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("detail request did not start")
	}
	time.Sleep(10 * time.Millisecond)
	if len(fake.calls) != 1 {
		t.Fatalf("concurrent calls=%d", len(fake.calls))
	}
	close(release)
	for range 2 {
		select {
		case detail := <-results:
			if detail.GitHub == nil {
				t.Fatalf("detail=%#v", detail)
			}
		case <-time.After(time.Second):
			t.Fatal("coalesced detail did not complete")
		}
	}
}
