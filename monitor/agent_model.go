package main

import (
	"fmt"
	"regexp"
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
	Codes     []string           `json:"codes,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	AnchorSHA string             `json:"anchorSha,omitempty"`
}

type ObservationProvenance struct {
	Source     string     `json:"source,omitempty"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
	CapturedAt *time.Time `json:"capturedAt,omitempty"`
	Scope      string     `json:"scope,omitempty"`
	Stale      bool       `json:"stale,omitempty"`
	Partial    bool       `json:"partial,omitempty"`
	Fallback   bool       `json:"fallback,omitempty"`
	Notice     string     `json:"notice,omitempty"`
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
	Role       string     `json:"role,omitempty"`
	Author     string     `json:"author,omitempty"`
}

func NormalizeExecutionState(raw string) ExecutionState {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "queued":
		return ExecutionQueued
	case "running", "working":
		return ExecutionRunning
	case "waiting", "blocked":
		return ExecutionWaiting
	case "idle":
		// Herdr idle only says the pane is at rest; it is neither progress nor
		// completion, so it must not block (or confirm) verification.
		return ExecutionUnknown
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

func shaMatches(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	const minSHAPrefix = 7
	if len(a) >= minSHAPrefix && len(b) >= minSHAPrefix {
		return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
	}
	return false
}

func EvidenceMatchesCandidate(state VerificationState, evidence EvidenceItem) bool {
	return shaMatches(state.AnchorSHA, evidence.Revision)
}

var (
	evidenceMarkerRegex = regexp.MustCompile(`(?i)<!--\s*evidence:\s*(.*?)\s*-->`)
	markerFieldRegex    = regexp.MustCompile(`([a-zA-Z0-9_-]+)=([^\s]+)`)
)

type ReviewerMarker struct {
	Reviewer string `json:"reviewer"`
	SHA      string `json:"sha"`
	Verdict  string `json:"verdict"`
	Raw      string `json:"raw"`
}

func ParseEvidenceMarkers(text string) []ReviewerMarker {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	matches := evidenceMarkerRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	markers := make([]ReviewerMarker, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		fieldMatches := markerFieldRegex.FindAllStringSubmatch(match[1], -1)
		fields := make(map[string]string, len(fieldMatches))
		for _, fm := range fieldMatches {
			if len(fm) >= 3 {
				fields[strings.ToLower(fm[1])] = fm[2]
			}
		}
		reviewer := fields["reviewer"]
		sha := fields["sha"]
		verdict := strings.ToUpper(fields["verdict"])
		if reviewer == "" || sha == "" || (verdict != "ACCEPT" && verdict != "REJECT") {
			continue
		}
		markers = append(markers, ReviewerMarker{
			Reviewer: reviewer,
			SHA:      sha,
			Verdict:  verdict,
			Raw:      match[0],
		})
	}
	return markers
}

func resolveReviewerIdentities(project *Project) map[string][]string {
	if project == nil || len(project.ReviewerIdentities) == 0 {
		return nil
	}
	return project.ReviewerIdentities
}

func CorrelateReviewerRole(role string, author string, identityMap map[string][]string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	author = strings.ToLower(strings.TrimSpace(author))
	if role == "" || author == "" || len(identityMap) == 0 {
		return false
	}
	allowed, ok := identityMap[role]
	if !ok || len(allowed) == 0 {
		return false
	}
	for _, id := range allowed {
		if strings.ToLower(strings.TrimSpace(id)) == author {
			return true
		}
	}
	return false
}

func ExtractEvidenceItems(work *WorkItem, identityMap map[string][]string) []EvidenceItem {
	if work == nil {
		return nil
	}
	evidences := make([]EvidenceItem, 0)

	if work.GitHub != nil {
		for _, check := range work.GitHub.Checks {
			claim := strings.ToLower(check.Conclusion)
			if claim == "" {
				claim = strings.ToLower(check.Status)
			}
			ref := check.Name
			if check.URL != "" {
				ref = check.URL
			}
			evidences = append(evidences, EvidenceItem{
				Type:       "ci",
				Source:     "github:checks",
				ObservedAt: work.GitHub.ObservedAt,
				Reference:  ref,
				Revision:   check.HeadSHA,
				Claim:      claim,
			})
		}
	}

	if work.GitHub != nil {
		for cIdx, comment := range work.GitHub.Comments {
			markers := ParseEvidenceMarkers(comment.Body)
			for mIdx, marker := range markers {
				if CorrelateReviewerRole(marker.Reviewer, comment.Author, identityMap) {
					ref := comment.URL
					if ref == "" {
						ref = fmt.Sprintf("comment:%d", cIdx)
					}
					if len(markers) > 1 {
						ref = fmt.Sprintf("%s:%d", ref, mIdx)
					}
					evidences = append(evidences, EvidenceItem{
						Type:       "review",
						Source:     "github:comment",
						ObservedAt: comment.ObservedAt,
						Reference:  ref,
						Revision:   marker.SHA,
						Claim:      marker.Verdict,
						Role:       strings.ToLower(strings.TrimSpace(marker.Reviewer)),
						Author:     strings.ToLower(strings.TrimSpace(comment.Author)),
					})
				}
			}
		}
	}

	return evidences
}

func DeriveVerificationState(candidateSHA string, evidences []EvidenceItem, checks []GitHubCheck, requiredRoles []string, ambiguous bool, coordinatorFallback bool, childIncomplete bool) VerificationState {
	candidateSHA = strings.TrimSpace(candidateSHA)
	vState := VerificationState{
		Status:    VerificationUnverified,
		AnchorSHA: candidateSHA,
	}

	if candidateSHA == "" {
		vState.Codes = []string{"awaiting_pr"}
		vState.Summary = "PR 생성 대기 중 (로컬 작업 중)"
		return NormalizeVerificationState(vState)
	}

	ciFailed := false
	ciPending := false
	ciPassCount := 0
	for _, check := range checks {
		if check.HeadSHA == "" || candidateSHA == "" || !shaMatches(candidateSHA, check.HeadSHA) {
			continue
		}
		conclusion := strings.ToLower(check.Conclusion)
		status := strings.ToLower(check.Status)
		if conclusion == "failure" || conclusion == "timed_out" || conclusion == "action_required" || conclusion == "cancelled" || conclusion == "stale" || conclusion == "startup_failure" {
			ciFailed = true
		} else if status == "in_progress" || status == "queued" || status == "pending" || status == "waiting" || status == "requested" {
			ciPending = true
		} else if conclusion == "success" || conclusion == "neutral" {
			ciPassCount++
		}
	}
	if ciFailed {
		vState.Codes = append(vState.Codes, "ci_failed")
	} else if ciPending {
		vState.Codes = append(vState.Codes, "ci_pending")
	} else if ciPassCount > 0 {
		vState.Codes = append(vState.Codes, "ci_passed")
	}

	type reviewerVerdict struct {
		claim      string
		observedAt *time.Time
		index      int
	}
	latestVerdicts := make(map[string]reviewerVerdict)
	hasStaleReview := false

	dummyAnchorState := VerificationState{AnchorSHA: candidateSHA}
	for idx, ev := range evidences {
		if ev.Type != "review" {
			continue
		}
		if EvidenceMatchesCandidate(dummyAnchorState, ev) {
			if ev.Claim != "ACCEPT" && ev.Claim != "REJECT" {
				continue
			}
			key := ev.Role
			if key == "" {
				key = ev.Author
			}
			if key == "" {
				key = ev.Reference
			}

			prev, exists := latestVerdicts[key]
			if !exists {
				latestVerdicts[key] = reviewerVerdict{claim: ev.Claim, observedAt: ev.ObservedAt, index: idx}
			} else {
				if prev.observedAt != nil && ev.ObservedAt != nil {
					if ev.ObservedAt.After(*prev.observedAt) || ev.ObservedAt.Equal(*prev.observedAt) {
						latestVerdicts[key] = reviewerVerdict{claim: ev.Claim, observedAt: ev.ObservedAt, index: idx}
					}
				} else {
					latestVerdicts[key] = reviewerVerdict{claim: ev.Claim, observedAt: ev.ObservedAt, index: idx}
				}
			}
		} else {
			if ev.Claim == "ACCEPT" || ev.Claim == "REJECT" {
				hasStaleReview = true
			}
		}
	}

	hasMatchingReview := false
	hasAcceptedReview := false
	hasRejectedReview := false
	hasMissingRoleReview := false

	if len(requiredRoles) > 0 {
		allRolesAccepted := true
		anyRoleRejected := false
		anyRoleReviewed := false

		for _, role := range requiredRoles {
			normRole := strings.ToLower(strings.TrimSpace(role))
			v, exists := latestVerdicts[normRole]
			if !exists {
				allRolesAccepted = false
				hasMissingRoleReview = true
			} else {
				anyRoleReviewed = true
				if v.claim == "REJECT" {
					anyRoleRejected = true
					allRolesAccepted = false
				} else if v.claim != "ACCEPT" {
					allRolesAccepted = false
				}
			}
		}

		if anyRoleReviewed {
			hasMatchingReview = true
		}
		if anyRoleRejected {
			hasRejectedReview = true
		} else if allRolesAccepted {
			hasAcceptedReview = true
		}
	}

	if hasMatchingReview {
		if hasAcceptedReview && !hasRejectedReview {
			vState.Codes = append(vState.Codes, "reviewer_accepted")
		} else if hasRejectedReview {
			vState.Codes = append(vState.Codes, "reviewer_not_accepted")
		} else if hasMissingRoleReview {
			vState.Codes = append(vState.Codes, "reviewer_missing")
		}
	} else if hasStaleReview {
		vState.Codes = append(vState.Codes, "stale_sha")
	} else {
		vState.Codes = append(vState.Codes, "reviewer_missing")
	}

	if hasAcceptedReview && !hasRejectedReview && !ciFailed && !ciPending {
		// Provenance caveats may only downgrade an otherwise verified state.
		switch {
		case ambiguous:
			vState.Status = VerificationPartial
			vState.Summary = "복수 세션 바인딩으로 검증 상태 부분 확인"
		case coordinatorFallback:
			vState.Status = VerificationPartial
			vState.Summary = "저장소 코디네이터 폴백으로 검증 상태 부분 확인"
		case childIncomplete:
			vState.Status = VerificationPartial
			vState.Summary = "하위 에이전트 작업 미완료로 부분 확인"
		default:
			vState.Status = VerificationVerified
			if ciPassCount > 0 {
				vState.Summary = "검증 완료 (리뷰어 승인 및 CI 통과)"
			} else {
				vState.Summary = "검증 완료 (리뷰어 승인)"
			}
		}
	} else {
		vState.Status = VerificationUnverified
		hasStale := false
		hasMissing := false
		hasNotAccepted := false
		for _, c := range vState.Codes {
			switch c {
			case "stale_sha":
				hasStale = true
			case "reviewer_missing":
				hasMissing = true
			case "reviewer_not_accepted":
				hasNotAccepted = true
			}
		}
		if hasStale {
			vState.Summary = "최신 커밋에 대한 리뷰어 증거 대기 중 (이전 커밋 리뷰됨)"
		} else if ciFailed {
			vState.Summary = "CI 검증 실패"
		} else if ciPending {
			vState.Summary = "CI 검증 진행 중"
		} else if hasNotAccepted {
			vState.Summary = "리뷰어 미승인 (REJECT)"
		} else if hasMissing {
			vState.Summary = "리뷰어 승인 대기 중 (검증 미완료)"
		} else {
			vState.Summary = "검증 대기 중"
		}
	}

	return NormalizeVerificationState(vState)
}

type AgentWorkflow struct {
	Agents            []AgentObservation    `json:"agents"`
	Tasks             []TaskObservation     `json:"tasks"`
	Results           []TaskResult          `json:"results"`
	ExecutionState    ExecutionState        `json:"executionState"`
	GitHubWorkState   string                `json:"githubWorkState,omitempty"`
	VerificationState VerificationState     `json:"verificationState"`
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
			// Connections bound to a specific issue belong to that issue only.
			if _, ok := projectURLs[conn.ProjectURL]; ok && conn.IssueURL == "" {
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

	agentIDByName := make(map[string]string)
	for _, conn := range matched {
		name := strings.TrimSpace(conn.AgentName)
		if name != "" {
			agentIDByName[name] = herdrAgentIdentity(conn, agentNames[name] > 1)
		}
	}

	agents := make([]AgentObservation, 0, len(matched))
	for _, conn := range matched {
		agentID := herdrAgentIdentity(conn, agentNames[strings.TrimSpace(conn.AgentName)] > 1)
		parentID := strings.TrimSpace(conn.ParentAgentID)
		if parentID == "" && strings.TrimSpace(conn.ParentAgentName) != "" {
			parentID = agentIDByName[strings.TrimSpace(conn.ParentAgentName)]
		}
		agents = append(agents, AgentObservation{
			AgentID:       agentID,
			ParentAgentID: parentID,
			Role:          conn.Role,
			Runtime:       "herdr",
			ExecutionRef:  herdrExecutionRef(conn),
			Execution:     NormalizeExecutionState(conn.AgentStatus),
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
		candidateSHA = firstTrimmed(work.GitHub.Fields["headSha"], work.GitHub.Fields["sha"])
	}

	hasChildIncomplete := false
	for _, agent := range agents {
		if agent.ParentAgentID != "" {
			if agent.Execution == ExecutionFailed || agent.Execution == ExecutionCancelled || agent.Execution == ExecutionWaiting || agent.Execution == ExecutionRunning || agent.Execution == ExecutionQueued {
				hasChildIncomplete = true
			}
		}
	}

	identities := resolveReviewerIdentities(project)
	var requiredRoles []string
	if project != nil && len(project.ReviewerIdentities) > 0 {
		requiredRoles = make([]string, 0, len(project.ReviewerIdentities))
		for r := range project.ReviewerIdentities {
			requiredRoles = append(requiredRoles, strings.ToLower(strings.TrimSpace(r)))
		}
		sort.Strings(requiredRoles)
	}

	evidences := ExtractEvidenceItems(work, identities)
	checks := []GitHubCheck{}
	if work.GitHub != nil {
		checks = work.GitHub.Checks
	}

	vState := DeriveVerificationState(candidateSHA, evidences, checks, requiredRoles, ambiguous, matchType == "coordinator", hasChildIncomplete)

	evidenceRefs := append([]string{}, work.EvidenceRefs...)
	for _, ev := range evidences {
		if ev.Reference != "" {
			evidenceRefs = append(evidenceRefs, ev.Reference)
		}
	}
	evidenceRefs = uniqueStrings(evidenceRefs)

	tasks := make([]TaskObservation, 0, len(agents))
	results := make([]TaskResult, 0, len(agents))
	for _, agent := range agents {
		taskID := firstTrimmed(agent.TaskID, work.WorkID+"/"+agent.AgentID)
		tasks = append(tasks, TaskObservation{
			TaskID:        taskID,
			AgentID:       agent.AgentID,
			Role:          agent.Role,
			Objective:     work.Title,
			InputRevision: candidateSHA,
		})

		if agent.Execution == ExecutionCompleted || agent.Execution == ExecutionFailed || agent.Execution == ExecutionCancelled {
			summary := "작업 실행 완료"
			if agent.Execution == ExecutionFailed {
				summary = "작업 실행 실패"
			} else if agent.Execution == ExecutionCancelled {
				summary = "작업 실행 취소"
			}
			results = append(results, TaskResult{
				ResultID:       agent.AgentID + ":result",
				TaskID:         taskID,
				AgentID:        agent.AgentID,
				Status:         agent.Execution,
				OutputRevision: candidateSHA,
				Summary:        summary,
				Verification:   vState.Status,
			})
		}
	}

	observedAt := herdr.ObservedAt
	return &AgentWorkflow{
		Agents:            agents,
		Tasks:             tasks,
		Results:           results,
		ExecutionState:    overallExecution,
		GitHubWorkState:   ghWorkState,
		VerificationState: vState,
		CandidateRevision: candidateSHA,
		EvidenceRefs:      evidenceRefs,
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

func firstTrimmed(values ...string) string {
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
