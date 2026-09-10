# GitHub와 Herdr로 바로 시작하기

기능 개발은 기존 Agent의 Skill로 시작하고, 업무 현황은 아래 브라우저 Monitor로 확인한다.
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
브라우저 Monitor에 THREADDOCK_SESSIONS_FILE로 이 파일의 절대 경로를 지정한다.
기존 Wails 앱에는 이 연결 파일을 적용하지 않는다.

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
현재 adapter는 Herdr 0.8.2의 `herdr --session NAME agent list` 응답 형식을 기준으로 한다.
여기서 확인한 name, workspace_id, tab_id, pane_id와 실제 worktree를 기록한다.
UI 표시명이나 탭 순서로 ID를 추측하지 않는다.

파일에 적힌 세션만 조회하므로 여러 세션을 보려면 각 세션의 연결을 함께 적는다.
각 지정 세션에서 연결되지 않은 Agent도 목록으로 볼 수 있다.
파일을 수정하면 다음 갱신에 반영된다. 파일 자체는 Monitor가 수정하지 않는다.
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

## 브라우저 Monitor 실행

ThreadDock checkout에서 Node.js 22.12 이상과 인증된 gh를 사용한다.
Windows 사용자는 GitHub 인증이 있는 WSL 터미널에서 실행하고 Windows 브라우저로 연다.
ChatGPT의 GitHub 연결과 사용자 PC의 gh 인증은 별개다.

~~~bash
gh auth status
cd monitor/frontend
npm ci
THREADDOCK_REPOS=Middleages/thread-dock npm run dev
~~~

터미널에 나온 로컬 주소를 연다. 기본 주소는 http://127.0.0.1:5173이다.
다른 저장소도 함께 보려면 THREADDOCK_REPOS에 OWNER/REPO를 쉼표로 구분한다.
Projects 보드는 THREADDOCK_PROJECTS에 실제 보드 URL을 쉼표로 구분해 선택적으로 연결한다.

~~~bash
THREADDOCK_REPOS=OWNER/APP,OWNER/API \
THREADDOCK_PROJECTS=https://github.com/users/OWNER/projects/1,https://github.com/orgs/ORG/projects/2 \
npm run dev
~~~

조직 보드는 https://github.com/orgs/OWNER/projects/1 형식이다. 예시의 OWNER와 번호는 실제 값으로 바꾼다.
설정을 바꿨으면 개발 서버를 다시 시작한다. GitHub 조회는 60초 캐시를 사용한다.
저장소별 Issue·PR과 보드 항목 조회는 각각 최대 100개이며 화면에 제한을 표시한다.
일부 조회가 실패하면 해당 원본의 마지막 성공 결과와 시각을 보존한다.
화면에서 Issue·PR·Wiki를 열거나 선택한 업무의 handoff를 복사할 수 있다.
Wiki의 실제 존재·내용과 Issue 댓글 전체를 자동 동기화하는 범위는 아니다.

이 브라우저 모드는 npm run dev로 제공된다. 정적 build 파일만 열면 gh 조회 서버가 없으므로 동작하지 않는다.
기존 Wails 데스크톱 앱은 이전 로컬 Work 경로를 유지한다.
Herdr 상태도 함께 보려면 실제 Herdr pane의 터미널에서 다음처럼 실행한다.

~~~bash
THREADDOCK_REPOS=Middleages/thread-dock \
THREADDOCK_SESSIONS_FILE=/absolute/path/to/.threaddock/sessions.json \
npm run dev
~~~

HERDR_ENV는 Herdr가 설정하는 값이다. 이 값을 직접 설정해 실행 조건을 우회하지 않는다.
파일 경로를 설정하지 않았거나 Herdr 밖에서 실행했다면 이유와 설정 안내를 표시하고 GitHub 조회는 계속한다.
Herdr 조회는 세션별 5초 캐시를 사용한다. 실패 시 마지막 성공 시각을 보존하며 GitHub 시각과 따로 표시한다.
대상이 없어진 상태, 위치 불일치, 조회 실패, 입력 대기와 실행 중을 구분한다.
선택한 업무의 handoff를 복사하면 연결 위치와 확인 시각·다음 행동도 포함된다.
화면에서는 시작·입력·종료 명령을 실행하지 않는다. 재개는 위 Skill이 최신 기록과 실제 화면을 확인한 뒤 수행한다.

구현·fixture 검증과 사용자 PC의 live 성공은 별개다. 실제 설치 버전의 도움말/응답과 정상 세션을 확인하고 사용한다.

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
