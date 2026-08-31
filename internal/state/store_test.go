package state

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestSaveAndLoadPersistsTaskMapDeterministically(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	first := RunSnapshot{
		RunID: "run-tasks",
		Phase: contract.PhaseBuilding,
		Tasks: map[string]TaskRunState{
			"tests": {State: "pending", ProgressFingerprint: "tests-fingerprint", RecoveryCount: 1},
			"api":   {State: "running", ProgressFingerprint: "api-fingerprint", RecoveryCount: 2},
		},
	}
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(filepath.Join(root, "runs", "run-tasks", "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Tasks = map[string]TaskRunState{
		"api":   first.Tasks["api"],
		"tests": first.Tasks["tests"],
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(filepath.Join(root, "runs", "run-tasks", "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("snapshot JSON is not deterministic:\nfirst=%ssecond=%s", wantBytes, gotBytes)
	}
	got, err := store.Load(context.Background(), "run-tasks")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tasks, first.Tasks) {
		t.Fatalf("tasks=%#v want=%#v", got.Tasks, first.Tasks)
	}
}

func TestSaveAndLoadPersistsPreviousTaskRequestID(t *testing.T) {
	store := NewStore(t.TempDir())
	want := RunSnapshot{
		RunID: "run-request-id", Phase: contract.PhaseBuilding,
		Tasks: map[string]TaskRunState{
			"api": {Prompt: PromptReceipt{RequestID: "run-request-id:api:attempt-1"}, PromptGeneration: 1, PreviousRequestID: "run-request-id:api:prompt"},
		},
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), want.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tasks["api"].PreviousRequestID != "run-request-id:api:prompt" {
		t.Fatalf("previous request ID=%q", got.Tasks["api"].PreviousRequestID)
	}
	if got.Tasks["api"].PromptGeneration != 1 {
		t.Fatalf("prompt generation=%d", got.Tasks["api"].PromptGeneration)
	}
}

func TestLegacySingleRunSnapshotStillDecodes(t *testing.T) {
	var got RunSnapshot
	if err := json.Unmarshal([]byte(`{"runId":"run-1","builder":{"name":"builder-run-1"}}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.Builder.Name != "builder-run-1" || got.Tasks != nil {
		t.Fatalf("snapshot=%#v", got)
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

func TestAppendRepairsPreExistingPartialTail(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if err := store.Create(context.Background(), RunSnapshot{RunID: "run-1", Phase: contract.PhaseRegistered}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runs", "run-1", "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"partial"`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), "run-1", Event{Type: "repaired", At: time.Unix(2, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
	if len(lines) != 1 {
		t.Fatalf("lines=%q", lines)
	}
	var event Event
	if err := json.Unmarshal(lines[0], &event); err != nil {
		t.Fatalf("line=%q: %v", lines[0], err)
	}
	if event.Type != "repaired" {
		t.Fatalf("event=%#v", event)
	}
}

func TestAppendRollsBackPrefixWhenWriteFails(t *testing.T) {
	file := &prefixThenErrorFile{fail: errInjectedWrite}
	err := appendEventFile(file, []byte(`{"type":"event"}`+"\n"))
	if !errors.Is(err, errInjectedWrite) {
		t.Fatalf("err=%v", err)
	}
	if got := file.data.Bytes(); len(got) != 0 {
		t.Fatalf("partial data remains: %q", got)
	}
	if file.syncs < 1 {
		t.Fatalf("rollback was not synced: %d", file.syncs)
	}
}

func TestAppendReportsRollbackAndSyncErrorsWithWriteError(t *testing.T) {
	errTruncate := errors.New("injected truncate failure")
	errSync := errors.New("injected sync failure")
	file := &prefixThenErrorFile{fail: errInjectedWrite, truncateFail: errTruncate, syncFail: errSync}
	err := appendEventFile(file, []byte(`{"type":"event"}`+"\n"))
	for _, want := range []error{errInjectedWrite, errTruncate, errSync} {
		if !errors.Is(err, want) {
			t.Fatalf("err=%v does not contain %v", err, want)
		}
	}
}

var errInjectedWrite = errors.New("injected write failure")

type prefixThenErrorFile struct {
	data         bytes.Buffer
	position     int64
	fail         error
	writeCount   int
	syncs        int
	truncateFail error
	syncFail     error
}

func (f *prefixThenErrorFile) Read(p []byte) (int, error) {
	if f.position >= int64(f.data.Len()) {
		return 0, io.EOF
	}
	n := copy(p, f.data.Bytes()[f.position:])
	f.position += int64(n)
	return n, nil
}

func (f *prefixThenErrorFile) Write(p []byte) (int, error) {
	if f.writeCount > 0 {
		return 0, f.fail
	}
	f.writeCount++
	prefix := len(p) / 2
	if prefix == 0 {
		prefix = 1
	}
	_, _ = f.data.Write(p[:prefix])
	f.position += int64(prefix)
	return prefix, f.fail
}

func (f *prefixThenErrorFile) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = f.position + offset
	case io.SeekEnd:
		next = int64(f.data.Len()) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if next < 0 {
		return 0, errors.New("negative position")
	}
	f.position = next
	return next, nil
}

func (f *prefixThenErrorFile) Truncate(size int64) error {
	if f.truncateFail != nil {
		return f.truncateFail
	}
	if size < 0 || size > int64(f.data.Len()) {
		return errors.New("invalid truncate")
	}
	data := append([]byte(nil), f.data.Bytes()[:size]...)
	f.data.Reset()
	_, _ = f.data.Write(data)
	if f.position > size {
		f.position = size
	}
	return nil
}

func (f *prefixThenErrorFile) Sync() error {
	f.syncs++
	if f.syncFail != nil {
		return f.syncFail
	}
	return nil
}

func (f *prefixThenErrorFile) Close() error { return nil }

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
