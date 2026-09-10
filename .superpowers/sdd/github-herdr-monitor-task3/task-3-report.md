# Task 3 report

status: DONE_WITH_CONCERNS

## changedFiles

- `Makefile`
- `README.md`
- `HANDOFF.md`
- `docs/operator/github-first-quickstart.md`
- `monitor/frontend/vite.config.ts`
- `monitor/frontend/src/App.tsx`
- `monitor/frontend/src/styles.css`
- `monitor/frontend/src/monitor.test.tsx`
- deleted every file under `monitor/frontend/server/`
- `task-3-report.md`

`monitor/frontend/src/bindings.ts`, `safe-url.ts`, `package.json`, `package-lock.json`, shared types, and all Go files were preserved.

## commitSHA

66158d9c8f6883434f7271e5c5f64e31ba0221b (last reviewed product/dependency implementation SHA; this Windows evidence update is a docs-only follow-up).

## executedCommands

- `npm exec vitest run src/monitor.test.tsx --reporter=verbose` — exit 1, RED evidence: 18 tests with 3 expected failures for legacy sections, duplicate Herdr location, and nonfunctional navigation.
- `npm exec vitest run src/bindings.test.ts src/monitor.test.tsx --reporter=verbose` — exit 0, 2 files and 20 tests passed.
- `npm run build` — exit 0, TypeScript compilation and Vite production build passed.
- `make -n check` — exit 0; planned gate includes all-Go formatting/vet/tests, shell syntax, focused UI tests, and frontend build; the actual `make check` was not run per task policy.
- `bash -n scripts/single-run-pilot.sh` — exit 0.
- `git diff --check` — exit 0.
- `find monitor/frontend/server -type f -print` — no files; Node adapter/server tests are deleted.
- `go test ./monitor -run 'Test(GitHub|Commands|App|Herdr|Snapshot)'` — upstream dependency evidence recorded in `progress.md` as exit 0; not rerun because this Task changes no Go source/test.

## TDD red/green evidence

The UI behavior tests were changed first to require removal of legacy task/publication sections, automation panel, nonfunctional top-menu buttons, and duplicate selected-work Herdr location. The focused monitor test run failed with those three expected failures. The minimal App/CSS/Vite changes were then applied, and the focused bindings + monitor run passed 20/20.

Deletion/config changes were additionally validated by the empty server directory, source-reference scan, TypeScript/Vite build, Makefile dry-run, and shell/diff checks.

## self-review findings and corrections

- Removed Node server imports, middleware, endpoint path, and Vite test exclusion while retaining React/Vite build tooling.
- Removed old task/publication and automation presentation, and kept decisions, handoff, GitHub evidence, notices, degraded states, safe links, clipboard handoff, observed sessions, and unconnected Agents.
- Moved selected-work Herdr presentation to the single Herdr panel; the handoff still receives the selected connection.
- Updated the three requested documents to separate Linux fixture/UI/build evidence, managed-pane live context, and unverified Windows evidence. No GitHub or Wiki write was claimed.
- Confirmed no forbidden worktree or Go/shared-interface file was changed. Re-added the tracked frontend `dist/.placeholder` after the build generated assets.

## unverified

- Packaged live GitHub/Herdr UI after the successful standard Wails build.
- Completion of the requested Windows scheduled-task/process cleanup.

## blockers

Standard Wails packaging is verified, but the repeated Windows→WSL interop `UtilAcceptVsock:281: accept4 failed 110` leaves packaged live acceptance and requested cleanup confirmation pending.

## Round 1 reviewer fix

- Updated `README.md`, `HANDOFF.md`, and `docs/operator/github-first-quickstart.md` with the inherited managed-pane evidence: `gh auth status` succeeded on the actual Windows→WSL workstation; bare `wsl.exe --exec herdr` failed PATH lookup; absolute `/home/appuser/.local/bin/herdr` status and agent list succeeded. The documents explicitly identify this as externally provided evidence without the original transcript and state that it is not Windows→WSL Monitor read-path verification.
- Historical Round 1 wording recorded the then-observed root-owned Windows toolchain as Go `go1.27.0 windows/amd64` and Wails `v2.10.2`; the later reviewed `v2.15.0` toolchain/build evidence is recorded in the Windows evidence docs follow-up below.
- Round 1 docs commit SHA: `c0d3ffb`; validation was docs parse/link/diff only. The report wording is finalized in the follow-up report commit.

## Round 2 reviewer fix

- Narrowed the README, HANDOFF, quickstart, and `unverified` report wording so the inherited transcript-free component observations remain evidence: `gh auth status` success, bare `wsl.exe --exec herdr` PATH failure, and absolute Herdr status/agent-list success.
- The remaining unverified scope is exactly Windows Wails build/app execution plus the Wails Monitor process's Windows→WSL read path/live desktop integration.
- Docs-only validation: relative-link parse and `git diff --check` passed; no product code or tests were changed or rerun.

## Integration gate fix

