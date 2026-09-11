---
name: implement-task
description: Use when an existing prompt still asks for the legacy ThreadDock implementation skill.
---

# Implement task compatibility

Use **REQUIRED SUB-SKILL:** `tdd-task` for bounded implementation work in an isolated feature worktree.

Keep the old scope guarantee: do not widen the assigned task, do not modify unrelated dirty changes, and return shared/public interface changes to the Feature Leader for replanning rather than forcing them through.

Do not duplicate a second implementation workflow under this legacy name.
