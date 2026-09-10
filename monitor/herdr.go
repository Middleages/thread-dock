package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	herdrTTL        = 5 * time.Second
	maxHerdrOutput  = 512 * 1024
	herdrShell      = `exec "$HOME/.local/bin/herdr" "$@"`
	herdrCommandTag = "threaddock-herdr"
)

var herdrSessionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type HerdrSnapshotSource interface {
	FetchHerdr(context.Context) (HerdrSnapshot, error)
}

type HerdrSnapshot struct {
	Source            string               `json:"source"`
	SchemaVersion     int                  `json:"schemaVersion"`
	Revision          uint64               `json:"revision"`
	ObservedAt        time.Time            `json:"observedAt"`
	Status            string               `json:"status"`
	State             string               `json:"state,omitempty"`
	SyncStatus        string               `json:"syncStatus"`
	NextAction        string               `json:"nextAction,omitempty"`
	Freshness         Freshness            `json:"freshness"`
	Notices           []string             `json:"notices"`
	Sessions          []HerdrSession       `json:"sessions"`
	Connections       []HerdrConnection    `json:"connections"`
	UnconnectedAgents []HerdrObservedAgent `json:"unconnectedAgents"`
	Links             []Link               `json:"links"`
}

type HerdrAgent struct {
	Name          string `json:"name,omitempty"`
	AgentStatus   string `json:"agent_status,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	TabID         string `json:"tab_id,omitempty"`
	PaneID        string `json:"pane_id,omitempty"`
	CWD           string `json:"cwd,omitempty"`
	ForegroundCWD string `json:"foreground_cwd,omitempty"`
}

type HerdrObservedAgent struct {
	HerdrAgent
	Session string `json:"session"`
}

type HerdrSession struct {
	Session    string       `json:"session"`
	Status     string       `json:"status"`
	ObservedAt *time.Time   `json:"observedAt,omitempty"`
	Agents     []HerdrAgent `json:"agents"`
	Links      []Link       `json:"links,omitempty"`
}

type HerdrLocation struct {
	Session     string `json:"session,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	TabID       string `json:"tabId,omitempty"`
	PaneID      string `json:"paneId,omitempty"`
	AgentName   string `json:"agentName,omitempty"`
	CWD         string `json:"cwd,omitempty"`
}

type HerdrConnection struct {
	IssueURL    string        `json:"issueUrl,omitempty"`
	ProjectURL  string        `json:"projectUrl,omitempty"`
	Repository  string        `json:"repository,omitempty"`
	Worktree    string        `json:"worktree,omitempty"`
	Session     string        `json:"session"`
	WorkspaceID string        `json:"workspaceId,omitempty"`
	TabID       string        `json:"tabId,omitempty"`
	PaneID      string        `json:"paneId,omitempty"`
	AgentName   string        `json:"agentName,omitempty"`
	Role        string        `json:"role,omitempty"`
	Status      string        `json:"status"`
	AgentStatus string        `json:"agentStatus,omitempty"`
	ObservedAt  *time.Time    `json:"observedAt,omitempty"`
	NextAction  string        `json:"nextAction"`
	Handoff     string        `json:"handoff"`
	Location    HerdrLocation `json:"location"`
}

type herdrBinding struct {
	IssueURL, ProjectURL, Repository string
	Worktree, Session                string
	WorkspaceID, TabID, PaneID       string
	AgentName, Role                  string
	Conflict                         string
}

type parsedHerdrConfig struct {
	bindings []herdrBinding
	sessions []string
}

type herdrCachedSession struct {
	agents        []HerdrAgent
	observedAt    time.Time
	nextAttemptAt time.Time
	status        string
	hasData       bool
}

type herdrCall struct {
	done     chan struct{}
	snapshot HerdrSnapshot
	err      error
}

type HerdrMonitor struct {
	env          map[string]string
	process      CommandRunner
	distribution string
	timeout      time.Duration
	now          func() time.Time
	realpath     func(string) (string, error)

	mu       sync.Mutex
	cache    map[string]herdrCachedSession
	revision uint64
	inFlight *herdrCall
}

func NewHerdrMonitor(env map[string]string, process CommandRunner, timeout time.Duration) *HerdrMonitor {
	return &HerdrMonitor{
		env:          cloneStringMap(env),
		process:      process,
		distribution: strings.TrimSpace(env["THREADDOCK_WSL_DISTRIBUTION"]),
		timeout:      timeout,
		now:          time.Now,
		cache:        make(map[string]herdrCachedSession),
	}
}

