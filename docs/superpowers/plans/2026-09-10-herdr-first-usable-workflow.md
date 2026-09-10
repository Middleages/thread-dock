# Herdr-first First Usable Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** GitHub Work Evidence와 Herdr의 persistent multi-session topology를 기존 Wails Monitor에서 함께 보고, project skill로 안전하게 Top-level Agent를 열어 첫 실제 업무를 이어간다.

**Architecture:** GitHub는 업무·결정·PR·문서의 영속 원본이고 Herdr는 Session·Workspace·Tab·Pane과 Top-level Agent의 live 원본이다. 좁은 read-only Herdr status adapter와 aggregate Monitor wire가 둘을 결합하며, Agent 시작·attach는 Herdr CLI를 따르는 명시적 skill 행동으로만 수행한다. 기존 Go state·verification·publisher는 선택적 evidence module로 유지하고 새 Codex runtime wiring이나 중첩 lifecycle orchestrator는 만들지 않는다.

**Tech Stack:** Go 1.27 검증 환경, Herdr CLI 0.8.2, Wails v2, React, TypeScript, Vitest, GitHub Issues/PR/Wiki

**Spec:** `docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md`

## Global Constraints

- 설계 기준 SHA는 `f12bf77c5755ef16c3d93ffcb4403f710a374cda`다. 각 Task 착수 시 latest main과 dependency review SHA를 대조하고 packet의 baseSHA가 달라지면 Sol이 재발행한다.
- Task 1이 `internal/monitor/types.go`, aggregate interface와 CLI wire를 단독 소유한다. 후속 Task는 이 public interface를 변경하지 않는다.
- Task 1→2→3→4→5를 직렬 실행한다. 각 code Task는 Luna high 구현·focused test·self-review 뒤 고정 SHA를 fresh Sol medium이 task-review한다.
- 실제 resolved model/effort telemetry를 확인할 수 없으면 packet result의 `unverified`에 그대로 남긴다. 역할 설정을 제품 Execution Profile이나 runtime capability 보장으로 해석하지 않는다.
- Herdr가 Session·Workspace·Tab·Pane과 Top-level Agent를 소유한다. Top-level Agent가 native Internal Subagent를 소유하며 ThreadDock은 별도 registry, scheduler, retry budget 또는 lifecycle state machine을 추가하지 않는다.
- Monitor public wire에 raw transcript, prompt, pane output, socket path, terminal ID, native agent session ID, secret을 넣지 않는다.
- `agent_prompt_stalled`, `blocked`, `unknown`을 성공이나 완료로 바꾸지 않고 자동 prompt 재전송을 하지 않는다. attach 또는 수동 prompt는 Operator의 명시적 행동 뒤에만 수행한다.
- 기존 `/tmp/threaddock-herdr-live.7YEhfV` invocation과 workspace는 조회·prompt·종료·재사용하지 않는다.
- Worker별 full suite는 금지한다. Task별 focused 검증만 수행하고, 통합 code PR 마지막에 `make check`를 한 번 실행한다.
- PR·Issue 제목과 본문은 한국어로 작성하고 main merge는 Operator가 GitHub에서 수행한다.
- DXHub 메뉴·MCP, 공유 실행 제어, Monitor 채팅, 자동 merge는 제외한다.

## 파일 책임

| 경로 | 책임 |
|---|---|
| `internal/herdrstatus/` | Herdr 0.8.2 read-only JSON을 provider-neutral live topology로 정규화 |
| `internal/monitor/types.go` | GitHub Work Evidence와 LiveSessions를 함께 운반하는 public wire |
| `internal/monitor/aggregate.go` | work source와 live source의 partial-failure 결합 |
| `internal/cli/`, `cmd/agentctl/` | `agentctl monitor snapshot --json` production composition |
| `internal/monitorcli/` | Windows에서 새 aggregate 명령 하나를 호출하고 last-good 보존 |
| `monitor/frontend/` | GitHub 중심 작업 view와 Session/Agent view |
| `project-template/.agents/skills/open-agent-session/` | Herdr skill/CLI를 따르는 명시적 open/focus/start 절차 |
| `docs/operator/herdr-first-pilot.md` | 실제 pilot의 비밀 없는 실행·실패·근거 기록 |
| `HANDOFF.md` | first-use 결과, GitHub 링크와 다음 행동 |

