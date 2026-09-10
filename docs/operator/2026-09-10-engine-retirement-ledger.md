# SDD ledger — plan: docs/superpowers/plans/2026-09-10-engine-retirement.md

## 기준과 예약

- baseSHA: `3f4bebf08bb099637d1b2bfff44a9ba56e91a5df` (fetch한 origin/main과 로컬 main 일치).
- 통합: `/home/appuser/dev_system/.worktrees/engine-retirement`, `agent/engine-retirement`.
- 중단된 `.worktrees/codex-runtime`의 `cmd/agentctl/main.go`, `cmd/agentctl/main_test.go`, `internal/config/config.go`, `internal/config/config_test.go`는 착수 시 modified였다. 수정·stage·commit·재개 금지.
- 구현 worker 전체 상한 3, slice1 Luna 1개 예약. 문서 Sol 1, 읽기 전용 조사 Sol 1. reviewer 여유 유지.
- 요청 역할: td_coordinator Sol medium, td_implementer Luna high, td_reviewer Sol medium. 실제 runtime model/effort를 검증하는 메타데이터는 노출되지 않아 unverified. 자동 대체·승격하지 않는다.
- `brainstorming`의 bounded 정리 절차, `writing-plans`, `using-git-worktrees`, `subagent-driven-development`, `requesting-code-review`, `verification-before-completion`을 적용한다. 사용자의 작은 첫 slice 구현·push·기록 승인에 따라 반복 확인하지 않는다. 사용자 지침에 따라 기존 worktree와 ledger를 보존하며 모델 승격·worker full suite를 하지 않는다.

## Task packet: 문서

- taskId: `post69-docs`
- baseSHA: `3f4bebf08bb099637d1b2bfff44a9ba56e91a5df`
- deps: `[]`
- ownedPaths: `AGENTS.md`, `PRODUCT.md`, `HANDOFF.md`, `docs/adr/0008-github-first-skills-before-engine.md`, `docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md`, `docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md`, `docs/operator/github-first-quickstart.md`, `.superpowers/sdd/engine-retirement/docs-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-docs`
- branch: `agent/engine-retirement-docs`
- forbiddenPaths: 제품 코드·테스트·dependency·공유 타입·모든 기존 실험 및 소유 밖 경로
- interface: public/shared 변경 없음. AGENTS는 착수 상태 한 문장만 정정하고 역할 정책 유지.
- acceptance: #69 병합과 Task 1~3 구현 근거 반영, native 오류·독립 두 기능 E2E 미검증 보존.
- tests: 상대 링크 검사와 `git diff --check`; 기존 제품 테스트 재실행 없음.
- result: `changedFiles`, `commitSHA`, `executedCommands`, `outcomes`, `unverified`, `blockers`는 해당 Task report와 최종 통합 기록으로 확정한다.

## Task packet: 의존성 조사

- taskId: `engine-inventory`
- baseSHA: `3f4bebf08bb099637d1b2bfff44a9ba56e91a5df`
- deps: `[]`
- ownedPaths: `[]` (읽기 전용)
- worktree: `/home/appuser/dev_system`
- branch: `main` (읽기 전용)
- forbiddenPaths: 모든 쓰기
- interface: Monitor가 사용하는 runner·링크·경로 경계와 옛 엔진을 구분한다.
- acceptance: import·test·config·fixture·docs 의존성으로 가장 작은 삭제 후보를 증명한다.
- tests: `rg`, `go list` 등 조회만. suite 실행 없음.
- result: changedFiles `[]`, commitSHA 없음; executedCommands·outcomes·unverified·blockers는 inventory 문서에 기록한다.

## GitHub 쓰기 영수증

본문과 열린 상태를 보존한 한국어 분류 댓글을 게시했다.

| Issue | 댓글 |
|---|---|
| #42 | https://github.com/Middleages/thread-dock/issues/42#issuecomment-5619390550 |
| #43 | https://github.com/Middleages/thread-dock/issues/43#issuecomment-5619390839 |
| #44 | https://github.com/Middleages/thread-dock/issues/44#issuecomment-5619391154 |
| #45 | https://github.com/Middleages/thread-dock/issues/45#issuecomment-5619391477 |
| #46 | https://github.com/Middleages/thread-dock/issues/46#issuecomment-5619391808 |
| #47 | https://github.com/Middleages/thread-dock/issues/47#issuecomment-5619392088 |
| #48 | https://github.com/Middleages/thread-dock/issues/48#issuecomment-5619392409 |