func (m *HerdrMonitor) FetchHerdr(ctx context.Context) (HerdrSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return HerdrSnapshot{}, err
	}
	m.mu.Lock()
	if m.inFlight == nil {
		call := &herdrCall{done: make(chan struct{})}
		m.inFlight = call
		go m.completeHerdr(call)
	}
	call := m.inFlight
	m.mu.Unlock()
	select {
	case <-call.done:
		return call.snapshot, call.err
	case <-ctx.Done():
		return HerdrSnapshot{}, ctx.Err()
	}
}

func (m *HerdrMonitor) completeHerdr(call *herdrCall) {
	call.snapshot, call.err = m.load(context.Background())
	m.mu.Lock()
	close(call.done)
	if m.inFlight == call {
		m.inFlight = nil
	}
	m.mu.Unlock()
}

func (m *HerdrMonitor) load(ctx context.Context) (HerdrSnapshot, error) {
	now := m.now()
	file := strings.TrimSpace(m.env["THREADDOCK_SESSIONS_FILE"])
	if file == "" {
		return m.statusSnapshot(now, "disabled", "disabled", "unknown", "configure", []string{"Herdr 연결 파일이 설정되지 않았습니다. THREADDOCK_SESSIONS_FILE에 읽을 연결 JSON을 지정하세요."}), nil
	}
	if !strings.HasPrefix(file, "/") {
		return m.statusSnapshot(now, "unverified", "setup_required", "needs_operator", "configure", []string{"THREADDOCK_SESSIONS_FILE은 WSL 내부의 절대 경로여야 합니다."}), nil
	}
	if m.process == nil || m.distribution == "" {
		return m.statusSnapshot(now, "unverified", "setup_required", "needs_operator", "configure", []string{"Herdr 조회에는 THREADDOCK_WSL_DISTRIBUTION과 WSL 명령 경계가 필요합니다."}), nil
	}
	raw, err := m.runWSL(ctx, "/bin/cat", file)
	if err != nil {
		return m.statusSnapshot(now, "unverified", "setup_required", "needs_operator", "configure", []string{"Herdr 연결 파일을 읽지 못했습니다. WSL 경로와 권한을 확인하세요."}), nil
	}
	config, err := parseHerdrConfig(string(raw))
	if err != nil {
		return m.statusSnapshot(now, "unverified", "setup_required", "needs_operator", "configure", []string{err.Error()}), nil
	}
	notices := []string{}
	observed := make([]herdrSessionResult, 0, len(config.sessions))
	for _, session := range config.sessions {
		entry := m.observeSession(ctx, session, now, &notices)
		observed = append(observed, entry)
	}
	bySession := make(map[string]herdrSessionResult, len(observed))
	for _, entry := range observed {
		bySession[entry.session] = entry
	}
	connections := make([]HerdrConnection, 0, len(config.bindings))
	matched := map[string]bool{}
	for _, binding := range config.bindings {
		entry := bySession[binding.Session]
		connection, matchedKey := m.connect(binding, entry)
		connections = append(connections, connection)
		if matchedKey != "" {
			matched[matchedKey] = true
		}
	}
	unconnected := make([]HerdrObservedAgent, 0)
	for _, entry := range observed {
		for _, agent := range entry.agents {
			if !matched[agentKey(entry.session, agent)] {
				unconnected = append(unconnected, HerdrObservedAgent{HerdrAgent: agent, Session: entry.session})
			}
		}
	}
	sessions := make([]HerdrSession, 0, len(observed))
	var lastSynced *time.Time
	hasFailure, hasCache := false, false
	for _, entry := range observed {
		var observedAt *time.Time
		if !entry.observedAt.IsZero() {
			observedAt = timePtr(entry.observedAt)
			lastSynced = laterTime(lastSynced, observedAt)
		}
		sessions = append(sessions, HerdrSession{Session: entry.session, Status: entry.status, ObservedAt: observedAt, Agents: entry.agents, Links: []Link{{Kind: "herdr", Label: "Herdr " + entry.session, URL: "herdr://session/" + entry.session}}})
		hasFailure = hasFailure || entry.status == "offline"
		hasCache = hasCache || entry.status == "cached"
	}
	status := "fresh"
	if hasFailure {
		status = "offline"
		if lastSynced != nil {
			status = "cached"
		}
	} else if hasCache {
		status = "cached"
	}
	syncStatus, state, next := "synced", "running", "observe"
	if status == "cached" {
		syncStatus, state, next = "degraded", "stale", "recheck"
	} else if status == "offline" {
		syncStatus, state, next = "offline", "unknown", "recheck"
	}
	return m.snapshot(now, HerdrSnapshot{Status: status, State: state, SyncStatus: syncStatus, NextAction: next, Freshness: Freshness{State: stateToFreshness(state), SyncStatus: syncStatus, ObservedAt: timePtr(now), LastSyncedAt: lastSynced}, Notices: notices, Sessions: sessions, Connections: connections, UnconnectedAgents: unconnected}), nil
}

