# GitHub 중심 첫 사용 구현 계획

> **For agentic workers:** use superpowers:subagent-driven-development or superpowers:executing-plans when implementing code tasks. 이미 승인한 범위에서 작은 사용 가능한 결과를 끝낸다.

**Goal:** 지금은 Skill로 기능 개발을 시작하고, Monitor에는 실제 GitHub 업무와 Herdr 연결을 순서대로 붙인다.
**Architecture:** GitHub가 업무 원본, Herdr가 실행 원본, Agent가 분배·개발·기록을 맡는다. Monitor는 읽기와 돌아가기 안내만 제공한다.
**Tech Stack:** 기존 GitHub/gh, Herdr, Codex/OpenCode, React/Vite/Node. 기존 Go/Wails는 보존한다.
**Spec:** [첫 사용 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## 범위

- 이 계획이 기존 herdr-first-01-status-wire부터 시작하는 5-Task 계획을 대체한다.
- 신규 경로는 Contract v2, Work store, Invocation/Artifact, Go Publisher에 의존하지 않는다.
- 기존 실험 branch와 사용자 미커밋 변경을 보존한다.
- 중앙 Sol medium, 구현 Luna high, 독립 리뷰 Sol medium 지침을 유지한다. 실제 환경이 지원하지 않으면 정확히 보고한다.
- 문서/Skill은 구조·링크·실제 요청 시나리오를 검증한다. 제품 코드 변경은 focused 검사와 마지막 통합 make check 한 번.
- 각 PR은 사용자에게 쓸 수 있는 결과와 남은 제한을 함께 보고한다. main 병합은 사람에게 남긴다.

## A. 바로 사용하는 Skill 운영 경로

**Files:** project-template/.agents/skills/{plan-work,implement-task,review-change,open-agent-session,record-work}/SKILL.md,
project-template/AGENTS.md, internal/projecttemplate/template_test.go, docs/operator/github-first-quickstart.md,
AGENTS.md, .codex/agents/td_coordinator.toml, README.md, PRODUCT.md, CONTEXT.md, HANDOFF.md.

- [x] 이전 strict runtime 입력·Go 커밋 소유 규칙을 기존 세 Skill에서 제거한다.
- [x] 중앙의 기능 분배와 기능 리더의 상세계획·native subagent·커밋·PR 책임을 정한다.
- [x] 세션 열기/돌아가기와 GitHub 기록 Skill을 제공한다. 새로운 실행 engine을 호출하지 않는다.
- [x] 기존 Issue를 가진 사용자, 승인된 병렬 기능 배정, 읽기 전용 리뷰 세션, 세션 부재와 handoff, 불명확한 prompt 결과의 요청 시나리오를 검토한다.
- [x] 오래된 v2 문구 일치 검사를 제거하고 Skill 파일/이름/설명·참조 구조 검사를 유지한다.
- [x] 빠른 시작을 따라 기존 Agent에서 Skill을 직접 읽거나 선택적으로 설치할 수 있는지 확인한다.

**완료 결과:** 중앙 관제/기능 세션에 줄 시작 지침을 실제로 사용할 수 있다. 통합 Monitor 완성으로 보고하지 않는다.

## B. 기존 GitHub 업무를 보여주는 첫 Monitor PR

**Files:** monitor/frontend/server의 새 GitHub 조회 adapter와 Node tests,
monitor/frontend/vite.config.ts, src/{bindings.ts,App.tsx,types.ts}와 focused UI tests.
기존 화면에 브라우저용 조회 경로를 직접 연결하고 새 Go/agentctl route는 만들지 않는다.

**입력:** 사용자가 선택한 Projects URL과 관련 repository 목록. 로컬 Work/Contract 없이 시작한다.
**출력:** 프로젝트·Issue URL/제목/상태/우선순위/최근 기록·PR 상태/검증/리뷰·설계/Wiki 링크,
GitHub 마지막 성공 조회 시각과 부분 조회/오류 여부.
필드명은 이 PR의 실제 provider 응답과 기존 UI를 대조해 한 번 정한다. 먼저 큰 공통 schema를 따로 만들지 않는다.

- [x] 기존 React 화면을 재사용하고 Wails binding 부재 시 로컬 read-only GET endpoint를 조회한다.
- [x] 기존 Work store가 비어 있는 상태에서 GitHub Issue가 화면에 나오는 검사를 먼저 작성한다.
- [x] Projects 항목·필드에서 상태/우선순위를 읽고 Issue/PR 근거를 연결한다.
- [x] 저장소와 보드별 최대 100개 조회 제한을 화면에 명시한다. 두 저장소의 같은 Issue 번호를 혼동하지 않는다.
- [x] GitHub 오류에는 마지막 성공 결과와 그 시각을 유지한다. 인증·원문 오류는 그대로 출력하지 않는다.
- [x] 기존 목록/상세를 브라우저에서도 사용한다. GitHub 링크 열기와 handoff 복사를 제공한다. 기존 Wails 모드는 보존한다.
- [x] GitHub-only 응답·부분 실패·조회 제한을 focused 검증하고, fixture CLI로 두 저장소와 보드를 실제 Vite HTTP 경로에서 확인했다. UI 테스트는 통과했으며, 원격 브라우저의 localhost 접속 차단으로 육안 화면 확인은 미확인이다.

**완료 결과:** GitHub에서 바꾼 상태·우선순위·기록이 Monitor에 반영된다.
B 단계에는 Herdr 조회를 포함하지 않는다. C에서 읽기 전용 관찰을 추가하며 prompt 버튼과 실행 계약 작성 기능은 계속 제외한다.
Projects 접근 불가 시 저장소 Issue 한정 표시와 제한을 명시한다.

## C. 명시적 Herdr 연결과 작업 재개

**Files:** B의 로컬 read-only adapter와 frontend, 빠른 시작 문서.
현재 환경에는 Herdr가 없어 v0.8.2 공개 응답과 fixture를 기준으로 관찰 함수를 구현한다.
설치 버전의 실제 응답 확인과 두 기능 운영 검증은 사용자 환경에 남긴다.

**입력:** B의 GitHub 업무와 선택적 .threaddock/sessions.json, 설치된 Herdr read-only 응답.
**출력:** 업무 옆에 해당 세션 위치·실행 상태·Herdr 마지막 성공 시각, 미연결/부재/확인 불가.
중앙 관제는 프로젝트 수준으로 연결한다.

- [ ] 먼저 설치된 Herdr 응답으로 실제 이름·opaque ID·상태 필드를 확인한다.
- [x] 명시된 Issue 또는 프로젝트 연결을 조회한다. 정확한 pane이 있으면 같은 workspace의 다른 Agent 때문에 conflict로 처리하지 않는다.
- [x] GitHub 실패/Herdr 성공과 그 반대에서 살아 있는 원본은 계속 표시하고 시각을 독립 보존하는 검사를 작성한다.
- [x] 존재하지 않는 위치와 조회 실패를 구분한다. 연결 없는 업무/세션도 탐색할 수 있게 한다.
- [x] UI는 위치 안내·GitHub 열기·handoff 복사만 제공한다. 실제 시작/입력은 기존 Agent와 open-agent-session Skill을 사용한다.
- [ ] 중앙이 작은 독립 기능 둘을 배정하고 각 리더가 구현·독립 리뷰·PR을 만든다. Issue 결정·Projects 현황·필요한 Wiki 갱신을 확인한다.
- [ ] 프로젝트 전환 후 기존 세션에 돌아가거나 handoff로 재개한다. 실제 Windows/Herdr 결과와 fixture 검증을 별도로 기록한다.

**완료 결과:** GitHub 기록과 로컬 상태를 한 화면에서 보고 해당 기능으로 돌아갈 수 있다.
이후 반복되는 실제 불편이 있을 때만 작은 조작 편의를 추가한다.

### Task 1: 지정된 Herdr 세션의 관찰·연결·재개 안내

**배경:** 사용자가 PR #69 이후 가능한 구현을 계속하도록 요청했다. 새 엔진 없이 브라우저 Monitor의 남은 로컬 관찰을 연결한다.
**taskId:** simple-herdr-status
**baseSHA:** c523dda54fbfbc43794e43c3594de1ad4b44bae8 (로컬 검토 snapshot; 원격 PR 기준 6e1822702283dd5f43c881e0c473de7e12b9548d)
**deps:** 완료된 GitHub 브라우저 조회와 기존 화면
**ownedPaths:** monitor/frontend/
**forbiddenPaths:** 그 외 경로, 추적되는 node_modules/build 결과, 기존 Go 실행기와 사용자 호스트
**worktree:** /workspace/scratch/aa953ecaf317/thread-dock-herdr
**branch:** agent/simple-herdr-status

**interface와 동작:**

- 선택적 THREADDOCK_SESSIONS_FILE에 로컬 연결 JSON의 절대 경로를 받는다. 기존 GitHub 환경 설정이 이 변수를 허용한다. 설정이 없으면 Herdr 조회 없이 안내만 제공한다.
- JSON은 version=1, bindings 배열이다. 기존 quickstart의 issueUrl/repository/worktree/session/workspaceId/tabId/paneId/agentName/role을 사용한다. coordinator는 issueUrl 없이 repository 또는 projectUrl로 연결할 수 있다. 파일은 읽기만 하고, 매 갱신에 새 내용을 읽어 삭제/수정을 반영한다.
- HERDR_ENV=1인 실제 Herdr pane에서만 조회한다. 프로그램이 환경 값을 임의로 설정하지 않는다. 파일에 명시된 session 이름을 중복 제거하고 `herdr --session NAME agent list`만 호출한다. 다른 세션 탐색/생성/입력/종료는 하지 않는다.
- 기준 API는 Herdr v0.8.2 공개 소스의 src/session.rs, src/cli/agent.rs, src/api/schema/{agents,response,common}.rs다. 실제 설치 버전 확인과 live 성공은 사용자 PC에서 별도로 확인한다.
- 응답의 result.type=agent_list, result.agents를 읽는다. name, agent_status, workspace_id, tab_id, pane_id, cwd/foreground_cwd만 허용해서 변환한다. terminal_id, agent_session, tokens, 원문 출력은 응답과 UI에 넣지 않는다.
- `getSnapshot()`을 가진 Herdr source를 추가한다. 세션별 5초 캐시/실패 간격과 마지막 성공 시각을 독립 유지한다. subprocess는 인자 배열·timeout·출력 크기 제한을 사용한다. 오류는 분류와 복구 안내를 주되 원문 stderr를 그대로 노출하지 않는다.
- 기존 Snapshot에 선택적 herdr 관찰을 붙인다. GitHub와 Herdr를 독립 source로 읽고 어느 쪽의 실패도 다른 쪽 기록·시각을 지우거나 갱신하지 않는다. 기존 Wails source는 유지한다.
- 연결은 Issue의 완전한 URL 또는 명시적 projectUrl/repository로만 선택한다. issueUrl과 repository가 다른 저장소를 가리키면 불일치다. session/pane/workspace/tab/agentName을 확인하고 기능 연결의 worktree와 실제 cwd를 realpath로 확인한다. 경로가 없거나 확인 불가하면 연결을 확정하지 않는다. cwd가 worktree 하위 폴더인 경우 포함 관계를 확인하며 Git remote 검증으로 과장하지 않는다.
- 같은 workspace의 다른 Agent는 충돌이 아니다. 하나의 Issue에 여러 연결을 허용한다. fresh 목록에 대상이 없으면 missing, 조회 실패는 offline 또는 cached stale, 식별자/경로 불일치는 conflict/unverified로 표시한다. 상태를 업무 완료로 변환하지 않는다.
- 화면에는 선택 업무와 중앙 관제의 연결, 조회한 세션의 미연결 Agent, 마지막 관찰 시각을 보여준다. GitHub 목록이 비어 있거나 실패해도 로컬 관찰을 볼 수 있다.
- handoff 복사에 명시적 위치와 관찰 상태·시각·다음 행동을 포함한다. working은 돌아가 관찰, blocked는 질문 확인, idle/done은 최신 GitHub 기록과 남은 일 확인, missing은 보존된 Git 변경·handoff 확인, stale/offline/conflict/unknown은 먼저 재확인으로 안내한다. 명령 실행 버튼은 추가하지 않는다.

**acceptance/tests:**

- Node focused tests: v0.8.2 실제 응답 형태, 두 세션의 같은 pane ID, 같은 workspace의 여러 Agent, 한 Issue 여러 연결, coordinator, canonical 경로, 대상 부재와 source 실패, 파일 미설정/잘못된 파일, 환경 guard, 숨길 필드 제거, 서로 독립인 GitHub/Herdr 관찰.
- UI focused tests: 업무/프로젝트 전환 시 정확한 연결, GitHub 없음에도 로컬 표시, 상태별 안내와 handoff 복사. 기존 Vite document/module/API composition도 확인한다.
- frontend build 확인. 이미 확인한 동작의 반복 검사와 worker full suite는 피한다. Go/full make check 및 live Herdr는 실행 불가를 정확히 보고한다.
- **result:** changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers와 self-review. native live 성공은 fixture 성공으로 대체하지 않는다.

**검증 상태:** C의 조회·연결·안내 코드는 구현했다. v0.8.2 fixture와 focused 테스트/빌드 검증이며, 위에서 미체크한 실제 설치 응답·두 기능 운영·사용자 환경 재개는 아직 수행하지 않았다.
