---
name: review-change
description: Use when a feature PR or candidate change needs an independent review before a human merge.
---

# Review change

This is **code-review mode** for an implemented candidate. For a plan-review or adjudication assignment, use the corresponding input/evidence contract in `grill-plan` instead; future screenshots or passing implementation tests are not prerequisites for a plan verdict.

Review the exact candidate SHA or PR head from a fresh read-only context. The reviewer is independent of the implementer, is a transient native subagent rather than a ThreadDock top-level binding, and does not edit the candidate.

Read [the shared development stance](../develop-feature/references/development-stance.md) and assess added complexity against a concrete requirement or failure case. For UI changes, follow [UI quality](../develop-feature/references/ui-quality.md): inspect the rendered screenshots through image input and assess interaction evidence for changed behavior. Missing required visual/interaction evidence is unverified UI acceptance, even when build/DOM/capture succeeded. Return missing evidence to the Leader; do not delegate, edit, or silently replace required checks.

Check the linked Issue acceptance criteria, repository guidance, allowed feature scope, regressions, error handling, tests, documentation, and relevant security or compatibility implications. Treat CI and local commands as evidence only for the exact SHA/environment they ran against; do not rerun an already trustworthy identical check without a concrete reason.

Review complexity and diagnostics for outcome risk, not taste. Flag an unnecessary layer, setting, validator, or duplicated source of truth only when it can block a valid normal path, create divergent behavior, or increase recovery cost. Check that logs retain the operation, target, failure point, and confirmed cause needed to diagnose a failed task; connect missing context to the concrete recovery or support risk. Preserve necessary safety checks and report preference-only differences without blocking.

For every original acceptance criterion, link supporting diff/check evidence or state the missing implementation/check. Compare scope and shared-contract revisions with the current source decisions. Flag silently weakened acceptance, removed assertions, skipped tests or lowered gates used to obtain a pass. Every blocking finding names a concrete file, symbol, behavior or command, its consequence and the smallest fix/check; the Leader routes that work through the existing implementation loop.

When native review receipts are spread across a supplied Codex/OpenCode event artifact and manual extraction is repeating, the optional read-only collector in [review evidence receipts](references/review-evidence-receipts.md) can summarize explicit child metadata. Its output is observation evidence only: it never establishes review quality, acceptance, readiness, or the revision actually reviewed. Read the reference only for that collection task.

Return `accept` only when required acceptance and checks are supported. Otherwise return `block` with ordered findings and the smallest next fix/check. Do not merge, force-push, reset another worktree, change Project status as if review were complete, or silently repair the implementation yourself. Human merge remains final.
