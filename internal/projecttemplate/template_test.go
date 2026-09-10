package projecttemplate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredFiles(t *testing.T) {
	required := []string{
		"AGENTS.md",
		"CONTEXT.md",
		"docs/architecture/CODEMAP.md",
		".github/ISSUE_TEMPLATE/development-request.yml",
		".github/pull_request_template.md",
		".github/workflows/ci.yml",
		".agents/skills/plan-work/SKILL.md",
		".agents/skills/implement-task/SKILL.md",
		".agents/skills/review-change/SKILL.md",
	}

	for _, name := range required {
		if _, err := os.Stat(templatePath(name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestSkillFrontmatter(t *testing.T) {
	for _, skill := range []string{"plan-work", "implement-task", "review-change"} {
		t.Run(skill, func(t *testing.T) {
			data, err := os.ReadFile(templatePath(filepath.Join(".agents", "skills", skill, "SKILL.md")))
			if err != nil {
				t.Fatalf("read skill: %v", err)
			}

			frontmatter := string(data)
			if !strings.HasPrefix(frontmatter, "---\n") {
				t.Fatal("frontmatter must start with a delimiter")
			}
			end := strings.Index(frontmatter[4:], "\n---\n")
			if end < 0 {
				t.Fatal("frontmatter must end with a delimiter")
			}
			frontmatter = frontmatter[:end+4]
			if !strings.Contains(frontmatter, "name: "+skill+"\n") {
				t.Fatalf("frontmatter name must be %q", skill)
			}
			if !strings.Contains(frontmatter, "description: Use when ") {
				t.Fatal("description must start with Use when")
			}
		})
	}
}

func TestPlanWorkSkillDescribesContractV2Planning(t *testing.T) {
	content := readSkill(t, "plan-work")
	requireContains(t, content,
		"version: 2",
		"workId",
		"projectId",
		"revision",
		"request",
		"acceptanceCriteria",
		"repositoryPlans",
		"tasks",
		"repoKey",
		"baseSha",
		"targetBranch",
		"verification",
		"prOrder",
		"mergeOrder",
		"argv",
		"cwdRepoKey",
		"timeoutSeconds",
		"shellScript",
		"issueDrafts",
		"key",
		"title",
		"body",
		"documentation",
		"required",
		"wikiTargets",
		"allowedPaths",
		"executionProfiles",
		"builder",
		"reviewer",
		"documenter",
		"decisionRefs",
		"interfaceAgreements",
		"agreementId",
		"summary",
		"repoKeys",
		"crossRepoVerification",
		"agentctl contract validate CONTRACT.json",
		"agentctl contract preview CONTRACT.json",
		"approval",
	)
	requireNotContains(t, content,
		"version: 1",
		"version `1`",
		"`parent`",
		"`children`",
		"protectedPaths",
		"`protectedPaths`",
		"baseCommit",
		"`baseCommit`",
		"issueKey",
		"`issueKey`",
	)
}

func TestImplementTaskSkillUsesInvocationArtifactBoundary(t *testing.T) {
	content := readSkill(t, "implement-task")
	requireContains(t, content,
		"requestId",
		"role",
		"profileId",
		"worktree",
		"readOnly",
		"packet",
		"logical profile",
		"canonical Worktree",
		"allowedPaths",
		"output schema",
		"workspace-write",
		"Artifact",
		"status",
		"result",
		"Builder result",
		"focused",
		"TDD",
		"Go owns",
		"staging",
		"commit",
		"authoritative verification",
		"production behavior changes",
		"docs/config-only",
		"test-inapplicable",
		"packet-approved",
		"document/config validation",
		"without inventing a failing test",
	)
	requireNotContains(t, content,
		"task_id:",
		"changed_paths:",
		"commit_sha:",
		"checks:",
		"Builder commits",
		"Builder owns the commit",
	)
}

func TestReviewChangeSkillUsesFreshReadOnlyReviewerArtifact(t *testing.T) {
	content := readSkill(t, "review-change")
	requireContains(t, content,
		"fresh",
		"read-only",
		"requestId",
		"role",
		"profile",
		"candidate SHA",
		"candidateSha",
		"diff",
		"gate",
		"criteria",
		"repair budget",
		"repairBudget",
		"Artifact",
		"Reviewer result",
		"accept",
		"block",
		"blockingFindings",
		"reviewedSHA",
		"reviewedSha",
		"no mutation",
	)
	requireNotContains(t, content,
		"status: approved",
		"repair_round:",
	)
}

func readSkill(t *testing.T, skill string) string {
	t.Helper()
	data, err := os.ReadFile(templatePath(filepath.Join(".agents", "skills", skill, "SKILL.md")))
	if err != nil {
		t.Fatalf("read %s skill: %v", skill, err)
	}
	return string(data)
}

func requireContains(t *testing.T, content string, markers ...string) {
	t.Helper()
	for _, marker := range markers {
		if !strings.Contains(content, marker) {
			t.Errorf("skill must describe %q", marker)
		}
	}
}

func requireNotContains(t *testing.T, content string, markers ...string) {
	t.Helper()
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			t.Errorf("skill must not contain v1 or ownership marker %q", marker)
		}
	}
}

func templatePath(name string) string {
	return filepath.Join("..", "..", "project-template", name)
}
