# Task 3 report

status: DONE_WITH_CONCERNS

## changedFiles

- `README.md`
- `HANDOFF.md`
- `docs/operator/github-first-quickstart.md`
- `.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md`

This final task is a docs-only follow-up. Product/config/dependency implementation remains at
`325db89`; no product, test, Makefile, dependency, generated asset, or shared-interface file changed.

## commitSHA

325db89ad81d53c9f1a33ba0454287e98b22c0eb (last reviewed product/config/dependency implementation SHA; this native Windows evidence update is a docs-only follow-up and is intentionally not self-referential).

## executedCommands

- See the final docs-only validation ledger below. No Go/UI/product tests, Windows commands, `make check`, push, PR, or merge were run for this task.

## TDD red/green evidence

The UI behavior tests were changed first to require removal of legacy task/publication sections, automation panel, nonfunctional top-menu buttons, and duplicate selected-work Herdr location. The focused monitor test run failed with those three expected failures. The minimal App/CSS/Vite changes were then applied, and the focused bindings + monitor run passed 20/20.

Deletion/config changes were additionally validated by the empty server directory, source-reference scan, TypeScript/Vite build, Makefile dry-run, and shell/diff checks.

## Historical implementation self-review (superseded by final native evidence)

- Removed Node server imports, middleware, endpoint path, and Vite test exclusion while retaining React/Vite build tooling.
- Removed old task/publication and automation presentation, and kept decisions, handoff, GitHub evidence, notices, degraded states, safe links, clipboard handoff, observed sessions, and unconnected Agents.
- Moved selected-work Herdr presentation to the single Herdr panel; the handoff still receives the selected connection.
- Updated the three requested documents to separate Linux fixture/UI/build evidence, managed-pane live context, and unverified Windows evidence. No GitHub or Wiki write was claimed.
- Confirmed no forbidden worktree or Go/shared-interface file was changed. Re-added the tracked frontend `dist/.placeholder` after the build generated assets.

## unverified

- Native Windows error/degraded-state rendering; no native error occurred during the healthy run.
- Worker/reviewer runtime model and effort identity remain unverified from this thread.

## blockers

None for the healthy packaged acceptance. The prior `UtilAcceptVsock:281: accept4 failed 110` and pending-cleanup state are historical diagnostics superseded by the final native run.

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

## Historical Windows evidence docs follow-up (superseded by final native evidence)

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

### Historical follow-up result (superseded by final native run)

- changedFiles: `README.md`, `HANDOFF.md`, `docs/operator/github-first-quickstart.md`, `.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md`
- commitSHA: docs-only follow-up to `66158d9` (the top-level `commitSHA` remains the last product/dependency implementation SHA and is intentionally not self-referential)
- executedCommands: see the auditable command ledger below; every command exited 0
- outcomes: all docs-only validation passed; no Go/UI/product tests, Windows commands, `make check`, push, PR, or merge was run
- unverified: packaged live GitHub/Herdr UI and completion of `ThreadDockValidation66158d9` cleanup; worker/reviewer runtime model/effort
- blockers: repeated `UtilAcceptVsock:281: accept4 failed 110` during scheduled-task/WSL interop launch; cleanup confirmation pending

### Auditable docs-only validation ledger

The following commands were run in this worktree against the four assigned documents. No product
tests, Windows commands, or `make check` were run.

1. Relative Markdown link parser — exit 0; output `OK relative Markdown links: 4 files, 10 links`.

   ```sh
   python3 - <<'PY'
   from pathlib import Path
   import re
   files = [Path('README.md'), Path('HANDOFF.md'), Path('docs/operator/github-first-quickstart.md'), Path('.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md')]
   links = 0
   for source in files:
       for target in re.findall(r'(?<!!)\[[^\]]+\]\(([^)]+)\)', source.read_text()):
           if target.startswith(('http://', 'https://', '#', 'mailto:')):
               continue
           links += 1
           path = (source.parent / target.split('#', 1)[0]).resolve()
           if not path.exists():
               raise SystemExit(f'MISSING {source}: {target}')
   print(f'OK relative Markdown links: {len(files)} files, {links} links')
   PY
   status=$?
   printf 'EXIT %s\n' "$status"
   exit "$status"
   ```

