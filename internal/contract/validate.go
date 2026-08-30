package contract

import (
	"fmt"
	"strings"

	"thread-dock/internal/dag"
	"thread-dock/internal/pathscope"
)

// Validate reports every violation of the version 1 task contract.
func Validate(c TaskContract) []Violation {
	var violations []Violation

	if c.Version != CurrentVersion {
		violations = append(violations, Violation{
			Code:    "required",
			Field:   "version",
			Message: fmt.Sprintf("계약 버전은 %d이어야 합니다", CurrentVersion),
		})
	}

	validateIssue(&violations, "parent", c.Parent)
	validateIssues(&violations, "children", c.Children)
	requireString(&violations, "repository.owner", c.Repository.Owner)
	requireString(&violations, "repository.name", c.Repository.Name)
	requireString(&violations, "repository.defaultBranch", c.Repository.DefaultBranch)
	requireString(&violations, "baseCommit", c.BaseCommit)

	if !isLowerHexCommit(c.BaseCommit) {
		violations = append(violations, Violation{
			Code:    "unsafe_base",
			Field:   "baseCommit",
			Message: "기준 커밋은 40자리 소문자 16진수 SHA여야 합니다",
		})
	}

	validateTasks(&violations, c)
	taskPaths := normalizeTaskPaths(&violations, c.Tasks)
	protectedPaths := validateProtectedPaths(&violations, c.Protected)
	validatePathOwnership(&violations, c.Tasks, taskPaths, protectedPaths)
	requireStrings(&violations, "verification", c.Verification)
	validateRiskCategories(&violations, c.RiskCategories)

	return violations
}

func validateIssues(violations *[]Violation, field string, issues []IssueDraft) {
	for i, issue := range issues {
		validateIssue(violations, fmt.Sprintf("%s[%d]", field, i), issue)
	}

}

func validateIssue(violations *[]Violation, field string, issue IssueDraft) {
	requireString(violations, field+".key", issue.Key)
	requireString(violations, field+".title", issue.Title)
	requireString(violations, field+".body", issue.Body)
	requireStrings(violations, field+".acceptanceCriteria", issue.AcceptanceCriteria)
}

func validateTasks(violations *[]Violation, c TaskContract) {
	if len(c.Tasks) == 0 {
		*violations = append(*violations, requiredViolation("tasks"))
	}

	issueKeys := map[string]struct{}{c.Parent.Key: {}}
	for _, issue := range c.Children {
		if _, exists := issueKeys[issue.Key]; exists && strings.TrimSpace(issue.Key) != "" {
			*violations = append(*violations, Violation{
				Code:    "duplicate",
				Field:   "issues",
				Message: fmt.Sprintf("Issue 키 %q가 중복됩니다", issue.Key),
			})
		}
		issueKeys[issue.Key] = struct{}{}
	}

	taskIDs := make(map[string]struct{}, len(c.Tasks))
	for i, task := range c.Tasks {
		field := fmt.Sprintf("tasks[%d]", i)
		requireString(violations, field+".id", task.ID)
		requireString(violations, field+".issueKey", task.IssueKey)
		requireString(violations, field+".owner", task.Owner)
		requireString(violations, field+".role", task.Role)
		requireString(violations, field+".branch", task.Branch)
		requireStrings(violations, field+".allowedPaths", task.AllowedPaths)
		requireStrings(violations, field+".acceptanceCriteria", task.AcceptanceCriteria)
		requireStrings(violations, field+".verification", task.Verification)

		if _, exists := taskIDs[task.ID]; exists && strings.TrimSpace(task.ID) != "" {
			*violations = append(*violations, Violation{
				Code:    "duplicate",
				Field:   field + ".id",
				Message: fmt.Sprintf("Task ID %q가 중복됩니다", task.ID),
			})
		}
		taskIDs[task.ID] = struct{}{}

		if _, exists := issueKeys[task.IssueKey]; !exists && strings.TrimSpace(task.IssueKey) != "" {
			*violations = append(*violations, Violation{
				Code:    "missing_dependency",
				Field:   field + ".issueKey",
				Message: fmt.Sprintf("Issue 키 %q가 존재하지 않습니다", task.IssueKey),
			})
		}
		if strings.EqualFold(task.Role, "builder") && (task.Branch == "main" || task.Branch == c.Repository.DefaultBranch) {
			*violations = append(*violations, Violation{
				Code:    "unsafe_base",
				Field:   field + ".branch",
				Message: "Builder 브랜치는 기본 브랜치를 사용할 수 없습니다",
			})
		}
	}

	graphNodes := make([]dag.Node, len(c.Tasks))
	for i, task := range c.Tasks {
		graphNodes[i] = dag.Node{ID: task.ID, DependsOn: task.DependsOn}
	}
	_, graphViolations := dag.Build(graphNodes)
	for _, graphViolation := range graphViolations {
		switch graphViolation.Code {
		case dag.MissingDependency:
			index := taskIndex(c.Tasks, graphViolation.NodeID)
			*violations = append(*violations, Violation{
				Code:    "missing_dependency",
				Field:   fmt.Sprintf("tasks[%d].dependsOn", index),
				Message: fmt.Sprintf("Task 의존성 %q가 존재하지 않습니다", graphViolation.Dependency),
			})
		case dag.DependencyCycle:
			*violations = append(*violations, Violation{
				Code:    "missing_dependency",
				Field:   "tasks.dependsOn",
				Message: "Task 의존성은 순환할 수 없습니다",
			})
		}
	}

}

