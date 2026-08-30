// Package integration validates Builder evidence and merges immutable results
// into the dedicated integration Worktree.
package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"thread-dock/internal/contract"
	"thread-dock/internal/pathscope"
	"thread-dock/internal/state"
	"thread-dock/internal/worktree"
)

var (
	ErrPathOwnership   = errors.New("changed path is outside task ownership")
	ErrBlockedConflict = errors.New("integration blocked by merge conflict")
	ErrInvalidResult   = errors.New("invalid integration result")
)

// Result identifies one Builder's immutable commit. TaskID preserves
// contract order metadata for callers; CommitSHA is the only merge target.
type Result struct {
	TaskID    string
	CommitSHA string
}

// Check is the normalized public verification result. It intentionally has no
// process output fields.
type Check struct {
	Command  string
	Outcome  string
	Duration string
	ExitCode int
}

// IntegrationResult is the final immutable integration commit and global
// verification evidence.
type IntegrationResult struct {
	CommitSHA    string
	Verification []Check
}

// Git is the narrow adapter required by Integrator. worktree owns the minimal
// check result so this package does not create an import cycle.
type Git interface {
	MergeCommitNoFF(context.Context, string, string) error
	AbortMerge(context.Context, string) error
	RunChecks(context.Context, string, []string) ([]worktree.VerificationCheck, error)
	CurrentCommit(context.Context, string) (string, error)
}

type Integrator struct {
	git Git
}

func New(git Git) *Integrator {
	return &Integrator{git: git}
}

// ValidateResult accepts only the strict evidence schema and Git-derived
// inspection. Agent-reported paths and patch fields are never authoritative.
func ValidateResult(task contract.Task, evidence state.AgentEvidence, inspection worktree.CommitInspection) error {
	if !isCommitSHA(evidence.CommitSHA) || !isCommitSHA(inspection.CommitSHA) || evidence.CommitSHA != inspection.CommitSHA {
		return fmt.Errorf("%w: commit SHA mismatch", ErrInvalidResult)
	}
	if task.Branch == "" || strings.TrimSpace(task.Branch) != task.Branch || inspection.Branch == "" || strings.TrimSpace(inspection.Branch) != inspection.Branch || inspection.Branch != task.Branch {
		return fmt.Errorf("%w: Builder branch mismatch", ErrInvalidResult)
	}
	if len(inspection.ChangedFiles) == 0 || strings.TrimSpace(inspection.Patch) == "" {
		return fmt.Errorf("%w: Git inspection is incomplete", ErrInvalidResult)
	}
	for _, file := range inspection.ChangedFiles {
		owned := false
		for _, allowed := range task.AllowedPaths {
			if pathscope.Contains(allowed, file) {
				owned = true
				break
			}
		}
		if !owned {
			return fmt.Errorf("%w: %s", ErrPathOwnership, file)
		}
	}
	if !exactVerification(evidence.VerificationEvidence, task.Verification) {
		return fmt.Errorf("%w: verification evidence mismatch", ErrInvalidResult)
	}
	return nil
}

func exactVerification(evidence []state.VerificationEvidence, required []string) bool {
	if len(required) == 0 || len(evidence) != len(required) {
		return false
	}
	want := make(map[string]int, len(required))
	for _, raw := range required {
		if raw == "" || strings.TrimSpace(raw) != raw {
			return false
		}
		want[raw]++
	}
	for _, check := range evidence {
		if check.Command == "" || strings.TrimSpace(check.Command) != check.Command || want[check.Command] == 0 || check.Outcome != "passed" || check.Duration == "" || strings.TrimSpace(check.Duration) != check.Duration {
			return false
		}
		duration, err := time.ParseDuration(check.Duration)
		if err != nil || duration <= 0 {
			return false
		}
		want[check.Command]--
	}
	for _, count := range want {
		if count != 0 {
			return false
		}
	}
	return true
}

// MergeResults merges immutable commits in the supplied contract order. A
// confirmed conflict is aborted through Git and blocks the integration; no
// reset or automatic conflict resolution is attempted.
func (i *Integrator) MergeResults(ctx context.Context, integrationPath string, results []Result, checks []string) (IntegrationResult, error) {
	if i == nil || i.git == nil {
		return IntegrationResult{}, errors.New("integration Git adapter is required")
	}
	if strings.TrimSpace(integrationPath) == "" {
		return IntegrationResult{}, worktree.ErrUnsafeTarget
	}
	for _, result := range results {
		if !isCommitSHA(result.CommitSHA) {
			return IntegrationResult{}, fmt.Errorf("%w: commit SHA must be 40 lowercase hexadecimal characters", ErrInvalidResult)
		}
		if err := i.git.MergeCommitNoFF(ctx, integrationPath, result.CommitSHA); err != nil {
			if !errors.Is(err, worktree.ErrConflict) {
				return IntegrationResult{}, err
			}
			abortErr := i.git.AbortMerge(ctx, integrationPath)
			if abortErr != nil {
				return IntegrationResult{}, errors.Join(ErrBlockedConflict, fmt.Errorf("abort merge: %w", abortErr))
			}
			return IntegrationResult{}, ErrBlockedConflict
		}
	}
	verification, err := i.git.RunChecks(ctx, integrationPath, checks)
	if err != nil {
		return IntegrationResult{}, err
	}
	commit, err := i.git.CurrentCommit(ctx, integrationPath)
	if err != nil {
		return IntegrationResult{}, err
	}
	commit = strings.TrimSpace(commit)
	if !isCommitSHA(commit) {
		return IntegrationResult{}, fmt.Errorf("%w: final commit SHA unavailable", ErrInvalidResult)
	}
	result := IntegrationResult{CommitSHA: commit, Verification: make([]Check, 0, len(verification))}
	for _, check := range verification {
		result.Verification = append(result.Verification, Check{
			Command:  strings.TrimSpace(check.Command),
			Outcome:  strings.TrimSpace(check.Outcome),
			Duration: strings.TrimSpace(check.Duration),
			ExitCode: check.ExitCode,
		})
	}
	return result, nil
}

func isCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
