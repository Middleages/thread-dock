package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const githubTTL = 60 * time.Second

var (
	repoPattern    = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)$`)
	projectPattern = regexp.MustCompile(`^https://github\.com/(users|orgs)/([A-Za-z0-9_.-]+)/projects/([0-9]+)/?$`)
	bodyURLPattern = regexp.MustCompile(`https?://[^\s)\]}>,]+`)
)

type repositoryTarget struct{ owner, repo string }
type projectTarget struct{ owner, number, url string }
type monitorConfig struct {
	repos    []repositoryTarget
	projects []projectTarget
	err      error
}

type cachedSource struct {
	data          any
	observedAt    time.Time
	nextAttemptAt time.Time
	stale         bool
	hasData       bool
}

type sourceRead struct {
	data       any
	observedAt time.Time
	stale      bool
	hasData    bool
}

type aggregateCall struct {
	done     chan struct{}
	snapshot Snapshot
	err      error
}

type GitHubMonitor struct {
	config       monitorConfig
	sessionsFile string
	command      *wslCommandRunner
	timeout      time.Duration
	now          func() time.Time

	mu       sync.Mutex
	cache    map[string]cachedSource
	revision uint64
	inFlight *aggregateCall
}

func NewGitHubMonitor(env map[string]string, process CommandRunner, timeout time.Duration) *GitHubMonitor {
	config, sessions := parseMonitorConfig(env)
	return &GitHubMonitor{
		config: config, sessionsFile: sessions,
		command: newWSLCommandRunner(process, env["THREADDOCK_WSL_DISTRIBUTION"], timeout),
		timeout: timeout, now: time.Now, cache: make(map[string]cachedSource),
	}
}

func parseMonitorConfig(env map[string]string) (monitorConfig, string) {
	config := monitorConfig{}
	config.repos, config.err = parseRepos(env["THREADDOCK_REPOS"])
	if config.err == nil {
		config.projects, config.err = parseProjects(env["THREADDOCK_PROJECTS"])
	}
	known := map[string]bool{"THREADDOCK_REPOS": true, "THREADDOCK_PROJECTS": true, "THREADDOCK_WSL_DISTRIBUTION": true, "THREADDOCK_SESSIONS_FILE": true}
	var unknown []string
	for key := range env {
		if strings.HasPrefix(key, "THREADDOCK_") && !known[key] {
			unknown = append(unknown, key)
		}
	}
	if config.err == nil && len(unknown) > 0 {
		sort.Strings(unknown)
		config.err = fmt.Errorf("unknown ThreadDock monitor configuration: %s", strings.Join(unknown, ", "))
	}
	if config.err == nil && (len(config.repos) > 0 || len(config.projects) > 0) && strings.TrimSpace(env["THREADDOCK_WSL_DISTRIBUTION"]) == "" {
		config.err = errors.New("THREADDOCK_WSL_DISTRIBUTION must be set when GitHub targets are configured")
	}
	return config, strings.TrimSpace(env["THREADDOCK_SESSIONS_FILE"])
}

func parseRepos(raw string) ([]repositoryTarget, error) {
	var result []repositoryTarget
	for _, item := range splitConfig(raw) {
		match := repoPattern.FindStringSubmatch(item)
		if match == nil {
			return nil, fmt.Errorf("THREADDOCK_REPOS must contain OWNER/REPO entries: %s", item)
		}
		result = append(result, repositoryTarget{owner: match[1], repo: match[2]})
	}
	return result, nil
}

func parseProjects(raw string) ([]projectTarget, error) {
	var result []projectTarget
	for _, item := range splitConfig(raw) {
		match := projectPattern.FindStringSubmatch(item)
		if match == nil {
			return nil, errors.New("THREADDOCK_PROJECTS must contain GitHub user or org project URLs")
		}
		result = append(result, projectTarget{owner: match[2], number: match[3], url: item})
	}
	return result, nil
}

