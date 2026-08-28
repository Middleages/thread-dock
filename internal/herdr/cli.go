package herdr

import (
	"context"
	"encoding/json"
	"fmt"

	"thread-dock/internal/runner"
)

const promptTimeout = "3600000"

type CLI struct {
	runner     runner.Runner
	executable string
}

func NewCLI(r runner.Runner, executable string) *CLI {
	return &CLI{runner: r, executable: executable}
}

func (c *CLI) CreateWorktree(ctx context.Context, req CreateWorktreeRequest) (Worktree, error) {
	cwd := req.Cwd
	if cwd == "" {
		cwd = req.Repo
	}
	result, err := c.run(ctx, "worktree create", "worktree", "create", "--cwd", cwd, "--branch", req.Branch, "--base", req.Base, "--label", req.Label, "--no-focus")
	if err != nil {
		return Worktree{}, err
	}
	var response struct {
		Result struct {
			RootPane struct {
				PaneID      string `json:"pane_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"root_pane"`
			Workspace struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := decode(result.Stdout, &response); err != nil {
		return Worktree{}, safeError("worktree create", result.ExitCode)
	}
	workspaceID := response.Result.Workspace.WorkspaceID
	paneID := response.Result.RootPane.PaneID
	if workspaceID == "" || paneID == "" {
		return Worktree{}, safeError("worktree create", result.ExitCode)
	}
	if response.Result.RootPane.WorkspaceID != workspaceID {
		return Worktree{}, safeError("worktree create", result.ExitCode)
	}
	panes, err := c.run(ctx, "pane list", "pane", "list", "--workspace", workspaceID)
	if err != nil {
		return Worktree{}, err
	}
	var paneResponse struct {
		Result struct {
			Panes []struct {
				PaneID      string `json:"pane_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := decode(panes.Stdout, &paneResponse); err != nil {
		return Worktree{}, safeError("pane list", panes.ExitCode)
	}
	for _, pane := range paneResponse.Result.Panes {
		if pane.PaneID == paneID && pane.WorkspaceID == workspaceID {
			return Worktree{WorkspaceID: workspaceID, PaneID: paneID}, nil
		}
	}
	return Worktree{}, safeError("pane list", panes.ExitCode)
}

func (c *CLI) StartAgent(ctx context.Context, req StartAgentRequest) error {
	_, err := c.run(ctx, "agent start", "agent", "start", req.Name, "--kind", "opencode", "--pane", req.PaneID)
	return err
}

func (c *CLI) Prompt(ctx context.Context, name, packet string) error {
	_, err := c.run(ctx, "agent prompt", "agent", "prompt", name, packet, "--wait", "--timeout", promptTimeout)
	return err
}

func (c *CLI) Get(ctx context.Context, name string) (AgentState, error) {
	result, err := c.run(ctx, "agent get", "agent", "get", name)
	if err != nil {
		return AgentStateUnknown, err
	}
	var response struct {
		Result struct {
			Agent struct {
				Status string `json:"agent_status"`
			} `json:"agent"`
		} `json:"result"`
	}
	if err := decode(result.Stdout, &response); err != nil {
		return AgentStateUnknown, safeError("agent get", result.ExitCode)
	}
	if response.Result.Agent.Status == "" {
		return AgentStateUnknown, safeError("agent get", result.ExitCode)
	}
	return ParseAgentState(response.Result.Agent.Status), nil
}

func (c *CLI) ReadRecent(ctx context.Context, name string) (string, error) {
	result, err := c.run(ctx, "agent read", "agent", "read", name, "--source", "recent-unwrapped", "--lines", "120")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

func (c *CLI) run(ctx context.Context, operation string, args ...string) (runner.Result, error) {
	result, err := c.runner.Run(ctx, "", c.executable, args...)
	if err != nil {
		return result, safeError(operation, result.ExitCode)
	}
	return result, nil
}

func safeError(operation string, exitCode int) error {
	return fmt.Errorf("herdr %s failed (exit code %d)", operation, exitCode)
}

func decode(output string, target any) error {
	if err := json.Unmarshal([]byte(output), target); err != nil {
		return err
	}
	return nil
}
