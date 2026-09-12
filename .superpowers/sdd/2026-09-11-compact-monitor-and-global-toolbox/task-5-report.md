# Task 5 결과 — compact-herdr-summary

## Packet

- taskId: `compact-herdr-summary`
- baseSHA: `2fc9f3f820b664845d7440dd08038b92334cfc81`
- deps: Task4 accepted; private `monitor-presentation.ts/test` preserved read-only
- worktree: `/home/appuser/dev_system/.worktrees/compact-herdr-summary`
- branch: `agent/compact-herdr-summary`
- ownedPaths: HerdrSummary component/test/CSS, App, ProjectDetail, monitor/styles tests/CSS, this report
- forbiddenPaths: monitor-presentation.ts/test, bindings, project-key, types, Task3 components, AppScope test, Go, Makefile, modules, real data, other worktrees

## Implementation

- implementationSHA: `500da557c5efcc8da48a5b785503462559448cf4`
- `HerdrSummary` renders a one-line collapsed `실행 상태` row with explicit `자세히`/`접기` controls and `aria-expanded`/`aria-controls`.
- Summary uses the unchanged `connectionsForWork` helper and aggregates snapshot plus selected-connection severity so later blocked/offline/stale/unverified/missing conditions remain visible even when an earlier connection is healthy.
- Expanded detail includes selected connection locations, observed sessions, notices, and unconnected agents; location details remain hidden while collapsed.
- App’s expanded standalone Herdr panel was removed. `ProjectDetail` renders the summary for selected work; App renders it independently when GitHub has no selected project so local Herdr status remains visible.
- Existing monitor tests now explicitly expand details before asserting locations/session evidence; matching and handoff behavior remain covered.

## Tests and evidence

- RED: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-herdr-red.Plv0bZ npm --prefix monitor/frontend test -- src/HerdrSummary.test.tsx`; log `/dev/shm/td-compact-herdr-red.Plv0bZ/herdr-red.log`, exit `1`, missing component import.
- GREEN: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-herdr-green.U3V3W3 npm --prefix monitor/frontend test -- src/HerdrSummary.test.tsx`; log `/dev/shm/td-compact-herdr-green.U3V3W3/herdr-green.log`, exit `0`, `1 file / 9 tests`.
- Integrated focused UI: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task5-ui.pXgTU4 npm --prefix monitor/frontend test -- src/HerdrSummary.test.tsx src/ProjectDetail.test.tsx src/monitor.test.tsx src/AppScope.test.tsx`; log `/dev/shm/td-compact-task5-ui.pXgTU4/herdr-focused.log`, exit `0`, `4 files / 47 tests`.
- TypeScript check: `cd monitor/frontend && NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-task5-tsc.o7WzWE ./node_modules/.bin/tsc --noEmit --project tsconfig.json`; log `/dev/shm/td-compact-task5-tsc.o7WzWE/tsc.log`, exit `0`.
- `git diff --check`: clean before commit.

## Result

- changedFiles:
  - `monitor/frontend/src/HerdrSummary.tsx`
  - `monitor/frontend/src/HerdrSummary.test.tsx`
  - `monitor/frontend/src/herdr-summary.css`
  - `monitor/frontend/src/App.tsx`
  - `monitor/frontend/src/ProjectDetail.tsx`
  - `monitor/frontend/src/monitor.test.tsx`
  - `monitor/frontend/src/styles.css`
  - this report
- outcomes: focused Herdr, ProjectDetail, monitor, and AppScope behavior passes; no Go suite, full frontend suite, build, or final gate repeated.
- unverified: runtime model/effort, Windows native behavior, final browser/native visual and integrated gate
- blockers: none
- prepared `monitor/frontend/node_modules` symlink is untracked and was not staged.

## Self-review

- No matching helper or wire/API file was modified.
- No session launch, Agent action, auto-expand, or color-only severe-state signaling was added.
- The selected work/project reset effect prevents stale expanded detail when the user changes context.
