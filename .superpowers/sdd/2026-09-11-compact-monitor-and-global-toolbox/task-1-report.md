# Task 1 implementation report — compact-toolbox-storage

## Packet

- taskId: `compact-toolbox-storage`
- baseSHA: `33d279ab59653a42a997b4f9c835c7857ce075dd`
- branch/worktree: `agent/compact-toolbox-storage` / `.worktrees/compact-toolbox-storage`
- ownedPaths: `monitor/toolbox.go`, `monitor/toolbox_test.go`, this report
- implementation commitSHA: `a03b2dba73773288b95840b164ab7146a1d41c11`

## Changed files

- `monitor/toolbox.go`: added v2 project-reference types/store, v1 global store, validation/normalization, atomic writer seam, and idempotent v1 migration.
- `monitor/toolbox_test.go`: added RED/GREEN schema, persistence, migration, retry, failure-preservation, validation-limit, legacy-write-guard, and ID-collision coverage.
- `.superpowers/sdd/2026-09-11-compact-monitor-and-global-toolbox/task-1-report.md`: this report.

## Verification evidence

Go runtime: `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go`; each test used a unique `TMPDIR` under `/dev/shm`.

1. RED (before implementation):

   `TMPDIR=<unique /dev/shm path> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(ProjectReferencesStore|GlobalToolboxStore|GlobalTodo)' -count=1`

   Outcome: exit 1; compile failed with the expected missing `ProjectReferences`, `GlobalToolbox`, `ToolboxTodo`, and store methods.

2. GREEN focused storage/migration:

   `TMPDIR=<unique /dev/shm path> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(ProjectReferencesStore|GlobalToolboxStore|GlobalTodo|ToolboxMigration)' -count=1`

   Outcome: exit 0.

3. GREEN affected existing Toolbox tests:

   `TMPDIR=<same unique /dev/shm path> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(ProjectToolbox|ValidateProject|AppProject|AppOpen)' -count=1`

   Outcome: exit 0.

4. Package compile/regression check:

   `TMPDIR=<unique /dev/shm path> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -count=1`

   Outcome: exit 0 (`ok thread-dock/monitor`).

5. Formatting/diff check:

   `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/gofmt -w monitor/toolbox.go monitor/toolbox_test.go`

   `git diff --check`

   Outcome: exit 0 for both.

## Self-review

- Project writes now emit only version-2 references; `putReferences` fails closed on a legacy version-1 file, so the transitional v1 writer cannot downgrade migrated data.
- Global reads/writes call `ensureMigrated`; migration validates the entire legacy/global input before any write, saves global data atomically first, then rewrites project references atomically.
- Retry migration is keyed by project-sorted semantic pairs. Commands dedupe by an unambiguous length-prefixed `(trimmed label, trimmed command)` key; Todos dedupe by `(project key, trimmed text)` and preserve incomplete state. Colliding IDs are deterministically re-keyed.
- Nil lists normalize to non-nil lists, common Todos omit `projectKey`, per-type limits and existing reference/command/Todo validation remain enforced, and writes use private test-only writer seams.
- Legacy `ProjectToolbox`/get/put remain solely for the untouched consumer and reject version-2 files through the existing version check.

## Unverified / blockers

- Runtime model/effort and native Windows/Wails smoke are unverified/not run.
- No user-profile or product data was read or migrated; all file fixtures use `t.TempDir()`.
- No blockers. Final review candidate SHA is to be pinned externally by the coordinator after this report commit; no main merge/push was performed.

