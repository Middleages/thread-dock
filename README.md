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
- managed-pane live 근거(상위에서 전달된 외부 관찰, 원본 transcript 없음): 실제 Windows→WSL workstation에서 `gh auth status`가 성공했고, bare `wsl.exe --exec herdr`는 PATH lookup에 실패했으며, absolute `/home/appuser/.local/bin/herdr`의 status와 agent list는 성공했습니다. 이는 Wails Monitor의 Windows→WSL 읽기 경로를 검증한 증거가 아니며, 이 Task에서 GitHub/Wiki 쓰기를 수행했다는 뜻도 아닙니다.
- Windows toolchain 사실: user-local Go는 `go1.27.0 windows/amd64`, Wails는 `v2.10.2`를 보고합니다. 설치·버전 확인은 Windows Wails build/app 실행 성공을 뜻하지 않습니다.
- 미검증: Windows Wails build/app 실행과 실제 Windows→WSL gh/Herdr 접근. 따라서 live 데스크톱 통합 완료로 표시하지 않습니다.

## 사용·개발 지침

[운영 흐름](docs/operator/github-first-quickstart.md)의 중앙/기능 세션 프롬프트와
project-template/.agents/skills의 plan-work, open-agent-session, implement-task, review-change, record-work를 사용합니다.
기존 Work 등록이나 Go 실행 계약은 필요하지 않습니다.

- [제품 역할](PRODUCT.md)
- [용어와 책임](CONTEXT.md)
- [Go/Wails 진행 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)
- [구현 계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)
- [현재 상태와 Codex 착수 프롬프트](HANDOFF.md)
