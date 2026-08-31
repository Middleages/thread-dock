package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/pathscope"
	"thread-dock/internal/runner"
)

const promptTimeout = "3600000"

const maxRecentEvidenceBytes = 64 * 1024

var evidenceCredentialPattern = regexp.MustCompile(`(?im)(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|(?:AKIA|ASIA)[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|(?:^|[^A-Za-z0-9])["']?(?:token|secret|password|authorization|api[_-]?key|private[_-]?key|client[_-]?(?:secret|key)|[A-Za-z_][A-Za-z0-9_.-]*(?:token|secret|password|authorization|api[_-]?key|private[_-]?key|client[_-]?(?:secret|key)))["']?[ \t]*[:=][ \t]*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:Bearer[ \t]+)?[^\s,;}\]]+))`)
var providerSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

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

// GetWorkspace observes one exact Herdr Workspace. Herdr's workspace get
// response does not include a pane ID, so the read-only pane list is composed
// into this one identity observation. A root pane is considered identifiable
// only when exactly one pane in the active tab has the Workspace's canonical
// checkout path as its cwd.
func (c *CLI) GetWorkspace(ctx context.Context, workspaceID string) (WorkspaceInfo, bool, error) {
	if !validHerdrWorkspaceID(workspaceID) {
		return WorkspaceInfo{}, false, errors.New("Herdr workspace identity is invalid")
	}

	result, err := c.run(ctx, "workspace get", "workspace", "get", workspaceID)
	envelope, envelopeErr := decodeHerdrEnvelope(result.Stdout)
	if envelopeErr == nil && envelope.Error != nil && envelope.Error.Code == "workspace_not_found" {
		return WorkspaceInfo{}, false, nil
	}
	if err != nil {
		return WorkspaceInfo{}, false, err
	}
	if envelopeErr != nil || envelope.Error != nil || envelope.Result == nil {
		return WorkspaceInfo{}, false, safeError("workspace get", result.ExitCode)
	}
	var response workspaceResult
	if err := decodeStrictJSON(*envelope.Result, &response); err != nil || response.Type != "workspace_info" {
		return WorkspaceInfo{}, false, safeError("workspace get", result.ExitCode)
	}
	workspace := response.Workspace
	if workspace.WorkspaceID != workspaceID || !validHerdrWorkspaceID(workspace.WorkspaceID) || !validHerdrWorkspaceID(workspace.ActiveTabID) || workspace.AgentStatus == "" || !canonicalHerdrPath(workspace.Worktree.CheckoutPath) {
		return WorkspaceInfo{}, false, safeError("workspace get", result.ExitCode)
	}

	panes, err := c.run(ctx, "pane list", "pane", "list", "--workspace", workspaceID)
	if err != nil {
		return WorkspaceInfo{}, false, err
	}
	paneEnvelope, paneEnvelopeErr := decodeHerdrEnvelope(panes.Stdout)
	if paneEnvelopeErr != nil || paneEnvelope.Error != nil || paneEnvelope.Result == nil {
		return WorkspaceInfo{}, false, safeError("pane list", panes.ExitCode)
	}
	var paneResponse paneListResult
	if err := decodeStrictJSON(*paneEnvelope.Result, &paneResponse); err != nil || paneResponse.Panes == nil || paneResponse.Type != "pane_list" {
		return WorkspaceInfo{}, false, safeError("pane list", panes.ExitCode)
	}
	rootPaneID := ""
	rootPaneState := AgentStateUnknown
	for _, pane := range paneResponse.Panes {
		if pane.WorkspaceID != workspaceID || pane.TabID != workspace.ActiveTabID || pane.CWD != workspace.Worktree.CheckoutPath {
			continue
		}
		if !validHerdrPaneID(pane.PaneID) || !canonicalHerdrPath(pane.CWD) {
			return WorkspaceInfo{}, false, safeError("pane list", panes.ExitCode)
		}
		if rootPaneID != "" {
			return WorkspaceInfo{}, false, errors.New("herdr pane list returned ambiguous canonical root panes")
		}
		rootPaneID = pane.PaneID
		rootPaneState = ParseAgentState(pane.AgentStatus)
	}
	if rootPaneID == "" {
		return WorkspaceInfo{}, false, safeError("pane list", panes.ExitCode)
	}
	workspaceState := ParseAgentState(workspace.AgentStatus)
	if !safeWorkspaceState(workspaceState) || !safeWorkspaceState(rootPaneState) {
		return WorkspaceInfo{}, false, safeError("pane list", panes.ExitCode)
	}
	return WorkspaceInfo{WorkspaceID: workspace.WorkspaceID, RootPaneID: rootPaneID, Path: workspace.Worktree.CheckoutPath, State: rootPaneState}, true, nil
}

