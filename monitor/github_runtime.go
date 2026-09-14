package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"thread-dock/internal/runner"
)

const maxConcurrentGitHubTargets = 4

type githubCommandDiagnosticCollector struct {
	mu       sync.Mutex
	messages []string
}

func (c *githubCommandDiagnosticCollector) add(message string) {
	if c == nil || strings.TrimSpace(message) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.messages {
		if existing == message {
			return
		}
	}
	c.messages = append(c.messages, message)
}

func (c *githubCommandDiagnosticCollector) drain() []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	messages := append([]string(nil), c.messages...)
	c.messages = nil
	return messages
}

type githubDiagnosticRunner struct {
	base      CommandRunner
	collector *githubCommandDiagnosticCollector
}

func (r *githubDiagnosticRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	if r == nil || r.base == nil {
		return runner.Result{}, errors.New("github diagnostic runner is unavailable")
	}
	result, err := r.base.Run(ctx, cwd, executable, args...)
	if err != nil || result.ExitCode != 0 {
		if diagnostic := safeGitHubCommandDiagnostic(result.Stderr); diagnostic != "" {
			r.collector.add(fmt.Sprintf("%s 조회 실패: %s", githubCommandLabel(executable, args), diagnostic))
		}
	}
	return result, err
}

func (r *githubDiagnosticRunner) SupportsConcurrentRuns() bool {
	return r != nil && supportsConcurrentRuns(r.base)
}

func githubCommandLabel(executable string, args []string) string {
	tokens := append([]string{executable}, args...)
	for index := 0; index < len(tokens); index++ {
		if tokens[index] != "gh" || index+1 >= len(tokens) {
			continue
		}
		switch tokens[index+1] {
		case "issue":
			return "GitHub Issue"
		case "pr":
			return "GitHub PR"
		case "project":
			return "GitHub Project"
		}
	}
	return "GitHub"
}

type githubDiagnosticSource struct {
	inner     SnapshotSource
	collector *githubCommandDiagnosticCollector
}

func (s *githubDiagnosticSource) FetchAll(ctx context.Context) (Snapshot, error) {
	if s == nil || s.inner == nil {
		return Snapshot{}, errors.New("github diagnostic source is unavailable")
	}
	snapshot, err := s.inner.FetchAll(ctx)
	diagnostics := s.collector.drain()
	if err != nil {
		return snapshot, err
	}
	snapshot.Notices = append(snapshot.Notices, diagnostics...)
	return snapshot, nil
}

type concurrentGitHubSource struct {
	monitor *GitHubMonitor
}

func (s *concurrentGitHubSource) FetchAll(ctx context.Context) (Snapshot, error) {
	if s == nil || s.monitor == nil {
		return Snapshot{}, errors.New("concurrent GitHub source is unavailable")
	}
	return s.monitor.fetchAllConcurrent(ctx)
}

func (m *GitHubMonitor) fetchAllConcurrent(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	if m.inFlight == nil {
		call := &aggregateCall{done: make(chan struct{})}
		m.inFlight = call
		go m.completeAggregateConcurrent(call)
	}
	call := m.inFlight
	m.mu.Unlock()
	select {
	case <-call.done:
		return call.snapshot, call.err
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	}
}

func (m *GitHubMonitor) completeAggregateConcurrent(call *aggregateCall) {
	call.snapshot, call.err = m.loadConcurrent()
	m.mu.Lock()
	if m.inFlight == call {
		m.inFlight = nil
	}
	m.mu.Unlock()
	close(call.done)
}

type githubTargetResult struct {
	project Project
	notices []string
}

func (m *GitHubMonitor) loadConcurrent() (Snapshot, error) {
	now := m.now()
	if m.config.err != nil {
		return m.setupSnapshot(now, m.config.err.Error()), nil
	}
	if len(m.config.repos) == 0 && len(m.config.projects) == 0 {
		return m.setupSnapshot(now, "GitHub Monitor가 설정되지 않았습니다. 저장소, Project URL, WSL 배포판을 설정하세요."), nil
	}

	total := len(m.config.repos) + len(m.config.projects)
	results := make([]githubTargetResult, total)
	limit := maxConcurrentGitHubTargets
	if total < limit {
		limit = total
	}
	semaphore := make(chan struct{}, limit)
	var wg sync.WaitGroup
	launch := func(index int, fetch func(*[]string) Project) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			localNotices := []string{}
			results[index] = githubTargetResult{project: fetch(&localNotices), notices: localNotices}
		}()
	}

	for index, repo := range m.config.repos {
		repo := repo
		launch(index, func(notices *[]string) Project { return m.fetchRepository(repo, notices) })
	}
	offset := len(m.config.repos)
	for index, board := range m.config.projects {
		board := board
		launch(offset+index, func(notices *[]string) Project { return m.fetchProject(board, notices) })
	}
	wg.Wait()

	projects := make([]Project, 0, total)
	notices := []string{}
	stale := false
	var lastSynced *time.Time
	for _, result := range results {
		projects = append(projects, result.project)
		notices = append(notices, result.notices...)
		if result.project.State == "stale" {
			stale = true
		}
		lastSynced = laterTime(lastSynced, result.project.UpdatedAt)
	}

	m.mu.Lock()
	m.revision++
	revision := m.revision
	m.mu.Unlock()
	for index := range projects {
		notices = append(notices, projects[index].Notices...)
	}
	return Snapshot{
		SchemaVersion: 2,
		Revision:      revision,
		ObservedAt:    now,
		Freshness:     Freshness{State: choose(stale, "stale", "fresh"), SyncStatus: choose(stale, "degraded", "synced"), ObservedAt: timePtr(now), LastSyncedAt: lastSynced},
		State:         choose(stale, "stale", "running"),
		SyncStatus:    choose(stale, "degraded", "synced"),
		NextAction:    "review",
		EvidenceRefs:  projectEvidence(projects),
		Projects:      projects,
		Source:        "github",
		Notices:       notices,
	}, nil
}
