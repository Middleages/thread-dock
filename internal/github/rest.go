package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"thread-dock/internal/contract"
)

const defaultAPIVersion = "2022-11-28"

// RESTClient implements Client against the GHES REST and GraphQL endpoints.
type RESTClient struct {
	baseURL      string
	restBasePath string
	graphqlPath  string
	publicAPI    bool
	initErr      error
	token        string
	apiVersion   string
	httpClient   *http.Client
}

// NewRESTClient creates a GitHub API client. GitHub.com uses its public API
// paths, while a GHES base URL may be either the host or its /api/v3 REST
// root; both GHES forms are accepted.
func NewRESTClient(baseURL, token, apiVersion string, httpClient *http.Client) *RESTClient {
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/v3")
	restBasePath, graphqlPath := "/api/v3", "/api/graphql"
	publicAPI := false
	var initErr error
	if parsed, err := url.Parse(baseURL); err == nil && isPublicHostname(parsed.Hostname(), "api.github.com") {
		if parsed.Scheme != "https" {
			initErr = errors.New("github public API requires HTTPS")
		} else {
			parsed.Host = canonicalPublicOriginHost(parsed)
			baseURL = parsed.String()
		}
		restBasePath, graphqlPath = "", "/graphql"
		publicAPI = true
	}
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &RESTClient{baseURL: baseURL, restBasePath: restBasePath, graphqlPath: graphqlPath, publicAPI: publicAPI, initErr: initErr, token: token, apiVersion: apiVersion, httpClient: httpClient}
}

func isPublicHostname(hostname, expected string) bool {
	return strings.EqualFold(strings.TrimSuffix(hostname, "."), expected)
}

func canonicalPublicOriginHost(u *url.URL) string {
	if !isPublicHostname(u.Hostname(), "api.github.com") {
		return u.Host
	}
	if port := u.Port(); port != "" && port != "443" {
		return "api.github.com:" + port
	}
	return "api.github.com"
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && canonicalPublicOriginHost(a) == canonicalPublicOriginHost(b)
}

// AuthError indicates missing or invalid GHES credentials.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("github authentication failed (%d): %s", e.StatusCode, e.Message)
}

// NotFoundError indicates that a requested GHES resource does not exist.
type NotFoundError struct {
	StatusCode int
	Message    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("github resource not found (%d): %s", e.StatusCode, e.Message)
}

// ConflictError indicates a request rejected because it conflicts with the
// current GHES resource or validation rules (including HTTP 422).
type ConflictError struct {
	StatusCode int
	Message    string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("github conflict (%d): %s", e.StatusCode, e.Message)
}

// TemporaryError indicates a transient GHES or transport failure. RetryAfter
// is a hint only; retry policy belongs to the orchestrator.
type TemporaryError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e *TemporaryError) Error() string {
	return fmt.Sprintf("temporary github failure (%d): %s", e.StatusCode, e.Message)
}

type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("github API error (%d): %s", e.StatusCode, e.Message)
}

func (c *RESTClient) FindIssueBundle(ctx context.Context, repo Repository, marker string) (IssueBundle, bool, error) {
	bundle, found, _, err := c.findIssueBundle(ctx, repo, marker)
	return bundle, found, err
}

