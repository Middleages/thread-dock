package testfixture

import "thread-dock/internal/contract"

func ValidContract() contract.TaskContract {
	return contract.TaskContract{
		Version: 1,
		Parent: contract.IssueDraft{
			Key:                "parent",
			Title:              "결제 실패 재시도 개선",
			Body:               "실패한 결제를 안전하게 재시도한다.",
			AcceptanceCriteria: []string{"중복 결제가 없다"},
		},
		Children: []contract.IssueDraft{
			{Key: "api", Title: "재시도 정책 구현", Body: "API 변경", AcceptanceCriteria: []string{"재시도 한도를 지킨다"}},
			{Key: "tests", Title: "회귀 검증", Body: "집중 회귀 검사", AcceptanceCriteria: []string{"중복 결제를 검증한다"}},
		},
		Repository: contract.RepositoryRef{Owner: "platform", Name: "payments-api", DefaultBranch: "main"},
		BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		Tasks: []contract.Task{
			{ID: "api", IssueKey: "api", Role: "builder", Branch: "agent/api", AllowedPaths: []string{"src/payments/**"}, AcceptanceCriteria: []string{"재시도 한도를 지킨다"}, Verification: []string{"go test ./internal/payments"}},
			{ID: "tests", IssueKey: "tests", Role: "builder", Branch: "agent/tests", AllowedPaths: []string{"tests/payments/**"}, AcceptanceCriteria: []string{"중복 결제를 검증한다"}, Verification: []string{"go test ./tests/payments"}},
		},
		Protected:    []string{"migrations/**", "authentication/**", ".github/workflows/**", "deployment/**"},
		Verification: []string{"go test ./...", "go vet ./..."},
	}
}