---

### Task 1: Herdr status adapter와 aggregate Monitor wire

**Task packet**

```yaml
taskId: herdr-first-01-status-wire
baseSHA: f12bf77c5755ef16c3d93ffcb4403f710a374cda
deps: []
ownedPaths:
  - internal/herdrstatus/**
  - internal/monitor/types.go
  - internal/monitor/types_test.go
  - internal/monitor/aggregate.go
  - internal/monitor/aggregate_test.go
  - internal/workflow/service.go
  - internal/workflow/service_test.go
  - internal/cli/run.go
  - internal/cli/project_work.go
  - internal/cli/project_work_test.go
  - cmd/agentctl/main.go
  - cmd/agentctl/main_test.go
  - internal/monitorcli/client.go
  - internal/monitorcli/client_test.go
worktree: /home/appuser/dev_system/.worktrees/herdr-first-workflow
branch: agent/herdr-first-workflow
forbiddenPaths:
  - go.mod
  - go.sum
  - internal/runtime/**
  - internal/coordinator/**
  - internal/herdr/runtime.go
  - monitor/frontend/**
  - project-template/**
interface: spec의 Aggregate Monitor interface 전체와 agentctl monitor snapshot --json
acceptance:
  - Work Evidence와 Herdr live topology가 한 snapshot에 있으나 freshness와 오류 상태는 분리된다.
  - exact repoKey와 canonical Worktree가 모두 일치한 유일한 Workspace에서 Agent 1/0/multiple은 linked/ended/conflict이고 Workspace 0/multiple은 missing/conflict다.
  - 업무 기록 상태와 로컬 실행 상태를 서로 추정하거나 덮어쓰지 않는다.
  - working/idle|done/blocked/missing|ended 상태별 safe action이 spec matrix와 같고 unknown/conflict/stale/offline에는 Herdr action이 없다.
  - stopped session과 한 session의 상세 실패가 다른 GitHub/Herdr 근거를 제거하지 않는다.
  - public wire에 transcript, terminal/native session identity와 socket path가 없다.
  - 기존 project status --all --json 의미는 바뀌지 않는다.
tests:
  - go test ./internal/herdrstatus ./internal/monitor ./internal/workflow ./internal/cli ./internal/monitorcli ./cmd/agentctl
result:
  status: not_started
  changedFiles: []
  commitSHA: null
  executedCommands: []
  outcomes: []
  unverified:
    - resolved model과 reasoning effort runtime telemetry
    - 실제 named session 여러 개의 live 조회
  blockers: []
```

**Interfaces:**

- Consumes: 기존 `workflow.Service.Snapshot(context.Context, time.Time) (monitor.Snapshot, error)`, `runner.Runner`, Herdr 0.8.2 read-only commands.
- Produces: spec의 `LiveSessions`, `SessionDetail`, `WorkspaceDetail`, `TabDetail`, `PaneDetail`, `TopLevelAgent`, `WorkSessionBinding`, `SafeAction`, `WorkEvidenceSource`, `LiveSessionSource`; `monitor.NewAggregator(work WorkEvidenceSource, live LiveSessionSource, clock func() time.Time) *Aggregator`; `(*Aggregator).FetchAll(context.Context) (Snapshot, error)`; `agentctl monitor snapshot --json`.

