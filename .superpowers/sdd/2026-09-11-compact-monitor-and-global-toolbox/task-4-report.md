# Task 4 결과 — compact-monitor-integration

## Packet

- taskId: `compact-monitor-integration`
- baseSHA: `153c196054c27f0d0addbcd829e9e9985b105992`
- deps: Task3a/Task3b accepted and integrated
- worktree: `/home/appuser/dev_system/.worktrees/compact-monitor-integration`
- branch: `agent/compact-monitor-integration`
- ownedPaths: TopBar, ProjectDetail, SettingsShell, App, main/styles/settings, presentation helper/tests, bindings, old ProjectToolbox files, monitor App/toolbox files/tests, this report
- forbiddenPaths: Task3 component files/tests/CSS, shared `types.ts`/`project-key.ts`/`tokens.css`, Go modules, Makefile, real data, other worktrees

## Implementation

- implementationSHA: `1481e0dda7785cb0695e0b9ea4a3233cc41559ba`
- App now owns one global `GlobalToolbox | null` cache, initial StrictMode-safe load, explicit/reopen-after-error retry, save serialization, stale load/save revision protection, and project Todo counts.
- TopBar provides ThreadDock identity, connection status, Toolbox, and Settings actions.
- ProjectDetail owns the selected Issue/PR identity, safe current work link, exact handoff clipboard behavior, `업무 / 자료` tabs, ProjectReferences, and positive incomplete Todo shortcut.
- SettingsShell opens through App’s controlled callback; floating launcher was removed without changing settings payload/load/save semantics.
- Herdr panel remains expanded in App and retains issue → trusted project → exact coordinator repository matching semantics.
- Old ProjectToolbox UI files and old TS/Wails API contracts were removed. Private Go v1 decoder types remain only for migration; private legacy store get/put/saveFile and old Go bindings were removed.
- Shell CSS is single-column centered max1440 with no permanent side rails.

## Tests and evidence

- RED: `/dev/shm/td-compact-task4-red.NGwzAS/focused-red.log`, exit `1`; missing new component modules plus the new App cache assertions failed against the pre-implementation tree.
- Component focused GREEN: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-components.vFhqGp npm --prefix monitor/frontend test -- src/TopBar.test.tsx src/ProjectDetail.test.tsx src/SettingsShell.test.tsx src/monitor-presentation.test.ts`; log `/dev/shm/td-compact-task4-components.vFhqGp/components.log`, exit `0`, `4 files / 10 tests`.
- Go focused GREEN: `PATH=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin:$PATH TMPDIR=/dev/shm/td-compact-task4-go.ohjp4i go test ./monitor`; log `/dev/shm/td-compact-task4-go.ohjp4i/monitor-go.log`, exit `0`.
- Affected UI GREEN: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-ui.L3to6B npm --prefix monitor/frontend test -- src/TopBar.test.tsx src/ProjectDetail.test.tsx src/SettingsShell.test.tsx src/monitor-presentation.test.ts src/AppScope.test.tsx src/monitor.test.tsx src/bindings.test.ts`; log `/dev/shm/td-compact-task4-ui.L3to6B/affected-ui.log`, exit `0`, `7 files / 42 tests`.
- AppScope save-failure regression: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-appscope-save.tu353U npm --prefix monitor/frontend test -- src/AppScope.test.tsx`; log `/dev/shm/td-compact-task4-appscope-save.tu353U/appscope.log`, exit `0`, `1 file / 7 tests`.
- TypeScript check: `cd monitor/frontend && NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-tsc.GokYxv ./node_modules/.bin/tsc --noEmit --project tsconfig.json`; log `/dev/shm/td-compact-task4-tsc.GokYxv/tsc.log`, exit `0`.
- `git diff --check`: clean before implementation commit.
- Static import proof: no frontend/App old ProjectToolbox or old Wails API references remain; remaining `ProjectToolbox` symbols are private migration decoder/test fixtures only.

## Result

- changedFiles: implementation commit changed 22 owned files; report is added in the following report commit.
- outcomes: required Task4 focused behavior passes; no worker full suite, Vite full build, or `make check` was run.
- unverified:
  - actual runtime model/effort identity
  - Windows Wails native clipboard, package/build, and child-console smoke
  - final desktop/narrow browser fixture review
  - final integrated `make check`
- blockers: none
- prepared `monitor/frontend/node_modules` symlink is untracked and was not staged.

## Self-review

- StrictMode initial Toolbox load is guarded by a persistent started ref; accepted AppScope test proves one binding call.
- Load/save operation revisions prevent stale responses from replacing a newer successful cache.
- Failed global save leaves the committed command and draft inputs visible; accepted AppScope regression covers this.
- No command execution, Agent injection, browser fetch, new Wails API, or shared wire type was added.

## Review fix r1 — `compact-monitor-integration`

- review base: `86a0ad4d3860cfb8e49f375d6ed6e322f1c6fc30`
- fix candidate: `b33dd8bae8f28fb28d9f9504d5e2d19f747d1882`
- findings addressed:
  - Restored the original separate `Project 필드` section and per-field evidence rows in `ProjectDetail`; the exact `In progress` assertion is retained.
  - Removed unused `copyText`, `HerdrConnections`, and their imports from `ProjectDetail`; Herdr rendering remains in App for Task5.
  - Restored the exact migration validator regression `TestValidateProjectToolboxRejectsExecutableOrRelativeReferences`.
  - Added App-level shortcut/filter, successful-save cache/count, normal-entry reset, deferred duplicate-save, and committed-command reopen coverage. The existing Drawer lifecycle already resets normal entry after controlled close/reopen, so no App production change was needed for that finding.
- executedCommands:
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-fix-red.A1eIo2 npm --prefix monitor/frontend test -- src/AppScope.test.tsx` — log `/dev/shm/td-compact-task4-fix-red.A1eIo2/appscope-red.log`, exit `1`; an absence assertion incorrectly used getByRole and threw before .not.toBeInTheDocument(). Correcting it to queryByRole fixed the test-construction error, not a product regression. The new App behavior tests are coverage for already-correct production behavior.
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-fix-green.pgGYxf npm --prefix monitor/frontend test -- src/AppScope.test.tsx` — log `/dev/shm/td-compact-task4-fix-green.pgGYxf/appscope-green.log`, exit `0`, `1 file / 10 tests`
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-fix-ui.C2IV85 npm --prefix monitor/frontend test -- src/ProjectDetail.test.tsx src/monitor.test.tsx` — log `/dev/shm/td-compact-task4-fix-ui.C2IV85/focused-ui.log`, exit `0`, `2 files / 28 tests`
  - `PATH=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin:$PATH TMPDIR=/dev/shm/td-compact-task4-fix-go.675M97 go test ./monitor -run '^TestValidateProjectToolboxRejectsExecutableOrRelativeReferences$' -count=1` — log `/dev/shm/td-compact-task4-fix-go.675M97/validator.log`, exit `0`
  - `cd monitor/frontend && NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task4-fix-tsc.gEpQLl ./node_modules/.bin/tsc --noEmit --project tsconfig.json` — log `/dev/shm/td-compact-task4-fix-tsc.gEpQLl/tsc.log`, exit `0`
- outcomes: exact Project field presentation, migration validator, App cache/count/save/reset regressions all pass; no full suite/build/make check repeated.
- unverified: runtime model/effort, Windows native behavior, final browser/native visual and integrated gate
- blockers: none
