package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const maxArtifactBytes = 64 * 1024

func validRole(role Role) bool {
	return role == RoleBuilder || role == RoleReviewer || role == RoleDocumenter
}

// ValidateEnvelope checks correlation identity and the bounded, JSON result
// shape returned by a role runtime.
func ValidateEnvelope(invocation Invocation, envelope ArtifactEnvelope) error {
	if !validRole(invocation.Role) || !validRole(envelope.Role) {
		return fmt.Errorf("unknown role")
	}
	if invocation.RequestID == "" || envelope.RequestID != invocation.RequestID {
		return fmt.Errorf("requestId mismatch")
	}
	if envelope.Role != invocation.Role {
		return fmt.Errorf("role mismatch")
	}
	if len(envelope.Result) == 0 || len(envelope.Result) > maxArtifactBytes || !json.Valid(envelope.Result) {
		return fmt.Errorf("result must be bounded valid JSON")
	}
	if envelope.Status == "" {
		return fmt.Errorf("status must be non-empty")
	}
	if envelope.Status != "success" {
		var failure struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(envelope.Result, &failure); err != nil || bytes.Equal(bytes.TrimSpace(envelope.Result), []byte("null")) || failure.Error == "" {
			return fmt.Errorf("non-success result must contain an error")
		}
	}
	return nil
}
