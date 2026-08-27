# ThreadDock Implementation Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver ThreadDock in four independently testable increments, from a deterministic work contract to the Windows Monitor and production Actions dispatch.

**Architecture:** A Go `agentctl` deep module owns contracts, state and orchestration inside WSL. OpenCode and the Windows Wails application consume the same JSON CLI interface; GHES remains the permanent record and Herdr remains the execution runtime.

**Tech Stack:** Go 1.27.0, Wails v2.14.0, React 19 with TypeScript, GitHub Enterprise Server REST API `2022-11-28`, Herdr v0.8.2, OpenCode, Git Worktree.

**Spec:** `../../../gitops-agent-system-design.md`

## Global Constraints

- Support Windows 11 with WSL and one Operator per Trusted Workstation.
- Run one active Parent Issue and at most two Builders per PC.
- Use Worktree Isolation; do not claim it is a security sandbox.
- Keep production credentials out of WSL and Agent processes.
- Use Korean UI copy from `../../../opendesign/design-systems/thread-dock-product/brand/voice-and-tone.md`.
- Use the Civic Cobalt tokens from `../../../opendesign/design-systems/thread-dock-product/tokens/colors_and_type.css`.
- Do not add a local HTTP server, message queue, plugin system or multi-user account system.
- Keep GitHub as permanent record; local state must remain disposable and recoverable.
- Pin dependencies and commit `go.sum` and `frontend/package-lock.json`.

---

## Delivery sequence

| Order | Plan | Working result | Gate before next plan |
|---|---|---|---|
| 1 | [`2026-08-28-threaddock-foundation.md`](./2026-08-28-threaddock-foundation.md) | `agentctl` validates and previews a complete work contract; a repository template installs the same contract | Contract fixtures, CLI and template tests pass |
| 2 | [`2026-08-28-threaddock-single-run.md`](./2026-08-28-threaddock-single-run.md) | One Builder and an independent Reviewer complete a recoverable local run against fake GHES and fake Herdr, then one real pilot | Restart and GHES outage drills preserve state |
| 3 | [`2026-08-28-threaddock-parallel-merge.md`](./2026-08-28-threaddock-parallel-merge.md) | Two Builders integrate, repair at most twice and pass the supervised auto-merge gate | Conflict, Protected Change and recovery scenarios pass |
| 4 | [`2026-08-28-threaddock-monitor.md`](./2026-08-28-threaddock-monitor.md) | Windows ThreadDock Monitor observes runs, controls local recovery, dispatches Actions and updates through GHES Releases | Windows scale, workflow dispatch and updater rollback E2E pass |

## Cross-plan interfaces

The following names are stable across all four plans.

```go
package contract

const CurrentVersion = 1

type RunID string
type RunPhase string

type TaskContract struct {
    Version      int            `json:"version"`
    Parent       IssueDraft     `json:"parent"`
    Children     []IssueDraft   `json:"children"`
    Repository   RepositoryRef  `json:"repository"`
    BaseCommit   string         `json:"baseCommit"`
    Tasks        []Task         `json:"tasks"`
    Protected    []string       `json:"protectedPaths"`
    Verification []string       `json:"verification"`
}

type StatusView struct {
    ContractVersion int        `json:"contractVersion"`
    RunID           RunID      `json:"runId"`
    Phase           RunPhase   `json:"phase"`
    Summary         string     `json:"summary"`
    NextAction      *NextAction `json:"nextAction,omitempty"`
    Agents          []AgentView `json:"agents"`
    GitHub          GitHubView `json:"github"`
    UpdatedAt       time.Time  `json:"updatedAt"`
}
```

`agentctl status --json` must preserve these fields and ignore unknown fields. New optional fields do not increase `ContractVersion`; incompatible meaning or removal does.

## Program-level acceptance

- [ ] A real request produces one approved Parent Issue and only necessary Child Issues.
- [ ] One active run survives OpenCode exit, WSL restart and a temporary GHES outage.
- [ ] Two independent Builders integrate without modifying each other's owned paths.
- [ ] Reviewer and CI share a two-round repair budget that the Orchestrator cannot bypass.
- [ ] Ordinary changes merge automatically; Protected Changes require the Operator.
- [ ] The Monitor shows plain Korean state and every workflow, but only dispatchable workflows expose an action.
- [ ] Production dispatch originates from Windows, never from WSL.
- [ ] A failed application update restores the previous runnable version.
- [ ] Pilot metrics are captured for 5–10 real Issues before declaring v0.1 adopted.

## Spec coverage map

| Spec area | Implemented by |
|---|---|
| Work Interview contract, Parent/Child model, repository template, CODEMAP/Serena procedure | Foundation Tasks 2–5 |
| GHES Issue bundle, Organization Project state, local snapshot/events, Worktree and Herdr | Single-Run Tasks 1–5 |
| CLI status/recovery and disposable one-Builder pilot | Single-Run Task 6 |
| DAG, two Builders, ownership, integration and global verification | Parallel Tasks 1–3 |
| Reviewer/CI repair budget and slow-model recovery | Parallel Tasks 4–5 |
| Protected Change, auto-merge, Revert PR and Project completion | Parallel Tasks 6–7 |
| Windows/WSL CLI seam, News Desk/Civic Cobalt and 100–200% scale | Monitor Tasks 1–4 |
| Workflow Catalog, Windows-only production credential and dispatch | Monitor Task 5 |
| GHES Releases, assisted update, rollback and package | Monitor Tasks 6–7 |
| 2-week/5–10 Issue pilot metrics | each plan's pilot guide; roadmap program acceptance |
