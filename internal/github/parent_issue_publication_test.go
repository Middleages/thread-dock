package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
)

func TestParentIssueRESTMethodsValidateBeforeIOAndCreateExactIssue(t *testing.T) {
	var requests int
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 19, "node_id": "I_parent", "html_url": "https://ghes.example/issues/19", "title": gotBody["title"], "body": gotBody["body"]})
	}))
	defer server.Close()

	client := NewRESTClient(server.URL, "secret-token", "2022-11-28", server.Client())
	if _, err := client.FindParentIssueMarkers(context.Background(), Repository{}, "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256="); err == nil {
		t.Fatal("expected repository validation error")
	}
	if requests != 0 {
		t.Fatalf("validation made %d requests", requests)
	}

	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", AcceptanceCriteria: []string{"works"}, Labels: []string{"parent", "workflow"}, RepoKey: "primary"}
	issue, err := client.CreateParentIssue(context.Background(), Repository{Owner: "acme", Name: "app"}, draft, "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256="+strings.Repeat("a", 64)+" -->")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v3/repos/acme/app/issues" {
		t.Fatalf("request=%s %s", gotMethod, gotPath)
	}
	wantMarker := "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256=" + strings.Repeat("a", 64) + " -->"
	if gotBody["title"] != draft.Title || gotBody["body"] != draft.Body+"\n\n"+wantMarker || !reflect.DeepEqual(gotBody["labels"], []any{"parent", "workflow"}) {
		t.Fatalf("body=%v", gotBody)
	}
	if issue.Number != 19 || issue.NodeID != "I_parent" || issue.HTMLURL == "" {
		t.Fatalf("issue=%+v", issue)
	}
	if strings.Contains(issue.Body, "secret-token") {
		t.Fatal("response leaked token")
	}
}

func TestParentIssueRESTRejectsNonCanonicalMarkersBeforeIO(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; _ = json.NewEncoder(w).Encode([]Issue{}) }))
	defer server.Close()
	client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
	validPrefix := "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256="
	validMarker := validPrefix + strings.Repeat("a", 64) + " -->"
	for _, tc := range []struct{ name, prefix string }{
		{"empty", ""},
		{"wrong field", "<!-- threaddock:v2:parent_issue:task=work-1:draft=design:sha256="},
		{"unicode ID", "<!-- threaddock:v2:parent_issue:work=工作:draft=design:sha256="},
		{"slash ID", "<!-- threaddock:v2:parent_issue:work=work/1:draft=design:sha256="},
		{"equals ID", "<!-- threaddock:v2:parent_issue:work=work=1:draft=design:sha256="},
		{"extra suffix", validPrefix + "extra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := requests
			if _, err := client.FindParentIssueMarkers(context.Background(), Repository{Owner: "acme", Name: "app"}, tc.prefix); err == nil {
				t.Fatal("accepted empty prefix")
			}
			if requests != before {
				t.Fatalf("validation made %d requests", requests-before)
			}
		})
	}
	for _, tc := range []struct{ name, marker string }{
		{"empty", ""},
		{"prefix", validPrefix},
		{"uppercase hash", validPrefix + strings.Repeat("A", 64) + " -->"},
		{"short hash", validPrefix + strings.Repeat("a", 63) + " -->"},
		{"non-hex hash", validPrefix + strings.Repeat("g", 64) + " -->"},
		{"extra comment text", validMarker + " trailing"},
	} {
		t.Run("exact-"+tc.name, func(t *testing.T) {
			before := requests
			if _, err := client.CreateParentIssue(context.Background(), Repository{Owner: "acme", Name: "app"}, contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Body", RepoKey: "primary"}, tc.marker); err == nil {
				t.Fatal("accepted non-canonical marker")
			}
			if requests != before {
				t.Fatalf("validation made %d requests", requests-before)
			}
		})
	}
}

func TestParentIssueRESTSuppressesProviderBodiesForFindAndCreateErrors(t *testing.T) {
	for _, tc := range []struct {
		name, path, sentinel string
	}{
		{"find", "find", "find-provider-secret"},
		{"create", "create", "create-provider-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tc.sentinel+" token", http.StatusBadGateway)
			}))
			defer server.Close()
			client := NewRESTClient(server.URL, "token", "2022-11-28", server.Client())
			var err error
			if tc.path == "find" {
				_, err = client.FindParentIssueMarkers(context.Background(), Repository{Owner: "acme", Name: "app"}, "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256=")
			} else {
				_, err = client.CreateParentIssue(context.Background(), Repository{Owner: "acme", Name: "app"}, contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Body", RepoKey: "primary"}, "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256="+strings.Repeat("a", 64)+" -->")
			}
			if err == nil || strings.Contains(err.Error(), tc.sentinel) || strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "Design") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func TestFindParentIssueMarkersScansAllPagesAndSuppressesProviderBody(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.URL.Query().Get("page") == "2" {
			w.Header().Set("Link", "")
			_ = json.NewEncoder(w).Encode([]Issue{{Number: 7, NodeID: "node", HTMLURL: "https://ghes/issues/7", Body: "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256="}})
			return
		}
		w.Header().Set("Link", "</api/v3/repos/acme/app/issues?state=all&per_page=100&page=2>; rel=\"next\"")
		_ = json.NewEncoder(w).Encode([]Issue{{Number: 6, NodeID: "node-6", HTMLURL: "https://ghes/issues/6", Body: "unrelated"}})
	}))
	defer server.Close()
	client := NewRESTClient(server.URL, "secret-token", "2022-11-28", server.Client())
	issues, err := client.FindParentIssueMarkers(context.Background(), Repository{Owner: "acme", Name: "app"}, "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256=")
	if err != nil || len(issues) != 1 || issues[0].Number != 7 || pages != 2 {
		t.Fatalf("issues=%v err=%v pages=%d", issues, err, pages)
	}
}
