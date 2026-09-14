---
name: tdd-task
description: Use when an implementation subagent receives one bounded coding task with clear acceptance criteria and an isolated feature worktree.
---

# TDD task

Implement only the bounded task in the provided feature worktree. Keep assigned paths, dependencies, and shared-interface ownership intact; return any public or shared interface change outside the packet to the Feature Leader for replanning. Do not delegate, publish, or widen scope.

For product behavior with a test seam, use RED -> GREEN -> REFACTOR: add one focused failing test for the missing behavior, verify the intended failure, implement the smallest change, verify the focused test passes, then clean up without changing behavior. For docs/config-only work, use the repository parser, schema check, link check, or existing validation instead of inventing a fake test. Logs should retain the operation, target, failure point, and confirmed cause needed for diagnosis.

Run focused checks for changed behavior and affected dependencies. Do not run a worker-level full suite. Inspect the final diff for accidental paths and return changed files, exact commands, outcomes, unverified checks, blockers, and a candidate commit SHA when available.

Never report a planned, stale, or ambiguous check as passed.
