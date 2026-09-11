# 옛 confirm CLI 연결 제거

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검사·self-review → fresh Sol Task 리뷰 → root 단일 gate·전체 리뷰 → 후속 PR 전달.

**Goal:** 옛 `agentctl confirm RUN protected-change` CLI adapter/wiring을 제거한다.
**Architecture:** CLI 진입점과 주입 interface만 제거하고 orchestrator 내부 protected-change 구현·상태·테스트를 보존한다.
**Tech Stack:** Go 1.27.0, 기존 Go/Wails·React/Vite 유지.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## Global Constraints

- PR #78 head `5f3d2b6b22022479fd5a19836a4b2c80bb734586`를 base로 사용한다. #78은 아직 main에 병합되지 않았으므로 새 PR은 해당 branch를 기준으로 한다.
- root/Sol이 `cli.ProtectedChangeConfirmer`, `cli.Dependencies.Confirmer` 제거를 직렬 합의했다.
- orchestrator의 ConfirmProtectedChange·protected state/event·merge-gate·테스트와 retire/cleanup은 보존한다.
- Monitor/runner, config/state/contract, go.mod/go.sum을 변경하거나 새 엔진·Node/HTTP 경로를 추가하지 않는다.
- 기존 codex-runtime dirty 4파일, 기존 worktree·Windows staging은 수정·삭제·재개하지 않는다.
- Luna high 단독 구현, fresh Sol medium 독립 리뷰. actual runtime identity는 미노출이면 unverified, 승격 없음.
- worker는 focused 검사만, root의 마지막 make check는 별도 tmpfs에서 한 번 수행한다. main 병합은 사용자에게 남긴다.

## 호출·보존 근거

confirm adapter는 28줄이며 CLI route/usage, Dependencies와 production dependency 분류, cmd 구성·repository/GHES 분기에 연결된다.
전용 injected-service tests는 제거하되 기존 create-revert 거부 검증은 보존한다.
고유 config field/fixture는 없다. retire/cleanup은 state·Herdr/worktree·설정·pilot과 넓게 연결돼 이번 범위에 포함하지 않는다.
`internal/pilot`, `internal/projecttemplate`은 실제 script/Skill 계약을 검사하므로 production importer가 없어도 삭제하지 않는다.

## 구현 Task packet

- taskId: `engine-retirement-slice6-confirm-cli`
- baseSHA: `5f3d2b6b22022479fd5a19836a4b2c80bb734586`
- deps: PR #78의 완료된 구현, slice6 조사, 위 직렬 interface 결정
- ownedPaths: `internal/cli/confirm.go`(삭제), `internal/cli/confirm_test.go`, `internal/cli/run.go`, `internal/cli/run_commands.go`, `cmd/agentctl/main.go`, `cmd/agentctl/main_test.go`, `.superpowers/sdd/engine-retirement-slice6/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-confirm-cli`
- branch: `agent/engine-retirement-confirm-cli`
- forbiddenPaths: 소유 밖 모든 경로. 특히 run_commands_test.go, orchestrator/state/contract/retirement/Monitor/runner/config/module, 운영 문서 및 다른 worktree.
- interface: 위 두 CLI shared symbol과 confirm route/usage/production wiring만 제거. 올바른 형태와 잘못된 confirm 호출 모두 dependency-free usage, exit 2, stdout empty이며 config/token/repository 탐색을 시작하지 않는다.
- acceptance: fakeConfirmer와 전용 injected-service tests 제거, confirm_test.go에서 새 거부 계약과 confirm/create-revert 모두 production-deps=false 검증. cmd의 no-setup table에서 confirm/create-revert 모두 credential=false·discovery 0 확인. 나머지 명령과 orchestrator는 그대로.
- tests: 기존 파일의 meaningful negative 사례를 먼저 바꿔 RED 확인 후 구현한다. 명시 Go 1.27.0·별도 tmpfs로 `go test ./internal/cli ./cmd/agentctl`, `go vet ./internal/cli ./cmd/agentctl`, `go list ./...`, `git diff --check` 및 CLI type/route 참조 검사. 기존 create-revert 거부 검증을 없애지 않는다.
- result: changedFiles, 실제 commitSHA, executedCommands(원문 TMPDIR 선언 포함), outcomes, unverified, blockers를 report에 기록한다.

## root 문서 Task

root는 `parallel-pilot.md` story 3의 confirm 실행 안내만 제거된 명령의 역사 설명으로 바꾼다.
나머지 stories와 retirement/evidence guard는 그대로 둔다. 직접 영향 테스트
`TestParallelPilotRunbookProtectsUnacceptedEvidenceFromRetirement`를 별도 focused 검사한다.
이 문서와 HANDOFF/plan/ledger는 root 소유이며 worker가 수정하지 않는다.

## 단계

- [x] 거부·의존성 미초기화 회귀 사례를 먼저 작성하고 adapter/wiring을 제거했다.
- [x] focused 검사·self-review 후 commit했고 `7234f66`에서 fresh Sol Task ACCEPT를 받았다.
- [x] root 문서의 직접 영향 pilot 테스트를 통과했다. 후속 역사 설명까지 최종 gate에서 확인한다.
- [x] 최초 gate의 기존 테스트 결함을 수정한 뒤 `98861c8`의 최종 gate와 `60df27d`의 전체 리뷰를 통과하고 PR #78 위의 후속 PR을 준비했다.

## 통합 gate에서 발견한 기존 테스트 결함

최초 gate `80c9a0d`는 기존 동시 Advance 테스트의 overlap 미보장으로 실패했다. 제품 lock/claim과 CLI 변경에는
원인 연결이 없으며, 별도 Luna Task가 `TestRound1ConcurrentAdvanceSerializesOneAction`의 동기화만 수정한다.
정확한 소유 경로·focused 반복·새 gate 실행 근거는 ledger의 `slice6-gate-concurrency-test-fix` packet을 따른다.

[ledger](../../operator/2026-09-11-engine-retirement-slice6-ledger.md)에 검증 SHA·결과·PR 의존성을 기록한다.