func (c *RESTClient) findIssueBundle(ctx context.Context, repo Repository, marker string) (IssueBundle, bool, map[string]Issue, error) {
	issuePath := fmt.Sprintf("%s/repos/%s/%s/issues", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	path := issuePath + "?state=all&per_page=100"
	var issues []Issue
	seenPages := make(map[string]struct{})
	for page := 0; page < 1000; page++ {
		if _, seen := seenPages[path]; seen {
			return IssueBundle{}, false, nil, errors.New("github issue pagination repeated a page")
		}
		seenPages[path] = struct{}{}
		var pageIssues []Issue
		link, err := c.doJSONWithLink(ctx, http.MethodGet, path, nil, &pageIssues)
		if err != nil {
			return IssueBundle{}, false, nil, err
		}
		issues = append(issues, pageIssues...)
		if page == 999 {
			return IssueBundle{}, false, nil, errors.New("github issue pagination exceeded the safety limit")
		}
		next := nextIssueLink(link)
		if next == "" && len(pageIssues) == 100 {
			u, parseErr := url.Parse(path)
			if parseErr != nil {
				return IssueBundle{}, false, nil, errors.New("github issue pagination is malformed")
			}
			query := u.Query()
			pageNumber, parseErr := strconv.Atoi(query.Get("page"))
			if parseErr != nil || pageNumber < 1 {
				pageNumber = 1
			}
			query.Set("page", strconv.Itoa(pageNumber+1))
			u.RawQuery = query.Encode()
			next = u.RequestURI()
		}
		if next == "" {
			break
		}
		path, err = c.validateIssuePageURL(next, issuePath)
		if err != nil {
			return IssueBundle{}, false, nil, err
		}
	}
	var parent Issue
	children := make(map[string]Issue)
	for _, issue := range issues {
		role, key, ok := parseRoleMarker(issue.Body, marker)
		if !ok {
			continue
		}
		if role == "parent" && parent.Number == 0 {
			parent = issue
		} else if role == "child" {
			children[key] = issue
		}
	}
	if parent.Number == 0 {
		return IssueBundle{}, false, children, nil
	}
	orderedChildren := make([]Issue, 0, len(children))
	for _, issue := range issues {
		role, key, ok := parseRoleMarker(issue.Body, marker)
		if ok && role == "child" && children[key].Number == issue.Number {
			orderedChildren = append(orderedChildren, issue)
		}
	}
	return IssueBundle{Parent: parent, Children: orderedChildren}, true, children, nil
}

func nextIssueLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		segments := strings.Split(part, ";")
		if len(segments) < 2 || !strings.Contains(strings.ToLower(strings.Join(segments[1:], ";")), "rel=\"next\"") {
			continue
		}
		value := strings.TrimSpace(segments[0])
		if len(value) >= 2 && value[0] == '<' && value[len(value)-1] == '>' {
			return value[1 : len(value)-1]
		}
	}
	return ""
}

func (c *RESTClient) validateIssuePageURL(raw, issuePath string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", errors.New("github issue pagination base is malformed")
	}
	next, err := url.Parse(raw)
	if err != nil || next.IsAbs() && !sameOrigin(next, base) || next.User != nil {
		return "", errors.New("github issue pagination link is outside the configured GitHub API base")
	}
	if !next.IsAbs() {
		next = base.ResolveReference(next)
	}
	if !sameOrigin(next, base) || !c.validIssuePagePath(next.Path, issuePath) || next.Fragment != "" {
		return "", errors.New("github issue pagination link is outside the configured GitHub API base")
	}
	if !c.publicAPI {
		if err := validateSensitivePaginationQuery(next.Query(), c.token); err != nil {
			return "", err
		}
		return next.RequestURI(), nil
	}
	query, err := copyPublicPaginationQuery(next.Query(), c.token)
	if err != nil {
		return "", err
	}
	requestURI := issuePath
	if encodedQuery := query.Encode(); encodedQuery != "" {
		requestURI += "?" + encodedQuery
	}
	return requestURI, nil
}

func validateSensitivePaginationQuery(query url.Values, token string) error {
	for key, values := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") {
			return errors.New("github issue pagination link contains sensitive query data")
		}
		for _, value := range values {
			if token != "" && strings.Contains(value, token) {
				return errors.New("github issue pagination link contains sensitive query data")
			}
		}
	}
	return nil
}

func copyPublicPaginationQuery(query url.Values, token string) (url.Values, error) {
	if err := validateSensitivePaginationQuery(query, token); err != nil {
		return nil, err
	}
	filtered := make(url.Values)
	for key, values := range query {
		lower := strings.ToLower(key)
		switch lower {
		case "state", "per_page", "page":
			filtered[lower] = append([]string(nil), values...)
		case "after", "before":
			if len(values) != 1 || strings.TrimSpace(values[0]) == "" || filtered[lower] != nil || (lower == "after" && filtered["before"] != nil) || (lower == "before" && filtered["after"] != nil) {
				return nil, errors.New("github issue pagination link contains an invalid cursor")
			}
			filtered[lower] = append([]string(nil), values...)
		default:
			return nil, errors.New("github issue pagination link contains unsupported query data")
		}
	}
	return filtered, nil
}

