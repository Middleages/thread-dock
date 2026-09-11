---
name: review-change
description: Use when a feature PR or candidate change needs an independent review before a human merge.
---

# Review change

Review the exact candidate SHA or PR head from a fresh read-only context. The reviewer is independent of the implementer, is a transient native subagent rather than a ThreadDock top-level binding, and does not edit the candidate.

Check the linked Issue acceptance criteria, repository guidance, allowed feature scope, regressions, error handling, tests, documentation, and relevant security or compatibility implications. Treat CI and local commands as evidence only for the exact SHA/environment they ran against; do not rerun an already trustworthy identical check without a concrete reason.

For every acceptance criterion, identify the supporting diff/check evidence or state what is missing. Every blocking finding must name a concrete file, symbol, behavior, or command and explain the consequence. Do not block on preference-only cleanup that is outside acceptance or material risk.

Return `accept` only when required acceptance and checks are supported. Otherwise return `block` with ordered findings and the smallest next fix/check. Do not merge, force-push, reset another worktree, change Project status as if review were complete, or silently repair the implementation yourself. Human merge remains final.
