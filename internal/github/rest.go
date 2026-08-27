package github

import (
	"bytes"
	"context"
	"encoding/json"
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
	var issues []Issue
	path := fmt.Sprintf("/api/v3/repos/%s/%s/issues?state=all&per_page=100", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &issues); err != nil {
		return IssueBundle{}, false, err
	}
	want := markerText(marker)
	var parent Issue
	for _, issue := range issues {
		if strings.Contains(issue.Body, want) {
			parent = issue
			break
		}
	}
	if parent.Number == 0 {
		return IssueBundle{}, false, nil
	}
	// The marker is carried by every issue in the bundle, allowing a later
	// listing response to recover the full bundle without another endpoint.
	var children []Issue
	for _, issue := range issues {
		if issue.Number != parent.Number && strings.Contains(issue.Body, want) {
			children = append(children, issue)
		}
	}
	return IssueBundle{Parent: parent, Children: children}, true, nil
}

func (c *RESTClient) CreateIssueBundle(ctx context.Context, repo Repository, task contract.TaskContract, marker string) (IssueBundle, error) {
	if bundle, found, err := c.FindIssueBundle(ctx, repo, marker); err != nil {
		return IssueBundle{}, err
	} else if found {
		return bundle, nil
	}

	parentBody := withMarker(task.Parent.Body, marker)
	parent, err := c.createIssue(ctx, repo, task.Parent.Title, parentBody, task.Parent.Labels)
	if err != nil {
		return IssueBundle{}, err
	}
	bundle := IssueBundle{Parent: parent}
	for _, draft := range task.Children {
		child, createErr := c.createIssue(ctx, repo, draft.Title, withMarker(draft.Body, marker), draft.Labels)
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
	var pr PullRequest
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &pr); err != nil {
		return PullRequest{}, err
	}
	return pr, nil
}

func (c *RESTClient) UpdateIssueState(ctx context.Context, repo Repository, number int, state string) error {
	path := fmt.Sprintf("/api/v3/repos/%s/%s/issues/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	return c.doJSON(ctx, http.MethodPatch, path, struct {
		State string `json:"state"`
	}{State: state}, nil)
}

func (c *RESTClient) GetPullRequest(ctx context.Context, repo Repository, number int) (PullRequest, error) {
	path := fmt.Sprintf("/api/v3/repos/%s/%s/pulls/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	var pr PullRequest
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &pr); err != nil {
		return PullRequest{}, err
	}
	return pr, nil
}

func (c *RESTClient) SetProjectStatus(ctx context.Context, project ProjectRef, issueNodeID, status string) error {
	projectID := project.ID
	optionID := project.StatusOptions[status]
	if projectID == "" || project.StatusFieldID == "" || optionID == "" || issueNodeID == "" {
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
	payload.Variables.ItemID = issueNodeID
	payload.Variables.FieldID = project.StatusFieldID
	payload.Variables.Value.SingleSelectOptionID = optionID
	var response struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/graphql", payload, &response); err != nil {
		return err
	}
	if len(response.Errors) > 0 {
		return &apiError{StatusCode: http.StatusUnprocessableEntity, Message: sanitize(response.Errors[0].Message, c.token)}
	}
	return nil
}

func markerText(marker string) string { return "<!-- threaddock:" + marker + " -->" }

func withMarker(body, marker string) string {
	mark := markerText(marker)
	if strings.Contains(body, mark) {
		return body
	}
	if body == "" {
		return mark
	}
	return body + "\n\n" + mark
}

func (c *RESTClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return &apiError{Message: sanitize(err.Error(), c.token)}
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
		return &TemporaryError{Message: sanitize(err.Error(), c.token)}
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return &TemporaryError{StatusCode: resp.StatusCode, Message: sanitize(readErr.Error(), c.token)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return c.statusError(resp, data)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &apiError{StatusCode: resp.StatusCode, Message: sanitize("invalid JSON response: "+err.Error(), c.token)}
	}
	return nil
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
