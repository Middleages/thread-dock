# ThreadDock 진행 설계 정정과 다음 작업

## 최상위 기준

사용자 요구는 **Go 모니터 도구**다. Windows Go/Wails 앱과 기존 React 화면을 유지한다.
“심플하게”는 ThreadDock 자체 실행 엔진을 줄이라는 의미이며 브라우저 전용 전환은 승인되지 않았다.
현재 기준: [ADR 0008](docs/adr/0008-github-first-skills-before-engine.md),
[설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md).

## PR #69에서 완료된 상태

- PR #69의 최종 head `f35e248`은 main의 merge commit `3f4bebf`로 병합됐다.
- 2026-09-10 재확인에서 `gh auth status`는 `read:project` scope를 포함했고 Projects 조회는 성공했다. `Middleages`의 Projects와 저장소 ProjectsV2는 각각 0개, 열린 PR은 0개이며 Issue #42~48의 project item도 모두 0개다. Wiki는 비활성화 상태다.
- Task 1·2가 Go/Wails `GetMonitorSnapshot` 경로에 GitHub와 Herdr 관찰을 연결했다. 공유 Go/TS wire는 유지한다.
- Task 3가 `monitor/frontend/server/`의 Node adapter·전용 테스트, Vite middleware와 HTTP monitor endpoint를 제거했다. Vite는 React 화면 개발·빌드만 담당한다.
- 화면에는 GitHub 업무·근거, 단일 선택 업무 Herdr 연결, 관찰 세션·미연결 Agent, degraded/notices, 안전한 외부 링크와 handoff 복사만 남겼다. 비기능 상단 메뉴, 자동화 작업, 옛 Work/발행 표시는 제거했다.
- Linux fixture/UI/build 근거와 managed-pane live gh/Herdr 근거는 서로 구분한다. 상위에서 전달된 외부 관찰(원본 transcript 없음)으로 실제 Windows→WSL workstation의 `gh auth status`는 성공했고, bare `wsl.exe --exec herdr`는 PATH lookup에 실패했으며, absolute `/home/appuser/.local/bin/herdr`의 status와 agent list는 성공했다. 이 component 관찰 자체는 Windows→WSL Monitor 읽기 검증이 아니며, 당시 Task 3 문서 worker는 GitHub Issue/PR/Projects/Wiki 쓰기를 수행하지 않았다. 이번 후속 세션의 쓰기는 아래 ledger에 따로 기록한다.
- Final reviewed product/config/dependency SHA는 `325db89`다. Windows native 값은 `THREADDOCK_REPOS=Middleages/thread-dock`, `THREADDOCK_PROJECTS` unset, `THREADDOCK_WSL_DISTRIBUTION=Ubuntu`, `THREADDOCK_SESSIONS_FILE=/tmp/threaddock-aeca770-sessions.json`이다.
- Windows user-local toolchain은 Go `1.27.0 windows/amd64`, Node `26.8.1`, Wails CLI/runtime `v2.15.0`이며, 공식 Go/Node checksum은 제공된 범위에서 일치했다. final SHA의 matching `wails build`는 exit 0, `1m9.285s`에 완료됐고 `monitor\build\bin\ThreadDockMonitor.exe`와 bindings/frontend/assets/app stages `Done`을 확인했다. native child console은 표시되지 않았다.
- Healthy packaged live acceptance는 GitHub 69개 work item의 약 14초 동기화, Herdr 기본 session과 3개 Agent의 약 30초 관찰, handoff 성공 toast와 실제 clipboard 길이 `188`(repository name 포함), 초록 점과 `로컬 연결 정상` 문구의 일치를 확인했다. 앱 종료 후 Monitor process 수는 `0`, `ThreadDockValidation66158d9` scheduled task는 없음, 관련 process도 `0`이었다. WSL interop은 복구됐고 computer-use는 파일이나 worktree를 수정하지 않았다.
- Native run에서 오류가 발생하지 않아 native error/degraded 상태는 검증하지 않았다. 결합 degradation은 fixture/UI 테스트 근거만 있다. `UtilAcceptVsock:281: accept4 failed 110`은 superseded historical diagnostic이고, 이전 `aeca770` plain `go build` 관찰과 `66158d9` staging build는 최종 표준 package evidence가 아니다.
- 기존 Wiki 링크는 문서 참고이며 Wiki 실제 반영과 구분한다.

