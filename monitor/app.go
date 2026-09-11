package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// App is the single Wails-facing monitor binding. It can run with an injected
// source for tests or own a configurable GitHub+Herdr source in production.
type App struct {
	mu sync.RWMutex

	source SnapshotSource

	baseEnv  map[string]string
	process  CommandRunner
	timeout  time.Duration
	store    settingsStore
	settings MonitorSettings
}

func NewApp(source SnapshotSource) *App {
	return &App{source: source}
}

func NewConfigurableApp(baseEnv map[string]string, process CommandRunner, timeout time.Duration) *App {
	store := defaultSettingsStore()
	settings := settingsFromEnv(baseEnv)
	if stored, ok, err := store.load(); err == nil && ok {
		settings = stored
	}
	app := &App{
		baseEnv:  cloneStringMap(baseEnv),
		process:  process,
		timeout:  timeout,
		store:    store,
		settings: settings,
	}
	app.source = app.buildSource(settings)
	return app
}

func (a *App) buildSource(settings MonitorSettings) SnapshotSource {
	env := settings.environment(a.baseEnv)
	github := NewGitHubMonitor(env, a.process, a.timeout)
	herdr := NewHerdrMonitor(env, a.process, a.timeout)
	return NewCombinedMonitor(github, herdr)
}

// GetMonitorSnapshot returns the provider-neutral aggregate wire for the UI.
func (a *App) GetMonitorSnapshot() (Snapshot, error) {
	if a == nil {
		return Snapshot{}, errors.New("monitor snapshot source is nil")
	}
	a.mu.RLock()
	source := a.source
	a.mu.RUnlock()
	if source == nil {
		return Snapshot{}, errors.New("monitor snapshot source is nil")
	}
	return source.FetchAll(context.Background())
}

// GetMonitorSettings returns the currently effective local monitor settings.
func (a *App) GetMonitorSettings() (MonitorSettings, error) {
	if a == nil {
		return MonitorSettings{}, errors.New("monitor app is nil")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings, nil
}

// SaveMonitorSettings validates and persists settings, then immediately rebuilds
// the read-only GitHub and Herdr monitor sources without requiring an app restart.
func (a *App) SaveMonitorSettings(settings MonitorSettings) (MonitorSettings, error) {
	if a == nil {
		return MonitorSettings{}, errors.New("monitor app is nil")
	}
	if err := validateMonitorSettings(settings); err != nil {
		return MonitorSettings{}, err
	}
	if err := a.store.save(settings); err != nil {
		return MonitorSettings{}, err
	}
	newSource := a.buildSource(settings)
	a.mu.Lock()
	a.settings = settings
	a.source = newSource
	a.mu.Unlock()
	return settings, nil
}