// CloseWorkspace closes exactly one Workspace. Herdr forgets a Workspace
// after a successful close, so a not-found response is an idempotent success.
func (c *CLI) CloseWorkspace(ctx context.Context, workspaceID string) error {
	if !validHerdrWorkspaceID(workspaceID) {
		return errors.New("Herdr workspace identity is invalid")
	}
	result, err := c.run(ctx, "workspace close", "workspace", "close", workspaceID)
	envelope, envelopeErr := decodeHerdrEnvelope(result.Stdout)
	if envelopeErr == nil && envelope.Error != nil && envelope.Error.Code == "workspace_not_found" {
		return nil
	}
	if err != nil {
		return err
	}
	if envelopeErr != nil || envelope.Error != nil || envelope.Result == nil {
		return safeError("workspace close", result.ExitCode)
	}
	var response closeResult
	if err := decodeStrictJSON(*envelope.Result, &response); err != nil || (response.Type != "ok" && response.Type != "workspace_closed") {
		return safeError("workspace close", result.ExitCode)
	}
	return nil
}

func validHerdrWorkspaceID(value string) bool {
	return providerSessionIDPattern.MatchString(value) && !strings.Contains(value, "..")
}

func validHerdrPaneID(value string) bool {
	return providerSessionIDPattern.MatchString(value) && !strings.Contains(value, "..")
}

func canonicalHerdrPath(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value && !strings.ContainsRune(value, '\x00')
}

type herdrProviderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type herdrEnvelope struct {
	ID     string              `json:"id"`
	Type   string              `json:"type"`
	Error  *herdrProviderError `json:"error"`
	Result *json.RawMessage    `json:"result"`
}

type workspaceResult struct {
	Type      string        `json:"type"`
	Workspace workspaceWire `json:"workspace"`
}

type workspaceWire struct {
	ActiveTabID string            `json:"active_tab_id"`
	AgentStatus string            `json:"agent_status"`
	Focused     bool              `json:"focused"`
	Label       string            `json:"label"`
	Number      int               `json:"number"`
	PaneCount   int               `json:"pane_count"`
	TabCount    int               `json:"tab_count"`
	WorkspaceID string            `json:"workspace_id"`
	Worktree    workspaceWorktree `json:"worktree"`
}

