package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"thread-dock/internal/runner"
)

const defaultGitHubHost = "github.com"

type hostedCommandRunner struct {
	base CommandRunner
	host string
}

type hostedGitHubMonitor struct {
	inner SnapshotSource
	host  string
}

type hostedHerdrMonitor struct {
	inner HerdrSnapshotSource
	host  string
}

func normalizeGitHubHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" {
		return defaultGitHubHost, nil
	}
	if strings.Contains(host, "://") {
		return "", errors.New("GitHub Host에는 https:// 없이 호스트 이름만 입력하세요")
	}
	parsed, err := url.Parse("https://" + host)
	if err != nil || parsed.Host != host || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("올바른 GitHub Host가 아닙니다: %s", raw)
	}
	return host, nil
}

func normalizeGitHubEnvironment(env map[string]string) (map[string]string, string, error) {
	host, err := normalizeGitHubHost(env["THREADDOCK_GITHUB_HOST"])
	if err != nil {
		return nil, "", err
	}
	normalized := cloneStringMap(env)
	delete(normalized, "THREADDOCK_GITHUB_HOST")

	projects := splitConfig(env["THREADDOCK_PROJECTS"])
	if len(projects) > 0 {
		values := make([]string, 0, len(projects))
		for _, value := range projects {
			parsed, parseErr := url.Parse(value)
			if parseErr != nil || parsed.Scheme != "https" || strings.ToLower(parsed.Host) != host || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
				return nil, "", errors.New("THREADDOCK_PROJECTS must contain GitHub user or org project URLs on the configured GitHub Host")
			}
			dotComURL := "https://" + defaultGitHubHost + parsed.EscapedPath()
			if !projectPattern.MatchString(dotComURL) {
				return nil, "", errors.New("THREADDOCK_PROJECTS must contain GitHub user or org project URLs")
			}
			values = append(values, dotComURL)
		}
		normalized["THREADDOCK_PROJECTS"] = strings.Join(values, ",")
	}
	return normalized, host, nil
}

func NewHostedGitHubMonitor(env map[string]string, process CommandRunner, timeout time.Duration) SnapshotSource {
	normalized, host, err := normalizeGitHubEnvironment(env)
	if err != nil {
		return &hostConfigErrorSource{err: err}
	}
	proxy := &hostedCommandRunner{base: process, host: host}
	return &hostedGitHubMonitor{inner: NewGitHubMonitor(normalized, proxy, timeout), host: host}
}

func NewHostedHerdrMonitor(env map[string]string, process CommandRunner, timeout time.Duration) HerdrSnapshotSource {
	_, host, err := normalizeGitHubEnvironment(env)
	if err != nil {
		host = defaultGitHubHost
	}
	proxy := &hostedCommandRunner{base: process, host: host}
	return &hostedHerdrMonitor{inner: NewHerdrMonitor(env, proxy, timeout), host: host}
}

type hostConfigErrorSource struct{ err error }

func (s *hostConfigErrorSource) FetchAll(context.Context) (Snapshot, error) {
	return Snapshot{}, s.err
}

func (r *hostedCommandRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	if r == nil || r.base == nil {
		return runner.Result{}, errors.New("hosted command runner is unavailable")
	}
	adjusted := append([]string(nil), args...)
	if r.host != "" && r.host != defaultGitHubHost && executable == "wsl.exe" {
		for index := 0; index+1 < len(adjusted); index++ {
			if adjusted[index] == "--exec" && adjusted[index+1] == "gh" {
				prefix := append([]string(nil), adjusted[:index+1]...)
				suffix := append([]string(nil), adjusted[index+2:]...)
				adjusted = append(prefix, "/usr/bin/env", "GH_HOST="+r.host, "gh")
				adjusted = append(adjusted, suffix...)
				break
			}
		}
		for index := 0; index+1 < len(adjusted); index++ {
			if adjusted[index] == "--repo" && !strings.HasPrefix(adjusted[index+1], r.host+"/") {
				adjusted[index+1] = r.host + "/" + adjusted[index+1]
			}
		}
	}

	result, err := r.base.Run(ctx, cwd, executable, adjusted...)
	if r.host != "" && r.host != defaultGitHubHost && result.Stdout != "" {
		result.Stdout = strings.ReplaceAll(result.Stdout, "https://"+r.host+"/", "https://"+defaultGitHubHost+"/")
		result.Stdout = strings.ReplaceAll(result.Stdout, r.host+"/", defaultGitHubHost+"/")
	}
	return result, err
}