- [ ] **Step 1: public wire와 JSON red tests를 작성한다.**

  `internal/monitor/types_test.go`에 `LiveSessions`, `Bindings`, `SafeActions`가 빈 배열도 `[]`로 직렬화되고 forbidden key `rawTranscript`, `terminalId`, `agentSessionId`, `socketPath`, `canonicalWorktree`가 어느 depth에도 없음을 검증한다. 새 aggregate wire의 `schemaVersion`은 `3`이고 기존 Work Evidence 전용 wire는 `2`를 유지한다.

- [ ] **Step 2: aggregate partial-failure red tests를 작성한다.**

  `internal/monitor/aggregate_test.go`에 다음 table을 실제 expected struct로 작성한다: 둘 다 성공→fresh work+live, work 실패→error, live 첫 실패→fresh work+`unavailable`, live 성공 뒤 실패→fresh work+retained `degraded`. exact repoKey+canonical Worktree Workspace 0/1/2 match와 유일 Workspace의 Agent 0/1/2 match가 각각 `missing`, `ended|linked|conflict`, `conflict`이고 label·focus·path suffix는 match가 아님을 단언한다. working→focus/attach, idle|done→focus/attach/prompt_handoff, blocked→inspect_blocked, missing|ended→start_agent, unknown|conflict|stale|offline→Herdr action 없음도 exact slice로 단언한다. 같은 호출의 업무·live·binding/action `ObservedAt`은 주입 clock의 UTC 값 하나여야 한다.

- [ ] **Step 3: Herdr fixture red tests를 작성한다.**

  `internal/herdrstatus/testdata/`에 설치된 0.8.2의 `session_list`, `workspace_list`, `tab_list`, `pane_list` 최소 JSON을 저장한다. test runner call log로 정확히 `herdr session list --json`, running session별 `herdr --session <name> ... list`만 실행하는지 단언한다. stopped session에는 상세 명령을 실행하지 않고, unknown field/trailing JSON, nonzero exit, timeout과 한 session 상세 실패를 stable error code로 검증한다.

- [ ] **Step 4: 최소 adapter와 aggregator를 구현한다.**

  `internal/herdrstatus/source.go`는 runner와 timeout만 받아 `Observe`를 구현한다. provider JSON은 private wire struct로 decode하고 public JSON에는 opaque Herdr ID, label, repoKey, agent kind/name/state만 복사한다. exact binding용 canonical checkout path는 `json:"-"` 내부 필드로만 운반하고 cwd, terminal ID와 native session identity는 버린다. `internal/monitor/aggregate.go`는 work 실패, live 실패와 binding 정책을 Step 2와 동일하게 구현하고 record/local state를 독립 유지한다.

- [ ] **Step 5: 새 read-only CLI route를 연결한다.**

  `internal/cli`에서 정확한 form `monitor snapshot --json`만 dependency를 요구하게 하고 `cmd/agentctl/main.go`가 existing workflow service, `herdrstatus.Source`, `monitor.Aggregator`를 조합하게 한다. 기존 `project status --all --json`은 기존 service만 호출한다. malformed form은 provider command를 한 번도 실행하지 않고 usage exit 2를 반환한다.

- [ ] **Step 6: Windows adapter 명령을 교체한다.**

  `internal/monitorcli/client.go`의 argv를 `--exec agentctl monitor snapshot --json`으로 바꾸고 exact argv, timeout, last-good clone, `schemaVersion != 3`과 `liveSessions.sessions` nil 거부를 기존 테스트에 추가한다.

- [ ] **Step 7: focused 검증·self-review·commit을 수행한다.**

  Run: `go test ./internal/herdrstatus ./internal/monitor ./internal/workflow ./internal/cli ./internal/monitorcli ./cmd/agentctl`

  Expected: PASS. `git diff --check`도 통과한 뒤 packet result를 채우고 `git commit -m "기능: Herdr 세션 상태를 Monitor wire에 연결"`로 commit한다. fresh reviewer가 fixed SHA, spec, packet과 command evidence를 검토하며 block이면 같은 Luna가 영향받은 테스트만 재실행한다.

