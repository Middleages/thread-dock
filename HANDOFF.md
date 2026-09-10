# ThreadDock 진행 설계 정정과 다음 작업

## 최상위 기준

사용자 요구는 **Go 모니터 도구**다. Windows Go/Wails 앱과 기존 React 화면을 유지한다.
“심플하게”는 ThreadDock 자체 실행 엔진을 줄이라는 의미이며 브라우저 전용 전환은 승인되지 않았다.
현재 기준: [ADR 0008](docs/adr/0008-github-first-skills-before-engine.md),
[설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md).

## 현재 상태 (Task 3 이후)

- Task 1·2가 Go/Wails `GetMonitorSnapshot` 경로에 GitHub와 Herdr 관찰을 연결했다. 공유 Go/TS wire는 유지한다.
- Task 3가 `monitor/frontend/server/`의 Node adapter·전용 테스트, Vite middleware와 HTTP monitor endpoint를 제거했다. Vite는 React 화면 개발·빌드만 담당한다.
- 화면에는 GitHub 업무·근거, 단일 선택 업무 Herdr 연결, 관찰 세션·미연결 Agent, degraded/notices, 안전한 외부 링크와 handoff 복사만 남겼다. 비기능 상단 메뉴, 자동화 작업, 옛 Work/발행 표시는 제거했다.
- Linux fixture/UI/build 근거와 managed-pane live gh/Herdr 근거는 서로 구분한다. 상위에서 전달된 외부 관찰(원본 transcript 없음)으로 실제 Windows→WSL workstation의 `gh auth status`는 성공했고, bare `wsl.exe --exec herdr`는 PATH lookup에 실패했으며, absolute `/home/appuser/.local/bin/herdr`의 status와 agent list는 성공했다. 이는 Windows→WSL Monitor 읽기 검증이 아니며, 이 작업에서 GitHub Issue/PR/Projects/Wiki 쓰기는 수행하지 않았다.
- Windows toolchain은 user-local Go `go1.27.0 windows/amd64`와 Wails `v2.10.2`가 확인됐지만, build/app 실행은 아직 검증하지 않았다.
- 미검증: Windows Wails build/app 실행과 Wails Monitor 프로세스의 Windows→WSL 읽기 경로 및 live 데스크톱 통합. component-level CLI 관찰 자체는 상위 전달 근거로 기록한다.
- 기존 Wiki 링크는 문서 참고이며 Wiki 실제 반영과 구분한다.

## 다음 구현

현재 계획 Task 1·2 구현과 Task 3 정리를 반영했다. 다음은 fresh task review 후 통합 `make check`를
한 번 실행하고, Windows에서 설치된 Wails build/app과 Windows→WSL 읽기 접근을 별도로 확인하는 일이다.
Vite는 화면 개발·빌드에 남긴다. Go를 없애거나 브라우저 제품으로 다시 전환하지 않는다.
옛 엔진 대량 삭제는 필요한 모니터 의존성을 확인한 뒤 후속 정리한다.

Windows의 Go→WSL 호출은 기존 Herdr pane 환경을 자동 상속하지 않는다.
실제 읽기 접근을 검증하고 환경값을 위조하지 않는다. 접근 실패는 명시적 blocker이며 새 실행 엔진을 만들 이유가 아니다.

## 보존

사용자 호스트 /home/appuser/dev_system/.worktrees/codex-runtime의 중단된 실험과 미커밋 파일은 보존한다.
사용자 기록의 cmd/agentctl/main.go, cmd/agentctl/main_test.go, internal/config/config.go,
internal/config/config_test.go를 reset·삭제·commit하거나 실험을 재개하지 않는다.
Herdr #3813 해결을 가정하지 않고 /tmp/threaddock-herdr-live.7YEhfV의 실패 invocation을 재사용하지 않는다.
PR #69는 Go 경로가 준비되기 전 그대로 병합할 완성품이 아니다. main 병합은 사용자에게 남긴다.

## Codex 다음 세션 프롬프트

~~~text
/home/appuser/dev_system의 ThreadDock 작업을 이어가.
실제 Git 상태와 PR #69의 최신 head를 확인하고 미커밋 실험을 보존한 독립 worktree에서 작업해.
AGENTS.md, HANDOFF.md, PRODUCT.md, CONTEXT.md, ADR 0008,
현재 2026-09-10 Go/Wails 설계·구현 계획과 운영 문서를 읽어.

제품은 Go/Wails 데스크톱 모니터와 기존 React 화면이다. 브라우저 전용으로 바꾸지 마.
Go가 GitHub·Herdr 조회·결합을 담당하고, Herdr가 세션 실행을, Agent와 Skills가 개발·기록을 맡아.
Task 1·2의 Go/Wails GitHub·Herdr 경로와 Task 3의 Node 경로 제거가 반영되어 있다.
다음은 fresh review 후 통합 gate와 실제 Windows 앱 검증이다.
Windows→WSL Herdr 읽기 접근은 실제 설치 조건으로 확인하고 HERDR_ENV를 임의 설정하지 마.
새 scheduler/runtime/Publisher나 로컬 Work 계약을 만들지 마.

Sol medium이 작은 Task를 계획·분배하고 Luna high가 구현해.
독립 작업만 worktree로 병렬화하고 고정 변경은 fresh Sol medium이 검토해.
수정 범위 focused 테스트를 사용하고 마지막 통합 make check만 한 번 수행해.
Windows Wails 빌드·앱 실행·실제 gh/Herdr와 fixture 결과를 구분해.
PR·Issue는 한국어로 작성하고 main 병합은 나에게 남겨.
~~~