type workspaceWorktree struct {
	CheckoutPath     string `json:"checkout_path"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
	RepoKey          string `json:"repo_key"`
	RepoName         string `json:"repo_name"`
	RepoRoot         string `json:"repo_root"`
}

type paneListResult struct {
	Type  string     `json:"type"`
	Panes []paneWire `json:"panes"`
}

type paneWire struct {
	Agent                 string           `json:"agent"`
	AgentSession          paneAgentSession `json:"agent_session"`
	AgentStatus           string           `json:"agent_status"`
	CWD                   string           `json:"cwd"`
	Focused               bool             `json:"focused"`
	ForegroundCWD         string           `json:"foreground_cwd"`
	PaneID                string           `json:"pane_id"`
	Revision              int              `json:"revision"`
	Scroll                paneScroll       `json:"scroll"`
	TabID                 string           `json:"tab_id"`
	TerminalID            string           `json:"terminal_id"`
	TerminalTitle         string           `json:"terminal_title"`
	TerminalTitleStripped string           `json:"terminal_title_stripped"`
	WorkspaceID           string           `json:"workspace_id"`
}

type paneAgentSession struct {
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

type paneScroll struct {
	MaxOffsetFromBottom int `json:"max_offset_from_bottom"`
	OffsetFromBottom    int `json:"offset_from_bottom"`
	ViewportRows        int `json:"viewport_rows"`
}

type closeResult struct {
	Type string `json:"type"`
}

func decodeHerdrEnvelope(output string) (herdrEnvelope, error) {
	var envelope herdrEnvelope
	if err := decodeStrictJSON([]byte(output), &envelope); err != nil || envelope.ID == "" {
		return herdrEnvelope{}, errors.New("invalid Herdr response envelope")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &fields); err != nil {
		return herdrEnvelope{}, errors.New("invalid Herdr response envelope")
	}
	errorRaw, hasError := fields["error"]
	resultRaw, hasResult := fields["result"]
	if hasError == hasResult || (hasError && (envelope.Error == nil || bytes.Equal(bytes.TrimSpace(errorRaw), []byte("null")))) || (hasResult && (envelope.Result == nil || bytes.Equal(bytes.TrimSpace(resultRaw), []byte("null")))) {
		return herdrEnvelope{}, errors.New("Herdr response must contain exactly one result or error")
	}
	if envelope.Error != nil && !canonicalProviderCode(envelope.Error.Code) {
		return herdrEnvelope{}, errors.New("Herdr response error code is invalid")
	}
	return envelope, nil
}

func decodeStrictJSON(output []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON has trailing data")
		}
		return err
	}
	return nil
}

func canonicalProviderCode(value string) bool {
	return providerSessionIDPattern.MatchString(value)
}

func safeWorkspaceState(state AgentState) bool {
	return state == AgentStateIdle || state == AgentStateDone
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
	return c.findWorktree(ctx, cwd, "", path, label)
}

func (c *CLI) FindWorktreeByBranch(ctx context.Context, cwd, branch, label string) (Worktree, bool, error) {
	return c.findWorktree(ctx, cwd, branch, "", label)
}

func (c *CLI) findWorktree(ctx context.Context, cwd, branch, path, label string) (Worktree, bool, error) {
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
		if (branch == "" || candidate.Branch == branch) && (path == "" || candidate.Path == path) && (label == "" || candidate.Label == label || path != "") && candidate.Path != "" {
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

func (c *CLI) ResumeAgent(ctx context.Context, req ResumeAgentRequest) error {
	if strings.TrimSpace(req.Name) == "" || len(req.Name) > 32 || !validHerdrName(req.Name) || strings.TrimSpace(req.PaneID) == "" || strings.TrimSpace(req.SessionID) == "" || req.SessionID != strings.TrimSpace(req.SessionID) || strings.HasPrefix(strings.ToLower(req.SessionID), "herdr-terminal:") || !providerSessionIDPattern.MatchString(req.SessionID) {
		return errors.New("Herdr provider session identity is invalid")
	}
	_, err := c.run(ctx, "agent resume", "agent", "start", req.Name, "--kind", "opencode", "--pane", req.PaneID, "--", "--session", req.SessionID)
	return err
}

func validHerdrName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
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

// ReadReviewEvidence accepts only strict structured Reviewer output between
// the review markers. It returns no transcript text or unvalidated fields.
func (c *CLI) ReadReviewEvidence(ctx context.Context, name, expectedRequestID string) (ReviewEvidence, error) {
	recent, err := c.ReadRecent(ctx, name)
	if err != nil {
		return ReviewEvidence{}, err
	}
	if len(recent) > maxRecentEvidenceBytes || strings.ContainsRune(recent, '\x00') {
		return ReviewEvidence{}, errors.New("herdr review evidence is oversized or malformed")
	}
	if strings.TrimSpace(expectedRequestID) == "" || strings.TrimSpace(expectedRequestID) != expectedRequestID || evidenceCredentialPattern.MatchString(expectedRequestID) {
		return ReviewEvidence{}, errors.New("herdr review evidence has an invalid expected requestId")
	}
	payload, err := lastMarkedPayload(recent, THREADDOCK_REVIEW_BEGIN, THREADDOCK_REVIEW_END, "review evidence")
	if err != nil {
		return ReviewEvidence{}, err
	}
	var evidence ReviewEvidence
	if err := decodeEvidenceJSON([]byte(payload), &evidence); err != nil {
		return ReviewEvidence{}, errors.New("herdr review evidence is not structured JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil || !jsonArrayField(fields, "blockingFindings") || !jsonArrayField(fields, "riskCategories") {
		return ReviewEvidence{}, errors.New("herdr review evidence requires findings and risk category arrays")
	}
	if evidence.RequestID == "" || strings.TrimSpace(evidence.RequestID) != evidence.RequestID || evidence.RequestID != expectedRequestID || evidenceCredentialPattern.MatchString(evidence.RequestID) {
		return ReviewEvidence{}, errors.New("herdr review evidence has a stale or invalid requestId")
	}
	if evidence.Decision != "accept" && evidence.Decision != "block" || evidenceCredentialPattern.MatchString(evidence.Decision) {
		return ReviewEvidence{}, errors.New("herdr review evidence has an invalid decision")
	}
	if evidence.Decision == "accept" && len(evidence.BlockingFindings) != 0 {
		return ReviewEvidence{}, errors.New("accepted review evidence cannot contain blocking findings")
	}
	if evidence.Decision == "block" && len(evidence.BlockingFindings) == 0 {
		return ReviewEvidence{}, errors.New("blocked review evidence requires blocking findings")
	}
	seenIDs := make(map[string]struct{}, len(evidence.BlockingFindings))
	for i := range evidence.BlockingFindings {
		finding := &evidence.BlockingFindings[i]
		if finding.ID == "" || strings.TrimSpace(finding.ID) != finding.ID || evidenceCredentialPattern.MatchString(finding.ID) {
			return ReviewEvidence{}, errors.New("herdr review evidence contains an invalid finding ID")
		}
		if _, exists := seenIDs[finding.ID]; exists {
			return ReviewEvidence{}, errors.New("herdr review evidence contains duplicate finding IDs")
		}
		seenIDs[finding.ID] = struct{}{}
		if finding.Summary == "" || strings.TrimSpace(finding.Summary) != finding.Summary || evidenceCredentialPattern.MatchString(finding.Summary) {
			return ReviewEvidence{}, errors.New("herdr review evidence contains an invalid finding summary")
		}
		if len(finding.Paths) == 0 {
			return ReviewEvidence{}, errors.New("herdr review evidence finding paths are required")
		}
		for _, path := range finding.Paths {
			if !canonicalReviewPath(path) || evidenceCredentialPattern.MatchString(path) {
				return ReviewEvidence{}, errors.New("herdr review evidence contains an invalid finding path")
			}
		}
	}
	seenRisk := make(map[string]struct{}, len(evidence.RiskCategories))
	for _, category := range evidence.RiskCategories {
		if !contract.IsRiskCategory(category) || evidenceCredentialPattern.MatchString(category) {
			return ReviewEvidence{}, errors.New("herdr review evidence contains an invalid risk category")
		}
		if _, exists := seenRisk[category]; exists {
			return ReviewEvidence{}, errors.New("herdr review evidence contains duplicate risk categories")
		}
		seenRisk[category] = struct{}{}
	}
	return evidence, nil
}

func jsonArrayField(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	return ok && len(bytes.TrimSpace(raw)) > 0 && bytes.TrimSpace(raw)[0] == '['
}

func canonicalReviewPath(path string) bool {
	if path == "" || strings.TrimSpace(path) != path {
		return false
	}
	normalized, err := pathscope.Normalize(path)
	return err == nil && normalized == path
}

func lastEvidencePayload(recent string) (string, error) {
	return lastMarkedPayload(recent, THREADDOCK_EVIDENCE_BEGIN, THREADDOCK_EVIDENCE_END, "evidence")
}

// lastMarkedPayload extracts the latest complete structured envelope while
// preserving the Builder parser's sidebar-boundary and payload-size rules.
// Marker names are parameters so Reviewer output cannot accidentally share
// the Builder marker protocol.
func lastMarkedPayload(recent, beginMarker, endMarker, kind string) (string, error) {
	lines := strings.Split(recent, "\n")
	auxiliaryColumn := 0
	var payloads []string
	inEnvelope := false
	envelopeAuxiliaryColumn := 0
	var payloadLines []string
	rawPayloadBytes := 0
	for _, line := range lines {
		text := strings.TrimSuffix(line, "\r")
		if inEnvelope {
			if actualEvidenceMarker(text, beginMarker, envelopeAuxiliaryColumn) {
				return "", fmt.Errorf("herdr %s has an incomplete envelope", kind)
			}
			if actualEvidenceMarker(text, endMarker, envelopeAuxiliaryColumn) {
				payload := strings.Join(payloadLines, "\n")
				if rawPayloadBytes > MaxEvidencePayloadBytes || len(payload) > MaxEvidencePayloadBytes {
					return "", fmt.Errorf("herdr %s payload is oversized", kind)
				}
				payloads = append(payloads, payload)
				inEnvelope = false
				payloadLines = nil
				continue
			}
			rawPayloadBytes += len(text) + 1
			if rawPayloadBytes > MaxEvidencePayloadBytes {
				return "", fmt.Errorf("herdr %s payload is oversized", kind)
			}
			if clean, keep := cleanEvidenceLine(text, envelopeAuxiliaryColumn); keep {
				payloadLines = append(payloadLines, clean)
			}
			continue
		}
		if actualEvidenceMarker(text, beginMarker, auxiliaryColumn) {
			inEnvelope = true
			envelopeAuxiliaryColumn = auxiliaryColumn
			payloadLines = nil
			rawPayloadBytes = 0
			continue
		}
		if actualEvidenceMarker(text, endMarker, auxiliaryColumn) {
			return "", fmt.Errorf("herdr %s has an unmatched envelope marker", kind)
		}
		if boundary, ok := sidebarBoundary(text); ok {
			auxiliaryColumn = boundary
		}
	}
	if inEnvelope || len(payloads) == 0 {
		return "", fmt.Errorf("herdr %s has no complete envelope", kind)
	}
	return extractCompleteObject(payloads[len(payloads)-1], kind)
}

const (
	minimumAuxiliaryColumn = 32
	minimumAuxiliaryGap    = 8
)

func actualEvidenceMarker(line, marker string, auxiliaryColumn int) bool {
	first := firstNonSpace(line)
	if first < 0 || isPromptEchoLine(line) || (auxiliaryColumn > 0 && first >= auxiliaryColumn) {
		return false
	}
	value := line[first:]
	if value == marker || (strings.HasPrefix(value, marker) && strings.TrimSpace(value[len(marker):]) == "") {
		return true
	}
	if !strings.HasPrefix(value, marker) {
		return false
	}
	remainder := value[len(marker):]
	trimmed := strings.TrimLeft(remainder, " \t")
	leading := len(remainder) - len(trimmed)
	if leading < minimumAuxiliaryGap || auxiliaryColumn == 0 {
		return false
	}
	return first+len(marker)+leading == auxiliaryColumn
}

func sidebarBoundary(line string) (int, bool) {
	first := firstNonSpace(line)
	if first < minimumAuxiliaryColumn || isPromptEchoLine(line) || !isOpenCodeAuxiliaryText(line[first:]) {
		return 0, false
	}
	return first, true
}

func cleanEvidenceLine(line string, auxiliaryColumn int) (string, bool) {
	first := firstNonSpace(line)
	if first < 0 {
		return "", false
	}
	if auxiliaryColumn > 0 && first >= auxiliaryColumn && isOpenCodeAuxiliaryText(line[first:]) {
		return "", false
	}
	if prefix, suffix, ok := wideSuffix(line); ok {
		if auxiliaryColumn > 0 && suffix >= auxiliaryColumn && isOpenCodeAuxiliaryText(line[suffix:]) {
			line = prefix
		}
	}
	if strings.TrimSpace(line) == "" {
		return "", false
	}
	return line, true
}

// isOpenCodeAuxiliaryText deliberately recognizes stable UI labels rather
// than arbitrary right-column content. Dynamic or unknown text is retained so
// strict JSON decoding can reject it when it is actually left-pane data.
func isOpenCodeAuxiliaryText(value string) bool {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{
		"Context",
		"LSP",
		"LSPs are disabled",
		"Getting started",
		"Connect provider",
		"OpenCode includes",
		"Connect from",
		"Build ·",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func isPromptEchoLine(line string) bool {
	first := firstNonSpace(line)
	return first >= 0 && strings.HasPrefix(line[first:], "┃")
}

func firstNonSpace(value string) int {
	for i := 0; i < len(value); i++ {
		if value[i] != ' ' && value[i] != '\t' {
			return i
		}
	}
	return -1
}

// wideSuffix finds a non-JSON-string suffix separated by a wide whitespace
// run. Tracking JSON strings prevents spaces inside a quoted command/value
// from being mistaken for the auxiliary pane boundary.
func wideSuffix(line string) (prefix string, suffix int, ok bool) {
	inString := false
	escaped := false
	for i := 0; i < len(line); i++ {
		char := line[i]
		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		if char == '"' {
			inString = true
			continue
		}
		if char != ' ' && char != '\t' {
			continue
		}
		start := i
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i-start >= minimumAuxiliaryGap && firstNonSpace(line[:start]) >= 0 && i < len(line) && line[i] != '\r' {
			return line[:start], i, true
		}
		i--
	}
	return line, 0, false
}

func extractCompleteEvidenceObject(payload string) (string, error) {
	return extractCompleteObject(payload, "evidence")
}

func extractCompleteObject(payload, kind string) (string, error) {
	payload = joinDisplayWrappedJSON(payload)
	payload = strings.TrimSpace(payload)
	if payload == "" || payload[0] != '{' {
		return "", fmt.Errorf("herdr %s is not structured JSON", kind)
	}
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(payload); i++ {
		char := payload[i]
		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return "", fmt.Errorf("herdr %s is not structured JSON", kind)
			}
			if depth == 0 {
				if strings.TrimSpace(payload[i+1:]) != "" {
					return "", fmt.Errorf("herdr %s has trailing data", kind)
				}
				return payload[:i+1], nil
			}
		}
	}
	return "", fmt.Errorf("herdr %s is not structured JSON", kind)
}

// joinDisplayWrappedJSON repairs physical terminal wraps that occur while a
// JSON string is being displayed. The indentation before the first payload
// object is the only display prefix we know to remove from continuation
// lines. Newlines outside strings remain intact so strict JSON and trailing
// data checks keep their existing behavior.
func joinDisplayWrappedJSON(payload string) string {
	lines := strings.Split(payload, "\n")
	if len(lines) < 2 {
		return payload
	}
	displayIndent := leadingWhitespace(lines[0])
	if displayIndent == "" {
		return payload
	}

	var joined strings.Builder
	inString := false
	escaped := false
	for i, line := range lines {
		if i > 0 {
			if inString {
				if hasExactDisplayIndent(line, displayIndent) {
					line = line[len(displayIndent):]
				} else {
					joined.WriteByte('\n')
				}
			} else {
				joined.WriteByte('\n')
			}
		}
		joined.WriteString(line)
		for j := 0; j < len(line); j++ {
			char := line[j]
			if inString {
				if escaped {
					escaped = false
				} else if char == '\\' {
					escaped = true
				} else if char == '"' {
					inString = false
				}
			} else if char == '"' {
				inString = true
			}
		}
	}
	return joined.String()
}

func hasExactDisplayIndent(line, displayIndent string) bool {
	if !strings.HasPrefix(line, displayIndent) || len(line) == len(displayIndent) {
		return false
	}
	next := line[len(displayIndent)]
	return next != ' ' && next != '\t'
}

func leadingWhitespace(value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] != ' ' && value[i] != '\t' {
			return value[:i]
		}
	}
	return value
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
				Terminal  string `json:"terminal_id"`
				Session   struct {
					Value string `json:"value"`
				} `json:"agent_session"`
			} `json:"agent"`
		} `json:"result"`
	}
	if err := decode(result.Stdout, &response); err != nil || response.Result.Agent.Name == "" || response.Result.Agent.PaneID == "" {
		return AgentInfo{}, safeError("agent get", result.ExitCode)
	}
	sessionID := strings.TrimSpace(response.Result.Agent.Session.Value)
	if sessionID == "" {
		terminalID := strings.TrimSpace(response.Result.Agent.Terminal)
		if terminalID == "" {
			return AgentInfo{}, safeError("agent get", result.ExitCode)
		}
		sessionID = "herdr-terminal:" + terminalID
	}
	return AgentInfo{Name: response.Result.Agent.Name, SessionID: sessionID, WorkspaceID: response.Result.Agent.Workspace, PaneID: response.Result.Agent.PaneID, Path: response.Result.Agent.CWD, State: ParseAgentState(response.Result.Agent.Status), StateChangeSeq: response.Result.Agent.Seq}, nil
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
