# 다섯 번째 엔진 정리 ledger

## 기준·보존·역할

- baseSHA: `5f5b31e519074f599a1d951cf6c7278e88a4cfef`. 사용자 지시로 PR #76을 검토된 head에서 병합하고 local main도 fast-forward했다.
- Issue #75는 닫혔고 Project #1의 Issue #75/PR #76은 Done·완료 근거로 갱신했다.
- 통합 worktree `/home/appuser/dev_system/.worktrees/engine-retirement-slice5`, branch `agent/engine-retirement-slice5`.
- 중단된 codex-runtime의 cmd/agentctl main.go/main_test.go와 internal/config config.go/config_test.go는 기존 modified 상태로 보존했다. 기존 worktree·Windows staging은 삭제하지 않는다.
- root는 계약·문서·GitHub·통합, Sol medium은 조사, Luna high는 코드·테스트, fresh Sol medium은 독립 리뷰를 맡는다. 실제 runtime identity는 unverified, 승격 없음.
- 구현 상한 3개 중 Luna 1개만 예약하며 reviewer 슬롯을 확보한다.

## 조사 Task packet

- taskId: `engine-retirement-slice5-inventory`
- baseSHA: `5f5b31e519074f599a1d951cf6c7278e88a4cfef`
- deps: PR #76 병합 및 기존 engine inventory
- ownedPaths: `[]`
- worktree: `/home/appuser/dev_system`
- branch: `main` (읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: 남은 engine의 최소 제거 경계와 보존할 현재 소비자 확인
- acceptance: importer/caller/config/test/docs 근거로 최소 후보 제안
- tests: rg·source reads·Go 1.27 go list만, 테스트 미실행
- result: changedFiles `[]`, commitSHA 없음. executedCommands는 위 조회. outcomes는 internal/integration importer 0·전용 테스트만 존재, 현재 workrun 서비스 보존 필요 확인. unverified는 runtime identity·구현 후 검사, blockers 없음.

## 직렬 계약·소유권

[구현 packet](../superpowers/plans/2026-09-11-engine-retirement-v1-integration.md)의 package 삭제를 root/Sol이 먼저 합의했다.
Luna는 옛 integration 두 파일과 report만, root는 HANDOFF/plan/ledger만 소유한다.

| 조합 | 확인 | 판단 |
|---|---|---|
| 조사 / 구현 | importer 0과 현재 workrun 대체 경로 | 계약 고정 후 구현 |
| 구현 / root 문서 | 제품 두 파일 / 문서 세 파일 | 중복 소유 없음 |

## 검증 원칙

이전 fsync 지연 근거에 따라 처음부터 별도 tmpfs TMPDIR을 사용한다. worker full suite와 동일 tuple
재실행을 금지하며 root의 마지막 make check만 한 번 수행한다. docs-only 후속은 링크/diff만 검사한다.
기존 Windows healthy-path를 새 native 실행으로 확대하지 않는다. native 오류·현재 보드의 native 실행,
Herdr/Projects E2E·Wiki 페이지 발행·runtime identity는 미검증으로 남긴다.

## 착수 기록

- [Issue #77](https://github.com/Middleages/thread-dock/issues/77)을 생성해 Project #1에서 In Progress·정리 방향 유지로 추적한다.
- 통합 TMPDIR은 `/dev/shm/threaddock-slice5-gate.Uxv15j`다. npm ci는 exit 0이며 package lock은 변경하지 않았다.
- 제품 소유 범위는 옛 integration 두 파일뿐이고, 현재 통합 서비스나 운영 문서 재작성은 포함하지 않는다.
