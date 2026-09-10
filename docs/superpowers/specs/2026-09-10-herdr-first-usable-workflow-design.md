# Herdr-first First Usable Workflow Design

## 상태와 설계 명제

2026-09-10 사용자가 승인한 architecture pivot이다. 기준 main은
`f12bf77c5755ef16c3d93ffcb4403f710a374cda`이며, 이 문서는 구현 완료를 뜻하지 않는다.

> 멀티 세션과 서브 에이전트가 잘 잡힌 구조에서 GitHub 중심 모니터링을 제공한다.

GitHub는 업무 목적·결정·Issue·PR·검증·문서의 영속 중심이다. Herdr는 live Session·Workspace·Tab·Pane과 그 안의 Top-level Codex/OpenCode를 소유한다. 각 Top-level Agent는 native Internal Subagent를 직접 조정한다. ThreadDock은 두 원본을 결합하는 얇은 Monitor·project skill·선택적 GitHub evidence module이며 별도 Agent lifecycle orchestrator가 아니다.

상세 용어는 [CONTEXT.md](../../../CONTEXT.md), 소유권 결정은 [ADR 0007](../../adr/0007-herdr-owns-interactive-execution-topology.md)을 따른다. 2026-09-07 설계의 GitHub Work Evidence·검증·발행 원칙은 유지하지만 Go coordinator가 대화형 실행 topology를 소유한다는 부분은 이 문서가 대체한다.

## 문제와 성공 조건

Operator는 여러 프로젝트의 Agent 대화를 유지하면서도 “왜 시작했는가, 지금 어디까지 왔는가, 무엇을 해야 하는가”를 다시 복원해야 한다. Herdr만 보면 terminal 상태는 알 수 있지만 GitHub 업무 목적과 결정이 약하고, GitHub만 보면 어떤 live 대화로 돌아가야 하는지 알기 어렵다. ThreadDock이 이 둘을 연결한다.

첫 usable workflow의 성공 조건은 다음과 같다.

1. Monitor에서 GitHub 중심 Project·Work Item·결정·PR·문서 링크와 최신 handoff를 먼저 찾는다.
2. 같은 화면에서 exact Work/repository/canonical Worktree binding으로 연결된 Herdr Session·Workspace·Tab·Pane과 Top-level Agent의 live 상태를 구분해 본다.
3. 사용자가 project skill로 기존 Git Worktree를 열거나 새 Worktree/Tab/Pane을 만들고 Top-level Codex/OpenCode를 시작할 수 있다.
4. Top-level Agent가 native Internal Subagent를 사용해도 Monitor는 개별 subagent를 강제 추적하지 않고 결과 요약과 GitHub evidence만 연결한다.
5. Herdr 상태를 읽지 못하거나 prompt가 정체되면 GitHub evidence는 계속 보이며, 실패 이유와 마지막 관찰 시각을 숨기지 않는다.
6. 실제 작은 업무 하나에서 Herdr 재접속, GitHub 문맥 회수, human merge와 evidence 갱신을 확인한다.
7. 작업 재개는 기존 Agent, blocked Agent, missing/ended Agent를 구분하고 완료 Task를 다시 실행하지 않는다.

## 범위

### 포함

- 설치된 Herdr CLI의 read-only 상태를 정규화하는 adapter
- 기존 Wails Monitor의 aggregate wire에 live Session topology 추가
- GitHub 업무를 우선하는 Project/Work 상세와 별도 Session/Agent 보기
- Herdr의 기존 skill/CLI를 따르는 project skill: create/open Worktree, create/focus Tab·Pane, start/attach Top-level Agent
- 사용자가 승인한 작은 실제 pilot와 GitHub evidence 연결
- 기존 Go state·verification·publisher의 선택적 재사용

### 제외

- 새 Codex runtime adapter나 ThreadDock 소유 Invocation scheduler
- Internal Subagent별 화면, process registry, retry budget 또는 lifecycle 제어
- Herdr를 복제하는 session/process supervisor
- raw prompt, transcript 또는 pane output의 ThreadDock 저장
- Monitor의 채팅 UI, 자동 prompt 전송, 자동 pane/session 종료
- DXHub 메뉴·MCP, 공유 실행 제어, 다중 사용자 권한 모델
- 자동 main merge와 Herdr #3813 해결 주장

## 원본과 소유권

