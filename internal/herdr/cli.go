package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"thread-dock/internal/runner"
)

const promptTimeout = "3600000"

const maxRecentEvidenceBytes = 64 * 1024

var evidenceCredentialPattern = regexp.MustCompile(`(?im)(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|(?:AKIA|ASIA)[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|(?:^|[^A-Za-z0-9])(?:token|secret|password|authorization|api[_-]?key|private[_-]?key|client[_-]?(?:secret|key)|[A-Za-z_][A-Za-z0-9_.-]*(?:token|secret|password|authorization|api[_-]?key|private[_-]?key|client[_-]?(?:secret|key)))[ \t]*[:=][ \t]*(?:Bearer[ \t]+)?[^\s,;}\]]+)`)

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
	return c.decodeWorktree(ctx, "worktree create", result.Stdout, result.ExitCode)
}

func (c *CLI) OpenWorktree(ctx context.Context, req OpenWorktreeRequest) (Worktree, error) {
	result, err := c.run(ctx, "worktree open", "worktree", "open", "--cwd", req.Cwd, "--path", req.Path, "--label", req.Label, "--no-focus")
	if err != nil {
		return Worktree{}, err
	}
	return c.decodeWorktree(ctx, "worktree open", result.Stdout, result.ExitCode)
}

func (c *CLI) decodeWorktree(ctx context.Context, operation, output string, exitCode int) (Worktree, error) {
	var response struct {
		Result struct {
			RootPane struct {
				PaneID      string `json:"pane_id"`
				WorkspaceID string `json:"workspace_id"`
				CWD         string `json:"cwd"`
			} `json:"root_pane"`
			Workspace struct {
				WorkspaceID string `json:"workspace_id"`
				Worktree    struct {
					Path string `json:"checkout_path"`
				} `json:"worktree"`
			} `json:"workspace"`
			Worktree struct {
				Path string `json:"path"`
			} `json:"worktree"`
		} `json:"result"`
	}
	if err := decode(output, &response); err != nil {
		return Worktree{}, safeError(operation, exitCode)
	}
	workspaceID := response.Result.Workspace.WorkspaceID
	if workspaceID == "" {
		workspaceID = response.Result.RootPane.WorkspaceID
	}
	paneID := response.Result.RootPane.PaneID
	path := response.Result.Worktree.Path
	if path == "" {
		path = response.Result.Workspace.Worktree.Path
	}
	if path == "" {
		path = response.Result.RootPane.CWD
	}
	if workspaceID == "" || paneID == "" || response.Result.RootPane.WorkspaceID != workspaceID {
		return Worktree{}, safeError(operation, exitCode)
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
			if path == "" {
				return Worktree{}, safeError(operation, exitCode)
			}
			return Worktree{WorkspaceID: workspaceID, PaneID: paneID, Path: path}, nil
		}
	}
	return Worktree{}, safeError("pane list", panes.ExitCode)
}

