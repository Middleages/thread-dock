package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/runner"
)

type recordingRunner struct {
	result runner.Result
	err    error
	args   []string
	ctx    context.Context
}

func (r *recordingRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	r.ctx = ctx
	r.args = append([]string{cwd, executable}, args...)
	return r.result, r.err
}

func TestWSLCommandRunnerPreservesDistributionAndArgumentBoundaries(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{Stdout: `[]`}}
	adapter := newWSLCommandRunner(fake, "Ubuntu 24.04", time.Second)
	if _, err := adapter.run(context.Background(), "gh", "issue", "list", "--repo", "acme/app", "--title", "a value"); err != nil {
		t.Fatal(err)
	}
	want := []string{"", "wsl.exe", "--distribution", "Ubuntu 24.04", "--exec", "gh", "issue", "list", "--repo", "acme/app", "--title", "a value"}
	if strings.Join(fake.args, "|") != strings.Join(want, "|") {
		t.Fatalf("args=%q want=%q", fake.args, want)
	}
}

func TestWSLCommandRunnerRejectsOversizedOutputAndRedactsProcessOutput(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{Stdout: strings.Repeat("secret", 400000), Stderr: "token=secret"}}
	adapter := newWSLCommandRunner(fake, "Ubuntu", time.Second)
	_, err := adapter.run(context.Background(), "gh", "issue", "list")
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("err=%v", err)
	}
}

func TestWSLCommandRunnerReportsContextTimeoutWithoutRawStderr(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{Stderr: "password=hidden"}}
	adapter := newWSLCommandRunner(fake, "Ubuntu", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.run(ctx, "gh", "issue", "list")
	if err == nil || !strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "hidden") {
		t.Fatalf("err=%v", err)
	}
}