| 관심사 | 원본·소유자 | ThreadDock의 역할 |
|---|---|---|
| 업무 목적·완료 조건·결정 | GitHub Parent/Child Issue와 repository docs | 링크·요약·freshness 표시 |
| 코드 변경·review·merge | Git commit, PR, CI, human merge | evidence 연결과 선택적 검증 receipt |
| 장기 사용법·runbook | GitHub Wiki 또는 repository docs | publication 상태와 링크 표시 |
| persistent terminal topology | Herdr Session·Workspace·Tab·Pane | read-only 관찰, 명시적 skill action 연결 |
| 대화형 Agent session | Herdr Pane의 Top-level Codex/OpenCode | 상태·위치 표시, 시작/attach를 Herdr에 위임 |
| 내부 작업 분배 | Top-level Agent의 native Internal Subagent 기능 | 개별 lifecycle 미소유, 결과 요약만 evidence로 연결 |
| 결정적 state·검증·발행 | 선택적 Go evidence module | 필요할 때 receipt 생성; Session owner 아님 |

이 분리는 의도적인 seam이다. Herdr status adapter의 작은 interface 뒤에 CLI 응답 정규화를 숨기고, Monitor는 provider-specific JSON을 알지 않는다. adapter를 삭제하면 Herdr parsing 복잡성이 Monitor와 Wails 호출부에 퍼져야 하므로 충분한 depth가 있다. 반대로 lifecycle 명령을 adapter interface에 추가하면 Herdr와 ThreadDock의 소유권이 다시 겹치므로 포함하지 않는다.

## Aggregate Monitor interface

Task 1에서 아래 public wire를 직렬로 고정한다. 필드 이름이나 의미가 바뀌면 후속 Task를 멈추고 Sol이 재계획한다.

```go
// internal/monitor/types.go
type Snapshot struct {
    SchemaVersion int             `json:"schemaVersion"` // 3
    Revision      contractv2.Revision `json:"revision"`
    ObservedAt    time.Time       `json:"observedAt"`
    Freshness     Freshness       `json:"freshness"`
    State         string          `json:"state"`
    SyncStatus    string          `json:"syncStatus"`
    NextAction    string          `json:"nextAction"`
    EvidenceRefs  []string        `json:"evidenceRefs"`
    Projects      []Project       `json:"projects"`
    LiveSessions  LiveSessions    `json:"liveSessions"`
    Bindings      []WorkSessionBinding `json:"bindings"`
    SafeActions   []SafeAction    `json:"safeActions"`
}

type LiveSessions struct {
    State      string         `json:"state"` // live, degraded, unavailable
    ObservedAt time.Time      `json:"observedAt,omitempty"`
    Sessions   []SessionDetail `json:"sessions"`
    ErrorCode  string         `json:"errorCode,omitempty"`
}

// Existing public fields stay unchanged. This field exists only inside the
// WSL aggregate process and is never serialized by project status or Monitor.
type TaskDetail struct {
    TaskID contractv2.TaskID `json:"taskId"`
    RepoKey contractv2.RepoKey `json:"repoKey"`
    State string `json:"state"`
    Verification string `json:"verification,omitempty"`
    Review string `json:"review,omitempty"`
    Merge string `json:"merge,omitempty"`
    CanonicalWorktree string `json:"-"`
}

type SessionDetail struct {
    Name       string            `json:"name"`
    State      string            `json:"state"` // running, stopped, unavailable
    Workspaces []WorkspaceDetail `json:"workspaces"`
}

type WorkspaceDetail struct {
    WorkspaceID string      `json:"workspaceId"`
    Label       string      `json:"label"`
    RepoKey     string      `json:"repoKey,omitempty"`
    Tabs        []TabDetail `json:"tabs"`
    CanonicalWorktree string `json:"-"`
}

type TabDetail struct {
    TabID string       `json:"tabId"`
    Label string       `json:"label"`
    Panes []PaneDetail `json:"panes"`
}

type PaneDetail struct {
    PaneID string         `json:"paneId"`
    Label  string         `json:"label"`
    Agent  *TopLevelAgent `json:"agent,omitempty"`
}

type TopLevelAgent struct {
    Kind  string `json:"kind"`  // codex, opencode
    Name  string `json:"name,omitempty"`
    State string `json:"state"` // working, blocked, idle, done, unknown
}

type WorkSessionBinding struct {
    BindingID  string               `json:"bindingId"`
    ProjectID   contractv2.ProjectID `json:"projectId"`
    WorkID      contractv2.WorkID    `json:"workId"`
    TaskID      contractv2.TaskID    `json:"taskId"`
    RepoKey     contractv2.RepoKey   `json:"repoKey"`
    SessionName string               `json:"sessionName,omitempty"`
    WorkspaceID string               `json:"workspaceId,omitempty"`
    TabID       string               `json:"tabId,omitempty"`
    PaneID      string               `json:"paneId,omitempty"`
    State       string               `json:"state"` // linked, missing, ended, conflict
    ObservedAt  time.Time            `json:"observedAt,omitempty"`
}

type SafeAction struct {
    Kind             string            `json:"kind"` // open_github, copy_handoff, focus_agent, attach_session, prompt_handoff, start_agent, inspect_blocked
    WorkID           contractv2.WorkID `json:"workId"`
    BindingID        string            `json:"bindingId,omitempty"`
    RequiresOperator bool              `json:"requiresOperator"`
    ObservationTime  time.Time         `json:"observationTime,omitempty"`
}

type WorkEvidenceSource interface {
    Snapshot(context.Context, time.Time) (Snapshot, error)
}

type LiveSessionSource interface {
    Observe(context.Context, time.Time) (LiveSessions, error)
}

type AggregateSource interface {
    FetchAll(context.Context) (Snapshot, error)
}
```

