# GitHub와 Herdr로 바로 시작하기

기능 개발은 기존 Agent의 Skill로 시작하고, 업무 현황은 Go/Wails 데스크톱 Monitor에서 확인한다.
현재 PR #69의 화면 조회는 `GetMonitorSnapshot` Wails binding을 사용하며 Node HTTP 서버를 실행하지 않는다.
필요한 것은 대상 Git 저장소, GitHub에 접근 가능한 gh 또는 MCP, 설치된 Herdr, Codex/OpenCode다.
새 agentctl 계약이나 Work 등록은 필요하지 않다.

## 1. Skill을 읽히기

ThreadDock을 받은 위치의 project-template/.agents/skills 아래 다섯 Skill을 사용한다.
처음에는 Agent에게 절대 경로의 SKILL.md를 읽으라고 지정해도 된다.
반복 사용할 저장소에는 해당 다섯 폴더를 .agents/skills 아래 복사한다.
동명 Skill이 있으면 기존 사용자 수정을 확인하고 이번 흐름에 필요한 내용만 반영한다.
루트 AGENTS.md, 모델 설정, CI와 다른 프로젝트 문서를 통째로 덮어쓰지 않는다.

Codex/OpenCode 버전에 따라 자동 Skill 탐색 경로가 다르면 실제 설치 도움말을 따른다.
자동 탐색이 안 돼도 SKILL.md 경로를 직접 읽혀 시작할 수 있다.
이 저장소의 .codex 역할 모델은 ThreadDock 개발용이며 다른 프로젝트에 자동 설치하지 않는다.

## 2. 중앙 관제 세션에서 시작하기

아래 요청을 Herdr pane 안에서 대상 저장소를 연 Agent에 전달한다.
설치된 herdr --skill의 실행 조건을 따른다. HERDR_ENV=1은 Herdr가 설정하는 값이며 임의로 설정해 우회하지 않는다.
Herdr 밖에서 시작했다면 먼저 실제 Herdr pane에서 Agent를 연다.
Skill 경로는 실제 ThreadDock checkout 위치로 바꾼다.

~~~text
ThreadDock의 plan-work, open-agent-session, record-work Skill을 읽고 이 프로젝트의 중앙 관제를 맡아줘.
기존 GitHub Issue·PR·Projects와 저장소 지침을 먼저 확인해.
새 로컬 Work나 실행 계약을 만들지 말고 GitHub Issue를 업무 기준으로 사용해.

내가 요청한 기능의 범위·완료 조건을 정리하고, 공유 변경과 의존성을 먼저 구분해.
승인된 범위에서 독립 기능을 별도 Herdr 세션과 작성 worktree로 배정해.
각 기능 리더가 native subagent로 상세계획·구현·독립 리뷰·커밋·PR 준비를 맡게 해.
나는 중앙에서 주요 결정·막힌 점·PR 결과를 보고 싶어.

Issue에는 결정과 handoff, Projects에는 상태·우선순위,
설계·PR에는 구현과 검증 근거, Wiki에는 반영된 사용법과 운영 지식을 남겨.
이미 승인한 통상 작업은 반복 확인하지 말고 진행해.
main 병합은 내가 할게.
~~~

기존 연결에서 저장소/보드를 확인할 수 없는 경우에만 필요한 대상 정보를 확인한다.
Projects 접근이 안 되면 문제를 보고하고 Issue·PR 작업은 계속할 수 있다.
이 fallback을 Projects 연동 완료로 표시하지 않는다.

## 3. 기능 세션으로 전달할 최소 내용

~~~text
기능 Issue: 실제 GitHub URL
작업 저장소와 branch/worktree: 실제 위치
목적과 완료 조건: Issue 참조
허용 범위·공유 변경 주의점·선행 기능: 해당 내용
역할: 기능 리더. plan-work, implement-task, review-change, record-work Skill 사용
독립 reviewer: review-change Skill을 읽고 지정한 commit SHA와 diff를 검토
결과: 변경 요약, 검사 명령/결과, 리뷰한 SHA, PR URL, 남은 일
~~~

진행 중인 작업은 먼저 기존 세션에 연결한다.
Herdr 생성·입력은 open-agent-session Skill과 설치된 herdr --skill/도움말을 따른다.
내부 subagent가 별도 pane에 보이지 않아도 정상이다.
두 기능이 같은 파일을 수정해야 하면 중앙이 그 부분만 직렬화한다.

## 4. 세션 위치를 선택적으로 보관하기

