package contractv2

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestReadRejectsUnknownAndTrailingJSON(t *testing.T) {
	valid := `{"version":2,"workId":"work-1","projectId":"project-1","revision":1,"request":"ship it","acceptanceCriteria":["works"],"repositoryPlans":[{"repoKey":"app","baseSha":"0123456789012345678901234567890123456789","targetBranch":"main"}],"tasks":[{"taskId":"task-1","repoKey":"app","allowedPaths":["internal"],"acceptanceCriteria":["works"]}],"issueDrafts":[],"interfaceAgreements":[],"crossRepoVerification":[],"documentation":{"required":false,"reason":"not required"},"executionProfiles":{"builder":"builder","reviewer":"reviewer","documenter":"documenter"},"decisionRefs":[]}`
	tests := []struct {
		name string
		data string
	}{
		{"unknown field", strings.TrimSuffix(valid, "}") + `,"localPath":"/tmp/x"}`},
		{"trailing value", valid + ` {}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Read(strings.NewReader(tc.data)); err == nil {
				t.Fatalf("Read(%s) accepted invalid JSON", tc.name)
			}
		})
	}
}

func TestValidSingleRepositoryFixtureRoundTripsStrictly(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/contracts/v2/valid-single-repo.json")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Write(&out, contract); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(got, want) {
		t.Fatalf("round trip changed fixture: got %s", out.Bytes())
	}
}

func TestValidSingleRepositoryFixtureDoesNotLeakRuntimeOrGeneratedIdentity(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/contracts/v2/valid-single-repo.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"localPath", "model", "provider", "token", "issueNumber", "processId", "sessionId"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("fixture leaked forbidden field %q", forbidden)
		}
	}
	if _, err := Read(bytes.NewReader(data)); err != nil {
		t.Fatalf("fixture is not a valid contract: %v", err)
	}
}

func jsonEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}
