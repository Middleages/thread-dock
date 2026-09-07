package contractv2

import (
	"fmt"
	"regexp"
	"strings"

	"thread-dock/internal/dag"
	"thread-dock/internal/pathscope"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Validate reports all semantic violations in a v2 contract.
func Validate(c WorkItemContract) []Violation {
	var violations []Violation
	if c.Version != CurrentVersion {
		violations = append(violations, violation("version", "version", "must be 2"))
	}
	if !stableID(string(c.WorkID)) {
		violations = append(violations, violation("workId", "required", "must be non-empty"))
	}
	if !stableID(string(c.ProjectID)) {
		violations = append(violations, violation("projectId", "required", "must be non-empty"))
	}
	if c.Revision == 0 {
		violations = append(violations, violation("revision", "required", "must be positive"))
	}
	if strings.TrimSpace(c.Request) == "" {
		violations = append(violations, violation("request", "required", "must be non-empty"))
	}
	if len(c.AcceptanceCriteria) == 0 {
		violations = append(violations, violation("acceptanceCriteria", "required", "must not be empty"))
	}

	repos := make(map[RepoKey]int, len(c.RepositoryPlans))
	for i, plan := range c.RepositoryPlans {
		field := fmt.Sprintf("repositoryPlans[%d]", i)
		if !stableID(string(plan.RepoKey)) {
			violations = append(violations, violation(field+".repoKey", "required", "must be non-empty"))
		}
		if _, ok := repos[plan.RepoKey]; ok {
			violations = append(violations, violation(field+".repoKey", "duplicate", "must be unique"))
		}
		repos[plan.RepoKey] = i
		if !shaPattern.MatchString(plan.BaseSHA) {
			violations = append(violations, violation(field+".baseSha", "invalid", "must be a 40-character lowercase SHA"))
		}
		if strings.TrimSpace(plan.TargetBranch) == "" {
			violations = append(violations, violation(field+".targetBranch", "required", "must be non-empty"))
		}
	}
	if len(c.RepositoryPlans) == 0 {
		violations = append(violations, violation("repositoryPlans", "required", "must not be empty"))
	}
	for i, plan := range c.RepositoryPlans {
		validateCommands(&violations, fmt.Sprintf("repositoryPlans[%d].verification", i), plan.Verification, repos)
	}

	taskIDs := make(map[TaskID]int, len(c.Tasks))
	nodes := make([]dag.Node, len(c.Tasks))
	for i, task := range c.Tasks {
		field := fmt.Sprintf("tasks[%d]", i)
		id := task.TaskID
		if id == "" {
			id = task.ID
		}
		if !stableID(string(id)) {
			violations = append(violations, violation(field+".taskId", "required", "must be non-empty"))
		}
		if _, ok := taskIDs[id]; ok {
			violations = append(violations, violation(field+".taskId", "duplicate", "must be unique"))
		}
		taskIDs[id] = i
		if _, ok := repos[task.RepoKey]; !ok {
			violations = append(violations, violation(field+".repoKey", "missing_reference", "must reference a repository plan"))
		}
		if len(task.AllowedPaths) == 0 {
			violations = append(violations, violation(field+".allowedPaths", "required", "must not be empty"))
		}
		for j, raw := range task.AllowedPaths {
			if _, err := pathscope.Normalize(raw); err != nil {
				violations = append(violations, violation(fmt.Sprintf("%s.allowedPaths[%d]", field, j), "invalid_path", err.Error()))
			}
		}
		if len(task.AcceptanceCriteria) == 0 {
			violations = append(violations, violation(field+".acceptanceCriteria", "required", "must not be empty"))
		}
		validateCommands(&violations, field+".verification", task.Verification, repos)
		nodes[i] = dag.Node{ID: string(id), DependsOn: taskIDsToStrings(task.DependsOn)}
	}
	_, graphViolations := dag.Build(nodes)
	for _, graphViolation := range graphViolations {
		code := "missing_dependency"
		if graphViolation.Code == dag.DependencyCycle {
			code = "dependency_cycle"
		}
		violations = append(violations, violation("tasks.dependsOn", code, graphViolation.Dependency))
	}
	for i, left := range c.Tasks {
		for j := i + 1; j < len(c.Tasks); j++ {
			right := c.Tasks[j]
			if left.RepoKey == "" || left.RepoKey != right.RepoKey {
				continue
			}
			for _, lp := range left.AllowedPaths {
				for _, rp := range right.AllowedPaths {
					overlaps, err := pathscope.Overlaps(lp, rp)
					if err == nil && overlaps {
						violations = append(violations, violation(fmt.Sprintf("tasks[%d].allowedPaths", j), "overlap", "allowed paths overlap another task"))
					}
				}
			}
		}
	}
	for i, command := range c.CrossRepoVerification {
		validateCommand(&violations, fmt.Sprintf("crossRepoVerification[%d]", i), command, repos)
	}
	for i, issue := range c.IssueDrafts {
		field := fmt.Sprintf("issueDrafts[%d]", i)
		if !stableID(issue.Key) {
			violations = append(violations, violation(field+".key", "required", "must be non-empty"))
		}
		if strings.TrimSpace(issue.Title) == "" {
			violations = append(violations, violation(field+".title", "required", "must be non-empty"))
		}
		if strings.TrimSpace(issue.Body) == "" {
			violations = append(violations, violation(field+".body", "required", "must be non-empty"))
		}
		if !stableID(string(issue.RepoKey)) {
			violations = append(violations, violation(field+".repoKey", "required", "must reference a repository plan"))
		} else {
			if _, ok := repos[issue.RepoKey]; !ok {
				violations = append(violations, violation(field+".repoKey", "missing_reference", "must reference a repository plan"))
			}
		}
	}
	for i, agreement := range c.InterfaceAgreements {
		field := fmt.Sprintf("interfaceAgreements[%d]", i)
		if strings.TrimSpace(agreement.Summary) == "" {
			violations = append(violations, violation(field+".summary", "required", "must be non-empty"))
		}
		for j, repo := range agreement.RepoKeys {
			if _, ok := repos[repo]; !ok {
				violations = append(violations, violation(fmt.Sprintf("%s.repoKeys[%d]", field, j), "missing_reference", "must reference a repository plan"))
			}
		}
	}
	if c.Documentation.Repository != "" {
		if _, ok := repos[c.Documentation.Repository]; !ok {
			violations = append(violations, violation("documentation.repository", "missing_reference", "must reference a repository plan"))
		}
	}
	for i, p := range c.Documentation.AllowedPaths {
		if _, err := pathscope.Normalize(p); err != nil {
			violations = append(violations, violation(fmt.Sprintf("documentation.allowedPaths[%d]", i), "invalid_path", err.Error()))
		}
	}
	if c.Documentation.Required && c.Documentation.Repository == "" && len(c.Documentation.WikiTargets) == 0 {
		violations = append(violations, violation("documentation", "required", "a repository or wiki target is required"))
	}
	if !c.Documentation.Required && strings.TrimSpace(c.Documentation.Reason) == "" && (c.Documentation.Repository == "" && len(c.Documentation.WikiTargets) == 0) {
		// A deliberate no-documentation plan records why no target exists.
		violations = append(violations, violation("documentation.reason", "required", "explain why documentation is not required"))
	}
	if strings.TrimSpace(c.ExecutionProfiles.Builder) == "" {
		violations = append(violations, violation("executionProfiles.builder", "required", "must be non-empty"))
	}
	if strings.TrimSpace(c.ExecutionProfiles.Reviewer) == "" {
		violations = append(violations, violation("executionProfiles.reviewer", "required", "must be non-empty"))
	}
	if strings.TrimSpace(c.ExecutionProfiles.Documenter) == "" {
		violations = append(violations, violation("executionProfiles.documenter", "required", "must be non-empty"))
	}
	return violations
}

func validateCommands(violations *[]Violation, field string, commands []CommandSpec, repos map[RepoKey]int) {
	for i, command := range commands {
		validateCommand(violations, fmt.Sprintf("%s[%d]", field, i), command, repos)
	}
}

func validateCommand(violations *[]Violation, field string, command CommandSpec, repos map[RepoKey]int) {
	argv := len(command.Argv) > 0
	shell := strings.TrimSpace(command.ShellScript) != ""
	if argv == shell {
		*violations = append(*violations, violation(field, "invalid_command", "exactly one of argv and shellScript is required"))
	}
	if _, ok := repos[command.CwdRepoKey]; !ok {
		*violations = append(*violations, violation(field+".cwdRepoKey", "missing_reference", "must reference a repository plan"))
	}
	if command.TimeoutSeconds == 0 {
		*violations = append(*violations, violation(field+".timeoutSeconds", "required", "must be positive"))
	}
	for i, arg := range command.Argv {
		if strings.TrimSpace(arg) == "" {
			*violations = append(*violations, violation(fmt.Sprintf("%s.argv[%d]", field, i), "invalid_command", "argument must be non-empty"))
		}
	}
}

func taskIDsToStrings(ids []TaskID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = string(id)
	}
	return result
}

func stableID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if r == '/' || r == '\\' || r == ':' || r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			return false
		}
	}
	return true
}
func violation(field, code, message string) Violation {
	return Violation{Code: code, Field: field, Message: message}
}
