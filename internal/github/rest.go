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
	"strconv"
	"strings"
	"time"

	"thread-dock/internal/contract"
)

const defaultAPIVersion = "2022-11-28"

// RESTClient implements Client against the GHES REST and GraphQL endpoints.
type RESTClient struct {
	baseURL    string
	token      string
	apiVersion string
	httpClient *http.Client
}

// NewRESTClient creates a GHES client. The base URL may be either the GHES
// host or its /api/v3 REST root; both forms are accepted.
func NewRESTClient(baseURL, token, apiVersion string, httpClient *http.Client) *RESTClient {
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/v3")
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &RESTClient{baseURL: baseURL, token: token, apiVersion: apiVersion, httpClient: httpClient}
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
	issuePath := fmt.Sprintf("/api/v3/repos/%s/%s/issues", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
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
	if err != nil || next.IsAbs() && (next.Scheme != base.Scheme || next.Host != base.Host) || next.User != nil {
		return "", errors.New("github issue pagination link is outside the configured GHES base")
	}
	if !next.IsAbs() {
		next = base.ResolveReference(next)
	}
	if next.Scheme != base.Scheme || next.Host != base.Host || next.Path != issuePath || next.Fragment != "" {
		return "", errors.New("github issue pagination link is outside the configured GHES base")
	}
	for key, values := range next.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") {
			return "", errors.New("github issue pagination link contains sensitive query data")
		}
		for _, value := range values {
			if c.token != "" && strings.Contains(value, c.token) {
				return "", errors.New("github issue pagination link contains sensitive query data")
			}
		}
	}
	return next.RequestURI(), nil
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
	path := fmt.Sprintf("/api/v3/repos/%s/%s/issues", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
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
	path := fmt.Sprintf("/api/v3/repos/%s/%s/pulls", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
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

func (c *RESTClient) UpdateIssueState(ctx context.Context, repo Repository, number int, state string) error {
	path := fmt.Sprintf("/api/v3/repos/%s/%s/issues/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	return c.doJSON(ctx, http.MethodPatch, path, struct {
		State string `json:"state"`
	}{State: state}, nil)
}

func (c *RESTClient) GetPullRequest(ctx context.Context, repo Repository, number int) (PullRequest, error) {
	path := fmt.Sprintf("/api/v3/repos/%s/%s/pulls/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	var wire pullRequestWire
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &wire); err != nil {
		return PullRequest{}, err
	}
	return wire.toPullRequest(), nil
}

type pullRequestWire struct {
	Number  int    `json:"number"`
	NodeID  string `json:"node_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	Head    struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (w pullRequestWire) toPullRequest() PullRequest {
	return PullRequest{Number: w.Number, NodeID: w.NodeID, Title: w.Title, Body: w.Body, HTMLURL: w.HTMLURL, State: w.State, Draft: w.Draft, Head: w.Head.Ref, Base: w.Base.Ref}
}

func (c *RESTClient) SetProjectStatus(ctx context.Context, project ProjectRef, issueNodeID, status string) error {
	projectID := project.ID
	optionID := project.StatusOptions[status]
	if projectID == "" || project.StatusFieldID == "" || optionID == "" || issueNodeID == "" {
		return &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project status requires project, field, option, and item IDs"}
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
	if err := c.doJSON(ctx, http.MethodPost, "/api/graphql", addPayload, &addResponse); err != nil {
		return err
	}
	if err := addResponse.graphQLError(c.token); err != nil {
		return err
	}
	itemID := addResponse.Data.Add.Item.ID
	if itemID == "" {
		return &ConflictError{StatusCode: http.StatusUnprocessableEntity, Message: "project item was not returned"}
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
	if err := c.doJSON(ctx, http.MethodPost, "/api/graphql", payload, &response); err != nil {
		return err
	}
	return response.graphQLError(c.token)
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

func (c *RESTClient) doJSONWithLink(ctx context.Context, method, path string, body any, out any) (string, error) {
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
