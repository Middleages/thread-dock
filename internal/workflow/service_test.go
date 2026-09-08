package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

func TestCoordinatorAwareWorkflowExposesExplicitReconcileSeam(t *testing.T) {
	workID := contractv2.WorkID("work-1")
	reconciler := &workflowReconcilerFake{result: coordinator.ReconcileResult{WorkID: workID, State: statev2.StatePaused, NextAction: "resume", EvidenceRefs: []string{"evidence://pause"}}}
	service := NewWithCoordinator(nil, nil, reconciler)
	got, err := service.ReconcileWork(context.Background(), workID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkID != workID || got.NextAction != "resume" || len(got.EvidenceRefs) != 1 || reconciler.calls != 1 {
		t.Fatalf("result=%#v calls=%d", got, reconciler.calls)
	}
}

type workflowReconcilerFake struct {
	result coordinator.ReconcileResult
	calls  int
}

func (r *workflowReconcilerFake) Reconcile(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error) {
	r.calls++
	return r.result, nil
}

func validContract() contractv2.WorkItemContract {
	return contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "ship it", AcceptanceCriteria: []string{"works"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}}}, Documentation: contractv2.DocumentationPlan{Required: false, Reason: "not required"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
}

func registerTestProject(t *testing.T, projects registry.Store) {
	t.Helper()
	project := registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	payload, _ := json.Marshal(project)
	sum := sha256.Sum256(payload)
	if _, err := projects.Create(context.Background(), project, 0, "request-project", hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}

func TestPlanApproveStatus(t *testing.T) {
	root := t.TempDir()
	projects := registry.NewStore(root)
	works := statev2.NewStore(root)
	registerTestProject(t, projects)
	s := New(projects, works)
	var input strings.Builder
	if err := contractv2.Write(&input, validContract()); err != nil {
		t.Fatal(err)
	}
	planned, err := s.PlanWork(context.Background(), "request.json", strings.NewReader(input.String()), 0, "request-plan")
	if err != nil {
		t.Fatal(err)
	}
	if planned.Revision != 1 || planned.State != statev2.StateAwaitingApproval {
		t.Fatalf("planned = %#v", planned)
	}
	approved, err := s.ApproveWork(context.Background(), "work-1", 1, "approve-1")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Revision != 2 || approved.State != statev2.StateQueued {
		t.Fatalf("approved = %#v", approved)
	}
	status, err := s.Status(context.Background(), "work-1")
	if err != nil || status.Revision != 2 {
		t.Fatalf("status = %#v, err=%v", status, err)
	}
	if _, err := s.ApproveWork(context.Background(), "work-1", 1, "approve-1"); err != nil {
		t.Fatal("approve replay should succeed: ", err)
	}
}

func TestPlanRejectsLegacyWithoutWritesAndPreservesSource(t *testing.T) {
	root := t.TempDir()
	projects := registry.NewStore(root)
	works := statev2.NewStore(root)
	registerTestProject(t, projects)
	s := New(projects, works)
	var legacy *UnsupportedLegacyError
	_, err := s.PlanWork(context.Background(), "legacy.json", strings.NewReader(`{"version":1,"projectId":"project-1"}`), 0, "request-legacy")
	if !errors.As(err, &legacy) || legacy.Code != "unsupported_legacy" || legacy.Source != "legacy.json" {
		t.Fatalf("legacy error = %v", err)
	}
	if _, err := works.Load(context.Background(), "work-1"); err == nil {
		t.Fatal("legacy plan wrote state")
	}
}

func TestPlanCreationPassesExpectedRevisionRequestAndCanonicalActionHash(t *testing.T) {
	projects := &recordingProjectStore{project: registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}}
	works := &recordingWorkStore{}
	s := New(projects, works)
	var input strings.Builder
	if err := contractv2.Write(&input, validContract()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlanWork(context.Background(), "contract.json", strings.NewReader(input.String()), 0, "request-plan"); err != nil {
		t.Fatal(err)
	}
	if works.snapshot.Control != (statev2.WorkControl{}) || works.snapshot.Publications == nil || len(works.snapshot.Publications) != 0 {
		t.Fatalf("creation typed control/publications = %#v %#v", works.snapshot.Control, works.snapshot.Publications)
	}
	if len(works.snapshot.TaskStates) != len(validContract().Tasks) {
		t.Fatalf("creation task states = %#v", works.snapshot.TaskStates)
	}
	for _, task := range validContract().Tasks {
		state, ok := works.snapshot.TaskStates[task.TaskID]
		if !ok || state.TaskID != task.TaskID || state.Status != statev2.TaskPending || state.RepairLimit != statev2.DefaultRepairLimit || state.RecoveryLimit != statev2.DefaultRecoveryLimit || state.PriorAttempts == nil {
			t.Fatalf("creation task state %q = %#v", task.TaskID, state)
		}
	}
	var canonical bytes.Buffer
	if err := contractv2.Write(&canonical, validContract()); err != nil {
		t.Fatal(err)
	}
	contractSum := sha256.Sum256(canonical.Bytes())
	action, err := json.Marshal(struct {
		WorkID           contractv2.WorkID   `json:"workId"`
		ExpectedRevision contractv2.Revision `json:"expectedRevision"`
		Action           string              `json:"action"`
		ContractHash     string              `json:"contractHash"`
	}{"work-1", 0, "plan", hex.EncodeToString(contractSum[:])})
	if err != nil {
		t.Fatal(err)
	}
	actionSum := sha256.Sum256(action)
	if works.requestID != "request-plan" || works.expectedRevision != 0 || works.payloadHash != hex.EncodeToString(actionSum[:]) {
		t.Fatalf("creation args revision=%d request=%q hash=%q", works.expectedRevision, works.requestID, works.payloadHash)
	}
}

type recordingProjectStore struct{ project registry.Project }

func (s *recordingProjectStore) Create(context.Context, registry.Project, contractv2.Revision, contractv2.RequestID, string) (registry.Project, error) {
	return s.project, nil
}
func (s *recordingProjectStore) Load(context.Context, contractv2.ProjectID) (registry.Project, error) {
	return s.project, nil
}
func (s *recordingProjectStore) List(context.Context) ([]registry.Project, error) {
	return []registry.Project{s.project}, nil
}

type recordingWorkStore struct {
	snapshot         statev2.WorkSnapshot
	requestID        contractv2.RequestID
	expectedRevision contractv2.Revision
	payloadHash      string
}

func (s *recordingWorkStore) CreatePlan(_ context.Context, snapshot statev2.WorkSnapshot, requestID contractv2.RequestID, payloadHash string) (statev2.WorkSnapshot, error) {
	s.snapshot = snapshot
	s.requestID, s.expectedRevision, s.payloadHash = requestID, 0, payloadHash
	return statev2.WorkSnapshot{}, nil
}
func (s *recordingWorkStore) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return statev2.WorkSnapshot{}, nil
}
func (s *recordingWorkStore) List(context.Context) ([]statev2.WorkSnapshot, error) { return nil, nil }
func (s *recordingWorkStore) Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	return statev2.WorkSnapshot{}, nil
}

