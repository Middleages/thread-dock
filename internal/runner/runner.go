// Package runner provides the process boundary used by ThreadDock adapters.
package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// Result contains the complete output and exit status of a process.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes one executable with one argument per slice element.
type Runner interface {
	Run(ctx context.Context, cwd, executable string, args ...string) (Result, error)
}

// OSRunner runs processes directly through the operating system. It never
// passes commands through a shell, preserving argument boundaries.
type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, cwd, executable string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	applyPlatformCommandAttributes(cmd)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode(err)}, err
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
