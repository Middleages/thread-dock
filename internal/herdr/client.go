// Package herdr adapts the Herdr v0.8.2 CLI to ThreadDock's runtime boundary.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
)

var ErrClosedWorkspace = errors.New("Herdr workspace is closed and must be reopened")
var ErrAgentNotFound = errors.New("Herdr agent was not found")

type Client interface {
	CreateWorktree(context.Context, CreateWorktreeRequest) (Worktree, error)
	StartAgent(context.Context, StartAgentRequest) error
	Prompt(context.Context, string, string) error
	Get(context.Context, string) (AgentState, error)
	ReadRecent(context.Context, string) (string, error)
}

type SessionResumer interface {
	ResumeAgent(context.Context, ResumeAgentRequest) error
}

type CreateWorktreeRequest struct {
	Cwd    string
	Repo   string // Deprecated: use Cwd; retained for the plan's request shape.
	Branch string
	Base   string
	Label  string
}

type Worktree struct {
	WorkspaceID string
	PaneID      string
	Path        string
}

// OpenWorktreeRequest describes a previously-created Git worktree that Herdr
// should expose in a fresh workspace.
type OpenWorktreeRequest struct {
	Cwd   string
	Path  string
	Label string
}

type VerificationCheck struct {
	Command  string `json:"command"`
	Outcome  string `json:"outcome"`
	Duration string `json:"duration"`
}

// Evidence is the only structured Builder result accepted by the
// orchestrator. Terminal transcript text is deliberately not part of it.
type Evidence struct {
	RequestID    string              `json:"requestId"`
	CommitSHA    string              `json:"commitSha"`
	Verification []VerificationCheck `json:"verification"`
}

// ReviewFinding is one concrete issue that blocks acceptance of the
// integrated result.
type ReviewFinding struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Paths   []string `json:"paths"`
}

// ReviewEvidence is the only structured Reviewer result accepted by the
// runtime boundary. Decision is exactly "accept" or "block".
type ReviewEvidence struct {
	RequestID        string          `json:"requestId"`
	Decision         string          `json:"decision"`
	BlockingFindings []ReviewFinding `json:"blockingFindings"`
	RiskCategories   []string        `json:"riskCategories"`
}

// UnmarshalJSON accepts the array form emitted by the documented protocol and
// the singleton object form emitted by some live Herdr/OpenCode runs. Both
// forms are normalized to the same slice so downstream task matching keeps
// its exact count and command semantics.
func (e *Evidence) UnmarshalJSON(data []byte) error {
	var wire struct {
		RequestID    string          `json:"requestId"`
		CommitSHA    string          `json:"commitSha"`
		Verification json.RawMessage `json:"verification"`
	}
	if err := decodeEvidenceJSON(data, &wire); err != nil {
		return err
	}
	checks, err := decodeVerificationChecks(wire.Verification)
	if err != nil {
		return err
	}
	*e = Evidence{RequestID: wire.RequestID, CommitSHA: wire.CommitSHA, Verification: checks}
	return nil
}

func decodeVerificationChecks(data []byte) ([]VerificationCheck, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("verification is required")
	}

	switch data[0] {
	case '{':
		var check VerificationCheck
		if err := decodeEvidenceJSON(data, &check); err != nil {
			return nil, err
		}
		return []VerificationCheck{check}, nil
	case '[':
		var rawChecks []json.RawMessage
		if err := json.Unmarshal(data, &rawChecks); err != nil {
			return nil, err
		}
		if len(rawChecks) == 0 {
			return nil, errors.New("verification must contain at least one check")
		}
		checks := make([]VerificationCheck, 0, len(rawChecks))
		for _, rawCheck := range rawChecks {
			trimmed := bytes.TrimSpace(rawCheck)
			if len(trimmed) == 0 || trimmed[0] != '{' {
				return nil, errors.New("verification check must be an object")
			}
			var check VerificationCheck
			if err := decodeEvidenceJSON(rawCheck, &check); err != nil {
				return nil, err
			}
			checks = append(checks, check)
		}
		return checks, nil
	default:
		return nil, errors.New("verification must be an object or array")
	}
}

func decodeEvidenceJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
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

const EvidenceSchemaExample = `{"requestId":"<prompt request ID>","commitSha":"<40 lowercase hex>","verification":[{"command":"<required command>","outcome":"passed","duration":"<Go duration>"}]}`
const ReviewEvidenceSchemaExample = `{"requestId":"<prompt request ID>","decision":"accept","blockingFindings":[],"riskCategories":[]}`
const ReviewSchemaExample = ReviewEvidenceSchemaExample

const (
	THREADDOCK_EVIDENCE_BEGIN     = "THREADDOCK_EVIDENCE_BEGIN"
	THREADDOCK_EVIDENCE_END       = "THREADDOCK_EVIDENCE_END"
	THREADDOCK_REVIEW_BEGIN       = "THREADDOCK_REVIEW_BEGIN"
	THREADDOCK_REVIEW_END         = "THREADDOCK_REVIEW_END"
	EvidenceBeginMarker           = THREADDOCK_EVIDENCE_BEGIN
	EvidenceEndMarker             = THREADDOCK_EVIDENCE_END
	ReviewBeginMarker             = THREADDOCK_REVIEW_BEGIN
	ReviewEndMarker               = THREADDOCK_REVIEW_END
	ReviewEvidenceBeginMarker     = THREADDOCK_REVIEW_BEGIN
	ReviewEvidenceEndMarker       = THREADDOCK_REVIEW_END
	MaxEvidencePayloadBytes       = 16 * 1024
	MaxReviewEvidencePayloadBytes = MaxEvidencePayloadBytes
)

type AgentInfo struct {
	Name           string
	SessionID      string
	WorkspaceID    string
	PaneID         string
	Path           string
	State          AgentState
	StateChangeSeq int64
}

type StartAgentRequest struct {
	Name   string
	PaneID string
}

type ResumeAgentRequest struct {
	Name      string
	PaneID    string
	SessionID string
}

type AgentState string

const (
	AgentStateWorking AgentState = "working"
	AgentStateBlocked AgentState = "blocked"
	AgentStateIdle    AgentState = "idle"
	AgentStateDone    AgentState = "done"
	AgentStateUnknown AgentState = "unknown"
)

func ParseAgentState(value string) AgentState {
	switch AgentState(value) {
	case AgentStateWorking, AgentStateBlocked, AgentStateIdle, AgentStateDone:
		return AgentState(value)
	default:
		return AgentStateUnknown
	}
}
