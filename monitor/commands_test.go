package main

import (
	"context"
	"errors"
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
	run    func(context.Context) (runner.Result, error)
}

func (r *recordingRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	r.ctx = ctx
	r.args = append([]string{cwd, executable}, args...)
	if r.run != nil {
		return r.run(ctx)
	}
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

func TestWSLCommandRunnerAppliesConstructorDeadline(t *testing.T) {
	started := make(chan struct{})
	observedDeadline := make(chan bool, 1)
	fake := &recordingRunner{}
	fake.run = func(ctx context.Context) (runner.Result, error) {
		close(started)
		_, hasDeadline := ctx.Deadline()
		observedDeadline <- hasDeadline
		<-ctx.Done()
		return runner.Result{}, ctx.Err()
	}
	adapter := newWSLCommandRunner(fake, "Ubuntu", 10*time.Millisecond)
	_, err := adapter.run(context.Background(), "gh", "issue", "list")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	if !<-observedDeadline {
		t.Fatal("constructor timeout was not applied to command context")
	}
}

func TestWSLCommandRunnerRedactsStderrOnNonZeroExit(t *testing.T) {
	fake := &recordingRunner{result: runner.Result{ExitCode: 9, Stderr: "secret-token"}, err: errors.New("process failed")}
	adapter := newWSLCommandRunner(fake, "Ubuntu", time.Second)
	_, err := adapter.run(context.Background(), "gh", "pr", "list")
	if err == nil || !strings.Contains(err.Error(), "exit status 9") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err=%v", err)
	}
}
