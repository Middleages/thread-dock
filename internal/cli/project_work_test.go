package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/monitor"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

type fakeWorkflowService struct {
	project          registry.Project
	projects         []registry.Project
	plan             statev2.WorkSnapshot
	approved         statev2.WorkSnapshot
	status           statev2.WorkSnapshot
	snapshot         monitor.Snapshot
	err              error
	calls            []string
	approveID        contractv2.WorkID
	approveRevision  contractv2.Revision
	approveRequest   contractv2.RequestID
	planInput        string
	planPath         string
	registerRevision contractv2.Revision
	registerRequest  contractv2.RequestID
	planRevision     contractv2.Revision
	planRequest      contractv2.RequestID
}

type fakeWorkflowControlService struct {
	*fakeWorkflowService
	paused         statev2.WorkSnapshot
	resumed        statev2.WorkSnapshot
	reconcile      coordinator.ReconcileResult
	pauseErr       error
	resumeErr      error
	reconcileErr   error
	pauseID        contractv2.WorkID
	pauseRevision  contractv2.Revision
	pauseRequest   contractv2.RequestID
	resumeID       contractv2.WorkID
	resumeRevision contractv2.Revision
	resumeRequest  contractv2.RequestID
	reconcileID    contractv2.WorkID
	sequence       []string
}

type fakeWorkflowRunnerService struct {
	*fakeWorkflowService
	runID       contractv2.WorkID
	runRevision contractv2.Revision
	runRequest  contractv2.RequestID
	runSnapshot statev2.WorkSnapshot
	runErr      error
}

type fakeWorkflowParentIssuePublisher struct {
	*fakeWorkflowService
	publishID       contractv2.WorkID
	publishDraftKey string
	publishRevision contractv2.Revision
	publishRequest  contractv2.RequestID
	publishSnapshot statev2.WorkSnapshot
	publishErr      error
	publishCalls    int
}

func (f *fakeWorkflowParentIssuePublisher) PublishParentIssue(_ context.Context, id contractv2.WorkID, draftKey string, revision contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	f.publishCalls++
	f.publishID, f.publishDraftKey, f.publishRevision, f.publishRequest = id, draftKey, revision, request
	if f.publishErr != nil {
		return statev2.WorkSnapshot{}, f.publishErr
	}
	return f.publishSnapshot, nil
}

func (f *fakeWorkflowRunnerService) RunWork(_ context.Context, id contractv2.WorkID, revision contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	f.runID, f.runRevision, f.runRequest = id, revision, request
	if f.runErr != nil {
		return statev2.WorkSnapshot{}, f.runErr
	}
	return f.runSnapshot, nil
}

func (f *fakeWorkflowControlService) PauseWork(_ context.Context, id contractv2.WorkID, revision contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	f.sequence = append(f.sequence, "pause")
	f.pauseID, f.pauseRevision, f.pauseRequest = id, revision, request
	if f.pauseErr != nil {
		return statev2.WorkSnapshot{}, f.pauseErr
	}
	return f.paused, nil
}

func (f *fakeWorkflowControlService) ResumeWork(_ context.Context, id contractv2.WorkID, revision contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	f.sequence = append(f.sequence, "resume")
	f.resumeID, f.resumeRevision, f.resumeRequest = id, revision, request
	if f.resumeErr != nil {
		return statev2.WorkSnapshot{}, f.resumeErr
	}
	return f.resumed, nil
}

func (f *fakeWorkflowControlService) ReconcileWork(_ context.Context, id contractv2.WorkID) (coordinator.ReconcileResult, error) {
	f.sequence = append(f.sequence, "reconcile")
	f.reconcileID = id
	if f.reconcileErr != nil {
		return coordinator.ReconcileResult{}, f.reconcileErr
	}
	return f.reconcile, nil
}

func (f *fakeWorkflowControlService) Status(ctx context.Context, id contractv2.WorkID) (statev2.WorkSnapshot, error) {
	f.sequence = append(f.sequence, "status")
	return f.fakeWorkflowService.Status(ctx, id)
}

