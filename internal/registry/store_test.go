package registry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"thread-dock/internal/contract/v2"
)

func validProject() Project {
	return Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "app", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{
		"app": {Host: "github.com", Owner: "acme", Name: "app", DefaultBranch: "main"},
	}}
}

func TestCreatePersistsProjectAtomicallyAndLoadsStrictly(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	want := validProject()
	got, err := s.Create(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("created project = %#v, want %#v", got, want)
	}
	loaded, err := s.Load(context.Background(), want.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("loaded project = %#v, want %#v", loaded, want)
	}
	info, err := os.Stat(filepath.Join(root, "v2", "projects", "project-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("project mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "runs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected v1 runs path: %v", err)
	}
}

func TestCreateRejectsDuplicateAndListIsDeterministic(t *testing.T) {
	s := NewStore(t.TempDir())
	first := validProject()
	second := validProject()
	second.ProjectID = "project-0"
	if _, err := s.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(context.Background(), first); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate error = %v, want conflict", err)
	}
	projects, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := projects[0].ProjectID; got != "project-0" {
		t.Fatalf("first project = %q", got)
	}
}

func TestCreateRejectsMissingPrimaryAndInvalidRepository(t *testing.T) {
	s := NewStore(t.TempDir())
	p := validProject()
	p.PrimaryRepoKey = "missing"
	if _, err := s.Create(context.Background(), p); err == nil {
		t.Fatal("accepted missing primary")
	}
	p = validProject()
	p.Repositories["app"] = contractv2.RepositoryIdentity{Host: "", Owner: "acme", Name: "app", DefaultBranch: "main"}
	if _, err := s.Create(context.Background(), p); err == nil {
		t.Fatal("accepted invalid repository")
	}
}
