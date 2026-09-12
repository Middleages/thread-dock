# Task 2 implementation report — compact-toolbox-bindings

## Packet

- taskId: `compact-toolbox-bindings`
- baseSHA: `5dc0abb5cc473dc80512ecec0d21219f25942473`
- deps: Task 1 fixed storage candidate `b3b9919`, integrated at base SHA
- branch/worktree: `agent/compact-toolbox-bindings` / `.worktrees/compact-toolbox-bindings`
- ownedPaths: `monitor/app.go`, `monitor/app_test.go`, frontend bindings/project-key files, App helper extraction, this report

## Changed files

Eight files are changed, including this report:

- `monitor/app.go`: added independent project/global stores, `toolboxMu`, migration-aware Wails methods, and serialized transitional old methods. Both constructors initialize both stores; the old `toolbox` field remains only as a compatibility alias for Task 1's transitional tests/consumer.
- `monitor/app_test.go`: added temp-directory App boundary tests for references-only persistence, validate-before-migration, first global save preservation, v1 first reference-save migration, and malformed/unsupported-global error/byte preservation.
- `monitor/frontend/src/bindings.ts`: added `ProjectReferences`, `ToolboxTodo`, `GlobalToolbox`, and exact four Wails binding functions. Old ProjectToolbox bindings remain until Task 4.
- `monitor/frontend/src/bindings.test.ts`: added missing-binding/no-fetch and exact Wails method/argument coverage for all four methods.
- `monitor/frontend/src/project-key.ts`: moved the existing URL-hostname/project-id key derivation helper.
- `monitor/frontend/src/project-key.test.ts`: added GitHub, GHES, and project-id fallback coverage.
- `monitor/frontend/src/App.tsx`: imports the extracted helper; no output behavior was changed.
- `.superpowers/sdd/2026-09-11-compact-monitor-and-global-toolbox/task-2-report.md`: this report.

## Verification evidence

Actual runtimes:

- Go: `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go` and matching `gofmt`.
- Node: `v26.8.1`; npm: `11.19.0`.
- Frontend test cache: `/dev/shm/threaddock-compact-nodecache.dJMMfd`.
- Frontend test temp directory: `/dev/shm/threaddock-compact-bindings-tmp`.
- `monitor/frontend/node_modules` was an existing untracked prepared dependency symlink and was not staged or changed.

RED:

1. `NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/threaddock-compact-bindings-tmp VITEST_MAX_WORKERS=1 npm --prefix monitor/frontend test -- src/bindings.test.ts src/project-key.test.ts`

   Exit `1` (Vitest session `78819`). The two binding tests failed because the new exports were absent, and the project-key suite failed to resolve the absent `./project-key` module. This was the expected pre-implementation failure.

GREEN:

1. `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/gofmt -w monitor/app.go monitor/app_test.go`

   Exit `0`.

2. `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run 'Test.*Toolbox|Test.*ProjectReferences' -count=1`

   Exit `0`; package passed (`ok thread-dock/monitor 0.044s`). This included the Task 1 storage/migration tests and new App boundary tests.

3. `NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/threaddock-compact-bindings-tmp VITEST_MAX_WORKERS=1 npm --prefix monitor/frontend test -- src/bindings.test.ts src/project-key.test.ts`

   Exit `0`; `2` test files and `5` tests passed. No fetch fallback was observed.

4. From `monitor/frontend`: `NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/threaddock-compact-bindings-tmp npx tsc -b --pretty false`

   Exit `0`.

5. From `monitor/frontend`: `NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/threaddock-compact-bindings-tmp VITEST_MAX_WORKERS=1 npm test -- src/AppScope.test.tsx`

   Exit `0`; `1` test file and `4` tests passed. The first attempt to run TypeScript from the worktree root (`npx --prefix monitor/frontend tsc -b --pretty false`) exited `1` because that command searched for a root `/home/appuser/dev_system/.worktrees/compact-toolbox-bindings/tsconfig.json`; the corrected command above passed from `monitor/frontend`.

6. `git diff --check`

   Exit `0`.

Round-1 review coverage fix (base `57bb3b29780cce2ac5e9f041229f1ca3adc726fc`):

7. `bash -o pipefail -c '/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./monitor -run "TestApp.*(Toolbox|ProjectReferences)" -count=1 2>&1 | tee /dev/shm/td-compact-toolbox-bindings-fix.ZEdOb6/app-focused.log; status=${PIPESTATUS[0]}; printf "%s\\n" "$status" > /dev/shm/td-compact-toolbox-bindings-fix.ZEdOb6/app-focused.exit; exit "$status"'`

   Exit `0`; log: `/dev/shm/td-compact-toolbox-bindings-fix.ZEdOb6/app-focused.log`; exit record: `/dev/shm/td-compact-toolbox-bindings-fix.ZEdOb6/app-focused.exit` contains `0`; output was `ok thread-dock/monitor 0.027s`. This ran only the affected App Toolbox/reference tests after adding the reviewer-requested coverage.

No repository-wide suite, browser smoke, package install, or `make check` was run.

## Self-review

- `toolboxMu` is independent of the controller/settings lock and guards new reference/global calls plus transitional old get/save calls.
- Reference key/payload validation happens before `ensureMigrated`; references then use the Task 1 store. Global get delegates to migration-aware `get`, while global save delegates directly to migration-aware `put` under the mutex, preserving legacy, existing-global, and caller entries on the first v1 save.
- `GetProjectReferences`/`SaveProjectReferences` do not read the global file for already-v2/absent project files because Task 1's `ensureMigrated` returns before global access; legacy paths migrate before reference access.
- Both constructors initialize both stores. Tests only use `t.TempDir()` stores and do not call default-profile persistence.
- App boundary coverage now proves a valid first `SaveProjectReferences` migrates legacy commands/checklist data before replacing only the selected project's references, preserves other project references, and links migrated Todo data to its origin key. Table-driven App tests prove malformed/unsupported global files propagate through global get/save and migration-required reference save without changing either file's original bytes.
- The extracted `toolboxKeyFor` preserves valid project/work URL hostname plus project ID behavior and project-ID fallback. Existing App rendering was not otherwise changed.
- Old Wails/TypeScript ProjectToolbox contracts remain intentionally for the pre-Task-4 consumer. The compatibility `toolbox` field is retained only so that transitional Task 1 tests/old consumer can still inject a project store; production constructors use the new two-store fields.

## Unverified / blockers

- Runtime model/effort and native Windows/Wails smoke are unverified/not run.
- No user profile or product data was read or migrated; all new Go fixtures use `t.TempDir()`.
- No blockers.

Initial implementation commit SHA: `e6f5482367abcb3ac1d564689a9a5f9245a8e2c3`; previous amended candidate: `57bb3b29780cce2ac5e9f041229f1ca3adc726fc`. Review-fix commit SHA is supplied in the coordinator handoff after this commit; no main merge or push was performed.