func (c *RESTClient) validIssuePagePath(path, issuePath string) bool {
	if path == issuePath {
		return true
	}
	if !c.publicAPI || !strings.HasPrefix(path, "/repositories/") || !strings.HasSuffix(path, "/issues") {
		return false
	}
	repositoryID := strings.TrimSuffix(strings.TrimPrefix(path, "/repositories/"), "/issues")
	if repositoryID == "" {
		return false
	}
	for _, character := range repositoryID {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (c *RESTClient) CreateIssueBundle(ctx context.Context, repo Repository, task contract.TaskContract, marker string) (IssueBundle, error) {
	bundle, found, existingChildren, err := c.findIssueBundle(ctx, repo, marker)
	if err != nil {
		return IssueBundle{}, err
	}
	if !found {
		parent, createErr := c.createIssue(ctx, repo, task.Parent.Title, withRoleMarker(task.Parent.Body, marker, "parent", task.Parent.Key), task.Parent.Labels)
		if createErr != nil {
			return IssueBundle{}, createErr
		}
		bundle.Parent = parent
	}
	// Rebuild children in contract order while reconciling the stable key map.
	bundle.Children = nil
	for _, draft := range task.Children {
		if child, ok := existingChildren[draft.Key]; ok {
			bundle.Children = append(bundle.Children, child)
			continue
		}
		child, createErr := c.createIssue(ctx, repo, draft.Title, withRoleMarker(draft.Body, marker, "child", draft.Key), draft.Labels)
		if createErr != nil {
			return IssueBundle{}, createErr
		}
		bundle.Children = append(bundle.Children, child)
	}
	return bundle, nil
}

func (c *RESTClient) createIssue(ctx context.Context, repo Repository, title, body string, labels []string) (Issue, error) {
	path := fmt.Sprintf("%s/repos/%s/%s/issues", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	payload := struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels,omitempty"`
	}{Title: title, Body: body, Labels: labels}
	var issue Issue
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func (c *RESTClient) CreateDraftPR(ctx context.Context, repo Repository, req DraftPRRequest) (PullRequest, error) {
	path := fmt.Sprintf("%s/repos/%s/%s/pulls", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	payload := struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Draft bool   `json:"draft"`
	}{Title: req.Title, Body: req.Body, Head: req.Head, Base: req.Base, Draft: true}
	var wire pullRequestWire
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &wire); err != nil {
		return PullRequest{}, err
	}
	return wire.toPullRequest(), nil
}

// CreateSafeDraftPR is the narrow draft-PR port used by retryable workflows.
// Unlike legacy CreateDraftPR, all endpoint errors discard provider bodies.
func (c *RESTClient) CreateSafeDraftPR(ctx context.Context, repo Repository, req DraftPRRequest) (PullRequest, error) {
	if err := ValidateSafeDraftPRRequest(repo, req); err != nil {
		return PullRequest{}, err
	}
	path := fmt.Sprintf("%s/repos/%s/%s/pulls", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	payload := struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Draft bool   `json:"draft"`
	}{Title: req.Title, Body: req.Body, Head: req.Head, Base: req.Base, Draft: true}
	var wire pullRequestWire
	if err := c.doSafeJSON(ctx, "create draft pull request", http.MethodPost, path, payload, &wire); err != nil {
		return PullRequest{}, err
	}
	pr := wire.toPullRequest()
	if pr.Number <= 0 || !pr.Draft || pr.Head != req.Head || pr.Base != req.Base {
		return PullRequest{}, errors.New("github draft pull request response did not match the requested identity")
	}
	return pr, nil
}

func (c *RESTClient) UpdateIssueState(ctx context.Context, repo Repository, number int, state string) error {
	path := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	return c.doJSON(ctx, http.MethodPatch, path, struct {
		State string `json:"state"`
	}{State: state}, nil)
}

func (c *RESTClient) GetPullRequest(ctx context.Context, repo Repository, number int) (PullRequest, error) {
	path := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	var wire pullRequestWire
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &wire); err != nil {
		return PullRequest{}, err
	}
	return wire.toPullRequest(), nil
}

type checkRunsWire struct {
	TotalCount *int           `json:"total_count"`
	CheckRuns  []checkRunWire `json:"check_runs"`
}

type checkRunWire struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
}

