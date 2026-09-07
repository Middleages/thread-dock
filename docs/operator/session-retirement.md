> **Legacy v1 참고 자료.** 기존 구현·파일럿의 절차와 관찰 기록이며 새 MVP의 실행 계획이나 완료 조건이 아닙니다. [현재 설계](../superpowers/specs/2026-09-07-project-workflow-mvp-design.md)를 먼저 따르십시오.

# Session Retirement Runbook

This runbook covers the safe, two-stage lifecycle for completed ThreadDock
execution sessions. Retirement closes Herdr Workspaces and records immutable
Git identity; cleanup, which is a separate operator action after seven days,
removes only verified clean Git Worktrees and disposable run state.

This procedure is for a new disposable pilot run only. Never retire or clean a
parallel-pilot run while its evidence is awaiting acceptance: this is the
evidence awaiting acceptance guard. In particular,
do not target the accepted or diagnostic parallel-pilot Workspaces, RUNs,
state directories, or evidence paths named in
`docs/operator/parallel-pilot.md`.

## Safety boundary and preconditions

Use a new temporary directory for every pilot. Do not point the commands below
at the source checkout, `$HOME`, the repository's existing ThreadDock state
directory, or an accepted/diagnostic evidence directory.

Before creating any state or provider resource:

```bash
set -euo pipefail
export PILOT_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/threaddock-retirement-pilot.XXXXXX")"
export PILOT_REPO="$PILOT_ROOT/repo"
export PILOT_STATE="$PILOT_ROOT/state"
export PILOT_WORKTREES="$PILOT_ROOT/worktrees"
export PILOT_BIN="$PILOT_ROOT/bin/agentctl"
mkdir -p "$PILOT_STATE" "$PILOT_WORKTREES" "$(dirname "$PILOT_BIN")"
git clone --no-local . "$PILOT_REPO"
cd "$PILOT_REPO"
export FROZEN_SHA="$(git rev-parse HEAD)"
test "$(git rev-parse --verify "$FROZEN_SHA^{commit}")" = "$FROZEN_SHA"
go version                         # must report go1.27.0
herdr --version                    # must report herdr 0.8.2
command -v git herdr opencode jq
```

Run the focused and repository review gates from the frozen checkout before
starting the live probe. A pilot is not evidence if either gate has not
passed:

```bash
go test ./internal/orchestrator ./internal/cli ./internal/herdr ./internal/worktree ./internal/config ./internal/pilot -v
go test -race ./...
make check
go build -trimpath -o "$PILOT_BIN" ./cmd/agentctl
```

The pilot process may receive `THREADDOCK_GH_TOKEN` only when a separate
GitHub contract operation requires it. Retirement and cleanup use persisted
identity and do not require a GitHub token. Do not put a token, credential,
OpenCode transcript, or raw Herdr output in a snapshot, event, or evidence
file.

## Configuration and lifecycle

Use a dedicated config whose state and Herdr roots are both under
`$PILOT_ROOT`:

```json
{
  "ghesHost": "https://github.example.test",
  "apiBase": "https://github.example.test/api/v3",
  "stateDir": "/absolute/path/to/PILOT_ROOT/state",
  "herdrWorktreeRoot": "/absolute/path/to/PILOT_ROOT/worktrees",
  "projectId": "pilot-project",
  "projectStatusFieldId": "pilot-status-field",
  "projectStatusOptions": {
    "Backlog": "backlog",
    "Ready": "ready",
    "In Progress": "in-progress",
    "Review": "review",
    "Done": "done"
  },
  "autoRetireCompletedSessions": true
}
```

Set `THREADDOCK_CONFIG` to that file and verify that paths resolve inside
`$PILOT_ROOT`. `autoRetireCompletedSessions` defaults to `true` when omitted
for a newly started RUN. With `true`, the parallel coordinator schedules
retirement after merge and Project `Done` reconciliation, before the final
`completed` snapshot. With `false`, completion leaves sessions active and the
operator must use `agentctl retire RUN`.

The durable lifecycle is:

```text
completed outcome ready
  -> PhaseRetiring / phase=retiring
  -> close one exact Workspace per resume action
  -> phase=completed, retirement.status=retired
  -> wait until UpdatedAt is older than seven days
  -> agentctl cleanup RUN
```

