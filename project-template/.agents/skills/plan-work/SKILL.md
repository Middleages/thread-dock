---
name: plan-work
description: Use when a central coordinator or feature leader needs to turn a development request into an actionable GitHub plan.
---

# Plan work

Plan in GitHub terms. Read the request, the repository's `AGENTS.md`, `CONTEXT.md`, relevant architecture notes, and the current Issue or Project item when one exists. Use the connected `gh` CLI or GitHub MCP with the authorization already available to the session.

## Central coordinator

The central coordinator owns the project views. Discover every relevant Project membership for the Issue or request, read each board's schema and options, and keep status, priority, and waiting accurate per board. Split the request into independently deliverable **features**, identify dependencies, and assign each feature to a Herdr top-level session. A coordinator session may represent the overall request and does not need a fake task or worktree. Report feature assignments, blockers, and links back to the parent Issue and each Project.

## Feature leader

A feature leader owns one feature from detail planning through implementation and PR. Use native subagents for detail planning, coding, and an independent review; commit and open the feature PR from the feature worktree. Create a separate worktree for every independently writing feature. A reviewer may use a read-only checkout of the candidate. Keep shared interfaces and cross-feature dependencies explicit in the Issue before coding.

For an existing Issue, preserve its rationale and acceptance criteria and add decisions, assumptions, and a handoff section. If no Issue exists, draft one with:

- problem and rationale;
- acceptance criteria and out-of-scope behavior;
- repository, target branch, branch ownership, and dependencies;
- implementation, checks, and review expectations;
- decisions, blockers, and the next handoff.

Optional task sub-items are useful for a feature leader's checklist, but they are not a second orchestration protocol. Use the Issue and Project as the durable plan.

Before external writes, present the proposed Issue/Project changes when the session's authorization requires it. If the request is ambiguous, ask for the smallest missing decision or record a clearly labeled assumption in the Issue before dispatch. Query GitHub and the available Herdr help for existing repository identities, field option IDs, and session identifiers; choose a descriptive new branch name when the feature needs one.

## Reporting

The coordinator reports each feature's Issue URL, repository, branch, owner, status, priority, waiting reason (if any), PR URL, checks, review result, and next action. Keep absolute worktree paths and session, workspace, tab, or pane IDs in the optional local locator/local handoff. The feature leader reports GitHub facts in the Issue handoff and PR, then updates the Project. Humans merge PRs into `main`.
