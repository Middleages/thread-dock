package main

import (
	"sort"
	"strings"
	"time"
)

// ExecutionState describes runtime execution only. It must not be used as a
// proxy for GitHub work completion or verification.
type ExecutionState string

const (
	ExecutionQueued    ExecutionState = "queued"
	ExecutionRunning   ExecutionState = "running"
	ExecutionWaiting   ExecutionState = "waiting"
	ExecutionCompleted ExecutionState = "completed"
	ExecutionFailed    ExecutionState = "failed"
	ExecutionCancelled ExecutionState = "cancelled"
	ExecutionUnknown   ExecutionState = "unknown"
)

type VerificationStatus string

const (
	VerificationUnverified VerificationStatus = "unverified"
	VerificationPartial    VerificationStatus = "partial"
	VerificationVerified   VerificationStatus = "verified"
)

// VerificationState is the canonical verification result exposed to the UI.
// Codes explain conditions; success is represented by Status=verified rather
// than a success reason such as fully_verified.
type VerificationState struct {
	Status    VerificationStatus `json:"status"`
	Codes     []string            `json:"codes,omitempty"`
	Summary   string              `json:"summary,omitempty"`
	AnchorSHA string              `json:"anchorSha,omitempty"`
}

type ObservationProvenance struct {
	Source      string     `json:"source,omitempty"`
	ObservedAt  *time.Time `json:"observedAt,omitempty"`
	CapturedAt  *time.Time `json:"capturedAt,omitempty"`
	Scope       string     `json:"scope,omitempty"`
	Stale       bool       `json:"stale,omitempty"`
	Partial     bool       `json:"partial,omitempty"`
	Fallback    bool       `json:"fallback,omitempty"`
	Notice      string     `json:"notice,omitempty"`
}

type AgentObservation struct {
	AgentID       string                `json:"agentId"`
	ParentAgentID string                `json:"parentAgentId,omitempty"`
	Role          string                `json:"role,omitempty"`
	Runtime       string                `json:"runtime,omitempty"`
	TaskID        string                `json:"taskId,omitempty"`
	ExecutionRef  string                `json:"executionRef,omitempty"`
	Execution     ExecutionState        `json:"execution"`
	Provenance    ObservationProvenance `json:"provenance"`
}

type TaskObservation struct {
	TaskID         string     `json:"taskId"`
	ParentTaskID   string     `json:"parentTaskId,omitempty"`
	AgentID        string     `json:"agentId,omitempty"`
	Role           string     `json:"role,omitempty"`
	Objective      string     `json:"objective,omitempty"`
	InputRevision  string     `json:"inputRevision,omitempty"`
	Constraints    []string   `json:"constraints,omitempty"`
	Acceptance     []string   `json:"acceptance,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
}

type TaskResult struct {
	ResultID       string              `json:"resultId"`
	TaskID         string              `json:"taskId"`
	AgentID        string              `json:"agentId,omitempty"`
	Status         ExecutionState      `json:"status"`
	OutputRevision string              `json:"outputRevision,omitempty"`
	Summary        string              `json:"summary,omitempty"`
	Findings       []string            `json:"findings,omitempty"`
	Verification   VerificationStatus  `json:"verification"`
	NextAction     string              `json:"nextAction,omitempty"`
	CreatedAt      *time.Time          `json:"createdAt,omitempty"`
}

type EvidenceItem struct {
	Type       string     `json:"type"`
	Source     string     `json:"source,omitempty"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
	Reference  string     `json:"reference,omitempty"`
	Revision   string     `json:"revision,omitempty"`
	Claim      string     `json:"claim,omitempty"`
}

func NormalizeExecutionState(raw string) ExecutionState {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "queued":
		return ExecutionQueued
	case "running":
		return ExecutionRunning
	case "waiting":
		return ExecutionWaiting
	case "done", "completed", "complete":
		return ExecutionCompleted
	case "failed", "failure", "error":
		return ExecutionFailed
	case "cancelled", "canceled":
		return ExecutionCancelled
	default:
		return ExecutionUnknown
	}
}

