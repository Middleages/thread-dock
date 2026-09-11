package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const settingsDirectoryName = "ThreadDock"
const settingsFileName = "config.json"

type MonitorSettings struct {
	GitHubHost      string `json:"githubHost"`
	Repositories    string `json:"repositories"`
	Projects        string `json:"projects"`
	WSLDistribution string `json:"wslDistribution"`
	SessionsFile    string `json:"sessionsFile"`
}

type settingsStore struct {
	path string
}

func defaultSettingsStore() settingsStore {
	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return settingsStore{}
	}
	return settingsStore{path: filepath.Join(dir, settingsDirectoryName, settingsFileName)}
}

func settingsFromEnv(env map[string]string) MonitorSettings {
	host, err := normalizeGitHubHost(env["THREADDOCK_GITHUB_HOST"])
	if err != nil {
		host = strings.TrimSpace(env["THREADDOCK_GITHUB_HOST"])
	}
	return MonitorSettings{
		GitHubHost:      host,
		Repositories:    strings.TrimSpace(env["THREADDOCK_REPOS"]),
		Projects:        strings.TrimSpace(env["THREADDOCK_PROJECTS"]),
		WSLDistribution: strings.TrimSpace(env["THREADDOCK_WSL_DISTRIBUTION"]),
		SessionsFile:    strings.TrimSpace(env["THREADDOCK_SESSIONS_FILE"]),
	}
}

func (s MonitorSettings) environment(base map[string]string) map[string]string {
	env := cloneStringMap(base)
	setOrDelete := func(key, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			delete(env, key)
			return
		}
		env[key] = value
	}
	setOrDelete("THREADDOCK_GITHUB_HOST", s.GitHubHost)
	setOrDelete("THREADDOCK_REPOS", s.Repositories)
	setOrDelete("THREADDOCK_PROJECTS", s.Projects)
	setOrDelete("THREADDOCK_WSL_DISTRIBUTION", s.WSLDistribution)
	setOrDelete("THREADDOCK_SESSIONS_FILE", s.SessionsFile)
	return env
}

func validateMonitorSettings(settings MonitorSettings) error {
	env := settings.environment(map[string]string{})
	normalized, _, err := normalizeGitHubEnvironment(env)
	if err != nil {
		return err
	}
	config, _ := parseMonitorConfig(normalized)
	if config.err != nil {
		return config.err
	}
	if settings.SessionsFile != "" && !strings.HasPrefix(strings.TrimSpace(settings.SessionsFile), "/") {
		return fmt.Errorf("Herdr 연결 파일은 WSL 내부 절대 경로여야 합니다")
	}
	if strings.TrimSpace(settings.SessionsFile) != "" && strings.TrimSpace(settings.WSLDistribution) == "" {
		return fmt.Errorf("Herdr 연결 파일을 사용하려면 WSL 배포판을 지정하세요")
	}
	return nil
}

func (s settingsStore) load() (MonitorSettings, bool, error) {
	if strings.TrimSpace(s.path) == "" {
		return MonitorSettings{}, false, nil
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return MonitorSettings{}, false, nil
	}
	if err != nil {
		return MonitorSettings{}, false, err
	}
	var settings MonitorSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return MonitorSettings{}, false, fmt.Errorf("ThreadDock 설정 파일을 읽을 수 없습니다: %w", err)
	}
	if settings.GitHubHost == "" {
		settings.GitHubHost = defaultGitHubHost
	}
	if err := validateMonitorSettings(settings); err != nil {
		return MonitorSettings{}, false, fmt.Errorf("저장된 ThreadDock 설정이 올바르지 않습니다: %w", err)
	}
	return settings, true, nil
}

func (s settingsStore) save(settings MonitorSettings) error {
	if strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("사용자 설정 경로를 확인할 수 없습니다")
	}
	if settings.GitHubHost == "" {
		settings.GitHubHost = defaultGitHubHost
	}
	if err := validateMonitorSettings(settings); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("설정 폴더를 만들 수 없습니다: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("설정을 직렬화할 수 없습니다: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("설정 파일을 저장할 수 없습니다: %w", err)
	}
	return nil
}
