# ThreadDock

**Go/Wails 데스크톱 모니터 + 프로젝트 Skill**로 GitHub 중심 멀티 세션 개발을 지원합니다.
중앙 Agent가 기능을 분배하고, 각 Herdr 기능 세션이 native subagent로 구현·독립 리뷰를 진행합니다.
Go 모니터는 GitHub 업무 기록과 Herdr 실행 상태를 함께 보여주고 작업 재개를 돕습니다.

## 현재 상태

2026-09-10 사용자 정정에 따라 Go/Wails 제품 방향을 다시 고정했습니다.
PR #69의 별도 Node/Vite 조회 서버는 Go 백엔드로 옮기고 제거할 대상입니다.
현재는 문서 정정 단계이며 Go 이식과 실제 Windows/WSL 실행은 아직 완료하지 않았습니다.
이전 npm run dev 브라우저 전용 실행 안내는 제품 사용 지침으로 사용하지 않습니다.
Vite와 Node는 React 화면 개발·빌드에 사용합니다.

## 사용·개발 지침

[운영 흐름](docs/operator/github-first-quickstart.md)의 중앙/기능 세션 프롬프트와
project-template/.agents/skills의 plan-work, open-agent-session, implement-task, review-change, record-work를 사용합니다.
기존 Work 등록이나 Go 실행 계약은 필요하지 않습니다.

- [제품 역할](PRODUCT.md)
- [용어와 책임](CONTEXT.md)
- [Go/Wails 진행 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)
- [구현 계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)
- [현재 상태와 Codex 착수 프롬프트](HANDOFF.md)
