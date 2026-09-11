package main

import (
	"path/filepath"
	"testing"
)

func TestSettingsFromEnvMapsExistingMonitorConfiguration(t *testing.T) {
	env := map[string]string{
		"THREADDOCK_REPOS":            "Middleages/thread-dock,Middleages/jmj",
		"THREADDOCK_PROJECTS":         "https://github.com/users/Middleages/projects/1",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
		"THREADDOCK_SESSIONS_FILE":    "/home/appuser/.threaddock/sessions.json",
	}

	got := settingsFromEnv(env)
	if got.Repositories != env["THREADDOCK_REPOS"] || got.Projects != env["THREADDOCK_PROJECTS"] || got.WSLDistribution != "Ubuntu" || got.SessionsFile != env["THREADDOCK_SESSIONS_FILE"] {
		t.Fatalf("settings=%#v", got)
	}
}

func TestValidateMonitorSettingsRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name     string
		settings MonitorSettings
	}{
		{name: "bad repository", settings: MonitorSettings{Repositories: "not-a-repository", WSLDistribution: "Ubuntu"}},
		{name: "project view url", settings: MonitorSettings{Projects: "https://github.com/users/Middleages/projects/1/views/2", WSLDistribution: "Ubuntu"}},
		{name: "missing distribution", settings: MonitorSettings{Repositories: "Middleages/thread-dock"}},
		{name: "relative sessions file", settings: MonitorSettings{WSLDistribution: "Ubuntu", SessionsFile: ".threaddock/sessions.json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateMonitorSettings(test.settings); err == nil {
				t.Fatalf("settings=%#v should fail validation", test.settings)
			}
		})
	}
}

func TestSettingsStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ThreadDock", "config.json")
	store := settingsStore{path: path}
	want := MonitorSettings{
		Repositories:    "Middleages/thread-dock",
		Projects:        "https://github.com/users/Middleages/projects/1",
		WSLDistribution: "Ubuntu",
		SessionsFile:    "/home/appuser/.threaddock/sessions.json",
	}

	if err := store.save(want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != want {
		t.Fatalf("got=%#v ok=%v want=%#v", got, ok, want)
	}
}

func TestSettingsEnvironmentCanReplaceAndClearInheritedValues(t *testing.T) {
	base := map[string]string{
		"THREADDOCK_REPOS":            "Middleages/old",
		"THREADDOCK_PROJECTS":         "https://github.com/users/Middleages/projects/99",
		"THREADDOCK_WSL_DISTRIBUTION": "OldDistro",
		"THREADDOCK_SESSIONS_FILE":    "/tmp/old.json",
		"OTHER":                       "preserved",
	}
	settings := MonitorSettings{Repositories: "Middleages/thread-dock", WSLDistribution: "Ubuntu"}

	got := settings.environment(base)
	if got["THREADDOCK_REPOS"] != "Middleages/thread-dock" || got["THREADDOCK_WSL_DISTRIBUTION"] != "Ubuntu" || got["OTHER"] != "preserved" {
		t.Fatalf("env=%#v", got)
	}
	if _, exists := got["THREADDOCK_PROJECTS"]; exists {
		t.Fatalf("empty project setting should remove inherited value: %#v", got)
	}
	if _, exists := got["THREADDOCK_SESSIONS_FILE"]; exists {
		t.Fatalf("empty sessions setting should remove inherited value: %#v", got)
	}
}
