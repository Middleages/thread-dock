# Task 5 report

## Status

Implemented the manual retirement CLI, lifecycle status representation,
production retirement wiring, and mixed active/retired cleanup migration.

## Changes

- Added `agentctl retire RUN` and guarded `agentctl retire RUN --blocked`
  routing through a narrow `RetirementService` seam.
- Added durable-phase validation, idempotent retired handling, bounded
  retirement advancement, and generic retirement CLI errors.
- Allowed `resume RUN` to advance `PhaseRetiring`.
- Added per-Agent `Lifecycle` status (`active`, `retiring`, `retired`) to
  human and JSON status output, including parallel task agents.
- Wired production retirement to the snapshot's exact repository path, the
  configured state-managed root, the configured Herdr root, Workspace reader/
  closer ports, Git retirement proof adapter, and auto-retirement setting.
- Added retired Git proof preflight/removal ports. Cleanup now derives target
  ownership from durable retirement state, preflights all active Herdr and
  retired Git targets before mutation, deduplicates shared reviewer/
  integration paths, and never sends a closed Workspace ID to Herdr removal.
- Added CLI and mixed-cleanup regression tests.

## Verification

- `go test ./internal/cli ./cmd/agentctl` — PASS
- `go vet ./...` — PASS
- Focused retirement orchestration tests — PASS
- Other repository packages in `go test ./...` — PASS

The workspace image did not have Go installed, so verification used a
temporary Go 1.24.6 toolchain. A repository-wide `go test ./... -timeout=180s`
reached the existing `internal/orchestrator/TestParallelStories` test and
timed out after three minutes while it was repeatedly exercising the long
parallel repair story; no assertion failure was reported before the timeout.

## Concerns

- The full orchestrator package is unusually slow in this container; the
  focused retirement stories and all changed packages pass.
- Optional retirement inspector support keeps narrow legacy cleanup fakes
  source-compatible; the production `SafeWorktreeCleanup` always performs
  strict read-only Git proof before removal.
