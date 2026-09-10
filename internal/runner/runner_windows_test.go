//go:build windows

package runner

import (
	"os/exec"
	"testing"
)

func TestApplyPlatformCommandAttributesHidesWindowsConsole(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	applyPlatformCommandAttributes(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow=false, want true")
	}
	if cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Fatalf("CreationFlags=%#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}
