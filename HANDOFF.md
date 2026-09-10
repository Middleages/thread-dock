# ThreadDock 진행 설계 정정과 다음 작업

## 최상위 기준

사용자 요구는 **Go 모니터 도구**다. Windows Go/Wails 앱과 기존 React 화면을 유지한다.
“심플하게”는 ThreadDock 자체 실행 엔진을 줄이라는 의미이며 브라우저 전용 전환은 승인되지 않았다.
현재 기준: [ADR 0008](docs/adr/0008-github-first-skills-before-engine.md),
[설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md).

## 현재 사실

- 문서 정정 전 PR #69 head: 3d832bc9ca6d72365d40e8e2318bae5ea6ded894. 당시 main: 7f5fa0e4edf4269f09a12d9181e15f03e015478c. 착수 시 최신 상태를 다시 조회한다.
- 이 PR에는 다섯 프로젝트 Skill과 React 화면, 별도 Node/Vite GitHub·Herdr 조회 구현이 있다.
- 기존 Go/Wails 앱은 아직 옛 agentctl/local Work 경로다. 새로운 GitHub·Herdr Go 조회는 미구현이다.
- 이번 커밋은 설계·계획·ADR·제품/착수/운영 문서와 coordinator 지침만 정정한다. 제품 코드를 이식하거나 삭제하지 않는다.
- 과거 Node/UI 테스트·fixture·빌드·리뷰는 해당 구현의 증거다. Go/Wails 통합·Windows/WSL 실제 동작 증거로 사용하지 않는다.
- 작성 환경에는 Go·사용자 Windows/WSL·live Herdr가 없어 이를 실행 검증하지 않았다.
- 이전 조회 시 ThreadDock Wiki는 비활성화였다. Wiki 링크와 Wiki 실제 반영을 구분한다.

## 다음 구현

현재 계획 Task 1: Go/Wails에서 실제 GitHub 업무를 읽고 표시한다.
Task 2: 같은 Go 경로에 Herdr 읽기 관찰·정확한 연결·handoff 안내를 붙인다.
Task 3: 이식된 Node 서버·fetch fallback과 중복/미완성 화면을 제거하고 실제 앱을 검증한다.
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
계획 Task 1부터 기존 Node 조회 동작을 Go/Wails로 옮겨.
Task 2에서 Herdr 연결, Task 3에서 Node 서버 제거와 실제 Windows 앱 검증을 진행해.
Windows→WSL Herdr 읽기 접근은 실제 설치 조건으로 확인하고 HERDR_ENV를 임의 설정하지 마.
새 scheduler/runtime/Publisher나 로컬 Work 계약을 만들지 마.

Sol medium이 작은 Task를 계획·분배하고 Luna high가 구현해.
독립 작업만 worktree로 병렬화하고 고정 변경은 fresh Sol medium이 검토해.
수정 범위 focused 테스트를 사용하고 마지막 통합 make check만 한 번 수행해.
Windows Wails 빌드·앱 실행·실제 gh/Herdr와 fixture 결과를 구분해.
PR·Issue는 한국어로 작성하고 main 병합은 나에게 남겨.
~~~
