---
name: review-change
description: Use when a candidate Task change needs a fresh read-only Reviewer decision before the merge gate.
---

# Review change

1. Obtain a fresh read-only capability, separate from the Builder. Accept only an exact Reviewer `Invocation` packet containing `requestId`, `role: reviewer`, fixed logical `profile`, canonical read-only Worktree, `candidateSha`, diff, Go `gate` evidence, acceptance `criteria`, and current shared `repairBudget`. Reject stale or mismatched identity.
2. Review the fixed candidate SHA against Task ownership, allowed paths, acceptance criteria, regressions, protected changes, and gate evidence. Every blocking finding names a path or command evidence. Do not inspect or reuse an earlier Reviewer conclusion as current evidence.
3. Return one strict `Artifact` envelope with `requestId`, `role`, `status`, and a bounded typed Reviewer result: `decision` `accept` or `block`, `blockingFindings`, and exact `reviewedSHA` (JSON `reviewedSha`). An `accept` has no blocking findings; a `block` has at least one concrete finding.
4. Perform no mutation or GitHub write. Treat the repair budget as packet state: report the remaining count or exhaustion, but never increment, reset, or self-manage it. Go records the decision and schedules any repair; at exhaustion, preserve the block for operator review.