.threaddock/sessions.json은 로컬 연결 메모이며 Git에 커밋하지 않는다.
필요하면 `git rev-parse --git-path info/exclude`로 확인한 파일에 /.threaddock/을 추가한다.
기능 리더가 동시 갱신하지 않고 중앙 관제 한 곳에서 연결 메모를 관리한다.
Go/Wails 모니터의 연결 파일 설정에 선택 WSL 배포판에서의 절대 경로를 지정하는 것으로 설계한다.
Go 설정의 실제 전달 방식과 사용 명령은 구현 후 실행 검증 결과로 확정한다.

~~~json
{
  "version": 1,
  "bindings": [
    {
      "issueUrl": "https://github.com/OWNER/REPO/issues/NUMBER",
      "repository": "github.com/OWNER/REPO",
      "worktree": "/absolute/feature-worktree",
      "session": "actual-session-name",
      "workspaceId": "actual-workspace-id",
      "tabId": "actual-tab-id",
      "paneId": "actual-pane-id",
      "agentName": "actual-agent-name",
      "role": "feature"
    }
  ]
}
~~~

위 값은 형식 예시이며 실제 응답에서 확인한 값으로만 기록한다.
중앙 관제 연결은 role=coordinator와 repository 또는 projectUrl을 사용하고 issueUrl을 생략할 수 있다.
하나의 Issue에 구현/검토 등 여러 연결을 남길 수 있다. 여러 Agent가 있다는 이유로 충돌 처리하지 않는다.
로컬 경로·세션 ID를 공개 Issue에 복사할 필요는 없다.

연결 값을 얻을 때는 실제 Herdr pane 안에서 설치된 도움말을 읽고 대상 세션을 조회한다.
Herdr 관찰 형식은 설치된 Herdr 0.8.2의 `herdr --session NAME agent list` 응답을 기준으로 한다.
여기서 확인한 name, workspace_id, tab_id, pane_id와 실제 worktree를 기록한다.
UI 표시명이나 탭 순서로 ID를 추측하지 않는다.

파일에 적힌 세션만 조회하므로 여러 세션을 보려면 각 세션의 연결을 함께 적는다.
각 지정 세션에서 연결되지 않은 Agent도 목록으로 볼 수 있다.
Go 이식에서도 파일 변경을 다음 갱신에 반영하고 파일 자체는 Monitor가 수정하지 않는다.
정확한 pane과 이름이 있어도 cwd를 확인할 수 없거나 worktree 밖이면 기능 연결을 확정하지 않는다.
Git remote를 자동 검증한 결과가 아니라, 사용자가 지정한 연결과 관찰한 위치를 대조한 결과다.

## 5. 재개와 기록

Issue의 최근 handoff에는 다음만 남긴다.

- 검증된 완료 내용과 관련 PR/commit
- 중요한 결정과 이유
- 남은 일·blocker·다음 행동
- 업데이트 시각과 관련 설계·Wiki 링크

세션이 살아 있으면 돌아가고, 없어졌으면 보존된 Git 변경과 handoff로 새 세션을 연다.
완료 내용은 참고 자료로 제공하고 재구현하지 않는다.
입력 실패를 만나면 실제 화면을 확인한다. prompt를 보냈다는 사실만으로 전달 성공을 보고하지 않는다.
Wiki 갱신 권한이나 초기화가 막혔으면 문서 초안을 PR에 포함하고 Wiki 미반영을 handoff에 남긴다.

## Go/Wails Monitor 구현 후 실행

제품은 Windows Go/Wails 앱이다. 기존 React/Vite는 앱 화면을 개발·빌드하는 데 사용한다.
별도의 npm run dev 조회 서버나 브라우저 전용 API를 제품 실행 조건으로 두지 않는다.
Go 백엔드는 선택한 WSL 배포판의 gh 인증과 Herdr 읽기 접근을 사용한다.
ChatGPT GitHub 연결과 사용자 WSL의 gh 인증은 별개다.

개발·배포는 설치된 Wails의 `wails dev`, `wails build` 경로를 사용한다. Windows build/app 실행과
Windows→WSL 접근은 Linux 검증과 별도의 증거로 기록한다. 현재 root-owned Windows toolchain은
user-local Go `go1.27.0 windows/amd64`, Node `26.8.1`, Wails CLI/runtime `v2.15.0`이며,
공식 Go/Node checksum은 제공된 범위에서 일치했다.