---

### Task 2: GitHub 중심 Monitor Session/Agent view

**Task packet**

```yaml
taskId: herdr-first-02-monitor-view
baseSHA: f12bf77c5755ef16c3d93ffcb4403f710a374cda
deps:
  - herdr-first-01-status-wire
ownedPaths:
  - monitor/frontend/src/App.tsx
  - monitor/frontend/src/types.ts
  - monitor/frontend/src/styles.css
  - monitor/frontend/src/monitor.test.tsx
  - monitor/frontend/src/test-setup.ts
worktree: /home/appuser/dev_system/.worktrees/herdr-first-workflow
branch: agent/herdr-first-workflow
forbiddenPaths:
  - go.mod
  - go.sum
  - internal/**
  - cmd/**
  - monitor/app.go
  - project-template/**
interface: Task 1의 monitor Snapshot JSON을 그대로 소비
acceptance:
  - 기본 작업 현황은 GitHub 목적, next action, 결정, PR/Wiki 링크를 session보다 먼저 보여준다.
  - 세션과 Agent view는 Session→Workspace→Tab→Pane→Top-level Agent만 보여준다.
  - Internal Subagent와 forbidden runtime identity는 렌더링하지 않는다.
  - Herdr degraded/unavailable은 GitHub Work Evidence를 가리거나 전체 offline으로 오표시하지 않는다.
  - 업무 기록 상태와 로컬 실행 상태, 각 observedAt과 binding missing/conflict를 별도 표시한다.
  - stale/offline/conflict에는 Herdr action을 제공하지 않고 모든 safe action은 사용자 확인을 요구한다.
tests:
  - npm test -- --run
  - npm run build
result:
  status: not_started
  changedFiles: []
  commitSHA: null
  executedCommands: []
  outcomes: []
  unverified:
    - Windows 11 실제 Wails 렌더링과 100~200% 배율
  blockers: []
```

**Interfaces:**

- Consumes: Task 1의 exact JSON fields.
- Produces: frontend 내부 `WorkView`와 `SessionView`; Go/Wails public interface 변경 없음.

- [ ] **Step 1: frontend type와 fixture를 Task 1 wire에 맞춘다.**

  `types.ts`에 spec과 동일한 `LiveSessions`, `SessionDetail`, `WorkspaceDetail`, `TabDetail`, `PaneDetail`, `TopLevelAgent`를 추가하고 `Snapshot.liveSessions`를 required로 둔다.

- [ ] **Step 2: 실패하는 interaction tests를 쓴다.**

  기본 render에서 “오늘의 작업 원장”, Parent Issue/PR/Wiki 링크와 next action이 보이고 live pane 상세는 접혀 있음을 단언한다. “세션과 Agent”를 누르면 hierarchy와 `Codex · 진행 중`이 보이되 fixture의 Internal Subagent 설명이나 native session ID sentinel은 보이지 않아야 한다. record `completed`+local `working` fixture가 두 label을 그대로 보이고, `degraded/missing/conflict` fixture에서는 GitHub 링크와 정확한 로컬 상태가 동시에 보여야 한다.

- [ ] **Step 3: 두 view를 최소 구현한다.**

  기존 top nav의 placeholder “자동화 작업”을 “세션과 Agent”로 바꾼다. Project/Work view의 Issue·PR·Wiki 링크를 kind별 한국어 label로 우선 정렬하고 record/local state와 두 observedAt을 나란히 둔다. Session view는 native `<details>` 또는 접근 가능한 disclosure button으로 topology를 접는다. agent state와 binding `missing/conflict`는 텍스트와 기존 StatusMark를 함께 사용한다.