Projects 조회는 성공/0개. 갱신 대상 부재로 보드 쓰기는 없음. Wiki 비활성화로 실제 Wiki 반영 없음.

## 사전 검토

| Task 또는 조합 | 입력·출력 및 검사 | 판단 |
|---|---|---|
| 문서 | merge SHA와 기존 검증 근거 → 현재 상태·체크박스 | healthy 검증과 native 오류·두 기능 E2E 미검증을 분리 |
| 조사 | main import·참조 → 삭제 후보 | 읽기 전용으로 코드 소유권 충돌 없음 |
| 문서 / 조사 | 동일 base 읽기, 작성 경로 공유 없음 | 독립 수행 가능 |
| slice1 | importer 없는 bridge 두 파일 삭제 → Monitor focused 테스트/vet | 테스트는 보존할 실제 소비 경로를 검증, 새 mirror test 없음 |
| 문서 / slice1 | 현재 문서의 bridge 설명과 코드 삭제 | 문서 Sol, 코드 Luna로 소유 분리. public/shared 변경 없음 |
| 조사 / slice1 | import·test·config·docs 근거 → 두 파일 삭제 | 광범위 engine 본체는 기존 CLI 소비가 있으므로 이번 소유 밖 |

## 첫 삭제 범위 결정

`engine-retirement-slice1` packet은 [계획](../superpowers/plans/2026-09-10-engine-retirement.md)에 있다.
현재 Wails Monitor가 직접 사용하는 `internal/runner`를 유지하고, importer 없는
`internal/monitorcli/client.go`, `client_test.go`만 제거한다. `cmd/agentctl`과 engine 본체 삭제는 후속으로 남긴다.
이 선택이 잘못되면 Linux/Windows build-tag 의존성이 누락될 수 있으므로 조사에서 양쪽 그래프를 확인하고
Task reviewer가 삭제 전 참조와 focused 근거를 독립 검토한다.

## 검증 정책

기존 `325db89`의 통합/Windows healthy 근거를 재실행하지 않는다. 새 code PR의 최종 통합 SHA에서
`make check` 한 번을 수행하고 실패 시 원인과 수정 소유자를 기록한다. docs-only 후속은 링크/diff 검사만 한다.
이번 세션의 Linux gh 조회와 Linux fixture/build를 Windows Wails build/app 및 live Herdr와 구분한다.
HERDR_ENV 설정, 실패 invocation 재사용, Herdr #3813 해결 가정은 하지 않는다.

## 구현 및 통합 결과

- 문서 Task: 원본 `cdf89774afc715f74617ba92bf652063c2bf203f`, report `476bb28`, 통합 `3aae2fc`/`6db85d0`. 상대 링크 8파일/6링크, diff/stale 검사 통과. 기존 Task 3 report 보존.
- 조사 Task: 쓰기 없음. Linux·Windows Go 그래프에서 monitorcli importer 0개 확인. [inventory](2026-09-10-engine-dependency-inventory.md)에 호출·config·fixture·문서 경계 기록.
- slice1 Luna: 코드 `3656616c91be6366f9753423d85e50a73eeaec63`, report 후속 포함 head `3d89c93cef34c2b4c4aa35ae794afd39c7f6dff6`. 두 bridge 파일 제거, public/shared 변화 없음.
- slice1 focused: Go 1.27.0 `go test ./monitor`, `go vet ./monitor`, diff 검사 및 package 경로 부재 확인 통과. 첫 chained 실행은 약 90초 무출력 후 중단했으며 성공 근거로 쓰지 않았다. 상세 [Luna report](../../.superpowers/sdd/engine-retirement-slice1/task-report.md)는 통합 시 포함한다.
- slice1 검토: fresh Sol이 고정 head `3d89c93`의 Task diff와 report 검토 중. 최종 verdict 전 완료로 처리하지 않는다.
- root 문서 검증: inventory/분류/ledger/새 계획 4파일의 상대 링크 6개 검사와 `git diff --check` 통과. 통합 `npm ci --no-audit --no-fund`는 107 packages 설치/exit 0이며 제품 테스트 근거와 구분한다.
