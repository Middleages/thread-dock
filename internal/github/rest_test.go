package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/testfixture"
)

func TestCreateIssueBundleIsIdempotent(t *testing.T) {
	server, calls := fakeGHES(t)
	defer server.Close()
	client := NewRESTClient(server.URL, "secret-token", "2022-11-28", server.Client())

	first, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "td:run-184")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "td:run-184")
	if err != nil {
		t.Fatal(err)
	}
	if first.Parent != second.Parent || calls.createIssue != 3 {
		t.Fatalf("first=%v second=%v calls=%d", first, second, calls.createIssue)
	}
}

func TestIssuePostsContainCanonicalAndRoleKeyMarkers(t *testing.T) {
	server, calls := fakeGHES(t)
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	if _, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "td:run-184"); err != nil {
		t.Fatal(err)
	}
	wantCanonical := "<!-- threaddock:td:run-184 -->"
	wantMarkers := []string{
		"<!-- threaddock:td:run-184:role=parent:key=parent -->",
		"<!-- threaddock:td:run-184:role=child:key=api -->",
		"<!-- threaddock:td:run-184:role=child:key=tests -->",
	}
	if len(calls.bodies) != len(wantMarkers) {
		t.Fatalf("bodies=%d want=%d", len(calls.bodies), len(wantMarkers))
	}
	for i, body := range calls.bodies {
		if !strings.Contains(body, wantCanonical) || !strings.Contains(body, wantMarkers[i]) {
			t.Fatalf("body[%d]=%q", i, body)
		}
	}
}

func TestCreateIssueBundleReconcilesTransientPartialCreation(t *testing.T) {
	var issues []map[string]any
	posts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/platform/payments-api/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(issues)
			return
		}
		var body struct{ Title, Body string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		posts++
		if posts == 3 {
			http.Error(w, `{"message":"temporary"}`, http.StatusServiceUnavailable)
			return
		}
		issue := map[string]any{"number": posts, "node_id": fmt.Sprintf("I_%d", posts), "title": body.Title, "body": body.Body}
		issues = append(issues, issue)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issue)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	if _, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "run-184"); err == nil {
		t.Fatal("expected transient child failure")
	}
	bundle, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "run-184")
	if err != nil {
		t.Fatal(err)
	}
	if posts != 4 || bundle.Parent.Title != "결제 실패 재시도 개선" || len(bundle.Children) != 2 {
		t.Fatalf("posts=%d bundle=%+v", posts, bundle)
	}
	if strings.Contains(bundle.Children[0].Body, "key=tests") || !strings.Contains(bundle.Children[1].Body, "key=tests") {
		t.Fatalf("children=%+v", bundle.Children)
	}
}

func TestFindIssueBundleIdentifiesParentByRoleMarker(t *testing.T) {
	issues := []map[string]any{
		{"number": 2, "node_id": "I2", "title": "child", "body": "<!-- threaddock:run-184:role=child:key=api -->"},
		{"number": 1, "node_id": "I1", "title": "parent", "body": "<!-- threaddock:run-184:role=parent:key=parent -->"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(issues) }))
	defer server.Close()
	bundle, found, err := NewRESTClient(server.URL, "token", "2022-11-28", server.Client()).FindIssueBundle(context.Background(), repo(), "run-184")
	if err != nil || !found || bundle.Parent.Title != "parent" {
		t.Fatalf("bundle=%+v found=%v err=%v", bundle, found, err)
	}
}

