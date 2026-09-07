---
status: accepted
---

# Go와 Wails v2로 로컬 도구를 구현한다

WSL의 Go Orchestrator CLI와 Windows Wails v2 앱, React·TypeScript UI를 사용한다.
Main Agent와 Monitor는 같은 구조화된 CLI 계약으로 업무 상태를 읽고 제어한다.

2026-09-07 [ADR 0006](0006-project-workflow-mvp.md)에 따라 기술 선택을 유지한다.
자동 업데이트와 rollback 배포 체계는 이번 MVP 완료 조건에서 제외한다.
UI는 명령 schema의 호환성으로 지원 기능을 확인한다.
