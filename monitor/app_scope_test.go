package main

import "testing"

func TestMonitorSnapshotScopesUseSeparateSources(t *testing.T) {
	openSource := &fakeSnapshotSource{snapshot: Snapshot{SchemaVersion: 2, Revision: 1, Projects: []Project{}}}
	allSource := &fakeSnapshotSource{snapshot: Snapshot{SchemaVersion: 2, Revision: 2, Projects: []Project{}}}
	app := &App{source: openSource, allSource: allSource}

	openSnapshot, err := app.GetMonitorSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	allSnapshot, err := app.GetMonitorSnapshotAll()
	if err != nil {
		t.Fatal(err)
	}
	if openSnapshot.Revision != 1 || allSnapshot.Revision != 2 {
		t.Fatalf("open=%#v all=%#v", openSnapshot, allSnapshot)
	}
	if openSource.calls != 1 || allSource.calls != 1 {
		t.Fatalf("open calls=%d all calls=%d", openSource.calls, allSource.calls)
	}
}
