package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProjectToolboxStoreKeepsProjectsIndependent(t *testing.T) {
	store := projectToolboxStore{path: filepath.Join(t.TempDir(), "projects.json")}
	first := ProjectToolbox{
		References: []ToolboxReference{{ID: "ref-1", Label: "Confluence", Type: "web", Target: "https://example.com/wiki"}},
		Commands:   []ToolboxCommand{{ID: "cmd-1", Label: "psql", Command: "psql -h localhost -U postgres"}},
		Checklist:  []ToolboxChecklistItem{{ID: "check-1", Text: "Smoke test", Done: true}},
	}
	if _, err := store.put("github.example/acme/app", first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.put("github.example/acme/other", ProjectToolbox{Checklist: []ToolboxChecklistItem{{ID: "check-2", Text: "Deploy"}}}); err != nil {
		t.Fatal(err)
	}

	got, err := store.get("github.example/acme/app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.References) != 1 || got.References[0].Label != "Confluence" || len(got.Commands) != 1 || len(got.Checklist) != 1 || !got.Checklist[0].Done {
		t.Fatalf("unexpected toolbox: %#v", got)
	}
}

func TestValidateProjectToolboxRejectsExecutableOrRelativeReferences(t *testing.T) {
	cases := []ProjectToolbox{
		{References: []ToolboxReference{{ID: "bad", Label: "script", Type: "web", Target: "javascript:alert(1)"}}},
		{References: []ToolboxReference{{ID: "bad", Label: "relative", Type: "file", Target: "docs/readme.pdf"}}},
		{References: []ToolboxReference{{ID: "bad", Label: "relative wsl", Type: "wsl-file", Target: "home/appuser/readme.md"}}},
	}
	for index, value := range cases {
		if err := validateProjectToolbox(value); err == nil {
			t.Fatalf("case %d should fail", index)
		}
	}
}

func TestProjectToolboxStoreReturnsEmptyForUnknownProject(t *testing.T) {
	store := projectToolboxStore{path: filepath.Join(t.TempDir(), "projects.json")}
	got, err := store.get("github.example/acme/missing")
	if err != nil {
		t.Fatal(err)
	}
	if got.References == nil || got.Commands == nil || got.Checklist == nil {
		t.Fatalf("empty toolbox slices must be non-nil: %#v", got)
	}
}

func TestAppProjectToolboxBindingsUseLocalStore(t *testing.T) {
	app := &App{toolbox: projectToolboxStore{path: filepath.Join(t.TempDir(), "projects.json")}}
	want := ProjectToolbox{Commands: []ToolboxCommand{{ID: "cmd-1", Label: "psql", Command: "psql -d app"}}}
	if _, err := app.SaveProjectToolbox("github.example/repo:acme/app", want); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetProjectToolbox("github.example/repo:acme/app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 1 || got.Commands[0].Command != "psql -d app" {
		t.Fatalf("unexpected toolbox: %#v", got)
	}
}

func TestAppOpenToolboxReferenceRejectsUnsafeTargetBeforeLaunch(t *testing.T) {
	app := &App{}
	if err := app.OpenToolboxReference("file", "relative/path.txt"); err == nil {
		t.Fatal("relative local paths must be rejected before launch")
	}
	if err := app.OpenToolboxReference("web", "javascript:alert(1)"); err == nil {
		t.Fatal("unsafe web targets must be rejected")
	}
}

func TestProjectReferencesStorePersistsOnlyReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	store := projectToolboxStore{path: path}
	want := ProjectReferences{References: []ToolboxReference{{ID: "ref-1", Label: "Wiki", Type: "web", Target: "https://example.com"}}}
	if _, err := store.putReferences("github.example/acme/app", want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	projects, ok := raw["projects"].(map[string]any)
	if !ok {
		t.Fatalf("missing projects: %s", data)
	}
	project := projects["github.example/acme/app"].(map[string]any)
	if _, ok := project["commands"]; ok {
		t.Fatal("project references file must not persist commands")
	}
	if _, ok := project["checklist"]; ok {
		t.Fatal("project references file must not persist checklist")
	}
	if _, ok := project["todos"]; ok {
		t.Fatal("project references file must not persist todos")
	}
	got, err := store.getReferences("github.example/acme/app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.References) != 1 || got.References[0].Target != "https://example.com" {
		t.Fatalf("unexpected references: %#v", got)
	}
}

func TestGlobalToolboxStorePersistsCommandsAndTodos(t *testing.T) {
	dir := t.TempDir()
	store := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	want := GlobalToolbox{
		Commands: []ToolboxCommand{{ID: "cmd-1", Label: "psql", Command: "psql -h localhost"}},
		Todos: []ToolboxTodo{
			{ID: "todo-common", Text: "GitHub token 확인"},
			{ID: "todo-jmj", Text: "staging migration 확인", ProjectKey: "github.example/repo:acme/jmj"},
		},
	}
	if _, err := store.put(projects, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 1 || len(got.Todos) != 2 || got.Todos[0].ProjectKey != "" || got.Todos[1].ProjectKey == "" {
		t.Fatalf("unexpected global toolbox: %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "toolbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "projects") {
		t.Fatalf("global toolbox must not contain project map: %s", data)
	}
}

func TestGlobalTodoAcceptsCommonOrOneProjectKey(t *testing.T) {
	valid := []GlobalToolbox{{Todos: []ToolboxTodo{{ID: "common", Text: "common"}}}, {Todos: []ToolboxTodo{{ID: "project", Text: "project", ProjectKey: "github.example/repo:acme/app"}}}}
	for _, value := range valid {
		if err := validateGlobalToolbox(value); err != nil {
			t.Fatalf("valid todo rejected: %v", err)
		}
	}
	invalid := GlobalToolbox{Todos: []ToolboxTodo{{ID: "bad", Text: "bad", ProjectKey: "valid\ninvalid"}}}
	if err := validateGlobalToolbox(invalid); err == nil {
		t.Fatal("invalid project key must be rejected")
	}
}

func writeLegacyToolbox(t *testing.T, path string, value projectToolboxFile) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func legacyFixture() projectToolboxFile {
	return projectToolboxFile{Version: toolboxVersion, Projects: map[string]ProjectToolbox{
		"github.example/repo:acme/jmj": {
			References: []ToolboxReference{{ID: "ref-jmj", Label: "JMJ Wiki", Type: "web", Target: "https://example.com/jmj"}},
			Commands:   []ToolboxCommand{{ID: "cmd-psql", Label: "psql", Command: "psql -h localhost"}},
			Checklist:  []ToolboxChecklistItem{{ID: "check-jmj", Text: "migration 확인", Done: false}},
		},
		"github.example/repo:acme/piece": {
			References: []ToolboxReference{},
			Commands:   []ToolboxCommand{{ID: "cmd-psql-2", Label: "psql", Command: "psql -h localhost"}},
			Checklist:  []ToolboxChecklistItem{{ID: "check-piece", Text: "migration 확인", Done: true}},
		},
	}}
}

func TestToolboxMigrationPreservesProjectReferences(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	if _, err := global.get(projects); err != nil {
		t.Fatal(err)
	}
	got, err := projects.getReferences("github.example/repo:acme/jmj")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.References) != 1 || got.References[0].ID != "ref-jmj" {
		t.Fatalf("references were not preserved: %#v", got)
	}
	data, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "commands") || strings.Contains(string(data), "checklist") {
		t.Fatalf("legacy fields remain after migration: %s", data)
	}
}

func TestToolboxMigrationMovesCommandsToGlobalStore(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	got, err := global.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 1 || got.Commands[0].Command != "psql -h localhost" {
		t.Fatalf("unexpected migrated commands: %#v", got.Commands)
	}
	if len(got.Todos) != 2 {
		t.Fatalf("unexpected migrated todos: %#v", got.Todos)
	}
}

func TestToolboxMigrationLinksLegacyChecklistToOriginProject(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	got, err := global.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, todo := range got.Todos {
		if todo.Text != "migration 확인" {
			t.Fatalf("unexpected todo: %#v", todo)
		}
		seen[todo.ProjectKey] = true
	}
	if !seen["github.example/repo:acme/jmj"] || !seen["github.example/repo:acme/piece"] {
		t.Fatalf("missing origin project links: %#v", seen)
	}
}

func TestToolboxMigrationDeduplicatesCommandsByLabelAndCommand(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	if _, err := global.get(projects); err != nil {
		t.Fatal(err)
	}
	got, err := global.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 1 {
		t.Fatalf("duplicate command was not removed: %#v", got.Commands)
	}
}

func TestToolboxMigrationKeepsSameTodoTextFromDifferentProjectsSeparate(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	got, err := global.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Todos) != 2 || got.Todos[0].ProjectKey == got.Todos[1].ProjectKey {
		t.Fatalf("same text from projects must remain separate: %#v", got.Todos)
	}
}