`internal/monitor.Aggregator`가 `WorkEvidenceSource`와 `LiveSessionSource`를 한 시각에 읽어 하나의 Snapshot을 반환한다. Work Evidence 조회가 실패하면 기존 last-good 정책으로 전체 snapshot을 stale/offline 처리한다. Herdr만 실패하면 Projects와 GitHub 링크는 새 값으로 유지하고 `liveSessions.state=degraded|unavailable`, stable `errorCode`, 마지막 성공 관찰을 표시한다. 오류 원문, socket path, terminal output과 Agent session ID는 public wire에 넣지 않는다.

Work source는 각 Task의 exact `projectId`, `workId`, `taskId`, `repoKey`, 승인된 canonical Worktree를 aggregate 내부 입력으로 제공하고 JSON에는 canonical path를 노출하지 않는다. Herdr source도 Workspace의 exact `repo_key`와 canonical checkout path를 aggregate 내부 입력으로 제공한다. aggregator는 두 값이 모두 byte-for-byte 일치할 때만 Workspace 후보로 삼는다. Workspace 0개는 `missing`, 2개 이상은 `conflict`다. 유일한 Workspace에서 Top-level Agent Pane 1개는 `linked`, 0개는 `ended`, 2개 이상은 `conflict`다. label, cwd suffix, focused Pane, 최근 시각으로 연결을 추정하지 않는다.

`bindingId`는 `work:<workId>:task:<taskId>` 형식이며 Session/PID 같은 provider identity를 넣지 않는다. 하나의 Task에 여러 live match가 있어도 binding row 하나를 `conflict`로 반환하고 safe action을 만들지 않는다.

`WorkItem.state`와 검증·publication 상태는 **업무 기록 상태**, `TopLevelAgent.state`와 `LiveSessions.state`는 **로컬 실행 상태**다. 둘은 각각 observedAt과 stale/offline을 유지한다. live Agent가 working이어도 Work를 running으로 바꾸지 않고, Work가 completed여도 Pane을 done/close로 바꾸지 않는다.

WSL의 새 read-only 명령은 `agentctl monitor snapshot --json` 하나며 aggregate wire의 `schemaVersion`은 `3`이다. Windows `monitorcli`는 이 명령만 호출하고 Wails는 기존 `GetMonitorSnapshot()` binding을 유지한다. 기존 `agentctl project status --all --json`과 schema `2`는 호환을 위해 Work Evidence 전용으로 유지하며 topology를 암묵적으로 추가하지 않는다.

## Herdr 상태 adapter

설치된 Herdr 0.8.2에서 다음 read-only interface가 확인됐다.

- `herdr session list --json`
- `herdr --session <name> workspace list`
- `herdr --session <name> tab list --workspace <workspace-id>`
- `herdr --session <name> pane list --workspace <workspace-id>`
- `herdr --session <name> agent list`

adapter는 `session list`의 running session만 상세 조회하고 stopped session은 `state=stopped`, 빈 Workspaces로 보존한다. ID는 opaque handle로 취급하고 생성하지 않는다. Workspace의 `repo_key`와 canonical checkout path는 aggregate 내부 binding 입력으로만 사용한다. 둘 중 하나라도 없으면 `missing`이며 label이나 cwd 일부 문자열로 Work Item을 추정하지 않는다. Pane에 인식된 Codex/OpenCode가 있을 때만 `TopLevelAgent`를 만든다. Internal Subagent, raw terminal text, terminal ID와 native agent session ID는 수집하지 않는다.