// FindWorktree reconciles a durable path/label with the worktree list without
// creating a second workspace. Herdr's list payload has changed shape across
// minor releases, so only the stable IDs and path fields are inspected.
func (c *CLI) FindWorktree(ctx context.Context, cwd, path, label string) (Worktree, bool, error) {
	if strings.TrimSpace(cwd) == "" {
		return Worktree{}, false, errors.New("Herdr worktree lookup requires repository cwd")
	}
	result, err := c.run(ctx, "worktree list", "worktree", "list", "--cwd", cwd)
	if err != nil {
		return Worktree{}, false, err
	}
	var response struct {
		Result struct {
			Worktrees []struct {
				Branch          string `json:"branch"`
				Label           string `json:"label"`
				OpenWorkspaceID string `json:"open_workspace_id"`
				Path            string `json:"path"`
			} `json:"worktrees"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &response); err != nil {
		return Worktree{}, false, safeError("worktree list", result.ExitCode)
	}
	for _, candidate := range response.Result.Worktrees {
		if (path == "" || candidate.Path == path) && (label == "" || candidate.Label == label || path != "") && candidate.Path != "" {
			workspaceID := candidate.OpenWorkspaceID
			if workspaceID == "" {
				return Worktree{}, false, ErrClosedWorkspace
			}
			paneID := ""
			if workspaceID != "" {
				var err error
				paneID, err = c.firstPane(ctx, workspaceID)
				if err != nil {
					return Worktree{}, false, err
				}
			}
			if paneID == "" {
				return Worktree{}, false, errors.New("Herdr workspace has no open pane")
			}
			return Worktree{WorkspaceID: workspaceID, PaneID: paneID, Path: candidate.Path}, true, nil
		}
	}
	return Worktree{}, false, nil
}

func (c *CLI) firstPane(ctx context.Context, workspaceID string) (string, error) {
	result, err := c.run(ctx, "pane list", "pane", "list", "--workspace", workspaceID)
	if err != nil {
		return "", err
	}
	var response struct {
		Result struct {
			Panes []struct {
				PaneID      string `json:"pane_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := decode(result.Stdout, &response); err != nil {
		return "", err
	}
	for _, pane := range response.Result.Panes {
		if pane.WorkspaceID == workspaceID {
			return pane.PaneID, nil
		}
	}
	return "", nil
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

// ReadPromptReceipt combines the agent sequence and recent output into the
// narrow observation needed by recovery. Terminal output is not returned or
// persisted; only exact request-ID presence is exposed to the orchestrator.
func (c *CLI) ReadPromptReceipt(ctx context.Context, name, requestID string) (AgentInfo, bool, error) {
	recent, err := c.ReadRecent(ctx, name)
	if err != nil {
		return AgentInfo{}, false, err
	}
	observed := strings.TrimSpace(requestID) != "" && strings.Contains(recent, requestID)
	info, err := c.GetInfo(ctx, name)
	if err != nil {
		return AgentInfo{}, false, err
	}
	return info, observed, nil
}

// ReadEvidence accepts only the strict structured result emitted by the
// Builder protocol. It never forwards raw terminal text to callers.
func (c *CLI) ReadEvidence(ctx context.Context, name string) (Evidence, error) {
	recent, err := c.ReadRecent(ctx, name)
	if err != nil {
		return Evidence{}, err
	}
	if len(recent) > maxRecentEvidenceBytes || strings.ContainsRune(recent, '\x00') {
		return Evidence{}, errors.New("herdr evidence is oversized or malformed")
	}
	payload, err := lastEvidencePayload(recent)
	if err != nil {
		return Evidence{}, err
	}
	var evidence Evidence
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&evidence); err != nil {
		return Evidence{}, errors.New("herdr evidence is not structured JSON")
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return Evidence{}, errors.New("herdr evidence has trailing data")
	} else if !errors.Is(err, io.EOF) {
		return Evidence{}, errors.New("herdr evidence has trailing data")
	}
	evidence.RequestID = strings.TrimSpace(evidence.RequestID)
	evidence.CommitSHA = strings.TrimSpace(evidence.CommitSHA)
	if evidence.RequestID == "" || !validEvidenceSHA(evidence.CommitSHA) || len(evidence.Verification) == 0 || evidenceCredentialPattern.MatchString(evidence.RequestID) || evidenceCredentialPattern.MatchString(evidence.CommitSHA) {
		return Evidence{}, errors.New("herdr evidence is missing requestId or commitSha")
	}
	for i := range evidence.Verification {
		check := &evidence.Verification[i]
		check.Command = strings.TrimSpace(check.Command)
		check.Outcome = strings.ToLower(strings.TrimSpace(check.Outcome))
		check.Duration = strings.TrimSpace(check.Duration)
		if check.Command == "" || check.Duration == "" || (check.Outcome != "passed" && check.Outcome != "failed") || evidenceCredentialPattern.MatchString(check.Command) || evidenceCredentialPattern.MatchString(check.Outcome) || evidenceCredentialPattern.MatchString(check.Duration) {
			return Evidence{}, errors.New("herdr evidence contains an invalid verification")
		}
		if _, err := time.ParseDuration(check.Duration); err != nil {
			return Evidence{}, errors.New("herdr evidence contains an invalid duration")
		}
	}
	return evidence, nil
}

func lastEvidencePayload(recent string) (string, error) {
	var payloads []string
	inEnvelope := false
	start := 0
	offset := 0
	for _, line := range strings.SplitAfter(recent, "\n") {
		text := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		switch text {
		case THREADDOCK_EVIDENCE_BEGIN:
			if inEnvelope {
				return "", errors.New("herdr evidence has an incomplete envelope")
			}
			inEnvelope = true
			start = offset + len(line)
		case THREADDOCK_EVIDENCE_END:
			if !inEnvelope {
				return "", errors.New("herdr evidence has an unmatched envelope marker")
			}
			payload := recent[start:offset]
			if len(payload) > MaxEvidencePayloadBytes {
				return "", errors.New("herdr evidence payload is oversized")
			}
			payloads = append(payloads, payload)
			inEnvelope = false
		}
		offset += len(line)
	}
	if inEnvelope || len(payloads) == 0 {
		return "", errors.New("herdr evidence has no complete envelope")
	}
	return strings.TrimSpace(payloads[len(payloads)-1]), nil
}

func validEvidenceSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func (c *CLI) GetInfo(ctx context.Context, name string) (AgentInfo, error) {
	result, err := c.run(ctx, "agent get", "agent", "get", name)
	if err != nil {
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(result.Stdout), &failure) == nil && failure.Error.Code == "agent_not_found" {
			return AgentInfo{}, ErrAgentNotFound
		}
		return AgentInfo{}, err
	}
	var response struct {
		Result struct {
			Agent struct {
				Name      string `json:"name"`
				PaneID    string `json:"pane_id"`
				Workspace string `json:"workspace_id"`
				CWD       string `json:"cwd"`
				Status    string `json:"agent_status"`
				Seq       int64  `json:"state_change_seq"`
				Session   struct {
					Value string `json:"value"`
				} `json:"agent_session"`
			} `json:"agent"`
		} `json:"result"`
	}
	if err := decode(result.Stdout, &response); err != nil || response.Result.Agent.Name == "" || response.Result.Agent.PaneID == "" {
		return AgentInfo{}, safeError("agent get", result.ExitCode)
	}
	return AgentInfo{Name: response.Result.Agent.Name, SessionID: response.Result.Agent.Session.Value, WorkspaceID: response.Result.Agent.Workspace, PaneID: response.Result.Agent.PaneID, Path: response.Result.Agent.CWD, State: ParseAgentState(response.Result.Agent.Status), StateChangeSeq: response.Result.Agent.Seq}, nil
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