// GetChecks reads all check-runs for an exact, immutable commit SHA. The
// result contains no provider payload or output and is deterministic by name.
func (c *RESTClient) GetChecks(ctx context.Context, repo Repository, sha string) ([]CheckState, error) {
	if err := validateRepository(repo); err != nil {
		return nil, err
	}
	if !isLowerCommitSHA(sha) {
		return nil, errors.New("github checks require an exact 40-character lowercase SHA")
	}
	rootPath := fmt.Sprintf("%s/repos/%s/%s/commits/%s/check-runs", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name), sha)
	path := rootPath + "?per_page=100"
	var checks []CheckState
	seenPages := make(map[string]struct{})
	seenNames := make(map[string]struct{})
	totalCount := -1
	for page := 0; page < 1000; page++ {
		if _, seen := seenPages[path]; seen {
			return nil, errors.New("github check pagination repeated a page")
		}
		seenPages[path] = struct{}{}
		var wire checkRunsWire
		link, err := c.doSafeJSONWithLink(ctx, "read checks", http.MethodGet, path, nil, &wire)
		if err != nil {
			return nil, err
		}
		if wire.TotalCount == nil || *wire.TotalCount < 0 {
			return nil, errors.New("github checks response requires total_count")
		}
		if totalCount < 0 {
			totalCount = *wire.TotalCount
		} else if totalCount != *wire.TotalCount {
			return nil, errors.New("github checks response changed total_count")
		}
		if len(checks)+len(wire.CheckRuns) > totalCount {
			return nil, errors.New("github checks response exceeded total_count")
		}
		for _, run := range wire.CheckRuns {
			name := strings.TrimSpace(run.Name)
			if name == "" || name != run.Name {
				return nil, errors.New("github check run name is required")
			}
			if _, exists := seenNames[name]; exists {
				return nil, errors.New("github check run names must be unique")
			}
			seenNames[name] = struct{}{}
			checks = append(checks, CheckState{Name: name, State: normalizeCheckState(run.Status, run.Conclusion)})
		}
		if page == 999 {
			return nil, errors.New("github check pagination exceeded the safety limit")
		}
		if len(checks) == totalCount {
			break
		}
		next := nextIssueLink(link)
		if len(wire.CheckRuns) == 0 {
			return nil, errors.New("github checks pagination made no progress")
		}
		if next == "" {
			u, parseErr := url.Parse(path)
			if parseErr != nil {
				return nil, errors.New("github check pagination is malformed")
			}
			query := u.Query()
			pageNumber, parseErr := strconv.Atoi(query.Get("page"))
			if parseErr != nil || pageNumber < 1 {
				pageNumber = 1
			}
			query.Set("page", strconv.Itoa(pageNumber+1))
			u.RawQuery = query.Encode()
			next = u.RequestURI()
		}
		if next == "" {
			break
		}
		path, err = c.validateChecksPageURL(next, rootPath)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	return checks, nil
}

func normalizeCheckState(status string, conclusion *string) string {
	switch status {
	case "queued", "in_progress", "pending", "requested", "waiting":
		return "pending"
	case "completed":
		if conclusion != nil && *conclusion == "success" {
			return "success"
		}
		return "failure"
	default:
		return "failure"
	}
}

func (c *RESTClient) validateChecksPageURL(raw, rootPath string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", errors.New("github check pagination base is malformed")
	}
	next, err := url.Parse(raw)
	if err != nil || next.User != nil {
		return "", errors.New("github check pagination link is malformed")
	}
	if !next.IsAbs() {
		next = base.ResolveReference(next)
	}
	if !sameOrigin(next, base) || next.Path != rootPath || next.Fragment != "" {
		return "", errors.New("github check pagination link is outside the configured GitHub API base")
	}
	for key, values := range next.Query() {
		if key != "page" && key != "per_page" {
			return "", errors.New("github check pagination link contains unsupported query data")
		}
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return "", errors.New("github check pagination link contains an invalid query")
		}
	}
	return next.RequestURI(), nil
}

// CreateIssueComment posts a non-empty issue comment.
func (c *RESTClient) CreateIssueComment(ctx context.Context, repo Repository, number int, body string) error {
	if err := validateRepository(repo); err != nil {
		return err
	}
	if number <= 0 {
		return errors.New("github issue number must be positive")
	}
	if strings.TrimSpace(body) == "" || len(body) > MaxIssueCommentBytes || !safeUserText(body) {
		return errors.New("github issue comment body is required")
	}
	path := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	return c.doSafeJSON(ctx, "create issue comment", http.MethodPost, path, struct {
		Body string `json:"body"`
	}{Body: body}, nil)
}

func safeUserText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == '\x00' || character == '\x7f' || character < ' ' && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return true
}

// MarkReadyForReview transitions a pull request from draft to ready and
// returns the provider response through the same normalized PullRequest type.
func (c *RESTClient) MarkReadyForReview(ctx context.Context, repo Repository, number int) (PullRequest, error) {
	if err := validateRepository(repo); err != nil {
		return PullRequest{}, err
	}
	if number <= 0 {
		return PullRequest{}, errors.New("github pull request number must be positive")
	}
	path := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/ready_for_review", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	var wire pullRequestWire
	if err := c.doSafeJSON(ctx, "mark pull request ready", http.MethodPost, path, nil, &wire); err != nil {
		return PullRequest{}, err
	}
	pr := wire.toPullRequest()
	if pr.Number != number || pr.Draft {
		return PullRequest{}, errors.New("github ready response did not identify a non-draft requested pull request")
	}
	return pr, nil
}

