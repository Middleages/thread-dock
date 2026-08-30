//go:build windows

package worktree

func validateTrustedManagedRoot(path string) error {
	// ThreadDock's Git mutation runtime is WSL. Windows ACL ownership cannot
	// be represented by this trusted-workstation check, so fail closed.
	return ErrUnsafeTarget
}
