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
실제 runtime model/effort는 미노출이면 unverified이며 승격하지 않는다. worker 상한 3, 코드 worker 1개 예약, reviewer 슬롯 확보.
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