- Added the minimal README link `docs/operator/opencode-role-agents.md` required by `internal/pilot.TestReadmeLinksOpenCodeRoleAgentRunbook`; this preserves the current Go/Wails product state and does not revive legacy runtime or UI claims.
- Focused command `go test ./internal/pilot -run TestReadmeLinksOpenCodeRoleAgentRunbook` — exit 127: `go: command not found`. No supported Go executable was present at the checked local candidates, so this test result is unverified in the current environment and must not be reported as passing.
- Docs relative-link parse and `git diff --check` — exit 0.
- Self-review: only `README.md` and this report changed; the requested runbook link resolves, and no product code, tests, UI, or shared interface was modified.
- Initial blocker/concern: the focused Go test required an environment with Go available; resolved by the validation follow-up below.
- Validation follow-up at current SHA `613fcb5`: `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./internal/pilot -run TestReadmeLinksOpenCodeRoleAgentRunbook` — exit 0, `ok thread-dock/internal/pilot 0.003s`.
- Follow-up `git diff --check` — exit 0. The prior missing-Go concern is resolved for this focused test; no product code or tests changed.

## Makefile UI gate fix

- RED evidence from the tracked `make check` at `ef450f9`: the prior `npm --prefix monitor/frontend exec -- vitest ...` command ran with repository-root config cwd and produced 18/20 `window/document is not defined` failures; the same focused files pass from frontend context.
- Changed only the Makefile UI invocation in `test` and `check` to `npm --prefix monitor/frontend test -- src/bindings.test.ts src/monitor.test.tsx`, using the existing frontend package script and Vite/Vitest config while preserving the exact focused files.
- GREEN affected command: `npm --prefix monitor/frontend test -- src/bindings.test.ts src/monitor.test.tsx` — exit 0, 2 files and 20 tests passed.
- `make -n check` — exit 0 and shows the corrected UI command plus frontend build; `git diff --check` — exit 0. `make check`, full Go suite, and unrelated UI commands were not rerun.
- Self-review: only the two Makefile UI command lines changed; Go checks, formatting scope, frontend build command, and deleted-server exclusions remain unchanged.

## Final whole-branch review fix wave

- Historical final whole-branch review fix wave at `aeca770` updated `HANDOFF.md` so the then-current Linux integration gate was recorded and no fresh task review or full gate was requested. The later dependency alignment at `66158d9` supersedes that code/config evidence; the remaining acceptance is packaged live behavior and cleanup confirmation.
- Restored the tracked `monitor/frontend/dist/.placeholder` without adding generated build assets.
- Docs relative-link parse — exit 0; `git diff --check` — exit 0; `git status --short` — exit 0 with only the intended HANDOFF/report edits before commit. No product tests or `make check` were rerun.

## Windows evidence docs follow-up

This docs-only follow-up is based on the reviewed dependency SHA `66158d9`; it does not change
product code, tests, Makefile, Wails config, or the Go/TS wire.

### Evidence recorded consistently

- Windows user-local toolchain: Go `1.27.0 windows/amd64`, Node `26.8.1`, and matching Wails CLI/runtime `v2.15.0`; official Go/Node checksums matched where available.
- Exact commit `66158d9` was exported to NTFS staging because Windows Go UNC `RLock` failed. The matching `wails build` exited 0 in 27.436s and produced `monitor\build\bin\ThreadDockMonitor.exe`; bindings/frontend/assets/app stages were `Done`.
- Packaged live acceptance remains incomplete: scheduled-task/WSL interop repeatedly emitted `UtilAcceptVsock:281: accept4 failed 110`, and no valid packaged GitHub/Herdr screenshot exists.
- Earlier plain `go build` at `aeca770` rendered GitHub 69 items and the Herdr default session/4 agents, but it is not final standard-package evidence.
- Cleanup of scheduled task `ThreadDockValidation66158d9` and its related process was requested through user Windows computer-use and remains pending unless separately confirmed.

### Self-review

All four documents use the same distinction between standard package-build success and packaged live
acceptance. The plain-binary observation, component-level CLI observations, interop blocker, and
pending cleanup are not presented as packaged live success. No transient staging randomness or
unsupported Windows screenshot claim was added.

### Follow-up result

- changedFiles: `README.md`, `HANDOFF.md`, `docs/operator/github-first-quickstart.md`, `.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md`
- commitSHA: docs-only follow-up to `66158d9` (the top-level `commitSHA` remains the last product/dependency implementation SHA and is intentionally not self-referential)
- executedCommands: `python3` relative-Markdown-link parser — exit 0, 4 files/10 relative links; `python3` evidence-consistency scan — exit 0, 4 files × 9 required terms; `git diff --check` — exit 0; owned-path diff scan — exit 0, exactly the four assigned documents changed
- outcomes: all docs-only validation passed; no Go/UI/product tests, Windows commands, `make check`, push, PR, or merge was run
- unverified: packaged live GitHub/Herdr UI and completion of `ThreadDockValidation66158d9` cleanup; worker/reviewer runtime model/effort
- blockers: repeated `UtilAcceptVsock:281: accept4 failed 110` during scheduled-task/WSL interop launch; cleanup confirmation pending