func TestCreateIssueBundleFindsMarkerOnLaterPageWithoutPosting(t *testing.T) {
	var posts int
	var pages []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/platform/payments-api/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "unexpected post", http.StatusInternalServerError)
			return
		}
		pages = append(pages, r.URL.RawQuery)
		w.Header().Set("Link", "</api/v3/repos/platform/payments-api/issues?state=all&per_page=100&page=2>; rel=\"next\"")
		if r.URL.Query().Get("page") == "2" {
			w.Header().Set("Link", "")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"number": 184, "node_id": "parent-node", "title": "parent", "body": "<!-- threaddock:run-184:role=parent:key=parent -->"},
				{"number": 185, "node_id": "child-node", "title": "child", "body": "<!-- threaddock:run-184:role=child:key=api -->"},
				{"number": 186, "node_id": "tests-node", "title": "tests", "body": "<!-- threaddock:run-184:role=child:key=tests -->"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 1, "title": "unrelated", "body": "no marker"}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	bundle, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "run-184")
	if err != nil {
		t.Fatalf("err=%v pages=%v posts=%d", err, pages, posts)
	}
	if bundle.Parent.Number != 184 || len(bundle.Children) != 2 || bundle.Children[0].Number != 185 || bundle.Children[1].Number != 186 {
		t.Fatalf("bundle=%+v", bundle)
	}
	if posts != 0 {
		t.Fatalf("CreateIssueBundle posted %d times after finding later-page marker", posts)
	}
	if len(pages) != 2 || pages[1] == pages[0] {
		t.Fatalf("pages=%v", pages)
	}
}

func TestCreateIssueBundleReadsPastChildMarkerToFindCompleteOrderedBundle(t *testing.T) {
	var posts int
	var pages []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/platform/payments-api/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "unexpected post", http.StatusInternalServerError)
			return
		}
		pages = append(pages, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "2" {
			w.Header().Set("Link", "</api/v3/repos/platform/payments-api/issues?state=all&per_page=100&page=2>; rel=\"next\"")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"number": 185, "node_id": "child-api", "title": "api", "body": "<!-- threaddock:run-184:role=child:key=api -->"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"number": 184, "node_id": "parent-node", "title": "parent", "body": "<!-- threaddock:run-184:role=parent:key=parent -->"},
			{"number": 186, "node_id": "tests-node", "title": "tests", "body": "<!-- threaddock:run-184:role=child:key=tests -->"},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	bundle, err := NewRESTClient(server.URL, "token", "2022-11-28", server.Client()).CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "run-184")
	if err != nil {
		t.Fatalf("err=%v pages=%v posts=%d", err, pages, posts)
	}
	if posts != 0 {
		t.Fatalf("CreateIssueBundle posted %d times", posts)
	}
	if bundle.Parent.Number != 184 || len(bundle.Children) != 2 || bundle.Children[0].Number != 185 || bundle.Children[1].Number != 186 {
		t.Fatalf("bundle=%+v", bundle)
	}
	if len(pages) != 2 || pages[0] == pages[1] {
		t.Fatalf("pages=%v", pages)
	}
}

func TestFindIssueBundleRejectsNextLinkOutsideGHESBaseWithoutLeakingToken(t *testing.T) {
	const token = "secret-token"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/platform/payments-api/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", "<https://other-ghes.example/api/v3/repos/platform/payments-api/issues?page=2&access_token="+token+">; rel=\"next\"")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 1, "title": "unrelated", "body": "no marker"}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, _, err := NewRESTClient(server.URL, token, "2022-11-28", server.Client()).FindIssueBundle(context.Background(), repo(), "run-184")
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("err=%v", err)
	}
}

func TestErrorNeverContainsToken(t *testing.T) {
	client := NewRESTClient("http://127.0.0.1:1", "secret-token", "2022-11-28", http.DefaultClient)
	_, _, err := client.FindIssueBundle(context.Background(), repo(), "td:run-184")
	if strings.Contains(fmt.Sprint(err), "secret-token") {
		t.Fatal("token leaked")
	}
}

