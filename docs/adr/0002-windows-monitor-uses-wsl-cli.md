---
status: accepted
---

# Windows Monitor는 WSL CLI 계약을 사용한다

Monitor는 wsl.exe를 통해 agentctl의 구조화된 JSON 명령을 호출한다.
HTTP 서버를 추가하거나 WSL 상태 파일을 직접 편집하지 않는다.

2026-09-07 [ADR 0006](0006-project-workflow-mvp.md)에서 여러 프로젝트 동시 진행을
확정했으므로 기존 단일 active run 가정을 폐기한다.
3~5초 polling은 aggregate status 한 번으로 처리하고 GitHub refresh와 분리한다.
실행은 백그라운드 coordinator가 소유하며 UI 종료·polling에 의존하지 않는다.
실제 병목이 측정되기 전에는 transport를 추가하지 않는다.