func (f *fakeWorkflowService) RegisterProject(_ context.Context, p registry.Project, revision contractv2.Revision, request contractv2.RequestID) (registry.Project, error) {
	f.calls = append(f.calls, "register:"+string(p.ProjectID))
	f.registerRevision, f.registerRequest = revision, request
	if f.err != nil {
		return registry.Project{}, f.err
	}
	f.project = p
	return p, nil
}
func (f *fakeWorkflowService) ListProjects(context.Context) ([]registry.Project, error) {
	f.calls = append(f.calls, "list")
	if f.err != nil {
		return nil, f.err
	}
	return f.projects, nil
}
func (f *fakeWorkflowService) PlanWork(_ context.Context, path string, r io.Reader, revision contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	f.calls = append(f.calls, "plan:"+path)
	f.planRevision, f.planRequest = revision, request
	data, readErr := io.ReadAll(r)
	if readErr == nil {
		f.planInput = string(data)
	}
	f.planPath = path
	if f.err != nil {
		return statev2.WorkSnapshot{}, f.err
	}
	return f.plan, readErr
}
func (f *fakeWorkflowService) ApproveWork(_ context.Context, id contractv2.WorkID, revision contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	f.calls = append(f.calls, "approve")
	f.approveID = id
	f.approveRevision = revision
	f.approveRequest = request
	if f.err != nil {
		return statev2.WorkSnapshot{}, f.err
	}
	return f.approved, nil
}
func (f *fakeWorkflowService) Status(_ context.Context, id contractv2.WorkID) (statev2.WorkSnapshot, error) {
	f.calls = append(f.calls, "status:"+string(id))
	if f.err != nil {
		return statev2.WorkSnapshot{}, f.err
	}
	return f.status, nil
}
func (f *fakeWorkflowService) Snapshot(_ context.Context, at time.Time) (monitor.Snapshot, error) {
	f.calls = append(f.calls, "snapshot:"+at.UTC().Format(time.RFC3339Nano))
	if f.err != nil {
		return monitor.Snapshot{}, f.err
	}
	f.snapshot.ObservedAt = at
	return f.snapshot, nil
}

func workflowProject() registry.Project {
	return registry.Project{
		ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app",
		Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{
			"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"},
		},
	}
}

func workflowSnapshot() statev2.WorkSnapshot {
	return statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: "project-1", WorkID: "work-1", Revision: 1, State: statev2.StateAwaitingApproval, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}}
}

func runWorkflow(t *testing.T, service *fakeWorkflowService, args []string) (int, string, string) {
	t.Helper()
	return runWorkflowService(t, service, args)
}

func runWorkflowService(t *testing.T, service WorkflowService, args []string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := RunWithDependencies(context.Background(), args, &out, &errOut, Dependencies{Workflow: service})
	return code, out.String(), errOut.String()
}

func TestWorkflowControlCommandsRouteAndSerializeArgs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		want  string
		check func(*fakeWorkflowControlService)
	}{
		{name: "pause", args: []string{"work", "pause", "work-1", "--expected-revision", "7", "--request-id", "request-pause"}, want: "pause", check: func(f *fakeWorkflowControlService) {
			if f.pauseID != "work-1" || f.pauseRevision != 7 || f.pauseRequest != "request-pause" {
				t.Fatalf("pause args=(%q,%d,%q)", f.pauseID, f.pauseRevision, f.pauseRequest)
			}
		}},
		{name: "resume", args: []string{"work", "resume", "work-1", "--expected-revision", "8", "--request-id", "request-resume"}, want: "resume", check: func(f *fakeWorkflowControlService) {
			if f.resumeID != "work-1" || f.resumeRevision != 8 || f.resumeRequest != "request-resume" {
				t.Fatalf("resume args=(%q,%d,%q)", f.resumeID, f.resumeRevision, f.resumeRequest)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeWorkflowControlService{fakeWorkflowService: &fakeWorkflowService{}}
			code, out, errOut := runWorkflowService(t, service, tc.args)
			if code != 0 || errOut != "" || !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
			if len(service.sequence) != 1 || service.sequence[0] != tc.want {
				t.Fatalf("sequence=%v want [%s]", service.sequence, tc.want)
			}
			checkJSONKeys("schemaVersion", "workId", "revision", "state")(t, out)
			tc.check(service)
		})
	}
}