정확한 commit `66158d9`는 Windows Go의 UNC `RLock` 실패 때문에 NTFS staging 위치로 export했다.
matching CLI/runtime의 `wails build`는 27.436초 만에 exit 0으로 완료됐고
`monitor\build\bin\ThreadDockMonitor.exe`를 생성했으며 bindings/frontend/assets/app stages가
모두 `Done`이었다. 이는 표준 package build 증거다.

Windows→WSL 명령이 기존 Herdr pane 환경을 상속한다고 가정하지 않는다.
설치된 CLI의 읽기 접근 조건을 확인하고 HERDR_ENV를 임의로 설정하지 않는다.
접근이 안 되면 Herdr 조회 불가 이유를 표시하고 GitHub 관찰은 유지한다.
실제 앱과 정상 세션에서 확인하기 전에는 live 통합 완료로 표시하지 않는다.

Go/Wails Monitor의 Herdr 관찰은 `THREADDOCK_SESSIONS_FILE`에 지정한 WSL 내부 절대 경로의
version 1 연결 파일과 `THREADDOCK_WSL_DISTRIBUTION`을 읽기 전용으로 사용한다. 연결 파일은
WSL의 `cat`으로 읽고, worktree와 Agent cwd는 같은 배포판의 `realpath` 결과로 대조한다.
Herdr 실행은 설치된 `$HOME/.local/bin/herdr`를 고정된 `/bin/sh -c` 스크립트와 positional
인자로 호출하며, 세션·workspace·tab·pane·Agent 이름이 모두 맞아야 연결됨으로 표시한다.
기능 worktree는 canonical cwd가 그 아래일 때만 연결하고, `role=coordinator`는 worktree를
생략할 수 있다. 캐시된 빈 목록은 조회 실패 뒤 `cached`로 남기며 현재 `missing`으로 단정하지 않는다.

검증 근거의 범위를 구분한다.

- Linux Go fixture: `go test ./monitor -run 'Test(GitHub|Commands|App|Herdr|Snapshot)'` 통과 기록.
- Linux UI/build: `npm exec vitest run src/bindings.test.ts src/monitor.test.tsx --reporter=verbose` (2 files, 20 tests)와 `npm run build` 통과.
- Linux 통합: reviewed dependency SHA `66158d9`에서 `make check`가 shell syntax, `go vet ./...`, 전체 Go 테스트, UI 2 files/20 tests, frontend build까지 통과.
- managed-pane live(상위에서 전달된 외부 관찰, 원본 transcript 없음): 실제 Windows→WSL workstation에서 `gh auth status` 성공, bare `wsl.exe --exec herdr` PATH lookup 실패, absolute `/home/appuser/.local/bin/herdr` status와 agent list 성공. 이는 fixture/UI와 별도이며 Windows→WSL Monitor 읽기 경로 검증이 아니다.
- Windows packaged live: scheduled-task/WSL interop launch에서 `UtilAcceptVsock:281: accept4 failed 110`이 반복됐고 유효한 packaged GitHub/Herdr screenshot이 없어 live acceptance는 미완료다. 이전 `aeca770`의 plain `go build` binary가 GitHub 69개 항목과 Herdr 기본 session/4 agents를 렌더링한 관찰은 최종 표준 package 증거가 아니다.
- Cleanup: `ThreadDockValidation66158d9` scheduled task와 관련 process cleanup은 사용자 Windows computer-use에 요청했으며 별도 확인 전까지 pending이다. component-level CLI 관찰 자체는 외부 관찰 근거로 기록하되 packaged live 성공이나 cleanup 완료로 표시하지 않는다.

## 실제 사용 확인

작은 독립 기능 두 개로 시작한다.
각 기능에서 구현·독립 리뷰·검사·PR까지 끝내고 Projects 상태와 Issue 결정 기록을 확인한다.
사람 병합 후 필요한 Wiki를 갱신하고, 프로젝트를 전환했다 돌아와 다음 행동을 찾는다.
Herdr 오류가 있다면 정상 세션 수동 연결로 진행 가능한 부분과 막힌 부분을 구분한다.
별도 장애 인증 프로젝트나 새 복구 엔진을 만들지 않는다.

CLI 읽기 참고: [Issue 조회](https://cli.github.com/manual/gh_issue_view),
[PR 목록](https://cli.github.com/manual/gh_pr_list),
[Projects 항목 조회](https://cli.github.com/manual/gh_project_item-list).
Herdr 응답 기준: [0.8.2 Agent 정보](https://github.com/herdrdev/herdr/blob/v0.8.2/src/api/schema/agents.rs),
[세션 선택](https://github.com/herdrdev/herdr/blob/v0.8.2/src/session.rs).