// FindOpenPullRequest finds a draft or ready open PR whose head and base refs
// exactly match the requested pair. Pagination is bounded and provider text is
// discarded on endpoint failure.
func (c *RESTClient) FindOpenPullRequest(ctx context.Context, repo Repository, head, base string) (PullRequest, bool, error) {
	if err := validateRepository(repo); err != nil {
		return PullRequest{}, false, err
	}
	if !validRefInput(head) || !validRefInput(base) {
		return PullRequest{}, false, errors.New("github pull request lookup requires valid head and base refs")
	}
	rootPath := fmt.Sprintf("%s/repos/%s/%s/pulls", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	query := url.Values{"base": []string{base}, "head": []string{repo.Owner + ":" + head}, "per_page": []string{"100"}, "state": []string{"open"}}
	path := rootPath + "?" + query.Encode()
	seenPages := make(map[string]struct{})
	for page := 0; page < 100; page++ {
		if _, seen := seenPages[path]; seen {
			return PullRequest{}, false, errors.New("github pull request pagination repeated a page")
		}
		seenPages[path] = struct{}{}
		var wires []pullRequestWire
		link, err := c.doSafeJSONWithLink(ctx, "find pull request", http.MethodGet, path, nil, &wires)
		if err != nil {
			return PullRequest{}, false, err
		}
		for _, wire := range wires {
			pr := wire.toPullRequest()
			if pr.State == "open" && pr.Head == head && pr.Base == base {
				if pr.Number <= 0 || wire.Head.Label != repo.Owner+":"+head {
					return PullRequest{}, false, errors.New("github pull request lookup returned an unsafe identity")
				}
				return pr, true, nil
			}
		}
		if len(wires) == 0 {
			return PullRequest{}, false, nil
		}
		next := nextIssueLink(link)
		if next == "" && len(wires) == 100 {
			u, parseErr := url.Parse(path)
			if parseErr != nil {
				return PullRequest{}, false, errors.New("github pull request pagination is malformed")
			}
			pageNumber, parseErr := strconv.Atoi(u.Query().Get("page"))
			if parseErr != nil || pageNumber < 1 {
				pageNumber = 1
			}
			q := u.Query()
			q.Set("page", strconv.Itoa(pageNumber+1))
			u.RawQuery = q.Encode()
			next = u.RequestURI()
		}
		if next == "" {
			return PullRequest{}, false, nil
		}
		path, err = c.validatePullRequestPageURL(next, rootPath)
		if err != nil {
			return PullRequest{}, false, err
		}
	}
	return PullRequest{}, false, errors.New("github pull request pagination exceeded the safety limit")
}

func (c *RESTClient) validatePullRequestPageURL(raw, rootPath string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", errors.New("github pull request pagination base is malformed")
	}
	next, err := url.Parse(raw)
	if err != nil || next.User != nil {
		return "", errors.New("github pull request pagination link is malformed")
	}
	if !next.IsAbs() {
		next = base.ResolveReference(next)
	}
	if !sameOrigin(next, base) || next.Path != rootPath || next.Fragment != "" {
		return "", errors.New("github pull request pagination link is outside the configured GitHub API base")
	}
	for key, values := range next.Query() {
		if key != "base" && key != "head" && key != "page" && key != "per_page" && key != "state" {
			return "", errors.New("github pull request pagination link contains unsupported query data")
		}
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return "", errors.New("github pull request pagination link contains an invalid query")
		}
	}
	return next.RequestURI(), nil
}

// MergePullRequest merges only with the exact requested commit SHA and the
// ordinary GitHub merge method. A 2xx response with merged=false is an error.
func (c *RESTClient) MergePullRequest(ctx context.Context, repo Repository, number int, exactSHA, method string) (MergePullRequestResult, error) {
	if err := validateRepository(repo); err != nil {
		return MergePullRequestResult{}, err
	}
	if number <= 0 {
		return MergePullRequestResult{}, errors.New("github pull request number must be positive")
	}
	if !isLowerCommitSHA(exactSHA) {
		return MergePullRequestResult{}, errors.New("github merge requires an exact 40-character lowercase SHA")
	}
	if method != "merge" {
		return MergePullRequestResult{}, errors.New("github merge method must be exactly merge")
	}
	path := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge", c.restBasePath, url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	var result MergePullRequestResult
	if err := c.doSafeJSON(ctx, "merge pull request", http.MethodPut, path, struct {
		SHA         string `json:"sha"`
		MergeMethod string `json:"merge_method"`
	}{SHA: exactSHA, MergeMethod: method}, &result); err != nil {
		return MergePullRequestResult{}, err
	}
	if !result.Merged {
		result.Message = ""
		return result, &MergeError{Result: result}
	}
	return result, nil
}

