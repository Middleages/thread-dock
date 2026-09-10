//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

const createNoWindow uint32 = 0x08000000 // CREATE_NO_WINDOW

func applyPlatformCommandAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
		HideWindow:    true,
	}
}
