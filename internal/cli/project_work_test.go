package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/monitor"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

type fakeWorkflowService struct {
	project   registry.Project
	projects  []registry.Project
	plan      statev2.WorkSnapshot
	approved  statev2.WorkSnapshot
	status    statev2.WorkSnapshot
	snapshot  monitor.Snapshot
	err       error
	calls     []string
	planInput string
	planPath  string
}

func (f *fakeWorkflowService) RegisterProject(_ context.Context, p registry.Project) (registry.Project, error) {
	f.calls = append(f.calls, "register:"+string(p.ProjectID))
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
func (f *fakeWorkflowService) PlanWork(_ context.Context, path string, r io.Reader) (statev2.WorkSnapshot, error) {
	f.calls = append(f.calls, "plan:"+path)
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
	f.calls = append(f.calls, "approve:"+string(id)+":"+string(rune(revision))+":"+string(request))
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
	var out, errOut bytes.Buffer
	code := RunWithDependencies(context.Background(), args, &out, &errOut, Dependencies{Workflow: service})
	return code, out.String(), errOut.String()
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
		{name: "register", args: []string{"project", "register", projectFile}, wantCall: "register:project-1", check: checkJSONKeys("projectId")},
		{name: "list", args: []string{"project", "list", "--json"}, setup: func(f *fakeWorkflowService) { f.projects = []registry.Project{workflowProject()} }, wantCall: "list", check: checkJSONKeys("schemaVersion", "projects")},
		{name: "status", args: []string{"project", "status", "--all", "--json"}, wantCall: "snapshot:", check: checkJSONKeys("schemaVersion", "observedAt", "freshness", "state", "syncStatus", "nextAction", "evidenceRefs", "projects")},
		{name: "plan", args: []string{"work", "plan", contractFile}, setup: func(f *fakeWorkflowService) { f.plan = workflowSnapshot() }, wantCall: "plan:" + contractFile, check: checkJSONKeys("schemaVersion", "projectId", "workId", "revision", "state", "syncStatus", "nextAction", "evidenceRefs")},
		{name: "approve", args: []string{"work", "approve", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, setup: func(f *fakeWorkflowService) { f.approved = workflowSnapshot() }, wantCall: "approve:work-1:", check: checkJSONKeys("schemaVersion", "workId", "revision", "state")},
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
			tt.check(t, out)
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
		{"work", "approve", "work-1", "--expected-revision", "0", "--request-id", "request-1"},
		{"work", "status", "work-1"},
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
		{"project", "register", "project.json"}, {"project", "list", "--json"}, {"project", "status", "--all", "--json"},
		{"work", "plan", "contract.json"}, {"work", "approve", "work-1", "--expected-revision", "1", "--request-id", "request-1"}, {"work", "status", "work-1", "--json"},
	}
	for _, args := range valid {
		if !NeedsWorkflowDependencies(args) {
			t.Fatalf("valid args not recognized: %v", args)
		}
	}
}

func TestProjectStatusUsesServiceSnapshotOnce(t *testing.T) {
	service := &fakeWorkflowService{snapshot: monitor.Snapshot{SchemaVersion: 2, EvidenceRefs: []string{}, Projects: []monitor.Project{}}}
	code, out, errOut := runWorkflow(t, service, []string{"project", "status", "--all", "--json"})
	if code != 0 || errOut != "" || out == "" || len(service.calls) != 1 || !strings.HasPrefix(service.calls[0], "snapshot:") {
		t.Fatalf("code=%d out=%q err=%q calls=%v", code, out, errOut, service.calls)
	}
}

func writeTestFile(path, value string) error {
	return os.WriteFile(path, []byte(value), 0600)
}