한 named session의 상세 조회 실패는 다른 session과 GitHub evidence를 버리지 않는다. adapter는 허용된 stable code(`herdr_not_installed`, `session_list_failed`, `session_detail_failed`, `invalid_response`)와 관찰 시각만 반환한다. timeout은 bounded하며 polling은 기존 Monitor interval과 중첩 실행 방지를 따른다.

## Monitor 정보 구조와 행동

기본 view는 GitHub 중심 **작업 현황**이다. Project → Work Item 상세에서 목적, next action, blocker, 결정, 검증, Parent/Child Issue, PR, Wiki와 handoff를 먼저 보여준다. 연결된 live session 수와 상태는 보조 정보로 표시한다.

별도 **세션과 Agent** view는 Herdr Session → Workspace → Tab → Pane의 접힌 구조를 보여준다. Pane에는 Top-level Agent kind/name/state와 연결된 repoKey만 표시한다. Internal Subagent 수나 상태는 표시하지 않는다. Agent가 남긴 Task review·검증·handoff는 GitHub/ThreadDock evidence 영역에서 요약한다.

Monitor가 제안할 수 있는 safe action은 `open_github`, `copy_handoff`, `focus_agent`, `attach_session`, `prompt_handoff`, `start_agent`, `inspect_blocked`로 제한한다. 모든 Herdr action에는 해당 deterministic bindingId와 observationTime이 붙고 `requiresOperator=true`다. UI는 최신 refresh에서 같은 binding과 opaque handle, 상태가 유지되는지 확인한 뒤 project skill에 위임한다.

- 안전한 GitHub 링크 열기
- handoff 복사
- Herdr Session attach 또는 Top-level Agent focus 요청
- 현재 handoff를 기존 idle/done Agent에 한 번 전달하는 명시적 요청
- blocked Agent의 화면을 inspect한 뒤 질문·승인을 사용자에게 보여주는 요청
- missing/ended일 때 승인된 Worktree를 open/create하고 새 Top-level Agent를 시작하는 요청
- project skill 실행 방법 안내

Monitor는 자동 prompt, retry, stop, close, worktree remove, agent 분배, PR merge를 수행하지 않는다. live handle이 stale이면 조작하지 않고 refresh/attach 안내를 보여준다.

### 작업 재개 규칙

1. binding이 `linked`이고 Agent가 `working`이면 focus/attach만 제안하고 새 prompt를 보내지 않는다.
2. binding이 `linked`이고 Agent가 `idle|done`이면 focus/attach 또는 최신 handoff의 명시적 1회 prompt를 제안한다. `done`은 업무 완료 의미가 아니다.
3. Agent가 `blocked`이면 `inspect_blocked`만 제안한다. 안전한 `agent get/read`로 질문·approval UI를 확인하고 Operator 입력 전에는 key/text를 보내지 않는다.
4. binding이 `missing`이거나 기존 Agent가 ended이면 승인된 canonical Worktree를 open하고 새 Tab/Pane과 Top-level Agent를 시작한다. prompt는 최신 handoff, 미완료 Task, GitHub links만 포함하며 completed Task는 제외한다.
5. binding이 `conflict`, observation이 stale/offline 또는 Agent가 `unknown`이면 자동 action을 만들지 않고 exact target 선택 또는 refresh를 요구한다.

이 규칙은 작업을 다시 시작하는 workflow engine이 아니라 Herdr 원본으로 돌아가는 안전한 안내다. Top-level Agent 내부 subagent는 별도 Pane·binding·Monitor row가 아니다.

## Project skill 흐름

새 project skill은 사용자가 명시적으로 “이 업무를 Herdr에서 열어라/시작하라”고 요청했을 때만 동작한다.

1. `test "${HERDR_ENV:-}" = 1`과 `herdr --help`, `herdr --skill`을 확인한다.
2. GitHub Work Evidence에서 Project, Repository, branch/base와 next action을 읽고 사용자 변경을 보존한다.
3. aggregate가 제시한 binding과 observationTime을 최신 read-only 상태로 다시 확인한다. `conflict`, stale/offline이면 멈춘다.
4. existing live Agent는 focus/attach하고, idle/done에만 사용자가 승인한 최신 handoff를 한 번 prompt할 수 있다.
5. blocked Agent는 먼저 inspect하고 질문·approval을 사용자에게 보여준다. 입력은 사용자의 다음 행동이다.
6. missing/ended이면 기존 Git Worktree를 `herdr worktree open`, 새로 필요하고 승인됐으면 `herdr worktree create`로 연다. completed Task를 제외한 handoff를 준비한다.
7. 사용자가 원하는 topology에 따라 기존 Tab/Pane을 focus하거나 `tab create`/`pane split --no-focus`를 사용한다.
8. 빈 interactive shell Pane인지 확인한 뒤 `herdr agent start <name> --kind codex|opencode --pane <id>`로 Top-level Agent를 시작한다.
9. 생성 응답의 실제 opaque ID를 기록하고, 자동으로 추측한 ID나 sidebar 순서를 사용하지 않는다.
10. 초기 prompt가 필요하면 한 번만 전송한다. `blocked`, `unknown`, `agent_not_ready`, `agent_prompt_stalled`이면 `agent get/read`의 안전한 요약을 보여주고 멈춘다.