- [ ] **Step 4: safe resume actions를 상태별로 렌더링한다.**

  `working`은 focus/attach, `idle|done`은 focus/attach와 handoff 1회 전달, `blocked`는 inspect만 표시한다. `missing|ended`는 승인된 Worktree에서 새 Agent 시작 안내를 표시하고, `unknown|conflict|stale|offline`은 refresh 또는 exact target 선택만 표시한다. action click은 project skill handoff를 만들 뿐 raw Herdr 입력을 직접 보내지 않는다.

- [ ] **Step 5: responsive/accessibility styling을 추가한다.**

  기존 token을 재사용하고 1280px와 좁은 폭에서 horizontal overflow 없이 hierarchy가 줄바꿈되게 한다. tab navigation, `aria-expanded`, focus-visible을 보존하고 상태를 색만으로 표현하지 않는다.

- [ ] **Step 6: focused 검증·self-review·commit을 수행한다.**

  Run in `monitor/frontend`: `npm test -- --run`

  Run in `monitor/frontend`: `npm run build`

  Expected: both PASS. `git diff --check` 후 packet result를 채우고 `git commit -m "기능: Monitor에 Herdr 세션 보기를 추가"`로 commit한다. fresh reviewer는 GitHub-first hierarchy, degraded semantics, keyboard path와 Task 1 wire 일치를 검토한다.

---

### Task 3: Top-level Agent를 여는 project skill

**Task packet**

```yaml
taskId: herdr-first-03-open-agent-skill
baseSHA: f12bf77c5755ef16c3d93ffcb4403f710a374cda
deps:
  - herdr-first-02-monitor-view
ownedPaths:
  - project-template/.agents/skills/open-agent-session/SKILL.md
  - internal/projecttemplate/template_test.go
worktree: /home/appuser/dev_system/.worktrees/herdr-first-workflow
branch: agent/herdr-first-workflow
forbiddenPaths:
  - go.mod
  - go.sum
  - internal/herdr/**
  - internal/runtime/**
  - internal/coordinator/**
  - monitor/**
interface: 설치된 herdr --skill과 Herdr 0.8.2 CLI만 사용
acceptance:
  - 명시적 사용자 요청 없이는 Herdr topology를 변경하지 않는다.
  - existing open과 approved create를 구분하고 response JSON의 실제 ID만 사용한다.
  - Top-level Agent만 시작하며 native Internal Subagent 조정은 그 Agent에 맡긴다.
  - stalled/blocked/unknown에서 blind retry, close, remove, server stop을 하지 않는다.
  - existing/blocked/missing/ended를 구분하고 완료 Task를 재개 prompt에서 제외한다.
tests:
  - go test ./internal/projecttemplate -run 'TestProjectTemplateSkills'
result:
  status: not_started
  changedFiles: []
  commitSHA: null
  executedCommands: []
  outcomes: []
  unverified:
    - 실제 Herdr action과 agent startup은 Task 4까지 실행하지 않음
  blockers: []
```

**Interfaces:**

- Consumes: `HERDR_ENV=1`, installed `herdr --help`, `herdr --skill`, Work Evidence의 repo/base/branch/next action.
- Produces: project skill `open-agent-session`; 제품 Go interface 없음.

- [ ] **Step 1: static contract test를 먼저 추가한다.**

  `template_test.go`가 새 skill의 존재, frontmatter name, `HERDR_ENV`, `herdr --skill`, `worktree open`, `worktree create`, `tab create`, `pane split`, `agent start`, `agent attach`, `agent focus`, `agent get/read`, `agent_prompt_stalled`, `completed Task 제외` 문구를 요구한다. `server stop`, `worktree remove`, blind retry loop와 ThreadDock subagent registry 문구는 거부한다.

- [ ] **Step 2: skill을 작성한다.**

  skill은 순서대로 environment 확인→GitHub evidence와 exact binding/observationTime 재확인→existing/blocked/missing/ended 선택→명시적 Tab/Pane topology→빈 shell 확인→Top-level Agent start/attach→실제 response ID 보고를 지시한다. existing working은 focus/attach만, idle/done은 사용자가 요청한 최신 handoff 1회 prompt, blocked는 inspect-first, missing/ended는 승인된 Worktree와 미완료 Task handoff로 새 start다. 기본은 focus를 빼앗지 않는 `--no-focus`이며 사용자가 전환을 요청했을 때만 focus한다.

