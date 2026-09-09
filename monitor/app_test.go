package main

import (
	"context"
	"errors"
	"testing"

	"thread-dock/internal/monitor"
)

type fakeSnapshotSource struct {
	snapshot monitor.Snapshot
	err      error
	calls    int
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
