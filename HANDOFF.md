# ThreadDock 진행 설계 정정과 다음 작업

## 2026-09-12 재착수 상태

- main을 `2db8f776d22afd849fcb9a1328f7e8aff9b72e50`로 fast-forward했다. PR #82는 `6aced75`, #84는 `c487e8c`에 병합됐다.
- 사용자 추가 PR #85·#86·#87은 settings 저장/UI, GHES host, 열린 업무 중심 화면, Markdown, 프로젝트 Toolbox 및 새 Agent/Skill 구조를 포함한다. 삭제 작업에서 이 기능과 데이터는 보존한다.
- [현재 Agent/Skill 설계](docs/superpowers/specs/2026-09-11-thread-dock-agent-orchestration-design.md)와 [빠른 시작](docs/operator/github-first-quickstart.md)이 Coordinator/Feature Leader, 여섯 canonical Skill과 전역 locator를 설명한다.
- PR #88의 [compact Monitor 설계](docs/superpowers/specs/2026-09-11-compact-monitor-and-global-toolbox-design.md)는 추가 설계다. main에 설계가 있다는 이유로 global Toolbox migration/UI가 구현됐다고 간주하지 않는다. 이번 작업은 UI 재설계가 아니라 옛 엔진 제거다.
- PR #87의 [Toolbox 검증 기록](docs/superpowers/reviews/2026-09-11-project-toolbox-verification.md)과 [Skill 검증 기록](docs/superpowers/reviews/2026-09-11-thread-dock-agent-skill-verification.md)은 실행되지 않은 검증을 명시한다. 아래 `325db89` Windows healthy 증거는 과거 SHA의 증거이지 최신 기능의 검증이 아니다.
- 옛 실행 엔진 전체 제거는 production/test/config/docs 의존성을 증명한 순차 Task로 진행한다. 사용자 실험과 저장된 runtime 데이터는 삭제·이관하지 않는다.
- 현재 작업은 [Issue #89](https://github.com/Middleages/thread-dock/issues/89), [완료 계획](docs/superpowers/plans/2026-09-12-engine-retirement-completion.md), [진행 ledger](docs/operator/2026-09-12-engine-retirement-completion-ledger.md)에서 추적한다. #42/44/45/46/47은 superseded 종료했고 #43/48은 실제 재개·locator 검증으로 재작성했다.

## 제품 기준

### 옛 엔진 제거 구현 완료

독립 branch `agent/engine-retirement-completion`에서 8개 Task의 Luna 구현·fresh Sol 리뷰를 완료하고
옛 소스·전용 테스트·fixture 167개 파일을 제거했다. 현재 Go package는 `monitor`, `internal/runner`,
`internal/projecttemplate`뿐이다. 최신 main `2db8f77`의 Monitor/settings/GHES/Toolbox/Markdown/open-only 코드,
runner·템플릿 검증 Go 코드와 go.mod/go.sum은 그대로다. GitHub template의 문구와 Agent의 stale 지시만 정리했다.
아래 이전 slice에서 보존했던 state/gate/retirement 등은 당시의 중간 보존 범위이며 최종 트리에 남은 엔진이 아니다.
최종 gate·전체 리뷰·PR 상태는 [ledger](docs/operator/2026-09-12-engine-retirement-completion-ledger.md)에 이어 기록한다.
사용자 데이터·기존 worktree·중단된 실험은 보존했고 main 병합은 아직 수행하지 않았다.

최초 통합 gate는 Go 통과 후 UI timing/startup 오류로 실패했다. 지원되는 Node compile cache 환경의
focused 대조에서 유입 main의 오래된 테스트 선택자 한 곳을 발견해 Task9로 수정했다. 제품 코드는
그대로 두고 `monitor.test.tsx`의 row name 탐색만 현재 업무 표에 맞췄다. fresh Sol은 `1e3cbab`에서
ACCEPT했다. 실패와 환경 대조·최종 gate 결과는 ledger에 분리 기록한다.
최종 `1e427ed`의 tmpfs·Node compile cache 환경에서 `make check`가 통과했다: Go3 packages,
UI6파일/37tests, TypeScript/Vite build. Windows/native·실제 GHES/Herdr·Skill pressure scenario는
여전히 별도 미검증이며 #43/#48에서 추적한다. 전체 통합 리뷰·게시 상태는 ledger를 확인한다.

사용자 요구는 **Go 모니터 도구**다. Windows Go/Wails 앱과 기존 React 화면을 유지한다.
“심플하게”는 ThreadDock 자체 실행 엔진을 줄이라는 의미이며 브라우저 전용 전환은 승인되지 않았다.
현재 기준: [ADR 0008](docs/adr/0008-github-first-skills-before-engine.md),
[설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md).

## PR #69에서 완료된 상태

- PR #69의 최종 head `f35e248`은 main의 merge commit `3f4bebf`로 병합됐다.
- 2026-09-10 PR #69 직후 재확인에서 `gh auth status`는 `read:project` scope를 포함했고 Projects 조회는 성공했다. 당시 `Middleages`의 Projects와 저장소 ProjectsV2는 각각 0개, 열린 PR은 0개이며 Issue #42~48의 project item도 모두 0개였다. 당시 Wiki도 비활성화 상태였다. 이후 현재 상태는 아래에 따로 기록한다.
- Task 1·2가 Go/Wails `GetMonitorSnapshot` 경로에 GitHub와 Herdr 관찰을 연결했다. 공유 Go/TS wire는 유지한다.
- Task 3가 `monitor/frontend/server/`의 Node adapter·전용 테스트, Vite middleware와 HTTP monitor endpoint를 제거했다. Vite는 React 화면 개발·빌드만 담당한다.
- 화면에는 GitHub 업무·근거, 단일 선택 업무 Herdr 연결, 관찰 세션·미연결 Agent, degraded/notices, 안전한 외부 링크와 handoff 복사만 남겼다. 비기능 상단 메뉴, 자동화 작업, 옛 Work/발행 표시는 제거했다.
- Linux fixture/UI/build 근거와 managed-pane live gh/Herdr 근거는 서로 구분한다. 상위에서 전달된 외부 관찰(원본 transcript 없음)으로 실제 Windows→WSL workstation의 `gh auth status`는 성공했고, bare `wsl.exe --exec herdr`는 PATH lookup에 실패했으며, absolute `/home/appuser/.local/bin/herdr`의 status와 agent list는 성공했다. 이 component 관찰 자체는 Windows→WSL Monitor 읽기 검증이 아니며, 당시 Task 3 문서 worker는 GitHub Issue/PR/Projects/Wiki 쓰기를 수행하지 않았다. 이번 후속 세션의 쓰기는 아래 ledger에 따로 기록한다.
- Final reviewed product/config/dependency SHA는 `325db89`다. Windows native 값은 `THREADDOCK_REPOS=Middleages/thread-dock`, `THREADDOCK_PROJECTS` unset, `THREADDOCK_WSL_DISTRIBUTION=Ubuntu`, `THREADDOCK_SESSIONS_FILE=/tmp/threaddock-aeca770-sessions.json`이다.
- Windows user-local toolchain은 Go `1.27.0 windows/amd64`, Node `26.8.1`, Wails CLI/runtime `v2.15.0`이며, 공식 Go/Node checksum은 제공된 범위에서 일치했다. final SHA의 matching `wails build`는 exit 0, `1m9.285s`에 완료됐고 `monitor\build\bin\ThreadDockMonitor.exe`와 bindings/frontend/assets/app stages `Done`을 확인했다. native child console은 표시되지 않았다.
- Healthy packaged live acceptance는 GitHub 69개 work item의 약 14초 동기화, Herdr 기본 session과 3개 Agent의 약 30초 관찰, handoff 성공 toast와 실제 clipboard 길이 `188`(repository name 포함), 초록 점과 `로컬 연결 정상` 문구의 일치를 확인했다. 앱 종료 후 Monitor process 수는 `0`, `ThreadDockValidation66158d9` scheduled task는 없음, 관련 process도 `0`이었다. WSL interop은 복구됐고 computer-use는 파일이나 worktree를 수정하지 않았다.
- Native run에서 오류가 발생하지 않아 native error/degraded 상태는 검증하지 않았다. 결합 degradation은 fixture/UI 테스트 근거만 있다. `UtilAcceptVsock:281: accept4 failed 110`은 superseded historical diagnostic이고, 이전 `aeca770` plain `go build` 관찰과 `66158d9` staging build는 최종 표준 package evidence가 아니다.
- 기존 Wiki 링크는 문서 참고이며 Wiki 실제 반영과 구분한다.

## PR #70 이후 GitHub 운영 상태

- PR #70은 main의 merge commit `22dcc66`으로 병합됐고 이번 착수 시 열린 PR은 0개였다.
- 저장소는 public이며 Wiki가 활성화됐다. 사용자가 접근을 확인했지만 `thread-dock.wiki.git`의
  `git ls-remote`는 `Repository not found`였으므로 Wiki 페이지 발행은 수행하거나 검증하지 않았다.
- 비공개 사용자 Project [ThreadDock 개발 보드](https://github.com/users/Middleages/projects/1)가 생성됐고
  저장소에 연결됐다. 기본 [Board view](https://github.com/users/Middleages/projects/1/views/2)에는
  착수 당시 9개 항목이 있었으며 Issue #42~48은 `Todo`, PR #69·#70은 `Done`이었다.
- `정리 방향` 필드는 #43·#48이 재작성 후보, 나머지 Issue가 superseded 종료 후보인 원문 값으로
  확인했다. CLI의 한글 field key 표시 문제를 제품 버그로 판정하지 않는다.
- GitHub Projects 쓰기는 `project` scope로 성공했다. 보드 업무 상태는 GitHub가 원본이고,
  Herdr의 session·Agent 상태와 관찰 시각은 별도 실행 원본이다.

## 여덟 번째 엔진 정리

[Issue #83](https://github.com/Middleages/thread-dock/issues/83)에서 미사용 retirement alias/constructor와
bulk cleanup 후보 조회 API를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-unused-cleanup-api.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice8-ledger.md)에 caller·보존 경계·검증 결과를 기록한다.
exact-ID retire/cleanup·7일 guard·현재 composite inspector·state persistence/ListRecoverable는 보존한다.
당시 PR #82의 `00cd561` head 위에서 작업했다. 이후 사용자 지시로 #82를 병합하고 #84의 base를 main으로
전환한 뒤 `c487e8c4076d9ea804b096213cc8728a6b17e919`에 병합했다. Issue #83과 보드는 완료 상태다.
제품 세 파일의 미사용 API·전용 테스트 60줄 삭제를 완료했다. Task `a1e7da3` 및 전체 통합 `bfbd6dc`는
fresh Sol ACCEPT다. 같은 코드 `23906ad`의 tmpfs checkout에서 최종 `make check`가 Go 전체·UI 26개·build까지
통과했다. 앞선 UI startup/threads 실패와 환경 대조는 ledger에 기록했으며 정확한 환경 병목은 미확정이다.
Windows/native와 live gh/Herdr 제품 E2E를 새로 검증한 결과는 아니다.
[PR #84](https://github.com/Middleages/thread-dock/pull/84)를 `agent/engine-retirement-slice7` base로 게시했다.

## PR #82의 일곱 번째 정리 결과

[Issue #81](https://github.com/Middleages/thread-dock/issues/81)에서 미사용 backend ConfirmProtectedChange
진입점 두 개를 제거한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-backend-confirm.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice7-ledger.md)에 caller·mixed-test·state 보존 경계와 검증 결과를 기록한다.
현재 protected state/gate/comment/event와 저장된 confirmation 상태의 대기·재개 검증은 유지한다.
Task는 `ec80575`에서 ACCEPT했다. 최초 gate의 UI worker startup timeout을 기록했고 source/config 변경 없이
`VITEST_MAX_WORKERS=1` 환경의 `d40b693` gate에서 전체 Go·UI 26개·frontend build가 통과했다.
정확한 환경 병목은 미확정이다. 전체 branch 리뷰는 `f44082c7bcc8cab6cbb1fc75a62b92e14a736cca`에서
ACCEPT했으며 남은 finding은 없다. [PR #82](https://github.com/Middleages/thread-dock/pull/82)는
`6aced75a44d01b36f9ac632750609cd80c3dbe7a`에 병합됐다. Issue #81과 보드는 완료 상태다.

## PR #80의 여섯 번째 정리 결과

[Issue #79](https://github.com/Middleages/thread-dock/issues/79)에서 옛 confirm CLI adapter/wiring을 제거한다.
[계획](docs/superpowers/plans/2026-09-11-engine-retirement-confirm-cli.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice6-ledger.md)에 계약·보존 경계·검증 상태를 기록한다.
orchestrator의 protected-change 구현·상태와 나머지 CLI는 보존한다.
CLI Task는 `7234f66`에서 ACCEPT했다. 최초 gate `80c9a0d`에서 기존 동시 Advance 테스트의
동기화 결함을 발견해 테스트만 수정했고, 20회 반복·잠금 테스트 및 `eb62e6e`의 독립 리뷰를 통과했다.
수정 후 `98861c8`의 gate는 전체 Go·UI 26개·frontend build까지 통과했다. 제품 lock 구현은 불변이며
전체 리뷰는 `60df27dbbcf69c901caa39d4e832f69bfe1bb14c`에서 ACCEPT했다. 남은 blocking 사항은 없다.
PR #78을 main에 병합한 뒤 PR #80의 base를 main으로 바꿔 병합했다. 사용자 지시로 수행했으며
PR #80 merge SHA는 `f9fb7256e625e116493729c5accc390a2b8ec953`다. Issue #79와 보드 항목도 완료로 갱신했다.

## PR #78의 다섯 번째 정리 결과

[Issue #77](https://github.com/Middleages/thread-dock/issues/77)은 호출자가 없는 옛 `internal/integration`
구현과 전용 테스트를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-v1-integration.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice5-ledger.md)에 정확한 경계와 검증 상태를 기록한다.
현재 `internal/workrun.ReviewIntegrationService`, shared Git/state/CLI 및 Monitor는 유지한다.
Task 리뷰는 `6a4bb17`에서 ACCEPT했고 통합 `e1efc5d`의 단일 tmpfs make check는
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch 리뷰는
`3022baba83bebcb2a77b3f895100829b95d77a7b`에서 ACCEPT했으며 남은 finding은 없다.
[PR #78](https://github.com/Middleages/thread-dock/pull/78)은 `13b20d3dc1e75f55016dadb87993884402ee8088`에 병합됐다.
Issue #77은 닫혔고 보드의 Issue/PR도 Done으로 갱신했다.

## PR #76의 네 번째 정리 결과

[Issue #75](https://github.com/Middleages/thread-dock/issues/75)에서 미사용 worktree revert API 여섯 개와
전용 테스트를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-worktree-revert.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice4-ledger.md)에 소유 경로·호출 근거·검증 상태를 기록한다.
사용 중인 CreateManagedWorktree/PushBranch, conflict/path helper와 PushBranch의 no-force 검증은 유지한다.
Task 리뷰는 `537bda2`에서 ACCEPT했고 통합 `7a620c4`의 단일 tmpfs `make check`는
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch 리뷰는
`acfbf7be0c22fd333874f1e26539debb74abfd12`에서 ACCEPT했으며 남은 finding은 없다.
[PR #76](https://github.com/Middleages/thread-dock/pull/76)은 사용자 지시로
`5f5b31e519074f599a1d951cf6c7278e88a4cfef`에 병합됐다. Issue #75는 닫혔고 보드의 Issue/PR도 Done으로 갱신했다.

## PR #74의 세 번째 정리 결과

[Issue #73](https://github.com/Middleages/thread-dock/issues/73)은 PR #72 이후 호출자가 없어진
GitHub safe-draft API 세 개와 전용 테스트를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-safe-draft.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice3-ledger.md)에 정확한 경계와 검증 상태를 기록한다.
사용 중인 DraftPR 요청 타입·크기 제한·PR 생성/조회 기능은 유지하고 worktree helper 정리는 후속으로 남긴다.
Task는 `fea410c`에서 독립 리뷰 ACCEPT를 받았고 통합 `0325d41`의 단일 tmpfs `make check`는
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch 리뷰는
`bc1959226b3abd88f21d9ba92863fcf5e7be937d`에서 ACCEPT했으며 남은 finding은 없다.
[PR #74](https://github.com/Middleages/thread-dock/pull/74)는 사용자 지시로
`b2acf4c8f15c48901a0bfd0e6e50800b89a215f9`에 병합됐다. Issue #73은 닫혔고 보드의 Issue/PR을 Done으로 갱신했다.

## PR #72의 두 번째 정리 결과

[Issue #71](https://github.com/Middleages/thread-dock/issues/71)의 범위는 옛 `create-revert` CLI 경로와
전용 adapter/service다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-create-revert.md)과
[진행 ledger](docs/operator/2026-09-11-engine-retirement-slice2-ledger.md)에 정확한 소유 경로·공유 계약·검증 상태를 기록한다.
제거 명령은 production 설정·인증·저장소 조회 없이 기존 usage/exit 2로 끝나는 계약이다.
현재 Monitor·runner·공용 Git/worktree 및 나머지 CLI 동작은 보존한다.
Task 리뷰는 수정 head `2f5c841`에서 ACCEPT했고, 통합 SHA `be4b44b`의 tmpfs `make check`가
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch의 fresh Sol 리뷰는
`61745bac6f8500b3239a73314f0b26a3691bc2c3`에서 ACCEPT했으며 남은 blocking 사항은 없다.
[PR #72](https://github.com/Middleages/thread-dock/pull/72)는 사용자 지시로 main의
`a2f35f67ce58da5d787bd4c03165e9292624e15b`에 병합됐다. Issue #71은 닫혔고 보드의 Issue/PR도
Done·완료 근거로 갱신했다. 후속 작업은 남은 revert helper의 호출 여부를 별도로 확인해 정한다.

## PR #70의 첫 정리 결과

첫 작업은 [의존성 inventory](docs/operator/2026-09-10-engine-dependency-inventory.md),
[Issue 분류](docs/operator/2026-09-10-engine-retirement-issues.md),
[첫 삭제 계획](docs/superpowers/plans/2026-09-10-engine-retirement.md),
[검증·GitHub ledger](docs/operator/2026-09-10-engine-retirement-ledger.md)에서 추적한다.
현재 Monitor의 `internal/runner` 직접 사용을 보존하고 Linux/Windows 그래프에서 importer가 없는
옛 `internal/monitorcli` 두 파일을 제거했다. Luna focused 검사와 fresh Sol Task 리뷰를 통과했으며,
통합 SHA `ff977a2`의 첫 `make check`는 ext4 `/tmp`에서 fixture fsync 누적으로 timeout 실패했다.
코드·테스트·Makefile·dependency를 바꾸지 않고 `TMPDIR`만 `/dev/shm`으로 옮긴 같은 SHA의 새 tuple은
shell·gofmt·vet·전체 Go·UI 26 tests·frontend build까지 통과했다. 전체 branch 리뷰의 기존 BLOCK은
gate 미완료와 ledger 근거 누락 두 건이었으며, 문서 수정 후 fresh Sol의 scoped 재리뷰는
`ca2cae680a627aee222f2279d866fa5b24e52ee7`에서 ACCEPT를 받았다.
build가 제거한 tracked `dist/.placeholder`는 원본 바이트로 복원했고 `git diff --exit-code`로
`ff977a2` 코드 트리와 동일함을 확인했다. 통합 branch의 `58a59a6`은 재리뷰 SHA와 전체 tree가 동일하다.
현재 blocking 사항은 없으며 나머지 engine은 v1/v2 호출 군집으로 남는다.
Issue #42·44·45·46·47은 superseded 종료 후보, #43·48은 새 방향 재작성 후보로 한국어 댓글을 게시했다.
기존 본문·열린 상태는 보존했다. 당시 Projects 조회 성공/0개와 Wiki 비활성 상태는 이후 생성·활성화
이전의 과거 근거이며, 현재 보드와 Wiki 상태는 위 운영 상태가 기준이다.

## 남은 제품 검증과 후속 구현

현재 계획 Task 1·2 구현, Task 3 정리와 Wails runtime `v2.15.0` alignment를 반영했다. Linux 통합
`make check`는 final reviewed SHA `325db89`에서 통과했다. 표준 Windows Wails build와 healthy
packaged live 화면, Windows→WSL read path, clipboard/status 및 process/task cleanup도 확인됐다.
남은 제품 검증 범위는 native 오류 상태의 실제 재현이며, fixture/UI degradation 근거를 native live
성공으로 확대하지 않는다. 독립 기능 두 개를 Issue·Projects 기록과 함께 끝까지 운용하는 완료 기준도
아직 검증하지 않았다. `325db89`의 같은 Linux gate tuple은 반복하지 않으며, 후속 code PR은 마지막
통합 `make check`를 새 SHA에서 한 번 수행한다.
Vite는 화면 개발·빌드에 남긴다. Go를 없애거나 브라우저 제품으로 다시 전환하지 않는다.
옛 엔진 제거 구현은 위 완료 범위와 ledger를 따른다. 남은 native/E2E 검증은 엔진 제거 완료와 별개다.

Windows의 Go→WSL 호출은 기존 Herdr pane 환경을 자동 상속하지 않는다.
실제 읽기 접근을 검증하고 환경값을 위조하지 않는다. 접근 실패는 명시적 blocker이며 새 실행 엔진을 만들 이유가 아니다.

## 보존

사용자 호스트 /home/appuser/dev_system/.worktrees/codex-runtime의 중단된 실험과 미커밋 파일은 보존한다.
사용자 기록의 cmd/agentctl/main.go, cmd/agentctl/main_test.go, internal/config/config.go,
internal/config/config_test.go를 reset·삭제·commit하거나 실험을 재개하지 않는다.
Herdr #3813 해결을 가정하지 않고 /tmp/threaddock-herdr-live.7YEhfV의 실패 invocation을 재사용하지 않는다.
PR #69는 Go 경로 이식과 Node 경로 제거를 포함해 이미 main에 병합됐다. 후속 PR의 main 병합은 사용자에게 남긴다.

## Codex 다음 세션 프롬프트

~~~text
/home/appuser/dev_system의 ThreadDock 작업을 이어가.
실제 Git 상태와 origin/main을 확인하고 미커밋 실험을 보존한 독립 worktree에서 작업해.
AGENTS.md, HANDOFF.md, PRODUCT.md, CONTEXT.md, ADR 0008,
현재 2026-09-10 Go/Wails 설계·구현 계획과 운영 문서를 읽어.

제품은 Go/Wails 데스크톱 모니터와 기존 React 화면이다. 브라우저 전용으로 바꾸지 마.
Go가 GitHub·Herdr 조회·결합을 담당하고, Herdr가 세션 실행을, Agent와 Skills가 개발·기록을 맡아.
Task 1·2의 Go/Wails GitHub·Herdr 경로와 Task 3의 Node 경로 제거가 반영되어 있다.
PR #70의 monitorcli 제거와 PR #72의 create-revert CLI/service 제거는 main에 병합됐다.
PR #74의 미사용 GitHub safe-draft 제거와 PR #76의 worktree revert 제거도 병합됐다.
PR #78·#80·#82·#84도 main에 병합됐다. 2026-09-12 기준 main은 사용자 추가 커밋을 포함한 `2db8f77`다.
최신 settings/GHES/Toolbox와 여섯 canonical Skill을 보존한다. 2026-09-11 추가 설계·검증 제한도 읽는다.
현재 Project는 https://github.com/users/Middleages/projects/1 이다.
2026-09-12 엔진 제거 branch에서는 옛 CLI·엔진과 전용 fixture를 8개 reviewed Task로 모두 제거했다.
현재 HANDOFF/ledger의 PR·검증·병합 상태를 먼저 확인하고 완료된 삭제를 반복하지 마.
Linux 통합 gate와 표준 Windows package/live acceptance는 reviewed SHA `325db89`에서 완료됐다.
healthy native run은 GitHub 69개 work item, Herdr 기본 session/3 agents, clipboard 188, `로컬 연결 정상`,
process/task cleanup을 확인했으며 native error-state만 unverified다.
Windows→WSL Herdr 읽기 접근은 실제 설치 조건으로 확인하고 HERDR_ENV를 임의 설정하지 마.
새 scheduler/runtime/Publisher나 로컬 Work 계약을 만들지 마.
Wiki는 활성화됐지만 페이지 발행은 아직 검증되지 않았다. 다음 engine slice는 현재 dependency
inventory로 경계를 먼저 증명하고 완료되지 않은 삭제를 가정하지 마.

Sol medium이 작은 Task를 계획·분배하고 Luna high가 구현해.
독립 작업만 worktree로 병렬화하고 고정 변경은 fresh Sol medium이 검토해.
수정 범위 focused 테스트를 사용해. `325db89`에서 통과한 같은 Linux 통합 tuple은 반복하지 말고,
후속 code PR의 마지막 `make check`만 새 통합 SHA에서 한 번 실행해. root-owned Windows Wails
build/live evidence와 component CLI 관찰 및 fixture 결과를
구분해. native error-state는 아직 unverified이며, 과거 `UtilAcceptVsock:281: accept4 failed 110`은
superseded diagnostic으로만 기록한다.
PR·Issue는 한국어로 작성하고 main 병합은 나에게 남겨.
~~~
