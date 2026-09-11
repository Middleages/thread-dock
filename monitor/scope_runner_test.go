package main

import (
	"context"
	"strings"
	"testing"

	"thread-dock/internal/runner"
)

type captureCommandRunner struct {
	calls [][]string
}

func (c *captureCommandRunner) Run(_ context.Context, _ string, executable string, args ...string) (runner.Result, error) {
	call := append([]string{executable}, args...)
	c.calls = append(c.calls, call)
	return runner.Result{}, nil
}

func TestOpenOnlyRunnerRewritesIssueAndPRStateWithoutTouchingProjectOrHerdr(t *testing.T) {
	base := &captureCommandRunner{}
	scoped := openOnlyCommandRunner{base: base}

	_, _ = scoped.Run(context.Background(), "", "wsl.exe", "--exec", "gh", "issue", "list", "--repo", "acme/app", "--state", "all")
	_, _ = scoped.Run(context.Background(), "", "wsl.exe", "--exec", "gh", "pr", "list", "--repo", "acme/app", "--state", "all")
	_, _ = scoped.Run(context.Background(), "", "wsl.exe", "--exec", "gh", "project", "item-list", "2", "--owner", "acme")
	_, _ = scoped.Run(context.Background(), "", "wsl.exe", "--exec", "/bin/sh", "-c", "herdr")

	if len(base.calls) != 4 {
		t.Fatalf("calls=%#v", base.calls)
	}
	for _, index := range []int{0, 1} {
		joined := strings.Join(base.calls[index], "|")
		if !strings.Contains(joined, "--state|open") || strings.Contains(joined, "--state|all") {
			t.Fatalf("GitHub work query was not rewritten to open: %q", base.calls[index])
		}
	}
	if strings.Contains(strings.Join(base.calls[2], "|"), "--state|open") {
		t.Fatalf("Project query must not be rewritten: %q", base.calls[2])
	}
	if strings.Contains(strings.Join(base.calls[3], "|"), "--state|open") {
		t.Fatalf("Herdr query must not be rewritten: %q", base.calls[3])
	}
}
