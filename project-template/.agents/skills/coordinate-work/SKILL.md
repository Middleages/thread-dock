---
name: coordinate-work
description: Use when a project coordinator plans GitHub work and prepares or dispatches bounded Herdr feature sessions across a repository or Project.
---

# Coordinate work

Use GitHub Issue, PR, and Projects as durable work truth. Herdr owns execution lifecycle; ThreadDock only connects GitHub evidence to top-level session state.

For Herdr CLI operations, use **REQUIRED SUB-SKILL:** `herdr-local` for installed syntax, managed context, actual target identity, and any explicit network constraints. Existing authorization carries forward; report only a concrete missing prerequisite for the affected operation.

Read repository guidance, the relevant open GitHub work, and [the shared development stance](../develop-feature/references/development-stance.md) first. Keep the user's authorization separate from plan readiness: a planning-only request permits planning/review helpers, not implementation, new Feature Leader sessions, or GitHub writes. Existing authorized independent work may continue; preserve active sessions and drafts rather than stopping or recreating them to adopt this workflow.

## Plan before feature dispatch

1. Author or reuse an overall plan before assigning dependent features. Include goals/non-goals, current architecture evidence and chosen direction, independently deliverable feature boundaries and shared interfaces/contracts, dependencies and execution order/serialized boundaries, acceptance evidence for each feature, and unresolved decisions. Keep this at project/feature level; the Coordinator does not write every leaf's file/task/test plan.
2. For a significant overall plan, use `grill-plan`: freeze the revision, obtain a fresh adversarial review and a different neutral adjudication, and incorporate accepted findings with reasons for accepted/rejected decisions before dispatching features that depend on them. A small, clear task needs only a short plan. Reuse an existing reviewed plan and its evidence when scope, contracts, and assumptions still match; review only a material change, not the same plan again. If a decision/review is unresolved, hold only affected feature dispatch and continue independent authorized work. Review completion does not create a new user approval requirement for already-authorized implementation.
3. Pass the plan reference and revision, applicable decisions/contracts, feature goal and acceptance, dependencies/order, repository/target branch, Issue URL when applicable, and worktree in each Feature Leader packet. The Feature Leader expands its feature into concrete files, tasks, and tests. A change to another feature boundary, shared contract, dependency, or overall assumption returns to the Coordinator; replan/review the affected portion and update dependent packets while preserving unrelated work.

## Assign and observe

Before starting anything new, inspect `~/.threaddock/sessions.json` and the actual Herdr state. Reuse an exact live Feature Leader session when its Issue/repository/worktree locator matches. Never guess session, workspace, tab, pane, Agent name, or cwd values; use values returned by the installed Herdr tool.

For an authorized new feature whose prerequisite plan decisions are ready, create a bounded top-level Feature Leader session with the packet above. Planner/implementer/reviewer helpers belong inside that session as native subagents and are not top-level ThreadDock bindings.

When one or more bound Feature Leader sessions are still working and the coordinator has no immediate routing action, use **REQUIRED SUB-SKILL:** `agent-wait`. Periodically inspect only the bound top-level Herdr sessions and corresponding GitHub evidence. Do not inspect Feature Leader leaf agents, busy-poll, or create a separate scheduler.

Maintain the shared locator under one user-level exclusive lock such as `~/.threaddock/sessions.lock`: acquire lock, re-read the full JSON, mutate only the exact coordinator/feature binding, write a temporary file, then atomically rename it over `sessions.json`. Preserve every unrelated repository/project binding. If locking is unsafe or ambiguous, leave the file unchanged and report locator update pending.

Do not remove a feature binding for `idle`, `done`, or a temporary observation failure. Cleanup requires all three: Issue closed, related PR work finished, and top-level Herdr feature session absent.

**REQUIRED SUB-SKILL:** Use `publish-work` for durable GitHub updates and locator reconciliation within existing write authorization. For planning-only work without publication authorization, return the plan/review record in the requested local or conversational form instead.