func NormalizeVerificationState(state VerificationState) VerificationState {
	switch state.Status {
	case VerificationVerified, VerificationPartial, VerificationUnverified:
	default:
		state.Status = VerificationUnverified
	}

	codes := make([]string, 0, len(state.Codes))
	seen := make(map[string]struct{}, len(state.Codes))
	for _, code := range state.Codes {
		code = strings.TrimSpace(code)
		if code == "" || code == "fully_verified" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	state.Codes = codes
	return state
}

// EvidenceMatchesCandidate reports whether the evidence revision matches the
// candidate anchor SHA. It supports exact match or prefix match when both
// revisions are at least 7 characters long.
func EvidenceMatchesCandidate(state VerificationState, evidence EvidenceItem) bool {
	anchor := strings.ToLower(strings.TrimSpace(state.AnchorSHA))
	revision := strings.ToLower(strings.TrimSpace(evidence.Revision))
	if anchor == "" || revision == "" {
		return false
	}
	if anchor == revision {
		return true
	}
	const minSHAPrefix = 7
	if len(anchor) >= minSHAPrefix && len(revision) >= minSHAPrefix {
		return strings.HasPrefix(anchor, revision) || strings.HasPrefix(revision, anchor)
	}
	return false
}

// AgentWorkflow is the canonical additive read model exposing agent execution,
// lineage, work correlation, and verification state to the Wails UI.
type AgentWorkflow struct {
	Agents            []AgentObservation    `json:"agents"`
	Tasks             []TaskObservation     `json:"tasks"`
	Results           []TaskResult          `json:"results"`
	ExecutionState    ExecutionState        `json:"executionState"`
	GitHubWorkState   string                `json:"githubWorkState,omitempty"`
	VerificationState VerificationState      `json:"verificationState"`
	CandidateRevision string                `json:"candidateRevision,omitempty"`
	EvidenceRefs      []string              `json:"evidenceRefs"`
	Notices           []string              `json:"notices"`
	Provenance        ObservationProvenance `json:"provenance"`
}

func repositoryFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	prefix := "https://github.com/"
	if !strings.HasPrefix(rawURL, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(rawURL, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		if parts[0] == "users" || parts[0] == "orgs" {
			return ""
		}
		repo := strings.TrimSuffix(parts[1], ".git")
		return parts[0] + "/" + repo
	}
	return ""
}

// CorrelateAgentWorkflow binds Herdr agent observations and GitHub work state into
// a canonical AgentWorkflow read model. It guarantees that Herdr execution completion
// does not imply GitHub work completion or verification.
func CorrelateAgentWorkflow(project *Project, work *WorkItem, herdr *HerdrSnapshot) *AgentWorkflow {
	if work == nil || herdr == nil {
		return nil
	}

	primaryURL := ""
	if work.GitHub != nil && work.GitHub.URL != "" {
		primaryURL = work.GitHub.URL
	} else {
		for _, link := range work.Links {
			switch link.Kind {
			case "github", "issue", "pull_request", "project_item":
				if link.URL != "" {
					primaryURL = link.URL
					break
				}
			}
		}
	}

	repo := repositoryFromURL(primaryURL)
	if repo == "" && project != nil {
		for _, link := range project.Links {
			if r := repositoryFromURL(link.URL); r != "" {
				repo = r
				break
			}
		}
	}

	projectURLs := make(map[string]struct{})
	if project != nil {
		for _, link := range project.Links {
			if link.URL != "" {
				projectURLs[link.URL] = struct{}{}
			}
		}
	}

	var matched []HerdrConnection
	var matchType string

	// 1. Issue URL match
	if primaryURL != "" {
		for _, conn := range herdr.Connections {
			if conn.IssueURL != "" && conn.IssueURL == primaryURL {
				matched = append(matched, conn)
				matchType = "issue"
			}
		}
	}

	// 2. Project URL match
	if len(matched) == 0 && len(projectURLs) > 0 {
		for _, conn := range herdr.Connections {
			if conn.ProjectURL != "" {
				if _, ok := projectURLs[conn.ProjectURL]; ok {
					matched = append(matched, conn)
					matchType = "project"
				}
			}
		}
	}

	// 3. Fallback: coordinator connection for repository
	if len(matched) == 0 && repo != "" {
		for _, conn := range herdr.Connections {
			if conn.IssueURL == "" && conn.ProjectURL == "" && conn.Role == "coordinator" && conn.Repository == repo {
				matched = append(matched, conn)
				matchType = "coordinator"
			}
		}
	}

	// Legacy / no-Agent fallback
	if len(matched) == 0 {
		return nil
	}

	var notices []string

	// Ambiguity check: multiple distinct sessions matched
	sessions := make(map[string]struct{})
	for _, conn := range matched {
		sessions[conn.Session] = struct{}{}
	}
	if len(sessions) > 1 {
		notices = append(notices, "복수 세션이 동일 작업에 바인딩되어 관찰이 모호합니다.")
	}

	agents := make([]AgentObservation, 0, len(matched))
	for _, conn := range matched {
		agentID := conn.AgentName
		if agentID == "" {
			agentID = conn.Session + ":" + conn.PaneID
		}
		execState := NormalizeExecutionState(conn.AgentStatus)
		agents = append(agents, AgentObservation{
			AgentID:      agentID,
			Role:         conn.Role,
			Runtime:      "herdr",
			ExecutionRef: conn.Session + "/" + conn.PaneID,
			Execution:    execState,
			Provenance: ObservationProvenance{
				Source:     "herdr",
				ObservedAt: conn.ObservedAt,
				Scope:      conn.Session,
			},
		})
	}

	overallExecution := ExecutionUnknown
	hasRunning := false
	hasWaiting := false
	hasCompleted := false
	hasFailed := false
	for _, a := range agents {
		switch a.Execution {
		case ExecutionRunning:
			hasRunning = true
		case ExecutionWaiting:
			hasWaiting = true
		case ExecutionCompleted:
			hasCompleted = true
		case ExecutionFailed:
			hasFailed = true
		}
	}
	if hasRunning {
		overallExecution = ExecutionRunning
	} else if hasWaiting {
		overallExecution = ExecutionWaiting
	} else if hasFailed {
		overallExecution = ExecutionFailed
	} else if hasCompleted {
		overallExecution = ExecutionCompleted
	} else if len(agents) > 0 {
		overallExecution = agents[0].Execution
	}

	// GitHub Work State: explicitly preserved from work, never overwritten by Herdr completion
	ghWorkState := work.State
	if work.GitHub != nil && work.GitHub.State != "" {
		ghWorkState = work.GitHub.State
	}

	candidateSHA := ""
	if work.GitHub != nil && work.GitHub.Fields != nil {
		if sha, ok := work.GitHub.Fields["headSha"]; ok && sha != "" {
			candidateSHA = sha
		} else if sha, ok := work.GitHub.Fields["sha"]; ok && sha != "" {
			candidateSHA = sha
		}
	}

	vState := VerificationState{
		Status:    VerificationUnverified,
		AnchorSHA: candidateSHA,
	}

	if candidateSHA == "" {
		vState.Codes = append(vState.Codes, "awaiting_pr")
		vState.Summary = "PR 생성 대기 중 (로컬 작업 중)"
	} else {
		if work.GitHub != nil && len(work.GitHub.Checks) > 0 {
			ciPending := false
			ciFailed := false
			for _, check := range work.GitHub.Checks {
				conc := strings.ToLower(check.Conclusion)
				stat := strings.ToLower(check.Status)
				if conc == "failure" || conc == "timed_out" || conc == "action_required" {
					ciFailed = true
				} else if stat == "in_progress" || stat == "queued" || stat == "pending" {
					ciPending = true
				}
			}
			if ciFailed {
				vState.Codes = append(vState.Codes, "ci_failed")
			} else if ciPending {
				vState.Codes = append(vState.Codes, "ci_pending")
			}
		}
		vState.Codes = append(vState.Codes, "reviewer_missing")
		vState.Summary = "리뷰어 검증 증거 대기 중"
	}
	vState = NormalizeVerificationState(vState)

	observedAt := herdr.ObservedAt
	workflow := &AgentWorkflow{
		Agents:            agents,
		Tasks:             []TaskObservation{},
		Results:           []TaskResult{},
		ExecutionState:    overallExecution,
		GitHubWorkState:   ghWorkState,
		VerificationState: vState,
		CandidateRevision: candidateSHA,
		EvidenceRefs:      append([]string{}, work.EvidenceRefs...),
		Notices:           notices,
		Provenance: ObservationProvenance{
			Source:     "herdr+github",
			ObservedAt: &observedAt,
			Scope:      matchType,
		},
	}
	return workflow
}

func enrichSnapshotWithAgentWorkflow(snapshot *Snapshot, herdr *HerdrSnapshot) {
	if snapshot == nil || herdr == nil {
		return
	}
	for i := range snapshot.Projects {
		for j := range snapshot.Projects[i].WorkItems {
			snapshot.Projects[i].WorkItems[j].AgentWorkflow = CorrelateAgentWorkflow(&snapshot.Projects[i], &snapshot.Projects[i].WorkItems[j], herdr)
		}
	}
}
