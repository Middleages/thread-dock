# ThreadDock

GitHub에 업무와 문서를 남기고, Herdr에서 기능별 Agent 세션을 운영하는 개발 워크플로입니다.
중앙 관제 Agent가 기능을 분배하고, 각 기능 세션이 native subagent로 구현과 독립 리뷰를 진행합니다.

## 지금 시작하기

[빠른 시작](docs/operator/github-first-quickstart.md)에 따라 project-template의 Skill을 작업 저장소에 복사합니다.
GitHub와 Herdr를 사용할 수 있는 기존 Codex/OpenCode 세션에서 바로 시작하며 agentctl 설치나 로컬 Work 등록은 필요하지 않습니다.

- plan-work: 중앙 계획과 기능 분배
- open-agent-session: 기존 기능 세션으로 돌아가기 또는 승인된 새 세션 열기
- implement-task: 기능 세션의 상세계획·구현·커밋·PR 준비
- review-change: 독립 리뷰
- record-work: Issue·Projects·설계·Wiki·handoff 기록

브라우저 Monitor는 기존 화면에서 GitHub Issue·PR과 선택한 Projects 보드를 직접 조회합니다.
저장소에서 다음과 같이 실행한 뒤 터미널의 로컬 주소를 엽니다.

```bash
cd monitor/frontend
npm ci
THREADDOCK_REPOS=Middleages/thread-dock npm run dev
```

Node.js 22.12 이상과 같은 환경의 gh 인증이 필요합니다. 여러 저장소·Projects 설정은 [빠른 시작](docs/operator/github-first-quickstart.md#브라우저-monitor-실행)을 참고하세요.
Herdr pane 안에서 `THREADDOCK_SESSIONS_FILE`에 연결 파일을 지정하면 해당 세션의 상태·위치·재개 안내도 표시합니다. 설정 방법은 빠른 시작을 참고하세요. 실제 설치 버전과 사용자 환경의 동작 확인은 남아 있습니다.
기존 Wails 앱은 이전 로컬 Work 경로를 유지합니다.

## 기준 문서

- [제품 목적](PRODUCT.md)
- [용어와 책임](CONTEXT.md)
- [간단한 첫 사용 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)
- [구현 순서](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)
- [현재 상태와 다음 세션](HANDOFF.md)

이전 Go 실행기와 Codex adapter 실험은 신규 경로의 선행 조건이 아닙니다.
변경 검증은 영향 범위에 집중하고, 제품 코드 통합 PR의 마지막에 전체 검사를 한 번 수행합니다.
