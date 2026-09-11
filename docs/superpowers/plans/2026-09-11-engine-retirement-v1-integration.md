# 미사용 v1 integration 모듈 정리

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검사·self-review → fresh Sol Task 리뷰 → root 단일 통합 gate·전체 리뷰 → PR 전달.

**Goal:** 호출자가 없는 옛 `internal/integration` 구현과 전용 테스트를 제거한다.
**Architecture:** 미사용 v1 package만 제거하며 현재 `internal/workrun.ReviewIntegrationService`와 Go/Wails Monitor를 보존한다.
**Tech Stack:** Go 1.27.0, 기존 Go/Wails·React/Vite.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## Global Constraints

- root/Sol이 미사용 package API 제거를 직렬 합의했다. 바깥의 shared API와 사용자 CLI 계약은 유지한다.
- workrun/state/worktree/coordinator/orchestrator/CLI/Monitor/runner/GitHub/config/go.mod/go.sum은 변경하지 않는다.
- 새 engine·Node/HTTP 경로를 만들지 않는다. 기존 사용자 실험·worktree·Windows staging은 수정·삭제·재개하지 않는다.
- Luna high 단독 소유와 fresh Sol medium 리뷰를 사용한다. 실제 runtime identity는 미노출이면 unverified, 승격 금지.
- worker는 focused 검사만 하고 root는 별도 tmpfs에서 마지막 make check 한 번을 수행한다.
- 승인된 GitHub 기록·push는 진행하며 새 PR main 병합은 사용자에게 남긴다.

## 삭제 전 근거

기준 `5f5b31e519074f599a1d951cf6c7278e88a4cfef`에서 `internal/integration`은 production importer와
package 밖 symbol caller가 없다. `integrator.go` 182줄과 `integrator_test.go` 233줄의 전용 구현/검사만 남았다.
현재 통합은 workrun의 `NewReviewIntegrationService`/`ReviewIntegrationService.Advance`와 state/v2가 수행한다.
worktree의 `MergeCommitNoFF`, `AbortMerge`, `RunChecks`, `CurrentCommit` 등 실제 소비 경로는 보존한다.

`internal/workrun/review_integration_test.go`의 `integration.Advance`는 지역 변수 사용이며 삭제 package import가 아니다.
config/fixture/scripts 참조는 없고 과거 계획의 경로 언급은 역사 기록으로 보존한다.
confirm/retire/cleanup은 다른 실행 상태·설정에 연결돼 더 넓으므로 이번에 함께 제거하지 않는다.

## Task packet

- taskId: `engine-retirement-slice5-v1-integration`
- baseSHA: `5f5b31e519074f599a1d951cf6c7278e88a4cfef`
- deps: PR #76 병합, slice5 importer/caller 조사, 위 package API 제거 계약
- ownedPaths: `internal/integration/integrator.go`, `internal/integration/integrator_test.go`, `.superpowers/sdd/engine-retirement-slice5/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-v1-integration`
- branch: `agent/engine-retirement-v1-integration`
- forbiddenPaths: 소유 밖 모든 경로와 다른 worktree. 특히 현재 workrun integration 서비스와 shared Git/state/CLI/Monitor/module 파일.
- interface: 옛 package의 Integrator·생성/검증/통합 API 및 전용 타입/오류를 제거한다. package 밖 계약은 그대로다.
- acceptance: 두 파일 제거, importer 부재, 현재 ReviewIntegrationService 및 Git helper 보존. 다른 파일로 로직/테스트 이동이나 새 기능 추가 없음.
- tests: 명시한 Go 1.27.0과 별도 tmpfs TMPDIR에서 `go test ./internal/workrun -run '^TestReviewIntegration'`, `go vet ./internal/workrun`, `go list ./...`, `git diff --check`. `test ! -d internal/integration`; `rg -n 'thread-dock/internal/integration|integration\.(New|ValidateResult|MergeResults)' --glob '*.go' .`는 exit 1 예상. 단순 삭제를 고정하는 새 테스트는 만들지 않는다.
- result: changedFiles, 실제 commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록한다.

## 단계

- [ ] 참조·보존 경계를 확인하고 전용 두 파일을 제거한다.
- [ ] 현재 통합 경로의 기존 focused 검사와 self-review 후 commit한다.
- [ ] fresh Sol Task 리뷰를 통과한다.
- [ ] root 단일 통합 gate와 전체 리뷰를 통과해 PR을 준비한다.

[진행 ledger](../../operator/2026-09-11-engine-retirement-slice5-ledger.md)에 검증·GitHub 근거를 기록한다.