2. Evidence consistency check — exit 0; output `OK evidence consistency scan: 4 files x 9 required terms`.

   ```sh
   python3 - <<'PY'
   from pathlib import Path
   files = [Path('README.md'), Path('HANDOFF.md'), Path('docs/operator/github-first-quickstart.md'), Path('.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md')]
   terms = ['66158d9', 'v2.15.0', '26.8.1', '27.436', r'monitor\build\bin\ThreadDockMonitor.exe', 'UtilAcceptVsock:281: accept4 failed 110', 'aeca770', 'ThreadDockValidation66158d9', 'packaged live']
   for term in terms:
       missing = [str(path) for path in files if term not in path.read_text()]
       if missing:
           raise SystemExit(f'MISSING CONSISTENCY TERM {term!r}: {missing}')
   print(f'OK evidence consistency scan: {len(files)} files x {len(terms)} required terms')
   PY
   status=$?
   printf 'EXIT %s\n' "$status"
   exit "$status"
   ```

3. Owned-path diff against base `66158d9c8f6883434f7271e5c5f64e31ba0221b` — exit 0; output `OK owned-path diff against 66158d9: four assigned documents only`.

   ```sh
   test "$(git diff --name-only 66158d9c8f6883434f7271e5c5f64e31ba0221b..HEAD | sort)" = "$(printf '%s\n' '.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md' 'HANDOFF.md' 'README.md' 'docs/operator/github-first-quickstart.md' | sort)" && printf '%s\n' 'OK owned-path diff against 66158d9: four assigned documents only'
   ```

4. Whitespace check — exit 0; output `EXIT 0`.

   ```sh
   git diff --check; status=$?; printf 'EXIT %s\n' "$status"; exit "$status"
   ```

5. Worktree status — exit 0; output was ` M .superpowers/sdd/github-herdr-monitor-task3/task-3-report.md` followed by `EXIT 0` before committing this report-only follow-up.

   ```sh
   git status --short; status=$?; printf 'EXIT %s\n' "$status"; exit "$status"
   ```

## Final native Windows evidence docs

This is the final docs-only follow-up for reviewed product/config/dependency SHA `325db89`.
The native run is healthy-path evidence; native error/degraded rendering remains unverified because
no error occurred. The prior `UtilAcceptVsock:281: accept4 failed 110` and pending cleanup wording
above is historical and superseded, not current state.

### Final evidence

- Native environment: `THREADDOCK_REPOS=Middleages/thread-dock`, `THREADDOCK_PROJECTS` unset,
  `THREADDOCK_WSL_DISTRIBUTION=Ubuntu`, and
  `THREADDOCK_SESSIONS_FILE=/tmp/threaddock-aeca770-sessions.json` (the WSL-internal absolute
  version 1 connection-file path).
- User-local Windows toolchain: Go `1.27.0 windows/amd64`, Node `26.8.1`, and matching Wails
  CLI/runtime `v2.15.0`; official Go/Node checksums matched where available.
- Matching standard `wails build` at `325db89` exited 0 in `1m9.285s` and produced
  `monitor\build\bin\ThreadDockMonitor.exe`; bindings/frontend/assets/app stages were `Done`, and
  no black child console was visible.
- Healthy packaged live acceptance: GitHub showed 69개 work item after approximately 14 seconds;
  Herdr showed the default session and 3 agents after approximately 30 seconds; handoff showed a
  success toast and the real clipboard length was `188` including the repository name; the healthy
  green dot and `로컬 연결 정상` text agreed.
- Shutdown and cleanup: the app was closed with Monitor process count `0`; scheduled task
  `ThreadDockValidation66158d9` was absent and related process count was `0`; WSL interop was
  restored. Computer-use modified no files or worktrees.
- Limitation: no native error occurred. Combined degradation is covered by fixture/UI tests only;
  it is not native Windows error-state proof.

### Final self-review

All four owned documents now use the same final SHA, native toolchain/build facts, healthy live
observations, cleanup outcome, and error-state limitation. Stale failure/pending-cleanup language is
marked historical and superseded. No product/config/dependency file or shared interface changed.

### Final result

