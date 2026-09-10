package main

import (
	"context"
	"errors"
)

// App is the single Wails-facing monitor binding. State remains owned by the
// injected aggregate source; this layer never reads state files or GitHub.
type App struct {
	source SnapshotSource
}

func NewApp(source SnapshotSource) *App {
	return &App{source: source}
}

// GetMonitorSnapshot returns the provider-neutral aggregate wire for the UI.
func (a *App) GetMonitorSnapshot() (Snapshot, error) {
	if a == nil || a.source == nil {
		return Snapshot{}, errors.New("monitor snapshot source is nil")
	}
	return a.source.FetchAll(context.Background())
}
