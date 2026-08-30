package orchestrator

import (
	"context"
	"errors"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/github"
	"thread-dock/internal/state"
	"thread-dock/internal/testfixture"
)

func TestParallelBuilderWorktreeReconcileAdoptsCanonicalPathWhenPreEffectPathIsEmpty(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	task := snapshot.Tasks["api"]
	task.State = "pending"
	// A crash can leave both the path and the derived Worktree branch empty;
	// ExpectedBranch is the durable identity used for reconciliation.
	task.Worktree = state.WorktreeState{}
	task.ExpectedBranch = "agent/api"
	task.ExpectedLabel = "threaddock-" + string(id) + "-api"
	task.ExpectedBaseCommit = testfixture.ValidContract().BaseCommit
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
	if got.State != "running" || got.Stage != "start" || got.Worktree.Path != "/tmp/integration" || got.Worktree.WorkspaceID == "" || got.Worktree.PaneID == "" || got.Worktree.Branch != "agent/api" {
		t.Fatalf("reconciled task = %+v", got)
	}
	if h.herdr.findWorktreeSelector != "agent/api" || h.herdr.findWorktreeLabel != task.ExpectedLabel {
		t.Fatalf("lookup selector=%q label=%q, want branch=%q label=%q", h.herdr.findWorktreeSelector, h.herdr.findWorktreeLabel, "agent/api", task.ExpectedLabel)
	}
}

func TestParallelBuilderWorktreeIntentPersistsExpectedIdentityBeforeCreate(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseBuilding
	runtime := h.orchestrator.runs[id]
	h.parallelHD.createErr = errors.New("provider unavailable")
	if err := h.orchestrator.createParallelTaskWorktree(context.Background(), &snapshot, runtime, "api"); err == nil {
		t.Fatal("create unexpectedly succeeded")
	}
	got := h.mustLoad(id).Tasks["api"]
	if got.Worktree.Branch != got.ExpectedBranch || got.ExpectedBranch != "agent/api" || got.ExpectedLabel == "" || got.ExpectedBaseCommit == "" || got.Worktree.Path != "" {
		t.Fatalf("pre-effect worktree identity = %+v", got)
	}
}

func TestParallelProjectObservationPlansUpdateForItemWithoutStatus(t *testing.T) {
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
	snapshot.PendingAction = ""
	h.parallelGH.projectItemID = "ITEM_PROJECT"
	h.parallelGH.projectStatus = ""
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	want := "parallel_project_update_in_progress"
	if got.PendingAction != want || got.ProjectItemID != "ITEM_PROJECT" || h.parallelGH.projectAddCalls != 0 || h.parallelGH.projectUpdateCalls != 0 {
		t.Fatalf("snapshot=%+v add=%d update=%d", got, h.parallelGH.projectAddCalls, h.parallelGH.projectUpdateCalls)
	}
}

func TestParallelProjectMutationPersistsNextObservationBeforeMutation(t *testing.T) {
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
	snapshot.ProjectItemID = ""
	snapshot.PendingAction = "parallel_project_add_in_progress"
	h.parallelGH.projectItemID = ""
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "parallel_project_observe_in_progress" || h.parallelGH.projectAddCalls != 1 || h.parallelGH.projectObserveCalls != 0 {
		t.Fatalf("snapshot=%+v add=%d observe=%d", got, h.parallelGH.projectAddCalls, h.parallelGH.projectObserveCalls)
	}
}

func TestParallelProjectMutationResponseLossIsObservedBeforeRetry(t *testing.T) {
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
	snapshot.PendingAction = "parallel_project_add_in_progress"
	h.parallelGH.projectAddResponseLost = true
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	_ = h.orchestrator.Advance(context.Background(), id)
	if got := h.mustLoad(id); got.PendingAction != "parallel_project_observe_in_progress" {
		t.Fatalf("response-loss pending action = %q", got.PendingAction)
	}
	h.parallelGH.projectAddResponseLost = false
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.parallelGH.projectAddCalls != 1 || h.mustLoad(id).PendingAction != "parallel_project_update_in_progress" {
		t.Fatalf("response-loss reconcile snapshot=%+v add=%d", h.mustLoad(id), h.parallelGH.projectAddCalls)
	}
}

func TestParallelProjectUpdateResponseLossIsObservedBeforeRetry(t *testing.T) {
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
	snapshot.ProjectStatus = "Ready"
	snapshot.Registration.NodeID = "issue-node"
	snapshot.ProjectItemID = "ITEM_PROJECT"
	snapshot.PendingAction = "parallel_project_update_in_progress"
	h.parallelGH.projectItemID, h.parallelGH.projectStatus = "ITEM_PROJECT", "Ready"
	h.parallelGH.projectUpdateResponseLost = true
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	_ = h.orchestrator.Advance(context.Background(), id)
	if got := h.mustLoad(id); got.PendingAction != "parallel_project_observe_in_progress" {
		t.Fatalf("response-loss pending action = %q", got.PendingAction)
	}
	h.parallelGH.projectUpdateResponseLost = false
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if h.parallelGH.projectUpdateCalls != 1 || h.mustLoad(id).ProjectStatus != "In Progress" {
		t.Fatalf("response-loss reconcile snapshot=%+v update=%d", h.mustLoad(id), h.parallelGH.projectUpdateCalls)
	}
}

