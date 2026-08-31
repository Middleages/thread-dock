# ThreadDock Execution Session Retirement Design

## Status and scope

This design adds a lifecycle between active Agent execution and the existing seven-day artifact cleanup. It closes completed Agent terminal Workspaces without deleting Git Worktrees, run state, structured evidence, or GitHub records.

In scope:

- automatic retirement for newly completed parallel RUNs;
- explicit retirement for completed RUNs and guarded manual retirement for blocked RUNs;
- crash-safe Herdr Workspace close reconciliation;
- status and audit representation of retired sessions; and
- cleanup compatibility for Worktrees whose Herdr Workspace has already closed.

Out of scope:

- replacing runtime Agents with Codex subagents;
- deleting Worktrees or run state during retirement;
- retiring `needs_operator`, building, reviewing, CI, merging, or paused RUNs;
- a background daemon or timer-based grace period; and
- automatic retirement of historical snapshots that lack retirement state.

## Lifecycle model

Execution Session Retirement and Execution Artifact Cleanup are distinct transitions.

```text
active RUN
  → completed outcome ready
  → retiring (close one exact Workspace per Advance)
  → completed + retired sessions
  → seven-day evidence retention
  → explicit cleanup of clean Worktrees and run state
```

A new config field `autoRetireCompletedSessions` defaults to `true` for newly started RUNs. When enabled, the parallel state machine schedules retirement after main merge and Project `Done` reconciliation but before final `completed`. When disabled, the RUN completes with sessions still active and the Operator may run `agentctl retire RUN`.

`blocked` preserves its phase and sessions by default. `agentctl retire RUN --blocked` is the only automatic interface that may retire it. `needs_operator` and nonterminal phases reject retirement.

There is no five-minute timer in v0.1 because ThreadDock has no background daemon and status reads remain side-effect free. Automatic retirement is immediate once no repair, CI, merge, confirmation, or Project action remains.

## Durable state

`RunSnapshot` gains an additive `Retirement` object:

```go
type RetirementState struct {
    Status      string              `json:"status,omitempty"`
    TargetPhase contract.RunPhase   `json:"targetPhase,omitempty"`
    Automatic   bool                `json:"automatic,omitempty"`
    Targets     []RetirementTarget  `json:"targets,omitempty"`
    UpdatedAt   time.Time           `json:"updatedAt,omitempty"`
}

type RetirementTarget struct {
    Key          string    `json:"key"`
    Role         string    `json:"role"`
    TaskID       string    `json:"taskId,omitempty"`
    WorkspaceID  string    `json:"workspaceId"`
    PaneID       string    `json:"paneId"`
    Path          string    `json:"path"`
    Branch        string    `json:"branch"`
    HeadSHA       string    `json:"headSha"`
    Status        string    `json:"status"`
    RetiredAt     time.Time `json:"retiredAt,omitempty"`
}
```

Target order is deterministic: Reviewer first, then Builders in reverse contract order. Every Workspace ID must have exactly one durable owner; duplicate IDs, including otherwise identical aliases, fail closed to `needs_operator` before any close. Cleanup is task-keyed, so collapsing an alias would lose the ownership needed to classify every later cleanup target safely. Historical Agent, Workspace, pane, path, session, commit and evidence fields remain unchanged; retirement marks lifecycle state instead of erasing identity.

Before the first close action, ThreadDock derives every target and records its canonical path, branch, exact HEAD and repository common-directory identity. Missing or conflicting identity blocks retirement before any Workspace closes.

## Deep module and adapter seams

`internal/retirement` is a pure module. Its interface builds ordered targets and decides `Wait`, `Close`, `Complete`, or `NeedsOperator` from durable snapshot plus one Workspace observation. It performs no Herdr, Git, state, or clock side effects.

The Herdr adapter adds narrow ports:

```go
type WorkspaceReader interface {
    GetWorkspace(context.Context, string) (WorkspaceInfo, bool, error)
}

type WorkspaceCloser interface {
    CloseWorkspace(context.Context, string) error
}
```

`WorkspaceInfo` contains only Workspace ID, root pane ID, canonical checkout path and lifecycle status. Raw terminal output is absent.

The Git adapter adds read-only retirement proof and non-force retired cleanup operations. It verifies:

- configured repository and Herdr Worktree roots are canonical and trusted;
- target is strictly contained in the Herdr Worktree root;
- target is a registered Worktree of the exact repository common directory;
- branch and HEAD match durable retirement proof; and
- status is clean before removal.

## State transitions and reconciliation

Each `Advance` performs at most one external observation or mutation.

1. Persist `retirement_prepare`, then inspect Git identity for one target.
2. Persist the complete ordered target list before the first close.
3. Persist `retirement_close:<target-key>` before `workspace close`.
4. On restart, read the exact Workspace ID.
   - Not found means the close completed; mark the target retired.
   - Found with exact path/pane identity permits an idempotent close retry.
   - Found with conflicting identity yields `NeedsOperator` without closing.
5. After all targets are retired, transition to the recorded terminal phase and append a retirement audit event.

An Agent still reported as `working` or `blocked` prevents automatic close and requires Operator judgment. `idle`, `done`, or an already-closed Workspace may retire after structured evidence and Git identity have been preserved.

For automatic completed retirement, the parallel phase becomes `retiring` after all product and Project outcomes are durable. A retirement failure cannot undo main merge; it remains visible as a session-lifecycle blocker. Manual blocked retirement keeps `TargetPhase=blocked` and returns to `blocked` after all requested sessions close.

## CLI and status

New commands:

```text
agentctl retire RUN
agentctl retire RUN --blocked
```

`retire RUN` accepts completed RUNs with active sessions. `--blocked` is required for blocked RUNs and is rejected for every other phase. Repeated commands are idempotent. `resume RUN` advances `retiring` one action; the retire command may loop through Advances but each Advance retains the one-action invariant.

Human and JSON status show active, retiring, and retired Agent sessions separately. Retired sessions are hidden from the default active count but retain their historical identity and evidence.

## Cleanup compatibility

Existing cleanup remains restricted to completed RUNs older than seven days and performs a complete read-only preflight before the first deletion.

- Active Herdr Worktrees continue through the existing exact Workspace identity and `herdr worktree remove --workspace` path.
- Retired Worktrees use the durable retirement proof and the new path-based Git cleanup path because Herdr 0.8.2 forgets the Workspace ID after close.
- Mixed active/retired targets are supported, but any dirty, missing, foreign, moved, or identity-mismatched target aborts the entire cleanup before deletion.
- Cleanup never uses `--force`, never follows symlink escapes, and never deletes the canonical repository, home, filesystem root, or an unregistered checkout.

## Testing and pilot acceptance

Required deterministic stories:

1. completed parallel RUN closes Reviewer and both Builder Workspaces in order;
2. close response loss reconciles an already-missing Workspace without duplication;
3. exact Workspace mismatch stops before close;
4. working/blocked Agent prevents automatic retirement;
5. `needs_operator` and active RUN retirement are rejected;
6. blocked retirement requires `--blocked` and returns to blocked;
7. auto-retirement disabled leaves sessions active and manual retirement succeeds;
8. legacy snapshots decode and do not auto-retire;
9. cleanup removes retired clean Worktrees by verified Git path after seven days;
10. dirty or foreign retired Worktree prevents every cleanup mutation.

The live pilot uses a disposable completed RUN. It records Workspace IDs before close, proves they are absent afterward, proves all Worktree paths and run state remain, then executes cleanup only on a separate aged disposable RUN. Current parallel pilot evidence Workspaces are not retirement targets until their evidence is accepted.
