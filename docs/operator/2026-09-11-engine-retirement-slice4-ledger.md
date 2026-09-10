# 네 번째 엔진 정리 ledger

## 기준·역할·보존

- baseSHA: `b2acf4c8f15c48901a0bfd0e6e50800b89a215f9`. 사용자 지시로 PR #74를 검토된 head에서 병합하고 local main도 fast-forward했다.
- Issue #73은 병합으로 닫혔다. Project #1의 Issue #73/PR #74는 Done·완료 근거로 갱신했다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice4`, branch `agent/engine-retirement-slice4`.
- `.worktrees/codex-runtime`의 cmd/agentctl main.go/main_test.go, internal/config config.go/config_test.go는 기존 modified 상태로 보존했다. 기존 worktree·Windows staging은 삭제하지 않는다.
- root는 문서·계약·GitHub·통합, Sol medium은 조사, Luna high는 구현, fresh Sol medium은 독립 리뷰를 맡는다. actual runtime identity는 unverified, 자동 승격 없음.
- worker 상한 3개 중 예약한 Luna 1개는 Task 리뷰 완료 후 해제했고 reviewer 슬롯을 확보한다.

## 조사 packet과 결과

- taskId: `engine-retirement-slice4-inventory`
- baseSHA: `b2acf4c8f15c48901a0bfd0e6e50800b89a215f9`
- deps: PR #74 병합과 이전 worktree 후보 조사
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system`
- branch: `main` (읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: 미사용 revert 타입/메서드와 active PushBranch/CreateManagedWorktree 경계 확인
- acceptance: 참조·전용/결합 테스트·private helper·config/docs 영향으로 최소 범위 제안
- tests: rg, source reads, Go 1.27 go list만. 테스트 미실행.
- result: changedFiles `[]`, commitSHA 없음. executedCommands는 위 조회. outcomes는 여섯 symbol의 production 호출 0과 private helper/ErrConflict의 active 소비자 확인. unverified는 runtime identity·구현 후 검사, blockers 없음.

## 계약·분배

[구현 packet](../superpowers/plans/2026-09-11-engine-retirement-worktree-revert.md)의 여섯 symbol 제거를
root/Sol이 먼저 고정했다. Luna가 worktree 두 파일을 단독 소유하고 root는 HANDOFF/plan/ledger를 소유한다.

| Task 조합 | 확인 | 판단 |
|---|---|---|
| 조사 / 구현 | production 0 및 active helper/test 경계 | 합의 후 구현 |
| 구현 / root 문서 | worktree 두 파일 / 문서 세 파일 | 중복 소유 없음 |

## 검증 원칙

기존 ext4 fsync 지연 근거에 따라 처음부터 별도 tmpfs TMPDIR을 쓴다. worker full suite와
동일 tuple 재실행은 금지하며 root의 마지막 `make check`만 한 번 수행한다. docs-only 후속은 링크/diff만 검사한다.
기존 Windows healthy-path를 이번 SHA의 native 실행으로 확대하지 않는다. native 오류·현재 보드의
native 실행·Herdr/Projects E2E·Wiki 페이지 발행·runtime identity는 여전히 미검증이다.

## 착수 기록

- [Issue #75](https://github.com/Middleages/thread-dock/issues/75)를 생성하고 Project #1에서 In Progress·정리 방향 유지로 추적한다.
- root 통합 TMPDIR은 `/dev/shm/threaddock-slice4-gate.e9jTIw`다. npm ci는 exit 0이며 package lock 변경은 없다.
- 구현자가 수정할 제품 경로는 worktree 두 파일뿐이며 운영 문서 범위 확장은 없다.

## 구현 후보

- code commit `e3af253f9abd3eb5a802b41c3b5f043821d37380`, report 포함 head `537bda266b4f20f2ab81bb051f2b30e55f0acb13`.
- `/dev/shm/engine-retirement-worktree-revert.7mkvdP` TMPDIR과 명시한 Go 1.27.0에서 기존 worktree 테스트(3.109초), vet/list/ref/diff 검사를 통과했다.
- 최초 bare gofmt는 PATH에 없어 실패했으며 절대 toolchain 경로의 gofmt로 완료했다. 실패 호출을 성공 근거로 사용하지 않는다.
- 전용 API/테스트를 제거하고 결합 테스트를 PushBranch-only로 좁혔다. fresh Sol은 위 head에서 spec/quality 모두 ACCEPT했다. blocking/non-blocking finding과 수정 요구 없음.
- Task 리뷰는 여섯 symbol과 전용 테스트 삭제, active helpers/tests 및 정확한 PushBranch/no-force 검증 보존을 독립 확인했다. 통합 gate와 전체 리뷰는 별도 단계다.

## 통합 gate

고정 SHA `7a620c4523ac6fe8a65a0e0de4e64c56497a565a`에서 다음 명령을 한 번 실행해 exit 0을 확인했다.

```sh
TMPDIR=/dev/shm/threaddock-slice4-gate.e9jTIw PATH=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin:$PATH make check
```

gofmt·shell syntax·전체 Go vet/test·UI 2 files/26 tests·frontend build가 통과했다.
worktree package 1.709초, orchestrator 1.330초, pilot 10.343초, Monitor 0.026초다.
로그는 통합 worktree의 `.superpowers/sdd/2026-09-11-engine-retirement-worktree-revert/make-check-7a620c4.log`에
있는 비추적 로컬 증거다. frontend build가 제거한 tracked `.placeholder`는 원본 내용으로 복원했고
`git diff --exit-code`로 검사한 tree와 같음을 확인했다.

변경 Markdown 4파일의 상대 링크 17개 검사도 통과했다. 이후 gate 기록은 docs-only이며
제품 검사를 반복하지 않는다. 전체 branch 리뷰 결과는 아래와 같다.

## 최종 리뷰·결과

fresh Sol은 `acfbf7be0c22fd333874f1e26539debb74abfd12`에서 전체 통합 ACCEPT를 반환했다.
blocking/non-blocking finding 없음. Task와 통합 코드 일치, 승인된 여섯 symbol과 전용 테스트 제거,
active helper/소비자 및 PushBranch no-force 검증 보존, 단일 gate 로그와 이후 docs-only 변경을 확인했다.
이 승인 결과를 기록한 후속 변경도 문서뿐이며 제품 tree는 gate SHA와 동일하다.

- changedFiles: worktree 코드/테스트 2파일, HANDOFF·plan·ledger·report 4파일, 총 6경로.
- commitSHA: code `e3af253`, Task 리뷰 `537bda2`, 통합 gate `7a620c4`, 전체 리뷰 `acfbf7b`.
- executedCommands: Task report의 focused test/vet/list/ref/diff, root의 단일 tmpfs make check 및 링크/diff, GitHub Issue/Projects 기록.
- outcomes: Task/전체 리뷰 ACCEPT, full gate PASS. PR #74 병합·Issue #73 완료, Issue #75에서 이번 작업 추적.
- unverified: native Windows 오류·현재 보드의 native 실행·Herdr/Projects E2E·Wiki 페이지 발행·실제 runtime identity.
- blockers: 없음. 이번 PR의 main 병합은 사용자에게 남긴다.
- 다음 경계: 남은 v1/v2 CLI·엔진에서 호출과 config/test/docs 의존성을 새로 확인해 작은 경계 하나를 정한다. 현재 사용 중인 Git/worktree helper를 함께 삭제하지 않는다.
