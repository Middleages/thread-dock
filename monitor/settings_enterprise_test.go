package main

import "testing"

func TestMonitorSettingsMapsGitHubEnterpriseHost(t *testing.T) {
	const host = "github.samsungds.net"
	env := map[string]string{
		"THREADDOCK_GITHUB_HOST":      host,
		"THREADDOCK_REPOS":            "FDYPhotoDX/app",
		"THREADDOCK_PROJECTS":         "https://" + host + "/orgs/FDYPhotoDX/projects/4",
		"THREADDOCK_WSL_DISTRIBUTION": "Ubuntu",
	}
	settings := settingsFromEnv(env)
	if settings.GitHubHost != host {
		t.Fatalf("GitHubHost=%q", settings.GitHubHost)
	}
	mapped := settings.environment(map[string]string{})
	if mapped["THREADDOCK_GITHUB_HOST"] != host {
		t.Fatalf("env=%#v", mapped)
	}
	if err := validateMonitorSettings(settings); err != nil {
		t.Fatalf("enterprise settings rejected: %v", err)
	}
}

func TestMonitorSettingsDefaultsGitHubHostToDotCom(t *testing.T) {
	settings := settingsFromEnv(map[string]string{})
	if settings.GitHubHost != "github.com" {
		t.Fatalf("GitHubHost=%q", settings.GitHubHost)
	}
}
