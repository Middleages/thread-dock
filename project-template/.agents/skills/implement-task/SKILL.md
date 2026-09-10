---
name: implement-task
description: Use when a Builder must implement one approved Contract v2 Task in a bounded Worktree with workspace-write access.
---

# Implement task

1. Accept only the exact Builder `Invocation` packet. Confirm `requestId`, `role: builder`, `profileId` (the fixed logical profile), canonical Worktree (`worktree`) path, bounded `packet` (`taskId`, `allowedPaths`, acceptance criteria, dependencies, verification), output schema (`outputSchema`), `readOnly: false`, and `workspace-write` intent. Reject a missing, stale, or mismatched identity.
2. Inspect the assigned Worktree for unrelated or dirty changes and preserve them. Write only paths in the Task's `allowedPaths`; never widen scope or touch protected/unassigned paths. Use TDD: add a focused failing test, observe the expected RED failure, make the smallest change, then run focused self-checks.
3. Return one strict `Artifact` envelope containing `requestId`, `role`, `status` (`success` or `failure`), and `result` as a bounded typed Builder result. Report only focused TDD/self-check claims (the result's `verification` entries and any `commitSha` are claims). Go owns path checks, staging, commit, Git integrity, and authoritative verification; the Builder does not stage or commit and does not promote its checks to a Task Gate.
4. Emit no prose or legacy fields (`task_id`, `changed_paths`, `checks`, `commit_sha`). On an implementation blocker, return the same envelope with a failure status and bounded diagnostic; preserve edits and the exact blocker for Go to reconcile.
