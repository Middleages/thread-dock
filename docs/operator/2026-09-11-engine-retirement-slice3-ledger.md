# 세 번째 엔진 정리 ledger

## 기준·역할·보존

- baseSHA: `a2f35f67ce58da5d787bd4c03165e9292624e15b`. 사용자 지시에 따라 PR #72를 검토된 head로 병합했고 local main도 fast-forward했다.
- Issue #71은 병합으로 닫혔으며 Project #1의 Issue/PR #72는 Done·완료 근거로 갱신했다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice3`, branch `agent/engine-retirement-slice3`.
- 중단된 `.worktrees/codex-runtime`의 cmd/agentctl main.go/main_test.go 및 internal/config config.go/config_test.go는 기존 modified 상태로 보존한다. 기존 worktree·Windows staging도 삭제하지 않는다.
- root는 계약·문서·GitHub·통합, Sol medium은 조사, Luna high는 코드/테스트, fresh Sol medium은 독립 리뷰를 맡는다. 실제 runtime identity는 unverified, 자동 승격 없음.
- 구현 상한 3개 중 예약한 Luna 1개는 Task 리뷰 완료 후 해제했다. reviewer 슬롯은 확보한다.

## 조사 Task packet

- taskId: `engine-retirement-slice3-inventory`
- baseSHA: `a2f35f67ce58da5d787bd4c03165e9292624e15b`
- deps: PR #72의 route/service 삭제
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system`
- branch: `main` (읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: shared helper의 정의 외 production 호출을 확인해 가장 작은 제거 계약 제안
- acceptance: GitHub와 worktree 후보의 사용/미사용 경계·전용 테스트·config/docs 영향 증명
- tests: `rg`, source reads, Go 1.27 `go list`만. 테스트 미실행.
- result: changedFiles `[]`, commitSHA 없음. executedCommands는 위 조회. outcomes는 safe-draft 세 symbol의 production 참조 0, shared DraftPR/limits/query 보존 필요 확인. unverified는 runtime identity·구현 후 검사, blockers 없음.

## 직렬 계약·Task 분배

[계획/구현 packet](../superpowers/plans/2026-09-11-engine-retirement-safe-draft.md)의 세 symbol만 제거한다.
root/Sol이 계약을 먼저 고정했으며 Luna 한 명이 GitHub 세 파일을 단독 소유한다.
root 문서는 HANDOFF와 새 plan/ledger로 분리해 코드 경로를 공유하지 않는다.

| Task 조합 | 확인 | 결과 |
|---|---|---|
| 조사 / 구현 | 호출 0 증거 → 정확한 세 symbol 삭제 | 계약 고정 후 구현 |
| 구현 / root 문서 | GitHub 세 파일 / HANDOFF·plan·ledger | 소유 경로 중복 없음 |

## 검증 정책

이전 ext4 fixture의 fsync 지연 근거를 재사용해 이번 검사도 처음부터 분리된 tmpfs TMPDIR을 사용한다.
worker full suite는 금지하며 root의 마지막 `make check`만 한 번 수행한다. docs-only 후속은 링크/diff만 확인한다.
기존 Windows `325db89` healthy-path와 현재 Linux 검사·live GitHub 기록을 구분하고 Windows native 오류,
현재 보드의 native 실행, 두 기능의 Herdr/Projects E2E와 Wiki 페이지 발행을 완료로 확대하지 않는다.

## 착수 기록

- [Issue #73](https://github.com/Middleages/thread-dock/issues/73)을 생성해 Project #1에 연결했다. 상태는 In Progress, 정리 방향은 유지로 관리한다.
- 통합 fixture TMPDIR은 `/dev/shm/threaddock-slice3-gate.TsRe5M`로 새로 생성했다. npm ci는 exit 0이며 module/package lock 변경은 없다.
- 구현자가 소유하는 제품 경로는 GitHub 세 파일뿐이며 HANDOFF·plan·ledger는 root가 별도 소유한다.

## 구현 후보와 focused 근거

- code commit: `ea04779bf642f25db479c21ce733c412e7296af5`; report 포함 head: `fea410c6c2e7b6a57b7ff8b119690a5ab677efcd`.
- GitHub 세 파일에서 승인된 API/테스트/assertion/import 110줄만 제거했다. 새 테스트나 동작 이동은 없다.
- `/dev/shm/thread-dock-engine-retirement-safe-draft.mHjhhh` TMPDIR과 명시한 Go 1.27.0 경로에서 `go test ./internal/github`(0.151초), focused vet, go list, 참조/whitespace 검사를 통과했다.
- 최초 bare `go` 명령은 PATH에 없어 exit 127이었다. 이 실행은 성공 근거가 아니며 명시한 toolchain으로 수행한 결과를 채택한다.
- fresh Sol은 `fea410c6c2e7b6a57b7ff8b119690a5ab677efcd`에서 spec/quality 모두 ACCEPT했다. blocking finding과 수정 요구 없음. 정의·전용 테스트·assertion/import 제거와 사용 중인 shared 기능 보존을 독립 확인했다.
- Task는 완료했으며 아래 통합 gate·전체 리뷰는 별도 단계다. 제품 테스트를 옛 SHA에서 다시 실행하지 않는다.
