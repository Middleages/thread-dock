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