`agent_prompt_stalled` 뒤 prompt를 자동 재전송하지 않는다. Operator가 Herdr에 직접 attach해 화면을 확인한 다음 수동 prompt 또는 한 번의 명시적 재시도를 선택할 수 있다. 이 fallback은 사용자의 실제 행동 없이는 실행되지 않는다. Herdr #3813은 해결되지 않은 upstream 문제로 유지한다.

## 기존 Go 실행 실험의 위치

기존 atomic runtime seam과 Codex adapter 구현은 결정적 evidence module의 가능성을 검증한 실험이다. accepted commit은 보존하지만 Herdr-first workflow의 선행 조건이나 Top-level Agent owner가 아니다. 진행 중이던 production wiring 변경도 삭제·덮어쓰기하지 않고 paused superseded experiment로 남긴다.

`internal/state/v2`, verification gate, Git inspection과 GitHub Publisher는 다음 조건에서만 재사용한다.

- GitHub evidence나 authoritative verification receipt에 직접 가치를 준다.
- Top-level Agent와 Herdr topology의 생명주기를 소유하지 않는다.
- Monitor가 이 module 없이도 GitHub 링크와 live Herdr 상태를 읽을 수 있다.
- 기존 Run을 자동 재생하거나 오래된 Invocation을 재시도하지 않는다.

새 Codex runtime wiring, universal role packet scheduler, ThreadDock 내부 subagent registry는 추가하지 않는다.

## Pilot와 실패 처리

실제 pilot은 새 이름·새 Worktree를 쓰는 작고 되돌릴 수 있는 업무 하나다. 실행 전에 repository 상태, Herdr 0.8.2, security baseline, GitHub target과 human merge 조건을 확인한다. 기존 `/tmp/threaddock-herdr-live.7YEhfV`의 보존 invocation은 조회·prompt·종료·재사용하지 않는다.

Pilot은 다음을 증명한다.

- Herdr Session에 attach하고 Project Worktree의 Top-level Agent로 돌아갈 수 있다.
- Top-level Agent가 필요하면 native Internal Subagent를 사용하고 결과를 한 문단 summary와 commit/review evidence로 남긴다.
- Monitor에서 GitHub 업무와 live Pane 상태를 함께 찾는다.
- `agent_prompt_stalled` 또는 blocked 상태가 나면 실패가 보이고 자동 재전송되지 않는다.
- PR은 한국어 제목·본문으로 만들고 Operator가 GitHub에서 merge한다.

Herdr 상태가 unavailable이어도 GitHub Work Evidence는 계속 보인다. GitHub sync가 실패하면 live session 상태와 분리해 표시한다. 어떤 한쪽의 성공도 다른 쪽의 freshness나 완료를 대신하지 않는다.

## 검증과 완료 기준

각 구현 Task는 직접 변경과 의존 영향만 focused 검증하고 fresh Sol task-review를 거친다. shared monitor types와 CLI 공유 파일은 Task 1이 단독 소유한다. worker별 full suite는 금지하며, 통합 code PR의 마지막 `make check`만 한 번 실행한다. 이 문서 변경은 link/format 검사와 `git diff --check`만 수행한다.

첫 workflow는 다음 근거가 모두 있을 때만 완료다.

- aggregate wire fixture와 Herdr partial-failure 테스트
- Wails 작업 현황 및 세션/Agent view의 frontend 테스트와 실제 화면 확인
- project skill의 static contract test와 controlled dry-run
- 새 pilot의 Herdr Session/Workspace/Tab/Pane 및 Top-level Agent 관찰 요약
- GitHub Parent Issue/PR/docs/handoff 링크, 검증·review 근거, human merge SHA
- 성공하지 못한 단계의 explicit blocker와 next action

이 기준은 첫 usable workflow의 기준이며 다중 사용자 공유 실행, DXHub, 모든 GitHub publication 종류 또는 Herdr upstream 수정 완료를 뜻하지 않는다.