func TestToolboxMigrationIsIdempotentAfterPartialCompletion(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	failOnce := true
	projects.writer = func(path, pattern string, value any) error {
		if failOnce {
			failOnce = false
			return errors.New("simulated project rewrite failure")
		}
		return writeToolboxFile(path, pattern, value)
	}
	if _, err := global.get(projects); err == nil {
		t.Fatal("partial project rewrite must be reported")
	}
	got, err := global.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 1 || len(got.Todos) != 2 {
		t.Fatalf("retry duplicated migrated data: %#v", got)
	}
	if _, err := projects.getReferences("github.example/repo:acme/jmj"); err != nil {
		t.Fatal(err)
	}
}

func TestToolboxMigrationLeavesLegacyProjectFileUntouchedWhenGlobalWriteFails(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json"), writer: func(string, string, any) error { return errors.New("simulated global write failure") }}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	before, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := global.get(projects); err == nil {
		t.Fatal("global write failure must be reported")
	}
	after, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy project file changed after global write failure")
	}
}

func TestProjectReferencesStoreRejectsLegacyWriteWithoutMigration(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	writeLegacyToolbox(t, projects.path, legacyFixture())
	before, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.putReferences("github.example/repo:acme/jmj", ProjectReferences{}); err == nil {
		t.Fatal("references write must require migration for a legacy file")
	}
	after, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy file changed after rejected references write")
	}
}