func TestWorkflowPublishIssuesRoutesCanonicalArgumentsAndSnapshot(t *testing.T) {
	service := &fakeWorkflowParentIssuePublisher{
		fakeWorkflowService: &fakeWorkflowService{},
		publishSnapshot:     workflowSnapshot(),
	}
	args := []string{"work", "publish-issues", "work-1", "parent-draft", "--expected-revision", "7", "--request-id", "request-publish"}
	code, out, errOut := runWorkflowService(t, service, args)
	if code != 0 || out == "" || errOut != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if service.publishCalls != 1 || service.publishID != "work-1" || service.publishDraftKey != "parent-draft" || service.publishRevision != 7 || service.publishRequest != "request-publish" {
		t.Fatalf("publish=(%d,%q,%q,%d,%q)", service.publishCalls, service.publishID, service.publishDraftKey, service.publishRevision, service.publishRequest)
	}
	var got statev2.WorkSnapshot
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, service.publishSnapshot) {
		t.Fatalf("snapshot=%#v want=%#v", got, service.publishSnapshot)
	}
}

func TestPublishIssuesMalformedFormsRemainDependencyFree(t *testing.T) {
	valid := []string{"work", "publish-issues", "work-1", "draft", "--expected-revision", "1", "--request-id", "request-1"}
	malformed := [][]string{
		{"work", "publish-issues", "work-1", "draft"},
		{"work", "publish-issues", "work-1", "draft", "--expected-revision", "0", "--request-id", "request-1"},
		{"work", "publish-issues", "work-1", "draft", "--expected-revision", "-1", "--request-id", "request-1"},
		{"work", "publish-issues", "work-1", "draft", "--expected-revision", "x", "--request-id", "request-1"},
		{"work", "publish-issues", "work-1", "draft", "--request-id", "request-1", "--expected-revision", "1"},
		{"work", "publish-issues", "-work-1", "draft", "--expected-revision", "1", "--request-id", "request-1"},
		{"work", "publish-issues", "work-1", "-draft", "--expected-revision", "1", "--request-id", "request-1"},
		{"work", "publish-issues", "work-1", "draft", "--expected-revision", "1", "--request-id", ""},
		{"work", "publish-issues", "work-1", "draft", "--expected-revision", "1", "--request-id", " request-1"},
		{"work", "publish-issues", "work-1", "draft with space", "--expected-revision", "1", "--request-id", "request-1"},
	}
	if !NeedsWorkflowDependencies(valid) {
		t.Fatal("valid publish-issues form must require workflow dependencies")
	}
	for _, args := range malformed {
		if NeedsWorkflowDependencies(args) {
			t.Fatalf("malformed publish form requires dependencies: %v", args)
		}
		service := &fakeWorkflowParentIssuePublisher{fakeWorkflowService: &fakeWorkflowService{}}
		code, out, errOut := runWorkflowService(t, service, args)
		if code != 2 || out != "" || !strings.Contains(errOut, "사용법:") || service.publishCalls != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q calls=%d", args, code, out, errOut, service.publishCalls)
		}
	}
}

func TestPublishIssuesCapabilityMissingAndErrorsAreBounded(t *testing.T) {
	args := []string{"work", "publish-issues", "work-1", "draft", "--expected-revision", "1", "--request-id", "request-1"}
	for _, service := range []WorkflowService{
		&fakeWorkflowService{},
		&fakeWorkflowParentIssuePublisher{fakeWorkflowService: &fakeWorkflowService{}, publishErr: errors.New("provider token sentinel")},
	} {
		code, out, errOut := runWorkflowService(t, service, args)
		if code != 1 || out != "" || errOut != "프로젝트·워크플로 명령을 처리하지 못했습니다.\n" || strings.Contains(errOut, "sentinel") || strings.Contains(errOut, "token") {
			t.Fatalf("service=%T code=%d stdout=%q stderr=%q", service, code, out, errOut)
		}
	}
}

