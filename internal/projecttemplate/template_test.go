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

func templatePath(name string) string {
	return filepath.Join("..", "..", "project-template", name)
}
