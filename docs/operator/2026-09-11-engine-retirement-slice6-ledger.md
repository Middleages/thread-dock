# 여섯 번째 엔진 정리 ledger

## 기준·의존성·보존

- baseSHA: `5f3d2b6b22022479fd5a19836a4b2c80bb734586`, 검토와 검증을 마친 PR #78의 head.
- 이번 착수 시 PR #78은 OPEN, main/origin/main은 `5f5b31e519074f599a1d951cf6c7278e88a4cfef`다. 새 main 병합을 수행하지 않았다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice6`, branch `agent/engine-retirement-slice6`.
- 새 PR은 `agent/engine-retirement-slice5`를 base로 두어 이번 변경만 검토한다. PR #78의 main 병합 후 후속 PR을 main으로 전환하는 순서를 전달한다.
- codex-runtime의 중단된 cmd/agentctl main.go/main_test.go 및 internal/config config.go/config_test.go는 보존했다. 기존 worktree·Windows staging은 삭제하지 않는다.
- Monitor/runner와 현재 Go/Wails·React 제품 경계를 유지하고 새 실행 엔진·Node/HTTP 경로를 만들지 않는다.

## 조사 Task packet

- taskId: `engine-retirement-slice6-inventory`
- baseSHA: `5f3d2b6b22022479fd5a19836a4b2c80bb734586`
- deps: PR #78의 integration package 삭제
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice6`
- branch: `agent/engine-retirement-slice6` (조사는 읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: 남은 engine의 가장 작은 제거 계약부터 제안
- acceptance: caller/config/test/docs 영향과 정확한 소유 경로·focused 검사 증명. 완료된 다섯 slice 재구현 금지.
- tests: rg/source/go list 등 조회만
- result: changedFiles `[]`, commitSHA 없음. executedCommands/outcomes/unverified/blockers는 조사 결과에 기록한다.

## 역할과 검증

root는 문서·GitHub·계약·통합, Sol medium은 계획·조사, Luna high는 구현, fresh Sol medium은 리뷰를 맡는다.
실제 runtime model/effort는 미노출이면 unverified이며 승격하지 않는다. worker 상한 3, CLI worker 예약은 해제했고 gate test fix에 Luna 1개를 예약했다. reviewer 슬롯 확보.
이미 완료된 slice의 결과는 재사용하고 새로운 Task에 필요한 focused 검사만 한다. root의 마지막 통합 make check는
별도 tmpfs TMPDIR `/dev/shm/threaddock-slice6-gate.OKTr3F`에서 한 번 수행한다. npm ci exit 0, lock 변경 없음.
Windows native·오류 상태·현재 보드 native 실행·Herdr/Projects E2E·Wiki 페이지 발행은 별도 미검증 범위로 유지한다.

## 조사 결과와 계약

기존 조사 thread가 이전 slice 결과를 반환해 새 근거로 채택하지 않았다. fresh Sol이 현재 base에서 다시 조사했다.
새 조사는 confirm CLI adapter/wiring을 최소 경계로 확인했다. retired package 삭제를 반복하지 않는다.
실행 명령은 source/rg/git grep 및 Go 1.27 go list였고 파일 변경·테스트 실행은 없었다. blockers 없음.
root/Sol은 [구현 packet](../superpowers/plans/2026-09-11-engine-retirement-confirm-cli.md)의 두 CLI interface 제거와
backend protected-change 보존 계약을 먼저 고정했다. Luna 1개를 예약하며 shared CLI는 이 worker만 소유한다.

## root 문서 Task packet

- taskId: `engine-retirement-slice6-operator-docs`
- baseSHA: `5f3d2b6b22022479fd5a19836a4b2c80bb734586`
- deps: confirm CLI 제거 계약
- ownedPaths: `docs/operator/parallel-pilot.md`, `HANDOFF.md`, 이번 plan/ledger
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice6`
- branch: `agent/engine-retirement-slice6`
- forbiddenPaths: 모든 제품 코드·테스트·기존 실험
- interface: 문서만. story 3의 confirm 실행 안내만 역사화한다.
- acceptance: 나머지 story와 retirement guards 보존, PR #78 미병합·후속 PR 의존성 구분
- tests: 링크/diff와 기존 `TestParallelPilotRunbookProtectsUnacceptedEvidenceFromRetirement`
- result: changedFiles/commitSHA/commands/outcomes/unverified/blockers는 후속 기록에 남긴다.

| Task 조합 | 경계 | 판단 |
|---|---|---|
| 코드 / root 문서 | CLI 6파일 / 문서 | 소유 중복 없음 |
| 코드 내부 | 두 CLI interface와 cmd wiring | root/Sol 직렬 합의, Luna 단독 소유 |

## 착수 기록

