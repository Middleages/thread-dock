package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	contractv2 "thread-dock/internal/contract/v2"
)

var (
	// ErrInvalidOwner indicates invalid lease identity or process metadata.
	ErrInvalidOwner = errors.New("invalid owner lease input")
	// ErrOwnerBusy indicates that another live process owns the work item.
	ErrOwnerBusy = errors.New("work item owner is busy")
)

type ownerLocker struct {
	root string
}

type ownerLease struct {
	file       *os.File
	ownerPath  string
	record     OwnerRecord
	release    sync.Once
	releaseErr error
}

// NewOwnerLocker creates a locker rooted at root.
func NewOwnerLocker(root string) OwnerLocker {
	return &ownerLocker{root: root}
}

func (l *ownerLocker) Acquire(ctx context.Context, workID contractv2.WorkID, ownerID OwnerID, pid int, startedAt time.Time) (OwnerLease, error) {
	if err := validateOwnerInput(workID, ownerID, pid, startedAt); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	workDir := filepath.Join(l.root, "v2", "work", string(workID))
	if err := ensureOwnerDirs(l.root, workDir); err != nil {
		return nil, fmt.Errorf("create owner directory: %w", err)
	}
	lockPath := filepath.Join(workDir, "coordinator.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open coordinator lock: %w", err)
	}
	if err := os.Chmod(lockPath, 0o600); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("set coordinator lock mode: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("%w: %s", ErrOwnerBusy, lockPath)
		}
		return nil, fmt.Errorf("acquire coordinator lock: %w", err)
	}

	record := OwnerRecord{WorkID: workID, OwnerID: ownerID, PID: pid, StartedAt: startedAt}
	ownerPath := filepath.Join(workDir, "owner.json")
	if err := writeOwnerRecord(ownerPath, record); err != nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
		return nil, fmt.Errorf("write owner record: %w", err)
	}
	return &ownerLease{file: lockFile, ownerPath: ownerPath, record: record}, nil
}

func (l *ownerLease) Record() OwnerRecord {
	return l.record
}

func (l *ownerLease) Release() error {
	l.release.Do(func() {
		// Remove the record while the flock is still held so a successor cannot
		// have its freshly-written record removed by this release.
		if err := os.Remove(l.ownerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			l.releaseErr = fmt.Errorf("remove owner record: %w", err)
		}
		if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil && l.releaseErr == nil {
			l.releaseErr = fmt.Errorf("release coordinator lock: %w", err)
		}
		if err := l.file.Close(); err != nil && l.releaseErr == nil {
			l.releaseErr = fmt.Errorf("close coordinator lock: %w", err)
		}
	})
	return l.releaseErr
}

func validateOwnerInput(workID contractv2.WorkID, ownerID OwnerID, pid int, startedAt time.Time) error {
	if !validOwnerID(string(workID)) || !validOwnerID(string(ownerID)) || pid <= 0 || startedAt.IsZero() || startedAt.Location() != time.UTC {
		return ErrInvalidOwner
	}
	return nil
}

func validOwnerID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if r == '/' || r == '\\' || r == ':' || r == 0 || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func ensureOwnerDirs(root, workDir string) error {
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return err
	}
	v2Dir := filepath.Join(root, "v2")
	workRoot := filepath.Join(v2Dir, "work")
	for _, path := range []string{v2Dir, workRoot, workDir} {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func writeOwnerRecord(path string, record OwnerRecord) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".owner-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	removeTemp = false
	return os.Chmod(path, 0o600)
}