func TestToolboxMigrationRejectsMalformedProjectBeforeWrites(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	if err := os.WriteFile(projects.path, []byte(`{"version":1,"projects":{"bad\nkey":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := global.get(projects); err == nil {
		t.Fatal("malformed project data must be rejected")
	}
	if _, err := os.Stat(global.path); !os.IsNotExist(err) {
		t.Fatalf("global file should not be written, stat error=%v", err)
	}
}

func TestToolboxMigrationRejectsMergedOverLimitBeforeWrites(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	legacy := projectToolboxFile{Version: toolboxVersion, Projects: map[string]ProjectToolbox{"github.example/repo:acme/a": {}, "github.example/repo:acme/b": {}}}
	for i := 0; i < 101; i++ {
		projectA := legacy.Projects["github.example/repo:acme/a"]
		projectA.Commands = append(projectA.Commands, ToolboxCommand{ID: "cmd-a-" + strconv.Itoa(i), Label: "label-a-" + strconv.Itoa(i), Command: "run-a-" + strconv.Itoa(i)})
		legacy.Projects["github.example/repo:acme/a"] = projectA
		projectB := legacy.Projects["github.example/repo:acme/b"]
		projectB.Commands = append(projectB.Commands, ToolboxCommand{ID: "cmd-b-" + strconv.Itoa(i), Label: "label-b-" + strconv.Itoa(i), Command: "run-b-" + strconv.Itoa(i)})
		legacy.Projects["github.example/repo:acme/b"] = projectB
	}
	writeLegacyToolbox(t, projects.path, legacy)
	before, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := global.get(projects); err == nil {
		t.Fatal("merged command limit must be rejected")
	}
	after, err := os.ReadFile(projects.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("project file changed after over-limit rejection")
	}
	if _, err := os.Stat(global.path); !os.IsNotExist(err) {
		t.Fatalf("global file should not be written, stat error=%v", err)
	}
}

func TestToolboxMigrationRekeysCollidingIDsDeterministically(t *testing.T) {
	dir := t.TempDir()
	projects := projectToolboxStore{path: filepath.Join(dir, "projects.json")}
	global := globalToolboxStore{path: filepath.Join(dir, "toolbox.json")}
	legacy := projectToolboxFile{Version: toolboxVersion, Projects: map[string]ProjectToolbox{
		"github.example/repo:acme/a": {Commands: []ToolboxCommand{{ID: "same", Label: "one", Command: "one"}}},
		"github.example/repo:acme/b": {Commands: []ToolboxCommand{{ID: "same", Label: "two", Command: "two"}}},
	}}
	writeLegacyToolbox(t, projects.path, legacy)
	got, err := global.get(projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 2 || got.Commands[0].ID == got.Commands[1].ID || !strings.HasPrefix(got.Commands[1].ID, "legacy-command-") {
		t.Fatalf("colliding IDs were not rekeyed: %#v", got.Commands)
	}
}
