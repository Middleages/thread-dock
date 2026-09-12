# Task 3a report — ProjectReferences

- taskId: `compact-project-references`
- baseSHA: `894f51229b9ec897592462c3ee9b5c6d42eb6042`
- implementationSHA: coordinator records the final candidate commit SHA
- worktree: `/home/appuser/dev_system/.worktrees/compact-project-references`
- branch: `agent/compact-project-references`

## Changed files

- `monitor/frontend/index.html` — committed first as the pinned compact-monitor direction contract (`9f248274976e2c6fa1675f8c3690a15f51d07a89`).
- `monitor/frontend/src/ProjectReferences.tsx` — project-only reference loading, guarded save/add/delete, safe web open, Wails file/WSL open, native-first clipboard, loading/error/retry states, and project-switch generation guards.
- `monitor/frontend/src/ProjectReferences.test.tsx` — focused component coverage for loading, reference-only rendering, safe open/copy, add/delete, load retry, failed-save preservation, and stale load/save responses.
- `monitor/frontend/src/project-references.css` — compact reference list/form using existing tokens with narrow-window wrapping.
- `.superpowers/sdd/2026-09-11-compact-monitor-and-global-toolbox/task-3a-report.md` — this report.

## Executed commands and outcomes

- RED: `NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/threaddock-compact-references-red.2mtoZX VITEST_MAX_WORKERS=1 npm --prefix monitor/frontend test -- src/ProjectReferences.test.tsx`; expected failure because `ProjectReferences` was absent; Vitest reported `1 failed suite / 0 tests` after 77.26s. The shell wrapper's exit line was not retained, so the exact shell exit is unverified (process completed with the expected test failure).
- GREEN: `NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/threaddock-compact-references-green3.IZL2nk VITEST_MAX_WORKERS=1 npm --prefix monitor/frontend test -- src/ProjectReferences.test.tsx`; `8 tests passed`, exit `0`, duration `2.18s`.
- Type check: `./node_modules/.bin/tsc --noEmit` from `monitor/frontend`; exit `0`.
- `git diff --check` against the opening-contract commit; no whitespace errors.

## Self-review

- No command execution or browser/HTTP fallback was added. Web opening goes through the existing safe HTTP(S) helper; Windows and WSL paths use `openToolboxReference`.
- Mutating controls are unavailable until a successful load and disabled while saving. Failed saves leave the last confirmed list and draft inputs intact.
- A monotonically increasing project generation prevents old load/save responses from changing the newly selected project's list, error, or loading state.
- The prepared `monitor/frontend/node_modules` symlink is untracked and was not staged; its target was not changed or removed.

## Unverified / blockers

- Runtime model/effort identity is unavailable.
- Windows native Wails behavior, integrated desktop layout, and narrow visual rendering remain for the integration/browser/Windows verification tasks.
- No blockers found within Task 3a.

## Review fix r1 — `compact-project-references-fix-r1`

- review base: `473b55a8567cdcdd90cc3789a0274b1a8fcaaf27`
- fix commit: `55c95fa15b0b8a8c3aa1a7f50f69015fb1b950ee`
- reassignment reason: the fresh references review required explicit false/reject native clipboard regression coverage, exact Windows/WSL type-target save coverage, and scoped input/select focus-visible styling. The original references agent was no longer available for follow-up, so this narrow test/CSS-only fix was reassigned serially.
- changedFiles:
  - `monitor/frontend/src/ProjectReferences.test.tsx`
  - `monitor/frontend/src/project-references.css`
  - this report
- executedCommands:
  - `git diff --check` — clean before the fix commit
  - `VITEST_MAX_WORKERS=1 NODE_COMPILE_CACHE=/dev/shm/threaddock-compact-nodecache.dJMMfd TMPDIR=/dev/shm/td-compact-references-fix.cOh9K0 npm --prefix monitor/frontend test -- src/ProjectReferences.test.tsx` — log `/dev/shm/td-compact-references-fix.cOh9K0/focused.log`, exit `0`
- outcomes:
  - focused ProjectReferences test: `1 file passed`, `11 tests passed`
  - native ClipboardSetText returning `false` or rejecting never calls browser clipboard and reports the copy error
  - Windows `file` and WSL `wsl-file` add payloads preserve exact type and target
  - project reference inputs, select, and buttons have scoped `3px var(--focus-ring)` / `3px` offset focus-visible styling
- tests: coverage-only fix; no artificial RED was run. Existing trusted type-check evidence remains from the original implementation; no full suite/build was repeated.
- unverified: runtime model/effort, native Windows clipboard and rendering, integrated visual review
- blockers: none