type herdrSessionResult struct {
	session    string
	agents     []HerdrAgent
	observedAt time.Time
	status     string
}

func (m *HerdrMonitor) observeSession(ctx context.Context, session string, now time.Time, notices *[]string) herdrSessionResult {
	m.mu.Lock()
	cached, ok := m.cache[session]
	m.mu.Unlock()
	if ok && now.Before(cached.nextAttemptAt) {
		return herdrSessionResult{session: session, agents: cached.agents, observedAt: cached.observedAt, status: cached.status}
	}
	if ok && cached.hasData && now.Sub(cached.observedAt) < herdrTTL {
		return herdrSessionResult{session: session, agents: cached.agents, observedAt: cached.observedAt, status: "fresh"}
	}
	output, err := m.runHerdr(ctx, "--session", session, "agent", "list")
	if err == nil {
		agents, decodeErr := normalizeHerdrAgents(string(output))
		err = decodeErr
		if err == nil {
			entry := herdrCachedSession{agents: agents, observedAt: now, nextAttemptAt: now.Add(herdrTTL), status: "fresh", hasData: true}
			m.mu.Lock()
			m.cache[session] = entry
			m.mu.Unlock()
			return herdrSessionResult{session: session, agents: agents, observedAt: now, status: "fresh"}
		}
	}
	*notices = append(*notices, fmt.Sprintf("Herdr 세션 %s을 조회하지 못했습니다. 세션과 Herdr 연결을 다시 확인하세요.", session))
	cached.nextAttemptAt = now.Add(herdrTTL)
	if cached.hasData {
		cached.status = "cached"
		m.mu.Lock()
		m.cache[session] = cached
		m.mu.Unlock()
		return herdrSessionResult{session: session, agents: cached.agents, observedAt: cached.observedAt, status: "cached"}
	}
	cached.status = "offline"
	m.mu.Lock()
	m.cache[session] = cached
	m.mu.Unlock()
	return herdrSessionResult{session: session, status: "offline"}
}

func (m *HerdrMonitor) connect(binding herdrBinding, entry herdrSessionResult) (HerdrConnection, string) {
	connection := HerdrConnection{IssueURL: binding.IssueURL, ProjectURL: binding.ProjectURL, Repository: binding.Repository, Worktree: binding.Worktree, Session: binding.Session, WorkspaceID: binding.WorkspaceID, TabID: binding.TabID, PaneID: binding.PaneID, AgentName: binding.AgentName, Role: binding.Role, Status: "missing", NextAction: "recheck"}
	if binding.Conflict != "" {
		connection.Status = "conflict"
	} else if entry.status == "offline" {
		connection.Status = "offline"
	} else if !completeBindingIdentity(binding) {
		connection.Status = "unverified"
	} else {
		var matches []HerdrAgent
		for _, agent := range entry.agents {
			if binding.AgentName == agent.Name && binding.WorkspaceID == agent.WorkspaceID && binding.TabID == agent.TabID && binding.PaneID == agent.PaneID {
				matches = append(matches, agent)
			}
		}
		if len(matches) > 1 {
			connection.Status = "unverified"
		} else if len(matches) == 0 {
			if entry.status == "cached" {
				connection.Status = "cached"
			}
			for _, agent := range entry.agents {
				if binding.PaneID != "" && binding.PaneID == agent.PaneID {
					connection.Status = "conflict"
					break
				}
			}
		} else {
			agent := matches[0]
			connection.Status = chooseValue(entry.status == "cached", "cached", "connected")
			if binding.Worktree == "" && binding.Role != "coordinator" {
				connection.Status = "unverified"
			} else if binding.Worktree != "" {
				root, rootErr := m.canonical(binding.Worktree)
				cwdValue := agent.CWD
				if cwdValue == "" {
					cwdValue = agent.ForegroundCWD
				}
				cwd, cwdErr := m.canonical(cwdValue)
				if rootErr != nil || cwdErr != nil {
					connection.Status = "unverified"
				} else if !pathWithin(root, cwd) {
					connection.Status = "conflict"
				}
			}
			connection.AgentStatus = agent.AgentStatus
			connection.Location = HerdrLocation{Session: binding.Session, WorkspaceID: agent.WorkspaceID, TabID: agent.TabID, PaneID: agent.PaneID, AgentName: agent.Name, CWD: firstNonEmpty(agent.CWD, agent.ForegroundCWD)}
			return m.finishConnection(connection, entry.observedAt), agentKey(entry.session, agent)
		}
	}
	connection.Location = HerdrLocation{Session: binding.Session, WorkspaceID: binding.WorkspaceID, TabID: binding.TabID, PaneID: binding.PaneID, AgentName: binding.AgentName}
	return m.finishConnection(connection, entry.observedAt), ""
}

