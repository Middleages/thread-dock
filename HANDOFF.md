# ThreadDock 새 세션 Handoff

## 이번 세션의 종료점

2026-09-10 기준 제품 방향을 **Herdr-first, GitHub 중심 Monitor**로 전환한 문서 slice다.

> 멀티 세션과 서브 에이전트가 잘 잡힌 구조에서 GitHub 중심 모니터링을 제공한다.

GitHub는 Work Item의 목적·결정·Issue·PR·검증·문서를 영속 보존한다. Herdr는 persistent Session·Workspace·Tab·Pane과 Top-level Codex/OpenCode를 소유한다. 각 Top-level Agent가 native Internal Subagent를 조정한다. ThreadDock은 exact Work/repository/canonical Worktree binding으로 GitHub Work Evidence와 live Herdr 상태를 결합하고 작업 재개를 돕는 얇은 Wails Monitor, project skill과 선택적 Go evidence module이며 별도 lifecycle orchestrator가 아니다.

[Herdr-first 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[구현 계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md),
[ADR 0007](docs/adr/0007-herdr-owns-interactive-execution-topology.md)이 새 기준이다.
기존 [Project Workflow MVP 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md)의 GitHub evidence·검증·발행 원칙은 유지하지만 Go coordinator를 대화형 Session/Agent owner로 해석하지 않는다.

이 세션에서는 제품 코드·테스트를 변경하지 않았고 Herdr topology를 생성·종료하거나 Agent에 prompt하지 않았다. main 병합은 사람의 GitHub 작업이다.

## 새 세션에서 먼저 할 일

1. [AGENTS.md](AGENTS.md), 이 문서, [CONTEXT.md](CONTEXT.md), [PRODUCT.md](PRODUCT.md), 새 설계와 새 구현 계획을 읽는다.
2. checkout의 `git status`, HEAD, `origin/main`과 열린 PR을 확인한다. 아래 `f12bf77…`보다 main이 앞서면 새 변경을 먼저 읽는다.
3. 새 계획의 **Task 1 `herdr-first-01-status-wire`만** 최신 main에서 독립 worktree/branch로 구체화한다. shared monitor types와 CLI wire는 이 Task가 단독 소유한다.
4. Luna high 구현·focused test·self-review 뒤 fixed SHA를 fresh Sol medium이 task-review한다. Task 1 review 전 Task 2를 시작하지 않는다.
5. Herdr #3813은 새 정보가 필요할 때만 read-only 확인한다. 해결됐다고 가정하거나 보존된 invocation을 조회·prompt·종료하지 않는다.

| 항목 | 현재 기준선 |
|---|---|
| 저장소 | `Middleages/thread-dock` |
| main / origin/main | `f12bf77c5755ef16c3d93ffcb4403f710a374cda` — PR #67 병합 |
| 최근 병합 | [#62](https://github.com/Middleages/thread-dock/pull/62)~[#67](https://github.com/Middleages/thread-dock/pull/67) |
| Herdr | 로컬 `0.8.2`; persistent session/status CLI 확인 |
| 실행 도구 | native Go 없음으로 알려짐; code Task는 Docker `golang:1.27` 경로를 실제 환경에서 재확인 |
| 다음 Task | `herdr-first-01-status-wire` |

## main #62~#67의 현재 근거

| PR | merge SHA | 결과 |
|---|---|---|
| [#62](https://github.com/Middleages/thread-dock/pull/62) | `f66695d` | 첫 실제 사용 handoff 기준선 |
| [#63](https://github.com/Middleages/thread-dock/pull/63) | `52ed0a8` | fresh Reviewer 결과와 integration gate 연결 |
| [#64](https://github.com/Middleages/thread-dock/pull/64) | `fd10b7a` | Parent Issue Publisher 경계와 conflict evidence |
| [#65](https://github.com/Middleages/thread-dock/pull/65) | `390fde0` | `work publish-issues` production CLI 경로 |
| [#66](https://github.com/Middleages/thread-dock/pull/66) | `44abbed` | Wails Monitor, WSL aggregate client와 frontend |
| [#67](https://github.com/Middleages/thread-dock/pull/67) | `f12bf77` | project template의 plan/implement/review skills와 contract 검사 |

현재 Wails Monitor는 `internal/monitor.Snapshot`, `internal/monitorcli`의 WSL 호출, `monitor/App.GetMonitorSnapshot`과 React 작업 현황 화면까지 main에 있다. GitHub 링크·결정·handoff를 표시하지만 Herdr Session topology는 아직 wire/UI에 없다.

Parent Issue publication은 멱등 request와 ambiguous outcome 보존까지 구현됐다. 이는 GitHub Work Evidence module 근거이며 Session owner가 아니다. PR/Wiki 전체 publication과 실제 first-use evidence는 아직 남아 있다.

## 실제 Herdr 확인과 미해결 문제

이 문서 작업 환경에서 `HERDR_ENV=1`, `herdr 0.8.2`와 다음 read-only command surface를 확인했다.

- `herdr session list --json`
- `herdr workspace list`
- `herdr tab list --workspace <id>`
- `herdr pane list --workspace <id>`
- `herdr agent list`
- `herdr worktree create|open`, `tab create`, `pane split`, `agent start|attach|focus`

현재 Session에서 여러 Workspace와 Top-level Codex/OpenCode가 관찰됐지만 opaque ID, socket path와 native session ID는 문서 근거에 복사하지 않았다. 이 조회는 Monitor adapter나 Windows Wails 동작 검증이 아니다.

[실행/권한 점검 기록](docs/operator/herdr-builder-capability-preflight.md)과
[Herdr 시작 readiness 보고서](docs/operator/herdr-opencode-startup-readiness.md)의 기존 근거는 보존한다.

- Herdr 0.8.2와 임시 0.9.0의 시작 직후 Start→Get→Prompt에서 `agent_prompt_stalled`가 관찰됐다.
- [Herdr #3813](https://github.com/herdrdev/herdr/issues/3813)은 해결됐다고 확인하지 않았다.
- stalled/blocked/unknown은 Monitor와 skill에서 fail-visible로 남기며 blind retry하지 않는다.
- attach 또는 수동 prompt fallback은 Operator가 실제로 선택한 뒤에만 수행한다.
- 기존 `/tmp/threaddock-herdr-live.7YEhfV`의 보존 invocation/worktree는 조회·재승인·재예약·prompt·종료·재사용하지 않는다.
- 새 session/process registry나 readiness detector를 ThreadDock에 추가하지 않는다.

## Superseded Codex runtime 실험

`agent/codex-runtime`은 main `f12bf77…`에서 갈라져 다음 accepted-but-unmerged 실험 commit을 보존한다.

- `d848f94` atomic runtime dispatch seam
- `1e460a3` ephemeral Codex runtime adapter
- `3b9db40` independent Codex adapter evidence까지의 reviewed 결과

worktree `/home/appuser/dev_system/.worktrees/codex-runtime`에는 2026-09-10 확인 시 다음 **uncommitted production wiring**이 있다.

- `cmd/agentctl/main.go`
- `cmd/agentctl/main_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`

이 branch와 dirty 변경은 Herdr-first workflow에 의해 **superseded experiment / paused wiring**으로 분류한다. 삭제, reset, commit, merge하지 않았다. atomic/adapter 코드는 나중에 optional evidence module로 재평가할 수 있지만 새 Monitor의 선행 조건이 아니며 새 Codex runtime wiring을 계속하지 않는다.

## 새 구현 순서

새 [구현 계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md)의 Task를 직렬로 실행한다.

1. **Herdr status adapter + aggregate wire**: `agentctl monitor snapshot --json`, partial failure와 secret-free public types.
2. **Monitor Session/Agent view**: GitHub Work 상세를 기본으로 유지하고 record/local state와 각 observedAt, stale/offline/missing/conflict를 분리해 표시.
3. **project skill**: explicit user action으로 existing Agent focus/attach/handoff prompt, blocked inspect-first, missing/ended의 승인된 Worktree/Tab/Pane과 새 Agent start를 수행. completed Task는 prompt에서 제외하고 Internal Subagent는 top-level native 기능에 맡김.
4. **controlled actual pilot**: 새 이름과 새 topology만 사용하고 stalled 상태를 숨기거나 재전송하지 않음.
5. **GitHub evidence continuation**: Parent Issue, 한국어 PR, docs/handoff, human merge receipt를 한 Work Item으로 연결.

기존 Go `state/v2`, verification, Git inspection과 Publisher는 GitHub/검증 receipt에 직접 가치가 있을 때만 선택적으로 쓴다. 대화형 Session·Pane·Agent lifecycle을 소유하게 확장하지 않는다.

## 문서 Task ledger

```yaml
taskId: herdr-first-design
baseSHA: f12bf77c5755ef16c3d93ffcb4403f710a374cda
deps: []
ownedPaths:
  - CONTEXT.md
  - PRODUCT.md
  - HANDOFF.md
  - docs/adr/0007-herdr-owns-interactive-execution-topology.md
  - docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md
  - docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md
  - docs/superpowers/plans/2026-09-07-implementation-readiness.md
  - docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md
worktree: /home/appuser/dev_system/.worktrees/herdr-first-design
branch: agent/herdr-first-design
forbiddenPaths:
  - go.mod
  - go.sum
  - cmd/**
  - internal/**
  - monitor/**
  - project-template/**
interface: Herdr/GitHub/ThreadDock ownership terms and serial implementation plan
acceptance:
  - canonical terms distinguish Herdr Session/Workspace/Tab/Pane, Top-level Agent, Internal Subagent and ThreadDock Work Evidence
  - PRODUCT and ADR assign live topology to Herdr and durable evidence to GitHub
  - Monitor binds exact Work/repository/canonical Worktree and keeps record/local state separate
  - resume rules cover existing, blocked, missing, ended and exclude completed Tasks
  - focused approved design and no-placeholder five-Task plan exist
  - superseded Codex experiment and dirty wiring are preserved without mutation
tests:
  - repository-relative Markdown link existence
  - plan header and required packet field scan
  - placeholder scan
  - git diff --check
result:
  status: validated_awaiting_fresh_review
  changedFiles:
    - CONTEXT.md
    - HANDOFF.md
    - PRODUCT.md
    - docs/adr/0007-herdr-owns-interactive-execution-topology.md
    - docs/superpowers/plans/2026-09-07-implementation-readiness.md
    - docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md
    - docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md
    - docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md
  commitSHA: null
  executedCommands:
    - herdr --version; herdr --help; herdr --skill; read-only session/workspace/tab/pane/agent lists
    - repository-relative Markdown link existence check
    - plan header, five packet and required field count scan
    - placeholder scan
    - git diff --cached --check
  outcomes:
    - Herdr 0.8.2 read-only command surface confirmed; no control command or prompt executed
    - relative links PASS for eight changed Markdown files
    - plan header, five complete Task packets and placeholder scan PASS
    - staged diff check PASS
    - Go tests and make check intentionally not run for docs-only change
  unverified:
    - actual resolved Sol model and medium reasoning telemetry
    - Windows Wails and live pilot behavior
  blockers: []
```

최초 spawn은 `td_coordinator` 역할상 Sol medium으로 배정됐지만 실제 resolved model/effort telemetry를 확인할 interface가 없어 `unverified`다. 이 역할 설정은 제품 Execution Profile이 아니다.

## 검증·리뷰 규칙

- 이 branch는 docs-only이므로 repository-relative link 검사, plan/packet/placeholder scan과 `git diff --check`만 실행한다. Go test와 `make check`는 실행하지 않는다.
- fresh `td_reviewer`는 fixed commit SHA에서 새 설계, ADR, CONTEXT/PRODUCT/HANDOFF 일치와 plan 실행 가능성을 검토한다.
- code Task는 focused test만 수행하고 동일 SHA·command·환경 증거를 반복하지 않는다. 마지막 integration code PR에서만 `make check`를 한 번 실행한다.
- blocking 사항은 완료로 표시하지 않는다. main merge는 사람에게 남긴다.

## 다음 세션 시작 prompt

> 최신 main에서 AGENTS.md, HANDOFF.md, CONTEXT.md, PRODUCT.md, `docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md`, `docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md`를 읽어라. Herdr가 Session/Workspace/Tab/Pane과 Top-level Agent를 소유하고, 각 Top-level Agent가 native Internal Subagent를 소유하며, ThreadDock은 GitHub Work Evidence와 live Herdr 상태를 결합하는 얇은 Monitor라는 경계를 유지하라. 계획의 Task 1 `herdr-first-01-status-wire`만 최신 main에서 독립 worktree로 구체화하고 Luna high에 구현·focused test·self-review를 배정한 뒤 fixed SHA를 fresh Sol medium이 review하게 하라. 새 Codex runtime wiring, blind prompt retry, Internal Subagent registry를 추가하지 말고 Herdr #3813이 해결됐다고 주장하지 마라. 제품 code PR의 main merge는 사람에게 남겨라.
