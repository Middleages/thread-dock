package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func writeTestConfig(path string) error {
	data := []byte(`{"ghesHost":"https://github.example.test","projectId":"PVT_1","projectStatusFieldId":"PVTSSF_1","projectStatusOptions":{"Backlog":"opt-1","Ready":"opt-2","In Progress":"opt-3","Review":"opt-4","Done":"opt-5"}}`)
	return os.WriteFile(path, data, 0600)
}
