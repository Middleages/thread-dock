---
status: accepted
date: 2026-09-10
---

# Herdr가 대화형 실행 topology를 소유한다

ThreadDock은 자체 Run·Invocation 상태 기계를 대화형 Agent의 주 제어면으로 확장하지 않는다. Herdr가 persistent Session·Workspace·Tab·Pane과 그 안의 Top-level Codex/OpenCode를 소유하고, 각 Top-level Agent가 native Internal Subagent를 조정한다. ThreadDock은 GitHub의 업무·결정·PR·문서 근거와 Herdr의 시점성 상태를 결합하는 Monitor·skill·선택적 evidence module로 남는다.

## Considered Options

- ThreadDock Go coordinator가 모든 Agent Invocation, 재시도와 생명주기를 소유하면 결정적 자동화는 강해지지만 Herdr와 native subagent의 상태 기계를 중복하고 첫 실제 사용을 늦춘다.
- Herdr만 사용하면 live 멀티 세션은 해결되지만 프로젝트를 다시 열 때 GitHub 업무 목적·결정·검증·문서로 돌아가는 문맥 회수가 약하다.
- 선택한 구조는 Herdr의 실행 topology와 GitHub의 영속 업무 기록을 각각 원본으로 두고 ThreadDock이 읽기 중심으로 연결한다.

## Consequences

- Monitor는 Top-level Session·Pane과 요약 evidence를 보여주며 Internal Subagent를 일대일 추적하지 않는다.
- Monitor는 Work/repository/canonical Worktree의 exact binding으로 기록 상태와 로컬 실행 상태를 결합하되 각각의 observedAt·stale/offline/missing/conflict를 보존한다. 한 원본에서 다른 원본의 running/completed를 추정하지 않는다.
- 기존 Go state·verification·publisher는 삭제하지 않고 선택적 evidence module로 유지하지만 Session owner로 사용하지 않는다.
- Session/Workspace/Tab/Pane 조작은 Herdr CLI를 따르는 명시적 project skill로 제한한다. Monitor는 workflow engine이나 Herdr 대체 control plane이 아니다.
- Herdr `agent_prompt_stalled` 또는 관찰 실패는 보이는 blocker로 남기고 blind retry하지 않는다. attach나 prompt 재시도는 Operator가 직접 선택한다.
- 재개는 기존 Agent focus/attach, blocked 상태 inspect-first, missing/ended 상태에서 승인된 Worktree와 최신 handoff로 새 Top-level Agent 시작을 사용한다. 완료 Task는 다시 실행하지 않는다.

승인 근거는 2026-09-10 사용자의 “Herdr가 panes/tabs/worktrees/sessions를 소유하고 각 top-level Codex/OpenCode가 native subagents를 사용한다”는 결정과 “멀티 세션과 서브 에이전트가 잘 잡힌 구조에서 GitHub 중심 모니터링”이라는 제품 의도다. 이 결정은 [Herdr-first 첫 사용 설계](../superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)에서 구체화한다.
