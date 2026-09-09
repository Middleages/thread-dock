# Active artifact reconcile fix report

## Task

- taskId: `herdr-builder-active-artifact`
- baseSHA: `29749917552bf42e80f6c65954437add95ae0203`
- branch: `agent/runtime-adapter-ingestion`
- worktree: `/home/appuser/dev_system/.worktrees/runtime-adapter-ingestion`
- commitSHA: `13bfd3c0a8a866fa0dd85933609e84ee39668eb0` (implementation commit)

## RED

Added `TestCoordinatorReconcileRejectsActiveBuilderArtifactWhileRunning` using a real `statev2.Store`, temporary Git repository, and valid `reserve` → `begin` → `mark_running` transitions. The runtime returned `RuntimeObservationActive` with a Builder artifact.

Command:

```text
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/coordinator -run '^TestCoordinatorReconcileRejectsActiveBuilderArtifactWhileRunning$' -count=1 -timeout=120s
```

Raw outcome:

```text
--- FAIL: TestCoordinatorReconcileRejectsActiveBuilderArtifactWhileRunning (0.19s)
    builder_ingestion_integration_test.go:116: active Builder artifact unexpectedly reconciled
FAIL
FAIL	thread-dock/internal/coordinator	0.191s
FAIL
```

This reproduced the defect: `TaskRunning` with matching active identity and a non-ended artifact returned success without calling `Terminate` or persisting a blocker.

## GREEN

Added a guard immediately after Observe identity validation in `reconcileInvocation`. Any non-ended observation carrying an artifact now uses the existing `runtimeUnknown` path with the static diagnostic `active runtime returned an artifact before termination`.

Command:

```text
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/coordinator -run '^TestCoordinatorReconcileRejectsActiveBuilderArtifactWhileRunning$' -count=1 -timeout=120s
```

Raw outcome:

```text
ok  	thread-dock/internal/coordinator	0.212s
```

Focused package verification:

```text
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/coordinator -count=1 -timeout=120s
ok  	thread-dock/internal/coordinator	5.421s

docker run --rm -v "$PWD":/src -w /src golang:1.27 go vet ./internal/coordinator
exit 0
```

## Self-review

- Non-ended artifacts are rejected before `TaskTerminationPending` or `Active` handling.
- The rejection persists `TaskNeedsOperator` and an operator blocker through `runtimeUnknown`.
- The diagnostic is static and does not persist provider artifact/error contents.
- The regression test verifies no candidate is recorded and `Terminate` is not called.
- No public interface, shared type, dependency, or forbidden path was changed.

## Result

- changedFiles:
  - `internal/coordinator/reconcile.go`
  - `internal/coordinator/builder_ingestion_integration_test.go`
  - `.superpowers/sdd/2026-09-09-herdr-builder-bridge/active-artifact-fix-report.md`
- commitSHA: `13bfd3c0a8a866fa0dd85933609e84ee39668eb0` (implementation commit; report is included in the task worktree)
- executedCommands:
  - `docker run --rm -v "$PWD":/src -w /src golang:1.27 gofmt -w internal/coordinator/reconcile.go internal/coordinator/builder_ingestion_integration_test.go`
  - `docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/coordinator -run '^TestCoordinatorReconcileRejectsActiveBuilderArtifactWhileRunning$' -count=1 -timeout=120s`
  - `docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/coordinator -count=1 -timeout=120s`
  - `docker run --rm -v "$PWD":/src -w /src golang:1.27 go vet ./internal/coordinator`
- outcomes: RED reproduced; regression test GREEN; focused coordinator tests and vet passed.
- unverified: repository-wide `make check`; live Herdr/OpenCode capability.
- blockers: none.