- [Issue #79](https://github.com/Middleages/thread-dock/issues/79)를 생성해 Project #1의 In Progress·정리 방향 유지로 등록했다.
- root는 parallel-pilot story 3의 confirm 실행 안내만 역사화했다. 다른 stories·retirement/evidence guard는 수정하지 않았다.
- Issue #77·PR #78의 In Progress 상태를 확인했으며 선행 구현을 main 완료로 표시하지 않는다.

## 구현·문서 검사

- code commit `b77ff5cd47e1a406a37406e3a50a734d006db772`, report 포함 head `7234f6602fc071d8881d66d6f07181fd4159fb1c`.
- Luna는 새 negative 사례에서 RED를 확인한 뒤 CLI/cmd test·vet/list/ref/diff를 tmpfs·Go 1.27.0에서 통과했다. 실제 TMPDIR과 원문 명령은 Task report에 기록했다.
- root의 문서 commit `e1c6ccd`에서 `TMPDIR=/dev/shm/threaddock-slice6-gate.OKTr3F`와 명시한 Go로 `go test ./internal/pilot -run '^TestParallelPilotRunbookProtectsUnacceptedEvidenceFromRetirement$'`를 수행해 exit 0(0.003초)을 확인했다.
- 이후 story 3에 기존 exact-SHA/check/mergeability gate 조건을 역사 설명으로 명시했다. 최종 문서는 마지막 통합 gate에서도 검증한다. 다른 story·retirement guard는 불변이다.
- fresh Sol은 CLI Task head `7234f6602fc071d8881d66d6f07181fd4159fb1c`에서 ACCEPT했다. finding과 수정 요구 없음. root 문서는 최종 통합 리뷰 대상이다.

## 최초 통합 gate 실패

`80c9a0d9d22264e00e78e1389fd5be4bce61eaf4`에서 tmpfs TMPDIR과 Go 1.27.0으로 make check를 실행했다.
gofmt/shell/vet는 통과했고 Go 테스트 중 `TestRound1ConcurrentAdvanceSerializesOneAction`이
`concurrent errors=[<nil> <nil>]`로 실패해 make exit 2였다. 나머지 Go package는 통과했고 UI/build는 미실행이다.
원본 로그는 `.superpowers/sdd/2026-09-11-engine-retirement-confirm-cli/make-check-80c9a0d.log`의 비추적 로컬 근거다.

이번 CLI 변경의 orchestrator diff는 0이다. 기존 테스트가 첫 startEntered 신호 직후 releaseStart를 닫아
두 번째 호출이 lock을 시도하기 전에 첫 호출이 끝날 수 있는지 Sol에 원인 귀속을 요청했다.
실패를 성공으로 숨기거나 동일 tuple을 근거 없이 재실행하지 않는다.

## Gate 수정 Task packet

Sol은 기존 테스트가 동시 호출의 겹침을 보장하지 않아 정상 직렬 실행에서도 실패할 수 있음을 확인했다.
harness의 state.Store는 nonblocking flock을 사용하며 production lock/claim 변경은 필요 없다.
기존 StateLockIsProcessSafe 테스트가 lease 보유 중 ErrRunBusy 경계도 검증한다.

- taskId: `slice6-gate-concurrency-test-fix`
- baseSHA: `80c9a0d9d22264e00e78e1389fd5be4bce61eaf4`
- deps: 최초 통합 gate 실패, Sol의 test barrier 원인 귀속
- ownedPaths: `internal/orchestrator/review_round1_test.go`의 해당 테스트, `.superpowers/sdd/engine-retirement-slice6/gate-fix-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-confirm-gate-fix`
- branch: `agent/engine-retirement-confirm-gate-fix`
- forbiddenPaths: production orchestrator/state/lock/fakes와 나머지 모든 코드·테스트·문서
- interface: 제품 API/잠금 동작 불변. 테스트 동기화만 수정하는 좁은 예외다.
- acceptance: 첫 호출을 adapter에서 막은 동안 두 번째 결과를 받아 ErrRunBusy 확인 후 첫 호출 해제. 첫 결과 nil과 StartAgent 1회 검증 유지. timeout/error cleanup에서도 해제하여 goroutine 누수 방지. sleep을 추가하지 않는다.
- tests: tmpfs Go 1.27에서 해당 테스트 `-count=20`, 해당 테스트와 `TestRound1StateLockIsProcessSafe`를 함께 `-count=1`, orchestrator vet, diff 검사. 타이밍 회귀 확인을 위한 focused 반복이며 worker full suite는 금지한다.
- result: changedFiles/실제 commitSHA/executedCommands/outcomes/unverified/blockers를 report에 기록한다.

root가 기존 CLI Task와 분리해 Luna에 이 테스트만 배정한다. 코드 Task의 orchestrator 금지 범위를
제품 동작까지 넓혀 해제하지 않는다. 수정 SHA가 최초 실패 tuple을 무효화한 뒤에만 최종 gate를 재실행한다.

## Gate 수정 후보

- test-fix code commit `be2f0e6c8fae34014cff93b4721db048ad87d8c3`, report 포함 head `eb62e6ee39a7785f5ea089ad0df7617fae100cee`.
- 제품 lock/fake 코드는 그대로이며 해당 테스트와 report만 변경했다. 첫 호출을 해제하기 전에 두 번째의 ErrRunBusy를 확인한다.
- `/dev/shm/threaddock-slice6-gate-fix.rMHGd0`에서 타겟 20회 반복 PASS, `/dev/shm/threaddock-slice6-gate-check.w6iVBl`에서 타겟+StateLockIsProcessSafe 및 vet/diff PASS.
- fresh Sol의 별도 test-fix Task 리뷰를 받고 있으며, 이 후보를 통합한 뒤 새 SHA에서 최종 gate를 실행한다.
