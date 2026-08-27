package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestErrorNeverContainsToken(t *testing.T) {
	client := NewRESTClient("http://127.0.0.1:1", "secret-token", "2022-11-28", http.DefaultClient)
	_, _, err := client.FindIssueBundle(context.Background(), repo(), "td:run-184")
	if strings.Contains(fmt.Sprint(err), "secret-token") {
		t.Fatal("token leaked")
	}
}

func repo() Repository { return Repository{Owner: "platform", Name: "payments-api"} }

type callCounts struct{ createIssue int }

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
			_ = json.NewEncoder(w).Encode(PullRequest{Number: 17, Draft: true})
			return
		}
		_ = json.NewEncoder(w).Encode(PullRequest{Number: 17, State: "open"})
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
	if err != nil || pr.Number != 17 {
		t.Fatalf("pr=%v err=%v", pr, err)
	}
	if err := client.UpdateIssueState(context.Background(), repo(), 184, "closed"); err != nil {
		t.Fatal(err)
	}
	pr, err = client.GetPullRequest(context.Background(), repo(), 17)
	if err != nil || pr.Number != 17 {
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
	var payload struct {
		Query     string `json:"query"`
		Variables struct {
			ProjectID string `json:"projectId"`
			ItemID    string `json:"itemId"`
			FieldID   string `json:"fieldId"`
			Value     struct {
				OptionID string `json:"singleSelectOptionId"`
			} `json:"value"`
		} `json:"variables"`
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/graphql", func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updateProjectV2ItemFieldValue": map[string]any{"projectV2Item": map[string]string{"id": "item"}}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	err := client.SetProjectStatus(context.Background(), ProjectRef{ID: "P1", StatusFieldID: "F1", StatusOptions: map[string]string{"Review": "O_REVIEW"}}, "I1", "Review")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Variables.ProjectID != "P1" || payload.Variables.ItemID != "I1" || payload.Variables.FieldID != "F1" || payload.Variables.Value.OptionID != "O_REVIEW" {
		t.Fatalf("variables=%+v", payload.Variables)
	}
	if !strings.Contains(payload.Query, "updateProjectV2ItemFieldValue") || !strings.Contains(payload.Query, "ProjectV2FieldValue") {
		t.Fatalf("query=%q", payload.Query)
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
