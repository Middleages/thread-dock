---
name: review-change
description: Use when a feature PR or candidate change needs an independent review before a human merge.
---

# Review change

Review the exact PR head from a fresh checkout or a read-only view. The reviewer is independent of the implementer and does not edit the candidate. Use GitHub's PR diff, changed files, checks, and review history plus focused local inspection when useful.

Check the Issue acceptance criteria, repository guidance, allowed feature scope, regressions, error handling, tests, documentation, and security or compatibility implications relevant to the change. Treat CI and local commands as evidence only for the exact commit they ran against. Every blocking finding names a file, symbol, behavior, or command and explains the consequence. Ask for a focused fix when the evidence is insufficient.

Submit the review through the normal GitHub review mechanism when authorized, or report a concise decision in the PR and Issue handoff. State `accept` only when acceptance criteria and required checks are satisfied; otherwise state `block` with concrete findings and the next check or fix. Do not merge, force-push, reset another agent's worktree, or update Project status as if review were complete. Humans perform the final merge.