func TestWorkflowReconcileSettlesBeforeCanonicalStatus(t *testing.T) {
	service := &fakeWorkflowControlService{
		fakeWorkflowService: &fakeWorkflowService{status: workflowSnapshot()},
		reconcile:           coordinator.ReconcileResult{WorkID: "work-1", State: statev2.StateRunning, NextAction: "continue"},
	}
	code, out, errOut := runWorkflowService(t, service, []string{"work", "reconcile", "work-1", "--json"})
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !reflect.DeepEqual(service.sequence, []string{"reconcile", "status"}) {
		t.Fatalf("sequence=%v", service.sequence)
	}
	if service.reconcileID != "work-1" || len(service.fakeWorkflowService.calls) != 1 || service.fakeWorkflowService.calls[0] != "status:work-1" {
		t.Fatalf("reconcileID=%q calls=%v", service.reconcileID, service.fakeWorkflowService.calls)
	}
	var got statev2.WorkSnapshot
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, service.status) {
		t.Fatalf("snapshot=%#v want=%#v", got, service.status)
	}
}

func TestWorkflowControlMissingCapabilityAndErrorsAreBounded(t *testing.T) {
	tests := []struct {
		name    string
		service WorkflowService
		args    []string
	}{
		{name: "missing pause capability", service: &fakeWorkflowService{}, args: []string{"work", "pause", "work-1", "--expected-revision", "1", "--request-id", "request-1"}},
		{name: "missing resume capability", service: &fakeWorkflowService{}, args: []string{"work", "resume", "work-1", "--expected-revision", "1", "--request-id", "request-1"}},
		{name: "missing reconcile capability", service: &fakeWorkflowService{}, args: []string{"work", "reconcile", "work-1", "--json"}},
		{name: "pause error", service: &fakeWorkflowControlService{fakeWorkflowService: &fakeWorkflowService{}, pauseErr: errors.New("provider token secret")}, args: []string{"work", "pause", "work-1", "--expected-revision", "1", "--request-id", "request-1"}},
		{name: "reconcile error", service: &fakeWorkflowControlService{fakeWorkflowService: &fakeWorkflowService{}, reconcileErr: errors.New("filesystem credential secret")}, args: []string{"work", "reconcile", "work-1", "--json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runWorkflowService(t, tt.service, tt.args)
			if code != 1 || out != "" || errOut == "" || strings.Contains(errOut, "provider") || strings.Contains(errOut, "token") || strings.Contains(errOut, "filesystem") || strings.Contains(errOut, "credential") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
		})
	}
}

func TestWorkflowUsageIncludesControlCommands(t *testing.T) {
	var errOut bytes.Buffer
	printUsage(&errOut)
	for _, syntax := range []string{
		"work publish-issues WORK PARENT_DRAFT_KEY --expected-revision N --request-id ID",
		"work pause WORK --expected-revision N --request-id ID",
		"work resume WORK --expected-revision N --request-id ID",
		"work reconcile WORK --json",
	} {
		if !strings.Contains(errOut.String(), syntax) {
			t.Fatalf("usage=%q missing %q", errOut.String(), syntax)
		}
	}
}

func TestWorkflowRunShapeIsDependencyBacked(t *testing.T) {
	id, revision, request, ok := parseWorkflowRunArgs([]string{"work-1", "--expected-revision", "7", "--request-id", "request-1"})
	if !ok || id != "work-1" || revision != 7 || request != "request-1" {
		t.Fatalf("parsed run=(%q,%d,%q,%t)", id, revision, request, ok)
	}
	if !NeedsWorkflowDependencies([]string{"work", "run", "work-1", "--expected-revision", "7", "--request-id", "request-1"}) {
		t.Fatal("valid work run must require workflow dependencies")
	}
	if NeedsWorkflowDependencies([]string{"work", "run", "work-1", "--expected-revision", "0", "--request-id", "request-1"}) {
		t.Fatal("invalid work run must remain dependency-free")
	}
}