## 병합 후 정리 작업

이번 작업은 [의존성 inventory](docs/operator/2026-09-10-engine-dependency-inventory.md),
[Issue 분류](docs/operator/2026-09-10-engine-retirement-issues.md),
[첫 삭제 계획](docs/superpowers/plans/2026-09-10-engine-retirement.md),
[검증·GitHub ledger](docs/operator/2026-09-10-engine-retirement-ledger.md)에서 추적한다.
현재 Monitor의 `internal/runner` 직접 사용을 보존하고 Linux/Windows 그래프에서 importer가 없는
옛 `internal/monitorcli` 두 파일만 첫 삭제 대상으로 정했다. 나머지 engine은 v1/v2 호출 군집으로 남는다.
Issue #42·44·45·46·47은 superseded 종료 후보, #43·48은 새 방향 재작성 후보로 한국어 댓글을 게시했다.
기존 본문·열린 상태는 보존했다. Projects는 조회 성공/0개로 갱신 대상이 없고 Wiki는 비활성화돼 실제 쓰기가 없다.

## 남은 제품 검증과 후속 구현

현재 계획 Task 1·2 구현, Task 3 정리와 Wails runtime `v2.15.0` alignment를 반영했다. Linux 통합
`make check`는 final reviewed SHA `325db89`에서 통과했다. 표준 Windows Wails build와 healthy
packaged live 화면, Windows→WSL read path, clipboard/status 및 process/task cleanup도 확인됐다.
남은 제품 검증 범위는 native 오류 상태의 실제 재현이며, fixture/UI degradation 근거를 native live
성공으로 확대하지 않는다. 독립 기능 두 개를 Issue·Projects 기록과 함께 끝까지 운용하는 완료 기준도
아직 검증하지 않았다. `325db89`의 같은 Linux gate tuple은 반복하지 않으며, 후속 code PR은 마지막
통합 `make check`를 새 SHA에서 한 번 수행한다.
Vite는 화면 개발·빌드에 남긴다. Go를 없애거나 브라우저 제품으로 다시 전환하지 않는다.
옛 엔진 대량 삭제는 필요한 모니터 의존성을 확인한 뒤 후속 정리한다.

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
Linux 통합 gate와 표준 Windows package/live acceptance는 reviewed SHA `325db89`에서 완료됐다.
healthy native run은 GitHub 69개 work item, Herdr 기본 session/3 agents, clipboard 188, `로컬 연결 정상`,
process/task cleanup을 확인했으며 native error-state만 unverified다.
Windows→WSL Herdr 읽기 접근은 실제 설치 조건으로 확인하고 HERDR_ENV를 임의 설정하지 마.
새 scheduler/runtime/Publisher나 로컬 Work 계약을 만들지 마.

Sol medium이 작은 Task를 계획·분배하고 Luna high가 구현해.
독립 작업만 worktree로 병렬화하고 고정 변경은 fresh Sol medium이 검토해.
수정 범위 focused 테스트를 사용해. `325db89`에서 통과한 같은 Linux 통합 tuple은 반복하지 말고,
후속 code PR의 마지막 `make check`만 새 통합 SHA에서 한 번 실행해. root-owned Windows Wails
build/live evidence와 component CLI 관찰 및 fixture 결과를
구분해. native error-state는 아직 unverified이며, 과거 `UtilAcceptVsock:281: accept4 failed 110`은
superseded diagnostic으로만 기록한다.
PR·Issue는 한국어로 작성하고 main 병합은 나에게 남겨.
~~~
