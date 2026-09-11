# 일곱 번째 엔진 정리 ledger

## 기준·병합·보존

- baseSHA: `f9fb7256e625e116493729c5accc390a2b8ec953`. 사용자 지시에 따라 PR #78을 main에 병합(`13b20d3`)하고 PR #80의 base를 main으로 전환해 병합했다.
- local main을 fast-forward했고 `git diff ed42ea9 HEAD --exit-code`로 검토된 PR #80 head와 main의 전체 tree 일치를 확인했다. 기존 gate는 재실행하지 않았다.
- Issue #77·#79는 닫혔고 Project #1의 해당 Issue/PR #78·#80을 Done·완료 근거로 갱신했다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice7`, branch `agent/engine-retirement-slice7`.
- codex-runtime의 cmd/agentctl main.go/main_test.go와 internal/config config.go/config_test.go는 기존 modified 상태로 보존했다. 기존 worktree·Windows staging은 삭제하지 않는다.

## 조사 Task packet

- taskId: `engine-retirement-slice7-inventory`
- baseSHA: `f9fb7256e625e116493729c5accc390a2b8ec953`
- deps: PR #78·#80 병합, CLI confirm 제거
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system`
- branch: `main` (읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: backend confirmation 진입점의 caller와 state/event/test 보존 경계 확인
- acceptance: 다음 작은 삭제 범위·정확한 소유 파일·focused 검사 제안, 완료된 slice 반복 금지
- tests: rg/source/go list 등 조회만
- result: changedFiles `[]`, commitSHA 없음. commands/outcomes/unverified/blockers는 아래 조사 결과로 기록한다.

## 역할·검증 환경

root는 문서·계약·GitHub·통합, Sol medium은 조사, Luna high는 구현, fresh Sol medium은 독립 리뷰를 맡는다.
actual runtime identity는 미노출이면 unverified, 승격 없음. worker 상한 3, Task 리뷰 완료로 Luna 예약 해제, reviewer 슬롯 확보.
Go 1.27.0 절대 경로를 사용하며 root 통합 TMPDIR은 `/dev/shm/threaddock-slice7-gate.5weZvh`다.
npm ci는 exit 0이며 lock 변경 없음. worker full suite와 동일 tuple 재실행을 금지하고 새 최종 gate만 한 번 수행한다.
native Windows·오류 상태·현재 보드 native 실행·Herdr/Projects E2E·Wiki 페이지 발행은 별도 미검증으로 유지한다.

## 조사 결과·계약

fresh Sol은 Auto wrapper와 Orchestrator confirmation producer에 production 진입 caller가 없음을 확인했다.
후자는 wrapper 외에는 테스트 세 개만 소비한다. Source/rg/git grep/Go 1.27 go list로 확인했고 파일 변경·테스트 실행은 없다.
state/mergegate/comment/needs_operator와 invalidate helper는 실제 사용되므로 보존한다. blockers 없음, runtime identity 미검증.
root/Sol은 [구현 packet](../superpowers/plans/2026-09-11-engine-retirement-backend-confirm.md)의 두 method 제거를 직렬 고정했다.
mixed 대기·재개 테스트는 persisted fixture로 유지하고 전용 confirmation producer 테스트 두 개만 삭제한다.

| Task 조합 | 경계 | 판단 |
|---|---|---|
| 조사 / 구현 | caller 및 mixed-test 보존 근거 → 두 method 제거 | 직렬 합의 후 구현 |
| 구현 / root 문서 | 지정 Go 함수/테스트 / HANDOFF·plan·ledger | 소유 경로 중복 없음 |

## 착수 기록

- [Issue #81](https://github.com/Middleages/thread-dock/issues/81)을 생성해 Project #1에서 In Progress·정리 방향 유지로 추적한다.
- root는 HANDOFF의 PR #78·#80 미병합 문구를 실제 병합 결과로 정정했다. 지난 ledger의 당시 관찰은 역사 기록으로 보존한다.
- 코드 소유 범위는 지정된 두 method와 세 테스트이며, private helper/state/mergegate 구현 수정은 포함하지 않는다.

## 구현 후보

- code commit `9332e9ff433b50f77dcbfba46cdf724991842add`, report 포함 head `ec80575e03c2dac68babffda584a5a9c67322784`.
- 두 진입점과 전용 테스트 두 개를 제거하고 mixed test는 persisted-state 소비 검증으로 유지했다. 새 setter/API/helper는 없다.
- Go 1.27.0·독립 tmpfs에서 ParallelStories 5개 subcase, persisted confirmation 재개 사례, 요청한 mergegate 테스트 3개와 decision table 14개 subcase가 실제 실행돼 통과했다. vet/list/ref/diff도 통과했다.
- baseline focused 실행은 기존 사례를 확인한 PASS였으며, 인위적 RED나 제품 오류로 표시하지 않는다. 정확한 원문 명령/TMPDIR은 Task report에 기록했다.
- fresh Sol은 위 head `ec80575e03c2dac68babffda584a5a9c67322784`에서 ACCEPT했다. blocking/non-blocking finding과 수정 요구 없음. 정확한 두 method 제거와 persisted-state/active gate 보존을 확인했다.

## 최초 gate 및 환경 진단

`c62a718b2b2adb528dd79693a6ae8edfa1607d51`에서 tmpfs·Go 1.27.0 make check를 실행했다.
gofmt/shell/전체 Go vet/test는 통과했으나 Vitest forks worker 두 개의 startup 응답이 60초 안에 오지 않아
UI는 no tests/2 unhandled errors로 실패했고 make exit 2였다. frontend build는 미실행이다.
로그는 `.superpowers/sdd/2026-09-11-engine-retirement-backend-confirm/make-check-c62a718.log`의 비추적 로컬 근거다.

### 진단 Task packet

- taskId: `slice7-vitest-startup-diagnosis`
- baseSHA: `c62a718b2b2adb528dd79693a6ae8edfa1607d51`
- deps: 최초 gate UI worker startup 실패
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice7`
- branch: `agent/engine-retirement-slice7` (진단 읽기 전용)
- forbiddenPaths: 모든 repo 쓰기·전체 suite 재실행
- interface: 제품/frontend/config 불변, 검증환경 원인·지원 옵션 확인
- acceptance: 설치 Vitest 5 source/CLI와 실패 로그로 startup 경계 및 최소 환경 완화 제안
- tests: source/help/로그 읽기. root가 별도 focused 시작 검사 수행.
- result: changedFiles `[]`, commitSHA 없음. source/CLI 검색으로 hard-coded START_TIMEOUT=60s와 jsdom setup 이후 started 응답을 확인했다. public startup timeout 설정은 없고 VITEST_MAX_WORKERS는 지원된다. 정확한 CPU/memory/scheduler 병목은 unverified, 제품 회귀 근거 없음.

