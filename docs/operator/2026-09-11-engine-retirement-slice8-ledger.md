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

## 최초 gate 실패와 환경 대조

`23906adde5a1f90692ac4a099fca4a31e80b0932`에서 tmpfs·Go 1.27.0·VITEST_MAX_WORKERS=1로 make check를 실행했다.
gofmt/shell/전체 Go vet/test는 통과했으나 forks pool의 monitor test worker startup이 timeout으로 실패했다.
bindings 1파일/2 tests만 통과, UI unhandled error 1개, make exit 2였으며 frontend build는 미실행이다.
로그는 `.superpowers/sdd/2026-09-11-engine-retirement-unused-cleanup-api/make-check-23906ad.log`의 비추적 로컬 근거다.

### 진단 packet

- taskId: `slice8-vitest-startup-diagnosis`
- baseSHA: `23906adde5a1f90692ac4a099fca4a31e80b0932`
- deps: 단일 forks worker 시작 실패 재발
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice8`
- branch: `agent/engine-retirement-slice8` (진단 읽기 전용)
- forbiddenPaths: repo 쓰기·full suite 반복·다른 사용자 process 변경
- interface: 지원되는 pool 환경의 대조만 수행, source/config/assertion/timeout/isolation 유지
- acceptance: installed Vitest source와 로그로 단일 threads pool 대조가 가능한지 확인
- tests: source/help/log 조회만, root는 실패한 monitor 파일만 focused 실행
- result: changedFiles `[]`, commitSHA 없음. 정확한 자원/스케줄러 원인은 미확정이며 source/config 회귀로 단정하지 않는다.

기존 단일-worker 완화만으로 충분하지 않은 새 관찰이므로 다음 환경 대조는 `--pool=threads --maxWorkers=1`이다.
설치된 default의 isolation=true는 유지하며 assertion이나 테스트 timeout을 완화하지 않는다.

## 공통 bootstrap 분리와 tmpfs 대조

threads 단일-worker의 monitor-only 실행도 startup timeout/no tests/exit 1(62.40초)로 실패했다.
두 pool 모두 실패했으므로 fork 전용 원인으로 단정하지 않고 Sol 진단 brief를 공통 초기화 경로로 재분해했다.
설치 source에서 setupFiles와 assertion은 started 이후 실행됨을 확인했다.

- 지원 Node 26.8.1의 직접 jsdom 초기화는 최초 import 30.282초/create 3.031초, 재측정 전체 0.880초였다.
- Node 22.22.0 비교 probe는 jsdom 요구 범위(^22.22.2)에 미달하므로 검증 근거로 채택하지 않았다.
- 설치된 threads worker entrypoint를 그대로 사용한 no-test 진단 harness는 online 1.463초, started 23.813초에 성공했다. vendor/source 수정은 없었고 테스트 성공 근거가 아닌 bootstrap 가능 근거다.
- 같은 `23906ad`의 detached 검증 worktree를 `/dev/shm/threaddock-slice8-validation.cIPR6I/checkout`에 만들고 동일 node_modules를 복사했다. 패키지 설치/업그레이드나 source/config 변경은 없었다.
- tmpfs threads monitor probe는 startup 이후 첫 async 사례 timeout과 후속 실패(22 failed/2 passed)로 끝나 통과로 채택하지 않았다.
- 같은 tmpfs에서 원래 forks pool로 첫 async 사례만 분리한 probe는 기존 timeout/assertion 그대로 PASS(1 passed/23 skipped)였다. 이것도 전체 UI 통과로 확대하지 않는다.

현재 최종 대조는 이 tmpfs checkout에서 원래 forks/isolation, VITEST_MAX_WORKERS=1, 같은 Go/Node와
동일 source/lock을 사용하는 make check다. 물리적 검증 경로가 달라진 새 tuple이며 정확한 환경 병목은 미확정이다.
다른 사용자 process와 기존 worktree/staging은 변경하거나 삭제하지 않았다.

## 최종 통합 gate

- 검증 SHA: `23906adde5a1f90692ac4a099fca4a31e80b0932`.
- 환경: 위 tmpfs detached checkout, 동일 node_modules, Go 1.27.0, Node 26.8.1, 원래 forks/isolation 및 timeout. `VITEST_MAX_WORKERS=1 TMPDIR=/dev/shm/threaddock-slice8-gate.UQU5Sx`로 `make check`를 실행했다.
- 결과: exit 0. gofmt/shell/vet/전체 Go tests, UI 2 files·26 tests, TypeScript/Vite build 모두 통과했다. UI 41.06초, Vite build 11.33초였다.
- 로그: 비추적 로컬 `.superpowers/sdd/2026-09-11-engine-retirement-unused-cleanup-api/make-check-23906ad-tmpfs-checkout.log`.
- build가 제거한 tracked dist placeholder만 원문 복구했다. 검증 checkout의 tracked diff는 0이며 이후 root 변경은 문서뿐이다.
- 앞선 실패는 취소하거나 숨기지 않는다. 물리적 위치를 바꾼 tuple에서는 통과했지만 정확한 환경 병목은 미확정이다. Windows Wails/native 및 live gh/Herdr 제품 E2E는 이번에 재검증하지 않았다.

## 전체 통합 리뷰와 게시

- taskId: `slice8-integration-review`; baseSHA: `00cd56167a512ea784faeba8fb4ae303ab22dfde`; deps: Task ACCEPT 및 최종 gate.
- ownedPaths: `[]`; worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice8`; branch: `agent/engine-retirement-slice8`; forbiddenPaths: 모든 쓰기와 테스트 재실행.
- interface/acceptance: 승인된 60줄 삭제와 보존 경계, stacked base 및 검증 문서 정합성; tests: 기존 근거 읽기 전용 검토.
- result: changedFiles `[]`; commitSHA `bfbd6dc8d97b2fbda4381b59adb16a9b98c1d32e`; executedCommands: SHA/조상/diff/numstat/patch-id/symbol/문서/로그/검증 checkout 조회; outcomes: fresh Sol ACCEPT, blocking finding 없음; unverified: actual runtime model/effort 및 Windows/live E2E; blockers: 없음.
- 제품 수정 요구 없음. 이후 변경은 리뷰·게시 문서 기록뿐이다. Markdown 링크 25개와 diff 검사가 통과했다.
- gh auth의 `project` scope와 Project #1 조회가 성공했다. PR #82는 OPEN/head `00cd561`이며 main/origin main은 `f9fb725`다. 사용자 실험 worktree의 미커밋 4개 파일은 보존했다.
- [PR #84](https://github.com/Middleages/thread-dock/pull/84)를 `agent/engine-retirement-slice7` base로 게시했다. main 병합은 수행하지 않았다.
