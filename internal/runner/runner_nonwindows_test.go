//go:build !windows

package runner

import (
	"os/exec"
	"testing"
)

func TestApplyPlatformCommandAttributesIsNoOpOnNonWindows(t *testing.T) {
	cmd := exec.Command("printf")
	applyPlatformCommandAttributes(cmd)

	if cmd.SysProcAttr != nil {
		t.Fatalf("SysProcAttr=%#v, want nil", cmd.SysProcAttr)
	}
}