동일 tree에서 `VITEST_MAX_WORKERS=1 npm --prefix monitor/frontend test -- src/bindings.test.ts`가
exit 0, 1파일/2 tests, 4.16초로 통과했다. 이는 단일 fork/IPC/jsdom bootstrap 가능 근거이며
정확한 자원 원인을 확정하지 않는다. frontend/Makefile/lock/dependency diff는 0이다.
다음 gate는 `VITEST_MAX_WORKERS=1`만 추가해 동시 worker startup을 제한한다. assertion이나 test/hook timeout,
pool 종류, source/config는 변경하지 않는다. 이 환경 변경으로 실패 tuple과 구분해 최종 gate를 수행한다.

## 단일 worker 환경 최종 gate

고정 SHA `d40b693bcf617926fe86ca6ff9fa61d69f46c01a`에서 다음 명령이 exit 0으로 완료됐다.

```sh
VITEST_MAX_WORKERS=1 TMPDIR=/dev/shm/threaddock-slice7-gate.5weZvh PATH=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin:$PATH make check
```

gofmt/shell/전체 Go vet/test·UI 2 files/26 tests·frontend build가 통과했다. 변경 없는 Go 결과는 cache를
채택했고 UI는 11.80초에 실행 완료됐다. 정확한 startup 병목은 미확정이지만 지원되는 동시성 제한 환경에서
전체 검증을 완료했으며 assertion/timeout/isolation이나 제품 코드는 완화·변경하지 않았다.
로그는 `.superpowers/sdd/2026-09-11-engine-retirement-backend-confirm/make-check-d40b693-single-worker.log`의
비추적 로컬 근거다. `.placeholder`를 원본 내용으로 복원하고 `git diff --exit-code`로 tree 일치를 확인했다.
Markdown 4파일/상대 링크 23개 검사를 통과했다. 이후 문서 기록만 변경하며 전체 branch 리뷰 결과는 아래와 같다.

## 최종 리뷰·결과

fresh Sol은 `f44082c7bcc8cab6cbb1fc75a62b92e14a736cca`에서 전체 통합 ACCEPT를 반환했다.
blocking/non-blocking finding 없음. 두 method/전용 테스트 제거, persisted-state 소비 검증과 active gate 보존,
최초 startup 실패·원인 불확실성·지원 환경에서의 최종 gate 성공을 독립 확인했다. 이후 승인 기록은 docs-only다.

- changedFiles: orchestrator 코드/테스트 3파일, HANDOFF·plan·ledger·report 4파일, 총 7경로.
- commitSHA: code `9332e9f`, Task 리뷰 `ec80575`, 최종 gate `d40b693`, 전체 리뷰 `f44082c`.
- executedCommands: report의 focused orchestrator/mergegate 및 vet/list/ref, root의 첫 실패 gate와 단일-worker 환경 gate, 링크/diff, GitHub 기록.
- outcomes: Task/전체 리뷰 ACCEPT, 최종 full gate PASS. PR #78·#80 병합 완료와 Issue #81 추적.
- unverified: 정확한 Vitest 자원 병목, native Windows/오류 상태·현재 보드 native 실행·Herdr/Projects E2E·Wiki 페이지 발행·actual runtime identity.
- blockers: 없음. 새 PR의 main 병합은 사용자에게 남긴다.
- 다음 경계: 남은 engine의 caller/config/test/docs를 다시 확인해 작은 slice를 정한다. 현재 protected state/gate/comment 소비를 미사용으로 간주하지 않는다.