type countingProjectStore struct {
	projects []registry.Project
	listCall int
}

func (f *countingProjectStore) Create(context.Context, registry.Project, contractv2.Revision, contractv2.RequestID, string) (registry.Project, error) {
	panic("unexpected Create")
}
func (f *countingProjectStore) Load(context.Context, contractv2.ProjectID) (registry.Project, error) {
	panic("unexpected Load")
}
func (f *countingProjectStore) List(context.Context) ([]registry.Project, error) {
	f.listCall++
	return append([]registry.Project(nil), f.projects...), nil
}

type countingWorkStore struct {
	works    []statev2.WorkSnapshot
	listCall int
}

func (f *countingWorkStore) CreatePlan(context.Context, statev2.WorkSnapshot, contractv2.RequestID, string) (statev2.WorkSnapshot, error) {
	panic("unexpected CreatePlan")
}
func (f *countingWorkStore) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	panic("unexpected Load")
}
func (f *countingWorkStore) List(context.Context) ([]statev2.WorkSnapshot, error) {
	f.listCall++
	return append([]statev2.WorkSnapshot(nil), f.works...), nil
}
func (f *countingWorkStore) Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	panic("unexpected Apply")
}

func TestSnapshotAggregatesRegisteredProjectAndWorkWithoutSideEffects(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{
		{ProjectID: "project-z", Name: "Zed"},
		{ProjectID: "project-a", Name: "Alpha"},
	}}
	work := statev2.WorkSnapshot{
		SchemaVersion: 2,
		ProjectID:     "project-a",
		WorkID:        "work-1",
		Revision:      7,
		State:         statev2.StateNeedsOperator,
		SyncStatus:    "pending",
		NextAction:    "inspect",
		EvidenceRefs:  []string{"evidence://one"},
		Contract: func() contractv2.WorkItemContract {
			contract := validContract()
			contract.ProjectID = "project-a"
			contract.IssueDrafts = []contractv2.IssueDraft{{Title: "Ship the thing"}}
			contract.Tasks = []contractv2.Task{{TaskID: "task-2", RepoKey: "backend"}, {TaskID: "task-1", RepoKey: "app"}}
			return contract
		}(),
		Control: statev2.WorkControl{ApprovedContractHash: "approved"},
		TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{
			"task-2": {TaskID: "task-2", Status: statev2.TaskNeedsOperator},
			"task-1": {TaskID: "task-1", Status: statev2.TaskPending},
		},
		Publications: map[statev2.PublicationIntentID]statev2.PublicationState{},
	}
	s := New(projects, &countingWorkStore{works: []statev2.WorkSnapshot{work}})
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	got, err := s.Snapshot(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	if projects.listCall != 1 {
		t.Fatalf("projects.List calls = %d", projects.listCall)
	}
	works := s.works.(*countingWorkStore)
	if works.listCall != 1 {
		t.Fatalf("works.List calls = %d", works.listCall)
	}
	if got.SchemaVersion != 2 || !got.ObservedAt.Equal(at) || got.Revision != 7 {
		t.Fatalf("snapshot metadata = %#v", got)
	}
	if got.Projects == nil || len(got.Projects) != 2 || got.Projects[0].ProjectID != "project-a" {
		t.Fatalf("projects = %#v", got.Projects)
	}
	project := got.Projects[0]
	if project.Name != "Alpha" || project.State != string(work.State) || project.SyncStatus != work.SyncStatus || project.NextAction != "resolve" || !reflect.DeepEqual(project.EvidenceRefs, work.EvidenceRefs) {
		t.Fatalf("project = %#v", project)
	}
	if len(project.WorkItems) != 1 {
		t.Fatalf("work items = %#v", project.WorkItems)
	}
	item := project.WorkItems[0]
	if item.WorkID != work.WorkID || item.Title != "Ship the thing" || item.Request != work.Contract.Request || item.State != string(work.State) || item.SyncStatus != work.SyncStatus || item.NextAction != "resolve" || !reflect.DeepEqual(item.EvidenceRefs, work.EvidenceRefs) {
		t.Fatalf("work item = %#v", item)
	}
	if len(item.Tasks) != 2 || item.Tasks[0].TaskID != "task-2" || item.Tasks[0].RepoKey != "backend" || item.Tasks[1].TaskID != "task-1" || item.Tasks[1].RepoKey != "app" {
		t.Fatalf("tasks = %#v", item.Tasks)
	}
	if got.State != item.State || got.SyncStatus != item.SyncStatus || got.NextAction != item.NextAction || !reflect.DeepEqual(got.EvidenceRefs, item.EvidenceRefs) {
		t.Fatalf("global status = %#v", got)
	}
	if got.Freshness.State != "fresh" || got.Freshness.SyncStatus != got.SyncStatus || !got.Freshness.ObservedAt.Equal(at) {
		t.Fatalf("freshness = %#v", got.Freshness)
	}
}