func splitConfig(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func (m *GitHubMonitor) FetchAll(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	if m.inFlight == nil {
		call := &aggregateCall{done: make(chan struct{})}
		m.inFlight = call
		go m.completeAggregate(call)
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

func (m *GitHubMonitor) completeAggregate(call *aggregateCall) {
	call.snapshot, call.err = m.load()
	m.mu.Lock()
	if m.inFlight == call {
		m.inFlight = nil
	}
	m.mu.Unlock()
	close(call.done)
}

func (m *GitHubMonitor) load() (Snapshot, error) {
	now := m.now()
	if m.config.err != nil {
		return m.setupSnapshot(now, m.config.err.Error()), nil
	}
	if len(m.config.repos) == 0 && len(m.config.projects) == 0 {
		return m.setupSnapshot(now, "GitHub Monitor가 설정되지 않았습니다. 저장소, Project URL, WSL 배포판을 설정하세요."), nil
	}
	notices := []string{}
	projects := make([]Project, 0, len(m.config.repos)+len(m.config.projects))
	stale := false
	var lastSynced *time.Time
	for _, repo := range m.config.repos {
		project := m.fetchRepository(repo, &notices)
		projects = append(projects, project)
		if project.State == "stale" {
			stale = true
		}
		lastSynced = laterTime(lastSynced, project.UpdatedAt)
	}
	for _, board := range m.config.projects {
		project := m.fetchProject(board, &notices)
		projects = append(projects, project)
		if project.State == "stale" {
			stale = true
		}
		lastSynced = laterTime(lastSynced, project.UpdatedAt)
	}
	m.mu.Lock()
	m.revision++
	revision := m.revision
	m.mu.Unlock()
	observed := now
	for i := range projects {
		notices = append(notices, projects[i].Notices...)
	}
	return Snapshot{SchemaVersion: 2, Revision: revision, ObservedAt: observed, Freshness: Freshness{State: choose(stale, "stale", "fresh"), SyncStatus: choose(stale, "degraded", "synced"), ObservedAt: timePtr(observed), LastSyncedAt: lastSynced}, State: choose(stale, "stale", "running"), SyncStatus: choose(stale, "degraded", "synced"), NextAction: "review", EvidenceRefs: projectEvidence(projects), Projects: projects, Source: "github", Notices: notices}, nil
}

func (m *GitHubMonitor) setupSnapshot(now time.Time, reason string) Snapshot {
	m.mu.Lock()
	m.revision++
	revision := m.revision
	m.mu.Unlock()
	notices := []string{reason, "Windows Wails Monitor 설정에서 저장소·Projects URL·WSL 배포판을 확인하세요."}
	return Snapshot{SchemaVersion: 2, Revision: revision, ObservedAt: now, Freshness: Freshness{State: "stale", SyncStatus: "setup_required", ObservedAt: timePtr(now)}, State: "needs_operator", SyncStatus: "setup_required", NextAction: "setup", EvidenceRefs: []string{}, Projects: []Project{}, Source: "github", Notices: notices}
}

func (m *GitHubMonitor) fetchRepository(repo repositoryTarget, notices *[]string) Project {
	key := repo.owner + "/" + repo.repo
	issueArgs := []string{"gh", "issue", "list", "--repo", key, "--state", "all", "--limit", "100", "--json", "number,title,url,state,body,updatedAt,author"}
	prArgs := []string{"gh", "pr", "list", "--repo", key, "--state", "all", "--limit", "100", "--json", "number,title,url,state,body,updatedAt,reviewDecision,statusCheckRollup,closingIssuesReferences"}
	issues := m.readSource(key+" issues", issueArgs, decodeIssueRows, notices)
	prs := m.readSource(key+" prs", prArgs, decodePullRequestRows, notices)
	issueRows, _ := issues.data.([]rawIssue)
	prRows, _ := prs.data.([]rawPullRequest)
	prsByIssue := map[string][]string{}
	for _, pr := range prRows {
		prURL := safeGitHubURL(pr.URL)
		for _, ref := range pr.ClosingIssuesReferences {
			if issueURL := safeGitHubURL(ref.URL); issueURL != "" && prURL != "" {
				prsByIssue[issueURL] = append(prsByIssue[issueURL], prURL)
			}
		}
	}
	items := make([]WorkItem, 0, len(issueRows)+len(prRows))
	for _, issue := range issueRows {
		items = append(items, mapIssue(key, issue, issues, prsByIssue[safeGitHubURL(issue.URL)]))
	}
	for _, pr := range prRows {
		items = append(items, mapPullRequest(key, pr, prs))
	}
	updated := laterTime(issues.observedAtPtr(), prs.observedAtPtr())
	isStale := issues.stale || prs.stale
	if len(issueRows) >= 100 {
		*notices = append(*notices, key+" 이슈 조회 결과가 최대 100개로 제한됐습니다.")
	}
	if len(prRows) >= 100 {
		*notices = append(*notices, key+" PR 조회 결과가 최대 100개로 제한됐습니다.")
	}
	projectNotices := []string{}
	if isStale {
		projectNotices = append(projectNotices, key+"의 일부 GitHub 조회가 오래된 상태입니다.")
	}
	return Project{ProjectID: "repo:" + key, Name: key, State: choose(isStale, "stale", "running"), SyncStatus: choose(isStale, "degraded", "synced"), NextAction: "review", EvidenceRefs: []string{"https://github.com/" + key}, UpdatedAt: updated, ObservedAt: updated, WorkItems: items, Source: "github", Notices: projectNotices, Links: repositoryLinks(repo.owner, repo.repo)}
}

func (m *GitHubMonitor) fetchProject(board projectTarget, notices *[]string) Project {
	key := "board:" + board.owner + "/" + board.number
	args := []string{"gh", "project", "item-list", board.number, "--owner", board.owner, "--format", "json", "--limit", "100"}
	result := m.readSource(key, args, func(data []byte) (any, error) {
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		items, ok := value["items"].([]any)
		if !ok {
			return nil, errors.New("invalid project items")
		}
		if err := validateProjectItems(items); err != nil {
			return nil, err
		}
		return projectPayload{totalCount: numberValue(value["totalCount"]), items: items}, nil
	}, notices)
	payload, _ := result.data.(projectPayload)
	if payload.totalCount > 100 || len(payload.items) >= 100 {
		*notices = append(*notices, key+" 조회 결과가 최대 100개로 제한됐습니다.")
	}
	inaccessible := 0
	items := make([]WorkItem, 0, len(payload.items))
	for index, item := range payload.items {
		if itemMap, ok := item.(map[string]any); !ok || itemMap["content"] == nil {
			inaccessible++
		}
		items = append(items, mapProjectItem(item, board, index, result))
	}
	if inaccessible > 0 {
		*notices = append(*notices, fmt.Sprintf("%s에서 %d개 항목의 내용을 확인할 수 없습니다.", key, inaccessible))
	}
	projectNotices := []string{}
	if result.stale {
		projectNotices = append(projectNotices, key+" 조회가 오래된 상태입니다.")
	}
	updated := result.observedAtPtr()
	return Project{ProjectID: key, Name: "GitHub Project " + board.owner + "/" + board.number, State: choose(result.stale, "stale", "running"), SyncStatus: choose(result.stale, "degraded", "synced"), NextAction: "review", EvidenceRefs: []string{board.url}, UpdatedAt: updated, ObservedAt: updated, WorkItems: items, Source: "github", Notices: projectNotices, Links: []Link{{Kind: "github", Label: "Project", URL: board.url}}}
}

func (m *GitHubMonitor) readSource(key string, args []string, decode func([]byte) (any, error), notices *[]string) sourceRead {
	now := m.now()
	m.mu.Lock()
	cached, exists := m.cache[key]
	m.mu.Unlock()
	if exists && now.Before(cached.nextAttemptAt) {
		return sourceRead{data: cached.data, observedAt: cached.observedAt, stale: cached.stale, hasData: cached.hasData}
	}
	if exists && cached.hasData && now.Sub(cached.observedAt) < githubTTL {
		return sourceRead{data: cached.data, observedAt: cached.observedAt, hasData: true}
	}
	output, err := m.command.run(context.Background(), args[0], args[1:]...)
	var data any
	if err == nil {
		data, err = decode(output)
	}
	if err == nil {
		entry := cachedSource{data: data, observedAt: now, nextAttemptAt: now.Add(githubTTL), hasData: true}
		m.mu.Lock()
		m.cache[key] = entry
		m.mu.Unlock()
		return sourceRead{data: data, observedAt: now, hasData: true}
	}
	*notices = append(*notices, key+" 조회에 실패했습니다. 마지막 성공 시각이 있으면 보존된 데이터를 표시합니다.")
	cached.nextAttemptAt = now.Add(githubTTL)
	cached.stale = true
	m.mu.Lock()
	m.cache[key] = cached
	m.mu.Unlock()
	return sourceRead{data: cached.data, observedAt: cached.observedAt, stale: true, hasData: cached.hasData}
}

type rawIssue struct {
	Number                             *int `json:"number"`
	Title, URL, State, Body, UpdatedAt string
	Author                             rawAuthor `json:"author"`
}
type rawAuthor struct {
	Login string `json:"login"`
}
type rawPullRequest struct {
	Number                                             *int `json:"number"`
	Title, URL, State, Body, UpdatedAt, ReviewDecision string
	StatusCheckRollup                                  []map[string]any `json:"statusCheckRollup"`
	ClosingIssuesReferences                            []rawURL         `json:"closingIssuesReferences"`
}
type rawURL struct {
	URL string `json:"url"`
}
type projectPayload struct {
	totalCount int
	items      []any
}

func decodeIssueRows(data []byte) (any, error) {
	if err := requireJSONArray(data); err != nil {
		return nil, err
	}
	var rows []rawIssue
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	var shapes []map[string]json.RawMessage
	if err := json.Unmarshal(data, &shapes); err != nil {
		return nil, err
	}
	if len(rows) != len(shapes) {
		return nil, errors.New("invalid issue shape")
	}
	for _, shape := range shapes {
		if !requiredJSONNumber(shape, "number") || !requiredJSONString(shape, "title") || !requiredJSONString(shape, "url") || !requiredJSONString(shape, "state") {
			return nil, errors.New("invalid issue shape")
		}
		if err := optionalJSONText(shape, "body"); err != nil {
			return nil, err
		}
		if err := optionalJSONObject(shape, "author"); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func decodePullRequestRows(data []byte) (any, error) {
	if err := requireJSONArray(data); err != nil {
		return nil, err
	}
	var rows []rawPullRequest
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	var shapes []map[string]json.RawMessage
	if err := json.Unmarshal(data, &shapes); err != nil {
		return nil, err
	}
	if len(rows) != len(shapes) {
		return nil, errors.New("invalid pull request shape")
	}
	for _, shape := range shapes {
		if !requiredJSONNumber(shape, "number") || !requiredJSONString(shape, "title") || !requiredJSONString(shape, "url") || !requiredJSONString(shape, "state") {
			return nil, errors.New("invalid pull request shape")
		}
		if err := optionalJSONText(shape, "body"); err != nil {
			return nil, err
		}
		if err := optionalJSONArrayObjects(shape, "statusCheckRollup"); err != nil {
			return nil, err
		}
		if err := optionalJSONArrayObjects(shape, "closingIssuesReferences"); err != nil {
			return nil, err
		}
		if raw, ok := shape["closingIssuesReferences"]; ok && string(raw) != "null" {
			var references []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &references); err != nil {
				return nil, err
			}
			for _, reference := range references {
				if !requiredJSONString(reference, "url") {
					return nil, errors.New("invalid closing issue shape")
				}
			}
		}
	}
	return rows, nil
}

func requireJSONArray(data []byte) error {
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil || rows == nil {
		return errors.New("invalid top-level array shape")
	}
	return nil
}

func requiredJSONNumber(object map[string]json.RawMessage, key string) bool {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return false
	}
	var number int
	return json.Unmarshal(raw, &number) == nil
}

func requiredJSONString(object map[string]json.RawMessage, key string) bool {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != ""
}

func optionalJSONText(object map[string]json.RawMessage, key string) error {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return errors.New("invalid text field shape")
	}
	return nil
}

func optionalJSONObject(object map[string]json.RawMessage, key string) error {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return errors.New("invalid object field shape")
	}
	return nil
}

func optionalJSONArrayObjects(object map[string]json.RawMessage, key string) error {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	var values []map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return errors.New("invalid array field shape")
	}
	for _, value := range values {
		if value == nil {
			return errors.New("invalid array item shape")
		}
	}
	return nil
}

