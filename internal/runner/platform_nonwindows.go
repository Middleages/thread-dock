//go:build !windows

package runner

import "os/exec"

func applyPlatformCommandAttributes(*exec.Cmd) {}
