package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSnapshotSource struct {
	snapshot Snapshot
	err      error
	calls    int
}

func (f *fakeSnapshotSource) FetchAll(context.Context) (Snapshot, error) {
	f.calls++
	return f.snapshot, f.err
}

func TestGetMonitorSnapshotDelegatesToSnapshotSource(t *testing.T) {
	want := Snapshot{SchemaVersion: 2, Projects: []Project{}}
	source := &fakeSnapshotSource{snapshot: want}
	app := NewApp(source)

	got, err := app.GetMonitorSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != want.SchemaVersion || len(got.Projects) != 0 || source.calls != 1 {
		t.Fatalf("snapshot=%#v calls=%d", got, source.calls)
	}
}

func TestGetMonitorSnapshotPreservesSourceError(t *testing.T) {
	wantErr := errors.New("offline")
	source := &fakeSnapshotSource{err: wantErr}
	app := NewApp(source)

	if _, err := app.GetMonitorSnapshot(); !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want %v", err, wantErr)
	}
}

func TestGetMonitorSnapshotRejectsNilAppOrSource(t *testing.T) {
	var app *App
	if _, err := app.GetMonitorSnapshot(); err == nil {
		t.Fatal("nil app should return an error")
	}
	if _, err := NewApp(nil).GetMonitorSnapshot(); err == nil {
		t.Fatal("nil source should return an error")
	}
}

func appWithToolboxStores(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	projectsPath := filepath.Join(dir, "projects.json")
	globalPath := filepath.Join(dir, "toolbox.json")
	app := NewApp(&fakeSnapshotSource{})
	app.projectToolbox = projectToolboxStore{path: projectsPath}
	app.globalToolbox = globalToolboxStore{path: globalPath}
	return app, projectsPath, globalPath
}

func TestAppProjectReferencesBindingsUseDedicatedStore(t *testing.T) {
	app, projectsPath, _ := appWithToolboxStores(t)
	want := ProjectReferences{References: []ToolboxReference{{ID: "ref-1", Label: "Wiki", Type: "web", Target: "https://example.com/wiki"}}}
	if _, err := app.SaveProjectReferences("github.example/repo:acme/app", want); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetProjectReferences("github.example/repo:acme/app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.References) != 1 || got.References[0].ID != "ref-1" {
		t.Fatalf("unexpected references: %#v", got)
	}
	data, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "commands") || strings.Contains(string(data), "checklist") {
		t.Fatalf("reference binding wrote legacy fields: %s", data)
	}
}

func TestAppToolboxBindingsValidateBeforeMigration(t *testing.T) {
	app, projectsPath, globalPath := appWithToolboxStores(t)
	legacy := projectToolboxFile{Version: toolboxVersion, Projects: map[string]ProjectToolbox{
		"github.example/repo:acme/app": {Commands: []ToolboxCommand{{ID: "legacy", Label: "legacy", Command: "legacy"}}},
	}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectsPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetProjectReferences("bad\nkey"); err == nil {
		t.Fatal("invalid reference key must fail before migration")
	}
	if _, err := app.SaveProjectReferences("github.example/repo:acme/app", ProjectReferences{References: []ToolboxReference{{ID: "bad", Label: "bad", Type: "file", Target: "relative.txt"}}}); err == nil {
		t.Fatal("invalid references must fail before migration")
	}
	after, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("invalid reference requests must not trigger migration writes")
	}
	if _, err := os.Stat(globalPath); !os.IsNotExist(err) {
		t.Fatalf("invalid requests must not write global toolbox: %v", err)
	}
}

func TestAppSaveGlobalToolboxPreservesLegacyExistingAndCallerData(t *testing.T) {
	app, projectsPath, globalPath := appWithToolboxStores(t)
	legacy := projectToolboxFile{Version: toolboxVersion, Projects: map[string]ProjectToolbox{
		"github.example/repo:acme/app": {Commands: []ToolboxCommand{{ID: "legacy", Label: "legacy", Command: "legacy"}}},
	}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectsPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	existing := GlobalToolbox{Commands: []ToolboxCommand{{ID: "existing", Label: "existing", Command: "existing"}}}
	data, err = json.Marshal(globalToolboxFile{Version: globalToolboxVersion, Toolbox: existing})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	caller := GlobalToolbox{Commands: []ToolboxCommand{{ID: "caller", Label: "caller", Command: "caller"}}}
	got, err := app.SaveGlobalToolbox(caller)
	if err != nil {
		t.Fatal(err)
	}
	commands := map[string]bool{}
	for _, command := range got.Commands {
		commands[command.ID] = true
	}
	if !commands["legacy"] || !commands["existing"] || !commands["caller"] {
		t.Fatalf("SaveGlobalToolbox dropped data: %#v", got)
	}
}