func TestWorkflowRunRoutesRunnerAndEmitsCanonicalSnapshot(t *testing.T) {
	service := &fakeWorkflowRunnerService{fakeWorkflowService: &fakeWorkflowService{}, runSnapshot: workflowSnapshot()}
	code, out, errOut := runWorkflowService(t, service, []string{"work", "run", "work-1", "--expected-revision", "7", "--request-id", "request-run"})
	if code != 0 || errOut != "" || strings.Count(out, "\n") != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if service.runID != "work-1" || service.runRevision != 7 || service.runRequest != "request-run" {
		t.Fatalf("run args=(%q,%d,%q)", service.runID, service.runRevision, service.runRequest)
	}
	var got statev2.WorkSnapshot
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, service.runSnapshot) {
		t.Fatalf("snapshot=%#v want=%#v", got, service.runSnapshot)
	}
}

func TestWorkflowRunErrorsAndMalformedShapesAreBoundedAndDependencyFree(t *testing.T) {
	service := &fakeWorkflowRunnerService{fakeWorkflowService: &fakeWorkflowService{}, runErr: errors.New("credential token sentinel")}
	code, out, errOut := runWorkflowService(t, service, []string{"work", "run", "work-1", "--expected-revision", "1", "--request-id", "request-run"})
	if code != 1 || out != "" || !strings.Contains(errOut, "프로젝트·워크플로 명령을 처리하지 못했습니다.") || strings.Contains(errOut, "sentinel") {
		t.Fatalf("bounded run error code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if service.runID == "" {
		// The valid shape reached the runner; malformed cases below must not.
		t.Fatal("valid run did not reach runner")
	}
	for _, args := range [][]string{
		{"work", "run", "work-1"},
		{"work", "run", "work-1", "--expected-revision", "0", "--request-id", "id"},
		{"work", "run", "work-1", "--expected-revision", "x", "--request-id", "id"},
		{"work", "run", "work-1", "--request-id", "id"},
		{"work", "run", "work-1", "--expected-revision", "1"},
		{"work", "run", "-work-1", "--expected-revision", "1", "--request-id", "id"},
		{"work", "run", "work-1", "--expected-revision", "1", "--request-id", "-id"},
	} {
		before := service.runID
		code, out, errOut := runWorkflowService(t, service, args)
		if code != 2 || out != "" || !strings.Contains(errOut, "사용법:") || NeedsWorkflowDependencies(args) || service.runID != before {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q runID=%q", args, code, out, errOut, service.runID)
		}
	}
}

func TestWorkflowCommandsRouteAndSerializeOneJSONDocument(t *testing.T) {
	contractFile := t.TempDir() + "/contract.json"
	if err := writeTestFile(contractFile, `{"version":2}`); err != nil {
		t.Fatal(err)
	}
	projectFile := t.TempDir() + "/project.json"
	projectData, err := json.Marshal(workflowProject())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(projectFile, string(projectData)); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		args     []string
		setup    func(*fakeWorkflowService)
		wantCall string
		check    func(*testing.T, string)
	}{
		{name: "register", args: []string{"project", "register", projectFile, "--expected-revision", "0", "--request-id", "request-project-1"}, wantCall: "register:project-1", check: checkJSONKeys("projectId")},
		{name: "list", args: []string{"project", "list", "--json"}, setup: func(f *fakeWorkflowService) { f.projects = []registry.Project{workflowProject()} }, wantCall: "list", check: checkJSONKeys("schemaVersion", "projects")},
		{name: "status", args: []string{"project", "status", "--all", "--json"}, wantCall: "snapshot:", check: checkJSONKeys("schemaVersion", "observedAt", "freshness", "state", "syncStatus", "nextAction", "evidenceRefs", "projects")},
		{name: "plan", args: []string{"work", "plan", contractFile, "--expected-revision", "0", "--request-id", "request-plan-1"}, setup: func(f *fakeWorkflowService) { f.plan = workflowSnapshot() }, wantCall: "plan:" + contractFile, check: checkJSONKeys("schemaVersion", "projectId", "workId", "revision", "state", "syncStatus", "nextAction", "evidenceRefs")},
		{name: "approve", args: []string{"work", "approve", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, setup: func(f *fakeWorkflowService) { f.approved = workflowSnapshot() }, wantCall: "approve", check: checkJSONKeys("schemaVersion", "workId", "revision", "state")},
		{name: "work status", args: []string{"work", "status", "work-1", "--json"}, setup: func(f *fakeWorkflowService) { f.status = workflowSnapshot() }, wantCall: "status:work-1", check: checkJSONKeys("schemaVersion", "workId", "revision", "state")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeWorkflowService{snapshot: monitor.Snapshot{SchemaVersion: 2, State: "draft", SyncStatus: "local", EvidenceRefs: []string{}, Projects: []monitor.Project{}}}
			if tt.setup != nil {
				tt.setup(service)
			}
			code, out, errOut := runWorkflow(t, service, tt.args)
			if code != 0 || errOut != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
			if !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
				t.Fatalf("stdout=%q", out)
			}
			if len(service.calls) != 1 || !strings.HasPrefix(service.calls[0], tt.wantCall) {
				t.Fatalf("calls=%v want prefix %q", service.calls, tt.wantCall)
			}
			if tt.name == "approve" {
				if service.approveID != "work-1" || service.approveRevision != 1 || service.approveRequest != "request-1" {
					t.Fatalf("approve args=(%q,%d,%q), want (work-1,1,request-1)", service.approveID, service.approveRevision, service.approveRequest)
				}
			}
			tt.check(t, out)
		})
	}
}

