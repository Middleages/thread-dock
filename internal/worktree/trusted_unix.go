//go:build !windows

package worktree

import (
	"errors"
	"os"
	"syscall"
)

func validateTrustedManagedRoot(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return ErrUnsafeTarget
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || uint32(stat.Uid) != uint32(os.Getuid()) || info.Mode().Perm()&0o022 != 0 {
		return ErrUnsafeTarget
	}
	if !info.IsDir() {
		return errors.New("managed root is not a directory")
	}
	return nil
}