func TestSnapshotRejectsWorkForUnregisteredProject(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	works := &countingWorkStore{works: []statev2.WorkSnapshot{{ProjectID: "missing", WorkID: "work-1"}}}
	_, err := New(projects, works).Snapshot(context.Background(), time.Now())
	if err == nil {
		t.Fatal("Snapshot accepted work for an unregistered project")
	}
}

func TestSnapshotUsesWorkIDTitleFallbackAndDraftDefaults(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	works := &countingWorkStore{works: []statev2.WorkSnapshot{{ProjectID: "project-1", WorkID: "work-1", Revision: 1, State: statev2.StateAwaitingApproval, Contract: contractv2.WorkItemContract{WorkID: "work-1", ProjectID: "project-1", Request: "request"}}}}
	got, err := New(projects, works).Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got.Projects[0].State != "awaiting_approval" || got.Projects[0].WorkItems[0].Title != "work-1" {
		t.Fatalf("fallback/default mapping = %#v", got)
	}
}

func TestSnapshotOrdersWorkItemsByStateAndRecentActivity(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	newWork := func(id contractv2.WorkID, state statev2.WorkState, at time.Time) statev2.WorkSnapshot {
		work := statev2.WorkSnapshot{ProjectID: "project-1", WorkID: id, State: state, SyncStatus: "local", NextAction: string(state), Contract: contractv2.WorkItemContract{WorkID: id, ProjectID: "project-1", Tasks: []contractv2.Task{{TaskID: "activity"}}}, EvidenceRefs: []string{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"activity": {TaskID: "activity", Status: statev2.TaskPending, Invocation: &statev2.InvocationState{StartedAt: &at}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
		if state == statev2.StateDraft {
			work.Contract.Tasks = nil
			work.TaskStates = map[contractv2.TaskID]statev2.TaskExecutionState{}
		}
		if state != statev2.StateDraft {
			work.ContractHash = "contract-hash"
			work.Control.ApprovedContractHash = "contract-hash"
		}
		switch state {
		case statev2.StateNeedsOperator:
			work.TaskStates["activity"] = statev2.TaskExecutionState{TaskID: "activity", Status: statev2.TaskNeedsOperator, Invocation: &statev2.InvocationState{StartedAt: &at}}
		case statev2.StatePaused:
			work.Control.PauseRequested = true
		case statev2.StatePublicationPending:
			work.TaskStates["activity"] = statev2.TaskExecutionState{TaskID: "activity", Status: statev2.TaskIntegrated, Invocation: &statev2.InvocationState{StartedAt: &at}}
			work.Publications["intent"] = statev2.PublicationState{IntentID: "intent", Key: "issue", Generation: 1, Status: statev2.PublicationPending, CompletionRequired: true}
		case statev2.StateReview:
			work.TaskStates["activity"] = statev2.TaskExecutionState{TaskID: "activity", Status: statev2.TaskAccepted, Invocation: &statev2.InvocationState{StartedAt: &at}}
		case statev2.StateRunning:
			activity := work.TaskStates["activity"]
			activity.Status = statev2.TaskRunning
			work.TaskStates["activity"] = activity
		case statev2.StateReadyForPR, statev2.StateCompleted:
			work.TaskStates["activity"] = statev2.TaskExecutionState{TaskID: "activity", Status: statev2.TaskIntegrated, Invocation: &statev2.InvocationState{StartedAt: &at}}
		case statev2.StateQueued:
			activity := work.TaskStates["activity"]
			activity.Status = statev2.TaskPending
			work.TaskStates["activity"] = activity
		}
		return work
	}
	works := []statev2.WorkSnapshot{
		newWork("queued", statev2.StateQueued, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("draft", statev2.StateDraft, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("completed", statev2.StateCompleted, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("ready", statev2.StateReadyForPR, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("running", statev2.StateRunning, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("review", statev2.StateReview, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("publication", statev2.StatePublicationPending, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("operator", statev2.StateNeedsOperator, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		newWork("paused", statev2.StatePaused, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)),
	}
	got, err := New(projects, &countingWorkStore{works: works}).Snapshot(context.Background(), time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	items := got.Projects[0].WorkItems
	want := []contractv2.WorkID{"operator", "paused", "publication", "review", "running", "ready", "queued", "completed", "draft"}
	if len(items) != len(want) {
		t.Fatalf("work count = %d, want %d", len(items), len(want))
	}
	for i, id := range want {
		if items[i].WorkID != id {
			t.Fatalf("work order[%d] = %q, want %q", i, items[i].WorkID, id)
		}
	}
	if items[0].WorkID != "operator" || items[1].WorkID != "paused" {
		t.Fatalf("work order = %#v", items)
	}
}

func TestSnapshotOrdersEqualStateByRecentActivityThenWorkID(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	newWork := func(id contractv2.WorkID, at time.Time) statev2.WorkSnapshot {
		return statev2.WorkSnapshot{ProjectID: "project-1", WorkID: id, State: statev2.StateQueued, ContractHash: "hash", Control: statev2.WorkControl{ApprovedContractHash: "hash"}, Contract: contractv2.WorkItemContract{WorkID: id, ProjectID: "project-1", Tasks: []contractv2.Task{{TaskID: "task"}}}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task": {TaskID: "task", Status: statev2.TaskPending, Invocation: &statev2.InvocationState{StartedAt: &at}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	}
	older := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	works := &countingWorkStore{works: []statev2.WorkSnapshot{newWork("z", older), newWork("b", newer), newWork("a", newer)}}
	got, err := New(projects, works).Snapshot(context.Background(), newer)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Projects[0].WorkItems
	if want := []contractv2.WorkID{"a", "b", "z"}; len(items) != len(want) || items[0].WorkID != want[0] || items[1].WorkID != want[1] || items[2].WorkID != want[2] {
		t.Fatalf("equal-state order = %#v, want %#v", items, want)
	}
}

func TestSnapshotUsesProjectAndGlobalRepresentativesByPriority(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-a", Name: "A"}, {ProjectID: "project-b", Name: "B"}}}
	work := func(project contractv2.ProjectID, id contractv2.WorkID, state statev2.WorkState, task statev2.TaskStatus) statev2.WorkSnapshot {
		return statev2.WorkSnapshot{ProjectID: project, WorkID: id, State: state, ContractHash: "hash", Control: statev2.WorkControl{ApprovedContractHash: "hash"}, Contract: contractv2.WorkItemContract{WorkID: id, ProjectID: project, Tasks: []contractv2.Task{{TaskID: "task"}}}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task": {TaskID: "task", Status: task}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	}
	works := &countingWorkStore{works: []statev2.WorkSnapshot{work("project-a", "queued", statev2.StateQueued, statev2.TaskPending), work("project-a", "review", statev2.StateReview, statev2.TaskAccepted), work("project-b", "operator", statev2.StateNeedsOperator, statev2.TaskNeedsOperator)}}
	got, err := New(projects, works).Snapshot(context.Background(), time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Projects[0].WorkItems[0].WorkID != "review" || got.Projects[1].WorkItems[0].WorkID != "operator" || got.State != string(statev2.StateNeedsOperator) || got.NextAction != "resolve" {
		t.Fatalf("representatives = %#v global=%q/%q", got.Projects, got.State, got.NextAction)
	}
}

func TestSnapshotProjectsExposeTaskPublicationAndBoundedBlockerProjection(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	work := statev2.WorkSnapshot{
		ProjectID: "project-1", WorkID: "work-1", State: statev2.StateNeedsOperator, SyncStatus: "conflict", NextAction: "resolve",
		Contract:     contractv2.WorkItemContract{WorkID: "work-1", ProjectID: "project-1", Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app"}}},
		TaskStates:   map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskNeedsOperator, Invocation: &statev2.InvocationState{InvocationID: "provider-invocation-secret", ProviderIdentity: "provider-secret", ProviderSession: "session-secret", ProviderPane: "pane-secret", ProviderProcess: "process-secret", StartedAt: &at}, Gate: &statev2.GateEvidence{ObservedAt: at, Diagnostic: "raw-gate-secret"}, Worktree: &statev2.WorktreeIdentity{CanonicalPath: "/secret/canonical/path"}}},
		Control:      statev2.WorkControl{Blocker: &statev2.OperatorBlocker{Kind: statev2.BlockerKindRuntimeUnknown, Diagnostic: "raw-blocker-secret", OperatorRef: "operator-secret", TaskID: "task-1"}},
		Publications: map[statev2.PublicationIntentID]statev2.PublicationState{"intent-1": {IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, Status: statev2.PublicationCompleted, Attempts: 2, PayloadHash: "payload-secret", PayloadRef: "payload-ref-secret", Target: statev2.PublicationTarget{Host: "github.com", Base: "base-secret", Key: "issue:1"}, Receipt: &statev2.PublicationReceipt{URL: "https://example.test/issues/1", PublishedAt: at}}},
	}
	got, err := New(projects, &countingWorkStore{works: []statev2.WorkSnapshot{work}}).Snapshot(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	item := got.Projects[0].WorkItems[0]
	if item.Blocker != statev2.BlockerKindRuntimeUnknown || len(item.Tasks) != 1 || item.Tasks[0].State != string(statev2.TaskNeedsOperator) || len(item.Publications) != 1 || item.Publications[0].URL != "https://example.test/issues/1" {
		t.Fatalf("projection = %#v", item)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"provider-invocation-secret", "provider-secret", "session-secret", "pane-secret", "process-secret", "raw-gate-secret", "raw-blocker-secret", "operator-secret", "/secret/canonical/path", "payload-secret", "payload-ref-secret", "base-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("projection leaked %q: %s", secret, data)
		}
	}
}

func TestSnapshotMapsEveryTaskSummaryState(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	tasks := []contractv2.Task{
		{TaskID: "passed", RepoKey: "app"}, {TaskID: "failed", RepoKey: "app"}, {TaskID: "pending", RepoKey: "app"},
		{TaskID: "accepted", RepoKey: "app"}, {TaskID: "blocked", RepoKey: "app"}, {TaskID: "integrated", RepoKey: "app"},
	}
	states := map[contractv2.TaskID]statev2.TaskExecutionState{
		"passed":     {TaskID: "passed", Status: statev2.TaskGatePassed},
		"failed":     {TaskID: "failed", Status: statev2.TaskGateFailed},
		"pending":    {TaskID: "pending", Status: statev2.TaskPending},
		"accepted":   {TaskID: "accepted", Status: statev2.TaskAccepted},
		"blocked":    {TaskID: "blocked", Status: statev2.TaskReviewBlocked},
		"integrated": {TaskID: "integrated", Status: statev2.TaskIntegrated},
	}
	work := statev2.WorkSnapshot{ProjectID: "project-1", WorkID: "work-1", State: statev2.StateRunning, ContractHash: "hash", Control: statev2.WorkControl{ApprovedContractHash: "hash"}, Contract: contractv2.WorkItemContract{WorkID: "work-1", ProjectID: "project-1", Tasks: tasks}, TaskStates: states, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	got, err := New(projects, &countingWorkStore{works: []statev2.WorkSnapshot{work}}).Snapshot(context.Background(), time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := map[contractv2.TaskID][3]string{
		"passed": {"passed", "pending", "pending"}, "failed": {"failed", "pending", "pending"}, "pending": {"pending", "pending", "pending"},
		"accepted": {"passed", "accepted", "pending"}, "blocked": {"pending", "blocked", "pending"}, "integrated": {"passed", "accepted", "integrated"},
	}
	for _, task := range got.Projects[0].WorkItems[0].Tasks {
		values, ok := want[task.TaskID]
		if !ok || [3]string{task.Verification, task.Review, task.Merge} != values {
			t.Fatalf("task %q summary = %#v, want %#v", task.TaskID, task, values)
		}
	}
}

func TestSnapshotSortsPublicationDetailsByKeyGenerationAndIntent(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	work := statev2.WorkSnapshot{ProjectID: "project-1", WorkID: "work-1", State: statev2.StateReadyForPR, Contract: contractv2.WorkItemContract{WorkID: "work-1", ProjectID: "project-1"}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{
		"z": {IntentID: "z", Key: "a", Generation: 2}, "b": {IntentID: "b", Key: "a", Generation: 1}, "a": {IntentID: "a", Key: "a", Generation: 1}, "c": {IntentID: "c", Key: "b", Generation: 1},
	}}
	got, err := New(projects, &countingWorkStore{works: []statev2.WorkSnapshot{work}}).Snapshot(context.Background(), time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var gotOrder []string
	for _, publication := range got.Projects[0].WorkItems[0].Publications {
		gotOrder = append(gotOrder, publication.IntentID)
	}
	if want := []string{"a", "b", "z", "c"}; !reflect.DeepEqual(gotOrder, want) {
		t.Fatalf("publication order = %#v, want %#v", gotOrder, want)
	}
}

func TestSnapshotActivityUsesEachEvidenceTimestampAndProjectMaximum(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	times := []time.Time{
		time.Date(2026, 9, 8, 1, 0, 0, 0, time.UTC), time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 8, 3, 0, 0, 0, time.UTC), time.Date(2026, 9, 8, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 8, 5, 0, 0, 0, time.UTC), time.Date(2026, 9, 8, 6, 0, 0, 0, time.UTC),
	}
	work := statev2.WorkSnapshot{ProjectID: "project-1", WorkID: "work-1", State: statev2.StateReadyForPR, Contract: contractv2.WorkItemContract{WorkID: "work-1", ProjectID: "project-1", Tasks: []contractv2.Task{{TaskID: "task-1"}}}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskIntegrated}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	start := times[0]
	work.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskIntegrated, Invocation: &statev2.InvocationState{StartedAt: &start}}
	got, err := New(projects, &countingWorkStore{works: []statev2.WorkSnapshot{work}}).Snapshot(context.Background(), times[0])
	if err != nil {
		t.Fatal(err)
	}
	if !got.Projects[0].WorkItems[0].UpdatedAt.Equal(times[0]) || !got.Projects[0].UpdatedAt.Equal(times[0]) {
		t.Fatalf("invocation activity = %v/%v", got.Projects[0].WorkItems[0].UpdatedAt, got.Projects[0].UpdatedAt)
	}
	mutations := []func(*statev2.WorkSnapshot){
		func(s *statev2.WorkSnapshot) {
			ended := times[1]
			s.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskIntegrated, Invocation: &statev2.InvocationState{EndedAt: &ended}}
		},
		func(s *statev2.WorkSnapshot) {
			s.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskIntegrated, Gate: &statev2.GateEvidence{ObservedAt: times[2]}}
		},
		func(s *statev2.WorkSnapshot) {
			s.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskIntegrated, Review: &statev2.ReviewEvidence{ObservedAt: times[3]}}
		},
		func(s *statev2.WorkSnapshot) {
			s.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskIntegrated, Integration: &statev2.IntegrationEvidence{ObservedAt: times[4]}}
		},
		func(s *statev2.WorkSnapshot) {
			s.Publications["intent"] = statev2.PublicationState{IntentID: "intent", Receipt: &statev2.PublicationReceipt{PublishedAt: times[5]}}
		},
	}
	for i, mutate := range mutations {
		current := work
		current.TaskStates = map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskIntegrated}}
		current.Publications = map[statev2.PublicationIntentID]statev2.PublicationState{}
		mutate(&current)
		got, err := New(projects, &countingWorkStore{works: []statev2.WorkSnapshot{current}}).Snapshot(context.Background(), times[0])
		if err != nil {
			t.Fatal(err)
		}
		want := times[i+1]
		if !got.Projects[0].WorkItems[0].UpdatedAt.Equal(want) || !got.Projects[0].UpdatedAt.Equal(want) {
			t.Fatalf("activity source %d = %v/%v, want %v", i, got.Projects[0].WorkItems[0].UpdatedAt, got.Projects[0].UpdatedAt, want)
		}
	}
}

func TestSnapshotRejectsNonUTCObservationTime(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	works := &countingWorkStore{}
	_, err := New(projects, works).Snapshot(context.Background(), time.Date(2026, 9, 8, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60)))
	if err == nil {
		t.Fatal("Snapshot accepted non-UTC observation time")
	}
}

func TestSnapshotUsesReducerDerivedStateWithoutMutatingStoreSnapshot(t *testing.T) {
	projects := &countingProjectStore{projects: []registry.Project{{ProjectID: "project-1", Name: "Project"}}}
	work := statev2.WorkSnapshot{
		ProjectID: "project-1", WorkID: "work-1", State: statev2.StateQueued, SyncStatus: "raw-sync", NextAction: "raw-action",
		ContractHash: "contract-hash", Control: statev2.WorkControl{ApprovedContractHash: "contract-hash"},
		Contract:     contractv2.WorkItemContract{WorkID: "work-1", ProjectID: "project-1", Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app"}}},
		TaskStates:   map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskAccepted}},
		Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}, EvidenceRefs: []string{},
	}
	store := &countingWorkStore{works: []statev2.WorkSnapshot{work}}
	got, err := New(projects, store).Snapshot(context.Background(), time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	item := got.Projects[0].WorkItems[0]
	if item.State != string(statev2.StateReview) || item.NextAction != "integrate" {
		t.Fatalf("derived projection = %q/%q, want review/integrate", item.State, item.NextAction)
	}
	if store.works[0].State != statev2.StateQueued || store.works[0].NextAction != "raw-action" || store.works[0].SyncStatus != "raw-sync" {
		t.Fatalf("store snapshot was mutated: %#v", store.works[0])
	}
}

func TestSnapshotRejectsZeroObservationTime(t *testing.T) {
	_, err := New(&countingProjectStore{}, &countingWorkStore{}).Snapshot(context.Background(), time.Time{})
	if err == nil {
		t.Fatal("Snapshot accepted zero observation time")
	}
}
