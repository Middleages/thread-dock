package main

import (
	"context"

	"thread-dock/internal/runner"
)

// openOnlyCommandRunner keeps the existing GitHub adapter unchanged while
// narrowing repository Issue/PR reads to open items. Other commands, including
// GitHub Projects and Herdr, pass through untouched.
type openOnlyCommandRunner struct {
	base CommandRunner
}

func (r openOnlyCommandRunner) Run(ctx context.Context, cwd, executable string, args ...string) (runner.Result, error) {
	if r.base == nil {
		return runner.Result{}, nil
	}
	copyArgs := append([]string(nil), args...)
	if isGitHubWorkListCommand(executable, copyArgs) {
		for index := 0; index+1 < len(copyArgs); index++ {
			if copyArgs[index] == "--state" && copyArgs[index+1] == "all" {
				copyArgs[index+1] = "open"
				break
			}
		}
	}
	return r.base.Run(ctx, cwd, executable, copyArgs...)
}

func isGitHubWorkListCommand(executable string, args []string) bool {
	tokens := append([]string{executable}, args...)
	for index := 0; index+2 < len(tokens); index++ {
		if tokens[index] == "gh" && (tokens[index+1] == "issue" || tokens[index+1] == "pr") && tokens[index+2] == "list" {
			return true
		}
	}
	return false
}
