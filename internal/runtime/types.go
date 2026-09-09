// Package runtime defines the narrow provider-neutral invocation boundary.
package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
)

type Role string

const (
	RoleBuilder    Role = "builder"
	RoleReviewer   Role = "reviewer"
	RoleDocumenter Role = "documenter"
)

type Invocation struct {
	RequestID    contractv2.RequestID `json:"requestId"`
	Role         Role                 `json:"role"`
	ProfileID    string               `json:"profileId"`
	Worktree     string               `json:"worktree"`
	OutputSchema string               `json:"outputSchema"`
	ReadOnly     bool                 `json:"readOnly"`
	Packet       json.RawMessage      `json:"packet"`
}

type ArtifactEnvelope struct {
	RequestID contractv2.RequestID `json:"requestId"`
	Role      Role                 `json:"role"`
	Status    string               `json:"status"`
	Result    json.RawMessage      `json:"result"`
}

const BuilderOutputSchema = "thread-dock.builder-result.v1"

type VerificationResult struct {
	Command  string `json:"command"`
	Outcome  string `json:"outcome"`
	Duration string `json:"duration"`
}

type BuilderResult struct {
	CommitSHA    string               `json:"commitSha"`
	Verification []VerificationResult `json:"verification"`
}

type BuilderPacket struct {
	TaskID             contractv2.TaskID        `json:"taskId"`
	AllowedPaths       []string                 `json:"allowedPaths"`
	AcceptanceCriteria []string                 `json:"acceptanceCriteria"`
	Verification       []contractv2.CommandSpec `json:"verification"`
}

// DecodeBuilderResult strictly decodes the bounded result returned by a
// Builder. Unknown fields and trailing JSON are rejected at this boundary.
func DecodeBuilderResult(data []byte) (BuilderResult, error) {
	var result BuilderResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return BuilderResult{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return BuilderResult{}, fmt.Errorf("trailing JSON is not allowed")
		}
		return BuilderResult{}, err
	}
	if err := ValidateBuilderResult(result); err != nil {
		return BuilderResult{}, err
	}
	return result, nil
}

func ValidateBuilderResult(result BuilderResult) error {
	if !validSHA(result.CommitSHA) {
		return fmt.Errorf("commitSha must be a lowercase 40-character SHA")
	}
	if len(result.Verification) == 0 {
		return fmt.Errorf("verification is required")
	}
	for _, verification := range result.Verification {
		if strings.TrimSpace(verification.Command) != verification.Command || verification.Command == "" {
			return fmt.Errorf("verification command is required")
		}
		if verification.Outcome != "passed" && verification.Outcome != "failed" {
			return fmt.Errorf("verification outcome is invalid")
		}
		if strings.TrimSpace(verification.Duration) != verification.Duration || verification.Duration == "" {
			return fmt.Errorf("verification duration is required")
		}
		if _, err := time.ParseDuration(verification.Duration); err != nil {
			return fmt.Errorf("verification duration is invalid: %w", err)
		}
	}
	return nil
}

func validSHA(value string) bool {
	if len(value) != 40 || value != strings.ToLower(value) {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

type AgentRuntime interface {
	Invoke(context.Context, Invocation) (ArtifactEnvelope, error)
}
