# Task 1 implementation report — compact-toolbox-storage

## Packet

- taskId: `compact-toolbox-storage`
- baseSHA: `33d279ab59653a42a997b4f9c835c7857ce075dd`
- branch/worktree: `agent/compact-toolbox-storage` / `.worktrees/compact-toolbox-storage`
- ownedPaths: `monitor/toolbox.go`, `monitor/toolbox_test.go`, this report
- implementation commitSHA: `2fd25e3eaeb26532d809a1c9042b5b59cfe13f16` (includes the round-1 fix)

## Changed files

- `monitor/toolbox.go`: added v2 project-reference types/store, v1 global store, validation/normalization, atomic writer seam, and idempotent v1 migration. The round-1 fix makes legacy `put` merge existing global + legacy + caller data before global-first rewrite, validates existing global data before normal v2/empty replacement, and compacts duplicate existing entries.
- `monitor/toolbox_test.go`: added RED/GREEN schema, persistence, migration, retry, failure-preservation, validation-limit, legacy-write-guard, ID-collision, put-preservation, malformed/unsupported-global, duplicate-compaction, Todo conflict, and >200 Todo coverage.
- `.superpowers/sdd/2026-09-11-compact-monitor-and-global-toolbox/task-1-report.md`: this report.

## Verification evidence

Go runtime: `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go`; each test used a unique `TMPDIR` under `/dev/shm`. Logs below are the actual log paths from the round-1 fix verification.

1. RED (before implementation):

   `tmp=$(mktemp -d /dev/shm/td-compact-toolbox-red.XXXXXX); chmod 700 "$tmp"; TMPDIR="$tmp" /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(ProjectReferencesStore|GlobalToolboxStore|GlobalTodo)' -count=1`

   Outcome: exit 1; compile failed with the expected missing `ProjectReferences`, `GlobalToolbox`, `ToolboxTodo`, and store methods. The initial TMPDIR/log path was not recorded; no path is fabricated here.

6. Round-1 RED regression run at prior candidate `6e77d7939f09729abfe152fc3fc38f3491b4c414`:

   `tmp=$(mktemp -d /dev/shm/td-compact-toolbox-fix-red2.XXXXXX); chmod 700 "$tmp"; TMPDIR="$tmp" /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(GlobalToolboxPut|ToolboxMigration)' -count=1`

   Outcome: exit 1. It reproduced caller-only replacement, malformed-global overwrite for v2/absent projects, and uncompact existing command/Todo duplicates.

7. Round-1 GREEN exact focused migration command:

   `TMPDIR=/dev/shm/td-compact-toolbox-fix-final1.NlAkjL /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(ProjectReferencesStore|GlobalToolboxStore|GlobalTodo|ToolboxMigration)' -count=1`

   Log: `/dev/shm/td-compact-toolbox-fix-final1.NlAkjL/focused-migration.log`; status: `0`; output: `ok thread-dock/monitor 0.009s`.

8. Round-1 GREEN put/get regressions:

   `TMPDIR=/dev/shm/td-compact-toolbox-fix-final2.Wbtj7W /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(GlobalToolboxPut|GlobalToolboxGet)' -count=1`

   Log: `/dev/shm/td-compact-toolbox-fix-final2.Wbtj7W/put-get-regressions.log`; status: `0`; output: `ok thread-dock/monitor 0.005s`.

9. Round-1 GREEN affected existing Toolbox tests:

   `TMPDIR=/dev/shm/td-compact-toolbox-fix-final3.mrnjws /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test(ProjectToolbox|ValidateProject|AppProject|AppOpen)' -count=1`

   Log: `/dev/shm/td-compact-toolbox-fix-final3.mrnjws/existing-toolbox.log`; status: `0`; output: `ok thread-dock/monitor 0.004s`.

5. Formatting/diff check:

   `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/gofmt -w monitor/toolbox.go monitor/toolbox_test.go`

   `git diff --check`

   Outcome: exit 0 for both.

## Self-review

- Project writes now emit only version-2 references; `putReferences` fails closed on a legacy version-1 file, so the transitional v1 writer cannot downgrade migrated data.
- Global reads/writes are migration-aware; migration validates the entire legacy/global input before any write, saves global data atomically first, then rewrites project references atomically. Legacy `put` merges existing global + sorted legacy + caller before the first global save and returns the merged value; v2/empty `put` validates an existing global file before full replacement.
- Canonical merge uses fresh output slices, so existing global duplicates are removed. Commands dedupe by an unambiguous length-prefixed `(trimmed label, trimmed command)` key; Todos dedupe by `(project key, trimmed text)` and preserve incomplete state with logical AND. Colliding IDs are deterministically re-keyed.
- Nil lists normalize to non-nil lists, common Todos omit `projectKey`, per-type limits and existing reference/command/Todo validation remain enforced, and writes use private test-only writer seams.
- Legacy `ProjectToolbox`/get/put remain solely for the untouched consumer and reject version-2 files through the existing version check.

## Unverified / blockers

- Runtime model/effort and native Windows/Wails smoke are unverified/not run.
- No user-profile or product data was read or migrated; all file fixtures use `t.TempDir()`.
- No blockers. The post-report final head SHA will be sent externally to the coordinator after this report commit; no main merge/push was performed.