While the plan is running, the snapshot keeps `retirement.targetPhase`,
`retirement.automatic`, `retirement.targets`, exact Workspace/pane IDs,
canonical Worktree paths, branches, HEAD SHAs, repository common-directory
identity, and `UpdatedAt`. Append-only events include
`retirement_prepare`, one `retirement_close:<target-key>` event per target,
and `sessions_retired`. The default target order is Reviewer first, followed
by Builders in reverse contract order.

## Inspecting status and choosing the guard

These are the exact read and mutation commands. `status` is side-effect free;
`resume` advances one retirement action and is safe to repeat after an
interrupted process.

```bash
agentctl status RUN
agentctl status RUN --json
agentctl retire RUN
agentctl retire RUN --blocked
agentctl resume RUN
agentctl cleanup RUN
```

Human and JSON status distinguish active, `retiring`, and `retired` sessions.
Retired sessions leave the active count but retain their historical Agent,
Workspace, pane, Worktree, branch, commit, and evidence identity.

`agentctl retire RUN` is valid for a completed RUN. A blocked RUN requires the
explicit `--blocked` guard; the operator-facing error is exactly
`blocked retirement requires --blocked`. `agentctl retire RUN --blocked` is
rejected for completed and every non-blocked phase. `active`, `building`,
`reviewing`, `ci`, `merging`, `paused`, and `needs_operator` RUNs are not
retirable. A `working`, `blocked`, or `unknown` Agent/Workspace observation
also pauses automatic retirement and requires operator attention. A repeated
retire command is idempotent after `retirement.status=retired`.

## Stage 1: close sessions, preserve artifacts

Create a completed synthetic RUN with strict, internally consistent evidence:
the Agent state must be `idle` or `done`, every task must have exact
verification evidence and an immutable commit SHA, and each target must have
one Workspace ID, root pane ID, canonical path, branch, HEAD SHA, and common
Git directory. Start/prompt a disposable OpenCode Agent and wait until its
state is idle/done before recording the pre-close snapshot.

Record exact identifiers and read-only proofs before retirement:

```bash
RUN="retirement-disposable-RUN"
WORKSPACE_ID="retirement-disposable-workspace"
WORKTREE_PATH="$PILOT_WORKTREES/$RUN"
git -C "$PILOT_REPO" worktree list --porcelain
git -C "$WORKTREE_PATH" rev-parse --show-toplevel
git -C "$WORKTREE_PATH" rev-parse --abbrev-ref HEAD
git -C "$WORKTREE_PATH" rev-parse HEAD
git -C "$WORKTREE_PATH" status --porcelain=v1
herdr workspace get "$WORKSPACE_ID"
herdr pane list --workspace "$WORKSPACE_ID"
cp "$PILOT_STATE/runs/$RUN/run.json" "$PILOT_ROOT/run-before-retire.json"
cp "$PILOT_STATE/runs/$RUN/events.jsonl" "$PILOT_ROOT/events-before-retire.jsonl"
```

Run retirement from the frozen agentctl binary and capture only exit status
and structured summaries:

```bash
THREADDOCK_CONFIG="$PILOT_ROOT/config.json" "$PILOT_BIN" retire "$RUN"
THREADDOCK_CONFIG="$PILOT_ROOT/config.json" "$PILOT_BIN" status "$RUN" --json
```

The coordinator persists the complete plan before its first close and performs
at most one external observation or mutation per `resume RUN`. The `response loss`
case is handled when a close response is lost: the next `resume RUN` reads the exact
Workspace ID. A
missing Workspace means the close completed and is reconciled as retired; it
must not issue a duplicate close. A found Workspace is retryable only when
its path and pane identity exactly match the durable target. Any mismatch,
missing Git proof, or unsafe lifecycle status becomes `needs_operator` before
another close.

After the command succeeds, prove Workspace absence for every target while the
Git and ThreadDock artifacts remain:

```bash
! herdr workspace get "$WORKSPACE_ID"
git -C "$PILOT_REPO" worktree list --porcelain | grep -F -- "$WORKTREE_PATH"
test -d "$WORKTREE_PATH"
test -f "$PILOT_STATE/runs/$RUN/run.json"
test -f "$PILOT_STATE/runs/$RUN/events.jsonl"
test -f "$PILOT_STATE/runs/$RUN/evidence.json"
grep -F '"phase":"completed"' "$PILOT_STATE/runs/$RUN/run.json"
grep -F '"status":"retired"' "$PILOT_STATE/runs/$RUN/run.json"
```