func TestGitHubComUsesPublicRESTAndGraphQLEndpoints(t *testing.T) {
	var paths []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/repos/platform/payments-api/issues" {
			return jsonResponse(http.StatusOK, `[ {"number":1,"title":"parent","body":"<!-- threaddock:run-184:role=parent:key=parent -->"} ]`), nil
		}
		if r.URL.Path == "/graphql" {
			return jsonResponse(http.StatusOK, `{"data":{"addProjectV2ItemById":{"item":{"id":"ITEM_REVIEW"}}}}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{"message":"unexpected path"}`), nil
	})
	client := NewRESTClient("https://api.github.com", "token", "2022-11-28", &http.Client{Transport: transport})

	if _, _, err := client.FindIssueBundle(context.Background(), repo(), "run-184"); err != nil {
		t.Fatal(err)
	}
	if err := client.SetProjectStatus(context.Background(), ProjectRef{ID: "P1", StatusFieldID: "F1", StatusOptions: map[string]string{"Review": "O_REVIEW"}}, "I1", "Review"); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(paths, ","), "/repos/platform/payments-api/issues,/graphql,/graphql"; got != want {
		t.Fatalf("paths=%q want=%q", got, want)
	}
}

func TestGHESAPIv3InputKeepsEnterpriseEndpoints(t *testing.T) {
	server, _ := fakeGHES(t)
	defer server.Close()
	client := NewRESTClient(server.URL+"/api/v3/", "token", "2022-11-28", server.Client())
	if _, _, err := client.FindIssueBundle(context.Background(), repo(), "run-184"); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubComPaginationRejectsSensitiveSameOriginLink(t *testing.T) {
	const token = "secret-token"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/platform/payments-api/issues" {
			return jsonResponse(http.StatusNotFound, `{"message":"unexpected path"}`), nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Link": []string{"<https://api.github.com/repos/platform/payments-api/issues?page=2&access_token=" + token + ">; rel=\"next\""}},
			Body:       io.NopCloser(strings.NewReader(`[{"number":1,"title":"unrelated","body":"no marker"}]`)),
		}, nil
	})
	client := NewRESTClient("https://api.github.com", token, "2022-11-28", &http.Client{Transport: transport})
	_, _, err := client.FindIssueBundle(context.Background(), repo(), "run-184")
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("err=%v", err)
	}
}

