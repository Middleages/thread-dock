---
status: accepted
---

# 실행 세션 은퇴와 실행 근거 정리를 분리한다

ThreadDock의 Herdr Agent는 RUN 복구와 repair를 위해 독립 Workspace에서 실행된다. 작업이 끝난 뒤 `idle` 또는 `done` terminal을 계속 남기면 Operator의 활성 세션 목록이 누적되지만, 기존 `cleanup`은 Worktree와 run state까지 제거하므로 terminal 정리 수단으로 사용할 수 없다.

완료 RUN은 에이전트 Workspace를 먼저 은퇴시키고 Worktree, snapshot, event log와 GitHub 기록은 기존 보존 기간 동안 유지한다. `blocked` RUN은 forensic 가치가 있으므로 명시적인 Operator 요청이 있을 때만 은퇴한다. `needs_operator`와 진행 중 RUN은 은퇴하지 않는다. Codex subagent는 상위 대화에 종속되고 WSL 재시작·Operator 관찰·동일 RUN 복구의 durable identity가 없으므로 ThreadDock runtime Agent를 대체하지 않는다.

Herdr 0.8.2 probe에서 `workspace close` 후 Workspace ID가 사라져 `worktree remove --workspace`가 실패하고 Git checkout은 남는 것을 확인했다. 따라서 은퇴 전에 repository common directory, canonical path, branch와 HEAD를 durable proof로 보존한다. 이후 cleanup은 configured Herdr Worktree root containment, registered Git Worktree, repository identity, branch, HEAD와 clean 상태를 모두 재검증한 뒤 non-force `git worktree remove`를 사용한다.

이 선택은 terminal 누적을 빠르게 해소하면서 실행 근거 보존과 강제 삭제 금지를 유지한다. 대가로 retirement와 cleanup에 서로 다른 adapter와 reconciliation 상태가 필요하며, 닫힌 Workspace의 terminal 화면은 다시 열 수 없다. raw terminal은 완료 근거가 아니므로 structured evidence와 Git/GitHub 기록을 기준으로 삼는다.