func TestProjectRegisterRejectsNonObjectAndMalformedInputWithoutServiceCall(t *testing.T) {
	projectData, err := json.Marshal(workflowProject())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		input string
	}{
		{name: "unknown field", input: strings.TrimSuffix(string(projectData), "}") + `,"unexpected":true}`},
		{name: "trailing value", input: string(projectData) + ` {}`},
		{name: "null", input: "null"},
		{name: "malformed", input: "{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir() + "/project.json"
			if err := writeTestFile(path, tt.input); err != nil {
				t.Fatal(err)
			}
			service := &fakeWorkflowService{}
			code, out, errOut := runWorkflow(t, service, []string{"project", "register", path, "--expected-revision", "0", "--request-id", "request-invalid"})
			if code != 1 || out != "" || errOut == "" || len(service.calls) != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%v", code, out, errOut, service.calls)
			}
		})
	}
}

func checkJSONKeys(keys ...string) func(*testing.T, string) {
	return func(t *testing.T, output string) {
		var value map[string]any
		if err := json.Unmarshal([]byte(output), &value); err != nil {
			t.Fatal(err)
		}
		for _, key := range keys {
			if _, ok := value[key]; !ok {
				t.Fatalf("missing key %q in %s", key, output)
			}
		}
	}
}

func TestWorkflowMalformedCommandsStayDependencyFree(t *testing.T) {
	for _, args := range [][]string{
		{"project"}, {"project", "list"}, {"project", "status", "--json"}, {"work"},
		{"project", "register", "project.json"}, {"project", "register", "project.json", "--expected-revision", "1", "--request-id", "request-1"},
		{"work", "plan", "contract.json"}, {"work", "plan", "contract.json", "--expected-revision", "1", "--request-id", "request-1"},
		{"work", "approve", "work-1", "--expected-revision", "0", "--request-id", "request-1"},
		{"work", "status", "work-1"},
		{"work", "pause", "work-1"},
		{"work", "pause", "work-1", "--expected-revision", "0", "--request-id", "request-1"},
		{"work", "resume", "work-1", "--expected-revision", "1", "--request-id", ""},
		{"work", "reconcile", "work-1"},
		{"work", "reconcile", "work-1", "--json", "extra"},
	} {
		service := &fakeWorkflowService{}
		code, out, errOut := runWorkflow(t, service, args)
		if code != 2 || out != "" || errOut == "" || len(service.calls) != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q calls=%v", args, code, out, errOut, service.calls)
		}
		if NeedsWorkflowDependencies(args) {
			t.Fatalf("malformed args require dependencies: %v", args)
		}
	}
}

