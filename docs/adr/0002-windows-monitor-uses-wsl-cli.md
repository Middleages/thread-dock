---
status: accepted
date: 2026-09-10
---

# Windows Go/Wails Monitor는 WSL의 조회 도구를 사용한다

Windows Go/Wails 앱과 WSL 개발 환경의 경계는 유지한다.
Go 백엔드가 명시한 배포판의 gh·Herdr 읽기 명령을 인자 배열로 호출한다.
별도 HTTP/Node 서버나 상주 bridge를 추가하지 않는다.

[ADR 0008](0008-github-first-skills-before-engine.md)에 따라 이전의
`agentctl project status --all --json`·로컬 Work aggregate·백그라운드 coordinator 의존 결정은 대체한다.
앱 종료와 무관하게 개발 세션이 유지되는 이유는 Herdr가 실행을 소유하기 때문이다.

WSL 인증·경로·Herdr 접근은 실제 사용자 환경에서 검증한다.
Windows에서 실행한 WSL 명령에 기존 Herdr pane 환경이 자동 상속된다고 가정하지 않는다.
구체적인 조회·실패·재개 동작은 [현재 설계](../superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)를 따른다.
