package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

func validContract() contractv2.WorkItemContract {
	return contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "ship it", AcceptanceCriteria: []string{"works"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}}}, Documentation: contractv2.DocumentationPlan{Required: false, Reason: "not required"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
}

func TestPlanApproveStatus(t *testing.T) {
	root := t.TempDir()
	projects := registry.NewStore(root)
	works := statev2.NewStore(root)
	if _, err := projects.Create(context.Background(), registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}); err != nil {
		t.Fatal(err)
	}
	s := New(projects, works)
	var input strings.Builder
	if err := contractv2.Write(&input, validContract()); err != nil {
		t.Fatal(err)
	}
	planned, err := s.PlanWork(context.Background(), "request.json", strings.NewReader(input.String()))
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
	if _, err := projects.Create(context.Background(), registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"}}}); err != nil {
		t.Fatal(err)
	}
	s := New(projects, works)
	var legacy *UnsupportedLegacyError
	_, err := s.PlanWork(context.Background(), "legacy.json", strings.NewReader(`{"version":1,"projectId":"project-1"}`))
	if !errors.As(err, &legacy) || legacy.Code != "unsupported_legacy" || legacy.Source != "legacy.json" {
		t.Fatalf("legacy error = %v", err)
	}
	if _, err := works.Load(context.Background(), "work-1"); err == nil {
		t.Fatal("legacy plan wrote state")
	}
}
