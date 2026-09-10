---
status: accepted
date: 2026-09-10
---

# GitHub 업무와 Agent Skill로 먼저 사용한다

사용자는 중앙 관제의 기능 분배, 기능별 Herdr 세션과 내부 subagent 개발,
GitHub Projects·Issue·설계/PR·Wiki 기록, Monitor의 관찰·재개 지원을 승인했다.
추가로 “심플하게 구현해서 당장 사용가능하도록” 요청했다.

## 결정

- 업무의 식별자는 GitHub Issue URL이다. 로컬 Work/Contract 생성은 요구하지 않는다.
- 중앙 관제와 기능 세션의 책임을 Skill에 두고, 기능 리더/구현자가 Git 커밋을 수행한다.
- 승인 범위의 GitHub 쓰기는 gh/MCP/Git으로 한다. Go Publisher는 선택 사항이다.
- 기능을 배정하거나 세션을 선택할 때 Issue와 Herdr 위치를 명시적으로 연결한다.
- 같은 workspace의 여러 Agent는 허용한다. 쓰는 기능끼리는 독립 worktree를 사용한다.
- Monitor 첫 구현은 실제 GitHub 조회부터 시작한다. 로컬 Work snapshot으로 대체하지 않는다.
- 첫 실행 경로는 기존 React 화면과 Vite의 로컬 read-only gh adapter다. Node·gh로 시작하며 Wails/Go 실행기를 요구하지 않는다. 브라우저용 조회 adapter를 새 작업 실행기로 확장하지 않는다.
- 첫 UI의 재개 지원은 링크·handoff 복사·세션 위치 안내다. 실행 조작은 기존 Agent와 Herdr로 한다.

## 대체 범위

ADR 0007의 Herdr 소유권은 유지한다.
2026-09-07 설계의 Contract/Gate/Publisher/Finalize와 2026-09-10 초기 5-Task 계획의
로컬 Work 기반 조회·workspace당 Agent 하나 규칙은 신규 경로에서 폐기한다.
이전 코드를 지우는 일은 첫 사용의 선행 조건이 아니며 미커밋 실험을 보존한다.

## 결과

Agent의 모든 내부 동작을 프로그램이 보증하지 않는다.
대신 기능별 변경 분리, 독립 리뷰·실제 테스트, GitHub 근거와 handoff로 개발을 운영한다.
반복되는 실제 불편이 확인될 때만 작은 자동화를 추가한다.
