# SDD ledger — plan: docs/superpowers/plans/2026-09-12-engine-retirement-completion.md

## 착수

- main `c487e8c` → `2db8f776d22afd849fcb9a1328f7e8aff9b72e50` fast-forward, clean. 사용자 추가 PR #85/86/87와 compact 설계 보존.
- 독립 통합 worktree `.worktrees/engine-retirement-completion`, branch `agent/engine-retirement-completion`.
- gh auth project scope, Project #1 조회/쓰기 성공. 열린 PR 0. #42/44/45/46/47을 not planned로 종료하고 #43/48을 실사용·locator 검증으로 한국어 재작성했다. 종료는 구현 완료가 아니다.
- 기존 codex-runtime dirty 4개 파일 보존, 다른 worktree 및 Windows staging 삭제 없음.
- `38a505a`에서 stale 문서/Issue 방향 기록을 commit했다.

## 직렬 interface 결정과 사전 점검

전체 제거 방향은 ADR 0008과 사용자 요청으로 승인됐다. 새 엔진/대체 CLI 없이 옛 root부터 제거한다.
전체 그래프 조회에서 Linux/Windows Monitor의 repo-internal import는 runner뿐이며 settings/GHES/toolbox는 legacy package를 소비하지 않는다.

| Task 관계 | 생산/소비 및 자체 정합성 | 결정 |
|---|---|---|
| Task 1 / Monitor | CLI 제거, Monitor는 runner만 사용 | Monitor 전체 금지 경로로 보존 |
| Task 1 / 후속 internal 제거 | root caller를 먼저 끊고 이후 orphan 군집 삭제 | 직전 reviewed SHA에서 순차 배정 |
| Task 1 / root docs | 코드·Makefile 대 문서 | ownedPaths 분리 |
| Task 1 자체 | 삭제 경로와 남은 패키지 focused 검사 | full suite는 root 마지막 gate에만 실행 |

## 슬롯

inventory Sol 1(read-only), 구현 Luna 최대 1(현재 순차), fresh reviewer 1 예약. 모델/effort 실제 runtime identity는 unverified.

## Task 1 배정

- [Issue #89](https://github.com/Middleages/thread-dock/issues/89), Project #1 In Progress·유지로 추적한다.
- `retirement_root_cut` Luna에 실제 base `91d37632723ffca8114700498e43efcdba7f0ec8`, 독립 `.worktrees/retirement-root-cut`/`agent/retirement-root-cut`를 배정했다.
- 초기 plan의 `38a505a`는 앞 문서 commit이며 구현은 계획 commit `91d3763` 위에서 시작한다. packet에 실제 SHA로 정정했다.
- Inventory 확인: `internal/config`의 소비자는 삭제 대상 cmd/agentctl·internal/pilot뿐이다. 후속 단계의 내부 모듈은 이 첫 Task에서 건드리지 않는다.

## 검증 제한

사용자 추가 PR87의 source-level tests는 아직 실행 성공 근거가 없다. 최종 gate에서 현재 Makefile의 최신 UI 테스트 전부를 포함한다. native Windows/GHES/Herdr 및 Skill pressure scenario는 #43/#48에 남겨 코드 삭제와 구분한다.