func validateProjectItems(items []any) error {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return errors.New("invalid project item shape")
		}
		if content, exists := item["content"]; exists && content != nil {
			if _, ok := content.(map[string]any); !ok {
				return errors.New("invalid project content shape")
			}
		}
		values, exists := item["fieldValues"]
		if !exists || values == nil {
			continue
		}
		rows, ok := values.([]any)
		if !ok {
			return errors.New("invalid project field values shape")
		}
		for _, value := range rows {
			field, ok := value.(map[string]any)
			if !ok {
				return errors.New("invalid project field shape")
			}
			if nested, exists := field["field"]; exists {
				if nested == nil {
					return errors.New("invalid project field definition")
				}
				if _, ok := nested.(map[string]any); !ok {
					return errors.New("invalid project field definition")
				}
			}
		}
	}
	return nil
}

func mapIssue(repo string, issue rawIssue, source sourceRead, related []string) WorkItem {
	url := safeGitHubURL(issue.URL)
	github := &GitHubWork{Kind: "issue", Number: issue.Number, URL: url, State: issue.State, RelatedIssueURLs: []string{}, RelatedPullRequestURLs: uniqueStrings(related), Checks: []GitHubCheck{}, Fields: map[string]string{}, ContentAvailable: true, Author: issue.Author.Login, ObservedAt: source.observedAtPtr(), Stale: source.stale}
	return baseWork("issue", urlOrFallback(url, repo+"#"+numberText(issue.Number)), issue.Title, issue.Body, issue.UpdatedAt, github, source)
}

