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
	process      CommandRunner
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
		if diagnostic := safeGitHubCommandDiagnostic(result.Stderr); diagnostic != "" {
			return nil, fmt.Errorf("github command failed with exit status %d: %s", status, diagnostic)
		}
		return nil, fmt.Errorf("github command failed with exit status %d", status)
	}
	if len(result.Stdout) > maxGitHubOutput {
		return nil, errors.New("github command output limit exceeded")
	}
	return []byte(result.Stdout), nil
}

// safeGitHubCommandDiagnostic deliberately classifies stderr instead of
// exposing raw process output. This keeps authentication details private while
// still giving the operator an actionable cause.
func safeGitHubCommandDiagnostic(stderr string) string {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "required scope") || strings.Contains(lower, "required scopes") || strings.Contains(lower, "missing scope") || strings.Contains(lower, "missing scopes") {
		if scopes := safeGitHubScopeList(stderr); scopes != "" {
			return "GitHub 인증 scope가 부족합니다: " + scopes
		}
		return "GitHub 인증에 필요한 권한(scope)이 없습니다"
	}
	if strings.Contains(lower, "not logged") || strings.Contains(lower, "authentication") || strings.Contains(lower, "authenticate") {
		return "GitHub 인증 상태를 확인하세요"
	}
	if strings.Contains(lower, "forbidden") || strings.Contains(lower, "permission denied") || strings.Contains(lower, "access denied") {
		return "GitHub 접근 권한을 확인하세요"
	}
	if strings.Contains(lower, "rate limit") {
		return "GitHub API 요청 한도를 확인하세요"
	}
	if strings.Contains(lower, "graphql") {
		return "GitHub GraphQL 요청이 실패했습니다"
	}
	if strings.Contains(lower, "not found") {
		return "GitHub 대상을 찾을 수 없습니다"
	}
	return ""
}

func safeGitHubScopeList(stderr string) string {
	lower := strings.ToLower(stderr)
	for _, marker := range []string{"scopes [", "scope ["} {
		start := strings.Index(lower, marker)
		if start < 0 {
			continue
		}
		start += len(marker)
		end := strings.Index(stderr[start:], "]")
		if end < 0 {
			continue
		}
		candidate := strings.TrimSpace(stderr[start : start+end])
		if candidate == "" || len(candidate) > 80 {
			continue
		}
		valid := true
		for _, r := range candidate {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("_:-, ", r) {
				continue
			}
			valid = false
			break
		}
		if valid {
			return candidate
		}
	}
	return ""
}

// compile-time documentation that the process adapter uses the existing
// runner result contract rather than inventing a second process result type.
var _ = runner.Result{}