func (m *HerdrMonitor) finishConnection(connection HerdrConnection, observedAt time.Time) HerdrConnection {
	if !observedAt.IsZero() {
		connection.ObservedAt = timePtr(observedAt)
	}
	switch {
	case connection.Status == "connected" && connection.AgentStatus == "working":
		connection.NextAction = "observe"
	case connection.Status == "connected" && connection.AgentStatus == "blocked":
		connection.NextAction = "inspect_question"
	case connection.Status == "connected" && (connection.AgentStatus == "idle" || connection.AgentStatus == "done"):
		connection.NextAction = "check_github"
	case connection.Status == "missing":
		connection.NextAction = "check_handoff"
	default:
		connection.NextAction = "recheck"
	}
	connection.Handoff = handoffForHerdr(connection)
	return connection
}

func handoffForHerdr(connection HerdrConnection) string {
	location := []string{}
	if connection.Location.Session != "" {
		location = append(location, "세션 "+connection.Location.Session)
	}
	if connection.Location.WorkspaceID != "" {
		location = append(location, "workspace "+connection.Location.WorkspaceID)
	}
	if connection.Location.TabID != "" {
		location = append(location, "tab "+connection.Location.TabID)
	}
	if connection.Location.PaneID != "" {
		location = append(location, "pane "+connection.Location.PaneID)
	}
	if connection.Location.CWD != "" {
		location = append(location, "cwd "+connection.Location.CWD)
	}
	where := strings.Join(location, " · ")
	if where == "" {
		where = "지정된 Herdr 위치"
	}
	observed := "성공한 관찰 시각 없음"
	if connection.ObservedAt != nil {
		observed = "관찰 시각 " + connection.ObservedAt.Format(time.RFC3339Nano)
	}
	state := "연결 상태 " + connection.Status
	if connection.AgentStatus != "" {
		state = "관찰 상태 " + connection.AgentStatus
	}
	return fmt.Sprintf("%s · %s · %s · 다음 행동: %s", where, observed, state, herdrGuidance(connection))
}

func herdrGuidance(connection HerdrConnection) string {
	switch connection.NextAction {
	case "observe":
		return "해당 세션으로 돌아가 관찰합니다."
	case "inspect_question":
		return "세션의 질문과 필요한 승인을 먼저 확인합니다."
	case "check_github":
		return "최신 GitHub 기록과 남은 일을 확인합니다."
	case "check_handoff":
		return "보존된 Git 변경과 handoff를 확인합니다."
	default:
		return "세션 위치와 관찰을 먼저 재확인합니다."
	}
}

func (m *HerdrMonitor) statusSnapshot(now time.Time, status, syncStatus, state, next string, notices []string) HerdrSnapshot {
	return m.snapshot(now, HerdrSnapshot{Status: status, SyncStatus: syncStatus, State: state, NextAction: next, Freshness: Freshness{State: "stale", SyncStatus: syncStatus, ObservedAt: timePtr(now)}, Notices: notices, Sessions: []HerdrSession{}, Connections: []HerdrConnection{}, UnconnectedAgents: []HerdrObservedAgent{}})
}

