package state

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract"
)

func TestLoadIgnoresUncommittedTemporarySnapshot(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	want := RunSnapshot{RunID: "run-184", Phase: contract.PhaseBuilding}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, "runs", "run-184", "run.json.tmp")
	if err := os.WriteFile(temp, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), "run-184")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != contract.PhaseBuilding {
		t.Fatalf("phase=%s", got.Phase)
	}
}

func TestSaveAtomicallyReplacesSnapshot(t *testing.T) {
	store := NewStore(t.TempDir())
	first := RunSnapshot{RunID: "run-1", Phase: contract.PhaseRegistered}
	second := RunSnapshot{RunID: "run-1", Phase: contract.PhaseReviewing}
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, second) {
		t.Fatalf("got=%#v want=%#v", got, second)
	}
	if _, err := os.Stat(filepath.Join(store.root, "runs", "run-1", "run.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary snapshot remains: %v", err)
	}
}

func TestAppendWritesCompleteJSONLines(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Create(context.Background(), RunSnapshot{RunID: "run-1", Phase: contract.PhaseRegistered}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []Event{
		{RunID: "run-1", Type: "created", At: time.Unix(1, 0).UTC()},
		{RunID: "run-1", Type: "phase", Phase: contract.PhaseBuilding, At: time.Unix(2, 0).UTC()},
	} {
		if err := store.Append(context.Background(), "run-1", event); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(filepath.Join(store.root, "runs", "run-1", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
		var decoded Event
		if err := json.Unmarshal(scanner.Bytes(), &decoded); err != nil {
			t.Fatalf("line %d is not JSON: %v", count, err)
		}
		if strings.TrimSpace(scanner.Text()) == "" {
			t.Fatalf("line %d is empty", count)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("lines=%d", count)
	}
}

func TestListRecoverableSortsNewestFirstAndExcludesTerminalRuns(t *testing.T) {
	store := NewStore(t.TempDir())
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	for _, snapshot := range []RunSnapshot{
		{RunID: "old", Phase: contract.PhaseBuilding, UpdatedAt: base},
		{RunID: "new", Phase: contract.PhasePaused, UpdatedAt: base.Add(time.Hour)},
		{RunID: "complete", Phase: contract.PhaseCompleted, UpdatedAt: base.Add(2 * time.Hour)},
		{RunID: "blocked", Phase: contract.PhaseBlocked, UpdatedAt: base.Add(3 * time.Hour)},
	} {
		if err := store.Save(context.Background(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListRecoverable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ids := snapshotIDs(got); !reflect.DeepEqual(ids, []string{"new", "old"}) {
		t.Fatalf("ids=%v", ids)
	}
}

func TestListCleanupCandidatesOnlyReturnsOldCompletedRunsWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	for _, snapshot := range []RunSnapshot{
		{RunID: "old-complete", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		{RunID: "boundary-complete", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-7 * 24 * time.Hour)},
		{RunID: "recent-complete", Phase: contract.PhaseCompleted, UpdatedAt: now.Add(-6 * 24 * time.Hour)},
		{RunID: "old-active", Phase: contract.PhaseBuilding, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
	} {
		if err := store.Save(context.Background(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListCleanupCandidates(now, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ids := snapshotIDs(got); !reflect.DeepEqual(ids, []string{"old-complete"}) {
		t.Fatalf("ids=%v", ids)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", "old-complete", "run.json")); err != nil {
		t.Fatalf("candidate was deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", "boundary-complete", "run.json")); err != nil {
		t.Fatalf("boundary snapshot was deleted: %v", err)
	}
}

func snapshotIDs(snapshots []RunSnapshot) []string {
	ids := make([]string, len(snapshots))
	for i := range snapshots {
		ids[i] = string(snapshots[i].RunID)
	}
	return ids
}