- changedFiles: `README.md`, `HANDOFF.md`, `docs/operator/github-first-quickstart.md`, `.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md`
- commitSHA: `325db89ad81d53c9f1a33ba0454287e98b22c0eb` is the product/config/dependency SHA. Previous docs candidate `d34ce3a` is superseded by this fix; the final fix SHA is coordinator-ledgered after commit.
- executedCommands: exact docs-only commands and outputs are recorded below; no product tests, Windows commands, `make check`, push, PR, or merge were run.
- outcomes: docs consistently describe healthy packaged acceptance and the native error-state limitation.
- unverified: native Windows error/degraded-state rendering; worker/reviewer runtime model and effort identity.
- blockers: none for healthy packaged acceptance.

### Final auditable docs-only validation ledger

The following commands are reproducible from this worktree and touch only the four assigned documents.

1. Relative Markdown link parser — exit 0; output `OK relative Markdown links: 4 files, 10 links`.

   ```sh
   python3 - <<'PY'
   from pathlib import Path
   import re
   files = [Path('README.md'), Path('HANDOFF.md'), Path('docs/operator/github-first-quickstart.md'), Path('.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md')]
   links = 0
   for source in files:
       for target in re.findall(r'(?<!!)\[[^\]]+\]\(([^)]+)\)', source.read_text()):
           if target.startswith(('http://', 'https://', '#', 'mailto:')):
               continue
           links += 1
           path = (source.parent / target.split('#', 1)[0]).resolve()
           if not path.exists():
               raise SystemExit(f'MISSING {source}: {target}')
   print(f'OK relative Markdown links: {len(files)} files, {links} links')
   PY
   status=$?
   printf 'EXIT %s\n' "$status"
   exit "$status"
   ```

2. Evidence consistency scan — exit 0; output `OK evidence consistency scan: 4 files x 15 required terms`.

   ```sh
   python3 - <<'PY'
   from pathlib import Path
   files = [Path('README.md'), Path('HANDOFF.md'), Path('docs/operator/github-first-quickstart.md'), Path('.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md')]
   terms = ['325db89', 'v2.15.0', '26.8.1', '1m9.285', r'monitor\build\bin\ThreadDockMonitor.exe', 'Middleages/thread-dock', 'THREADDOCK_WSL_DISTRIBUTION=Ubuntu', 'THREADDOCK_SESSIONS_FILE=/tmp/threaddock-aeca770-sessions.json', '69개 work item', 'clipboard', '로컬 연결 정상', 'UtilAcceptVsock:281: accept4 failed 110', 'aeca770', 'ThreadDockValidation66158d9', 'packaged live']
   for term in terms:
       missing = [str(path) for path in files if term not in path.read_text()]
       if missing:
           raise SystemExit(f'MISSING CONSISTENCY TERM {term!r}: {missing}')
   print(f'OK evidence consistency scan: {len(files)} files x {len(terms)} required terms')
   PY
   status=$?
   printf 'EXIT %s\n' "$status"
   exit "$status"
   ```

3. Owned-path diff against base `325db89ad81d53c9f1a33ba0454287e98b22c0eb` — exit 0; output
   `OK owned-path diff against 325db89: four assigned documents only`.

   ```sh
   test "$(git diff --name-only 325db89ad81d53c9f1a33ba0454287e98b22c0eb -- | sort)" = "$(printf '%s\n' '.superpowers/sdd/github-herdr-monitor-task3/task-3-report.md' 'HANDOFF.md' 'README.md' 'docs/operator/github-first-quickstart.md' | sort)" && printf '%s\n' 'OK owned-path diff against 325db89: four assigned documents only'
   ```

4. Whitespace check — exit 0; output `EXIT 0`.

   ```sh
   git diff --check; status=$?; printf 'EXIT %s\n' "$status"; exit "$status"
   ```

5. Status check — exit 0 before commit; literal output was:
   ```text
    M .superpowers/sdd/github-herdr-monitor-task3/task-3-report.md
    M HANDOFF.md
    M README.md
    M docs/operator/github-first-quickstart.md
   EXIT 0
   ```

   ```sh
   git status --short; status=$?; printf 'EXIT %s\n' "$status"; exit "$status"
   ```
