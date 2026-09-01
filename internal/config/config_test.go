package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDefaultsRetirementSettingsWithoutBreakingOldJSON(t *testing.T) {
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !got.AutoRetireCompletedSessions || !strings.HasSuffix(got.HerdrWorktreeRoot, string(filepath.Separator)+".herdr"+string(filepath.Separator)+"worktrees") {
		t.Fatalf("config=%#v", got)
	}
}

func TestParseOpenCodeAgents(t *testing.T) {
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","openCodeAgents":{"builder":"threaddock-builder","reviewer":"threaddock-reviewer"},"projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenCodeAgents.Builder != "threaddock-builder" || got.OpenCodeAgents.Reviewer != "threaddock-reviewer" {
		t.Fatalf("openCodeAgents=%#v", got.OpenCodeAgents)
	}
}

func TestParseOpenCodeAgentsAllowsOmittedObjectAndEmptyFields(t *testing.T) {
	base := `{"ghesHost":"https://github.example.test","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
	for _, value := range []string{"", `{}`, `{"builder":"","reviewer":""}`} {
		data := base
		if value != "" {
			data = strings.Replace(data, `,"projectId"`, `,"openCodeAgents":`+value+`,"projectId"`, 1)
		}
		got, err := Parse(strings.NewReader(data))
		if err != nil {
			t.Fatalf("openCodeAgents=%s: %v", value, err)
		}
		if got.OpenCodeAgents.Builder != "" || got.OpenCodeAgents.Reviewer != "" {
			t.Fatalf("openCodeAgents=%#v", got.OpenCodeAgents)
		}
	}
}

func TestParseRejectsInvalidOpenCodeAgentNamesWithFieldName(t *testing.T) {
	for _, tt := range []struct {
		field string
		value string
	}{
		{field: "builder", value: " reviewer"},
		{field: "reviewer", value: "리뷰어"},
		{field: "builder", value: "review/agent"},
		{field: "reviewer", value: "reviewer;touch"},
		{field: "builder", value: strings.Repeat("a", 65)},
	} {
		data := `{"ghesHost":"https://github.example.test","openCodeAgents":{"builder":"build","reviewer":"review"},"projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
		original := map[string]string{"builder": "build", "reviewer": "review"}[tt.field]
		data = strings.Replace(data, `"`+tt.field+`":"`+original+`"`, `"`+tt.field+`":"`+tt.value+`"`, 1)
		_, err := Parse(strings.NewReader(data))
		if err == nil || !strings.Contains(err.Error(), "openCodeAgents."+tt.field) {
			t.Fatalf("field=%s value=%q err=%v", tt.field, tt.value, err)
		}
	}
}