func TestGitHubComPaginationAcceptsRepositoryIDLink(t *testing.T) {
	var paths []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/repos/platform/payments-api/issues":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "Link": []string{"<https://api.github.com/repositories/123456/issues?state=all&per_page=100&page=2>; rel=\"next\""}},
				Body:       io.NopCloser(strings.NewReader(`[{"number":1,"title":"unrelated","body":"no marker"}]`)),
			}, nil
		case "/repositories/123456/issues":
			return jsonResponse(http.StatusOK, `[{"number":184,"title":"parent","body":"<!-- threaddock:run-184:role=parent:key=parent -->"}]`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"message":"unexpected path"}`), nil
		}
	})
	client := NewRESTClient("https://api.github.com", "token", "2022-11-28", &http.Client{Transport: transport})
	bundle, found, err := client.FindIssueBundle(context.Background(), repo(), "run-184")
	if err != nil || !found || bundle.Parent.Number != 184 {
		t.Fatalf("bundle=%+v found=%v err=%v", bundle, found, err)
	}
	if got, want := strings.Join(paths, ","), "/repos/platform/payments-api/issues,/repositories/123456/issues"; got != want {
		t.Fatalf("paths=%q want=%q", got, want)
	}
}

func repo() Repository { return Repository{Owner: "platform", Name: "payments-api"} }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type callCounts struct {
	createIssue int
	bodies      []string
}

func fakeGHES(t *testing.T) (*httptest.Server, *callCounts) {
	t.Helper()
	calls := &callCounts{}
	issues := []map[string]any{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/platform/payments-api/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var body struct {
				Title, Body string
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			calls.createIssue++
			calls.bodies = append(calls.bodies, body.Body)
			issue := map[string]any{"number": calls.createIssue, "node_id": fmt.Sprintf("I_%d", calls.createIssue), "title": body.Title, "body": body.Body}
			issues = append(issues, issue)
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(issue); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(issues); err != nil {
			t.Fatal(err)
		}
	})
	return httptest.NewServer(mux), calls
}

func TestRESTOperationsSendRequiredHeadersAndPayloads(t *testing.T) {
	var seen []string
	var gotPR struct {
		Title, Body, Head, Base string
		Draft                   bool
	}
	var gotState struct{ State string }
	mux := http.NewServeMux()
	pullHandler := func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		assertHeaders(t, r)
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&gotPR); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 17, "draft": true, "head": map[string]string{"ref": "feature"}, "base": map[string]string{"ref": "main"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 17, "state": "open", "head": map[string]string{"ref": "feature"}, "base": map[string]string{"ref": "main"}})
	}
	mux.HandleFunc("/api/v3/repos/platform/payments-api/pulls", pullHandler)
	mux.HandleFunc("/api/v3/repos/platform/payments-api/pulls/", pullHandler)
	mux.HandleFunc("/api/v3/repos/platform/payments-api/issues/184", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		assertHeaders(t, r)
		if err := json.NewDecoder(r.Body).Decode(&gotState); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())

	pr, err := client.CreateDraftPR(context.Background(), repo(), DraftPRRequest{Title: "Draft", Body: "body", Head: "feature", Base: "main"})
	if err != nil || pr.Number != 17 || pr.Head != "feature" || pr.Base != "main" {
		t.Fatalf("pr=%v err=%v", pr, err)
	}
	if err := client.UpdateIssueState(context.Background(), repo(), 184, "closed"); err != nil {
		t.Fatal(err)
	}
	pr, err = client.GetPullRequest(context.Background(), repo(), 17)
	if err != nil || pr.Number != 17 || pr.Head != "feature" || pr.Base != "main" {
		t.Fatalf("pr=%v err=%v", pr, err)
	}
	if !gotPR.Draft || gotPR.Head != "feature" || gotPR.Base != "main" || gotState.State != "closed" {
		t.Fatalf("pr payload=%+v state=%+v", gotPR, gotState)
	}
	if len(seen) != 3 {
		t.Fatalf("seen=%v", seen)
	}
}

func TestSetProjectStatusUsesConfiguredOptionID(t *testing.T) {
	type graphCall struct {
		query, itemID, optionID string
	}
	var calls []graphCall
	mux := http.NewServeMux()
	mux.HandleFunc("/api/graphql", func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		var payload struct {
			Query     string `json:"query"`
			Variables struct {
				ItemID string `json:"itemId"`
				Value  struct {
					OptionID string `json:"singleSelectOptionId"`
				} `json:"value"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload.Query, "addProjectV2ItemById") {
			calls = append(calls, graphCall{query: "add"})
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"addProjectV2ItemById": map[string]any{"item": map[string]string{"id": "ITEM_REVIEW"}}}})
			return
		}
		calls = append(calls, graphCall{query: "update", itemID: payload.Variables.ItemID, optionID: payload.Variables.Value.OptionID})
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updateProjectV2ItemFieldValue": map[string]any{"projectV2Item": map[string]string{"id": "ITEM_REVIEW"}}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	err := client.SetProjectStatus(context.Background(), ProjectRef{ID: "P1", StatusFieldID: "F1", StatusOptions: map[string]string{"Review": "O_REVIEW"}}, "I1", "Review")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].query != "add" || calls[1].query != "update" || calls[1].itemID != "ITEM_REVIEW" || calls[1].optionID != "O_REVIEW" {
		t.Fatalf("calls=%+v", calls)
	}
}

