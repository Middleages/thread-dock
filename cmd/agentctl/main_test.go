package main

import (
	"context"
	"errors"
	"testing"

	"thread-dock/internal/runner"
)

func TestRepositoryDiscoveryOnlyRunsForStart(t *testing.T) {
	var calls int
	discover := func(context.Context, runner.Runner, string) (string, error) {
		calls++
		return "/workspace/repo", nil
	}

	path, err := repositoryPathForCommand(context.Background(), []string{"start", "contract.json"}, nil, "git", discover)
	if err != nil || path != "/workspace/repo" || calls != 1 {
		t.Fatalf("start path=%q err=%v calls=%d", path, err, calls)
	}
	for _, command := range []string{"status", "stop", "cleanup"} {
		path, err = repositoryPathForCommand(context.Background(), []string{command, "run-184"}, nil, "git", discover)
		if err != nil || path != "" || calls != 1 {
			t.Fatalf("%s path=%q err=%v calls=%d", command, path, err, calls)
		}
	}
	for _, command := range []string{"resume", "confirm", "create-revert"} {
		path, err = repositoryPathForCommand(context.Background(), []string{command, "run-184"}, nil, "git", discover)
		if err != nil || path != "/workspace/repo" {
			t.Fatalf("%s path=%q err=%v", command, path, err)
		}
	}
	if calls != 4 {
		t.Fatalf("discovery calls=%d, want four", calls)
	}
}

func TestRepositoryDiscoveryPropagatesStartFailure(t *testing.T) {
	want := errors.New("not a git checkout")
	discover := func(context.Context, runner.Runner, string) (string, error) {
		return "", want
	}
	if _, err := repositoryPathForCommand(context.Background(), []string{"start", "contract.json"}, nil, "git", discover); !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}
