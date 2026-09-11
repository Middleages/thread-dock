package main

import (
	"path/filepath"
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
