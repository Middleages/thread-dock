# engine-retirement-create-revert Task report

## Scope and baseline

- taskId: `engine-retirement-create-revert`
- baseSHA: `22dcc6610b34582d9a3a4ee375e7661fde499e5e`
- branch/worktree: `agent/engine-retirement-create-revert` / `/home/appuser/dev_system/.worktrees/engine-retirement-create-revert`
- deps: PR #70 merge, slice2 inventory, and the fixed no-interface-change decision
- owned code paths: the `internal/revert` service/tests, CLI revert adapter/tests, listed CLI/cmd files, `parallel-pilot.md`, and this report
- forbidden paths were not changed; no public/shared interface, config/state, Monitor, runner, module, or Git/worktree package was changed.

## Deletion evidence

At the fixed base SHA, the production references were limited to `cmd/agentctl/main.go`,
`internal/cli/revert.go`, and the route/dependency wiring in the owned CLI files. The
following baseline search also showed the dedicated adapter and service tests were the
only test consumers:

```text
git grep -n -E 'thread-dock/internal/revert|Reverter|RevertRunService|runCreateRevert|NewRevertRunService|create-revert' 22dcc6610b34582d9a3a4ee375e7661fde499e5e -- '*.go'
```

After deletion, production Go search has no `Reverter`, `RevertRunService`,
`runCreateRevert`, `NewRevertRunService`, or `thread-dock/internal/revert` references.
The remaining `create-revert` strings are intentional negative tests and the historical
operator note.

## Changes

- Removed `create-revert` from CLI routing, usage, production dependency gating,
  repository discovery, and GHES credential requirements.
- Removed `Dependencies.Reverter` and the production composition of the revert service.
- Deleted `internal/revert/service.go`, `service_test.go`, `internal/cli/revert.go`,
  `revert_test.go`, and `final_review_test.go`.
- Added negative CLI tests for usage/exit 2 with empty stdout and for the no-discovery /
  no-production-dependency boundary; retained the positive `confirm` dependency test.
- Recast `docs/operator/parallel-pilot.md` as a historical record with no current
  execution procedure.

## Focused verification

All test commands used an isolated `/dev/shm` temporary directory created with
`mktemp -d`.

```text
tmpdir=$(mktemp -d /dev/shm/threaddock-create-revert-red.XXXXXX) && TMPDIR="$tmpdir" /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./internal/cli ./cmd/agentctl
```

RED as required: the new negative tests failed because the old route still accepted
`create-revert` and required production setup.

```text
tmpdir=$(mktemp -d /dev/shm/threaddock-create-revert-green.XXXXXX) && TMPDIR="$tmpdir" /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./internal/cli ./cmd/agentctl
```

PASS: `thread-dock/internal/cli` and `thread-dock/cmd/agentctl`.

```text
/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./...
```

PASS: package list contains no `thread-dock/internal/revert` package.

```text
tmpdir=$(mktemp -d /dev/shm/threaddock-create-revert-vet.XXXXXX) && TMPDIR="$tmpdir" /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go vet ./internal/cli ./cmd/agentctl
```

PASS.

```text
test ! -e internal/revert
git diff --check
```

PASS after removing the empty package directory.

One chained verification command returned exit 1 only because its final `test ! -e
internal/revert` ran before the empty directory was removed; `go list` and `go vet` in
that chain had already succeeded, and each verification was rerun independently above.

## Self-review

- The diff is limited to the packet's owned paths and removes approximately 1,216 lines
  of dedicated revert implementation/tests while retaining shared helper code for later
  inventory work.
- `create-revert` now falls through normal unknown-command handling: usage is written to
  stderr, stdout stays empty, and exit code is 2 before config, token, Git repository, or
  production dependency setup.
- `confirm`, remaining v1/v2 commands, and Monitor/runner/shared contracts retain their
  existing boundaries.
- No new test was added to assert file deletion; tests cover the externally observable
  rejection and dependency/discovery behavior.

## Result

- changedFiles: `cmd/agentctl/main.go`, `cmd/agentctl/main_test.go`, `docs/operator/parallel-pilot.md`, `internal/cli/confirm_test.go`, `internal/cli/final_review_test.go` (deleted), `internal/cli/revert.go` (deleted), `internal/cli/revert_test.go` (deleted), `internal/cli/run.go`, `internal/cli/run_commands.go`, `internal/cli/run_commands_test.go`, `internal/revert/service.go` (deleted), `internal/revert/service_test.go` (deleted), `.superpowers/sdd/engine-retirement-slice2/task-report.md`
- commitSHA: `53110f96bc4c853d39457ca08fd4166124bd3355` (implementation commit; this report is a follow-up metadata commit because recording a SHA changes the containing commit)
- executedCommands: baseline `git grep` at fixed base; RED/GREEN focused `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test`; `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./...`; focused `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go vet`; `test ! -e internal/revert`; `git diff --check`; toolchain `gofmt`; owned-path diff check; `git status`/`git diff` self-review
- outcomes: route and dedicated engine removed; negative tests RED then GREEN; focused CLI tests/vet/list/diff checks passed
- unverified: fresh Sol reviewer decision; root integration `make check`; Windows native Wails execution; runtime model/effort identity
- blockers: none
