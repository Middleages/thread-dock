# Compact Monitor and Global Toolbox Design

## Goal

ThreadDock Monitor의 고정 좌우 사이드바를 제거해 GitHub 업무와 Issue/PR 본문에 더 넓은 공간을 주고, 사람용 Toolbox를 `프로젝트별 자료`와 `전역 명령어/Todo`로 분리한다. Todo는 전역 목록이지만 필요하면 프로젝트 하나에 연결해 프로젝트별 할 일처럼 필터링해서 사용할 수 있다.

## Product boundary

이 변경은 Monitor UI와 사람용 로컬 Toolbox만 다룬다.

- GitHub Issue/PR/Projects는 업무 원본이다.
- Herdr는 top-level 실행 상태를 제공한다.
- Agent/Skill orchestration은 현재 구조를 유지한다.
- Toolbox 내용은 Agent에 자동 주입하거나 자동 참조하지 않는다.
- 저장된 command는 실행하지 않고 복사만 제공한다.
- Todo는 개인 작업 메모이며 GitHub Issue/Project 상태를 대체하지 않는다.

## 1. Layout

현재 데스크톱의 `190px / main / 300px` 3열 고정 구조를 없애고 단일 본문 중심 구조로 바꾼다.

```text
┌────────────────────────────────────────────────────────────────────┐
│ ThreadDock   ● 연결 정상                  [Toolbox] [설정]        │
├────────────────────────────────────────────────────────────────────┤
│ 작업 현황                         [열린 항목|전체] [새로고침]    │
│ 최근 활동   프로젝트                         조회 상태             │
│ ...                                                                │
├────────────────────────────────────────────────────────────────────┤
│ JMJ                                        Issue #123              │
│ 최근 동기화 ...          [작업 정보 복사] [GitHub에서 보기]      │
│ [업무] [자료]                                                     │
│ ...                                                                │
│ 할 일 3개 [보기]                                                   │
│ 실행 상태 ● 연결됨 · feature-123 · 작업 중          [자세히]     │
└────────────────────────────────────────────────────────────────────┘
```

### Top bar

상단 한 줄에 다음만 둔다.

- ThreadDock brand
- 로컬 연결 상태
- 전역 `Toolbox` 버튼
- `설정` 버튼

기존 왼쪽 sidebar의 brand/connection과 우측 하단 floating settings launcher를 이 상단 bar로 통합한다.

### Main content

본문은 전체 가용 폭을 사용하되 너무 넓은 문단을 막기 위해 내부 최대 폭을 둔다. 프로젝트 목록과 선택 프로젝트 상세는 같은 세로 흐름에 둔다.

## 2. Selected work actions

기존 우측 action rail은 제거한다.

다음 정보는 선택 프로젝트 상세 header로 이동한다.

- `Issue #N` / `PR #N` 식별자
- `작업 정보 복사`
- `GitHub에서 보기`

연결 오류나 stale 상태는 기존 상단 degraded banner를 사용한다. 우측 rail을 유지하기 위해 같은 정보를 반복하지 않는다.

## 3. Project tabs

프로젝트 내부 탭은 다음 두 개만 둔다.

- `업무`
- `자료`

현재 `Toolbox` 탭은 `자료`로 이름을 바꾼다. 프로젝트별 자료에는 다음만 저장한다.

- HTTP(S) 링크 (Confluence 등)
- Windows absolute file path
- WSL absolute file path

프로젝트 자료는 기존 `%APPDATA%\ThreadDock\projects.json`을 사용한다.

프로젝트별 Todo를 별도 데이터로 중복 저장하지 않는다. 프로젝트 상세에는 해당 프로젝트에 연결된 미완료 Todo 개수와 `보기` 진입점만 노출할 수 있다. `보기`를 누르면 같은 전역 Toolbox Drawer를 해당 프로젝트 필터가 선택된 상태로 연다.

## 4. Global Toolbox drawer

Top bar의 `Toolbox` 버튼을 누르면 오른쪽 overlay drawer를 연다. Drawer는 본문 폭을 영구적으로 줄이지 않는다.

권장 폭:

- desktop: 420~480px
- narrow viewport: `min(92vw, 480px)`

Drawer 내부 탭:

- `명령어`
- `할 일`

Drawer는 backdrop 클릭, 닫기 버튼, `Escape`로 닫을 수 있다. 열려 있는 동안 별도 full page navigation은 하지 않는다.

### Commands

전역 command 항목:

```text
id
label
command
note
```

예:

```text
PostgreSQL 접속
psql -h localhost -U postgres -d app
[복사]
```

`실행` 기능은 추가하지 않는다.

### Todo

Todo는 전역 목록에 저장하며 선택적으로 프로젝트 하나에 연결할 수 있다.