func mapPullRequest(repo string, pr rawPullRequest, source sourceRead) WorkItem {
	url := safeGitHubURL(pr.URL)
	related := []string{}
	for _, ref := range pr.ClosingIssuesReferences {
		if value := safeGitHubURL(ref.URL); value != "" {
			related = append(related, value)
		}
	}
	github := &GitHubWork{Kind: "pull_request", Number: pr.Number, URL: url, State: pr.State, ReviewDecision: pr.ReviewDecision, RelatedIssueURLs: uniqueStrings(related), RelatedPullRequestURLs: []string{}, Checks: mapChecks(pr.StatusCheckRollup), Fields: map[string]string{}, ContentAvailable: true, ObservedAt: source.observedAtPtr(), Stale: source.stale}
	return baseWork("pr", urlOrFallback(url, repo+"#"+numberText(pr.Number)), pr.Title, pr.Body, pr.UpdatedAt, github, source)
}

func mapProjectItem(value any, board projectTarget, index int, source sourceRead) WorkItem {
	item, _ := value.(map[string]any)
	content, _ := item["content"].(map[string]any)
	url := safeGitHubURL(textValue(content["url"]))
	number := intPointer(content["number"])
	identity := urlOrFallback(url, textValue(item["id"]))
	if identity == "" {
		identity = strconv.Itoa(index)
	}
	title := textValue(content["title"])
	if title == "" {
		title = textValue(content["name"])
	}
	if title == "" {
		title = "GitHub project item (content unavailable)"
	}
	state := textValue(content["state"])
	github := &GitHubWork{Kind: "project_item", Number: number, URL: url, State: state, RelatedIssueURLs: []string{}, RelatedPullRequestURLs: []string{}, Checks: []GitHubCheck{}, Fields: projectFields(item), ContentAvailable: content != nil, ObservedAt: source.observedAtPtr(), Stale: source.stale}
	return baseWork("project-item", board.owner+"/"+board.number+":"+identity, title, textValue(content["body"]), textValue(content["updatedAt"]), github, source)
}

