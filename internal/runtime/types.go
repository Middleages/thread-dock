// Package runtime defines the narrow provider-neutral invocation boundary.
package runtime

import (
	"context"
	"encoding/json"

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

type AgentRuntime interface {
	Invoke(context.Context, Invocation) (ArtifactEnvelope, error)
}