```text
id
text
done
projectKey?   # null이면 공통, 값이 있으면 프로젝트 하나에 연결
```

예:

```json
{
  "id": "todo-123",
  "text": "staging DB migration 확인",
  "done": false,
  "projectKey": "github.samsungds.net/FDYPhotoDX/jmj"
}
```

프로젝트와 무관한 항목은 `projectKey = null`로 저장한다.

Todo 하나를 여러 프로젝트에 동시에 연결하는 기능은 넣지 않는다. 필요 시 동일 문구를 프로젝트별로 별도 Todo로 만든다.

등록 UI에는 선택적 프로젝트 selector를 둔다.

```text
할 일: [________________________]
프로젝트: [공통 ▾]
           JMJ
           PIECE
           LONA
[추가]
```

Todo 목록에는 다음 필터를 제공한다.

- `전체`
- `공통`
- 등록된 각 프로젝트
- `미완료` 기본 / `완료 포함` toggle

예:

```text
[전체] [공통] [JMJ] [PIECE] [LONA]      [완료 포함]

☐ GitHub 토큰 갱신                         공통
☐ staging DB migration 확인                JMJ
☐ 업무목록 필터 확인                       JMJ
☐ 배포 전 로그 확인                        PIECE
```

MVP에서는 다음을 추가하지 않는다.

- 여러 프로젝트 동시 연결
- 우선순위
- 마감일
- 반복 일정
- 담당자
- 완료 이력/감사 로그
- 그룹/보드/칸반

## 5. Storage split

사람용 데이터는 다음처럼 분리한다.

```text
%APPDATA%\ThreadDock\projects.json
  -> 프로젝트별 references만

%APPDATA%\ThreadDock\toolbox.json
  -> 전역 commands + todos
```

`config.json`과 Herdr `sessions.json`에는 섞지 않는다.

### JSON vs SQLite

MVP에서는 SQLite를 도입하지 않고 현재 JSON 저장 방식을 유지한다.

이유:

- 자료/command/Todo 데이터량이 작다.
- 현재 write 주체는 로컬 Monitor 한 프로세스다.
- 복잡한 join, full-text search, 정렬/집계, 동시 다중 writer가 필요하지 않다.
- SQLite를 추가하면 schema migration, driver, packaging, DB 복구 정책까지 새 운영 표면이 생긴다.

다만 저장 구현은 UI에서 분리된 store interface로 유지해 나중에 SQLite로 교체할 수 있게 한다. 다음 중 하나가 실제 요구로 생기면 SQLite 전환을 재검토한다.

- Todo/command/reference가 수천~수만 건으로 커짐
- 검색/태그/정렬/통계가 핵심 기능이 됨
- 여러 프로세스/창이 동시에 수정해야 함
- 완료 이력이나 audit history가 필요함
- JSON 전체 rewrite가 성능/안정성 병목이 됨

### Existing data migration

현재 버전의 `projects.json`에는 프로젝트별 `references`, `commands`, `checklist`가 들어갈 수 있다. 업데이트 후 사용자가 저장한 항목이 사라져 보이면 안 된다.

전역 `toolbox.json`이 아직 없을 때 한 번만 migration한다.

1. `projects.json`을 읽는다.
2. 모든 프로젝트의 command를 전역 commands로 모은다.
   - `label + command`가 완전히 같은 항목은 하나만 보존한다.
3. 모든 프로젝트 checklist를 전역 Todo로 옮긴다.
   - 기존 항목은 원래 속했던 프로젝트의 `projectKey`를 유지한다.
   - 같은 프로젝트 안에서 동일 text가 중복되면 하나만 보존한다.
   - 서로 다른 프로젝트에 같은 text가 있어도 프로젝트가 다르면 각각 보존한다.
   - 기존 공통 checklist 데이터가 없다면 임의로 공통으로 승격하지 않는다.
4. 새 `toolbox.json`을 먼저 원자적으로 저장한다.
5. global 저장 성공 후에만 `projects.json`을 references-only schema로 다시 저장한다.
6. 어느 단계라도 실패하면 기존 `projects.json`은 그대로 두고 migration 실패를 UI에 알린다.

Migration은 반복 실행해도 중복이 생기지 않아야 한다.

## 6. Herdr density

Herdr는 기본적으로 한 줄 summary만 보여준다.

```text
실행 상태  ● 연결됨 · feature-123 · 작업 중                  [자세히]
```

Summary에 우선 표시할 정보:

- connection status
- 연결된 top-level session 이름
- Agent status

`자세히`를 펼치면 현재 Herdr panel의 다음 정보를 보여준다.

- workspace/tab/pane/cwd
- 관찰 시각
- 관찰한 sessions
- unconnected agents
- notices

