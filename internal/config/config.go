// Package config loads the non-secret configuration used by ThreadDock.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAPIVersion    = "2022-11-28"
	defaultHerdrBinary   = "herdr"
	defaultGitBinary     = "git"
	defaultWorkingWait   = 60 * time.Minute
	defaultRecoveryLimit = 3
)

var requiredProjectStatuses = []string{"Backlog", "Ready", "In Progress", "Review", "Done"}

// Config contains local paths, GHES connection metadata and Project status
// option IDs. It deliberately contains no credentials.
type Config struct {
	GHESHost                 string            `json:"ghesHost"`
	APIBase                  string            `json:"apiBase"`
	APIVersion               string            `json:"apiVersion"`
	StateDir                 string            `json:"stateDir"`
	HerdrBinary              string            `json:"herdrBinary"`
	GitBinary                string            `json:"gitBinary"`
	WorkingWait              time.Duration     `json:"workingWait"`
	RecoveryLimit            int               `json:"recoveryLimit"`
	ProjectAutomationEnabled bool              `json:"projectAutomationEnabled"`
	ProjectID                string            `json:"projectId"`
	ProjectStatusFieldID     string            `json:"projectStatusFieldId"`
	ProjectStatusOptions     map[string]string `json:"projectStatusOptions"`
}

// Load reads and validates a JSON configuration file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return Parse(bytes.NewReader(data))
}

// Parse reads and validates a JSON configuration. Unknown fields and trailing
// JSON documents are rejected so an accidental typo cannot silently alter the
// runtime configuration.
func Parse(r io.Reader) (Config, error) {
	var raw configJSON
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, errors.New("parse config: trailing JSON document")
		}
		return Config{}, fmt.Errorf("parse config: trailing data: %w", err)
	}

	workingWait, err := decodeDuration(raw.WorkingWait)
	if err != nil {
		return Config{}, fmt.Errorf("workingWait: %w", err)
	}
	if workingWait == 0 {
		workingWait = defaultWorkingWait
	}
	recoveryLimit := raw.RecoveryLimit
	if recoveryLimit == 0 {
		recoveryLimit = defaultRecoveryLimit
	}
	stateDir := raw.StateDir
	if strings.TrimSpace(stateDir) == "" {
		stateDir = defaultStateDir()
	}
	apiVersion := raw.APIVersion
	if strings.TrimSpace(apiVersion) == "" {
		apiVersion = defaultAPIVersion
	}
	herdrBinary := raw.HerdrBinary
	if strings.TrimSpace(herdrBinary) == "" {
		herdrBinary = defaultHerdrBinary
	}
	gitBinary := raw.GitBinary
	if strings.TrimSpace(gitBinary) == "" {
		gitBinary = defaultGitBinary
	}
	apiBase := strings.TrimRight(strings.TrimSpace(raw.APIBase), "/")
	host := strings.TrimSpace(raw.GHESHost)
	if host != "" {
		if err := validateGHESHost(host); err != nil {
			return Config{}, err
		}
		// A single root slash is cosmetic; paths such as /api/v3 are not
		// valid GHES hosts and are rejected before this normalization.
		host = strings.TrimSuffix(host, "/")
	}
	if apiBase == "" && host != "" {
		apiBase = host + "/api/v3"
	}

	c := Config{
		GHESHost:                 host,
		APIBase:                  apiBase,
		APIVersion:               apiVersion,
		StateDir:                 stateDir,
		HerdrBinary:              herdrBinary,
		GitBinary:                gitBinary,
		WorkingWait:              workingWait,
		RecoveryLimit:            recoveryLimit,
		ProjectAutomationEnabled: raw.ProjectAutomationEnabled,
		ProjectID:                strings.TrimSpace(raw.ProjectID),
		ProjectStatusFieldID:     strings.TrimSpace(raw.ProjectStatusFieldID),
		ProjectStatusOptions:     cloneOptions(raw.ProjectStatusOptions),
	}
	if err := validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

type configJSON struct {
	GHESHost                 string            `json:"ghesHost"`
	APIBase                  string            `json:"apiBase"`
	APIVersion               string            `json:"apiVersion"`
	StateDir                 string            `json:"stateDir"`
	HerdrBinary              string            `json:"herdrBinary"`
	GitBinary                string            `json:"gitBinary"`
	WorkingWait              json.RawMessage   `json:"workingWait"`
	RecoveryLimit            int               `json:"recoveryLimit"`
	ProjectAutomationEnabled bool              `json:"projectAutomationEnabled"`
	ProjectID                string            `json:"projectId"`
	ProjectStatusFieldID     string            `json:"projectStatusFieldId"`
	ProjectStatusOptions     map[string]string `json:"projectStatusOptions"`
}

