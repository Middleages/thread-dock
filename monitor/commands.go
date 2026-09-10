package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"thread-dock/internal/runner"
)

const maxGitHubOutput = 2 * 1024 * 1024

type wslCommandRunner struct {
	process     CommandRunner
	distribution string
	timeout      time.Duration
}

func newWSLCommandRunner(process CommandRunner, distribution string, timeout time.Duration) *wslCommandRunner {
	return &wslCommandRunner{process: process, distribution: distribution, timeout: timeout}
}

func (r *wslCommandRunner) run(ctx context.Context, executable string, args ...string) ([]byte, error) {
	if r == nil || r.process == nil {
		return nil, errors.New("github command runner is unavailable")
	}
	if strings.TrimSpace(r.distribution) == "" {
		return nil, errors.New("github WSL distribution is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("github command timed out: %w", err)
	}
	commandCtx := ctx
	cancel := func() {}
	if r.timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()
	result, err := r.process.Run(commandCtx, "", "wsl.exe", append([]string{"--distribution", r.distribution, "--exec", executable}, args...)...)
	if commandCtx.Err() != nil {
		return nil, fmt.Errorf("github command timed out: %w", commandCtx.Err())
	}
	if err != nil || result.ExitCode != 0 {
		status := result.ExitCode
		if status == 0 {
			status = -1
		}
		return nil, fmt.Errorf("github command failed with exit status %d", status)
	}
	if len(result.Stdout) > maxGitHubOutput {
		return nil, errors.New("github command output limit exceeded")
	}
	return []byte(result.Stdout), nil
}

// compile-time documentation that the process adapter uses the existing
// runner result contract rather than inventing a second process result type.
var _ = runner.Result{}
