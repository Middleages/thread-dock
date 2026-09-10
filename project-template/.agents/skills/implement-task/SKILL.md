---
name: implement-task
description: Use when a feature leader or coding subagent is implementing an approved GitHub Issue feature in a bounded worktree.
---

# Implement task

Start from the linked Issue and its acceptance criteria. Inspect the repository guidance and current worktree before editing. Work only in the feature worktree and keep unrelated dirty changes intact. Do not require a Go contract, Invocation envelope, Artifact schema, publisher, or repair engine.

Implement the smallest coherent slice. Use a focused failing test first for production behavior when the repository has an applicable test seam; for docs/config-only work, run the appropriate parser, link check, or existing validation without inventing a test. Keep implementation, checks, and review evidence in the PR description or Issue handoff. Use the repository's normal native tools and preserve the existing authorization model.

Before committing, inspect the diff for accidental paths, run focused checks, and state any unavailable or unrun checks honestly. Commit on the feature branch with a useful message, push when authorized, and open or update the feature PR. A PR should identify the Issue, behavior changed, checks and outcomes, review status, and any follow-up. Never claim a check passed from a plan, transcript, or stale result.

If blocked, leave the worktree recoverable and record the exact blocker, attempted commands, and next decision in the Issue handoff. Discover every relevant Project membership and update each board's status, priority, and waiting fields using its actual options. Keep per-board failures distinct; continue the Issue and PR flow when one or all boards are unavailable. A coordinator session reports feature progress; it does not impersonate implementation or create a fake task record.