type pullRequestWire struct {
	Number   int        `json:"number"`
	NodeID   string     `json:"node_id"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	HTMLURL  string     `json:"html_url"`
	State    string     `json:"state"`
	Draft    bool       `json:"draft"`
	Merged   bool       `json:"merged"`
	MergedAt *time.Time `json:"merged_at"`
	Head     struct {
		Ref   string `json:"ref"`
		SHA   string `json:"sha"`
		Label string `json:"label"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Mergeable      *bool  `json:"mergeable"`
	MergeCommitSHA string `json:"merge_commit_sha"`
}

func (w pullRequestWire) toPullRequest() PullRequest {
	return PullRequest{Number: w.Number, NodeID: w.NodeID, Title: w.Title, Body: w.Body, HTMLURL: w.HTMLURL, State: w.State, Draft: w.Draft, Merged: w.Merged || w.MergedAt != nil, Head: w.Head.Ref, HeadSHA: w.Head.SHA, Base: w.Base.Ref, Mergeable: w.Mergeable, MergeCommitSHA: w.MergeCommitSHA}
}

func validRefInput(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && value != "." && value != ".." && !strings.HasPrefix(value, ".") && !strings.ContainsAny(value, "\x00\r\n ~^:?*[\\") && !strings.Contains(value, "..") && !strings.Contains(value, "@{") && !strings.Contains(value, "//") && !strings.Contains(value, "/.") && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, ".lock")
}

func (c *RESTClient) SetProjectStatus(ctx context.Context, project ProjectRef, issueNodeID, status string) error {
	itemID, err := c.AddProjectItem(ctx, project, issueNodeID)
	if err != nil {
		return err
	}
	return c.UpdateProjectStatus(ctx, project, itemID, status)
}

func (c *RESTClient) AddProjectItem(ctx context.Context, project ProjectRef, issueNodeID string) (string, error) {
	projectID := project.ID
	if projectID == "" || issueNodeID == "" {
		return "", &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project item requires project and issue IDs"}
	}
	const addMutation = `mutation AddProjectItem($projectId: ID!, $contentId: ID!) {
  addProjectV2ItemById(input: {projectId: $projectId, contentId: $contentId}) { item { id } }
}`
	addPayload := struct {
		Query     string `json:"query"`
		Variables struct {
			ProjectID string `json:"projectId"`
			ContentID string `json:"contentId"`
		} `json:"variables"`
	}{Query: addMutation}
	addPayload.Variables.ProjectID = projectID
	addPayload.Variables.ContentID = issueNodeID
	var addResponse graphQLAddResponse
	if err := c.doJSON(ctx, http.MethodPost, c.graphqlPath, addPayload, &addResponse); err != nil {
		return "", err
	}
	if err := addResponse.graphQLError(c.token); err != nil {
		return "", err
	}
	itemID := addResponse.Data.Add.Item.ID
	if itemID == "" {
		return "", &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project item was not returned"}
	}
	return itemID, nil
}

func (c *RESTClient) UpdateProjectStatus(ctx context.Context, project ProjectRef, itemID, status string) error {
	projectID := project.ID
	optionID := project.StatusOptions[status]
	if projectID == "" || project.StatusFieldID == "" || optionID == "" || itemID == "" {
		return &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project status requires project, field, option, and item IDs"}
	}
	const mutation = `mutation UpdateProjectStatus($projectId: ID!, $itemId: ID!, $fieldId: ID!, $value: ProjectV2FieldValue!) {
  updateProjectV2ItemFieldValue(input: {projectId: $projectId, itemId: $itemId, fieldId: $fieldId, value: $value}) {
    projectV2Item { id }
  }
}`
	payload := struct {
		Query     string `json:"query"`
		Variables struct {
			ProjectID string `json:"projectId"`
			ItemID    string `json:"itemId"`
			FieldID   string `json:"fieldId"`
			Value     struct {
				SingleSelectOptionID string `json:"singleSelectOptionId"`
			} `json:"value"`
		} `json:"variables"`
	}{Query: mutation}
	payload.Variables.ProjectID = projectID
	payload.Variables.ItemID = itemID
	payload.Variables.FieldID = project.StatusFieldID
	payload.Variables.Value.SingleSelectOptionID = optionID
	var response graphQLResponse
	if err := c.doJSON(ctx, http.MethodPost, c.graphqlPath, payload, &response); err != nil {
		return err
	}
	return response.graphQLError(c.token)
}

