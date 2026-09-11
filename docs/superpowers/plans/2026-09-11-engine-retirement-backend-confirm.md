# 미사용 backend confirmation 진입점 정리

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검사·self-review → fresh Sol Task 리뷰 → root 단일 gate·전체 리뷰 → PR 전달.

**Goal:** CLI 제거 후 production 호출자가 없는 두 backend ConfirmProtectedChange 진입점을 제거한다.
**Architecture:** Auto pass-through와 Orchestrator confirmation producer만 제거한다. 기존 protected state/comment/event/merge-gate 소비는 유지한다.
**Tech Stack:** Go 1.27.0, 기존 Go/Wails·React/Vite 유지.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## Global Constraints

- root/Sol이 두 exported method 제거 계약을 직렬 합의했다. 새 setter·대체 실행 API를 만들지 않는다.
- state/mergegate/CLI/config/contract/retirement/Monitor/runner/workrun 및 go.mod/go.sum은 변경하지 않는다.
- `invalidateLatestMainEvidence`, claim/append/Advance와 다른 active helper는 보존한다.
- 기존 codex-runtime dirty 4파일, worktree·Windows staging을 수정·삭제·재개하지 않는다.
- Luna high 단독 구현, fresh Sol medium 독립 리뷰. actual runtime identity는 미노출이면 unverified, 승격 금지.
- worker focused 검사만 수행하고 root 최종 make check는 별도 tmpfs에서 한 번 수행한다. 새 PR main 병합은 사용자에게 남긴다.

## 호출·보존 근거

기준 `f9fb7256e625e116493729c5accc390a2b8ec953`에서 production은 Auto를 Start/Advance/Stop 및
BeginRetirement/Advance로만 소비한다. Auto.ConfirmProtectedChange는 사용되지 않는 wrapper이며,
Orchestrator.ConfirmProtectedChange의 wrapper 외 호출은 전용 테스트 세 개뿐이다.

`ProtectedConfirmed`는 NeedsOperator 재개와 merge gate에서 소비한다. `ProtectedReasons`는 위험 분류·gate 입력·
comment 및 needs_operator payload에 쓰인다. `ProtectedCommentMarker/Posted`, comment 요청·reconcile,
needs_operator event와 persisted JSON 필드도 보존한다. confirmation 전용 intent/audit producer만 method와 함께 제거한다.

## Task packet

- taskId: `engine-retirement-slice7-backend-confirm`
- baseSHA: `f9fb7256e625e116493729c5accc390a2b8ec953`
- deps: PR #78·#80 main 병합, slice7 call inventory와 직렬 API 계약
- ownedPaths: `internal/orchestrator/parallel.go`, `internal/orchestrator/parallel_test.go`, `internal/orchestrator/final_review_test.go`, `.superpowers/sdd/engine-retirement-slice7/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-backend-confirm`
- branch: `agent/engine-retirement-backend-confirm`
- forbiddenPaths: 소유 파일의 지정 함수 밖과 모든 다른 경로. 특히 state/mergegate/CLI/cmd/config/docs/module/Monitor/runner/workrun/retirement 및 기존 실험.
- interface: `(*Auto).ConfirmProtectedChange`, `(*Orchestrator).ConfirmProtectedChange`만 제거. 다른 API와 persisted-state 소비는 유지한다.
- acceptance: 모든 `.ConfirmProtectedChange(` Go 참조 부재, 아래 테스트 보존/삭제 경계 준수, 새 helper·생산 setter·대체 runtime 없음.
- tests: 명시 Go 1.27.0·독립 tmpfs에서 `go test ./internal/orchestrator -run '^(TestParallelStories|TestPersistedProtectedConfirmationResumesMerge)$'`, `go test ./internal/mergegate -run '^(TestGateDecisionTable|TestProtectedReasonsClassifiesPathsAndValidRiskCategories|TestProtectedReasonsIgnoresInvalidPathsAndRiskCategories)$'`, `go vet ./internal/orchestrator ./internal/mergegate`, `go list ./...`, symbol/preservation 검색, `git diff --check`. 필터가 실제 테스트를 실행했는지 확인한다.
- result: changedFiles, 실제 commitSHA, 원문 executedCommands/TMPDIR, outcomes, unverified, blockers를 report에 기록한다.

## 테스트 변경 경계

- `TestProtectedConfirmationIsIdempotentAndResumesMerge`를 `TestPersistedProtectedConfirmationResumesMerge`로 좁힌다.
  protected run의 NeedsOperator 및 ListRecoverable 검증을 유지하고, 테스트 fixture snapshot의
  ProtectedConfirmed=true를 직접 Save한 뒤 기존 Advance로 Completed를 확인한다. 제거 API 호출과 terminal idempotence만 없앤다.
- `TestProtectedConfirmationRestartsLatestMainBeforeMerge`와 `TestProtectedConfirmationInvalidatesEvidenceBeforeResume`는 제거 API 전용이므로 삭제한다.
- 나머지 TestParallelStories/protected waits, gate protected/confirmed 사례, CLI negative guard,
  최근 고친 concurrent Advance 테스트 등은 수정하지 않는다. 테스트 파일 전체 삭제 금지.
- 새 테스트를 중복 추가하지 않고 기존 사례를 persisted-state 소비 검증으로 보존한다.

## 단계

- [ ] 두 진입점·전용 테스트를 정리하고 persisted-state 검증을 보존한다.
- [ ] focused 검사·self-review 후 commit하고 fresh Sol Task 리뷰를 통과한다.
- [ ] root가 단일 통합 gate·전체 리뷰 후 PR을 준비한다.

[ledger](../../operator/2026-09-11-engine-retirement-slice7-ledger.md)에 검증과 GitHub 기록을 남긴다.