func decodeDuration(data json.RawMessage) (time.Duration, error) {
	if len(data) == 0 || string(data) == "null" {
		return 0, nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		d, err := time.ParseDuration(text)
		if err != nil {
			return 0, err
		}
		if d < 0 {
			return 0, errors.New("must not be negative")
		}
		return d, nil
	}
	var number int64
	if err := json.Unmarshal(data, &number); err != nil {
		return 0, errors.New("must be a duration string or nanoseconds")
	}
	if number < 0 {
		return 0, errors.New("must not be negative")
	}
	return time.Duration(number), nil
}

func validate(c Config) error {
	if strings.TrimSpace(c.GHESHost) == "" {
		return errors.New("ghesHost is required")
	}
	if err := validateGHESHost(c.GHESHost); err != nil {
		return err
	}
	if strings.TrimSpace(c.APIBase) == "" {
		return errors.New("apiBase is required")
	}
	if err := validateAPIBase(c.APIBase); err != nil {
		return err
	}
	if err := validatePublicEndpointPair(c.GHESHost, c.APIBase); err != nil {
		return err
	}
	if strings.TrimSpace(c.APIVersion) == "" {
		return errors.New("apiVersion is required")
	}
	if c.WorkingWait <= 0 {
		return errors.New("workingWait must be positive")
	}
	if c.RecoveryLimit <= 0 {
		return errors.New("recoveryLimit must be positive")
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return errors.New("projectId is required")
	}
	if strings.TrimSpace(c.ProjectStatusFieldID) == "" {
		return errors.New("projectStatusFieldId is required")
	}
	for _, status := range requiredProjectStatuses {
		if strings.TrimSpace(c.ProjectStatusOptions[status]) == "" {
			return fmt.Errorf("projectStatusOptions.%s is required", status)
		}
	}
	return nil
}

func validatePublicEndpointPair(ghesHost, apiBase string) error {
	host, hostErr := url.Parse(ghesHost)
	api, apiErr := url.Parse(apiBase)
	if hostErr != nil || apiErr != nil {
		return nil // The field-specific endpoint validators report this error.
	}
	publicHost := isPublicHostname(host.Hostname(), "github.com")
	publicAPI := isPublicHostname(api.Hostname(), "api.github.com")
	if publicHost {
		if host.Scheme != "https" {
			return errors.New("ghesHost must use HTTPS for github.com")
		}
		if host.Host != "github.com" {
			return errors.New("ghesHost must be exactly https://github.com")
		}
		if !publicAPI || api.Scheme != "https" || api.Host != "api.github.com" || api.Path != "" {
			return errors.New("apiBase must be https://api.github.com for GitHub.com")
		}
	}
	if publicAPI {
		if api.Scheme != "https" {
			return errors.New("apiBase must use HTTPS for api.github.com")
		}
		if api.Host != "api.github.com" || api.Path != "" {
			return errors.New("apiBase must be exactly https://api.github.com")
		}
		if !publicHost {
			return errors.New("ghesHost must be https://github.com when apiBase targets GitHub.com")
		}
	}
	return nil
}

func isPublicHostname(hostname, expected string) bool {
	return strings.EqualFold(strings.TrimSuffix(hostname, "."), expected)
}

func validateGHESHost(value string) error {
	u, err := parseEndpoint(value, "ghesHost")
	if err != nil {
		return err
	}
	if u.Path != "" && u.Path != "/" {
		return errors.New("ghesHost must not contain an API path")
	}
	return nil
}

func validateAPIBase(value string) error {
	_, err := parseEndpoint(value, "apiBase")
	return err
}

func parseEndpoint(value, field string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be an absolute HTTP(S) URL", field)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https", field)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%s must include a hostname", field)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%s must not contain userinfo, query, or fragment", field)
	}
	return u, nil
}

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join("~", ".local", "state", "threaddock")
	}
	return filepath.Join(home, ".local", "state", "threaddock")
}

func cloneOptions(options map[string]string) map[string]string {
	if options == nil {
		return nil
	}
	clone := make(map[string]string, len(options))
	for key, value := range options {
		clone[key] = value
	}
	return clone
}