// ReadProjectStatus reads the exact ProjectV2 item for an Issue and returns
// presence independently for the item and its configured single-select field.
// An absent or uninitialized item is a normal observation (nil error), while
// ambiguous or malformed provider evidence still fails closed.
func (c *RESTClient) ReadProjectStatus(ctx context.Context, project ProjectRef, issueNodeID string) (ProjectStatus, error) {
	if project.ID == "" || project.StatusFieldID == "" || issueNodeID == "" {
		return ProjectStatus{}, &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project status read requires project, field, and issue IDs"}
	}
	const query = `query ProjectItemStatus($projectId: ID!) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: 100) {
        nodes {
          content { ... on Issue { id } }
          fieldValues(first: 100) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                optionId
                field { ... on ProjectV2SingleSelectField { id } }
              }
            }
          }
        }
      }
    }
  }
}`
	payload := struct {
		Query     string `json:"query"`
		Variables struct {
			ProjectID string `json:"projectId"`
		} `json:"variables"`
	}{Query: query}
	payload.Variables.ProjectID = project.ID
	var response graphQLProjectStatusResponse
	if err := c.doJSON(ctx, http.MethodPost, c.graphqlPath, payload, &response); err != nil {
		return ProjectStatus{}, err
	}
	if err := response.graphQLError(c.token); err != nil {
		return ProjectStatus{}, err
	}
	var itemID, status string
	itemPresent := false
	for _, item := range response.Data.Node.Items.Nodes {
		if item.Content.ID != issueNodeID {
			continue
		}
		if itemID != "" && itemID != item.ID {
			return ProjectStatus{}, &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project status item was ambiguous"}
		}
		itemID = item.ID
		itemPresent = true
		for _, value := range item.FieldValues.Nodes {
			fieldID := value.Field.ID
			if fieldID == "" {
				fieldID = value.FieldID
			}
			if fieldID != project.StatusFieldID || strings.TrimSpace(value.Name) == "" {
				continue
			}
			if expectedOption := project.StatusOptions[value.Name]; expectedOption != "" && value.OptionID != "" && value.OptionID != expectedOption {
				return ProjectStatus{}, &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project status option did not match configured field"}
			}
			if status != "" && status != value.Name {
				return ProjectStatus{}, &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project status read was ambiguous"}
			}
			status = value.Name
		}
	}
	statusPresent := status != ""
	return ProjectStatus{Found: itemPresent && statusPresent, ItemPresent: itemPresent, ItemFound: itemPresent, StatusPresent: statusPresent, StatusFound: statusPresent, ItemID: itemID, Status: status}, nil
}

func (c *RESTClient) GetProjectStatus(ctx context.Context, project ProjectRef, issueNodeID string) (string, error) {
	status, err := c.ReadProjectStatus(ctx, project, issueNodeID)
	return status.Status, err
}

type graphQLErrorItem struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Errors []graphQLErrorItem `json:"errors"`
}

type graphQLAddResponse struct {
	Data struct {
		Add struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		} `json:"addProjectV2ItemById"`
	} `json:"data"`
	Errors []graphQLErrorItem `json:"errors"`
}

type graphQLProjectStatusResponse struct {
	Data struct {
		Node struct {
			Items struct {
				Nodes []struct {
					ID      string `json:"id"`
					Content struct {
						ID string `json:"id"`
					} `json:"content"`
					FieldValues struct {
						Nodes []struct {
							Name     string `json:"name"`
							OptionID string `json:"optionId"`
							Field    struct {
								ID string `json:"id"`
							} `json:"field"`
							FieldID string `json:"fieldId"`
						} `json:"nodes"`
					} `json:"fieldValues"`
				} `json:"nodes"`
			} `json:"items"`
		} `json:"node"`
	} `json:"data"`
	Errors []graphQLErrorItem `json:"errors"`
}

func (r graphQLProjectStatusResponse) graphQLError(token string) error {
	if len(r.Errors) > 0 {
		return &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: sanitize(r.Errors[0].Message, token)}
	}
	return nil
}

func (r graphQLResponse) graphQLError(token string) error {
	if len(r.Errors) > 0 {
		return &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: sanitize(r.Errors[0].Message, token)}
	}
	return nil
}

func (r graphQLAddResponse) graphQLError(token string) error {
	if len(r.Errors) > 0 {
		return &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: sanitize(r.Errors[0].Message, token)}
	}
	return nil
}

func roleMarker(marker, role, key string) string {
	return "<!-- threaddock:" + marker + ":role=" + role + ":key=" + key + " -->"
}

func canonicalMarker(marker string) string {
	return "<!-- threaddock:" + marker + " -->"
}

func withRoleMarker(body, marker, role, key string) string {
	canonical, roleKey := canonicalMarker(marker), roleMarker(marker, role, key)
	if !strings.Contains(body, canonical) && !strings.Contains(body, roleKey) {
		if body == "" {
			return canonical + "\n" + roleKey
		}
		return body + "\n\n" + canonical + "\n" + roleKey
	}
	if !strings.Contains(body, canonical) {
		return body + "\n" + canonical
	}
	if !strings.Contains(body, roleKey) {
		return body + "\n" + roleKey
	}
	return body
}

