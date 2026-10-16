---
name: coordinate-work
description: Use when a project coordinator clarifies a user's goal and prepares feature briefs, dependencies, and Herdr assignments.
---

# Coordinate work

Use GitHub Issue, PR, and Projects as durable work truth. Herdr owns execution lifecycle; ThreadDock only connects GitHub evidence to top-level session state.

For Herdr CLI operations, use **REQUIRED SUB-SKILL:** `herdr-local` for installed syntax, managed context, actual target identity, and any explicit network constraints. Existing authorization carries forward; report only a concrete missing prerequisite for the affected operation.

Read repository guidance, the relevant open GitHub work, and [the shared development stance](../develop-feature/references/development-stance.md) first. Keep the user's authorization separate from plan readiness: a planning-only request permits planning/review helpers, not implementation, new Feature Leader sessions, or GitHub writes. Existing authorized independent work may continue; preserve active sessions and drafts rather than stopping or recreating them to adopt this workflow.

## Clarify the goal, then hand off a brief

The Coordinator owns goal, product scope, priority, dependencies, and shared constraints. The Feature Leader is the single owner of the feature's implementation plan; a converged project architecture plan is not a prerequisite for dispatch.

Before asking the user, inspect the relevant existing request, Issue, docs, or code. Classify what is missing:

- **Product intent:** competing readings would materially change the target user/workflow, scope, user-visible meaning, acceptance, data-loss behavior, or external effects. Ask only the unanswered material choice with concrete options and their consequences. Preserve the user's decision authority unless they delegated it.
- **Factual unknown:** the repository, Issue, or documentation can answer it. Inspect that source rather than asking the user to repeat discoverable facts.
- **Implementation choice:** architecture, file layout, algorithms, and test internals belong to the Feature Leader. Their absence does not make an otherwise clear goal ambiguous.
- **Harmless reversible assumption:** state the assumption and proceed within scope; revisit it if evidence changes.

Words such as "better", "faster", or "automate" are not by themselves a reason to start a questionnaire: existing context may already define the desired result. If two plausible readings still imply materially different acceptance or scope, ask a small number of focused questions; otherwise prepare the brief. Do not invent numeric targets or turn a user's example into a universal requirement. Risk and ambiguity are separate: route a clear risky migration to the Feature Leader's independent plan review without another product question or a Coordinator-authored architecture plan. Small clear fixes need a direct brief, not `grill-plan`.

The brief preserves the original request and source, confirmed desired behavior/acceptance/non-goals, facts versus assumptions and open product decisions, priority, dependencies/shared constraints, authority scope, and feature owner. Add repository/branch, Issue/worktree when applicable, and any already-agreed plan reference/revision. Hold only dispatch that depends on an unresolved product choice; continue useful independent authorized work and preserve active sessions/drafts. Do not require a new approval merely to pass an already-authorized brief.

If the user already agreed a plan, hand it over as the starting plan. The Feature Leader checks intake/feasibility and reuses it without rewriting or repeating approval/review of unchanged decisions. It may revise implementation details with reasons; cross-feature contracts/dependencies return to the Coordinator for narrow coordination, while an undelegated change to user-decided product scope or behavior returns to the user.

## Assign and observe

Before starting anything new, inspect `~/.threaddock/sessions.json` and the actual Herdr state. Reuse an exact live Feature Leader session when its Issue/repository/worktree locator matches. Never guess session, workspace, tab, pane, Agent name, or cwd values; use values returned by the installed Herdr tool.

For an authorized new feature whose goal and dispatch-relevant decisions are clear, create a bounded top-level Feature Leader session with the packet above. Planner/implementer/reviewer helpers belong inside that session as native subagents and are not top-level ThreadDock bindings.

When one or more bound Feature Leader sessions are still working and the coordinator has no immediate routing action, use **REQUIRED SUB-SKILL:** `agent-wait`. Periodically inspect only the bound top-level Herdr sessions and corresponding GitHub evidence. Do not inspect Feature Leader leaf agents, busy-poll, or create a separate scheduler.

Maintain the shared locator under one user-level exclusive lock such as `~/.threaddock/sessions.lock`: acquire lock, re-read the full JSON, mutate only the exact coordinator/feature binding, write a temporary file, then atomically rename it over `sessions.json`. Preserve every unrelated repository/project binding. If locking is unsafe or ambiguous, leave the file unchanged and report locator update pending.

Do not remove a feature binding for `idle`, `done`, or a temporary observation failure. Cleanup requires all three: Issue closed, related PR work finished, and top-level Herdr feature session absent.

**REQUIRED SUB-SKILL:** Use `publish-work` for durable GitHub updates and locator reconciliation within existing write authorization. For planning-only work without publication authorization, return the brief/plan record in the requested local or conversational form instead.