- [ ] **Step 3: prompt failure 절차를 고정한다.**

  initial prompt는 사용자가 요청한 경우 한 번만 보내고 completed Task를 제외한다. `agent_prompt_stalled`, `blocked`, `unknown`, `agent_not_ready`면 `agent get/read`로 안전한 상태만 확인하고 “Herdr에 attach해 직접 확인”, “사용자가 수동 prompt”, “명시적 한 번 재시도” 선택지를 보고한 뒤 멈춘다. 자동 재전송과 #3813 해결 주장을 금지한다.

- [ ] **Step 4: focused 검증·self-review·commit을 수행한다.**

  Run: `go test ./internal/projecttemplate -run 'TestProjectTemplateSkills'`

  Expected: PASS. `git diff --check` 후 packet result를 채우고 `git commit -m "문서: Herdr Agent 세션 열기 skill 추가"`로 commit한다. fresh reviewer는 installed Herdr skill과 명령 문법, destructive action 금지, explicit-user gate를 검토한다.

---

### Task 4: 제어된 실제 Herdr pilot

**Task packet**

```yaml
taskId: herdr-first-04-live-pilot
baseSHA: f12bf77c5755ef16c3d93ffcb4403f710a374cda
deps:
  - herdr-first-03-open-agent-skill
ownedPaths:
  - docs/operator/herdr-first-pilot.md
worktree: /home/appuser/dev_system/.worktrees/herdr-first-workflow
branch: agent/herdr-first-workflow
forbiddenPaths:
  - go.mod
  - go.sum
  - cmd/**
  - internal/**
  - monitor/**
  - project-template/**
interface: Task 3 skill과 새 이름 herdr-first-pilot만 사용
acceptance:
  - 기존 보존 invocation을 건드리지 않고 새 pilot topology만 사용한다.
  - start/attach/context recovery와 Monitor 관찰을 실제 결과 그대로 기록한다.
  - stalled/blocked/unknown이면 자동 재전송 없이 blocker와 사용자 선택을 기록한다.
  - Internal Subagent는 개별 lifecycle이 아니라 top-level summary evidence로만 기록한다.
  - existing Agent attach와 missing/ended 새 Agent 재개 중 실제 해당 경로를 확인하고 completed Task를 재실행하지 않는다.
tests:
  - markdown link existence check
  - git diff --check
result:
  status: not_started
  changedFiles: []
  commitSHA: null
  executedCommands: []
  outcomes: []
  unverified:
    - pilot 실행 전 live Herdr/Windows 결과 전체
  blockers: []
```

**Interfaces:**

- Consumes: reviewed Tasks 1–3 SHA와 project skill.
- Produces: 비밀 없는 pilot observation, stable blocker, next action과 evidence links.

- [ ] **Step 1: preflight를 read-only로 확인한다.**

  Run: `test "${HERDR_ENV:-}" = 1 && herdr --version && herdr session list --json`

  Expected: Herdr `0.8.2`, current managed pane, JSON session list. repository `git status --short`와 target branch/base를 기록하되 secret, socket path, native session ID는 문서에 복사하지 않는다.

- [ ] **Step 2: 새 pilot topology를 한 번 만든다.**

  Task 3 skill을 통해 label/name `herdr-first-pilot`을 사용한다. 기존 feature Worktree를 `herdr worktree open`으로 열고 새 Tab/Pane 또는 기존 빈 Pane을 사용한다. 응답의 Session/Workspace/Tab/Pane opaque ID만 메모리에 유지하며 기존 `/tmp/threaddock-herdr-live.7YEhfV` target은 사용하지 않는다.

