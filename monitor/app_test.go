package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"thread-dock/internal/monitor"
	"thread-dock/internal/monitorcli"
	"thread-dock/internal/runner"
)

type fakeSnapshotSource struct {
	snapshot monitor.Snapshot
	err      error
	calls    int
}

type appSequenceRunner struct {
	steps []runner.Result
	call  int
}

func (r *appSequenceRunner) Run(_ context.Context, _ string, _ string, _ ...string) (runner.Result, error) {
	result := r.steps[r.call]
	r.call++
	if r.call == 1 {
		return result, nil
	}
	return result, errors.New("refresh failed")
}

func TestGetMonitorSnapshotReturnsRetainedDegradedSnapshotThroughRealClient(t *testing.T) {
	sequence := &appSequenceRunner{steps: []runner.Result{{Stdout: `{"schemaVersion":2,"revision":1,"observedAt":"2026-09-07T00:00:00Z","freshness":{"state":"fresh","syncStatus":"synced"},"state":"running","syncStatus":"synced","nextAction":"review","evidenceRefs":[],"projects":[]}`}, {ExitCode: 1}}}
	app := NewApp(monitorcli.New(sequence, time.Second))
	if _, err := app.GetMonitorSnapshot(); err != nil {
		t.Fatal(err)
	}
	degraded, err := app.GetMonitorSnapshot()
	if err != nil {
		t.Fatalf("degraded binding err = %v, want nil", err)
	}
	if degraded.State != "stale" || degraded.SyncStatus != "offline" {
		t.Fatalf("degraded binding snapshot=%#v", degraded)
	}
}

func (f *fakeSnapshotSource) FetchAll(context.Context) (monitor.Snapshot, error) {
	f.calls++
	return f.snapshot, f.err
}

func TestGetMonitorSnapshotDelegatesToSnapshotSource(t *testing.T) {
	want := monitor.Snapshot{SchemaVersion: 2, Projects: []monitor.Project{}}
	source := &fakeSnapshotSource{snapshot: want}
	app := NewApp(source)

	got, err := app.GetMonitorSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != want.SchemaVersion || len(got.Projects) != 0 || source.calls != 1 {
		t.Fatalf("snapshot=%#v calls=%d", got, source.calls)
	}
}

func TestGetMonitorSnapshotPreservesSourceError(t *testing.T) {
	wantErr := errors.New("offline")
	source := &fakeSnapshotSource{err: wantErr}
	app := NewApp(source)

	if _, err := app.GetMonitorSnapshot(); !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want %v", err, wantErr)
	}
}
