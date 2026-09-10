# ThreadDock 현재 상태와 다음 작업

2026-09-10 사용자 요청: “심플하게 구현해서 당장 사용가능하도록”.
현재 기준은 [ADR 0008](docs/adr/0008-github-first-skills-before-engine.md),
[설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)다.

## 바로 사용할 경로

[빠른 시작](docs/operator/github-first-quickstart.md)의 Skill 운영 경로부터 사용한다.
중앙은 기능 분배, 각 Herdr 기능 세션은 native subagent 구현·독립 리뷰·커밋·PR,
GitHub는 Projects·Issue·설계/PR·Wiki 기록을 맡는다.
기존 Work/Contract/Go 실행기 없이 시작한다.
project-template의 Skill을 실제 대상 저장소에서 직접 읽거나 필요한 폴더만 설치한다.

이번 변경은 지침·Skill·템플릿 검사 전환과 브라우저 Monitor의 GitHub 직접 조회 경로다.
빠른 시작의 npm run dev로 기존 화면을 연다. gh 인증은 실행하는 호스트의 인증을 사용한다.
Herdr live 통합과 사용자 WSL의 실제 실행은 완료로 주장하지 않는다.
Wails 앱은 이전 로컬 Work 조회 구현을 유지한다. 브라우저 모드와 혼동하지 않는다.

## 이번 변경 검증

- Skill 구조 검사와 문서 링크·JSON 예시·coordinator TOML 검사 통과. 문서/Skill 독립 리뷰 ACCEPT.
- Node 조회/서버 검사 8개, 화면/연결 검사 15개, 프런트엔드 build 통과. 코드 독립 리뷰 ACCEPT.
- 실제 Vite 서버와 fixture gh executable로 화면 파일·API 응답, 두 저장소의 Issue 구분, 정확한 PR 연결, Projects 우선순위/대기 필드 확인.
- 위 결과는 live GitHub/Herdr 성공이 아니다. 원격 브라우저의 localhost 차단으로 육안 확인, 사용자 WSL/Windows 실행은 미확인.
- 현재 작업 환경에 Go가 없어 저장소 전체 make check와 Go 템플릿 테스트는 실행하지 못했다. 전체 gate 통과로 표시하지 않는다.

## 다음 코드 작업

먼저 이 PR의 브라우저 Monitor를 실제 gh 환경에서 사용한다.
그 뒤 계획 C의 명시적 Herdr 연결과 읽기 전용 로컬 상태 표시다.
이전 herdr-first-01-status-wire Task를 재개하지 않는다.
기존 GitHub 조회 경로를 다시 구현하거나 Go 실행기 입력으로 바꾸지 않는다.
사용자에게 필요한 것은 업무 현황과 해당 세션으로 돌아가는 안내다.

## 보존할 실험

PR #68의 기준 main은 7f5fa0e4edf4269f09a12d9181e15f03e015478c였다.
착수 시 실제 main/PR 상태를 다시 읽고 이 값을 최신 상태로 가정하지 않는다.
이전 사용자 호스트의 /home/appuser/dev_system/.worktrees/codex-runtime 실험과 다음 미커밋 파일은
사용자 기록상 보존 상태다. 이번 원격 변경에서 직접 확인하거나 변경하지 않는다.

- cmd/agentctl/main.go
- cmd/agentctl/main_test.go
- internal/config/config.go
- internal/config/config_test.go

Herdr #3813의 성공을 가정하지 않는다. 이전 /tmp/threaddock-herdr-live.7YEhfV의 실패 invocation은 재사용하지 않는다.
기존 코드의 다른 테스트/실험을 현재 Skill 경로의 실행 조건으로 요구하지 않는다.

## 다음 세션 프롬프트

~~~text
ThreadDock의 최신 AGENTS.md, HANDOFF.md와 빠른 시작, 현재 설계·계획을 읽어.
우선 Skill 경로로 기존 GitHub 업무와 Herdr 기능 세션을 운영할 수 있는지 확인해.
중앙 관제는 기능 분배, 기능 세션은 상세계획·native subagent 구현·독립 리뷰·커밋·PR,
GitHub는 업무·결정·검증·Wiki 기록을 맡는다.

이미 구현한 브라우저 Monitor를 빠른 시작대로 실제 gh 환경에서 실행해.
다음 코드는 계획 C: 실제 Herdr 위치와 read-only 상태를 기존 GitHub 화면에 연결하는 작은 변경이다.
설치된 Herdr 도움말·응답과 현재 UI를 읽고 Sol medium이 작은 범위를 정해 Luna high에 구현을 배정해.
고정 변경을 fresh Sol medium이 리뷰하고 필요한 focused 검사 후 마지막 통합 make check만 한 번 해.

새 scheduler·runtime adapter·Go Publisher는 만들지 마.
승인된 통상 작업을 반복 확인하지 말고, 제품 코드 PR의 main 병합은 사람에게 남겨.
사용자 호스트의 중단된 Codex 실험과 dirty 파일은 보존해.
구현·검증된 것과 live 환경에서 확인하지 못한 것을 구분해 보고해.
~~~