func requireString(violations *[]Violation, field, value string) {
	if strings.TrimSpace(value) == "" {
		*violations = append(*violations, requiredViolation(field))
	}
}

func requireStrings(violations *[]Violation, field string, values []string) {
	if len(values) == 0 {
		*violations = append(*violations, requiredViolation(field))
		return
	}
	for i, value := range values {
		requireString(violations, fmt.Sprintf("%s[%d]", field, i), value)
	}
}

func requiredViolation(field string) Violation {
	return Violation{Code: "required", Field: field, Message: "필수 항목입니다"}
}

type indexedPath struct {
	raw        string
	normalized string
}

func normalizeTaskPaths(violations *[]Violation, tasks []Task) [][]indexedPath {
	normalized := make([][]indexedPath, len(tasks))
	for i, task := range tasks {
		for pathIndex, rawPath := range task.AllowedPaths {
			path, err := pathscope.Normalize(rawPath)
			if err != nil {
				appendInvalidPathViolation(violations, fmt.Sprintf("tasks[%d].allowedPaths[%d]", i, pathIndex), rawPath)
				continue
			}
			normalized[i] = append(normalized[i], indexedPath{raw: rawPath, normalized: path})
		}
	}
	return normalized
}

func validateProtectedPaths(violations *[]Violation, paths []string) []indexedPath {
	requireStrings(violations, "protectedPaths", paths)
	valid := make([]indexedPath, 0, len(paths))
	present := make(map[string]struct{}, len(paths))
	for i, rawPath := range paths {
		path, err := pathscope.Normalize(rawPath)
		if err != nil {
			appendInvalidPathViolation(violations, fmt.Sprintf("protectedPaths[%d]", i), rawPath)
			continue
		}
		valid = append(valid, indexedPath{raw: rawPath, normalized: path})
		present[path] = struct{}{}
	}
	for _, requiredPath := range defaultProtectedPaths {
		normalizedRequired, err := pathscope.Normalize(requiredPath)
		if err != nil {
			continue
		}
		if _, ok := present[normalizedRequired]; !ok {
			*violations = append(*violations, Violation{
				Code:    "required",
				Field:   "protectedPaths",
				Message: fmt.Sprintf("보호 경로 %q가 필요합니다", requiredPath),
			})
		}
	}
	return valid
}

func isLowerHexCommit(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}

func validatePathOwnership(violations *[]Violation, tasks []Task, taskPaths [][]indexedPath, protectedPaths []indexedPath) {
	for i, left := range tasks {
		for j := i + 1; j < len(tasks); j++ {
			for _, leftPath := range taskPaths[i] {
				for _, rightPath := range taskPaths[j] {
					overlaps, err := pathscope.Overlaps(leftPath.normalized, rightPath.normalized)
					if err == nil && overlaps {
						*violations = append(*violations, Violation{
							Code:    "path_overlap",
							Field:   fmt.Sprintf("tasks[%d].allowedPaths", j),
							Message: fmt.Sprintf("경로 %q가 Task %q의 경로 %q와 겹칩니다", rightPath.raw, left.ID, leftPath.raw),
						})
					}
				}
			}
		}
		for _, allowedPath := range taskPaths[i] {
			for _, protectedPath := range protectedPaths {
				overlaps, err := pathscope.Overlaps(allowedPath.normalized, protectedPath.normalized)
				if err == nil && overlaps {
					*violations = append(*violations, Violation{
						Code:    "path_overlap",
						Field:   fmt.Sprintf("tasks[%d].allowedPaths", i),
						Message: fmt.Sprintf("허용 경로 %q가 보호 경로 %q와 겹칩니다", allowedPath.raw, protectedPath.raw),
					})
				}
			}
		}
	}
}

func appendInvalidPathViolation(violations *[]Violation, field, path string) {
	*violations = append(*violations, Violation{
		Code:    "path_overlap",
		Field:   field,
		Message: fmt.Sprintf("경로 %q가 올바른 저장소 상대 경로가 아닙니다", path),
	})
}

func taskIndex(tasks []Task, id string) int {
	for i, task := range tasks {
		if task.ID == id {
			return i
		}
	}
	return 0
}

var allowedRiskCategories = map[string]struct{}{
	"data":            {},
	"authentication":  {},
	"authorization":   {},
	"deployment":      {},
	"supply_chain":    {},
	"public_contract": {},
}

// IsRiskCategory reports whether category is one of the contract's exact
// allowed risk categories. Callers must pass the canonical spelling.
func IsRiskCategory(category string) bool {
	_, ok := allowedRiskCategories[category]
	return ok
}

func validateRiskCategories(violations *[]Violation, categories []string) {
	seen := make(map[string]struct{}, len(categories))
	for i, category := range categories {
		field := fmt.Sprintf("riskCategories[%d]", i)
		if _, exists := seen[category]; exists {
			*violations = append(*violations, Violation{
				Code:    "duplicate",
				Field:   field,
				Message: fmt.Sprintf("위험 범주 %q가 중복됩니다", category),
			})
		}
		seen[category] = struct{}{}
		if !IsRiskCategory(category) {
			*violations = append(*violations, Violation{
				Code:    "required",
				Field:   field,
				Message: fmt.Sprintf("위험 범주 %q를 사용할 수 없습니다", category),
			})
		}
	}
}
