package contract

import "testing"

func TestValidateValidContract(t *testing.T) {
	got := Validate(validContract())
	if len(got) != 0 {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsOverlappingOwnedPaths(t *testing.T) {
	c := validContract()
	c.Tasks[1].AllowedPaths = []string{"src/payments/**"}

	got := Validate(c)
	if !hasCode(got, "path_overlap") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsMainAsBuilderBranch(t *testing.T) {
	c := validContract()
	c.Tasks[0].Branch = "main"

	got := Validate(c)
	if !hasCode(got, "unsafe_base") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsMissingRequiredContractValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TaskContract)
	}{
		{"parent key", func(c *TaskContract) { c.Parent.Key = "" }},
		{"children", func(c *TaskContract) { c.Children = nil }},
		{"repository owner", func(c *TaskContract) { c.Repository.Owner = "" }},
		{"base commit", func(c *TaskContract) { c.BaseCommit = "" }},
		{"task allowed paths", func(c *TaskContract) { c.Tasks[0].AllowedPaths = nil }},
		{"contract verification", func(c *TaskContract) { c.Verification = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validContract()
			tt.mutate(&c)

			if got := Validate(c); !hasCode(got, "required") {
				t.Fatalf("violations = %#v", got)
			}
		})
	}
}

func TestValidateRejectsUnsafeBaseCommit(t *testing.T) {
	c := validContract()
	c.BaseCommit = "0123456789abcdef0123456789abcdef0123456G"

	if got := Validate(c); !hasCode(got, "unsafe_base") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsUnsupportedContractVersion(t *testing.T) {
	c := validContract()
	c.Version = 2

	if got := Validate(c); !hasCode(got, "required") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsDuplicateIssueKeysAndTaskIDs(t *testing.T) {
	c := validContract()
	c.Children[1].Key = "api"
	c.Tasks[1].ID = "api"

	if got := Validate(c); !hasCode(got, "duplicate") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsMissingAndCyclicDependencies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TaskContract)
	}{
		{
			name: "missing task",
			mutate: func(c *TaskContract) {
				c.Tasks[1].DependsOn = []string{"does-not-exist"}
			},
		},
		{
			name: "cycle",
			mutate: func(c *TaskContract) {
				c.Tasks[0].DependsOn = []string{"tests"}
				c.Tasks[1].DependsOn = []string{"api"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validContract()
			tt.mutate(&c)

			if got := Validate(c); !hasCode(got, "missing_dependency") {
				t.Fatalf("violations = %#v", got)
			}
		})
	}
}

func TestValidateTreatsOnlyDirectoryPrefixesAsOverlapping(t *testing.T) {
	c := validContract()
	c.Tasks[1].AllowedPaths = []string{"src/payments-api/**"}

	if got := Validate(c); hasCode(got, "path_overlap") {
		t.Fatalf("violations = %#v", got)
	}
}

func validContract() TaskContract {
	return TaskContract{
		Version: 1,
		Parent:  IssueDraft{Key: "parent", Title: "결제 실패 재시도 개선", Body: "실패한 결제를 안전하게 재시도한다.", AcceptanceCriteria: []string{"중복 결제가 없다"}},
		Children: []IssueDraft{
			{Key: "api", Title: "재시도 정책 구현", Body: "API 변경", AcceptanceCriteria: []string{"재시도 한도를 지킨다"}},
			{Key: "tests", Title: "회귀 검증", Body: "집중 회귀 검사", AcceptanceCriteria: []string{"중복 결제를 검증한다"}},
		},
		Repository: RepositoryRef{Owner: "platform", Name: "payments-api", DefaultBranch: "main"},
		BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		Tasks: []Task{
			{ID: "api", IssueKey: "api", Role: "builder", Branch: "agent/api", AllowedPaths: []string{"src/payments/**"}, AcceptanceCriteria: []string{"재시도 한도를 지킨다"}, Verification: []string{"go test ./internal/payments"}},
			{ID: "tests", IssueKey: "tests", Role: "builder", Branch: "agent/tests", AllowedPaths: []string{"tests/payments/**"}, AcceptanceCriteria: []string{"중복 결제를 검증한다"}, Verification: []string{"go test ./tests/payments"}},
		},
		Protected:    []string{"migrations/**", "authentication/**", ".github/workflows/**", "deployment/**"},
		Verification: []string{"go test ./...", "go vet ./..."},
	}
}

func hasCode(v []Violation, code string) bool {
	for _, item := range v {
		if item.Code == code {
			return true
		}
	}
	return false
}
