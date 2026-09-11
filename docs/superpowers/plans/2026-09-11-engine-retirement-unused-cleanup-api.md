# 미사용 retirement·cleanup API 정리

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검사·self-review → fresh Sol Task 리뷰 → root 단일 gate·전체 리뷰 → 후속 PR 전달.

**Goal:** 옛 compatibility alias와 호출자 없는 bulk cleanup 후보 조회 API를 작은 묶음으로 제거한다.
**Architecture:** 현재 exact-ID retire/cleanup 실행기·composite inspector·state persistence는 유지하고, 진입 caller 없는 API만 줄인다.
**Tech Stack:** Go 1.27.0, 기존 Go/Wails·React/Vite.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## Global Constraints

- PR #82의 검토된 head `00cd56167a512ea784faeba8fb4ae303ab22dfde`가 base다. 새 PR은 `agent/engine-retirement-slice7`을 대상으로 하며 #82 병합 후 main으로 전환한다.
- root/Sol이 아래 세 symbol 제거를 직렬 합의했다. 그 밖의 shared interface는 변경하지 않는다.
- CLI/cmd/config/orchestrator/retirement/Monitor/runner/module 파일과 기존 실험·worktree·Windows staging은 변경하지 않는다.
- 새 실행기·setter·대체 helper나 Node/HTTP 경로를 만들지 않는다.
- Luna high 단독 구현과 fresh Sol medium 리뷰를 사용한다. actual runtime identity는 미노출이면 unverified, 승격 없음.
- worker는 focused 검사만, root는 tmpfs·VITEST_MAX_WORKERS=1로 최종 make check를 한 번 수행한다. main 병합은 사용자에게 남긴다.

## 호출·보존 근거

`internal/worktree/retirement_composite.go`의 `RetirementInspector` type alias와 `NewRetirementInspector`는
정의 외 production/test/config/docs 참조가 없다. 현재 cmd 구성과 composite 테스트는 `NewCompositeRetirementInspector`를 사용한다.
`internal/state/store.go`의 `ListCleanupCandidates`는 전용 self-test 하나 외 caller가 없다.

실제 `agentctl cleanup RUN`은 CLI RunService가 정확한 RUN을 Load하고 PhaseCompleted 및
UpdatedAt < now-7d를 독립적으로 검사한다. 이 경로와 7일 guard는 그대로 둔다.
Composite inspector의 root/proof/removal, Store persistence/ListRecoverable, `snapshotIDs` helper와
진단용 RetirementRuntime도 보존한다. 삭제 후 store.go에서 불필요한 time import만 제거한다.

## Task packet

- taskId: `engine-retirement-slice8-unused-cleanup-api`
- baseSHA: `00cd56167a512ea784faeba8fb4ae303ab22dfde`
- deps: PR #82 완료 구현, slice8 caller inventory, 위 직렬 API 계약
- ownedPaths: `internal/worktree/retirement_composite.go`, `internal/state/store.go`, `internal/state/store_test.go`, `.superpowers/sdd/engine-retirement-slice8/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-unused-cleanup-api`
- branch: `agent/engine-retirement-unused-cleanup-api`
- forbiddenPaths: 소유 밖 모든 경로와 지정 symbol/test 밖의 동작. 특히 CLI/cmd/config/orchestrator/retirement/docs/Monitor/go.mod/go.sum 및 다른 worktree.
- interface: 세 미사용 symbol만 제거. exact-ID cleanup/retire, current constructor/inspector, state persistence/ListRecoverable와 진단 계약은 불변이다.
- acceptance: 세 symbol 참조 0, `TestListCleanupCandidatesOnlyReturnsOldCompletedRunsWithoutDeleting`만 제거, snapshotIDs 및 recoverable tests 보존. 파일 전체 삭제·로직 이동·새 테스트 추가 없음.
- tests: 명시 Go 1.27.0·별도 tmpfs로 아래 existing focused 검사를 실행하고 실제 사례 실행을 확인한다. vet/list/symbol/preservation/diff도 검사한다. worker full suite 금지.
- result: changedFiles, 실제 commitSHA, 원문 executedCommands/TMPDIR, outcomes, unverified, blockers를 report에 기록한다.

## Focused 검사

```sh
go test ./internal/state -run '^TestListRecoverableSortsNewestFirstAndExcludesTerminalRuns$'
go test ./internal/worktree -run '^TestCompositeRetirementInspector(SelectsManagedOrHerdrRoot|RejectsOverlapOutsideAndSymlink)$'
go test ./internal/cli -run '^TestCleanup'
go test ./cmd/agentctl -run '^TestProductionDependenciesUseSnapshotRepositoryAndCompositeRetirementRoots$'
go vet ./internal/state ./internal/worktree ./internal/cli ./cmd/agentctl
go list ./...
git diff --check
```

## 단계

- [x] 세 symbol과 전용 테스트만 제거하고 active 경계를 보존했다.
- [x] focused 검사·self-review 후 commit했고 `a1e7da3`에서 fresh Sol Task ACCEPT를 받았다.
- [x] root 최종 gate `23906ad` 통과·전체 리뷰 `bfbd6dc` ACCEPT 후 PR #82 위의 후속 PR을 준비했다. 실패·환경 대조 근거는 ledger에 보존한다.

[ledger](../../operator/2026-09-11-engine-retirement-slice8-ledger.md)에 검증과 PR 의존성을 기록한다.
