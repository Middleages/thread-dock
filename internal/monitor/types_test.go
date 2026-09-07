package monitor

import (
	"encoding/json"
	"testing"
	"time"

	"thread-dock/internal/contract/v2"
)

func TestSnapshotJSONUsesAggregateWireAndOmitsRuntimeIdentity(t *testing.T) {
	snapshot := Snapshot{
		SchemaVersion: 2, Revision: 3, ObservedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		Freshness: Freshness{State: "fresh", SyncStatus: "synced"},
		Projects:  []Project{{ProjectID: contractv2.ProjectID("project-1"), Name: "Project", State: "running", SyncStatus: "synced", NextAction: "review", EvidenceRefs: []string{"state://work-1"}}},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schemaVersion", "revision", "state", "syncStatus", "nextAction", "evidenceRefs"} {
		if _, ok := wire[key]; key == "state" || key == "syncStatus" || key == "nextAction" || key == "evidenceRefs" {
			if ok {
				continue
			}
			if len(snapshot.Projects) == 0 {
				t.Fatalf("missing %s", key)
			}
		} else if !ok {
			t.Fatalf("missing %s", key)
		}
	}
	for _, forbidden := range []string{"providerSession", "processId", "rawTranscript", "sessionId"} {
		if containsJSONKey(data, forbidden) {
			t.Fatalf("forbidden key %q leaked: %s", forbidden, data)
		}
	}
}

func containsJSONKey(data []byte, key string) bool {
	var wire any
	if json.Unmarshal(data, &wire) != nil {
		return false
	}
	return findJSONKey(wire, key)
}

func findJSONKey(value any, key string) bool {
	switch v := value.(type) {
	case map[string]any:
		if _, ok := v[key]; ok {
			return true
		}
		for _, child := range v {
			if findJSONKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if findJSONKey(child, key) {
				return true
			}
		}
	}
	return false
}
