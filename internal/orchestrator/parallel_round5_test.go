package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/herdr"
	"thread-dock/internal/state"
)

func TestParallelPromptRotationUsesOneMonotonicTaskGeneration(t *testing.T) {
	snapshot := state.RunSnapshot{RunID: "run-1788136436747506769-1"}
	task := state.TaskRunState{Prompt: state.PromptReceipt{RequestID: "run-1788136436747506769-1:api:prompt"}}
	for generation := 1; generation <= 3; generation++ {
		if err := rotateParallelPrompt(&snapshot, "api", &task); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("run-1788136436747506769-1:api:attempt-%d", generation)
		if task.PromptGeneration != generation || task.Prompt.RequestID != want {
			t.Fatalf("rotation %d: generation=%d requestID=%q want %q", generation, task.PromptGeneration, task.Prompt.RequestID, want)
		}
	}
}

func TestParallelPromptRotationRejectsCollisionWithoutPanicking(t *testing.T) {
	snapshot := state.RunSnapshot{RunID: "run-1788136436747506769-1"}
	task := state.TaskRunState{Prompt: state.PromptReceipt{RequestID: "run-1788136436747506769-1:api:attempt-1"}}
	if err := rotateParallelPrompt(&snapshot, "api", &task); err == nil {
		t.Fatal("rotation unexpectedly accepted duplicate request ID")
	}
}

func TestParallelLegacyGenerationZeroUsesInitialPromptID(t *testing.T) {
	h := newParallelHarness(t)
	snapshot := state.RunSnapshot{RunID: "run-1788136436747506769-1", Phase: contract.PhaseBuilding, Tasks: map[string]state.TaskRunState{}}
	task := state.TaskRunState{
		Agent:    state.AgentEvidence{Name: "builder-api"},
		Worktree: state.WorktreeState{Path: "/tmp/agent-api", WorkspaceID: "workspace-agent-api", PaneID: "pane-agent-api"},
		Prompt:   state.PromptReceipt{},
		// A legacy snapshot may carry a repair counter but has no prompt generation.
		RepairCount: 2,
	}
	if err := h.orchestrator.parallelBaselineTask(context.Background(), &snapshot, "api", task); err != nil {
		t.Fatal(err)
	}
	got := snapshot.Tasks["api"]
	if got.PromptGeneration != 0 || got.Prompt.RequestID != "run-1788136436747506769-1:api:prompt" {
		t.Fatalf("legacy baseline prompt=%q generation=%d", got.Prompt.RequestID, got.PromptGeneration)
	}
}

func TestParallelBaselinePreservesRotatedPromptIDAndGeneration(t *testing.T) {
	h := newParallelHarness(t)
	snapshot := state.RunSnapshot{RunID: "run-1788136436747506769-1", Phase: contract.PhaseBuilding, Tasks: map[string]state.TaskRunState{}}
	wantID := "run-1788136436747506769-1:api:attempt-2"
	task := state.TaskRunState{
		Agent:    state.AgentEvidence{Name: "builder-api"},
		Worktree: state.WorktreeState{Path: "/tmp/agent-api", WorkspaceID: "workspace-agent-api", PaneID: "pane-agent-api"},
		Prompt:   state.PromptReceipt{RequestID: wantID, BaselineSeq: 7}, PromptGeneration: 2,
		PreviousRequestID: "run-1788136436747506769-1:api:attempt-1",
	}
	if err := h.orchestrator.parallelBaselineTask(context.Background(), &snapshot, "api", task); err != nil {
		t.Fatal(err)
	}
	got := snapshot.Tasks["api"]
	if got.Prompt.RequestID != wantID || got.PromptGeneration != 2 || got.Prompt.BaselineSeq == 7 {
		t.Fatalf("baseline changed prompt identity: requestID=%q generation=%d baseline=%d", got.Prompt.RequestID, got.PromptGeneration, got.Prompt.BaselineSeq)
	}
}

