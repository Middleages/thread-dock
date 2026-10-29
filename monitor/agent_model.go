package main

import (
	"sort"
	"strings"
	"time"
)

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
	TaskID        string     `json:"taskId"`
	ParentTaskID  string     `json:"parentTaskId,omitempty"`
	AgentID       string     `json:"agentId,omitempty"`
	Role          string     `json:"role,omitempty"`
	Objective     string     `json:"objective,omitempty"`
	InputRevision string     `json:"inputRevision,omitempty"`
	Constraints   []string   `json:"constraints,omitempty"`
	Acceptance    []string   `json:"acceptance,omitempty"`
	CreatedAt     *time.Time `json:"createdAt,omitempty"`
}

type TaskResult struct {
	ResultID       string             `json:"resultId"`
	TaskID         string             `json:"taskId"`
	AgentID        string             `json:"agentId,omitempty"`
	Status         ExecutionState     `json:"status"`
	OutputRevision string             `json:"outputRevision,omitempty"`
	Summary        string             `json:"summary,omitempty"`
	Findings       []string           `json:"findings,omitempty"`
	Verification   VerificationStatus `json:"verification"`
	NextAction     string             `json:"nextAction,omitempty"`
	CreatedAt      *time.Time         `json:"createdAt,omitempty"`
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
	case "running", "working":
		return ExecutionRunning
	case "waiting", "blocked", "idle":
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

func normalizeReviewerLogins(raw string) string {
	seen := map[string]bool{}
	values := []string{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "@")))
		if value != "" && !seen[value] { seen[value] = true; values = append(values, value) }
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func reviewerAllowed(raw, login string) bool {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" { return false }
	for _, allowed := range strings.Split(normalizeReviewerLogins(raw), ",") { if allowed == login { return true } }
	return false
}

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
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(rawURL, prefix), "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" || parts[0] == "users" || parts[0] == "orgs" {
		return ""
	}
	return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
}

func herdrAgentIdentity(conn HerdrConnection, duplicate bool) string {
	name := strings.TrimSpace(conn.AgentName)
	if name != "" && !duplicate {
		return name
	}
	parts := []string{"herdr", conn.Session, conn.WorkspaceID, conn.TabID, conn.PaneID}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ":")
}

func herdrExecutionRef(conn HerdrConnection) string {
	session := strings.TrimSpace(conn.Session)
	pane := strings.TrimSpace(conn.PaneID)
	if pane == "" {
		return session
	}
	return session + "/" + pane
}