func TestProjectStatusOptionsAreUsedForEveryPhase(t *testing.T) {
	statuses := []struct{ name, option string }{{"Backlog", "O_BACKLOG"}, {"Ready", "O_READY"}, {"In Progress", "O_PROGRESS"}, {"Review", "O_REVIEW"}, {"Done", "O_DONE"}}
	var updates []struct{ item, option string }
	mux := http.NewServeMux()
	mux.HandleFunc("/api/graphql", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Query     string `json:"query"`
			Variables struct {
				ItemID string `json:"itemId"`
				Value  struct {
					OptionID string `json:"singleSelectOptionId"`
				} `json:"value"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload.Query, "addProjectV2ItemById") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"addProjectV2ItemById": map[string]any{"item": map[string]string{"id": "item-added"}}}})
			return
		}
		updates = append(updates, struct{ item, option string }{payload.Variables.ItemID, payload.Variables.Value.OptionID})
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updateProjectV2ItemFieldValue": map[string]any{"projectV2Item": map[string]string{"id": "item-added"}}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	options := map[string]string{}
	for _, status := range statuses {
		options[status.name] = status.option
	}
	for _, status := range statuses {
		if err := client.SetProjectStatus(context.Background(), ProjectRef{ID: "P1", StatusFieldID: "F1", StatusOptions: options}, "issue-node", status.name); err != nil {
			t.Fatal(err)
		}
	}
	if len(updates) != len(statuses) {
		t.Fatalf("updates=%v", updates)
	}
	for i, status := range statuses {
		if updates[i].item != "item-added" || updates[i].option != status.option {
			t.Fatalf("update[%d]=%+v", i, updates[i])
		}
	}
}

func TestHTTPFailuresAreTypedAndRetryHintIsParsed(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		retry     string
		want      string
		wantAfter time.Duration
	}{
		{name: "auth", status: http.StatusUnauthorized, want: "auth"},
		{name: "forbidden", status: http.StatusForbidden, want: "auth"},
		{name: "missing", status: http.StatusNotFound, want: "not-found"},
		{name: "conflict", status: http.StatusConflict, want: "conflict"},
		{name: "validation", status: http.StatusUnprocessableEntity, want: "conflict"},
		{name: "temporary", status: http.StatusServiceUnavailable, retry: "7", want: "temporary", wantAfter: 7 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", tt.retry)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"failure"}`))
			}))
			defer server.Close()
			client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
			_, _, err := client.FindIssueBundle(context.Background(), repo(), "marker")
			if err == nil {
				t.Fatal("expected error")
			}
			var typed error
			switch tt.want {
			case "auth":
				var target *AuthError
				typed = target
				if !errors.As(err, &target) {
					t.Fatalf("err=%T %v", err, err)
				}
			case "not-found":
				var target *NotFoundError
				typed = target
				if !errors.As(err, &target) {
					t.Fatalf("err=%T %v", err, err)
				}
			case "conflict":
				var target *ConflictError
				typed = target
				if !errors.As(err, &target) {
					t.Fatalf("err=%T %v", err, err)
				}
			case "temporary":
				var target *TemporaryError
				typed = target
				if !errors.As(err, &target) {
					t.Fatalf("err=%T %v", err, err)
				}
			}
			if typed == nil {
				t.Fatalf("untyped err=%T %v", err, err)
			}
			if got, ok := err.(*TemporaryError); ok && got.RetryAfter != tt.wantAfter {
				t.Fatalf("retry after=%s", got.RetryAfter)
			}
		})
	}
}

func TestRateLimitedForbiddenIsTemporary(t *testing.T) {
	for _, header := range []string{"retry-after", "x-rate-limit"} {
		t.Run(header, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if header == "retry-after" {
					w.Header().Set("Retry-After", "4")
				} else {
					w.Header().Set("X-RateLimit-Remaining", "0")
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"throttled"}`))
			}))
			defer server.Close()
			client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
			_, _, err := client.FindIssueBundle(context.Background(), repo(), "marker")
			var temporary *TemporaryError
			if !errors.As(err, &temporary) {
				t.Fatalf("err=%T %v", err, err)
			}
			if header == "retry-after" && temporary.RetryAfter != 4*time.Second {
				t.Fatalf("retry=%s", temporary.RetryAfter)
			}
		})
	}
}

func assertHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
		t.Fatalf("accept=%q", got)
	}
	if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Fatalf("api version=%q", got)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("authorization=%q", got)
	}
}
