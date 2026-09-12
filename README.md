# ThreadDock

**Go/Wails 데스크톱 Monitor + 프로젝트 Agent/Skill**로 GitHub 중심 멀티 세션 개발을 지원합니다.

고정 제품 경계는 단순합니다.

- GitHub Issue / PR / Projects: durable 업무 원본
- Herdr: top-level session, pane, worktree, Agent lifecycle
- native subagent: feature 내부 상세계획·구현·독립 리뷰
- ThreadDock: GitHub 업무와 top-level Herdr 상태를 한 화면에 연결

ThreadDock이 scheduler, lease/recovery engine, 자체 task DB, 별도 execution runtime을 소유하지 않습니다.

## 옛 실행 엔진에서 전환

`agentctl` CLI와 옛 pilot 실행 경로는 제거됐습니다. 현재 사용은 아래 Monitor와 Agent/Skill 흐름을 따릅니다.
옛 엔진 소스 정리의 범위·검증은 [완료 계획](docs/superpowers/plans/2026-09-12-engine-retirement-completion.md)과
[dependency inventory](docs/operator/2026-09-12-engine-dependency-inventory.md)에서 확인할 수 있습니다.
이 변경은 기존 runtime snapshot·event log·설정 데이터·Git worktree·Herdr session을 자동 삭제하거나 이관하지 않습니다.
과거 실행기를 조사해야 할 때는 Git 이력을 참고하고, 역사 runbook의 명령을 현재 제품 실행 방법으로 사용하지 마세요.

## Agent 구조

프로젝트에서 기억할 top-level Agent는 두 개뿐입니다.

```text
Coordinator -> Herdr Feature Leader -> transient planner/implementer/reviewer -> PR/handoff
```

- `td_coordinator`: 프로젝트의 열린 GitHub 업무를 feature로 나누고 Herdr Feature Leader session을 배정하며 전역 locator를 관리합니다.
- `td_feature_leader`: 하나의 Issue/feature를 상세계획부터 reviewed PR readiness까지 책임집니다.

Planner, implementer, reviewer, PR/Wiki/Project helper는 별도 상주 Agent가 아니라 필요할 때만 사용하는 native subagent입니다. ThreadDock Monitor와 `sessions.json`에는 Coordinator와 Feature Leader만 top-level 연결로 기록합니다.

## Canonical Skills

반복 사용할 프로젝트에는 `project-template/.agents/skills`를 대상 저장소의 `.agents/skills`에 복사합니다.

- `coordinate-work`
- `develop-feature`
- `grill-plan`
- `tdd-task`
- `review-change`
- `publish-work`

기존 `plan-work`, `open-agent-session`, `implement-task`, `record-work`는 예전 프롬프트 호환용 migration shim입니다.

Codex에서는 `project-template/.codex/agents`의 두 Agent 정의도 대상 `.codex/agents`에 복사합니다. 대상 `.codex/config.toml`의 기존 설정을 덮어쓰지 말고 필요하면 `[agents]` 활성화 stanza만 병합합니다.

자세한 시작 방법은 [GitHub와 Herdr로 바로 시작하기](docs/operator/github-first-quickstart.md)를 참고하세요.

## 전역 Herdr locator

여러 프로젝트를 동시에 써도 locator 파일은 WSL 사용자 기준 하나만 사용합니다.

```text
~/.threaddock/sessions.json
```

예:

```json
{
  "version": 1,
  "bindings": []
}
```

각 프로젝트 Coordinator는 user-level lock 아래에서 전체 파일을 다시 읽고 자기 exact binding만 upsert/cleanup합니다. 다른 repository/project binding을 추측하거나 bulk prune하지 않습니다.

Feature binding은 다음 세 조건이 모두 확인될 때만 제거합니다.

1. Issue closed
2. 관련 PR 작업 finished
3. Herdr top-level Feature Leader session absent

Agent의 `idle`/`done`만으로 GitHub 업무 완료나 locator 삭제를 판단하지 않습니다.

## Monitor 연결 설정

설정 UI에서 GitHub Host, 저장소, GitHub Projects, WSL 배포판과 Herdr 연결 파일을 저장할 수 있습니다.

GitHub Enterprise Server 예:

```text
GitHub Host: github.samsungds.net
GitHub 저장소: FDYPhotoDX/thread-dock
GitHub Projects: https://github.samsungds.net/orgs/FDYPhotoDX/projects/4
WSL 배포판: Ubuntu
Herdr 연결 파일: /home/appuser/.threaddock/sessions.json
```

GitHub Host는 `https://` 없이 입력합니다. Project URL은 `/views/N`이 아닌 Project root URL을 사용합니다. 선택한 WSL의 `gh`도 같은 host에 인증되어 있어야 합니다.

## Monitor UI

Windows Go/Wails Monitor는 기본적으로 열린 Issue/PR만 조회합니다. 닫힌 이력은 사용자가 `전체 보기`를 선택했을 때만 불러오며 전체 이력 모드에서는 4초 자동 polling을 반복하지 않습니다.

