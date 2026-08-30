package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMarkReadyForReviewUsesGraphQLNodeMutationForPublicAndGHES(t *testing.T) {
	for _, tc := range []struct {
		name string
		base string
		path string
	}{
		{name: "public", base: "https://api.github.com", path: "/graphql"},
		{name: "ghes", base: "https://github.example.test", path: "/api/graphql"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var method, requestPath string
			var payload struct {
				Query     string `json:"query"`
				Variables struct {
					PullRequestID string `json:"pullRequestId"`
				} `json:"variables"`
			}
			client := NewRESTClient(tc.base, "token", "2022-11-28", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requestPath = r.URL.Path
				method = r.Method
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					return nil, err
				}
				return jsonResponse(http.StatusOK, `{"data":{"markPullRequestReadyForReview":{"pullRequest":{"id":"PR_node","number":17,"isDraft":false,"headRefName":"feature","headRefOid":"0123456789abcdef0123456789abcdef01234567","baseRefName":"main"}}}}`), nil
			})})
			pr, err := client.MarkReadyForReview(context.Background(), repo(), "PR_node")
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodPost || requestPath != tc.path || payload.Variables.PullRequestID != "PR_node" || !strings.Contains(payload.Query, "markPullRequestReadyForReview") {
				t.Fatalf("method=%q path=%q variables=%+v query=%q", method, requestPath, payload.Variables, payload.Query)
			}
			if pr.NodeID != "PR_node" || pr.Number != 17 || pr.Draft || pr.HeadSHA == "" {
				t.Fatalf("pr=%+v", pr)
			}
		})
	}
}

func TestMarkReadyForReviewRejectsMismatchedGraphQLNode(t *testing.T) {
	transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"data":{"markPullRequestReadyForReview":{"pullRequest":{"id":"other","number":17,"isDraft":false}}}}`), nil
	})
	client := NewRESTClient("https://api.github.com", "token", "2022-11-28", &http.Client{Transport: transport})
	if _, err := client.MarkReadyForReview(context.Background(), repo(), "PR_node"); err == nil {
		t.Fatal("expected exact node mismatch to fail")
	}
}

func TestFindIssueCommentPaginatesExactMarker(t *testing.T) {
	page := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		page++
		if r.URL.Path != "/repos/platform/payments-api/issues/17/comments" {
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
		if page == 1 {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Link": []string{`</repos/platform/payments-api/issues/17/comments?page=2&per_page=100>; rel="next"`}}, Body: io.NopCloser(strings.NewReader(`[{"id":1,"body":"unrelated"}]`))}, nil
		}
		return jsonResponse(http.StatusOK, `[{"id":2,"body":"<!-- threaddock:run-1:protected-change -->"}]`), nil
	})
	client := NewRESTClient("https://api.github.com", "token", "2022-11-28", &http.Client{Transport: transport})
	found, err := client.FindIssueComment(context.Background(), repo(), 17, "<!-- threaddock:run-1:protected-change -->")
	if err != nil || !found || page != 2 {
		t.Fatalf("found=%v page=%d err=%v", found, page, err)
	}
}