func (m *HerdrMonitor) snapshot(now time.Time, snapshot HerdrSnapshot) HerdrSnapshot {
	m.mu.Lock()
	m.revision++
	snapshot.Revision = m.revision
	m.mu.Unlock()
	snapshot.Source = "herdr"
	snapshot.SchemaVersion = 1
	snapshot.ObservedAt = now
	if snapshot.Notices == nil {
		snapshot.Notices = []string{}
	}
	if snapshot.Sessions == nil {
		snapshot.Sessions = []HerdrSession{}
	}
	if snapshot.Connections == nil {
		snapshot.Connections = []HerdrConnection{}
	}
	if snapshot.UnconnectedAgents == nil {
		snapshot.UnconnectedAgents = []HerdrObservedAgent{}
	}
	if snapshot.Links == nil {
		snapshot.Links = []Link{}
	}
	return snapshot
}

func parseHerdrConfig(raw string) (parsedHerdrConfig, error) {
	var value struct {
		Version  int               `json:"version"`
		Bindings []json.RawMessage `json:"bindings"`
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return parsedHerdrConfig{}, errors.New("THREADDOCK_SESSIONS_FILE은 유효한 JSON이어야 합니다.")
	}
	if value.Version != 1 || value.Bindings == nil {
		return parsedHerdrConfig{}, errors.New("THREADDOCK_SESSIONS_FILE은 version=1과 bindings 배열을 포함해야 합니다.")
	}
	config := parsedHerdrConfig{}
	seen := map[string]bool{}
	for _, rawBinding := range value.Bindings {
		var input map[string]any
		if err := json.Unmarshal(rawBinding, &input); err != nil || input == nil {
			return parsedHerdrConfig{}, errors.New("각 Herdr binding은 객체여야 합니다.")
		}
		binding, err := parseHerdrBinding(input)
		if err != nil {
			return parsedHerdrConfig{}, err
		}
		config.bindings = append(config.bindings, binding)
		if !seen[binding.Session] {
			config.sessions = append(config.sessions, binding.Session)
			seen[binding.Session] = true
		}
	}
	return config, nil
}

func parseHerdrBinding(value map[string]any) (herdrBinding, error) {
	binding := herdrBinding{IssueURL: optionalString(value["issueUrl"]), ProjectURL: optionalString(value["projectUrl"]), Repository: optionalRepository(value["repository"]), Worktree: optionalString(value["worktree"]), Session: strings.TrimSpace(optionalString(value["session"])), WorkspaceID: optionalString(value["workspaceId"]), TabID: optionalString(value["tabId"]), PaneID: optionalString(value["paneId"]), AgentName: optionalString(value["agentName"]), Role: optionalString(value["role"])}
	if !herdrSessionPattern.MatchString(binding.Session) || binding.Session == "." || binding.Session == ".." {
		return herdrBinding{}, errors.New("Each binding requires an ASCII session name of 1-64 characters.")
	}
	if binding.IssueURL == "" && binding.ProjectURL == "" && binding.Repository == "" {
		return herdrBinding{}, errors.New("Each Herdr binding requires issueUrl, projectUrl, or repository.")
	}
	if value["repository"] != nil && binding.Repository == "" {
		return herdrBinding{}, errors.New("repository must look like github.com/OWNER/REPO.")
	}
	if binding.IssueURL != "" && !isGitHubURL(binding.IssueURL) {
		return herdrBinding{}, errors.New("issueUrl must be a complete GitHub URL.")
	}
	if binding.ProjectURL != "" && !isGitHubURL(binding.ProjectURL) {
		return herdrBinding{}, errors.New("projectUrl must be a complete GitHub URL.")
	}
	if issueRepo := issueRepositoryKey(binding.IssueURL); issueRepo != "" && binding.Repository != "" && issueRepo != binding.Repository {
		binding.Conflict = "issueUrl and repository refer to different repositories."
	}
	return binding, nil
}

