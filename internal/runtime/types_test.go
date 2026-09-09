package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"thread-dock/internal/contract/v2"
)

func TestDecodeBuilderResultRejectsNonCanonicalResults(t *testing.T) {
	validSHA := "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name string
		data string
	}{
		{"empty commit", `{"commitSha":"","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}`},
		{"uppercase commit", `{"commitSha":"0123456789ABCDEF0123456789ABCDEF01234567","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}`},
		{"empty verification", `{"commitSha":"` + validSHA + `","verification":[]}`},
		{"unknown field", `{"commitSha":"` + validSHA + `","verification":[{"command":"go test","outcome":"passed","duration":"1s","extra":true}]}`},
		{"trailing JSON", `{"commitSha":"` + validSHA + `","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]} {}`},
		{"invalid outcome", `{"commitSha":"` + validSHA + `","verification":[{"command":"go test","outcome":"ok","duration":"1s"}]}`},
		{"invalid duration", `{"commitSha":"` + validSHA + `","verification":[{"command":"go test","outcome":"passed","duration":"soon"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeBuilderResult([]byte(tc.data)); err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
		})
	}
}

func TestDecodeBuilderResultAcceptsStrictResult(t *testing.T) {
	got, err := DecodeBuilderResult([]byte(`{"commitSha":"0123456789abcdef0123456789abcdef01234567","verification":[{"command":"go test","outcome":"passed","duration":"1s"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.CommitSHA == "" || len(got.Verification) != 1 {
		t.Fatalf("result=%#v", got)
	}
}

func TestDecodeReviewerResultStrictContract(t *testing.T) {
	validSHA := "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"valid accept", `{"reviewedSha":"` + validSHA + `","decision":"accept","blockingFindings":[]}`, false},
		{"valid block", `{"reviewedSha":"` + validSHA + `","decision":"block","blockingFindings":[{"code":"unsafe","diagnostic":"unsafe change"}]}`, false},
		{"missing findings", `{"reviewedSha":"` + validSHA + `","decision":"accept"}`, true},
		{"nil findings", `{"reviewedSha":"` + validSHA + `","decision":"accept","blockingFindings":null}`, true},
		{"unknown field", `{"reviewedSha":"` + validSHA + `","decision":"accept","blockingFindings":[],"extra":true}`, true},
		{"trailing JSON", `{"reviewedSha":"` + validSHA + `","decision":"accept","blockingFindings":[]} {}`, true},
		{"invalid SHA", `{"reviewedSha":"0123456789ABCDEF0123456789ABCDEF01234567","decision":"accept","blockingFindings":[]}`, true},
		{"invalid decision", `{"reviewedSha":"` + validSHA + `","decision":"maybe","blockingFindings":[]}`, true},
		{"accept with findings", `{"reviewedSha":"` + validSHA + `","decision":"accept","blockingFindings":[{"code":"x","diagnostic":"y"}]}`, true},
		{"block without findings", `{"reviewedSha":"` + validSHA + `","decision":"block","blockingFindings":[]}`, true},
		{"empty finding fields", `{"reviewedSha":"` + validSHA + `","decision":"block","blockingFindings":[{"code":"","diagnostic":"x"}]}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeReviewerResult([]byte(tc.data))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

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
