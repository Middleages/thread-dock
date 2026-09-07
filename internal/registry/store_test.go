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

func createProject(s Store, p Project, request string) (Project, error) {
	return s.Create(context.Background(), p, 0, contractv2.RequestID(request), "payload-"+request)
}

func TestCreatePersistsProjectAtomicallyAndLoadsStrictly(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	want := validProject()
	got, err := createProject(s, want, "request-project-1")
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
	if _, err := createProject(s, first, "request-project-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := createProject(s, second, "request-project-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := createProject(s, first, "request-project-3"); !errors.Is(err, ErrConflict) {
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
	if _, err := createProject(s, p, "request-invalid-1"); err == nil {
		t.Fatal("accepted missing primary")
	}
	p = validProject()
	p.Repositories["app"] = contractv2.RepositoryIdentity{Host: "", Owner: "acme", Name: "app", DefaultBranch: "main"}
	if _, err := createProject(s, p, "request-invalid-2"); err == nil {
		t.Fatal("accepted invalid repository")
	}
}

func TestCreateHardensPreexistingDirectoryAndTemporaryFileModes(t *testing.T) {
	root := t.TempDir()
	projectsDir := filepath.Join(root, "v2", "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(projectsDir, "project-1.json.tmp")
	if err := os.WriteFile(tmp, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := createProject(NewStore(root), validProject(), "request-project-1"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "v2"), projectsDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0700 {
			t.Fatalf("%s mode = %o, want 700", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(projectsDir, "project-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("project mode = %o, want 600", info.Mode().Perm())
	}
}

func TestCreateIsIdempotentByRequestAndPersistsStrictReceipt(t *testing.T) {
	s := NewStore(t.TempDir())
	p := validProject()
	first, err := s.Create(context.Background(), p, 0, "request-1", "payload-1")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Create(context.Background(), p, 0, "request-1", "payload-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay=%#v first=%#v", replay, first)
	}
	if _, err := s.Create(context.Background(), p, 0, "request-1", "payload-2"); !errors.As(err, new(*RequestConflictError)) {
		t.Fatalf("changed payload error=%v, want typed request conflict", err)
	}
	if _, err := s.Create(context.Background(), p, 0, "request-2", "payload-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("new request duplicate error=%v, want conflict", err)
	}
	loaded, err := s.Load(context.Background(), p.ProjectID)
	if err != nil || !reflect.DeepEqual(loaded, p) {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestCreateRejectsNonZeroExpectedRevision(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Create(context.Background(), validProject(), 1, "request-revision", "payload"); err == nil {
		t.Fatal("accepted non-zero expected revision")
	}
	if _, err := s.Load(context.Background(), "project-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load after rejected creation = %v", err)
	}
}
