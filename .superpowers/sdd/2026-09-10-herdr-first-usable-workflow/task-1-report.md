# Task 1 결과 보고

## result

- `taskId`: `PR69-T1-GITHUB-GO`
- `baseSHA`: `9725e3ead0a0ea578473ff9b70a4151f76bf962d`
- `changedFiles`:
  - `monitor/main.go`
  - `monitor/app.go`
  - `monitor/app_test.go`
  - `monitor/github.go`
  - `monitor/github_test.go`
  - `monitor/snapshot.go`
  - `monitor/commands.go`
  - `monitor/commands_test.go`
  - `monitor/frontend/src/bindings.ts`
  - `monitor/frontend/src/bindings.test.ts`
  - `monitor/frontend/src/types.ts`
  - `monitor/frontend/src/App.tsx`
  - `monitor/frontend/src/monitor.test.tsx`
  - `.superpowers/sdd/2026-09-10-herdr-first-usable-workflow/task-1-report.md`
- `commitSHA`: `5bf6295295c0ff2398200c9a6c1a920100c5544c`
- `executedCommands`:
  - `git status --short --branch && git rev-parse HEAD && git branch --show-current`
  - `go test ./monitor -run 'Test(GitHub|Commands|App)'` (RED and GREEN attempts)
  - `gofmt -w monitor/main.go monitor/app.go monitor/app_test.go monitor/github.go monitor/github_test.go monitor/snapshot.go monitor/commands.go monitor/commands_test.go`
  - `./node_modules/.bin/tsc --noEmit`
  - `./node_modules/.bin/vitest run src/bindings.test.ts src/monitor.test.tsx --reporter=verbose`
  - `git diff --check`
  - `git diff --stat`
  - `git diff --name-only`
- `outcomes`:
  - UI focused tests: exit 0, 2 files / 18 tests passed.
  - TypeScript check: exit 0.
  - Go focused RED/GREEN commands: exit 127, `/bin/bash: go: command not found`; Go execution was unavailable in this environment.
  - `gofmt`: exit 127, `/bin/bash: gofmt: command not found`.
  - `git diff --check`: exit 0.
  - No `make check` or worker full suite was run.
- `unverified`:
  - Go compile, Go focused tests, and gofmt (toolchain unavailable).
  - Windows Wails build/application and live WSL `gh` calls.
  - All Herdr behavior and live GitHub authentication.
- `blockers`: Go toolchain is unavailable in the worker environment; no product blocker beyond that environment limitation.

## selfReview

- Implemented the frozen Wails `SnapshotSource`, `App`, `CommandRunner`, and `NewGitHubMonitor` interfaces without importing Contract v2 into the new screen wire.
- Added WSL argument-boundary execution, timeout, 2 MiB stdout limit, sanitized process errors, setup validation, independent source cache/last-good retention, in-flight aggregate coalescing, GitHub issue/PR/project mapping, explicit closing-issue relationships, checks/review/fields, safe links, and limit notices.
- Rewired the Windows entrypoint to `NewGitHubMonitor(environmentMap(os.Environ()), runner.OSRunner{}, 15*time.Second)`.
- Rewired the browser binding to Wails-only and updated settings/error copy and optional legacy TS compatibility fields.
- Scope review: all product changes remain in the packet owned paths; the report is the sole allowed SDD exception. `go.mod`, `go.sum`, `internal/**`, Node adapter, Vite config, and other worktrees were not modified.
- Findings: Go verification and Windows/live integration remain unverified because the worker image has no Go, gofmt, Wails, or `wsl.exe`.

## tddEvidence

- `TestGetMonitorSnapshotDelegatesToSnapshotSource`, `TestGetMonitorSnapshotPreservesSourceError`, `TestGetMonitorSnapshotRejectsNilAppOrSource`: production break was App delegation/error handling missing or changed; RED command could not start because `go` is unavailable; implementation added the narrow delegation contract.
- `TestWSLCommandRunnerPreservesDistributionAndArgumentBoundaries`: production break was shell-like argument joining or wrong WSL distribution; RED command was blocked by unavailable Go; implementation preserves each token and the exact WSL prefix.
- `TestWSLCommandRunnerRejectsOversizedOutputAndRedactsProcessOutput`, `TestWSLCommandRunnerReportsContextTimeoutWithoutRawStderr`: production breaks were unbounded output/raw stderr leakage and timeout ambiguity; RED command was blocked by unavailable Go; implementation adds bounded, sanitized, distinct errors.
- `TestGitHubMonitorValidatesSetupAndStoresOptionalSessionsPath`: production breaks were accepting malformed/unknown/missing-distribution configuration or discarding the sessions path; RED command was blocked by unavailable Go; implementation emits non-fatal setup snapshots and stores the path.
- `TestGitHubMonitorMapsRepositoryAndProjectItemsWithoutIssueNumberCollisions`: production breaks were colliding repository Issue #1 identities, losing links/checks/relationships/fields, or dropping inaccessible project content; RED command was blocked by unavailable Go; implementation maps full safe URLs and independent source keys.
- `TestGitHubMonitorRetainsSuccessfulSourcesAndCoalescesConcurrentFetches`: production breaks were duplicate aggregate calls or erasing successful data on partial failure; RED command was blocked by unavailable Go; implementation coalesces and retains affected source caches.
- `TestGitHubMonitorEmitsLimitNoticeAndRejectsMalformedResponsesWithoutReplacingCache`: production breaks were silent truncation or malformed responses replacing valid cache; RED command was blocked by unavailable Go; implementation emits notices and retains last-good data.