func TestParallelPendingBaselinePreservesRotatedPromptIDAndGeneration(t *testing.T) {
	h := newParallelHarness(t)
	snapshot := state.RunSnapshot{RunID: "run-1788136436747506769-1", Phase: contract.PhaseBuilding, Tasks: map[string]state.TaskRunState{}}
	wantID := "run-1788136436747506769-1:api:attempt-2"
	snapshot.Tasks["api"] = state.TaskRunState{
		Agent:    state.AgentEvidence{Name: "builder-api"},
		Worktree: state.WorktreeState{Path: "/tmp/agent-api", WorkspaceID: "workspace-agent-api", PaneID: "pane-agent-api"},
		Prompt:   state.PromptReceipt{RequestID: wantID, BaselineSeq: 7}, PromptGeneration: 2,
	}
	if err := h.orchestrator.reconcileParallelTaskBaseline(context.Background(), &snapshot, "api"); err != nil {
		t.Fatal(err)
	}
	got := snapshot.Tasks["api"]
	if got.Prompt.RequestID != wantID || got.PromptGeneration != 2 || got.Prompt.BaselineSeq == 7 {
		t.Fatalf("pending baseline changed prompt identity: requestID=%q generation=%d baseline=%d", got.Prompt.RequestID, got.PromptGeneration, got.Prompt.BaselineSeq)
	}
}

type projectReaderClient struct {
	*parallelGitHub
	reader github.ProjectStatusReader
}

func (c *projectReaderClient) ReadProjectStatus(ctx context.Context, project github.ProjectRef, issueNodeID string) (github.ProjectStatus, error) {
	return c.reader.ReadProjectStatus(ctx, project, issueNodeID)
}

func TestParallelPageTwoProjectObservationDoesNotAddDuplicateItem(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.ProjectAutomationEnabled = true
	h.Deps.Project = github.ProjectRef{ID: "PVT_1", StatusFieldID: "F_1", StatusOptions: map[string]string{"In Progress": "progress"}}
	h.orchestrator = NewParallel(h.Deps)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	snapshot.ProjectStatus = ""
	snapshot.Registration.NodeID = "issue-node"
	h.parallelGH.projectObservation = &github.ProjectStatus{ItemPresent: true, ItemFound: true, StatusPresent: true, StatusFound: true, ItemID: "ITEM_PAGE_2", Status: "Review"}
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "parallel_project_update_in_progress" || got.ProjectItemID != "ITEM_PAGE_2" || h.parallelGH.projectAddCalls != 0 {
		t.Fatalf("page-two observation snapshot=%+v addCalls=%d", got, h.parallelGH.projectAddCalls)
	}
}

func TestParallelRESTPageTwoObservationDoesNotAddDuplicateItem(t *testing.T) {
	h := newParallelHarness(t)
	projectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload.Query, "ProjectItemFields") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"node": map[string]any{"fieldValues": map[string]any{"nodes": []any{}}}}})
			return
		}
		if payload.Variables["cursor"] == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"node": map[string]any{"items": map[string]any{"nodes": []any{map[string]any{"id": "OTHER", "content": map[string]string{"id": "OTHER_ISSUE"}}}, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "items-cursor-1"}}}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"node": map[string]any{"items": map[string]any{"nodes": []any{map[string]any{"id": "ITEM_PAGE_2", "content": map[string]string{"id": "issue-node"}, "fieldValues": map[string]any{"nodes": []any{map[string]string{"name": "Review", "optionId": "O_REVIEW", "fieldId": "F_1"}}, "pageInfo": map[string]any{"hasNextPage": false}}}}, "pageInfo": map[string]any{"hasNextPage": false}}}}})
	}))
	defer projectServer.Close()
	reader := github.NewRESTClient(projectServer.URL, "token", "2022-11-28", projectServer.Client())
	client := &projectReaderClient{parallelGitHub: h.parallelGH, reader: reader}
	h.Deps.GitHub = client
	h.Deps.ProjectAutomationEnabled = true
	h.Deps.Project = github.ProjectRef{ID: "PVT_1", StatusFieldID: "F_1", StatusOptions: map[string]string{"In Progress": "O_PROGRESS", "Review": "O_REVIEW"}}
	h.orchestrator = NewParallel(h.Deps)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	snapshot.ProjectStatus = ""
	snapshot.Registration.NodeID = "issue-node"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "parallel_project_update_in_progress" || got.ProjectItemID != "ITEM_PAGE_2" || h.parallelGH.projectAddCalls != 0 {
		t.Fatalf("REST page-two observation snapshot=%+v addCalls=%d", got, h.parallelGH.projectAddCalls)
	}
}

