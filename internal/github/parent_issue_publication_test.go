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
	if _, err := client.FindParentIssueMarkers(context.Background(), Repository{}, "prefix"); err == nil {
		t.Fatal("expected repository validation error")
	}
	if requests != 0 {
		t.Fatalf("validation made %d requests", requests)
	}

	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", AcceptanceCriteria: []string{"works"}, Labels: []string{"parent", "workflow"}, RepoKey: "primary"}
	issue, err := client.CreateParentIssue(context.Background(), Repository{Owner: "acme", Name: "app"}, draft, "<!-- marker -->")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v3/repos/acme/app/issues" {
		t.Fatalf("request=%s %s", gotMethod, gotPath)
	}
	if gotBody["title"] != draft.Title || gotBody["body"] != draft.Body+"\n\n<!-- marker -->" || !reflect.DeepEqual(gotBody["labels"], []any{"parent", "workflow"}) {
		t.Fatalf("body=%v", gotBody)
	}
	if issue.Number != 19 || issue.NodeID != "I_parent" || issue.HTMLURL == "" {
		t.Fatalf("issue=%+v", issue)
	}
	if strings.Contains(issue.Body, "secret-token") {
		t.Fatal("response leaked token")
	}
}

func TestFindParentIssueMarkersScansAllPagesAndSuppressesProviderBody(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.URL.Query().Get("page") == "2" {
			w.Header().Set("Link", "")
			_ = json.NewEncoder(w).Encode([]Issue{{Number: 7, NodeID: "node", HTMLURL: "https://ghes/issues/7", Body: "prefix exact"}})
			return
		}
		w.Header().Set("Link", "</api/v3/repos/acme/app/issues?state=all&per_page=100&page=2>; rel=\"next\"")
		_ = json.NewEncoder(w).Encode([]Issue{{Number: 6, NodeID: "node-6", HTMLURL: "https://ghes/issues/6", Body: "unrelated"}})
	}))
	defer server.Close()
	client := NewRESTClient(server.URL, "secret-token", "2022-11-28", server.Client())
	issues, err := client.FindParentIssueMarkers(context.Background(), Repository{Owner: "acme", Name: "app"}, "prefix")
	if err != nil || len(issues) != 1 || issues[0].Number != 7 || pages != 2 {
		t.Fatalf("issues=%v err=%v pages=%d", issues, err, pages)
	}
}
