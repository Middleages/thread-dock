---
status: accepted
date: 2026-09-10
---

# Go/Wails 모니터와 GitHub 중심 Agent Skill을 사용한다

## 정정 이유

사용자는 “Go 모니터 도구”를 만들겠다고 명확히 정정했다.
“심플하게”를 Node/Vite 브라우저 전용 제품으로 해석한 이전 결정은 철회한다.
GitHub 중심 업무 기록과 Herdr 실행 소유권은 유지한다.

## 결정

- 제품은 Windows Go/Wails 데스크톱 모니터다. 기존 React/TypeScript 화면을 재사용한다.
- Vite는 화면 개발·빌드에만 사용한다. 별도 Node 조회 서버와 브라우저 제품 경로는 제거한다.
- Go 백엔드가 GitHub·Herdr 조회·결합을 맡고 Wails binding으로 화면에 전달한다.
- Windows/WSL 경계를 유지하되 기존 agentctl Work 상태와 Go 실행 엔진에 의존하지 않는다.
- 설치된 Herdr의 데스크톱 읽기 접근은 실제로 검증한다. 환경값을 위조하거나 새 실행 엔진으로 해결하지 않는다.
- 업무 식별자는 GitHub Issue URL이며 로컬 Work/Contract를 만들지 않는다.
- 중앙 관제와 기능 리더의 계획·분배·구현·리뷰·커밋·기록은 Agent와 Skills가 맡는다.
- GitHub 쓰기는 승인 범위의 gh/MCP/Git으로 수행하며 자체 Publisher는 신규 경로에 포함하지 않는다.
- 재개 첫 범위는 위치 안내·링크·handoff 복사다. Herdr와 Agent가 실제 실행을 맡는다.

## 대체 범위와 상태

ADR 0007의 Herdr 소유권과 Go/Wails 제품 형태는 유지한다.
ADR 0002의 Windows/WSL 경계는 유지하고 agentctl aggregate/coordinator 의존 부분은 대체한다.
PR #69의 브라우저 전용 실행 결정과 그 착수 프롬프트는 폐기한다.
후속 구현은 [현재 계획](../superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)에 따라
GitHub·Herdr 조회를 Go/Wails로 이식하고 별도 Node/Vite 조회 경로를 제거했다. PR #69의 최종 head
`f35e248`은 main의 merge commit `3f4bebf`로 병합됐다. 옛 Go 실행 엔진은 새 Monitor가 실제로
사용하는 runner·경로·링크 기능을 먼저 식별한 뒤 작은 slice로 정리한다.
사용자 호스트의 미커밋 실험은 계속 보존한다.
