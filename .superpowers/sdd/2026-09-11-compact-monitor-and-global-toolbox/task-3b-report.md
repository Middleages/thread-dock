# Task 3b 결과 — compact-global-toolbox-drawer

## Packet

- taskId: `compact-global-toolbox-drawer`
- baseSHA: `894f51229b9ec897592462c3ee9b5c6d42eb6042`
- deps: Task2 accepted/integrated; Task3a opening contract 확인 후 진행
- worktree: `/home/appuser/dev_system/.worktrees/compact-global-toolbox-drawer`
- branch: `agent/compact-global-toolbox-drawer`
- ownedPaths: `GlobalToolboxDrawer.tsx`, `GlobalToolboxDrawer.test.tsx`, `global-toolbox-drawer.css`, this report
- forbiddenPaths: App, bindings, project-key, shared CSS/types, Go, Makefile, modules, old `ProjectToolbox`, other worktrees, real data

## Interface and implementation

- `GlobalToolboxDrawer`는 packet의 controlled props를 그대로 사용한다.
- Drawer 자체는 global toolbox binding이나 파일 저장을 호출하지 않고, parent의 `toolbox`, `onSave`, `onReload`를 통해서만 상태를 관찰·저장한다.
- 명령어는 추가·삭제·복사만 제공하며 실행/Agent 호출은 없다.
- Todo는 공통 또는 단일 `projectKey`만 저장하고, `project:${key}` tagged filter로 실제 key가 `all`·`common`인 경우도 충돌하지 않게 했다.
- 미완료 기본 필터, 완료 포함 토글, 프로젝트 dedupe, 알 수 없는 project key 보존·재할당, initial project shortcut을 구현했다.
- modal role/aria-modal, close/backdrop/Escape, focus 진입·trap·복원 및 listener cleanup을 구현했다.
- 네이티브 Wails ClipboardSetText를 먼저 사용하고 브라우저 clipboard를 fallback으로 사용한다.
- Drawer CSS의 `.eyebrow`와 `.danger-lite`를 포함해 모든 스타일을 Drawer 하위에 scope했다.

## Result

- changedFiles:
  - `monitor/frontend/src/GlobalToolboxDrawer.tsx`
  - `monitor/frontend/src/GlobalToolboxDrawer.test.tsx`
  - `monitor/frontend/src/global-toolbox-drawer.css`
  - `.superpowers/sdd/2026-09-11-compact-monitor-and-global-toolbox/task-3b-report.md`
- implementationSHA: `05cefa558db0893fb133a0e19466b8b695b0f7b0`
- reportCommitSHA: `c9068f30be7d8950336765369b06ceb737f0f204`
- executedCommands:
  - `git diff --check` — implementation diff clean
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-drawer-test.LeUyOd npm --prefix monitor/frontend test -- src/GlobalToolboxDrawer.test.tsx` — log `/dev/shm/td-compact-drawer-test.LeUyOd/focused.log`, exit 0
  - `cd monitor/frontend && NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-drawer-tsc.DnSxBt ./node_modules/.bin/tsc --noEmit --project tsconfig.json` — log `/dev/shm/td-compact-drawer-tsc.DnSxBt/tsc.log`, exit 0
- outcomes:
  - focused Drawer test: `1 file passed`, `12 tests passed`
  - TypeScript project check: exit 0
  - final self-review: changed paths are limited to owned Drawer files and report; no old component, App, binding, shared type, or config changes
- unverified:
  - actual runtime model/effort identity
  - Wails native clipboard result and Windows rendering
  - Task4 parent cache/load/save integration
  - desktop/narrow browser fixture and final visual review
  - pure component-missing RED invocation: the first recorded RED was the initial focused run after component creation (6 behavior assertions failed); no missing-module RED was claimed or fabricated
- blockers: none

## Self-review notes

- Save failures leave controlled data and all draft inputs intact; only successful saves clear the relevant draft.
- `toolbox === null`, load error, loading, saving, and duplicate in-flight save states disable mutations.
- Unknown Todo keys remain in the global list and are offered as an explicit unknown option until reassigned.
- Existing `ProjectToolbox` remains untouched for Task4’s staged retirement.

## Review fix r1 — `compact-global-toolbox-drawer`

- review base: `c9068f30be7d8950336765369b06ceb737f0f204`
- fix commit: `27dc8d9b457f9f00a9dc744102c15f824df6e935`
- changedFiles:
  - `monitor/frontend/src/GlobalToolboxDrawer.tsx`
  - `monitor/frontend/src/GlobalToolboxDrawer.test.tsx`
  - `monitor/frontend/src/global-toolbox-drawer.css`
  - this report
- findings addressed:
  - Todo checkbox updates now preserve `projectKey`; only an explicit common reassignment removes the property.
  - Added focused coverage for linked and unknown-key checkbox updates, all/common/project filters, exact project-linked creation, command/Todo deletion, real `all`/`common` project-key collisions, unknown-to-common reassignment, native clipboard false/reject behavior, saving and duplicate in-flight guards.
  - Added scoped 3px `var(--focus-ring)` / 3px offset focus-visible styling for Drawer input/select/textarea.
  - Focus restoration now runs on controlled `open=false` and unmount cleanup, with a nulling ref to avoid double restoration or stable-open focus jumps.
- executedCommands:
  - `git diff --check` — clean before commit
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-drawer-fix-red.948MZa npm --prefix monitor/frontend test -- src/GlobalToolboxDrawer.test.tsx` — log `/dev/shm/td-compact-drawer-fix-red.948MZa/focused-red.log`, exit `1`; 3 expected regression failures before the fix
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-drawer-fix-green.PcQWPr npm --prefix monitor/frontend test -- src/GlobalToolboxDrawer.test.tsx` — log `/dev/shm/td-compact-drawer-fix-green.PcQWPr/focused-green.log`, exit `0`
  - `cd monitor/frontend && NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-drawer-fix-tsc.c34XMg ./node_modules/.bin/tsc --noEmit --project tsconfig.json` — log `/dev/shm/td-compact-drawer-fix-tsc.c34XMg/tsc.log`, exit `0`
- outcomes:
  - focused Drawer test: `1 file passed`, `21 tests passed`
  - TypeScript project check: exit `0`
  - final diff check clean; only owned Drawer source/test/CSS files changed
- unverified:
  - actual runtime model/effort identity
  - Wails native clipboard and Windows rendering
  - parent Task4 cache integration and final desktop/narrow browser review
  - the initial no-module test attempt exited `127` before the prepared frontend symlink was recreated; it is environment setup evidence, not a product result
- blockers: none
