package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func launchLocalPath(target string) error {
	target = strings.TrimSpace(target)
	if !isAbsoluteLocalPath(target) {
		return fmt.Errorf("로컬 파일은 절대 경로여야 합니다")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("로컬 파일을 열 수 없습니다: %w", err)
	}
	return nil
}