기본 상태는 collapsed다. `blocked`, `offline`, `stale`, `unverified`처럼 사용자 판단이 필요한 상태일 때는 summary 문구로 분명히 드러내되 자동 확장하지 않는다.

## 7. Responsive behavior

- desktop: top bar + full-width main content
- tablet/mobile: top bar actions는 줄바꿈 또는 compact button으로 허용
- Toolbox drawer는 viewport 대부분을 사용
- project table은 기존 small-screen behavior를 유지하거나 더 단순화할 수 있지만 좌우 고정 sidebar는 어떤 breakpoint에서도 다시 만들지 않는다.

## 8. Components

권장 책임 분리:

- `App.tsx`: snapshot selection과 page composition만 담당
- `TopBar.tsx`: brand, connection status, Toolbox/Settings actions
- `ProjectDetail.tsx`: 선택 프로젝트 header, 업무/자료 tabs, work actions, project Todo shortcut
- `HerdrSummary.tsx`: collapsed/expanded Herdr state
- `ProjectReferences.tsx`: project-local references CRUD
- `GlobalToolboxDrawer.tsx`: drawer + command/Todo tabs + project filter
- backend `project toolbox store`: references only
- backend `global toolbox store`: commands/Todo + migration

기존 `ProjectToolbox.tsx`는 기능을 두 책임으로 분리하며 호환용 wrapper를 유지할 필요는 없다.

## 9. Error handling

- Toolbox 저장 실패: 현재 입력을 UI에 남기고 inline error 표시
- migration 실패: 기존 프로젝트 데이터 보존, global toolbox를 빈 값으로 덮어쓰지 않음
- local/WSL file open 실패: 현재처럼 오류 메시지만 표시
- command clipboard 실패: 실행 fallback을 제공하지 않음
- 존재하지 않거나 제거된 projectKey를 가진 Todo는 삭제하지 않고 `알 수 없는 프로젝트`로 표시하며 사용자가 공통/다른 프로젝트로 재지정할 수 있게 한다.
- GitHub/Herdr degraded 상태는 Toolbox 오류와 독립적으로 표시

## 10. Tests

### Go

- global toolbox get/save
- project references store no longer persists new commands/checklist
- Todo의 `projectKey` 0개/1개 저장과 validation
- migration preserves project association for existing checklist items
- migration deduplicates commands and same-project duplicate Todo
- migration does not collapse same-text Todo from different projects
- failed global write leaves old project file intact
- Windows/WSL reference validation remains unchanged

### Frontend

- desktop layout no longer renders permanent left/right sidebars
- top bar exposes connection, Toolbox, Settings
- selected work actions render in project header
- project tabs are `업무 / 자료`
- Toolbox drawer opens/closes and renders `명령어 / 할 일`
- Todo can be created as common or linked to one project
- Todo filter supports all/common/project and incomplete/completed modes
- project Todo shortcut opens the drawer with that project filter
- commands offer copy but no execute action
- Herdr detail is collapsed initially and expands explicitly
- existing open/all polling and Markdown tests remain passing

## 11. Non-goals

이번 변경에서 하지 않는다.

- Toolbox 내용을 Agent prompt에 자동 주입
- command 실행
- shell/terminal embedding
- Todo 여러 프로젝트 동시 연결
- Todo priority/due date/assignee/repeat/history
- cloud sync
- Confluence API integration
- automatic document indexing
- Herdr execution controls
- SQLite 도입

## Acceptance criteria

1. 1280px desktop에서 고정 190px/300px sidebars가 사라지고 main content가 대부분의 폭을 사용한다.
2. brand, connection, Toolbox, Settings가 top bar에 모인다.
3. 선택 업무 action이 별도 rail 없이 project header에서 동작한다.
4. 프로젝트 내부에는 `업무 / 자료`만 있다.
5. 명령어는 프로젝트 선택과 무관하게 전역 Toolbox drawer에서 같은 내용을 본다.
6. Todo는 공통 또는 프로젝트 하나에 연결할 수 있고 전역/공통/프로젝트 필터로 모아볼 수 있다.
7. 프로젝트 상세에서 해당 프로젝트 Todo 개수를 확인하고 같은 Drawer를 프로젝트 필터 상태로 열 수 있다.
8. 기존 project-level command/checklist 데이터가 있으면 자동 migration 후 command는 전역, checklist는 원래 프로젝트 연결 Todo로 보존된다.
9. command 실행 API가 존재하지 않는다.
10. Herdr 세부 정보는 기본 collapsed이며 status summary는 항상 보인다.
11. Toolbox 데이터는 Agent/Skill에 자동 전달되지 않는다.
12. 기존 Monitor open-only, all-history, Markdown, GHES, Herdr read-only behavior를 깨지 않는다.
