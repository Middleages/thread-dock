package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"thread-dock/internal/contract"
)

var ErrRunBusy = errors.New("run is already being advanced")

// Lease is a process-safe, per-run advisory lock. Closing the descriptor
// releases it after a crash; the lock file itself is harmless durable state.
type Lease struct {
	file *os.File
}

func (l *Lease) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

// Acquire returns immediately with ErrRunBusy when another process owns the
// run lease. The context is checked before touching the filesystem.
func (s *Store) Acquire(ctx context.Context, runID contract.RunID) (*Lease, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := validateRunID(runID); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.runDir(runID), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(s.runDir(runID), "run.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrRunBusy
		}
		return nil, err
	}
	return &Lease{file: file}, nil
}
