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

type RequestConflictError struct {
	RequestID                 contractv2.RequestID
	ExistingHash, PayloadHash string
}

func (e *RequestConflictError) Error() string {
	return fmt.Sprintf("request %q was already used with a different payload", e.RequestID)
}

type receipt struct {
	RequestID   contractv2.RequestID `json:"requestId"`
	PayloadHash string               `json:"payloadHash"`
	Status      string               `json:"status"`
}

type record struct {
	Project  Project             `json:"project"`
	Revision contractv2.Revision `json:"revision"`
	Receipt  receipt             `json:"receipt"`
}

type store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) Store { return &store{root: filepath.Join(root, "v2", "projects")} }

func (s *store) Create(ctx context.Context, project Project, expectedRevision contractv2.Revision, requestID contractv2.RequestID, payloadHash string) (Project, error) {
	if err := contextErr(ctx); err != nil {
		return Project{}, err
	}
	if err := validateProject(project); err != nil {
		return Project{}, err
	}
	if expectedRevision != 0 {
		return Project{}, errors.New("project creation expected revision must be 0")
	}
	if requestID == "" || strings.TrimSpace(payloadHash) == "" {
		return Project{}, errors.New("request ID and payload hash are required")
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
		existing, readErr := readRecord(path)
		if readErr != nil {
			return Project{}, readErr
		}
		if err := validateRecord(existing); err != nil {
			return Project{}, err
		}
		if existing.Receipt.RequestID == requestID {
			if existing.Receipt.PayloadHash == payloadHash {
				return existing.Project, nil
			}
			return Project{}, fmt.Errorf("%w: %w", ErrConflict, &RequestConflictError{RequestID: requestID, ExistingHash: existing.Receipt.PayloadHash, PayloadHash: payloadHash})
		}
		return Project{}, fmt.Errorf("%w: project %q already exists", ErrConflict, project.ProjectID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Project{}, fmt.Errorf("check project: %w", err)
	}
	if err := writeAtomic(path, record{Project: project, Revision: 1, Receipt: receipt{RequestID: requestID, PayloadHash: payloadHash, Status: "committed"}}); err != nil {
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
	if project.ProjectID != id {
		return Project{}, fmt.Errorf("project ID %q does not match requested ID %q", project.ProjectID, id)
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
	stored, err := readRecord(path)
	if err != nil {
		return Project{}, err
	}
	if err := validateRecord(stored); err != nil {
		return Project{}, err
	}
	return stored.Project, nil
}

func validateRecord(stored record) error {
	if stored.Revision != 1 || stored.Receipt.RequestID == "" || strings.TrimSpace(stored.Receipt.PayloadHash) == "" || stored.Receipt.Status != "committed" {
		return errors.New("invalid project record")
	}
	if err := validateProject(stored.Project); err != nil {
		return fmt.Errorf("invalid project: %w", err)
	}
	return nil
}

func readRecord(path string) (record, error) {
	f, err := os.Open(path)
	if err != nil {
		return record{}, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var stored record
	if err := dec.Decode(&stored); err != nil {
		return record{}, fmt.Errorf("decode project: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return record{}, errors.New("decode project: trailing JSON")
	}
	return stored, nil
}

func writeAtomic(path string, value record) error {
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
