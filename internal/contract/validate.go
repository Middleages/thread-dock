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
			Message: fmt.Sprintf("must be %d", CurrentVersion),
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
			Message: "must be a 40-character lowercase hexadecimal commit SHA",
		})
	}

	validateTasks(&violations, c)
	requireStrings(&violations, "protectedPaths", c.Protected)
	requireStrings(&violations, "verification", c.Verification)

	return violations
}

func validateIssues(violations *[]Violation, field string, issues []IssueDraft) {
	if len(issues) == 0 {
		*violations = append(*violations, requiredViolation(field))
	}
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
				Message: fmt.Sprintf("issue key %q is duplicated", issue.Key),
			})
		}
		issueKeys[issue.Key] = struct{}{}
	}

	taskIDs := make(map[string]struct{}, len(c.Tasks))
	for i, task := range c.Tasks {
		field := fmt.Sprintf("tasks[%d]", i)
		requireString(violations, field+".id", task.ID)
		requireString(violations, field+".issueKey", task.IssueKey)
		requireString(violations, field+".role", task.Role)
		requireString(violations, field+".branch", task.Branch)
		requireStrings(violations, field+".allowedPaths", task.AllowedPaths)
		requireStrings(violations, field+".acceptanceCriteria", task.AcceptanceCriteria)
		requireStrings(violations, field+".verification", task.Verification)

		if _, exists := taskIDs[task.ID]; exists && strings.TrimSpace(task.ID) != "" {
			*violations = append(*violations, Violation{
				Code:    "duplicate",
				Field:   field + ".id",
				Message: fmt.Sprintf("task ID %q is duplicated", task.ID),
			})
		}
		taskIDs[task.ID] = struct{}{}

		if _, exists := issueKeys[task.IssueKey]; !exists && strings.TrimSpace(task.IssueKey) != "" {
			*violations = append(*violations, Violation{
				Code:    "missing_dependency",
				Field:   field + ".issueKey",
				Message: fmt.Sprintf("issue key %q does not exist", task.IssueKey),
			})
		}
		if strings.EqualFold(task.Role, "builder") && (task.Branch == "main" || task.Branch == c.Repository.DefaultBranch) {
			*violations = append(*violations, Violation{
				Code:    "unsafe_base",
				Field:   field + ".branch",
				Message: "builder branches must not use the default branch",
			})
		}
	}

	for i, task := range c.Tasks {
		for _, dependency := range task.DependsOn {
			if _, exists := taskIDs[dependency]; !exists || strings.TrimSpace(dependency) == "" {
				*violations = append(*violations, Violation{
					Code:    "missing_dependency",
					Field:   fmt.Sprintf("tasks[%d].dependsOn", i),
					Message: fmt.Sprintf("task dependency %q does not exist", dependency),
				})
			}
		}
	}

	if hasDependencyCycle(c.Tasks) {
		*violations = append(*violations, Violation{
			Code:    "missing_dependency",
			Field:   "tasks.dependsOn",
			Message: "task dependencies must be acyclic",
		})
	}

	validatePathOwnership(violations, c.Tasks)
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
	return Violation{Code: "required", Field: field, Message: "is required"}
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

func validatePathOwnership(violations *[]Violation, tasks []Task) {
	for i, left := range tasks {
		for j := i + 1; j < len(tasks); j++ {
			for _, leftPath := range left.AllowedPaths {
				for _, rightPath := range tasks[j].AllowedPaths {
					if pathsOverlap(leftPath, rightPath) {
						*violations = append(*violations, Violation{
							Code:    "path_overlap",
							Field:   fmt.Sprintf("tasks[%d].allowedPaths", j),
							Message: fmt.Sprintf("path %q overlaps task %q path %q", rightPath, left.ID, leftPath),
						})
					}
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
