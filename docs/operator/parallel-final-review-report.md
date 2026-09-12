> **옛 실행 엔진의 역사 자료.** 이 문서의 agentctl·pilot·runtime 절차는 현재 제품 실행 방법이 아닙니다. 소스 제거는 [완료 계획](../superpowers/plans/2026-09-12-engine-retirement-completion.md)에서 추적하며, 현재 사용법은 [GitHub-first 빠른 시작](github-first-quickstart.md)을 따릅니다. 과거 명령·경로는 재실행 지시가 아닌 당시 증거입니다.

# Parallel Integration Final Review Report

This branch closes the final integration review findings for the supervised
parallel run.

- Ready-for-review uses one GraphQL `markPullRequestReadyForReview` mutation
  with the persisted pull-request node ID. Create/read responses that omit the
  node ID are reconciled through a separate pull-request observation.
- Protected confirmation is audited before the snapshot write, invalidates
  latest-main, Full Suite, check, and mergeability evidence, and resumes at
  the latest-main refresh. A unique, secret-free PR comment marker is
  paginated/reconciled before `needs_operator` is persisted.
- Run snapshots capture immutable repository owner/name/default branch values.
  Revert composition uses those values, the persisted main merge SHA as the
  target identity, and the exact fetched default-branch head as its worktree
  base after an ancestry check.
- Confirmed task/latest-main conflicts attempt `AbortMerge`; abort failures
  remain in the audit stream and blocked summary.
- Recovery recomputes the Builder Worktree fingerprint in a separate action,
  acknowledges a changed fingerprint before returning to evidence observation,
  and permits three completed recovery actions. A fourth unchanged evaluation
  blocks the run; progress resets the count.
- CI repair preserves check name/state and consumes repair budget only when a
  check name uniquely matches one Task verification command. Repair packets
  use the failing final SHA as `RepairBaseSHA`.
- Protected stories use valid `riskCategories` such as `authentication` or
  `public_contract`; tests do not widen `allowedPaths` to bypass contract
  validation. Terminal fallback detection requires the exact
  `herdr-terminal:` prefix.

Verification commands and their fresh results are recorded in the handoff
message for this branch; no live GitHub or Herdr effects are performed by the
integration tests.
