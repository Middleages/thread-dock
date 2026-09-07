package statev2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	contractv2 "thread-dock/internal/contract/v2"
)

var (
	ErrConflict      = errors.New("state conflict")
	ErrNotFound      = errors.New("work state not found")
	ErrBusy          = errors.New("work state is busy")
	ErrWorkBusy      = ErrBusy
	ErrStaleRevision = errors.New("stale revision")
)

type store struct {
	root string
	mu   sync.Mutex
}
type lease struct{ f *os.File }

func NewStore(root string) Store { return &store{root: filepath.Join(root, "v2", "work")} }

func (s *store) CreatePlan(ctx context.Context, snapshot WorkSnapshot) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validatePlan(snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	dir := s.workDir(snapshot.WorkID)
	if err := os.MkdirAll(filepath.Join(dir, "contracts"), 0700); err != nil {
		return WorkSnapshot{}, fmt.Errorf("create work directory: %w", err)
	}
	l, err := acquireLease(dir)
	if err != nil {
		return WorkSnapshot{}, err
	}
	defer l.release()
	if _, err := os.Stat(filepath.Join(dir, "work.json")); err == nil {
		return WorkSnapshot{}, fmt.Errorf("%w: work %q already exists", ErrConflict, snapshot.WorkID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return WorkSnapshot{}, err
	}
	if err := writeContract(filepath.Join(dir, "contracts", "1.json"), snapshot.Contract); err != nil {
		return WorkSnapshot{}, err
	}
	if err := writeSnapshot(filepath.Join(dir, "work.json"), snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	return snapshot, nil
}

func (s *store) Load(ctx context.Context, id contractv2.WorkID) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validID(string(id)); err != nil {
		return WorkSnapshot{}, err
	}
	f, err := os.Open(filepath.Join(s.workDir(id), "work.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkSnapshot{}, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return WorkSnapshot{}, err
	}
	defer f.Close()
	return decodeSnapshot(f)
}

func (s *store) Mutate(ctx context.Context, mutation Mutation) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validID(string(mutation.WorkID)); err != nil {
		return WorkSnapshot{}, err
	}
	if mutation.RequestID == "" || strings.TrimSpace(mutation.PayloadHash) == "" {
		return WorkSnapshot{}, errors.New("request ID and payload hash are required")
	}
	dir := s.workDir(mutation.WorkID)
	l, err := acquireLease(dir)
	if err != nil {
		return WorkSnapshot{}, err
	}
	defer l.release()
	f, err := os.Open(filepath.Join(dir, "work.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkSnapshot{}, fmt.Errorf("%w: %q", ErrNotFound, mutation.WorkID)
		}
		return WorkSnapshot{}, err
	}
	snapshot, err := decodeSnapshot(f)
	_ = f.Close()
	if err != nil {
		return WorkSnapshot{}, err
	}
	if receipt, ok := snapshot.Receipts[mutation.RequestID]; ok {
		if receipt.PayloadHash == mutation.PayloadHash {
			return snapshot, nil
		}
		return WorkSnapshot{}, fmt.Errorf("%w: %w", ErrConflict, &RequestConflictError{RequestID: mutation.RequestID, ExistingHash: receipt.PayloadHash, PayloadHash: mutation.PayloadHash})
	}
	if mutation.ExpectedRevision != snapshot.Revision {
		return WorkSnapshot{}, &StaleRevisionError{CurrentRevision: snapshot.Revision, CurrentState: snapshot.State}
	}
	if mutation.Transition == nil {
		return WorkSnapshot{}, errors.New("transition is required")
	}
	if err := mutation.Transition(&snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	if snapshot.WorkID != mutation.WorkID || snapshot.Revision != mutation.ExpectedRevision {
		return WorkSnapshot{}, errors.New("transition may not change work identity or revision")
	}
	snapshot.Revision++
	if err := validateSnapshot(snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	result, err := json.Marshal(snapshot)
	if err != nil {
		return WorkSnapshot{}, err
	}
	if snapshot.Receipts == nil {
		snapshot.Receipts = map[contractv2.RequestID]Receipt{}
	}
	snapshot.Receipts[mutation.RequestID] = Receipt{RequestID: mutation.RequestID, PayloadHash: mutation.PayloadHash, Status: "committed", Result: result}
	if err := writeSnapshot(filepath.Join(dir, "work.json"), snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	return snapshot, nil
}

func validatePlan(s WorkSnapshot) error {
	if s.SchemaVersion != 2 || s.Revision != 1 || s.State != StateAwaitingApproval {
		return errors.New("plan must be schema 2, revision 1, awaiting approval")
	}
	if err := validID(string(s.ProjectID)); err != nil || validID(string(s.WorkID)) != nil {
		return errors.New("project and work IDs are required")
	}
	if s.Contract.Version != contractv2.CurrentVersion || s.Contract.Revision != 1 || s.Contract.WorkID != s.WorkID || s.Contract.ProjectID != s.ProjectID {
		return errors.New("contract identity or revision does not match plan")
	}
	if len(contractv2.Validate(s.Contract)) > 0 {
		return errors.New("contract is invalid")
	}
	return validateSnapshot(s)
}
func validateSnapshot(s WorkSnapshot) error {
	if s.SchemaVersion != 2 || s.ProjectID == "" || s.WorkID == "" || s.Revision == 0 || s.State == "" || s.ContractHash == "" || s.Contract.Version != 2 {
		return errors.New("invalid work snapshot")
	}
	if s.Contract.WorkID != s.WorkID || s.Contract.ProjectID != s.ProjectID || s.Contract.Revision != 1 {
		return errors.New("snapshot contract mismatch")
	}
	if s.Receipts == nil {
		return errors.New("receipts map is required")
	}
	return nil
}
func (s *store) workDir(id contractv2.WorkID) string { return filepath.Join(s.root, string(id)) }
func validID(v string) error {
	if v == "" || v != strings.TrimSpace(v) || strings.ContainsAny(v, "/\\:\r\n\t ") {
		return errors.New("invalid ID")
	}
	return nil
}
func decodeSnapshot(r io.Reader) (WorkSnapshot, error) {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	var s WorkSnapshot
	if err := d.Decode(&s); err != nil {
		return WorkSnapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	var x any
	if err := d.Decode(&x); err != io.EOF {
		return WorkSnapshot{}, errors.New("decode snapshot: trailing JSON")
	}
	if err := validateSnapshot(s); err != nil {
		return WorkSnapshot{}, err
	}
	return s, nil
}
func writeContract(path string, c contractv2.WorkItemContract) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: contract revision already exists", ErrConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	encErr := contractv2.Write(f, c)
	if encErr == nil {
		encErr = f.Sync()
	}
	closeErr := f.Close()
	if encErr != nil {
		_ = os.Remove(tmp)
		return encErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return syncDir(filepath.Dir(path))
}
func writeSnapshot(path string, s WorkSnapshot) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	encErr := json.NewEncoder(f).Encode(s)
	if encErr == nil {
		encErr = f.Sync()
	}
	closeErr := f.Close()
	if encErr != nil {
		_ = os.Remove(tmp)
		return encErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return syncDir(filepath.Dir(path))
}
func acquireLease(dir string) (*lease, error) {
	f, err := os.OpenFile(filepath.Join(dir, "lease.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return &lease{f: f}, nil
}
func (l *lease) release() {
	if l != nil && l.f != nil {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		_ = l.f.Close()
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
func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