func TestParseRejectsUnknownOpenCodeAgentFields(t *testing.T) {
	data := `{"ghesHost":"https://github.example.test","openCodeAgents":{"builder":"build","reviewer":"review","model":"provider/model"},"projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
	if _, err := Parse(strings.NewReader(data)); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("err=%v", err)
	}
}

func TestParsePreservesExplicitFalseAutoRetirement(t *testing.T) {
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","autoRetireCompletedSessions":false,"herdrWorktreeRoot":"./managed-worktrees","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoRetireCompletedSessions {
		t.Fatal("explicit false was replaced by the default")
	}
	if !filepath.IsAbs(got.HerdrWorktreeRoot) || got.HerdrWorktreeRoot != filepath.Clean(got.HerdrWorktreeRoot) {
		t.Fatalf("worktree root is not canonical: %q", got.HerdrWorktreeRoot)
	}
}

func TestParseExpandsHomeOnlyTildeWithoutPanicking(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","herdrWorktreeRoot":"~","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.HerdrWorktreeRoot != filepath.Clean(home) {
		t.Fatalf("root=%q want %q", got.HerdrWorktreeRoot, filepath.Clean(home))
	}
}

func TestParseRejectsMissingGHESHost(t *testing.T) {
	_, err := Parse(strings.NewReader(`{"stateDir":"/tmp/thread-dock","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err == nil || !strings.Contains(err.Error(), "ghesHost") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseAppliesSafeDefaults(t *testing.T) {
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.APIBase != "https://github.example.test/api/v3" {
		t.Fatalf("apiBase=%q", got.APIBase)
	}
	if got.APIVersion != "2022-11-28" || got.HerdrBinary != "herdr" || got.GitBinary != "git" {
		t.Fatalf("defaults=%#v", got)
	}
	if got.WorkingWait != 60*time.Minute || got.RecoveryLimit != 3 {
		t.Fatalf("timing defaults=%#v", got)
	}
	if got.ProjectAutomationEnabled {
		t.Fatal("project automation must default to disabled")
	}
}

func TestParseProjectAutomationFlag(t *testing.T) {
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","projectAutomationEnabled":true,"projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !got.ProjectAutomationEnabled {
		t.Fatal("project automation flag was not parsed")
	}
}

func TestParseRejectsMissingProjectStatusOption(t *testing.T) {
	data := `{"ghesHost":"https://github.example.test","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4"}}`
	_, err := Parse(strings.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "Done") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseAcceptsDurationString(t *testing.T) {
	got, err := Parse(strings.NewReader(`{"ghesHost":"https://github.example.test","workingWait":"5m","recoveryLimit":4,"projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkingWait != 5*time.Minute || got.RecoveryLimit != 4 {
		t.Fatalf("config=%#v", got)
	}
}

func TestLoadReadsConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := writeTestConfig(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.GHESHost != "https://github.example.test" {
		t.Fatalf("host=%q", got.GHESHost)
	}
}

func TestParseRejectsHostCredentialsAndDisallowedParts(t *testing.T) {
	tests := []string{
		"github.example.test",
		"https:///missing-host",
		"https://user:secret@github.example.test",
		"https://github.example.test/api/v3",
		"https://github.example.test?token=secret",
		"https://github.example.test#fragment",
		"https://github.example.test///",
	}
	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			_, err := Parse(strings.NewReader(testConfigJSON(host, "https://github.example.test/api/v3")))
			if err == nil || !strings.Contains(err.Error(), "ghesHost") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestParseAcceptsHostRootAndAPIPath(t *testing.T) {
	got, err := Parse(strings.NewReader(testConfigJSON("https://github.example.test/", "https://github.example.test/api/v3")))
	if err != nil {
		t.Fatal(err)
	}
	if got.GHESHost != "https://github.example.test" || got.APIBase != "https://github.example.test/api/v3" {
		t.Fatalf("config=%#v", got)
	}
}

func TestParseRejectsAPIBaseCredentialsAndDisallowedParts(t *testing.T) {
	tests := []string{
		"github.example.test/api/v3",
		"https:///api/v3",
		"https://user:secret@github.example.test/api/v3",
		"https://github.example.test/api/v3?token=secret",
		"https://github.example.test/api/v3#fragment",
	}
	for _, apiBase := range tests {
		t.Run(apiBase, func(t *testing.T) {
			_, err := Parse(strings.NewReader(testConfigJSON("https://github.example.test", apiBase)))
			if err == nil || !strings.Contains(err.Error(), "apiBase") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestParseRejectsKnownGitHubPublicEndpointsOverHTTP(t *testing.T) {
	tests := []struct {
		name     string
		ghesHost string
		apiBase  string
		field    string
	}{
		{name: "public api base", ghesHost: "https://github.example.test", apiBase: "http://api.github.com", field: "apiBase"},
		{name: "public github host", ghesHost: "http://github.com", apiBase: "https://github.example.test/api/v3", field: "ghesHost"},
		{name: "uppercase public api base", ghesHost: "https://github.example.test", apiBase: "http://API.GITHUB.COM", field: "apiBase"},
		{name: "trailing dot public api base", ghesHost: "https://github.example.test", apiBase: "http://api.github.com.", field: "apiBase"},
		{name: "uppercase public github host", ghesHost: "http://GITHUB.COM", apiBase: "https://github.example.test/api/v3", field: "ghesHost"},
		{name: "trailing dot public github host", ghesHost: "http://github.com.", apiBase: "https://github.example.test/api/v3", field: "ghesHost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(testConfigJSON(tt.ghesHost, tt.apiBase)))
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("err=%v, want %s rejection", err, tt.field)
			}
		})
	}
}

func TestParseAcceptsGitHubPublicProfileOnlyWithHTTPSAPI(t *testing.T) {
	got, err := Parse(strings.NewReader(testConfigJSON("https://github.com/", "https://api.github.com/")))
	if err != nil {
		t.Fatal(err)
	}
	if got.GHESHost != "https://github.com" || got.APIBase != "https://api.github.com" {
		t.Fatalf("config=%#v", got)
	}
}

func TestParseRejectsNonCanonicalGitHubPublicProfileOverHTTPS(t *testing.T) {
	tests := []struct {
		name     string
		ghesHost string
		apiBase  string
	}{
		{name: "uppercase github host", ghesHost: "https://GITHUB.COM", apiBase: "https://api.github.com"},
		{name: "trailing dot github host", ghesHost: "https://github.com.", apiBase: "https://api.github.com"},
		{name: "uppercase public api", ghesHost: "https://github.com", apiBase: "https://API.GITHUB.COM"},
		{name: "trailing dot public api", ghesHost: "https://github.com", apiBase: "https://api.github.com."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(testConfigJSON(tt.ghesHost, tt.apiBase))); err == nil {
				t.Fatal("expected non-canonical GitHub.com profile rejection")
			}
		})
	}
}

func TestParseRejectsGitHubPublicProfileMismatch(t *testing.T) {
	tests := []struct {
		name     string
		ghesHost string
		apiBase  string
	}{
		{name: "github host with enterprise api", ghesHost: "https://github.com", apiBase: "https://github.example.test/api/v3"},
		{name: "enterprise host with github api", ghesHost: "https://github.example.test", apiBase: "https://api.github.com"},
		{name: "public api path", ghesHost: "https://github.com", apiBase: "https://api.github.com/api/v3"},
		{name: "public host port", ghesHost: "https://github.com:443", apiBase: "https://api.github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(testConfigJSON(tt.ghesHost, tt.apiBase))); err == nil {
				t.Fatal("expected GitHub.com profile rejection")
			}
		})
	}
}

func testConfigJSON(host, apiBase string) string {
	return `{"ghesHost":"` + host + `","apiBase":"` + apiBase + `","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`
}

func writeTestConfig(path string) error {
	data := []byte(`{"ghesHost":"https://github.example.test","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`)
	return os.WriteFile(path, data, 0600)
}