func baseWork(kind, identity, title, body, updated string, github *GitHubWork, source sourceRead) WorkItem {
	if title == "" {
		title = identity
	}
	links := []Link{}
	if github.URL != "" {
		links = append(links, Link{Kind: kind, Label: map[string]string{"pr": "Pull request", "issue": "Issue", "project-item": "GitHub"}[kind], URL: github.URL})
	}
	for _, bodyURL := range bodyURLs(body) {
		if bodyURL != github.URL {
			links = append(links, Link{Kind: "evidence", Label: "본문 링크", URL: bodyURL})
		}
	}
	return WorkItem{WorkID: kind + ":" + identity, Title: title, Request: body, State: stateFor(github.State), SyncStatus: choose(source.stale, "stale", "synced"), NextAction: "review", EvidenceRefs: nonNilStrings(github.URL), UpdatedAt: parseTime(updated), Decisions: []Decision{}, Handoffs: []Handoff{}, Links: links, GitHub: github}
}

func projectFields(item map[string]any) map[string]string {
	fields := map[string]string{}
	if values, ok := item["fieldValues"].([]any); ok {
		for _, raw := range values {
			if field, ok := raw.(map[string]any); ok {
				name := textValue(fieldPath(field, "field", "name"))
				if name == "" {
					name = textValue(field["name"])
				}
				if name != "" {
					fields[name] = projectFieldValue(field)
				}
			}
		}
	}
	structural := map[string]bool{"id": true, "content": true, "fieldValues": true, "totalCount": true, "type": true, "number": true, "title": true, "body": true, "url": true, "repository": true, "state": true, "updatedAt": true}
	for name, value := range item {
		if !structural[name] {
			if rendered := projectFieldValue(value); rendered != "" {
				fields[name] = rendered
			}
		}
	}
	return fields
}

