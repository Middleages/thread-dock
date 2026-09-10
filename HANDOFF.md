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

이번 변경은 지침·Skill·템플릿 검사 전환과 브라우저 Monitor의 GitHub 직접 조회, 명시적 Herdr 연결 조회다.
빠른 시작의 npm run dev로 기존 화면을 연다. gh 인증은 실행하는 호스트의 인증을 사용한다.
Herdr pane 안에서 THREADDOCK_SESSIONS_FILE을 지정하면 등록한 세션의 상태와 재개 안내를 읽는다.
실제 설치 버전·인증된 GitHub·사용자 WSL의 실행은 완료로 주장하지 않는다.
Wails 앱은 이전 로컬 Work 조회 구현을 유지한다. 브라우저 모드와 혼동하지 않는다.

## 이번 변경 검증

- Skill 구조 검사와 문서 링크·JSON 예시·coordinator TOML 검사 통과. 문서/Skill 독립 리뷰 ACCEPT.
- GitHub 첫 변경: Node 조회/서버 검사 8개, 화면/연결 검사 15개, 프런트엔드 build 통과. 코드 독립 리뷰 ACCEPT.
- Herdr 최종 수정: adapter focused 11개, 화면 focused 16개 통과. 최종 코드에서 TypeScript/Vite build 통과. 고정 변경 독립 리뷰 ACCEPT.
- 이전 후보에서 Node 전체 조회/연결 20개와 Vitest 검증 통과. 최종 수정에서는 영향받는 adapter/화면 검사만 수행했다.
- 실제 Vite 서버와 fixture gh executable로 화면 파일·API 응답, 두 저장소의 Issue 구분, 정확한 PR 연결, Projects 우선순위/대기 필드 확인.
- Herdr 추가 후보를 실제 Vite 서버 + fixture gh/herdr로 실행해 GitHub 업무와 Herdr 연결을 동시에 반환하고 비공개 Agent 필드를 제외하는 것을 확인.
- 위 결과는 live GitHub/Herdr 성공이 아니다. 원격 브라우저의 localhost 차단으로 육안 확인, 사용자 WSL/Windows 실행은 미확인.
- 현재 작업 환경에 Go가 없어 저장소 전체 make check와 Go 템플릿 테스트는 실행하지 못했다. 전체 gate 통과로 표시하지 않는다.

GitHub에서 확인한 ThreadDock 저장소의 `has_wiki`는 false다(2026-09-10).
Wiki 링크 제공이 Wiki 활성화나 문서 반영을 뜻하지 않는다. 실제 저장소에서 활성화 가능 여부를 확인하고,
그전에는 운영 문서를 PR에 남기며 Wiki 미반영을 표시한다.

## 다음 작업: 실제 환경에서 사용하기

[Draft PR #69](https://github.com/Middleages/thread-dock/pull/69)의 최신 head와 실제 main을 확인한다.
사용자 작업을 보존하는 독립 worktree에서 PR branch를 받아 빠른 시작대로 실행한다.
Herdr pane 안에서 설치된 `herdr --version`, `herdr --skill`, `herdr agent --help`와 실제 agent list 응답 형태를 확인한다.
현재 코드는 v0.8.2 API 기준이다. 실제 위치로 연결 파일을 만들고 gh 인증 및 조회 권한을 확인한다.
GitHub 업무 선택 → 연결된 Agent 상태 확인 → handoff 복사 → 해당 Herdr 세션으로 돌아가는 흐름을 확인한다.
중앙에서 독립 기능 두 개를 나눠 각각 구현·독립 리뷰·PR로 전달하는 사용 검증은 아직 남아 있다.
차이가 발견되면 그 부분만 수정하고 마지막 통합 시 Go를 사용할 수 있는 환경에서 make check를 한 번 수행한다.
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

작업 위치는 /home/appuser/dev_system이며 실제 Git 상태와 Draft PR #69의 최신 head부터 확인해.
dirty 파일을 보존하고 PR branch를 독립 worktree에 받아. main에 먼저 병합하지 마.
이미 구현한 브라우저 Monitor를 빠른 시작대로 실제 gh·Herdr 환경에서 실행해.
설치된 Herdr의 skill·도움말·agent list 응답과 v0.8.2 기준 코드를 비교해.
실제 Issue와 세션 위치로 THREADDOCK_SESSIONS_FILE을 만들고,
업무 선택·로컬 상태·handoff 복사·기존 세션 복귀를 확인해. HERDR_ENV를 임의 설정하지 마.
중앙 관제가 독립 기능 둘을 분배해 각각 구현·리뷰·PR로 이어지는 사용 흐름을 확인해.
이미 구현한 조회 코드를 다시 만들지 말고 실제 드러난 차이만 Sol medium이 작게 정리해 Luna high에 배정해.
코드 수정이 있으면 고정 변경을 fresh Sol medium이 리뷰하고 필요한 focused 검사를 해.
마지막 통합 make check는 Go가 있는 이 환경에서 한 번 실행하고 결과를 PR에 남겨.

새 scheduler·runtime adapter·Go Publisher는 만들지 마.
승인된 통상 작업을 반복 확인하지 말고, 제품 코드 PR의 main 병합은 사람에게 남겨.
사용자 호스트의 중단된 Codex 실험과 dirty 파일은 보존해.
구현·검증된 것과 live 환경에서 확인하지 못한 것을 구분해 보고해.
~~~