func TestParallelLegacyProjectStatusPendingMigratesBeforeObservation(t *testing.T) {
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
	snapshot.Registration.NodeID = "issue-node"
	snapshot.ProjectStatus = "Ready"
	snapshot.PendingAction = "parallel_project_status_in_progress"
	h.parallelGH.projectItemID, h.parallelGH.projectStatus = "ITEM_PROJECT", "Ready"
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := h.orchestrator.Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.PendingAction != "parallel_project_observe_in_progress" || h.parallelGH.projectObserveCalls != 0 || h.parallelGH.projectUpdateCalls != 0 {
		t.Fatalf("legacy migration snapshot=%+v observe=%d update=%d", got, h.parallelGH.projectObserveCalls, h.parallelGH.projectUpdateCalls)
	}
}

func TestParallelStaleRepairSHAOnlySchedulesRecoveryObservation(t *testing.T) {
	h := newParallelHarness(t)
	runtime := &runRuntime{contract: testfixture.ValidContract()}
	snapshot := state.RunSnapshot{
		RunID: "run-184", Phase: contract.PhaseBuilding, RepositoryPath: "/repo",
		Integration: state.WorktreeState{Path: "/tmp/integration", Branch: "agent/parent-integration"},
		Tasks: map[string]state.TaskRunState{"api": {
			State: "running", Stage: "evidence", Agent: state.AgentEvidence{Name: "builder-api", CommitSHA: "1111111111111111111111111111111111111111"},
			Worktree: state.WorktreeState{Path: "/tmp/api", WorkspaceID: "workspace-api", PaneID: "pane-api"},
			Prompt:   state.PromptReceipt{RequestID: "run-184:api:repair-1"}, RequiresFreshCommit: true,
			PreviousCommitSHA: "1111111111111111111111111111111111111111",
		}},
	}
	before := h.herdr.getInfoCalls
	callErr := h.orchestrator.parallelCollectTaskEvidence(context.Background(), &snapshot, runtime, "api", runtime.contract.Tasks[0], snapshot.Tasks["api"])
	if callErr != nil {
		t.Fatal(callErr)
	}
	got := snapshot.Tasks["api"]
	if got.Stage != "recovery_observe" || h.herdr.getInfoCalls != before {
		t.Fatalf("stale repair task=%+v getInfo before=%d after=%d", got, before, h.herdr.getInfoCalls)
	}
}

func TestParallelStaleRepairSHAReconcileSchedulesRecoveryWithoutAgentRead(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	task := snapshot.Tasks["api"]
	task.State, task.Stage = "running", "evidence"
	task.Agent.Name = "builder-api"
	task.Prompt.RequestID = "run-184:api:repair-1"
	task.RequiresFreshCommit = true
	task.PreviousCommitSHA = "1111111111111111111111111111111111111111"
	snapshot.Phase = contract.PhaseBuilding
	snapshot.PendingAction = parallelTaskAction("parallel_collect_evidence_", "api")
	snapshot.PendingTaskID = "api"
	snapshot.Tasks["api"] = task
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	before := h.herdr.getInfoCalls
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id).Tasks["api"]
	if got.Stage != "recovery_observe" || h.herdr.getInfoCalls != before {
		t.Fatalf("reconciled stale task=%+v getInfo before=%d after=%d", got, before, h.herdr.getInfoCalls)
	}
}

func TestParallelMergedResponseLossEnabledProjectSchedulesDoneObservation(t *testing.T) {
	h := newParallelHarness(t)
	h.Deps.ProjectAutomationEnabled = true
	h.Deps.Project = github.ProjectRef{ID: "PVT_1", StatusFieldID: "F_1", StatusOptions: map[string]string{"Done": "done"}}
	h.orchestrator = NewParallel(h.Deps)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseMerging
	snapshot.PendingAction = "parallel_merge_main"
	snapshot.Registration.NodeID = "issue-node"
	snapshot.PullRequest, snapshot.PullRequestHeadSHA, snapshot.FinalSHA = 185, validSHA, validSHA
	snapshot.ProjectStatus = "Review"
	mergeSHA := "2222222222222222222222222222222222222222"
	h.parallelGH.pr = github.PullRequest{Number: 185, State: "closed", Merged: true, Head: "agent/parent-integration", HeadSHA: validSHA, Base: "main", MergeCommitSHA: mergeSHA}
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseMerging || got.PendingAction != "parallel_project_observe_done" || got.MergeSHA != mergeSHA || !got.PullRequestMerged {
		t.Fatalf("merged response-loss snapshot=%+v", got)
	}
}

func TestParallelMergedResponseLossDisabledProjectCompletesWithAudit(t *testing.T) {
	h := newParallelHarness(t)
	id, err := h.orchestrator.Start(context.Background(), h.contractPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.mustLoad(id)
	snapshot.Phase = contract.PhaseMerging
	snapshot.PendingAction = "parallel_merge_main"
	snapshot.PullRequest, snapshot.PullRequestHeadSHA, snapshot.FinalSHA = 185, validSHA, validSHA
	mergeSHA := "2222222222222222222222222222222222222222"
	h.parallelGH.pr = github.PullRequest{Number: 185, State: "closed", Merged: true, Head: "agent/parent-integration", HeadSHA: validSHA, Base: "main", MergeCommitSHA: mergeSHA}
	if err := h.store.Save(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := NewParallel(h.Deps).Advance(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got := h.mustLoad(id)
	if got.Phase != contract.PhaseCompleted || got.MergeSHA != mergeSHA || !hasEventMessage(h.events(id), "Project automation disabled") {
		t.Fatalf("merged response-loss snapshot=%+v", got)
	}
}
