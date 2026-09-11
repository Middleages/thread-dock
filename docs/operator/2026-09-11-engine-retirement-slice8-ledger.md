# 여덟 번째 엔진 정리 ledger

## 기준·의존성·보존

- baseSHA: `00cd56167a512ea784faeba8fb4ae303ab22dfde`, 검증·리뷰를 마친 PR #82 head.
- 착수 시 PR #82는 OPEN, main/origin/main은 `f9fb7256e625e116493729c5accc390a2b8ec953`다. 이번 main 병합은 수행하지 않았다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice8`, branch `agent/engine-retirement-slice8`.
- 새 PR은 `agent/engine-retirement-slice7`을 base로 하며 #82 main 병합 후 main으로 전환한다.
- 중단된 codex-runtime의 cmd/agentctl main.go/main_test.go 및 internal/config config.go/config_test.go는 보존했다. 기존 worktree·Windows staging은 삭제하지 않는다.

## 조사 Task packet

- taskId: `engine-retirement-slice8-inventory`
- baseSHA: `00cd56167a512ea784faeba8fb4ae303ab22dfde`
- deps: PR #82의 backend confirmation 정리
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice7`
- branch: `agent/engine-retirement-slice7` (읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: 남은 engine의 최소 제거 경계 제안
- acceptance: caller/config/test/docs 근거와 정확한 소유 경로·focused 검사, 완료된 slice 반복 금지
- tests: rg/source/go list 등 조회만
- result: changedFiles `[]`, commitSHA 없음. commands/outcomes/unverified/blockers는 조사 결과에 기록한다.

## 역할·검증

root는 계약·문서·GitHub·통합, Sol medium은 조사, Luna high는 구현, fresh Sol medium은 독립 리뷰를 맡는다.
actual runtime identity는 미노출이면 unverified, 자동 승격 없음. worker 상한 3, Task 리뷰 완료로 Luna 예약 해제, reviewer 슬롯 확보.
이전 환경 근거에 따라 Go 1.27.0·별도 tmpfs·`VITEST_MAX_WORKERS=1`을 처음부터 사용한다.
worker는 focused 검사만, root는 마지막 full gate 한 번을 수행하며 동일 tuple을 반복하지 않는다.
native Windows/오류 상태·현재 보드 native 실행·Herdr/Projects E2E·Wiki 페이지 발행·정확한 Vitest 병목은 미검증으로 유지한다.

## 준비·후보 판단

- npm ci exit 0, lock 변경 없음. root 최종 TMPDIR은 `/dev/shm/threaddock-slice8-gate.UQU5Sx`다.
- 최초 확인된 `NewRetirementInspector` alias는 정의 외 참조가 없지만 단독 3줄 삭제는 독립 리뷰/gate 단위로 너무 작다.
- 같은 옛 retirement/cleanup compatibility 목적의 orphan API와 함께 작은 단위로 묶을 수 있는지 추가 확인한다. 현재 실행기와 진단 계약은 보존한다.

## 확정된 경계

fresh Sol은 `RetirementInspector` alias, `NewRetirementInspector`, `Store.ListCleanupCandidates`의 production caller가
없고 마지막 method만 전용 self-test 한 개가 소비함을 확인했다. root/Sol은 [구현 packet](../superpowers/plans/2026-09-11-engine-retirement-unused-cleanup-api.md)의
세 파일 범위를 직렬 고정했다. CLI cleanup의 exact RUN Load·7일 guard와 composite inspector 및 persistence는 그대로다.
조사는 source/rg/go list였고 파일 변경·테스트 실행은 없었다. blockers 없음, actual runtime identity는 unverified다.

| Task 조합 | 경계 | 판단 |
|---|---|---|
| 조사 / 구현 | caller 부재와 active cleanup 검증 → 미사용 세 symbol 제거 | 직렬 합의 후 구현 |
| 구현 / root 문서 | Go 세 파일 / HANDOFF·plan·ledger | 소유 중복 없음 |

## 착수 기록

- [Issue #83](https://github.com/Middleages/thread-dock/issues/83)을 생성해 Project #1에서 In Progress·정리 방향 유지로 추적한다.
- PR #82/Issue #81은 열린 선행 작업으로 유지한다. 새 PR은 main 완료를 가정하지 않는다.
- 제품 소유 범위는 미사용 세 symbol과 전용 test뿐이며 current cleanup 실행기/guard 변경은 없다.

## 구현 후보

- code `60f85978f7220390fa7a7c0cde6a08dac077e80a`, 최초 report `cbd4351262045de8532224394a9eccc65ec03491`, report SHA 정정 head `a1e7da3fb779e932e062bd2f1467d0c62f2fa1c2`.
- 제품 세 파일에서 60줄을 제거했다. 새 동작이나 테스트는 추가하지 않았다.
- Go 1.27.0과 `/dev/shm/td-unused-cleanup-api.cQdC5J`에서 recoverable, composite 2 tests, CLI Cleanup 16 cases, cmd composition 사례가 실행돼 통과했다. vet/list/ref/diff도 통과했다.
- fresh Sol은 `a1e7da3fb779e932e062bd2f1467d0c62f2fa1c2`에서 ACCEPT했다. blocking finding과 수정 요구 없음. 현재 실행기/7일 guard와 shared helper 및 전용 test 보존을 확인했다.