func CorrelateAgentWorkflow(project *Project, work *WorkItem, herdr *HerdrSnapshot) *AgentWorkflow {
	if work == nil || herdr == nil {
		return nil
	}

	primaryURL := ""
	if work.GitHub != nil && work.GitHub.URL != "" {
		primaryURL = work.GitHub.URL
	} else {
		for _, link := range work.Links {
			if link.URL != "" {
				primaryURL = link.URL
				break
			}
		}
	}

	candidateURLs := map[string]string{}
	if primaryURL != "" {
		candidateURLs[primaryURL] = "issue"
	}
	if work.GitHub != nil {
		for _, issueURL := range work.GitHub.RelatedIssueURLs {
			if issueURL != "" {
				candidateURLs[issueURL] = "related_issue"
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
	matchType := ""
	for _, conn := range herdr.Connections {
		if kind, ok := candidateURLs[conn.IssueURL]; ok {
			matched = append(matched, conn)
			if matchType == "" || kind == "issue" {
				matchType = kind
			}
		}
	}
	if len(matched) == 0 && len(projectURLs) > 0 {
		for _, conn := range herdr.Connections {
			if _, ok := projectURLs[conn.ProjectURL]; ok {
				matched = append(matched, conn)
			}
		}
		if len(matched) > 0 {
			matchType = "project"
		}
	}
	if len(matched) == 0 && repo != "" {
		for _, conn := range herdr.Connections {
			if conn.IssueURL == "" && conn.ProjectURL == "" && conn.Role == "coordinator" && conn.Repository == repo {
				matched = append(matched, conn)
			}
		}
		if len(matched) > 0 {
			matchType = "coordinator"
		}
	}
	if len(matched) == 0 {
		return nil
	}

	notices := []string{}
	sessions := make(map[string]struct{})
	agentNames := make(map[string]int)
	for _, conn := range matched {
		sessions[conn.Session] = struct{}{}
		if name := strings.TrimSpace(conn.AgentName); name != "" {
			agentNames[name]++
		}
	}
	ambiguous := len(sessions) > 1
	if ambiguous {
		notices = append(notices, "복수 세션이 동일 작업에 바인딩되어 관찰이 모호합니다.")
	}

	agents := make([]AgentObservation, 0, len(matched))
	for _, conn := range matched {
		agents = append(agents, AgentObservation{
			AgentID:      herdrAgentIdentity(conn, agentNames[strings.TrimSpace(conn.AgentName)] > 1),
			Role:         conn.Role,
			Runtime:      "herdr",
			ExecutionRef: herdrExecutionRef(conn),
			Execution:    NormalizeExecutionState(conn.AgentStatus),
			Provenance: ObservationProvenance{
				Source:     "herdr",
				ObservedAt: conn.ObservedAt,
				Scope:      conn.Session,
			},
		})
	}

	overallExecution := ExecutionUnknown
	hasRunning, hasWaiting, hasCompleted, hasFailed := false, false, false, false
	for _, agent := range agents {
		switch agent.Execution {
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
	switch {
	case hasRunning:
		overallExecution = ExecutionRunning
	case hasWaiting:
		overallExecution = ExecutionWaiting
	case hasFailed:
		overallExecution = ExecutionFailed
	case hasCompleted:
		overallExecution = ExecutionCompleted
	case len(agents) > 0:
		overallExecution = agents[0].Execution
	}

	ghWorkState := work.State
	if work.GitHub != nil && work.GitHub.State != "" {
		ghWorkState = work.GitHub.State
	}
	candidateSHA := ""
	if work.GitHub != nil && work.GitHub.Fields != nil {
		candidateSHA = firstNonEmpty(work.GitHub.Fields["headSha"], work.GitHub.Fields["sha"])
	}

	vState := VerificationState{Status: VerificationUnverified, AnchorSHA: candidateSHA}
	if candidateSHA == "" {
		vState.Codes = append(vState.Codes, "awaiting_pr")
		vState.Summary = "PR 생성 대기 중 (로컬 작업 중)"
	} else {
		ciPending, ciFailed := false, false
		if work.GitHub != nil {
			for _, check := range work.GitHub.Checks {
				conclusion := strings.ToLower(check.Conclusion)
				status := strings.ToLower(check.Status)
				if conclusion == "failure" || conclusion == "timed_out" || conclusion == "action_required" {
					ciFailed = true
				} else if status == "in_progress" || status == "queued" || status == "pending" {
					ciPending = true
				}
			}
		}
		if ciFailed {
			vState.Codes = append(vState.Codes, "ci_failed")
		} else if ciPending {
			vState.Codes = append(vState.Codes, "ci_pending")
		}
		vState.Codes = append(vState.Codes, "reviewer_missing")
		vState.Summary = "리뷰어 검증 증거 대기 중"
	}
	if ambiguous || matchType == "coordinator" {
		vState.Status = VerificationPartial
	}
	vState = NormalizeVerificationState(vState)

	observedAt := herdr.ObservedAt
	return &AgentWorkflow{
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
			Partial:    ambiguous || matchType == "coordinator",
			Fallback:   matchType == "coordinator",
			Notice:     firstNotice(notices),
		},
	}
}

func firstNotice(notices []string) string {
	if len(notices) == 0 {
		return ""
	}
	return notices[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func enrichSnapshotWithAgentWorkflow(snapshot *Snapshot, herdr *HerdrSnapshot) {
	if snapshot == nil || herdr == nil {
		return
	}
	for projectIndex := range snapshot.Projects {
		for workIndex := range snapshot.Projects[projectIndex].WorkItems {
			snapshot.Projects[projectIndex].WorkItems[workIndex].AgentWorkflow = CorrelateAgentWorkflow(&snapshot.Projects[projectIndex], &snapshot.Projects[projectIndex].WorkItems[workIndex], herdr)
		}
	}
}
