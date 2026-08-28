package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	result, err := c.run(ctx, "worktree", "create", "--cwd", cwd, "--branch", req.Branch, "--base", req.Base, "--label", req.Label, "--no-focus")
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
		return Worktree{}, fmt.Errorf("parse worktree create response: %w", err)
	}
	workspaceID := response.Result.Workspace.WorkspaceID
	paneID := response.Result.RootPane.PaneID
	if workspaceID == "" || paneID == "" {
		return Worktree{}, errors.New("worktree create response missing workspace_id or pane_id")
	}
	panes, err := c.run(ctx, "pane", "list", "--workspace", workspaceID)
	if err != nil {
		return Worktree{}, err
	}
	var paneResponse struct {
		Result struct {
			Panes []struct {
				PaneID string `json:"pane_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := decode(panes.Stdout, &paneResponse); err != nil {
		return Worktree{}, fmt.Errorf("parse pane list response: %w", err)
	}
	for _, pane := range paneResponse.Result.Panes {
		if pane.PaneID == paneID {
			return Worktree{WorkspaceID: workspaceID, PaneID: paneID}, nil
		}
	}
	return Worktree{}, fmt.Errorf("created pane %q not found in workspace %q", paneID, workspaceID)
}

func (c *CLI) StartAgent(ctx context.Context, req StartAgentRequest) error {
	_, err := c.run(ctx, "agent", "start", req.Name, "--kind", req.Kind, "--pane", req.PaneID)
	return err
}

func (c *CLI) Prompt(ctx context.Context, name, packet string) error {
	_, err := c.run(ctx, "agent", "prompt", name, packet, "--wait", "--timeout", promptTimeout)
	return err
}

func (c *CLI) Get(ctx context.Context, name string) (AgentState, error) {
	result, err := c.run(ctx, "agent", "get", name)
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
		return AgentStateUnknown, fmt.Errorf("parse agent get response: %w", err)
	}
	if response.Result.Agent.Status == "" {
		return AgentStateUnknown, errors.New("agent get response missing agent_status")
	}
	return ParseAgentState(response.Result.Agent.Status), nil
}

func (c *CLI) ReadRecent(ctx context.Context, name string) (string, error) {
	result, err := c.run(ctx, "agent", "read", name, "--source", "recent-unwrapped", "--lines", "120")
	if err != nil {
		return "", err
	}
	var response struct {
		Result struct {
			Output string `json:"output"`
		} `json:"result"`
	}
	if err := decode(result.Stdout, &response); err != nil {
		return "", fmt.Errorf("parse agent read response: %w", err)
	}
	return response.Result.Output, nil
}

func (c *CLI) run(ctx context.Context, args ...string) (runner.Result, error) {
	result, err := c.runner.Run(ctx, "", c.executable, args...)
	if err != nil {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = err.Error()
		}
		return result, fmt.Errorf("herdr %s: %s", strings.Join(args, " "), message)
	}
	return result, nil
}

func decode(output string, target any) error {
	if err := json.Unmarshal([]byte(output), target); err != nil {
		return err
	}
	return nil
}
