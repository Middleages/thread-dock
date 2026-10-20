---
name: coordinate-work
description: Use when a project coordinator clarifies a user's goal and prepares feature briefs, dependencies, and Herdr assignments.
---

# Coordinate work

Use GitHub Issue, PR, and Projects as durable work truth. Herdr owns execution lifecycle; ThreadDock only connects GitHub evidence to top-level session state.

For Herdr CLI operations, first use `herdr-local`; preserve existing authorization and report concrete missing prerequisites.

Read repository guidance, the relevant open GitHub work, and [the shared development stance](../develop-feature/references/development-stance.md) first. Keep the user's authorization separate from plan readiness: a planning-only request permits planning/review helpers, not implementation, new Feature Leader sessions, or GitHub writes. Existing authorized independent work may continue; preserve active sessions and drafts rather than stopping or recreating them to adopt this workflow.

## Clarify the goal, then hand off a brief

The Coordinator owns goal, product scope, priority, dependencies, and shared constraints. The Feature Leader is the single owner of the feature's implementation plan; a converged project architecture plan is not a prerequisite for dispatch.

Before asking the user, inspect the relevant existing request, Issue, docs, or code. Classify what is missing:

- **Product intent:** competing readings would materially change the target user/workflow, scope, user-visible meaning, acceptance, data-loss behavior, or external effects. Ask only the unanswered material choice with concrete options and their consequences. Preserve the user's decision authority unless they delegated it.
- **Factual unknown:** the repository, Issue, or documentation can answer it. Inspect that source rather than asking the user to repeat discoverable facts.
- **Implementation choice:** architecture, file layout, algorithms, and test internals belong to the Feature Leader. Their absence does not make an otherwise clear goal ambiguous.
- **Harmless reversible assumption:** state the assumption and proceed within scope; revisit it if evidence changes.

Ask only when unresolved readings materially change acceptance or scope. Keep clear work moving; a risky migration still needs plan review even when no product question remains. Do not invent targets or turn examples into universal requirements. Small clear fixes need a short brief; ordinary questions and small documentation-only corrections can be handled directly without a new feature session.

The brief preserves the original request and source, confirmed desired behavior/acceptance/non-goals, facts versus assumptions and open product decisions, priority, dependencies/shared constraints, authority scope, and feature owner. Add repository/branch, Issue/worktree when applicable, and any already-agreed plan reference/revision. Hold only dispatch that depends on an unresolved product choice; continue useful independent authorized work and preserve active sessions/drafts. Do not require a new approval merely to pass an already-authorized brief.

If the user already agreed a plan, hand it over as the starting plan. The Feature Leader checks intake/feasibility and reuses it without rewriting or repeating approval/review of unchanged decisions. It may revise implementation details with reasons; cross-feature contracts/dependencies return to the Coordinator for narrow coordination, while an undelegated change to user-decided product scope or behavior returns to the user.

When a shared contract changes after dispatch, record the current document/commit reference, reason, affected features and owner in the existing GitHub record. Notify those Feature Leaders within authorization and collect their plan/task adjustment or evidence of no impact. Hold only dependent work until that response arrives; continue independent work. On handoff/resume, point to current decisions and unresolved dependencies. A routine unchanged task needs no new decision log, identifier scheme or acknowledgment round.

## Assign and observe

Before starting anything new, inspect `~/.threaddock/sessions.json` and the actual Herdr state. Reuse an exact live Feature Leader session when its Issue/repository/worktree locator matches. Never guess session, workspace, tab, pane, Agent name, or cwd values; use values returned by the installed Herdr tool.

For an authorized new feature whose goal and dispatch-relevant decisions are clear, create a bounded top-level Feature Leader session with the packet above. Apply [herdr-local's new top-level session settings](../herdr-local/SKILL.md#new-top-level-session-settings): explicitly preserve the authorized approval policy and sandbox at launch and verify the effective values before implementation dispatch. Planner/implementer/reviewer helpers belong inside that session as native subagents and are not top-level ThreadDock bindings.

When one or more bound Feature Leader sessions are still working and the coordinator has no immediate routing action, use **REQUIRED SUB-SKILL:** `agent-wait` for exact-target Herdr waiting and scoped GitHub evidence checks. Native child waits belong to their Feature Leader; do not inspect its leaf agents or create a separate scheduler.

Use `publish-work` for authorized GitHub updates and locator writes: it owns the exclusive-lock/atomic-write and cleanup rules. Herdr idle/done is not GitHub completion. For planning-only work without publication authorization, return the record in the requested local or conversational form.