func TestParallelResumeUsesExactProviderSessionAndReconcilesWithoutDuplicate(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	name := taskAgentName("builder", id, "api")
	session := "session-" + name
	task := snapshot.Tasks["api"]
	task.State, task.Stage = "running", "resume"
	task.Agent = state.AgentEvidence{Name: name, SessionID: session, IdentitySource: "provider"}
	task.NativeResume = true
	task.Worktree = state.WorktreeState{Path: "/tmp/agent-api", WorkspaceID: "workspace-agent-api", PaneID: "pane-agent-api", Branch: "agent/api"}
	snapshot.Phase = contract.PhaseBuilding
	snapshot.Tasks["api"] = task
	if err := h.orchestrator.parallelResumeTask(context.Background(), &snapshot, "api", task); err != nil {
		t.Fatal(err)
	}
	if len(h.parallelHD.resumes) != 1 || h.parallelHD.resumes[0].Name != name || h.parallelHD.resumes[0].PaneID != task.Worktree.PaneID || h.parallelHD.resumes[0].SessionID != session {
		t.Fatalf("resume requests=%+v, want exact provider session", h.parallelHD.resumes)
	}

	// Model a crash after the provider resume but before the post-effect save:
	// the durable intent is still pending, while Herdr reports the same
	// provider session. Reconciliation must not issue a second resume.
	snapshot = h.mustLoad(id)
	snapshot.PendingAction = parallelTaskAction("parallel_resume_agent_", "api")
	snapshot.PendingTaskID = "api"
	snapshot.Tasks["api"] = task
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	resumesBefore := len(h.parallelHD.resumes)
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id).Tasks["api"]
	if len(h.parallelHD.resumes) != resumesBefore || got.Stage != "baseline" || got.Agent.SessionID != session {
		t.Fatalf("resume reconciliation resumes=%d task=%+v", len(h.parallelHD.resumes), got)
	}
}

func TestParallelTerminalIdentityCannotResumeProviderSession(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	task := snapshot.Tasks["api"]
	task.State, task.Stage = "running", "resume"
	task.Agent = state.AgentEvidence{Name: taskAgentName("builder", id, "api"), SessionID: "herdr-terminal:terminal-builder", IdentitySource: "terminal"}
	// Even if a stale snapshot incorrectly carries NativeResume=true, a
	// terminal fallback identity must never be sent to the provider resume
	// port.
	task.NativeResume = true
	task.Worktree = state.WorktreeState{Path: "/tmp/agent-api", WorkspaceID: "workspace-agent-api", PaneID: "pane-agent-api", Branch: "agent/api"}
	snapshot.Phase = contract.PhaseBuilding
	snapshot.Tasks["api"] = task
	if err := h.orchestrator.parallelResumeTask(context.Background(), &snapshot, "api", task); err != nil {
		t.Fatal(err)
	}
	if snapshot.Phase != contract.PhaseNeedsOperator || len(h.parallelHD.resumes) != 0 {
		t.Fatalf("terminal resume phase=%s requests=%+v", snapshot.Phase, h.parallelHD.resumes)
	}
}

type branchLookupHerdr struct {
	*parallelHerdr
	cwd    string
	branch string
	label  string
}

func (h *branchLookupHerdr) FindWorktreeByBranch(_ context.Context, cwd, branch, label string) (herdr.Worktree, bool, error) {
	h.cwd, h.branch, h.label = cwd, branch, label
	return herdr.Worktree{WorkspaceID: "workspace-canonical", PaneID: "pane-canonical", Path: "/managed/canonical-api"}, true, nil
}

func TestParallelBuilderWorktreeBranchLocatorAdoptsExactIdentity(t *testing.T) {
	h := newParallelHarness(t)
	branchHerdr := &branchLookupHerdr{parallelHerdr: h.parallelHD}
	h.Deps.Herdr = branchHerdr
	h.orchestrator = NewParallel(h.Deps)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	task := snapshot.Tasks["api"]
	task.State = "pending"
	task.Worktree = state.WorktreeState{}
	task.ExpectedBranch = "agent/api"
	task.ExpectedLabel = "threaddock-" + string(id) + "-api"
	task.ExpectedBaseCommit = "0123456789abcdef0123456789abcdef01234567"
	snapshot.Phase = contract.PhaseBuilding
	snapshot.PendingAction = parallelTaskAction("parallel_create_worktree_", "api")
	snapshot.PendingTaskID = "api"
	snapshot.Tasks["api"] = task
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id).Tasks["api"]
	if branchHerdr.cwd != snapshot.RepositoryPath || branchHerdr.branch != task.ExpectedBranch || branchHerdr.label != task.ExpectedLabel || got.Worktree.Path != "/managed/canonical-api" || got.Worktree.WorkspaceID != "workspace-canonical" || got.Worktree.PaneID != "pane-canonical" || got.Worktree.Branch != task.ExpectedBranch {
		t.Fatalf("lookup cwd=%q branch=%q label=%q task=%+v", branchHerdr.cwd, branchHerdr.branch, branchHerdr.label, got)
	}
}
