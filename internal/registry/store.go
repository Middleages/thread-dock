package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	contractv2 "thread-dock/internal/contract/v2"
)

var (
	ErrConflict = errors.New("registry conflict")
	ErrNotFound = errors.New("registry project not found")
)

type store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) Store { return &store{root: filepath.Join(root, "v2", "projects")} }

func (s *store) Create(ctx context.Context, project Project) (Project, error) {
	if err := contextErr(ctx); err != nil {
		return Project{}, err
	}
	if err := validateProject(project); err != nil {
		return Project{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensureDir(filepath.Dir(s.root)); err != nil {
		return Project{}, fmt.Errorf("create v2 directory: %w", err)
	}
	if err := ensureDir(s.root); err != nil {
		return Project{}, fmt.Errorf("create projects directory: %w", err)
	}
	lease, err := acquireLease(s.root)
	if err != nil {
		return Project{}, err
	}
	defer lease.release()
	path := filepath.Join(s.root, string(project.ProjectID)+".json")
	if _, err := os.Stat(path); err == nil {
		return Project{}, fmt.Errorf("%w: project %q already exists", ErrConflict, project.ProjectID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Project{}, fmt.Errorf("check project: %w", err)
	}
	if err := writeAtomic(path, project); err != nil {
		return Project{}, err
	}
	return project, nil
}

type lease struct{ file *os.File }

func acquireLease(dir string) (*lease, error) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open registry lease: %w", err)
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("harden registry lease: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("%w: registry is busy", ErrConflict)
		}
		return nil, fmt.Errorf("acquire registry lease: %w", err)
	}
	return &lease{file: f}, nil
}

func (l *lease) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}

func (s *store) Load(ctx context.Context, id contractv2.ProjectID) (Project, error) {
	if err := contextErr(ctx); err != nil {
		return Project{}, err
	}
	if err := validateID(string(id)); err != nil {
		return Project{}, err
	}
	project, err := readProject(filepath.Join(s.root, string(id)+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Project{}, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return Project{}, err
	}
	return project, nil
}

func (s *store) List(ctx context.Context) ([]Project, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return []Project{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
		}
	}
	sort.Strings(ids)
	projects := make([]Project, 0, len(ids))
	for _, id := range ids {
		p, err := s.Load(ctx, contractv2.ProjectID(id))
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func validateProject(p Project) error {
	if err := validateID(string(p.ProjectID)); err != nil {
		return fmt.Errorf("projectId: %w", err)
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("project name is required")
	}
	if err := validateID(string(p.PrimaryRepoKey)); err != nil {
		return fmt.Errorf("primaryRepoKey: %w", err)
	}
	if len(p.Repositories) == 0 {
		return errors.New("at least one repository is required")
	}
	if _, ok := p.Repositories[p.PrimaryRepoKey]; !ok {
		return fmt.Errorf("primary repository %q is not registered", p.PrimaryRepoKey)
	}
	for key, repo := range p.Repositories {
		if err := validateID(string(key)); err != nil {
			return fmt.Errorf("repository key: %w", err)
		}
		if strings.TrimSpace(repo.Host) == "" || strings.TrimSpace(repo.Owner) == "" || strings.TrimSpace(repo.Name) == "" || strings.TrimSpace(repo.DefaultBranch) == "" {
			return fmt.Errorf("repository %q identity is incomplete", key)
		}
		for _, value := range []string{repo.Host, repo.Owner, repo.Name, repo.DefaultBranch} {
			if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n\t") {
				return fmt.Errorf("repository %q identity contains invalid whitespace", key)
			}
		}
	}
	return nil
}

func validateID(value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return errors.New("must be non-empty and trimmed")
	}
	if strings.ContainsAny(value, "/\\:\r\n\t ") {
		return errors.New("contains invalid path characters")
	}
	return nil
}

func readProject(path string) (Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return Project{}, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var p Project
	if err := dec.Decode(&p); err != nil {
		return Project{}, fmt.Errorf("decode project: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Project{}, errors.New("decode project: trailing JSON")
	}
	if err := validateProject(p); err != nil {
		return Project{}, fmt.Errorf("invalid project: %w", err)
	}
	return p, nil
}

func writeAtomic(path string, value Project) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("open temporary project: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return fmt.Errorf("harden temporary project: %w", err)
	}
	encErr := json.NewEncoder(f).Encode(value)
	if encErr == nil {
		encErr = f.Sync()
	}
	closeErr := f.Close()
	if encErr != nil {
		return fmt.Errorf("write project: %w", encErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close project: %w", closeErr)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit project: %w", err)
	}
	return syncDir(filepath.Dir(path))
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
