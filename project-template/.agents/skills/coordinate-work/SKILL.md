---
name: coordinate-work
description: Use when a project coordinator must turn GitHub work into one or more bounded Herdr feature sessions across a repository or Project.
---

# Coordinate work

Use GitHub Issue, PR, and Projects as durable work truth. Herdr owns execution lifecycle; ThreadDock only connects GitHub evidence to top-level session state.

If this workflow needs any Herdr CLI operation, use **REQUIRED SUB-SKILL:** `herdr-local` first and satisfy its local-only gate before discovery, state inspection, or control. If the gate cannot be satisfied, leave the operation pending and report the blocker.

Read repository guidance and the relevant open GitHub work first. Split only independently deliverable features, keep dependencies explicit, and serialize the narrow shared interface or file boundary when features would otherwise collide.

Before starting anything new, inspect `~/.threaddock/sessions.json` and the actual Herdr state. Reuse an exact live Feature Leader session when its Issue/repository/worktree locator matches. Never guess session, workspace, tab, pane, Agent name, or cwd values; use values returned by the installed Herdr tool.

For a new feature, create a bounded top-level Feature Leader session with only the Issue URL, repository/target branch, feature goal and acceptance criteria, worktree, dependencies, and shared-boundary warnings. Planner/implementer/reviewer helpers belong inside that session as native subagents and are not top-level ThreadDock bindings.

When one or more bound Feature Leader sessions are still working and the coordinator has no immediate routing action, use **REQUIRED SUB-SKILL:** `agent-wait`. Periodically inspect only the bound top-level Herdr sessions and corresponding GitHub evidence. Do not inspect Feature Leader leaf agents, busy-poll, or create a separate scheduler.

Maintain the shared locator under one user-level exclusive lock such as `~/.threaddock/sessions.lock`: acquire lock, re-read the full JSON, mutate only the exact coordinator/feature binding, write a temporary file, then atomically rename it over `sessions.json`. Preserve every unrelated repository/project binding. If locking is unsafe or ambiguous, leave the file unchanged and report locator update pending.

Do not remove a feature binding for `idle`, `done`, or a temporary observation failure. Cleanup requires all three: Issue closed, related PR work finished, and top-level Herdr feature session absent.

**REQUIRED SUB-SKILL:** Use `publish-work` for durable GitHub updates and locator reconciliation.