func parseRoleMarker(body, marker string) (role, key string, ok bool) {
	prefix := "<!-- threaddock:" + marker + ":role="
	start := strings.Index(body, prefix)
	if start < 0 {
		return "", "", false
	}
	rest := body[start+len(prefix):]
	roleEnd := strings.Index(rest, ":key=")
	if roleEnd < 0 {
		return "", "", false
	}
	role = rest[:roleEnd]
	keyEnd := strings.Index(rest[roleEnd+len(":key="):], " -->")
	if keyEnd < 0 {
		return "", "", false
	}
	key = rest[roleEnd+len(":key=") : roleEnd+len(":key=")+keyEnd]
	return role, key, role == "parent" || role == "child"
}

func (c *RESTClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	_, err := c.doJSONWithLink(ctx, method, path, body, out)
	return err
}

func (c *RESTClient) doSafeJSON(ctx context.Context, operation, method, path string, body any, out any) error {
	_, err := c.doSafeJSONWithLink(ctx, operation, method, path, body, out)
	return err
}

func (c *RESTClient) doSafeJSONWithLink(ctx context.Context, operation, method, path string, body any, out any) (string, error) {
	link, err := c.doJSONWithLink(ctx, method, path, body, out)
	if err != nil {
		return "", &EndpointError{Operation: operation, StatusCode: apiErrorStatus(err)}
	}
	return link, nil
}

func apiErrorStatus(err error) int {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return authErr.StatusCode
	}
	var notFoundErr *NotFoundError
	if errors.As(err, &notFoundErr) {
		return notFoundErr.StatusCode
	}
	var conflictErr *ConflictError
	if errors.As(err, &conflictErr) {
		return conflictErr.StatusCode
	}
	var temporaryErr *TemporaryError
	if errors.As(err, &temporaryErr) {
		return temporaryErr.StatusCode
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

func validateRepository(repo Repository) error {
	for field, value := range map[string]string{"owner": repo.Owner, "name": repo.Name} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/\\\x00\r\n") || value == "." || value == ".." {
			return fmt.Errorf("github repository %s is invalid", field)
		}
	}
	return nil
}

func isLowerCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func (c *RESTClient) doJSONWithLink(ctx context.Context, method, path string, body any, out any) (string, error) {
	if c.initErr != nil {
		return "", c.initErr
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return "", &apiError{Message: sanitize(err.Error(), c.token)}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", c.apiVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &TemporaryError{Message: sanitize(err.Error(), c.token)}
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return "", &TemporaryError{StatusCode: resp.StatusCode, Message: sanitize(readErr.Error(), c.token)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", c.statusError(resp, data)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return resp.Header.Get("Link"), nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return "", &apiError{StatusCode: resp.StatusCode, Message: sanitize("invalid JSON response: "+err.Error(), c.token)}
	}
	return resp.Header.Get("Link"), nil
}

func (c *RESTClient) statusError(resp *http.Response, data []byte) error {
	message := strings.TrimSpace(string(data))
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &payload) == nil && payload.Message != "" {
		message = payload.Message
	}
	message = sanitize(message, c.token)
	if resp.StatusCode == http.StatusForbidden && (strings.TrimSpace(resp.Header.Get("Retry-After")) != "" || strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")) == "0") {
		return &TemporaryError{StatusCode: resp.StatusCode, Message: message, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &AuthError{StatusCode: resp.StatusCode, Message: message}
	case http.StatusNotFound:
		return &NotFoundError{StatusCode: resp.StatusCode, Message: message}
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return &ConflictError{StatusCode: resp.StatusCode, Message: message}
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return &TemporaryError{StatusCode: resp.StatusCode, Message: message, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	default:
		if resp.StatusCode >= 500 {
			return &TemporaryError{StatusCode: resp.StatusCode, Message: message, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
		}
		return &apiError{StatusCode: resp.StatusCode, Message: message}
	}
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func sanitize(message, token string) string {
	if token != "" {
		message = strings.ReplaceAll(message, token, "[REDACTED]")
	}
	return message
}

var _ Client = (*RESTClient)(nil)
var _ CheckReader = (*RESTClient)(nil)
var _ IssueCommenter = (*RESTClient)(nil)
var _ PullRequestReadier = (*RESTClient)(nil)
var _ PullRequestFinder = (*RESTClient)(nil)
var _ SafeDraftPRCreator = (*RESTClient)(nil)
var _ PullRequestMerger = (*RESTClient)(nil)