Run a secret scan over all disposable runtime RUN state under
`$PILOT_STATE/runs`, config, saved snapshots, the recorded pilot result, and
structural checkout metadata (`config`, `HEAD`, `gitdir`, and `commondir`). A
fallback marker scan must not treat source checkouts or Git object blobs as
runtime metadata; repository source is covered by the normal repository
secret gate and may contain synthetic redaction fixtures. The scan must pass
and the output must contain no token, credential, or raw terminal response. If
the exact scanner used by the operator is `gitleaks`, for example:

```bash
gitleaks detect --no-banner --redact --source "$PILOT_ROOT"
```

Do not continue to cleanup when a Workspace is still present, a Worktree,
`run.json`, events, or evidence disappeared during retirement, or the secret
scan fails.

## Stage 2: age one fixture and clean safely

Cleanup is intentionally separate from retirement. Never age or edit a
production or evidence-awaiting-acceptance RUN. For this disposable pilot,
copy the pre-cleanup snapshot outside the state directory, then set the
disposable RUN's `UpdatedAt` to more than seven days in the past through a
controlled fixture edit. The cleanup command must keep using the original pilot config
so its state-managed Worktree trust root remains identical to the
one used when the RUN was created:

```bash
cp "$PILOT_STATE/runs/$RUN/run.json" "$PILOT_ROOT/run-before-cleanup.json"
export OLD_UPDATED_AT="$(date -u -d '8 days ago' '+%Y-%m-%dT%H:%M:%SZ')"
jq --arg updatedAt "$OLD_UPDATED_AT" '.updatedAt = $updatedAt' \
  "$PILOT_STATE/runs/$RUN/run.json" >"$PILOT_STATE/runs/$RUN/run.json.tmp"
mv "$PILOT_STATE/runs/$RUN/run.json.tmp" "$PILOT_STATE/runs/$RUN/run.json"
THREADDOCK_CONFIG="$PILOT_ROOT/config.json" "$PILOT_BIN" cleanup "$RUN"
```

Changing `stateDir` for this copied snapshot is unsafe: the managed Worktree
root is derived from `stateDir`, so it would no longer match the persisted
Integration path. The saved `run-before-cleanup.json` is the immutable
pre-cleanup evidence; the disposable RUN state itself is expected to be
deleted on success. Before any removal, cleanup preflights every target. For
an active Herdr Workspace it verifies exact
Workspace/pane/path identity and uses the provider removal handle. For a
retired target it re-proves the durable repository common directory, exact
path, branch, HEAD, Worktree registration, and clean status, then uses the
Git-path removal handle. Any dirty, foreign, moved, missing, or conflicting
target aborts the entire operation before the first removal.

Herdr 0.8.2 forgets the Workspace ID after a successful `workspace close`.
Consequently, retired cleanup cannot use `herdr worktree remove --workspace`;
it must use the persisted Git proof and `git worktree remove WORKTREE_PATH`
after the read-only re-proof. The path handle does not delete a branch. A
lost removal response is successful only when both the target path and its
Git registration are absent; a stale registration is unsafe.

The implementation never passes `--force` to Herdr or Git, never force-resets
or follows a symlink, and never removes the repository root, home directory,
filesystem root, or an unregistered checkout. Verify both resources are gone
only after cleanup reports success:

```bash
test ! -e "$WORKTREE_PATH"
test ! -e "$PILOT_STATE/runs/$RUN"
test -f "$PILOT_ROOT/run-before-cleanup.json"
! git -C "$PILOT_REPO" worktree list --porcelain | grep -F -- "$WORKTREE_PATH"
```

If a cleanup check fails, preserve the Worktree and run state, record the
blocked reason, and ask an operator to reconcile the exact identity. Never
add `--force` to make the command pass.

## Evidence record and acceptance

Write a short Markdown result under the disposable state root, for example
`$PILOT_STATE/pilot-results/session-retirement-RUN.md`. It must contain the
full frozen checkout SHA, disposable RUN ID, every Workspace ID, canonical
Worktree path, branch and HEAD proof, pre/post status, stage-1 Workspace
absence result, Worktree/run-state/evidence preservation result, cleanup age,
stage-2 removal result, exact commands, and secret-scan result. It must not
contain raw transcript, prompt text, token, credential, or unredacted provider
output. Reference that evidence path in the operator handoff and keep all
accepted/diagnostic parallel-pilot evidence untouched.
