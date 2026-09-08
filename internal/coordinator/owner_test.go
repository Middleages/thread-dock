package coordinator

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"thread-dock/internal/contract/v2"
)

func TestOwnerLeaseHelper(t *testing.T) {
	if os.Getenv("THREADDOCK_OWNER_HELPER") != "1" {
		return
	}

	root := os.Getenv("THREADDOCK_OWNER_ROOT")
	workID := contractv2.WorkID(os.Getenv("THREADDOCK_OWNER_WORK"))
	mode := os.Getenv("THREADDOCK_OWNER_MODE")
	lease, err := NewOwnerLocker(root).Acquire(context.Background(), workID, OwnerID("helper"), os.Getpid(), ownerTestTime())
	if mode == "busy" {
		if !errors.Is(err, ErrOwnerBusy) {
			fmt.Fprintf(os.Stderr, "expected ErrOwnerBusy, got %v", err)
			os.Exit(2)
		}
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper acquire: %v", err)
		os.Exit(2)
	}
	if mode != "hold" {
		fmt.Fprintf(os.Stderr, "unknown helper mode %q", mode)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "ready")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	_ = lease.Release()
}

func TestAcquireCreatesSecureOwnerRecord(t *testing.T) {
	root := t.TempDir()
	workID := contractv2.WorkID("work-1")
	startedAt := ownerTestTime()

	lease, err := NewOwnerLocker(root).Acquire(context.Background(), workID, OwnerID("owner-1"), 42, startedAt)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })
	if got := lease.Record(); got != (OwnerRecord{WorkID: workID, OwnerID: "owner-1", PID: 42, StartedAt: startedAt}) {
		t.Fatalf("Record() = %#v", got)
	}

	workDir := filepath.Join(root, "v2", "work", string(workID))
	for _, path := range []string{filepath.Join(root, "v2"), filepath.Join(root, "v2", "work"), workDir} {
		assertMode(t, path, 0o700)
	}
	assertMode(t, filepath.Join(workDir, "coordinator.lock"), 0o600)
	assertMode(t, filepath.Join(workDir, "owner.json"), 0o600)

	payload, err := os.ReadFile(filepath.Join(workDir, "owner.json"))
	if err != nil {
		t.Fatalf("ReadFile(owner.json): %v", err)
	}
	var got OwnerRecord
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode owner.json: %v", err)
	}
	if got != lease.Record() {
		t.Fatalf("owner.json = %#v, want %#v", got, lease.Record())
	}
}

func TestAcquireRejectsInvalidInputsWithoutCreatingFiles(t *testing.T) {
	validWork := contractv2.WorkID("work-1")
	validOwner := OwnerID("owner-1")
	validTime := ownerTestTime()
	tests := []struct {
		name  string
		work  contractv2.WorkID
		owner OwnerID
		pid   int
		at    time.Time
	}{
		{name: "blank work id", work: " ", owner: validOwner, pid: 1, at: validTime},
		{name: "path-like work id", work: "work/id", owner: validOwner, pid: 1, at: validTime},
		{name: "blank owner id", work: validWork, owner: "", pid: 1, at: validTime},
		{name: "path-like owner id", work: validWork, owner: "owner\\id", pid: 1, at: validTime},
		{name: "non-positive pid", work: validWork, owner: validOwner, pid: 0, at: validTime},
		{name: "zero timestamp", work: validWork, owner: validOwner, pid: 1},
		{name: "non-UTC timestamp", work: validWork, owner: validOwner, pid: 1, at: time.Date(2026, 9, 8, 1, 2, 3, 0, time.FixedZone("UTC", 0))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			_, err := NewOwnerLocker(root).Acquire(context.Background(), tc.work, tc.owner, tc.pid, tc.at)
			if !errors.Is(err, ErrInvalidOwner) {
				t.Fatalf("Acquire error = %v, want ErrInvalidOwner", err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatalf("ReadDir(root): %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("invalid Acquire created files: %v", entries)
			}
		})
	}
}

func TestOwnerLeaseUsesLiveFlockAndCrashUnlocks(t *testing.T) {
	root := t.TempDir()
	workID := contractv2.WorkID("work-1")
	lease, err := NewOwnerLocker(root).Acquire(context.Background(), workID, OwnerID("parent"), 41, ownerTestTime())
	if err != nil {
		t.Fatalf("parent Acquire: %v", err)
	}

	ownerPath := filepath.Join(root, "v2", "work", string(workID), "owner.json")
	stale := OwnerRecord{WorkID: workID, OwnerID: "stale", PID: 999, StartedAt: ownerTestTime().Add(-24 * time.Hour)}
	stalePayload, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownerPath, stalePayload, 0o600); err != nil {
		t.Fatalf("write stale owner.json: %v", err)
	}

	busy := ownerHelper(t, root, workID, "busy")
	if output, err := busy.CombinedOutput(); err != nil {
		t.Fatalf("contending helper: %v (%s)", err, output)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	hold := ownerHelper(t, root, workID, "hold")
	stdout, err := hold.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := hold.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	started := false
	defer func() {
		_ = stdin.Close()
		if started && hold.ProcessState == nil {
			_ = hold.Process.Kill()
			_ = hold.Wait()
		}
	}()
	if err := hold.Start(); err != nil {
		t.Fatalf("start holding helper: %v", err)
	}
	started = true
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(ready) != "ready" {
		_ = hold.Process.Kill()
		_ = hold.Wait()
		t.Fatalf("holding helper did not become ready: %q (%v)", ready, err)
	}
	if err := hold.Process.Kill(); err != nil {
		t.Fatalf("kill holding helper: %v", err)
	}
	_ = stdin.Close()
	if err := hold.Wait(); err == nil {
		t.Fatal("holding helper unexpectedly exited cleanly after kill")
	}

	newLease, err := NewOwnerLocker(root).Acquire(context.Background(), workID, OwnerID("after-crash"), 43, ownerTestTime())
	if err != nil {
		t.Fatalf("Acquire after helper crash: %v", err)
	}
	_ = newLease.Release()
}

func TestOwnerLeaseReleaseIsIdempotent(t *testing.T) {
	root := t.TempDir()
	locker := NewOwnerLocker(root)
	lease, err := locker.Acquire(context.Background(), "work-1", "owner-1", 1, ownerTestTime())
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
	other, err := locker.Acquire(context.Background(), "work-1", "owner-2", 2, ownerTestTime())
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	_ = other.Release()
}

func ownerHelper(t *testing.T, root string, workID contractv2.WorkID, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestOwnerLeaseHelper$")
	cmd.Env = append(os.Environ(),
		"THREADDOCK_OWNER_HELPER=1",
		"THREADDOCK_OWNER_ROOT="+root,
		"THREADDOCK_OWNER_WORK="+string(workID),
		"THREADDOCK_OWNER_MODE="+mode,
	)
	return cmd
}

func ownerTestTime() time.Time {
	return time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o, want %04o", path, got, want)
	}
}
