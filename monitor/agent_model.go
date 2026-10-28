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

func normalizeExecutionState(raw string) ExecutionState {
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

func evidenceMatchesCandidate(state VerificationState, evidence EvidenceItem) bool {
	anchor := strings.TrimSpace(state.AnchorSHA)
	revision := strings.TrimSpace(evidence.Revision)
	return anchor != "" && revision != "" && anchor == revision
}
