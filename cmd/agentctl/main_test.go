package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"thread-dock/internal/cli"
	"thread-dock/internal/config"
	"thread-dock/internal/contract"
	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/github"
	"thread-dock/internal/registry"
	"thread-dock/internal/runner"
	"thread-dock/internal/state"
	statev2 "thread-dock/internal/state/v2"
)

func TestRuntimeWorkflowCommandOnlyAcceptsExactRuntimeShapes(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want bool
	}{
		{[]string{"work", "run", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, true},
		{[]string{"work", "reconcile", "work-1", "--json"}, true},
		{[]string{"work", "status", "work-1", "--json"}, false},
		{[]string{"project", "status", "--all", "--json"}, false},
		{[]string{"work", "plan", "contract.json", "--expected-revision", "0", "--request-id", "request-1"}, false},
		{[]string{"work", "approve", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, false},
		{[]string{"work", "pause", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, false},
		{[]string{"work", "resume", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, false},
		{[]string{"work", "run", "work-1", "--expected-revision", "0", "--request-id", "request-1"}, false},
		{[]string{"work", "reconcile", "work-1"}, false},
	} {
		if got := runtimeWorkflowCommand(tt.args); got != tt.want {
			t.Errorf("args=%v runtime=%t want=%t", tt.args, got, tt.want)
		}
	}
}

func TestProductionDependenciesRouteOpenCodeRoleAgents(t *testing.T) {
	cfg := config.Config{OpenCodeAgents: config.OpenCodeAgents{
		Builder:  "build",
		Reviewer: "review",
	}}

	builder, reviewer := roleAgentRouting(cfg)
	if builder != "build" || reviewer != "review" {
		t.Fatalf("role agent routing builder=%q reviewer=%q, want build/review", builder, reviewer)
	}
}

func TestRepositoryDiscoveryOnlyRunsForStart(t *testing.T) {
	var calls int
	discover := func(context.Context, runner.Runner, string) (string, error) {
		calls++
		return "/workspace/repo", nil
	}

	path, err := repositoryPathForCommand(context.Background(), []string{"start", "contract.json"}, nil, "git", discover)
	if err != nil || path != "/workspace/repo" || calls != 1 {
		t.Fatalf("start path=%q err=%v calls=%d", path, err, calls)
	}
	for _, command := range []string{"status", "stop", "cleanup"} {
		path, err = repositoryPathForCommand(context.Background(), []string{command, "run-184"}, nil, "git", discover)
		if err != nil || path != "" || calls != 1 {
			t.Fatalf("%s path=%q err=%v calls=%d", command, path, err, calls)
		}
	}
	for _, command := range []string{"resume", "confirm", "create-revert"} {
		path, err = repositoryPathForCommand(context.Background(), []string{command, "run-184"}, nil, "git", discover)
		if err != nil || path != "/workspace/repo" {
			t.Fatalf("%s path=%q err=%v", command, path, err)
		}
	}
	if calls != 4 {
		t.Fatalf("discovery calls=%d, want four", calls)
	}
}

func TestRepositoryDiscoveryPropagatesStartFailure(t *testing.T) {
	want := errors.New("not a git checkout")
	discover := func(context.Context, runner.Runner, string) (string, error) {
		return "", want
	}
	if _, err := repositoryPathForCommand(context.Background(), []string{"start", "contract.json"}, nil, "git", discover); !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}

func TestRetireUsesSnapshotWiringWithoutRepositoryDiscoveryOrGHESCredential(t *testing.T) {
	if requiresGHESCredential([]string{"retire", "run-184"}) {
		t.Fatal("retire must not require a GHES token")
	}
	var calls int
	path, err := repositoryPathForCommand(context.Background(), []string{"retire", "run-184"}, nil, "git", func(context.Context, runner.Runner, string) (string, error) {
		calls++
		return "/wrong/current/checkout", nil
	})
	if err != nil || path != "" || calls != 0 {
		t.Fatalf("retire repository discovery path=%q err=%v calls=%d", path, err, calls)
	}
}

func TestPersistedRepositoryCommandsUseSnapshotPath(t *testing.T) {
	store := state.NewStore(t.TempDir())
	snapshot := state.RunSnapshot{RunID: "persisted-repository", Phase: contract.PhaseCompleted, RepositoryPath: "/snapshot/repository"}
	if err := store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"retire", "cleanup"} {
		got, err := repositoryPathForPersistedCommand(context.Background(), []string{command, string(snapshot.RunID)}, store, "")
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if got != snapshot.RepositoryPath {
			t.Fatalf("%s repository=%q, want %q", command, got, snapshot.RepositoryPath)
		}
	}
}

func TestProductionDependenciesUseSnapshotRepositoryAndCompositeRetirementRoots(t *testing.T) {
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	stateDir := t.TempDir()
	herdrRoot := filepath.Join(t.TempDir(), "herdr-worktrees")
	if err := os.MkdirAll(herdrRoot, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	configData, err := json.Marshal(map[string]any{
		"ghesHost": "https://github.example.test", "stateDir": stateDir,
		"herdrWorktreeRoot": herdrRoot, "projectId": "PVT_1", "projectStatusFieldId": "PVTSSF_1",
		"projectStatusOptions": map[string]string{"Backlog": "opt-1", "Ready": "opt-2", "In Progress": "opt-3", "Review": "opt-4", "Done": "opt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREADDOCK_CONFIG", configPath)
	store := state.NewStore(stateDir)
	snapshot := state.RunSnapshot{RunID: "retire-wiring", Phase: contract.PhaseCompleted, RepositoryPath: "/snapshot/repository"}
	if err := store.Create(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	deps, err := productionDependencies([]string{"retire", string(snapshot.RunID)})
	if err != nil {
		t.Fatal(err)
	}
	service, ok := deps.Retirement.(*cli.OrchestratorRunService)
	if !ok {
		t.Fatalf("retirement service=%T", deps.Retirement)
	}
	runtime, err := service.RetirementRuntime(context.Background(), snapshot.RunID)
	if err != nil {
		t.Fatal(err)
	}
	wantManaged := filepath.Join(stateDir, "worktrees")
	if runtime.RepositoryPath != snapshot.RepositoryPath || runtime.ManagedRoot != wantManaged || runtime.HerdrRoot != filepath.Clean(herdrRoot) || !runtime.Composite {
		t.Fatalf("runtime=%#v want repo=%q managed=%q herdr=%q composite=true", runtime, snapshot.RepositoryPath, wantManaged, herdrRoot)
	}
}

func TestProductionWorkflowDependenciesUseStateDirWithoutLegacySetup(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("THREADDOCK_STATE_DIR", stateDir)
	t.Setenv("THREADDOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	deps, err := productionWorkflowDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if deps.Workflow == nil || deps.Runs != nil || deps.Confirmer != nil || deps.Reverter != nil {
		t.Fatalf("workflow deps=%#v", deps)
	}
	project := registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	if _, err := deps.Workflow.RegisterProject(context.Background(), project, 0, "request-project"); err != nil {
		t.Fatal(err)
	}
	projects, err := deps.Workflow.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].ProjectID != project.ProjectID {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "v2", "projects", "project-1.json")); err != nil {
		t.Fatalf("v2 project state was not written under THREADDOCK_STATE_DIR: %v", err)
	}
}

func TestProductionWorkflowDependenciesStatusStaysProviderNeutralWithoutConfig(t *testing.T) {
	t.Setenv("THREADDOCK_STATE_DIR", t.TempDir())
	t.Setenv("THREADDOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	deps, err := productionWorkflowDependencies([]string{"work", "status", "work-1", "--json"})
	if err != nil || deps.Workflow == nil {
		t.Fatalf("deps=%#v err=%v", deps, err)
	}
	if _, ok := deps.Workflow.(interface {
		RunWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	}); ok {
		t.Fatal("status unexpectedly constructed runtime runner")
	}
}

func TestProductionWorkflowDependenciesPublishIssuesRequiresToken(t *testing.T) {
	t.Setenv("THREADDOCK_STATE_DIR", t.TempDir())
	t.Setenv("THREADDOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	args := []string{"work", "publish-issues", "work-1", "parent-draft", "--expected-revision", "1", "--request-id", "request-1"}
	deps, err := productionWorkflowDependencies(args)
	if err == nil || deps.Workflow != nil || !strings.Contains(err.Error(), "THREADDOCK_GH_TOKEN") || strings.Contains(err.Error(), "missing-config") {
		t.Fatalf("deps=%#v err=%v", deps, err)
	}
}

func TestProductionWorkflowDependenciesPublishIssuesComposesPrivatePublisher(t *testing.T) {
	stateDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.json")
	configData := `{"ghesHost":"https://github.example.test","stateDir":"` + stateDir + `","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREADDOCK_STATE_DIR", stateDir)
	t.Setenv("THREADDOCK_CONFIG", configPath)
	t.Setenv("THREADDOCK_GH_TOKEN", "composition-token-sentinel")
	args := []string{"work", "publish-issues", "work-1", "parent-draft", "--expected-revision", "1", "--request-id", "request-1"}
	deps, err := productionWorkflowDependencies(args)
	if err != nil {
		t.Fatal(err)
	}
	if deps.Runs != nil || deps.Confirmer != nil || deps.Reverter != nil {
		t.Fatalf("publish deps unexpectedly include legacy/runtime services: %#v", deps)
	}
	publisher, ok := deps.Workflow.(interface {
		PublishParentIssue(context.Context, contractv2.WorkID, string, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	})
	if !ok || publisher == nil {
		t.Fatalf("publish workflow capability=%T", deps.Workflow)
	}
	if _, ok := deps.Workflow.(*productionParentIssuePublisher); !ok {
		t.Fatalf("publish workflow is not private production bridge: %T", deps.Workflow)
	}
}

func TestMalformedPublishIssuesDoesNotEnterProductionPublicationBranch(t *testing.T) {
	t.Setenv("THREADDOCK_STATE_DIR", t.TempDir())
	t.Setenv("THREADDOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	malformed := []string{"work", "publish-issues", "work-1", "parent-draft", "--expected-revision", "0", "--request-id", "request-1"}
	deps, err := productionWorkflowDependencies(malformed)
	if err != nil || deps.Workflow == nil {
		t.Fatalf("deps=%#v err=%v", deps, err)
	}
	if _, ok := deps.Workflow.(*productionParentIssuePublisher); ok {
		t.Fatal("malformed publish unexpectedly composed provider publication")
	}
}

type productionPublicationHTTP struct {
	mu             sync.Mutex
	token          string
	issues         []github.Issue
	postedBody     string
	getCalls       int
	postCalls      int
	authorization  bool
	postStarted    chan struct{}
	unblockPost    chan struct{}
	responseMode   string
	postWasInvalid bool
}

func (h *productionPublicationHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.authorization = r.Header.Get("Authorization") == "Bearer "+h.token
	if r.URL.Path != "/api/v3/repos/acme/app/issues" {
		h.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.getCalls++
		issues := append([]github.Issue(nil), h.issues...)
		h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(issues)
	case http.MethodPost:
		h.postCalls++
		var payload struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		data, err := io.ReadAll(r.Body)
		if err == nil {
			err = json.Unmarshal(data, &payload)
		}
		h.postedBody = payload.Body
		mode := h.responseMode
		started, unblock := h.postStarted, h.unblockPost
		if mode == "lost" {
			h.issues = []github.Issue{{Number: 17, NodeID: "node-17", HTMLURL: "https://github.example.test/issues/17", Title: payload.Title, Body: payload.Body}}
		} else if mode == "multiple" {
			h.issues = []github.Issue{{Number: 17, NodeID: "node-17", HTMLURL: "https://github.example.test/issues/17", Title: payload.Title, Body: payload.Body}, {Number: 18, NodeID: "node-18", HTMLURL: "https://github.example.test/issues/18", Title: payload.Title, Body: payload.Body}}
		} else if mode == "mismatch" {
			prefixEnd := strings.Index(payload.Body, ":sha256=")
			if prefixEnd >= 0 {
				prefix := payload.Body[:prefixEnd+len(":sha256=")]
				h.issues = []github.Issue{{Number: 19, NodeID: "node-19", HTMLURL: "https://github.example.test/issues/19", Title: payload.Title, Body: prefix + strings.Repeat("0", 64) + " -->"}}
			}
		}
		h.mu.Unlock()
		if err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if started != nil {
			close(started)
		}
		if unblock != nil {
			<-unblock
		}
		if mode == "absent" || mode == "lost" || mode == "multiple" || mode == "mismatch" {
			h.mu.Lock()
			h.postWasInvalid = true
			h.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"providerBody": "provider-body-sentinel"})
			return
		}
		_ = json.NewEncoder(w).Encode(github.Issue{Number: 17, NodeID: "node-17", HTMLURL: "https://github.example.test/issues/17", Title: payload.Title, Body: payload.Body})
	default:
		h.mu.Unlock()
		http.NotFound(w, r)
	}
}

func (h *productionPublicationHTTP) counts() (get, post int, auth bool, invalid bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.getCalls, h.postCalls, h.authorization, h.postWasInvalid
}

func TestProductionPublishIssuesPendingReplayAndConflict(t *testing.T) {
	httpState := &productionPublicationHTTP{token: "publish-token", postStarted: make(chan struct{}), unblockPost: make(chan struct{}), responseMode: "pending"}
	server := httptest.NewServer(httpState)
	defer server.Close()
	works := seedProductionPublicationFixture(t, server.URL, "publish-token")
	args := []string{"work", "publish-issues", "work-publish", "parent-a", "--expected-revision", "2", "--request-id", "request-publish"}
	deps, err := productionWorkflowDependencies(args)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- cli.RunWithDependencies(context.Background(), args, &out, &errOut, deps) }()
	<-httpState.postStarted
	pending, err := works.Load(context.Background(), "work-publish")
	if err != nil {
		t.Fatal(err)
	}
	publication := pending.Publications["parent-issue:request-publish"]
	if publication.Status != statev2.PublicationPending || pending.State != statev2.StatePublicationPending {
		t.Fatalf("pending state=%q publication=%q", pending.State, publication.Status)
	}
	close(httpState.unblockPost)
	if code := <-done; code != 0 || out.Len() == 0 || errOut.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	completed, err := works.Load(context.Background(), "work-publish")
	if err != nil {
		t.Fatal(err)
	}
	_, posts, _, _ := httpState.counts()
	if completed.Publications["parent-issue:request-publish"].Status != statev2.PublicationCompleted || completed.Publications["parent-issue:request-publish"].Receipt == nil || posts != 1 {
		t.Fatalf("completed publication=%+v posts=%d", completed.Publications["parent-issue:request-publish"], posts)
	}
	var firstSnapshot statev2.WorkSnapshot
	if err := json.Unmarshal(out.Bytes(), &firstSnapshot); err != nil {
		t.Fatal(err)
	}
	firstReceipt := firstSnapshot.Publications["parent-issue:request-publish"].Receipt
	getBeforeReplay, postBeforeReplay, auth, _ := httpState.counts()
	if !auth {
		t.Fatal("server did not receive the expected Authorization header")
	}
	deps, err = productionWorkflowDependencies(args)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := cli.RunWithDependencies(context.Background(), args, &out, &errOut, deps); code != 0 || out.Len() == 0 || errOut.Len() != 0 {
		t.Fatalf("replay code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var replaySnapshot statev2.WorkSnapshot
	if err := json.Unmarshal(out.Bytes(), &replaySnapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replaySnapshot.Publications["parent-issue:request-publish"].Receipt, firstReceipt) {
		t.Fatalf("replay receipt=%+v first=%+v", replaySnapshot.Publications["parent-issue:request-publish"].Receipt, firstReceipt)
	}
	getAfterReplay, postAfterReplay, _, _ := httpState.counts()
	if getAfterReplay != getBeforeReplay || postAfterReplay != postBeforeReplay {
		t.Fatalf("replay I/O changed GET %d->%d POST %d->%d", getBeforeReplay, getAfterReplay, postBeforeReplay, postAfterReplay)
	}
	priorReceipt := completed.Publications["parent-issue:request-publish"].Receipt
	deps, err = productionWorkflowDependencies([]string{"work", "publish-issues", "work-publish", "parent-b", "--expected-revision", "3", "--request-id", "request-publish"})
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	conflictArgs := []string{"work", "publish-issues", "work-publish", "parent-b", "--expected-revision", "3", "--request-id", "request-publish"}
	if code := cli.RunWithDependencies(context.Background(), conflictArgs, &out, &errOut, deps); code != 1 || out.Len() != 0 || errOut.String() != "프로젝트·워크플로 명령을 처리하지 못했습니다.\n" {
		t.Fatalf("conflict code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	conflicted, err := works.Load(context.Background(), "work-publish")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(conflicted.Publications["parent-issue:request-publish"].Receipt, priorReceipt) || conflicted.Publications["parent-issue:request-publish"].Status != statev2.PublicationCompleted {
		t.Fatalf("prior receipt changed: %+v", conflicted.Publications["parent-issue:request-publish"])
	}
	getAfterConflict, postAfterConflict, _, _ := httpState.counts()
	if getAfterConflict != getAfterReplay || postAfterConflict != postAfterReplay {
		t.Fatalf("conflict performed I/O GET=%d POST=%d", getAfterConflict, postAfterConflict)
	}
}

func TestProductionPublishIssuesLostResponseAdoptsExactMarker(t *testing.T) {
	httpState := &productionPublicationHTTP{token: "publish-token", responseMode: "lost"}
	server := httptest.NewServer(httpState)
	defer server.Close()
	works := seedProductionPublicationFixture(t, server.URL, "publish-token")
	args := []string{"work", "publish-issues", "work-publish", "parent-a", "--expected-revision", "2", "--request-id", "request-lost"}
	deps, err := productionWorkflowDependencies(args)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := cli.RunWithDependencies(context.Background(), args, &out, &errOut, deps); code != 0 || out.Len() == 0 || errOut.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	snapshot, err := works.Load(context.Background(), "work-publish")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Publications["parent-issue:request-lost"].Status != statev2.PublicationCompleted || snapshot.Publications["parent-issue:request-lost"].Receipt == nil {
		t.Fatalf("publication=%+v", snapshot.Publications["parent-issue:request-lost"])
	}
	gets, posts, auth, invalid := httpState.counts()
	if posts != 1 || !auth || !invalid || strings.Contains(out.String(), "provider-body-sentinel") || strings.Contains(errOut.String(), "provider-body-sentinel") {
		t.Fatalf("GET=%d POST=%d auth=%t invalid=%t stdout=%q stderr=%q", gets, posts, auth, invalid, out.String(), errOut.String())
	}
}

func TestProductionPublishIssuesAmbiguousOutcomeBlocksWithoutRepublish(t *testing.T) {
	for _, mode := range []string{"absent", "multiple", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			httpState := &productionPublicationHTTP{token: "publish-token", responseMode: mode}
			server := httptest.NewServer(httpState)
			defer server.Close()
			works := seedProductionPublicationFixture(t, server.URL, "publish-token")
			args := []string{"work", "publish-issues", "work-publish", "parent-a", "--expected-revision", "2", "--request-id", "request-ambiguous"}
			deps, err := productionWorkflowDependencies(args)
			if err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			if code := cli.RunWithDependencies(context.Background(), args, &out, &errOut, deps); code != 1 || out.Len() != 0 || errOut.String() != "프로젝트·워크플로 명령을 처리하지 못했습니다.\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
			snapshot, err := works.Load(context.Background(), "work-publish")
			if err != nil {
				t.Fatal(err)
			}
			publication := snapshot.Publications["parent-issue:request-ambiguous"]
			if publication.Status != statev2.PublicationConflict || snapshot.Control.Blocker == nil || snapshot.Control.Blocker.Kind != statev2.BlockerKindPublicationConflict || snapshot.Control.Blocker.IntentID != publication.IntentID || snapshot.Control.Blocker.Diagnostic == "" || strings.Contains(snapshot.Control.Blocker.Diagnostic, "provider-body-sentinel") {
				t.Fatalf("snapshot publication=%+v blocker=%+v", publication, snapshot.Control.Blocker)
			}
			gets, posts, auth, invalid := httpState.counts()
			if posts != 1 || !auth || !invalid {
				t.Fatalf("GET=%d POST=%d auth=%t invalid=%t", gets, posts, auth, invalid)
			}
		})
	}
}

func seedProductionPublicationFixture(t *testing.T, apiBase, token string) statev2.Store {
	t.Helper()
	root := t.TempDir()
	projects := registry.NewStore(root)
	works := statev2.NewStore(root)
	project := registry.Project{ProjectID: "project-publish", Name: "Publish", PrimaryRepoKey: "primary", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"primary": {Host: apiBase, Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	projectBytes, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	projectSum := sha256.Sum256(projectBytes)
	if _, err := projects.Create(context.Background(), project, 0, "request-project", hex.EncodeToString(projectSum[:])); err != nil {
		t.Fatal(err)
	}
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-publish", ProjectID: project.ProjectID, Revision: 1, Request: "publish", AcceptanceCriteria: []string{"publish"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "primary", BaseSHA: strings.Repeat("a", 40), TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-publish", RepoKey: "primary", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"publish"}}}, IssueDrafts: []contractv2.IssueDraft{{Key: "parent-a", Title: "Parent A", Body: "Body A", RepoKey: "primary"}, {Key: "parent-b", Title: "Parent B", Body: "Body B", RepoKey: "primary"}}, Documentation: contractv2.DocumentationPlan{Reason: "not needed"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
	var canonical bytes.Buffer
	if err := contractv2.Write(&canonical, contract); err != nil {
		t.Fatal(err)
	}
	contractSum := sha256.Sum256(canonical.Bytes())
	initial := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: project.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: contract, ContractHash: hex.EncodeToString(contractSum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, Control: statev2.WorkControl{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-publish": {TaskID: "task-publish", Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	if _, err := works.CreatePlan(context.Background(), initial, "request-plan", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	approval := statev2.TransitionRequest{WorkID: contract.WorkID, ExpectedRevision: 1, RequestID: "request-approve", Work: &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: initial.ContractHash}}
	approval.PayloadHash, err = statev2.TransitionPayloadHash(approval)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := works.Apply(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	configData := `{"ghesHost":"` + apiBase + `","apiBase":"` + apiBase + `/api/v3","stateDir":"` + root + `","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREADDOCK_STATE_DIR", root)
	t.Setenv("THREADDOCK_CONFIG", configPath)
	t.Setenv("THREADDOCK_GH_TOKEN", token)
	return works
}

func TestProductionWorkflowDependenciesRuntimeBindingAndRequiredProfiles(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	if result, err := (runner.OSRunner{}).Run(context.Background(), "", "git", "init", repository); err != nil || result.ExitCode != 0 {
		t.Fatalf("git init exit=%d err=%v", result.ExitCode, err)
	}
	stateRoot := filepath.Join(root, "state")
	if err := os.Mkdir(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	workContract := contractv2.WorkItemContract{Version: 2, WorkID: "work-production", ProjectID: "project-production", Revision: 1, Request: "run", AcceptanceCriteria: []string{"done"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "repo", BaseSHA: base, TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-production", RepoKey: "repo", Branch: "agent/task-production", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"done"}}}, Documentation: contractv2.DocumentationPlan{Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder-logical", Reviewer: "reviewer", Documenter: "documenter"}}
	var encoded bytes.Buffer
	if err := contractv2.Write(&encoded, workContract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	store := statev2.NewStore(stateRoot)
	initial := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: workContract.ProjectID, WorkID: workContract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: workContract, ContractHash: hex.EncodeToString(sum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-production": {TaskID: "task-production", Status: statev2.TaskPending, RepairLimit: 2, RecoveryLimit: 1, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	if _, err := store.CreatePlan(context.Background(), initial, "plan-production", "plan-hash"); err != nil {
		t.Fatal(err)
	}
	approval := statev2.TransitionRequest{WorkID: workContract.WorkID, ExpectedRevision: 1, RequestID: "approve-production", Work: &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "approval", ContractHash: initial.ContractHash}}
	approval.PayloadHash, _ = statev2.TransitionPayloadHash(approval)
	if _, err := store.Apply(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	configData := `{"ghesHost":"https://github.example.test","stateDir":"` + stateRoot + `","openCodeAgents":{"builder":"builder-native"},"builderRuntimeFingerprint":"  fingerprint-production  ","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREADDOCK_STATE_DIR", stateRoot)
	t.Setenv("THREADDOCK_CONFIG", configPath)
	t.Setenv("THREADDOCK_GH_TOKEN", "")
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	runArgs := []string{"work", "run", string(workContract.WorkID), "--expected-revision", "2", "--request-id", "request-production"}
	deps, err := productionWorkflowDependencies(runArgs)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deps.Workflow.(interface {
		RunWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
	}); !ok {
		t.Fatal("run workflow lacks runner capability")
	}
	if _, ok := deps.Workflow.(interface {
		ReconcileWork(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error)
	}); !ok {
		t.Fatal("run workflow lacks reconcile capability")
	}
	reconcileDeps, err := productionWorkflowDependencies([]string{"work", "reconcile", string(workContract.WorkID), "--json"})
	if err != nil || reconcileDeps.Workflow == nil {
		t.Fatalf("reconcile deps=%#v err=%v", reconcileDeps, err)
	}
	for _, data := range []string{
		strings.Replace(configData, `,"builderRuntimeFingerprint":"  fingerprint-production  "`, "", 1),
		strings.Replace(configData, `,"openCodeAgents":{"builder":"builder-native"}`, "", 1),
	} {
		badPath := filepath.Join(root, "bad-config.json")
		if err := os.WriteFile(badPath, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("THREADDOCK_CONFIG", badPath)
		if _, err := productionWorkflowDependencies(runArgs); err == nil || strings.Contains(err.Error(), "fingerprint-production") {
			t.Fatalf("bad runtime config err=%v", err)
		}
	}
}
