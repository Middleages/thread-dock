# Task6 visual fix 결과 — compact-visual-fix

## Packet

- taskId: `compact-visual-fix`
- baseSHA: `516a40a4adcc700cdf3477bc7a8e35be927eea4f`
- worktree: `/home/appuser/dev_system/.worktrees/compact-visual-fix`
- branch: `agent/compact-visual-fix`
- implementationSHA: `1ee56093c741196534ef45c61436913167f038b7`

## Changes

- Scoped `.monitor-toolbar .secondary-action` width/margin rules prevent the generic full-width secondary action cascade from stretching the refresh control; scope actions stay nowrap and toolbar wraps safely.
- Removed only the new `GLOBAL TOOLBOX` eyebrow; retained the existing `빠른 작업` heading.
- Replaced Drawer Unicode close glyph with a two-stroke inline SVG while retaining `aria-label="닫기"` and keyboard behavior.
- Relaxed brand and h1 letter spacing from `-.045em` to `-.04em`.
- Removed `ProjectList` initial `autoFocus`, preserving selected `aria-pressed` state and global focus-visible styling.
- Added a deferred-snapshot AppScope regression proving that an already-open Drawer keeps focus when the first project row mounts.

## Verification

- RED: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-visual-red.6a0thP npm --prefix monitor/frontend test -- src/AppScope.test.tsx`; log `/dev/shm/td-compact-visual-red.6a0thP/appscope-red.log`, exit `1`; focus-steal regression failed as expected. The same invocation also had three unrelated existing AppScope timeout failures under the slow test environment; those are not claimed as product regressions.
- GREEN focus regression: `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-visual-focus.v6F53o npm --prefix monitor/frontend test -- src/AppScope.test.tsx --testNamePattern='does not let the first project row steal focus'`; log `/dev/shm/td-compact-visual-focus.v6F53o/focus.log`, exit `0`, `1 passed / 10 skipped`.
- Full AppScope rerun after visual changes was attempted with the requested focused command; log `/dev/shm/td-compact-visual-green.plzzVs/appscope-green.log`, exit `1` because three pre-existing global-toolbox cases timed out. No full-suite/build retry was performed.
- TypeScript: `cd monitor/frontend && NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-visual-tsc.55uhqX ./node_modules/.bin/tsc --noEmit --project tsconfig.json`; log `/dev/shm/td-compact-visual-tsc.55uhqX/tsc.log`, exit `0`.
- `git diff --check`: exit `0`.
- Static ownership checks: requested visual selectors/markup present; `GLOBAL TOOLBOX`, `.eyebrow`, and `autoFocus` absent from owned surfaces.

## Result

- changedFiles:
  - `monitor/frontend/src/styles.css`
  - `monitor/frontend/src/top-bar.css`
  - `monitor/frontend/src/global-toolbox-drawer.css`
  - `monitor/frontend/src/GlobalToolboxDrawer.tsx`
  - `monitor/frontend/src/App.tsx`
  - `monitor/frontend/src/AppScope.test.tsx`
  - this report
- outcomes: requested visual/focus fixes implemented; isolated focus regression and TypeScript check pass.
- unverified: runtime identity/native Windows, root recapture screenshots, final independent visual score, integrated gate
- blockers: no product blocker; full AppScope command retains an environment timeout tuple recorded above
- prepared `monitor/frontend/node_modules` symlink is untracked and was not staged.
