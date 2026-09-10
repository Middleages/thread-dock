// Package monitorcli reads the aggregate monitor snapshot from the WSL CLI.
package monitorcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"thread-dock/internal/monitor"
	runnerpkg "thread-dock/internal/runner"
)

// Runner is the process boundary used by the monitor adapter.
type Runner = runnerpkg.Runner

const (
	executable = "wsl.exe"
	command    = "--exec agentctl project status --all --json"
)

var commandArgs = []string{"--exec", "agentctl", "project", "status", "--all", "--json"}

// Client invokes the aggregate status command and retains one last-good copy
// for an honest degraded display when a later refresh cannot complete.
type Client struct {
	runner  Runner
	timeout time.Duration

	mu   sync.RWMutex
	last *monitor.Snapshot
}

func New(runner Runner, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{runner: runner, timeout: timeout}
}

// FetchAll executes the one aggregate command. Before any successful fetch,
// failures return an error. Once a good snapshot exists, later failures return
// a copied stale/offline snapshot with a nil error.
func (c *Client) FetchAll(ctx context.Context) (monitor.Snapshot, error) {
	if c.runner == nil {
		return monitor.Snapshot{}, errors.New("monitor command runner is nil")
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result, err := c.runner.Run(callCtx, "", executable, commandArgs...)
	if callCtx.Err() != nil {
		return c.degraded(fmt.Errorf("monitor command timeout: %w", callCtx.Err()))
	}
	if err != nil || result.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("exit status %d", result.ExitCode)
		}
		return c.degraded(fmt.Errorf("monitor command failed (exit %d): %w", result.ExitCode, err))
	}

	snapshot, err := decodeSnapshot([]byte(result.Stdout))
	if err != nil {
		return c.degraded(fmt.Errorf("invalid monitor snapshot: %w", err))
	}

	copyOfSnapshot, err := cloneSnapshot(snapshot)
	if err != nil {
		return monitor.Snapshot{}, fmt.Errorf("copy monitor snapshot: %w", err)
	}
	c.mu.Lock()
	c.last = &copyOfSnapshot
	c.mu.Unlock()
	returned, cloneErr := cloneSnapshot(copyOfSnapshot)
	if cloneErr != nil {
		return monitor.Snapshot{}, fmt.Errorf("copy monitor snapshot for caller: %w", cloneErr)
	}
	return returned, nil
}

func (c *Client) degraded(cause error) (monitor.Snapshot, error) {
	c.mu.RLock()
	last := c.last
	if last == nil {
		c.mu.RUnlock()
		return monitor.Snapshot{}, cause
	}
	snapshot, cloneErr := cloneSnapshot(*last)
	c.mu.RUnlock()
	if cloneErr != nil {
		return monitor.Snapshot{}, fmt.Errorf("copy retained monitor snapshot: %w: %v", cloneErr, cause)
	}
	snapshot.State = "stale"
	snapshot.SyncStatus = "offline"
	snapshot.Freshness.State = "stale"
	snapshot.Freshness.SyncStatus = "offline"
	return snapshot, nil
}

func decodeSnapshot(data []byte) (monitor.Snapshot, error) {
	var snapshot monitor.Snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return monitor.Snapshot{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return monitor.Snapshot{}, errors.New("trailing JSON values")
	} else if !errors.Is(err, io.EOF) {
		return monitor.Snapshot{}, fmt.Errorf("trailing JSON: %w", err)
	}
	if err := validateSnapshot(snapshot); err != nil {
		return monitor.Snapshot{}, err
	}
	return snapshot, nil
}

func validateSnapshot(snapshot monitor.Snapshot) error {
	if snapshot.SchemaVersion != 2 {
		return fmt.Errorf("schemaVersion = %d, want 2", snapshot.SchemaVersion)
	}
	if snapshot.EvidenceRefs == nil {
		return errors.New("evidenceRefs must be an array")
	}
	if snapshot.Projects == nil {
		return errors.New("projects must be an array")
	}
	for projectIndex, project := range snapshot.Projects {
		if project.EvidenceRefs == nil {
			return fmt.Errorf("projects[%d].evidenceRefs must be an array", projectIndex)
		}
		if project.WorkItems == nil {
			return fmt.Errorf("projects[%d].workItems must be an array", projectIndex)
		}
		for workIndex, work := range project.WorkItems {
			if work.EvidenceRefs == nil {
				return fmt.Errorf("projects[%d].workItems[%d].evidenceRefs must be an array", projectIndex, workIndex)
			}
			if work.Tasks == nil {
				return fmt.Errorf("projects[%d].workItems[%d].tasks must be an array", projectIndex, workIndex)
			}
			if work.Publications == nil {
				return fmt.Errorf("projects[%d].workItems[%d].publications must be an array", projectIndex, workIndex)
			}
		}
	}
	return nil
}

func cloneSnapshot(snapshot monitor.Snapshot) (monitor.Snapshot, error) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return monitor.Snapshot{}, err
	}
	return decodeSnapshot(data)
}
