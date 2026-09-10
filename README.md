# ThreadDock

**Go/Wails 데스크톱 모니터 + 프로젝트 Skill**로 GitHub 중심 멀티 세션 개발을 지원합니다.
중앙 Agent가 기능을 분배하고, 각 Herdr 기능 세션이 native subagent로 구현·독립 리뷰를 진행합니다.
Go 모니터는 GitHub 업무 기록과 Herdr 실행 상태를 함께 보여주고 작업 재개를 돕습니다.

## 현재 상태

2026-09-10 사용자 정정에 따라 Go/Wails 제품 방향을 고정했고, PR #69의 Task 1·2가
`GetMonitorSnapshot`으로 GitHub와 Herdr 관찰을 전달합니다. Task 3에서 별도 Node 조회 서버,
Vite middleware, HTTP monitor endpoint와 전용 테스트를 제거했습니다.
Vite/Node는 React 화면 개발·빌드에만 사용하며 제품 실행에는 Node HTTP 서버가 필요하지 않습니다.

현재 화면은 GitHub 업무·근거, 선택 업무의 단일 Herdr 연결, 관찰 세션·미연결 Agent,
degraded/notices 상태, 외부 링크와 handoff 복사를 제공합니다. 비기능 메뉴와 자동화/옛 Work·발행
표시는 포함하지 않습니다.

## 검증 범위

- Linux Go fixture 근거: Task 1·2에서 `go test ./monitor -run 'Test(GitHub|Commands|App|Herdr|Snapshot)'`가 통과했습니다. 이 Task는 Go 소스를 변경하지 않았습니다.
- Linux UI 근거: `npm exec vitest run src/bindings.test.ts src/monitor.test.tsx --reporter=verbose` — 2 files, 20 tests 통과.
- Linux build 근거: `npm run build` — TypeScript와 Vite production build 통과.
- Linux 통합 gate: final reviewed product/config/dependency SHA `325db89`에서 `make check`가 shell syntax, `go vet ./...`, 전체 Go 테스트, Vitest 26 tests, frontend production build까지 통과했습니다.
- managed-pane live 근거(상위에서 전달된 외부 관찰, 원본 transcript 없음): 실제 Windows→WSL workstation에서 `gh auth status`가 성공했고, bare `wsl.exe --exec herdr`는 PATH lookup에 실패했으며, absolute `/home/appuser/.local/bin/herdr`의 status와 agent list는 성공했습니다. 이는 Wails Monitor의 Windows→WSL 읽기 경로를 검증한 증거가 아니며, 이 Task에서 GitHub/Wiki 쓰기를 수행했다는 뜻도 아닙니다.
- Windows native 환경 값: user-local Go `1.27.0 windows/amd64`, Node `26.8.1`, Wails CLI/runtime `v2.15.0`, `THREADDOCK_REPOS=Middleages/thread-dock`, `THREADDOCK_PROJECTS`는 unset, `THREADDOCK_WSL_DISTRIBUTION=Ubuntu`, `THREADDOCK_SESSIONS_FILE=/tmp/threaddock-aeca770-sessions.json`입니다.
- Windows standard package/live 근거: final reviewed SHA `325db89`를 기준으로 matching Wails CLI/runtime `v2.15.0`의 `wails build`가 exit 0, `1m9.285s`에 완료됐고 `monitor\build\bin\ThreadDockMonitor.exe`를 생성했습니다. bindings/frontend/assets/app stages가 모두 `Done`이었고, native child console은 표시되지 않았습니다.
- 건강한 packaged live acceptance: GitHub 69개 work item이 약 14초에 동기화됐고 Herdr 기본 session과 3개 Agent가 약 30초에 관찰됐습니다. handoff 성공 toast와 실제 clipboard 길이 `188`(repository name 포함), 초록 점과 `로컬 연결 정상` 문구가 일치했습니다. 앱 종료 후 Monitor process 수는 `0`이었고, `ThreadDockValidation66158d9` scheduled task는 없으며 관련 process도 `0`으로 확인됐습니다. WSL interop은 복구됐고 computer-use는 파일이나 worktree를 수정하지 않았습니다.
- 오류 상태 범위: native run에서 오류가 발생하지 않았으므로 native degraded/error 화면은 검증하지 않았습니다. 결합 degradation 동작은 Linux fixture/UI 테스트로만 확인했으며, 이전 `UtilAcceptVsock:281: accept4 failed 110` 관찰은 원인 진단용 historical evidence로 superseded됐습니다.
- 참고 관찰: 이전 `aeca770`의 plain `go build` binary가 GitHub 69개 항목과 Herdr 기본 session/4 agents를 렌더링한 것은 최종 표준 Wails package 증거가 아니며, `66158d9` staging build evidence도 `325db89`의 최종 run으로 superseded됐습니다.

## 사용·개발 지침

[운영 흐름](docs/operator/github-first-quickstart.md)의 중앙/기능 세션 프롬프트와
project-template/.agents/skills의 plan-work, open-agent-session, implement-task, review-change, record-work를 사용합니다.
기존 Work 등록이나 Go 실행 계약은 필요하지 않습니다.

- [제품 역할](PRODUCT.md)
- [용어와 책임](CONTEXT.md)
- [Go/Wails 진행 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)
- [구현 계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)
- [OpenCode role-agent 운영 참고 (레거시 v1)](docs/operator/opencode-role-agents.md)
- [현재 상태와 Codex 착수 프롬프트](HANDOFF.md)
