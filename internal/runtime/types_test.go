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

func TestRuntimeJSONUsesLowerCamelCaseKeys(t *testing.T) {
	invocationData, err := json.Marshal(Invocation{RequestID: "request-1", Role: RoleBuilder, ProfileID: "profile", Worktree: "worktree", OutputSchema: "schema", ReadOnly: true, Packet: json.RawMessage(`{"taskId":"task-1"}`)})
	if err != nil {
		t.Fatal(err)
	}
	envelopeData, err := json.Marshal(ArtifactEnvelope{RequestID: "request-1", Role: RoleBuilder, Status: "success", Result: json.RawMessage(`{"ok":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{invocationData, envelopeData} {
		var wire map[string]any
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		for key := range wire {
			if key == "RequestID" || key == "ProfileID" || key == "OutputSchema" || key == "ReadOnly" || key == "Packet" || key == "Role" || key == "Status" || key == "Result" {
				t.Fatalf("upper camel key leaked: %q in %s", key, data)
			}
		}
	}
	var invocationWire map[string]any
	if err := json.Unmarshal(invocationData, &invocationWire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"requestId", "role", "profileId", "worktree", "outputSchema", "readOnly", "packet"} {
		if _, ok := invocationWire[key]; !ok {
			t.Fatalf("missing invocation key %q", key)
		}
	}
	var envelopeWire map[string]any
	if err := json.Unmarshal(envelopeData, &envelopeWire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"requestId", "role", "status", "result"} {
		if _, ok := envelopeWire[key]; !ok {
			t.Fatalf("missing envelope key %q", key)
		}
	}
}

type fakeRuntime struct{}

func (fakeRuntime) Invoke(context.Context, Invocation) (ArtifactEnvelope, error) {
	return ArtifactEnvelope{}, nil
}