- [ ] **Step 3: Top-level Agent와 native subagent 흐름을 확인한다.**

  Top-level Codex 또는 OpenCode를 시작하고 “현재 GitHub Work Evidence를 요약하고 reviewed Task의 focused 검증 근거만 확인하라”는 작은 prompt를 한 번 보낸다. 필요할 때 Agent 자체 native subagent 하나를 사용하게 하되 ThreadDock이 별도 lifecycle을 기록하지 않는다. 결과는 top-level summary, commit SHA, reviewer evidence만 기록한다.

- [ ] **Step 4: failure-visible rule을 적용한다.**

  prompt가 `agent_prompt_stalled`, `blocked`, `unknown`, `agent_not_ready`이면 즉시 멈추고 상태·관찰 시각·안전한 next action을 기록한다. Operator가 실제 attach 또는 수동 prompt를 선택하기 전에는 재전송하지 않는다. 실패한 pilot도 성공으로 꾸미지 않고 Task 5의 GitHub blocker evidence로 이어간다.

- [ ] **Step 5: Monitor context recovery를 확인한다.**

  Wails 화면에서 GitHub Work 상세의 기록 상태/observedAt→binding 상태→Session/Agent의 로컬 상태/observedAt→해당 Pane 위치를 확인한다. 가능하면 existing Agent focus/attach 또는 missing/ended 새 Agent 중 실제 상황에 맞는 재개를 한 번 확인한다. Windows 실행이 불가능하면 정확히 `unverified`로 남기고 Linux/browser fixture를 실제 Wails 검증으로 대체하지 않는다.

- [ ] **Step 6: 근거 문서와 commit을 만든다.**

  `docs/operator/herdr-first-pilot.md`에 version, observedAt, anonymized topology 요약, GitHub links, command outcomes, stalled 여부, manual action 여부, unverified와 blockers를 기록한다. link existence check와 `git diff --check` 후 `git commit -m "문서: Herdr-first 실제 pilot 근거 기록"`로 commit하고 docs-only fresh review를 받는다.

---

### Task 5: GitHub Work Evidence 연속성

**Task packet**

```yaml
taskId: herdr-first-05-github-evidence
baseSHA: f12bf77c5755ef16c3d93ffcb4403f710a374cda
deps:
  - herdr-first-04-live-pilot
ownedPaths:
  - HANDOFF.md
  - docs/operator/herdr-first-pilot.md
worktree: /home/appuser/dev_system/.worktrees/herdr-first-workflow
branch: agent/herdr-first-workflow
forbiddenPaths:
  - go.mod
  - go.sum
  - cmd/**
  - internal/**
  - monitor/**
  - project-template/**
interface: existing agentctl work publish-issues와 GitHub PR/human merge 흐름
acceptance:
  - Parent Issue, 한국어 PR, 검증/review, pilot, docs와 handoff 링크가 한 Work Item에서 이어진다.
  - publication 실패나 ambiguous outcome은 새 Issue를 blind retry하지 않고 conflict/blocker로 남는다.
  - 실제 merge SHA는 human merge 후에만 기록하며 PR 생성이나 green CI를 merge로 표시하지 않는다.
tests:
  - GitHub link read-only verification
  - markdown link existence check
  - git diff --check
result:
  status: not_started
  changedFiles: []
  commitSHA: null
  executedCommands: []
  outcomes: []
  unverified:
    - 외부 GitHub write와 human merge는 실행 시점 전까지 미검증
  blockers: []
```

**Interfaces:**

- Consumes: reviewed Task SHAs, existing Work Item `work-herdr-first-pilot`, Parent Issue draft key `parent`, existing idempotent Publisher, human GitHub merge.
- Produces: durable Parent Issue/PR/docs/handoff links와 실제 merge receipt; 제품 code interface 없음.