func TestWorkflowErrorsWriteNoJSONAndBoundedDiagnostic(t *testing.T) {
	service := &fakeWorkflowService{err: errors.New("provider leaked token secret")}
	code, out, errOut := runWorkflow(t, service, []string{"project", "list", "--json"})
	if code != 1 || out != "" || errOut == "" || strings.Contains(errOut, "provider") || strings.Contains(errOut, "token") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestNeedsWorkflowDependenciesOnlyForExactShapes(t *testing.T) {
	valid := [][]string{
		{"project", "register", "project.json", "--expected-revision", "0", "--request-id", "request-1"}, {"project", "list", "--json"}, {"project", "status", "--all", "--json"},
		{"work", "plan", "contract.json", "--expected-revision", "0", "--request-id", "request-1"}, {"work", "approve", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, {"work", "status", "work-1", "--json"},
		{"work", "pause", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, {"work", "resume", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, {"work", "reconcile", "work-1", "--json"},
	}
	for _, args := range valid {
		if !NeedsWorkflowDependencies(args) {
			t.Fatalf("valid args not recognized: %v", args)
		}
	}
}

func TestCreationCommandsRequireExpectedRevisionZeroAndRequestID(t *testing.T) {
	projectPath := t.TempDir() + "/project.json"
	if err := writeTestFile(projectPath, mustJSON(workflowProject())); err != nil {
		t.Fatal(err)
	}
	contractPath := t.TempDir() + "/contract.json"
	if err := writeTestFile(contractPath, `{"version":2}`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "project register", args: []string{"project", "register", projectPath, "--expected-revision", "0", "--request-id", "create-project-1"}},
		{name: "work plan", args: []string{"work", "plan", contractPath, "--expected-revision", "0", "--request-id", "create-work-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeWorkflowService{plan: workflowSnapshot()}
			code, _, errOut := runWorkflow(t, service, tc.args)
			if code != 0 || errOut != "" {
				t.Fatalf("code=%d stderr=%q calls=%v", code, errOut, service.calls)
			}
		})
	}
}

func TestOldCreationFormsExitTwoAndUsageNamesBothRequiredFlags(t *testing.T) {
	for _, args := range [][]string{
		{"project", "register", "PROJECT.json"},
		{"work", "plan", "CONTRACT.json"},
	} {
		service := &fakeWorkflowService{}
		code, out, errOut := runWorkflow(t, service, args)
		if code != 2 || out != "" || len(service.calls) != 0 {
			t.Fatalf("args=%v code=%d stdout=%q calls=%v", args, code, out, service.calls)
		}
		if !strings.Contains(errOut, "--expected-revision 0") || !strings.Contains(errOut, "--request-id ID") {
			t.Fatalf("args=%v usage=%q", args, errOut)
		}
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestProjectStatusUsesServiceSnapshotOnce(t *testing.T) {
	service := &fakeWorkflowService{snapshot: monitor.Snapshot{SchemaVersion: 2, EvidenceRefs: []string{}, Projects: []monitor.Project{{ProjectID: "project-1", EvidenceRefs: []string{}, WorkItems: []monitor.WorkItem{}}}}}
	code, out, errOut := runWorkflow(t, service, []string{"project", "status", "--all", "--json"})
	if code != 0 || errOut != "" || out == "" || len(service.calls) != 1 || !strings.HasPrefix(service.calls[0], "snapshot:") {
		t.Fatalf("code=%d out=%q err=%q calls=%v", code, out, errOut, service.calls)
	}
	if !strings.Contains(out, `"workItems":[]`) {
		t.Fatalf("missing empty workItems array: %s", out)
	}
}

func writeTestFile(path, value string) error {
	return os.WriteFile(path, []byte(value), 0600)
}