func projectFieldValue(value any) string {
	if value == nil {
		return ""
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"name", "value", "text", "number", "date"} {
			if text := textValue(object[key]); text != "" {
				return text
			}
		}
		if users, ok := object["users"].([]any); ok {
			return joinObjectValues(users, "login", "name")
		}
		if labels, ok := object["labels"].([]any); ok {
			return joinObjectValues(labels, "name")
		}
		return ""
	}
	if values, ok := value.([]any); ok {
		parts := make([]string, 0, len(values))
		for _, item := range values {
			if text := projectFieldValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	}
	return textValue(value)
}

func joinObjectValues(values []any, keys ...string) string {
	parts := []string{}
	for _, value := range values {
		object, _ := value.(map[string]any)
		for _, key := range keys {
			if text := textValue(object[key]); text != "" {
				parts = append(parts, text)
				break
			}
		}
	}
	return strings.Join(parts, ", ")
}
func fieldPath(object map[string]any, path ...string) any {
	var value any = object
	for _, key := range path {
		current, ok := value.(map[string]any)
		if !ok || current == nil {
			return nil
		}
		next, ok := current[key]
		if !ok {
			return nil
		}
		value = next
	}
	return value
}
func mapChecks(values []map[string]any) []GitHubCheck {
	result := []GitHubCheck{}
	for _, value := range values {
		name := firstText(value, "name", "context", "workflowName")
		if name == "" {
			name = "check"
		}
		result = append(result, GitHubCheck{Name: name, Status: firstText(value, "status", "state"), Conclusion: firstText(value, "conclusion", "state"), URL: safeGitHubURL(firstText(value, "detailsUrl", "targetUrl"))})
	}
	return result
}
func repositoryLinks(owner, repo string) []Link {
	base := "https://github.com/" + owner + "/" + repo
	return []Link{{Kind: "github", Label: "Issues", URL: base + "/issues"}, {Kind: "github", Label: "PRs", URL: base + "/pulls"}, {Kind: "github", Label: "Wiki", URL: base + "/wiki"}}
}
func bodyURLs(value string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, match := range bodyURLPattern.FindAllString(value, -1) {
		match = strings.TrimRight(match, ".,;:!?")
		if safe := safeGitHubURL(match); safe != "" && !seen[safe] {
			seen[safe] = true
			result = append(result, safe)
		}
	}
	return result
}
func safeGitHubURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || !strings.HasPrefix(parsed.Path, "/") {
		return ""
	}
	return value
}
func stateFor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}
func textValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case json.Number:
		return value.String()
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}
func firstText(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := textValue(object[key]); value != "" {
			return value
		}
	}
	return ""
}
func numberValue(value any) int {
	text := textValue(value)
	result, _ := strconv.Atoi(text)
	return result
}
func intPointer(value any) *int {
	result := numberValue(value)
	if result == 0 {
		return nil
	}
	return &result
}
func numberText(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}
func urlOrFallback(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
func nonNilStrings(value string) []string {
	if value == "" {
		return []string{}
	}
	return []string{value}
}
func uniqueStrings(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
func timePtr(value time.Time) *time.Time { return &value }
func (r sourceRead) observedAtPtr() *time.Time {
	if r.observedAt.IsZero() {
		return nil
	}
	return timePtr(r.observedAt)
}
func laterTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil || !b.After(*a) {
		return a
	}
	return b
}
func projectEvidence(projects []Project) []string {
	result := []string{}
	for _, project := range projects {
		result = append(result, project.EvidenceRefs...)
	}
	return result
}
func choose(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}
