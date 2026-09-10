# 세 번째 엔진 정리 ledger

## 기준·역할·보존

- baseSHA: `a2f35f67ce58da5d787bd4c03165e9292624e15b`. 사용자 지시에 따라 PR #72를 검토된 head로 병합했고 local main도 fast-forward했다.
- Issue #71은 병합으로 닫혔으며 Project #1의 Issue/PR #72는 Done·완료 근거로 갱신했다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice3`, branch `agent/engine-retirement-slice3`.
- 중단된 `.worktrees/codex-runtime`의 cmd/agentctl main.go/main_test.go 및 internal/config config.go/config_test.go는 기존 modified 상태로 보존한다. 기존 worktree·Windows staging도 삭제하지 않는다.
- root는 계약·문서·GitHub·통합, Sol medium은 조사, Luna high는 코드/테스트, fresh Sol medium은 독립 리뷰를 맡는다. 실제 runtime identity는 unverified, 자동 승격 없음.
- 구현 상한 3개 중 이번 Luna 1개만 예약한다. reviewer 슬롯은 확보한다.

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
