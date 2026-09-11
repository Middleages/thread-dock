---
name: tdd-task
description: Use when an implementation subagent receives one bounded coding task with clear acceptance criteria and an isolated feature worktree.
---

# TDD task

Implement only the assigned bounded task in the provided feature worktree. Do not spawn additional agents or widen scope.

**REQUIRED SUB-SKILL:** Use `superpowers:test-driven-development` when the task changes production behavior and an applicable test seam exists.

For applicable product behavior, follow RED -> GREEN -> REFACTOR: write one focused failing test, verify it fails for the intended missing behavior, implement the smallest change, verify the focused test passes, then clean up without changing behavior. For docs/config-only work, use the repository's parser, schema check, link check, or existing validation rather than inventing a fake test.

Respect assigned paths, dependencies, and shared-interface ownership. If the task requires a public/shared interface change outside the packet, stop and return that need to the Feature Leader for replanning.

Run only focused checks for the changed behavior and affected dependencies; do not run a worker-level full suite unless explicitly required by the Feature Leader for a concrete reason. Inspect the final diff for accidental paths.

Return: changed files, exact commands, outcomes, unverified checks, blocker if any, and candidate commit SHA when available. Never report a planned or stale check as passed.
