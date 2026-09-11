package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"thread-dock/internal/contract"
)

var (
	ErrEmptyRunID = errors.New("run ID is required")
	ErrInvalidRun = errors.New("run snapshot has no phase")
)

// Store is a filesystem-backed local state store. Each run has its own
// directory, making run.json replacement and events.jsonl append independent.
type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store { return &Store{root: root} }

func (s *Store) Create(ctx context.Context, snapshot RunSnapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(s.runDir(snapshot.RunID), 0700); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	return s.Save(ctx, snapshot)
}

func (s *Store) Load(ctx context.Context, runID contract.RunID) (RunSnapshot, error) {
	if err := validateRunID(runID); err != nil {
		return RunSnapshot{}, err
	}
	if err := checkContext(ctx); err != nil {
		return RunSnapshot{}, err
	}
	f, err := os.Open(filepath.Join(s.runDir(runID), "run.json"))
	if err != nil {
		return RunSnapshot{}, fmt.Errorf("load run %q: %w", runID, err)
	}
	defer f.Close()
	var snapshot RunSnapshot
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snapshot); err != nil {
		return RunSnapshot{}, fmt.Errorf("decode run %q: %w", runID, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return RunSnapshot{}, fmt.Errorf("decode run %q: trailing data", runID)
	}
	if err := validateSnapshot(snapshot); err != nil {
		return RunSnapshot{}, fmt.Errorf("load run %q: %w", runID, err)
	}
	return snapshot, nil
}

func (s *Store) Save(ctx context.Context, snapshot RunSnapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.runDir(snapshot.RunID), 0700); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	tmpPath := filepath.Join(s.runDir(snapshot.RunID), "run.json.tmp")
	finalPath := filepath.Join(s.runDir(snapshot.RunID), "run.json")
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open temporary snapshot: %w", err)
	}
	encodeErr := json.NewEncoder(f).Encode(snapshot)
	if encodeErr == nil {
		encodeErr = f.Sync()
	}
	closeErr := f.Close()
	if encodeErr != nil {
		return fmt.Errorf("write temporary snapshot: %w", encodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close temporary snapshot: %w", closeErr)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("commit snapshot: %w", err)
	}
	if err := syncDir(s.runDir(snapshot.RunID)); err != nil {
		return fmt.Errorf("sync snapshot directory: %w", err)
	}
	return nil
}

func (s *Store) Append(ctx context.Context, runID contract.RunID, event Event) error {
	if err := validateRunID(runID); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if event.RunID == "" {
		event.RunID = runID
	}
	if event.RunID != runID {
		return fmt.Errorf("event run ID %q does not match %q", event.RunID, runID)
	}
	if strings.TrimSpace(event.Type) == "" && strings.TrimSpace(event.Kind) == "" {
		return errors.New("event type is required")
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	encoded = append(encoded, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.runDir(runID), 0700); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(s.runDir(runID), "events.jsonl"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	if event.ID != "" {
		// Repair an incomplete tail before scanning so a provider/process crash
		// cannot hide an already-committed event behind partial JSON. The store
		// mutex covers repair, scan, and append as one idempotent operation.
		if err := repairEventTail(f); err != nil {
			_ = f.Close()
			return err
		}
		found, err := eventIDExists(f, event.ID)
		if err != nil {
			_ = f.Close()
			return err
		}
		if found {
			if err := f.Close(); err != nil {
				return fmt.Errorf("close event log: %w", err)
			}
			return nil
		}
	}
	appendErr := appendEventFile(f, encoded)
	closeErr := f.Close()
	if appendErr != nil || closeErr != nil {
		return fmt.Errorf("append event: %w", errors.Join(appendErr, closeErr))
	}
	return nil
}

func eventIDExists(f eventFile, id string) (bool, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("seek event log for ID scan: %w", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return false, fmt.Errorf("read event log for ID scan: %w", err)
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var existing Event
		if err := json.Unmarshal(line, &existing); err != nil {
			return false, fmt.Errorf("decode event log for ID scan: %w", err)
		}
		if existing.ID == id {
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				return false, fmt.Errorf("seek event log after ID scan: %w", err)
			}
			return true, nil
		}
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return false, fmt.Errorf("seek event log after ID scan: %w", err)
	}
	return false, nil
}

type eventFile interface {
	io.Reader
	io.Writer
	io.Seeker
	Truncate(size int64) error
	Sync() error
}

// appendEventFile repairs an incomplete tail before appending. A record is
// considered committed only after its complete JSON line has been synced.
// On any write/sync failure, the file is rolled back and that rollback is
// itself synced; errors are joined so callers can inspect every failure.
func appendEventFile(f eventFile, record []byte) error {
	if err := repairEventTail(f); err != nil {
		return err
	}
	start, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek event log: %w", err)
	}
	if err := writeComplete(f, record); err != nil {
		return rollbackEventAppend(f, start, err)
	}
	if err := f.Sync(); err != nil {
		return rollbackEventAppend(f, start, err)
	}
	return nil
}

