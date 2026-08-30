package contract

import (
	"strings"
	"testing"
)

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

func TestValidateRejectsTaskPathsOverlappingProtectedPaths(t *testing.T) {
	tests := []struct {
		name        string
		allowedPath string
	}{
		{name: "exact", allowedPath: "migrations/**"},
		{name: "task parent prefix", allowedPath: ".github/**"},
		{name: "protected parent prefix", allowedPath: "deployment/services/**"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validContract()
			c.Tasks[0].AllowedPaths = []string{tt.allowedPath}

			got := Validate(c)
			if !hasViolation(got, "path_overlap", "tasks[0].allowedPaths") {
				t.Fatalf("violations = %#v", got)
			}
			for _, violation := range got {
				if violation.Code == "path_overlap" && !containsHangul(violation.Message) {
					t.Fatalf("non-Korean message = %#v", violation)
				}
			}
		})
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
		{"repository owner", func(c *TaskContract) { c.Repository.Owner = "" }},
		{"base commit", func(c *TaskContract) { c.BaseCommit = "" }},
		{"task owner", func(c *TaskContract) { c.Tasks[0].Owner = "" }},
		{"task role", func(c *TaskContract) { c.Tasks[0].Role = "" }},
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

func TestValidateAllowsContractWithoutChildIssues(t *testing.T) {
	c := validContract()
	c.Children = nil
	c.Tasks = c.Tasks[:1]
	c.Tasks[0].IssueKey = "parent"

	if got := Validate(c); len(got) != 0 {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateAllowsOptionalRiskCategories(t *testing.T) {
	c := validContract()
	c.RiskCategories = []string{"data", "authentication", "authorization", "deployment", "supply_chain", "public_contract"}

	if got := Validate(c); len(got) != 0 {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsUnknownAndDuplicateRiskCategories(t *testing.T) {
	c := validContract()
	c.RiskCategories = []string{"data", "data", "unknown"}

	got := Validate(c)
	if !hasViolation(got, "duplicate", "riskCategories[1]") {
		t.Fatalf("missing duplicate violation: %#v", got)
	}
	if !hasViolation(got, "required", "riskCategories[2]") {
		t.Fatalf("missing unknown-category violation: %#v", got)
	}
}

func TestIsRiskCategoryUsesContractAllowedSet(t *testing.T) {
	for _, category := range []string{"data", "authentication", "authorization", "deployment", "supply_chain", "public_contract"} {
		if !IsRiskCategory(category) {
			t.Fatalf("IsRiskCategory(%q) = false", category)
		}
	}
	for _, category := range []string{"", "DATA", "unknown", " data"} {
		if IsRiskCategory(category) {
			t.Fatalf("IsRiskCategory(%q) = true", category)
		}
	}
}

func TestValidateNormalizesPathOwnershipBeforeComparing(t *testing.T) {
	c := validContract()
	c.Tasks[0].AllowedPaths = []string{`./src\payments/**`}
	c.Tasks[1].AllowedPaths = []string{"src/payments/api/**"}

	if got := Validate(c); !hasCode(got, "path_overlap") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateRejectsPathTraversal(t *testing.T) {
	c := validContract()
	c.Tasks[0].AllowedPaths = []string{"src/../authentication/**"}

	if got := Validate(c); !hasCode(got, "path_overlap") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestValidateAttributesMalformedLeftAllowedPathOnce(t *testing.T) {
	c := validContract()
	c.Tasks[0].AllowedPaths = []string{"src/../broken/**"}

	got := pathOverlapViolations(Validate(c))
	if len(got) != 1 {
		t.Fatalf("path overlap violations = %#v", got)
	}
	if got[0].Field != "tasks[0].allowedPaths[0]" || !strings.Contains(got[0].Message, "src/../broken/**") {
		t.Fatalf("violation = %#v", got[0])
	}
}

func TestValidateAttributesMalformedRightAllowedPathOnce(t *testing.T) {
	c := validContract()
	c.Tasks[1].AllowedPaths = []string{"tests/../broken/**"}

	got := pathOverlapViolations(Validate(c))
	if len(got) != 1 {
		t.Fatalf("path overlap violations = %#v", got)
	}
	if got[0].Field != "tasks[1].allowedPaths[0]" || !strings.Contains(got[0].Message, "tests/../broken/**") {
		t.Fatalf("violation = %#v", got[0])
	}
}

func TestValidateAttributesMalformedProtectedPathOnce(t *testing.T) {
	c := validContract()
	c.Protected = append(c.Protected, "protected/../broken/**")

	got := pathOverlapViolations(Validate(c))
	if len(got) != 1 {
		t.Fatalf("path overlap violations = %#v", got)
	}
	if got[0].Field != "protectedPaths[4]" || !strings.Contains(got[0].Message, "protected/../broken/**") {
		t.Fatalf("violation = %#v", got[0])
	}
}

func TestValidateRecognizesNormalizedMandatoryProtectedPaths(t *testing.T) {
	c := validContract()
	c.Protected = []string{`./migrations/**`, `authentication\**`, `.github/./workflows/**`, "deployment/**"}

	for _, violation := range Validate(c) {
		if violation.Code == "required" && violation.Field == "protectedPaths" {
			t.Fatalf("normalized protected paths were reported missing: %#v", violation)
		}
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

func TestDefaultProtectedPathsIsCopySafe(t *testing.T) {
	first := DefaultProtectedPaths()
	first[0] = "replaced/**"

	second := DefaultProtectedPaths()
	if second[0] != "migrations/**" {
		t.Fatalf("protected paths = %#v", second)
	}
}

func TestValidateRequiresAllDefaultProtectedPaths(t *testing.T) {
	tests := []struct {
		name      string
		protected []string
	}{
		{"omitted", nil},
		{"partial", DefaultProtectedPaths()[:3]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validContract()
			c.Protected = tt.protected

			if got := Validate(c); !hasViolation(got, "required", "protectedPaths") {
				t.Fatalf("violations = %#v", got)
			}
		})
	}
}

func TestValidateUsesKoreanMessagesWithStableCodes(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*TaskContract)
		wantCode string
	}{
		{"required", func(c *TaskContract) { c.Tasks[0].Owner = "" }, "required"},
		{"duplicate", func(c *TaskContract) { c.Children[1].Key = "api" }, "duplicate"},
		{"path overlap", func(c *TaskContract) { c.Tasks[1].AllowedPaths = []string{"src/payments/**"} }, "path_overlap"},
		{"unsafe base", func(c *TaskContract) { c.Tasks[0].Branch = "main" }, "unsafe_base"},
		{"missing dependency", func(c *TaskContract) { c.Tasks[1].DependsOn = []string{"missing"} }, "missing_dependency"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validContract()
			tt.mutate(&c)
			got := Validate(c)

			if !hasCode(got, tt.wantCode) {
				t.Fatalf("violations = %#v", got)
			}
			for _, violation := range got {
				if !containsHangul(violation.Message) {
					t.Fatalf("non-Korean message = %#v", violation)
				}
			}
		})
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
			{ID: "api", IssueKey: "api", Owner: "api-builder", Role: "builder", Branch: "agent/api", AllowedPaths: []string{"src/payments/**"}, AcceptanceCriteria: []string{"재시도 한도를 지킨다"}, Verification: []string{"go test ./internal/payments"}},
			{ID: "tests", IssueKey: "tests", Owner: "test-builder", Role: "builder", Branch: "agent/tests", AllowedPaths: []string{"tests/payments/**"}, AcceptanceCriteria: []string{"중복 결제를 검증한다"}, Verification: []string{"go test ./tests/payments"}},
		},
		Protected:    DefaultProtectedPaths(),
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

func pathOverlapViolations(v []Violation) []Violation {
	var got []Violation
	for _, violation := range v {
		if violation.Code == "path_overlap" {
			got = append(got, violation)
		}
	}
	return got
}

func hasViolation(v []Violation, code, field string) bool {
	for _, item := range v {
		if item.Code == code && item.Field == field {
			return true
		}
	}
	return false
}

func containsHangul(value string) bool {
	for _, char := range value {
		if char >= '\uac00' && char <= '\ud7a3' {
			return true
		}
	}
	return false
}
