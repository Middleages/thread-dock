package contractv2

import "testing"

func validContract() WorkItemContract {
	return WorkItemContract{
		Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1,
		Request: "ship it", AcceptanceCriteria: []string{"works"},
		RepositoryPlans:   []RepositoryPlan{{RepoKey: "app", BaseSHA: "0123456789012345678901234567890123456789", TargetBranch: "main"}},
		Tasks:             []Task{{TaskID: "task-1", RepoKey: "app", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}}},
		Documentation:     DocumentationPlan{Required: false, Reason: "not required"},
		ExecutionProfiles: ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"},
	}
}

func TestValidateRejectsCoreContractBreaks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WorkItemContract)
	}{
		{"version other than 2", func(c *WorkItemContract) { c.Version = 1 }},
		{"non-positive revision", func(c *WorkItemContract) { c.Revision = 0 }},
		{"missing repoKey reference", func(c *WorkItemContract) { c.Tasks[0].RepoKey = "missing" }},
		{"cyclic task dependency", func(c *WorkItemContract) {
			c.Tasks = []Task{{TaskID: "a", RepoKey: "app", AllowedPaths: []string{"a"}, DependsOn: []TaskID{"b"}}, {TaskID: "b", RepoKey: "app", AllowedPaths: []string{"b"}, DependsOn: []TaskID{"a"}}}
		}},
		{"missing task dependency", func(c *WorkItemContract) { c.Tasks[0].DependsOn = []TaskID{"missing"} }},
		{"overlapping allowed paths", func(c *WorkItemContract) {
			c.Tasks = append(c.Tasks, Task{TaskID: "task-2", RepoKey: "app", AllowedPaths: []string{"internal/**"}, AcceptanceCriteria: []string{"works"}})
		}},
		{"both argv and shell", func(c *WorkItemContract) {
			c.CrossRepoVerification = []CommandSpec{{Argv: []string{"go", "test"}, ShellScript: "go test", CwdRepoKey: "app", TimeoutSeconds: 30}}
		}},
		{"neither argv nor shell", func(c *WorkItemContract) {
			c.CrossRepoVerification = []CommandSpec{{CwdRepoKey: "app", TimeoutSeconds: 30}}
		}},
		{"zero timeout", func(c *WorkItemContract) {
			c.CrossRepoVerification = []CommandSpec{{Argv: []string{"go", "test"}, CwdRepoKey: "app"}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contract := validContract()
			tc.mutate(&contract)
			if got := Validate(contract); len(got) == 0 {
				t.Fatalf("Validate accepted %s", tc.name)
			}
		})
	}
}
