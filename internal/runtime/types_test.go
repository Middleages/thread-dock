package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"thread-dock/internal/contract/v2"
)

func TestValidateEnvelopeRejectsIdentityAndMalformedResults(t *testing.T) {
	invocation := Invocation{RequestID: contractv2.RequestID("request-1"), Role: RoleBuilder}
	tests := []struct {
		name     string
		envelope ArtifactEnvelope
	}{
		{"request mismatch", ArtifactEnvelope{RequestID: "request-2", Role: RoleBuilder, Status: "success", Result: json.RawMessage(`{}`)}},
		{"role mismatch", ArtifactEnvelope{RequestID: "request-1", Role: RoleReviewer, Status: "success", Result: json.RawMessage(`{}`)}},
		{"unknown role", ArtifactEnvelope{RequestID: "request-1", Role: Role("scout"), Status: "success", Result: json.RawMessage(`{}`)}},
		{"invalid JSON result", ArtifactEnvelope{RequestID: "request-1", Role: RoleBuilder, Status: "success", Result: json.RawMessage(`{`)}},
		{"failed result without bounded error", ArtifactEnvelope{RequestID: "request-1", Role: RoleBuilder, Status: "failed", Result: json.RawMessage(`[]`)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateEnvelope(invocation, tc.envelope); err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
		})
	}
}

func TestAgentRuntimeInterfaceCompiles(t *testing.T) {
	var _ AgentRuntime = fakeRuntime{}
}

type fakeRuntime struct{}

func (fakeRuntime) Invoke(context.Context, Invocation) (ArtifactEnvelope, error) {
	return ArtifactEnvelope{}, nil
}