func repairEventTail(f eventFile) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek event log for validation: %w", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read event log for validation: %w", err)
	}
	validEnd := validJSONLinesEnd(data)
	if validEnd != len(data) {
		if err := f.Truncate(int64(validEnd)); err != nil {
			return fmt.Errorf("truncate incomplete event tail: %w", err)
		}
		if err := f.Sync(); err != nil {
			return fmt.Errorf("sync repaired event tail: %w", err)
		}
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek event log after validation: %w", err)
	}
	return nil
}

func validJSONLinesEnd(data []byte) int {
	end := 0
	for end < len(data) {
		relative := bytes.IndexByte(data[end:], '\n')
		if relative < 0 {
			return end
		}
		lineEnd := end + relative
		line := data[end:lineEnd]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(bytes.TrimSpace(line)) == 0 || !json.Valid(line) {
			return end
		}
		end = lineEnd + 1
	}
	return end
}

func rollbackEventAppend(f eventFile, start int64, original error) error {
	rollbackErr := f.Truncate(start)
	syncErr := f.Sync()
	return errors.Join(original, rollbackErr, syncErr)
}

func writeComplete(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (s *Store) ListRecoverable(ctx context.Context) ([]RunSnapshot, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	all, err := s.list()
	if err != nil {
		return nil, err
	}
	result := make([]RunSnapshot, 0, len(all))
	for _, snapshot := range all {
		if snapshot.Phase != contract.PhaseCompleted && snapshot.Phase != contract.PhaseBlocked {
			result = append(result, snapshot)
		}
	}
	sortSnapshots(result)
	return result, nil
}

func (s *Store) list() ([]RunSnapshot, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return []RunSnapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	result := make([]RunSnapshot, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		snapshot, err := s.Load(context.Background(), contract.RunID(entry.Name()))
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func sortSnapshots(snapshots []RunSnapshot) {
	sort.SliceStable(snapshots, func(i, j int) bool {
		if snapshots[i].UpdatedAt.Equal(snapshots[j].UpdatedAt) {
			return snapshots[i].RunID < snapshots[j].RunID
		}
		return snapshots[i].UpdatedAt.After(snapshots[j].UpdatedAt)
	})
}

func validateSnapshot(snapshot RunSnapshot) error {
	if err := validateRunID(snapshot.RunID); err != nil {
		return err
	}
	if strings.TrimSpace(string(snapshot.Phase)) == "" {
		return ErrInvalidRun
	}
	return nil
}

func validateRunID(runID contract.RunID) error {
	value := string(runID)
	if strings.TrimSpace(value) == "" {
		return ErrEmptyRunID
	}
	if value == "." || value == ".." || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("invalid run ID %q", runID)
	}
	return nil
}

func (s *Store) runDir(runID contract.RunID) string {
	return filepath.Join(s.root, "runs", string(runID))
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
