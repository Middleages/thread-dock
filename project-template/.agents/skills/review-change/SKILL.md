---
name: review-change
description: Use when a candidate Task change needs a fresh read-only Reviewer decision before the merge gate.
---

# Review change

1. Obtain a fresh read-only capability, separate from the Builder. The `Invocation` envelope top level is exactly `requestId`, `role: reviewer`, `profileId`, canonical `worktree`, `outputSchema: thread-dock.reviewer-result.v1`, `readOnly: true`, and `packet`; reject stale or mismatched identity.
2. Inside `packet`, require `taskId`, `candidateSha`, `treeSha`, `changedFiles`, `patch`, `workAcceptanceCriteria`, `taskAcceptanceCriteria`, and `gate` containing `commands` and `outcomes`. If an orchestrator supplies `repairBudget` context, consume it only to assess exhaustion; omit it from the Invocation top level, the fixed typed ReviewPacket, and the Artifact. Review the fixed candidate against Task ownership, allowed paths, acceptance criteria, regressions, protected changes, and gate evidence; every blocking finding names path or command evidence.
3. Return one strict `Artifact` envelope with `requestId`, `role`, `status`, and a bounded typed Reviewer result: exact JSON `reviewedSha` (the reviewedSHA), `decision: accept|block`, and `blockingFindings` entries with `code` and `diagnostic`. An `accept` has no blocking findings; a `block` has at least one concrete finding.
4. Perform no mutation or GitHub write. Treat repair budget as consume-only context; Go owns repair budget reporting and state transitions, including exhaustion handling. Never increment or reset it in the Reviewer Artifact; at exhaustion, preserve the block for operator review.