func normalizeHerdrAgents(raw string) ([]HerdrAgent, error) {
	var payload struct {
		Result struct {
			Type   string            `json:"type"`
			Agents []json.RawMessage `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload.Result.Type != "agent_list" || payload.Result.Agents == nil {
		return nil, errors.New("Unexpected Herdr agent response")
	}
	agents := make([]HerdrAgent, 0, len(payload.Result.Agents))
	for _, rawAgent := range payload.Result.Agents {
		var row map[string]any
		if len(rawAgent) == 0 || string(rawAgent) == "null" || json.Unmarshal(rawAgent, &row) != nil || row == nil {
			return nil, errors.New("Unexpected Herdr agent response")
		}
		name, err := requiredHerdrAgentField(row, "name")
		if err != nil {
			return nil, err
		}
		status, err := requiredHerdrAgentField(row, "agent_status")
		if err != nil {
			return nil, err
		}
		workspace, err := requiredHerdrAgentField(row, "workspace_id")
		if err != nil {
			return nil, err
		}
		tab, err := requiredHerdrAgentField(row, "tab_id")
		if err != nil {
			return nil, err
		}
		pane, err := requiredHerdrAgentField(row, "pane_id")
		if err != nil {
			return nil, err
		}
		cwd, err := requiredHerdrAgentField(row, "cwd")
		if err != nil {
			return nil, err
		}
		foregroundCWD, err := requiredHerdrAgentField(row, "foreground_cwd")
		if err != nil {
			return nil, err
		}
		agents = append(agents, HerdrAgent{Name: name, AgentStatus: status, WorkspaceID: workspace, TabID: tab, PaneID: pane, CWD: cwd, ForegroundCWD: foregroundCWD})
	}
	return agents, nil
}

func requiredHerdrAgentField(row map[string]any, name string) (string, error) {
	value, present := row[name]
	if !present || value == nil {
		return "", nil
	}
	textValue, ok := value.(string)
	if !ok {
		return "", errors.New("Unexpected Herdr agent response")
	}
	return strings.TrimSpace(textValue), nil
}

func (m *HerdrMonitor) runHerdr(ctx context.Context, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--distribution", m.distribution, "--exec", "/bin/sh", "-c", herdrShell, herdrCommandTag}, args...)
	return m.runWSLArgs(ctx, commandArgs...)
}

func (m *HerdrMonitor) runWSL(ctx context.Context, executable string, args ...string) ([]byte, error) {
	return m.runWSLArgs(ctx, append([]string{"--distribution", m.distribution, "--exec", executable}, args...)...)
}

func (m *HerdrMonitor) runWSLArgs(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("herdr command timed out: %w", err)
	}
	commandCtx := ctx
	cancel := func() {}
	if m.timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, m.timeout)
	}
	defer cancel()
	result, err := m.process.Run(commandCtx, "", "wsl.exe", args...)
	if commandCtx.Err() != nil {
		return nil, fmt.Errorf("herdr command timed out: %w", commandCtx.Err())
	}
	if err != nil || result.ExitCode != 0 {
		status := result.ExitCode
		if status == 0 {
			status = -1
		}
		return nil, fmt.Errorf("herdr command failed with exit status %d", status)
	}
	if len(result.Stdout) > maxHerdrOutput {
		return nil, errors.New("herdr command output limit exceeded")
	}
	return []byte(result.Stdout), nil
}

func (m *HerdrMonitor) defaultRealpath(value string) (string, error) {
	output, err := m.runWSL(context.Background(), "/usr/bin/realpath", value)
	if err != nil {
		return "", err
	}
	canonical := strings.TrimSpace(string(output))
	if canonical == "" || !strings.HasPrefix(canonical, "/") {
		return "", errors.New("invalid canonical path")
	}
	return canonical, nil
}

func completeBindingIdentity(binding herdrBinding) bool {
	return binding.WorkspaceID != "" && binding.TabID != "" && binding.PaneID != "" && binding.AgentName != ""
}
func (m *HerdrMonitor) canonical(pathValue string) (string, error) {
	if pathValue == "" {
		return "", errors.New("empty path")
	}
	if m.realpath != nil {
		return m.realpath(pathValue)
	}
	return m.defaultRealpath(pathValue)
}
func pathWithin(parent, child string) bool {
	parent, child = path.Clean(parent), path.Clean(child)
	if parent == "/" {
		return strings.HasPrefix(child, "/")
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}
func agentKey(session string, agent HerdrAgent) string {
	return strings.Join([]string{session, agent.WorkspaceID, agent.TabID, agent.PaneID, agent.Name}, "\x00")
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func optionalString(value any) string {
	valueString, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(valueString)
}
func optionalRepository(value any) string {
	candidate := strings.TrimSpace(optionalString(value))
	candidate = strings.TrimPrefix(strings.TrimPrefix(candidate, "https://"), "http://")
	candidate = strings.TrimPrefix(candidate, "github.com/")
	candidate = strings.Trim(candidate, "/")
	if strings.HasSuffix(candidate, ".git") {
		candidate = strings.TrimSuffix(candidate, ".git")
	}
	parts := strings.Split(candidate, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return "github.com/" + parts[0] + "/" + parts[1]
}
func isGitHubURL(value string) bool {
	return strings.HasPrefix(value, "https://github.com/") && !strings.Contains(value, " ")
}
func issueRepositoryKey(value string) string {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(value, "https://github.com/"), "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return "github.com/" + parts[0] + "/" + parts[1]
}
func cloneStringMap(value map[string]string) map[string]string {
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}
func stateToFreshness(state string) string {
	if state == "running" {
		return "fresh"
	}
	return "stale"
}
func chooseValue[T any](condition bool, yes, no T) T {
	if condition {
		return yes
	}
	return no
}

type combinedMonitor struct {
	github   SnapshotSource
	herdr    HerdrSnapshotSource
	mu       sync.Mutex
	inFlight *combinedCall
}

type combinedCall struct {
	done     chan struct{}
	snapshot Snapshot
	err      error
}

func NewCombinedMonitor(github SnapshotSource, herdr HerdrSnapshotSource) SnapshotSource {
	return &combinedMonitor{github: github, herdr: herdr}
}

func (m *combinedMonitor) FetchAll(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	if m.inFlight == nil {
		call := &combinedCall{done: make(chan struct{})}
		m.inFlight = call
		go m.completeCombined(call)
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

func (m *combinedMonitor) completeCombined(call *combinedCall) {
	call.snapshot, call.err = m.loadCombined(context.Background())
	m.mu.Lock()
	close(call.done)
	if m.inFlight == call {
		m.inFlight = nil
	}
	m.mu.Unlock()
}

func (m *combinedMonitor) loadCombined(ctx context.Context) (Snapshot, error) {
	type githubResult struct {
		snapshot Snapshot
		err      error
	}
	type herdrResult struct {
		snapshot HerdrSnapshot
		err      error
	}
	githubDone := make(chan githubResult, 1)
	herdrDone := make(chan herdrResult, 1)
	go func() {
		if m.github == nil {
			githubDone <- githubResult{err: errors.New("github snapshot source is nil")}
			return
		}
		snapshot, err := m.github.FetchAll(ctx)
		githubDone <- githubResult{snapshot: snapshot, err: err}
	}()
	go func() {
		if m.herdr == nil {
			herdrDone <- herdrResult{err: errors.New("herdr snapshot source is nil")}
			return
		}
		snapshot, err := m.herdr.FetchHerdr(ctx)
		herdrDone <- herdrResult{snapshot: snapshot, err: err}
	}()
	githubValue, herdrValue := <-githubDone, <-herdrDone
	now := time.Now()
	if githubValue.err != nil {
		githubValue.snapshot = Snapshot{SchemaVersion: 2, Revision: 0, ObservedAt: now, Freshness: Freshness{State: "stale", SyncStatus: "offline", ObservedAt: timePtr(now)}, State: "stale", SyncStatus: "offline", NextAction: "recheck", Projects: []Project{}, Notices: []string{"GitHub Monitor를 불러오지 못했습니다. 마지막 성공 기록을 확인하세요."}, Source: "github"}
	}
	if herdrValue.err != nil {
		herdrValue.snapshot = HerdrSnapshot{Source: "herdr", SchemaVersion: 1, ObservedAt: now, Status: "offline", State: "unknown", SyncStatus: "offline", NextAction: "recheck", Freshness: Freshness{State: "stale", SyncStatus: "offline", ObservedAt: timePtr(now)}, Notices: []string{"Herdr Monitor를 불러오지 못했습니다. 세션과 Herdr 연결을 다시 확인하세요."}, Sessions: []HerdrSession{}, Connections: []HerdrConnection{}, UnconnectedAgents: []HerdrObservedAgent{}, Links: []Link{}}
	}
	githubValue.snapshot.Herdr = &herdrValue.snapshot
	return githubValue.snapshot, nil
}

var _ HerdrSnapshotSource = (*HerdrMonitor)(nil)