func (m *hostedGitHubMonitor) FetchAll(ctx context.Context) (Snapshot, error) {
	if m == nil || m.inner == nil {
		return Snapshot{}, errors.New("hosted GitHub monitor is unavailable")
	}
	snapshot, err := m.inner.FetchAll(ctx)
	if err != nil || m.host == "" || m.host == defaultGitHubHost {
		return snapshot, err
	}
	rewriteSnapshotHost(&snapshot, m.host)
	return snapshot, nil
}

func (m *hostedHerdrMonitor) FetchHerdr(ctx context.Context) (HerdrSnapshot, error) {
	if m == nil || m.inner == nil {
		return HerdrSnapshot{}, errors.New("hosted Herdr monitor is unavailable")
	}
	snapshot, err := m.inner.FetchHerdr(ctx)
	if err != nil || m.host == "" || m.host == defaultGitHubHost {
		return snapshot, err
	}
	rewriteHerdrHost(&snapshot, m.host)
	return snapshot, nil
}

func rewriteGitHubHost(value, host string) string {
	if value == "" || host == "" || host == defaultGitHubHost {
		return value
	}
	value = strings.ReplaceAll(value, "https://"+defaultGitHubHost+"/", "https://"+host+"/")
	return strings.ReplaceAll(value, defaultGitHubHost+"/", host+"/")
}

func rewriteStrings(values []string, host string) {
	for index := range values {
		values[index] = rewriteGitHubHost(values[index], host)
	}
}

func rewriteLinks(values []Link, host string) {
	for index := range values {
		values[index].URL = rewriteGitHubHost(values[index].URL, host)
	}
}

func rewriteSnapshotHost(snapshot *Snapshot, host string) {
	if snapshot == nil {
		return
	}
	rewriteStrings(snapshot.EvidenceRefs, host)
	for projectIndex := range snapshot.Projects {
		project := &snapshot.Projects[projectIndex]
		rewriteStrings(project.EvidenceRefs, host)
		rewriteLinks(project.Links, host)
		for workIndex := range project.WorkItems {
			work := &project.WorkItems[workIndex]
			work.WorkID = rewriteGitHubHost(work.WorkID, host)
			work.Request = rewriteGitHubHost(work.Request, host)
			rewriteStrings(work.EvidenceRefs, host)
			rewriteLinks(work.Links, host)
			for handoffIndex := range work.Handoffs {
				work.Handoffs[handoffIndex].Summary = rewriteGitHubHost(work.Handoffs[handoffIndex].Summary, host)
				rewriteStrings(work.Handoffs[handoffIndex].EvidenceRefs, host)
			}
			if work.GitHub != nil {
				work.GitHub.URL = rewriteGitHubHost(work.GitHub.URL, host)
				rewriteStrings(work.GitHub.RelatedIssueURLs, host)
				rewriteStrings(work.GitHub.RelatedPullRequestURLs, host)
				for checkIndex := range work.GitHub.Checks {
					work.GitHub.Checks[checkIndex].URL = rewriteGitHubHost(work.GitHub.Checks[checkIndex].URL, host)
				}
			}
		}
	}
}

func rewriteHerdrHost(snapshot *HerdrSnapshot, host string) {
	if snapshot == nil {
		return
	}
	rewriteLinks(snapshot.Links, host)
	for index := range snapshot.Connections {
		connection := &snapshot.Connections[index]
		connection.IssueURL = rewriteGitHubHost(connection.IssueURL, host)
		connection.ProjectURL = rewriteGitHubHost(connection.ProjectURL, host)
		connection.Repository = rewriteGitHubHost(connection.Repository, host)
		connection.Handoff = rewriteGitHubHost(connection.Handoff, host)
	}
}