고정 좌우 sidebar 대신 상단 bar에 연결 상태·Toolbox·설정을 모으고, 최대 1440px의 본문에 프로젝트 목록과 상세를 이어 표시합니다. 선택 업무의 식별자, `작업 정보 복사`, 안전한 `GitHub에서 보기` 링크는 프로젝트 상세에 있습니다.

프로젝트 표는 실제 workflow 단계를 추측하지 않고 `조회 상태`를 보여줍니다. Issue/PR 상세는 실제 GitHub 상태와 Project field, 연결된 Herdr 위치/Agent 상태를 표시합니다. 긴 Issue 제목은 세로 테이블에서 한 줄로 표시하고, Issue/PR 본문의 일반적인 Markdown을 읽을 수 있게 렌더링합니다.

`작업 정보 복사`는 선택한 작업의 GitHub 근거와 연결된 Herdr handoff 정보를 클립보드에 복사합니다.

Herdr 실행 상태는 기본 한 줄 요약입니다. `자세히 / 접기`로 위치·관찰 세션·안내를 확인하며, blocked/offline/stale/unverified 같은 주의 상태는 접힌 상태에서도 표시합니다. GitHub 업무 상태와 Herdr 관찰 시각은 구분합니다.

## 프로젝트 자료와 전역 Toolbox

프로젝트 상세는 `업무 / 자료`로 나뉩니다. `자료`에는 해당 프로젝트의 HTTP(S) 링크, Windows absolute file path, WSL absolute file path만 저장합니다. 자료 열기·경로 복사·추가·삭제를 지원하며 상대경로나 실행 URL은 허용하지 않습니다.

상단 `Toolbox`는 본문 폭을 줄이지 않는 우측 overlay Drawer입니다. 닫기 버튼·배경·Escape로 닫을 수 있습니다.

- `명령어`: 프로젝트와 무관하게 이름·내용·선택 메모를 저장하고 복사합니다. 실행 기능은 없습니다.
- `할 일`: 공통 또는 프로젝트 하나에 연결하는 개인 메모입니다. 전체·공통·프로젝트 필터와 기본 미완료/완료 포함 전환을 제공합니다.
- 현재 목록에서 사라진 프로젝트의 할 일도 원래 key와 함께 보존하며 공통 또는 다른 프로젝트로 재지정할 수 있습니다.
- 프로젝트 상세의 미완료 할 일 개수와 `보기`는 같은 전역 Drawer의 해당 프로젝트 필터로 연결됩니다. GitHub 업무 상태와는 별개입니다.

Toolbox의 명령어에는 실행 버튼이 없고 **복사만** 제공합니다. 로컬 파일도 사용자가 직접 등록한 absolute path만 열 수 있습니다.

Toolbox 데이터는 Monitor 설정이나 Herdr locator와 분리해 사용자 설정 디렉터리의 다음 파일에 저장합니다.

```text
%APPDATA%\ThreadDock\projects.json  # v2: 프로젝트별 자료
%APPDATA%\ThreadDock\toolbox.json   # v1: 전역 명령어와 할 일
```

이전 `projects.json` v1의 명령어·체크리스트는 처음 새 Toolbox 경로에 접근할 때 전역 파일로 옮깁니다. 전역 저장이 성공한 뒤 프로젝트 파일을 자료 전용 v2로 바꾸며, 중간 실패 시 재시도해 중복을 만들지 않습니다. 잘못된 파일·지원하지 않는 버전·항목 한도 초과는 오류로 남기고 원본을 조용히 덮거나 잘라내지 않습니다. SQLite는 사용하지 않습니다.

Toolbox 내용은 `td_coordinator`, `td_feature_leader`, canonical Skill에 자동 주입하지 않습니다. Agent가 참고해야 하는 자료는 사용자가 필요할 때 명시적으로 전달합니다.

ThreadDock 자체 dogfooding 기준은 [사용성 체크리스트](docs/usability-checklist.md)에 유지합니다.

## 검증

정적 project-template 확인:

```bash
make template-check
```

저장소 전체 gate:

```bash
make check
```

Windows package:

```powershell
cd monitor
wails build
.\build\bin\ThreadDockMonitor.exe
```

기존 final reviewed SHA `325db89`에서는 Go/Wails package와 healthy native run이 검증된 이력이 있습니다. 현재 PR/브랜치의 새 UI와 Agent/Skill 템플릿은 해당 과거 결과로 자동 승격하지 않으며, merge 전에 현재 SHA에서 `make check`, Windows `wails build`, 실제 GHES + WSL + Herdr smoke를 별도로 확인합니다.

## 문서

- [운영 Quickstart](docs/operator/github-first-quickstart.md)
- [사용성 체크리스트](docs/usability-checklist.md)
- [Agent orchestration 설계](docs/superpowers/specs/2026-09-11-thread-dock-agent-orchestration-design.md)
- [Agent orchestration 구현계획](docs/superpowers/plans/2026-09-11-thread-dock-agent-orchestration.md)
- [제품 역할](PRODUCT.md)
- [용어와 책임](CONTEXT.md)
- [현재 상태](HANDOFF.md)
