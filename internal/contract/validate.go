package contract

import (
	"fmt"
	"strings"
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
	validateProtectedPaths(&violations, c.Protected)
	requireStrings(&violations, "verification", c.Verification)

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

	for i, task := range c.Tasks {
		for _, dependency := range task.DependsOn {
			if _, exists := taskIDs[dependency]; !exists || strings.TrimSpace(dependency) == "" {
				*violations = append(*violations, Violation{
					Code:    "missing_dependency",
					Field:   fmt.Sprintf("tasks[%d].dependsOn", i),
					Message: fmt.Sprintf("Task 의존성 %q가 존재하지 않습니다", dependency),
				})
			}
		}
	}

	if hasDependencyCycle(c.Tasks) {
		*violations = append(*violations, Violation{
			Code:    "missing_dependency",
			Field:   "tasks.dependsOn",
			Message: "Task 의존성은 순환할 수 없습니다",
		})
	}

	validatePathOwnership(violations, c.Tasks, c.Protected)
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

func validateProtectedPaths(violations *[]Violation, paths []string) {
	requireStrings(violations, "protectedPaths", paths)
	present := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		present[path] = struct{}{}
	}
	for _, requiredPath := range defaultProtectedPaths {
		if _, ok := present[requiredPath]; !ok {
			*violations = append(*violations, Violation{
				Code:    "required",
				Field:   "protectedPaths",
				Message: fmt.Sprintf("보호 경로 %q가 필요합니다", requiredPath),
			})
		}
	}
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

func hasDependencyCycle(tasks []Task) bool {
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		dependencies[task.ID] = task.DependsOn
	}

	const (
		unvisited = iota
		visiting
		visited
	)
	states := make(map[string]int, len(tasks))
	var visit func(string) bool
	visit = func(id string) bool {
		switch states[id] {
		case visiting:
			return true
		case visited:
			return false
		}
		states[id] = visiting
		for _, dependency := range dependencies[id] {
			if _, exists := dependencies[dependency]; exists && visit(dependency) {
				return true
			}
		}
		states[id] = visited
		return false
	}

	for id := range dependencies {
		if visit(id) {
			return true
		}
	}
	return false
}

func validatePathOwnership(violations *[]Violation, tasks []Task, protectedPaths []string) {
	for i, left := range tasks {
		for j := i + 1; j < len(tasks); j++ {
			for _, leftPath := range left.AllowedPaths {
				for _, rightPath := range tasks[j].AllowedPaths {
					if pathsOverlap(leftPath, rightPath) {
						*violations = append(*violations, Violation{
							Code:    "path_overlap",
							Field:   fmt.Sprintf("tasks[%d].allowedPaths", j),
							Message: fmt.Sprintf("경로 %q가 Task %q의 경로 %q와 겹칩니다", rightPath, left.ID, leftPath),
						})
					}
				}
			}
		}
		for _, allowedPath := range left.AllowedPaths {
			for _, protectedPath := range protectedPaths {
				if pathsOverlap(allowedPath, protectedPath) {
					*violations = append(*violations, Violation{
						Code:    "path_overlap",
						Field:   fmt.Sprintf("tasks[%d].allowedPaths", i),
						Message: fmt.Sprintf("허용 경로 %q가 보호 경로 %q와 겹칩니다", allowedPath, protectedPath),
					})
				}
			}
		}
	}
}

func pathsOverlap(left, right string) bool {
	left = strings.TrimSuffix(left, "/**")
	right = strings.TrimSuffix(right, "/**")
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}