- [ ] **Step 1: external target과 revision을 read-only로 확인한다.**

  Run: `agentctl work status work-herdr-first-pilot --json`

  Run: `gh repo view Middleages/thread-dock --json nameWithOwner,defaultBranchRef,url`

  Expected: exact Work ID, current positive revision, repository `Middleages/thread-dock`, default branch `main`. 쓰기 target이나 권한이 모호하면 여기서 멈춘다.

- [ ] **Step 2: 승인된 Parent Issue를 멱등 발행한다.**

  status JSON의 revision을 `pilot_revision`에 담고 다음 exact logical request를 한 번 실행한다: `agentctl work publish-issues work-herdr-first-pilot parent --expected-revision "$pilot_revision" --request-id publish-herdr-first-pilot-parent`.

  Expected: completed receipt와 GitHub URL 또는 explicit conflict/blocker. timeout/response loss에서 동일 request를 자동 반복하지 않고 read-only status와 provider 검색으로 결과를 확정한다.

- [ ] **Step 3: reviewed branch를 push하고 한국어 PR을 만든다.**

  모든 Task review가 accept이고 마지막 code SHA에서 `make check`가 한 번 PASS한 근거가 있을 때만 `agent/herdr-first-workflow`를 push한다. PR 제목은 `기능: Herdr 세션과 GitHub 업무 근거를 Monitor에 연결`로 하고 본문에 Parent Issue, Task review SHA, focused commands, final gate, pilot 결과와 #3813 미해결 상태를 연결한다.

- [ ] **Step 4: human merge를 기다리고 read-only로 확인한다.**

  Operator가 GitHub의 Create a merge commit으로 병합한 뒤 `gh pr view --json state,mergeCommit,url,statusCheckRollup`을 한 번 읽는다. `state=MERGED`와 non-null mergeCommit oid가 확인되기 전에는 Work 완료나 Wiki 확정을 기록하지 않는다.

- [ ] **Step 5: handoff와 선택적 Wiki evidence를 갱신한다.**

  `HANDOFF.md`와 pilot 문서에 Parent Issue, PR, merge SHA, 검증/review, Monitor 확인 결과, 현재 blocker와 next action을 기록한다. 재사용할 운영 지식이 생겼고 Wiki target이 활성화된 경우에만 기존 Publisher/수동 human-reviewed Wiki 흐름으로 발행하며, 실패는 publication pending으로 남긴다.

- [ ] **Step 6: docs 검증과 마지막 docs commit을 수행한다.**

  repository-relative Markdown links가 존재하는지 검사하고 external link는 scheme/host만 parse한다. Run: `git diff --check`.

  Expected: PASS. packet result를 채우고 `git commit -m "문서: Herdr-first 업무 근거와 handoff 갱신"`로 commit한다. docs-only fresh review 뒤 main merge는 Operator에게 남긴다.

## Integration Gate와 Self-review

- Tasks 1–3의 fresh task-review가 모두 accept된 같은 branch의 code HEAD에서만 `make check`를 한 번 실행한다. Task 4–5 docs 변경은 이 결과를 무효화하지 않으므로 반복하지 않는다.
- final gate 실패는 소유 Task에 재현 원인을 귀속해 같은 Luna에 focused fix를 배정한다. 동일 원인이 두 번 반복되면 새 정보가 든 brief로 재분해한다.
- spec의 ownership, partial failure, GitHub-first IA, skill safety, pilot와 evidence continuity가 각각 Tasks 1–5에 매핑된다.
- public types와 CLI 공유 파일은 Task 1만 소유하고 Tasks 2–5는 변경하지 않는다.
- plan에는 새 Codex runtime wiring, Internal Subagent registry, blind retry, automatic merge 또는 Herdr #3813 해결 주장이 없다.
- 각 packet은 `taskId`, `baseSHA`, `deps`, `ownedPaths`, `worktree`, `branch`, `forbiddenPaths`, `interface`, `acceptance`, `tests`, 전체 `result` 필드를 포함한다.
