---
status: accepted
date: 2026-09-10
---

# Herdr가 대화형 실행을 소유한다

Herdr는 session/workspace/tab/pane과 상위 Agent의 실행·재접속을 관리한다.
각 상위 Agent는 native subagent를 직접 조정한다.
ThreadDock은 GitHub 업무 기록과 Herdr 관찰을 연결하는 Monitor와 Skill을 제공한다.

이 소유권 결정은 유지한다. 초기 문서의 workspace당 Agent 하나 연결 규칙,
로컬 Work/Contract와 Go evidence 의존성은 [ADR 0008](0008-github-first-skills-before-engine.md)이 대체한다.
명시적으로 지정된 여러 기능/리뷰 세션을 허용하고, 신규 사용에는 별도 실행 엔진을 요구하지 않는다.

현재 세부 기준은 [첫 사용 설계](../superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)다.

