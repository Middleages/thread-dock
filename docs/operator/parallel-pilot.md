> **옛 실행 엔진의 역사 자료.** 이 문서의 agentctl·pilot·runtime 절차는 현재 제품 실행 방법이 아닙니다. 소스 제거는 [완료 계획](../superpowers/plans/2026-09-12-engine-retirement-completion.md)에서 추적하며, 현재 사용법은 [GitHub-first 빠른 시작](github-first-quickstart.md)을 따릅니다. 과거 명령·경로는 재실행 지시가 아닌 당시 증거입니다.

# Parallel Orchestrator Pilot Runbook

This runbook is for the controller-led pilot of the multi-task orchestrator. It
is intentionally deterministic and must be executed only after review from a
frozen checkout; do not run a live GitHub pilot from a moving integration
checkout.

## Preconditions

1. Freeze the reviewed commit and record its full 40-character SHA. Create a
   disposable repository clone/worktree from that SHA. The pilot operator is
   the only person allowed to select the checkout, repository, GHES host, and
   credentials.
2. Confirm the contract with `agentctl contract validate CONTRACT` and record
   its JSON result. Record the contract path, run ID, parent Issue, child Issue
   numbers, repository/default branch, base SHA, task IDs/dependencies,
   allowed paths, protected paths, and exact verification commands.
3. Set `THREADDOCK_GH_TOKEN` only in the pilot process. Verify the token is
   absent from captured logs, terminal output, event payloads, and generated
   PR bodies. Run the repository secret scan before starting and after every
   blocked/repair story.
4. Project IDs and status option IDs may remain structurally present in the
   config for compatibility. They are not evidence of Project automation. If
   `projectAutomationEnabled` is false, record the explicit
   `project_automation_skipped` audit events and verify that no ProjectV2
   mutation was attempted. Enable ProjectV2 only with verified live IDs.
5. Record that cleanup is prohibited until the controller accepts all evidence
   below. Never remove a managed Worktree, delete run state, or force-reset a
   checkout to make a story look clean.
6. Session retirement is a separate, opt-in lifecycle. Do not run
   `agentctl retire RUN` or `agentctl cleanup RUN` against any accepted or
   diagnostic parallel-pilot RUN, Workspace, Worktree, state directory, or
   evidence path while evidence is awaiting acceptance. The retirement pilot
   must create a new disposable repository, Herdr Workspace, RUN, and
   isolated state root; follow
   [`session-retirement.md`](session-retirement.md) and record its exact
   SHA/RUN/Workspace/path evidence outside this pilot's accepted artifacts.
   In short: never retire and never clean an accepted or diagnostic RUN from
   this pilot.

## Six stories

Run every story with a fresh run ID. For each story, capture the exact run
snapshot and append-only event log, then record:

- Run ID, frozen checkout SHA, contract/base SHA, parent and child Issue links;
- integration branch, final PR link/number, PR head SHA, merge SHA, and exact
  check SHA observed;
- each stable `role/run/task` Agent name, provider or terminal execution
  identity, Workspace ID, pane ID, and canonical path;
- ordered task completion IDs, immutable Builder commit SHAs, Git inspection
  changed files/patch decision, verification commands/outcomes/durations;
- Reviewer decision/findings/risk categories, shared repair count, CI result,
  recovery count/fingerprint, Merge Gate decision, and whether operator
  confirmation was required; and
- secret-scan result and whether manual cleanup was needed (cleanup must be
  `no` while evidence is awaiting acceptance).

1. **Ordinary automatic merge.** Two independent Builder tasks dispatch within
   the two-slot limit, integrate in contract order, pass Wave End Verification,
   receive strict Reviewer evidence, create one Draft PR, observe checks for
   the final SHA, pass the Merge Gate, and merge. Main must remain unchanged
   until the final protected merge action.
2. **Shared repair exhaustion.** Cause one Reviewer block followed by two CI
   blocks (or any three shared Reviewer/CI blocks). Each repair must be a new
   Task packet, new strict evidence, and a new immutable commit before
   reintegration. The third block must end in `blocked` with no blind retry.
3. **Protected wait and confirmation.** Declare the valid contract
   `riskCategories` values (for example `authentication` or
   `public_contract`); do not broaden a Task's `allowedPaths` to make a
   protected story pass validation. Verify the PR contains exactly one
   secret-free `<!-- threaddock:RUN:protected-change -->` marker with the
   protected reasons before `needs_operator` is persisted. The former
   `agentctl confirm RUN protected-change` CLI is retired; do not execute it.
   Historically, its first invocation persisted intent and audit records,
   invalidated latest-main, Full Suite, check, and mergeability evidence, and
   resumed the refresh/recheck sequence. A second invocation was idempotent.
   The historical pilot merged only after the exact final SHA/check/
   mergeability gate passed.
   The orchestrator's protected-change state and tests remain as legacy
   implementation evidence; this story records the removed CLI's history.
4. **Git conflict.** Make immutable integration report a confirmed merge
   conflict. Verify the run is `blocked`, conflict evidence is recorded, and
   there is no reset, automatic conflict resolution, push, or main merge.
5. **Three recoveries.** Stall an eligible agent with no changed durable
   fingerprint. Verify the durable fingerprint includes commit SHA, the
   Builder Worktree Git fingerprint, sorted completed task IDs, and
   verification evidence. Three recovery actions may complete; the fourth
   unchanged evaluation blocks. Live stale/blocked/unknown and absent
   terminal-only identities are operator-owned, and terminal fallback never
   enables native resume.
6. **Historical revert story.** The former pilot included
   `agentctl create-revert RUN --reason "pilot regression"` to exercise the
   immutable merge state and draft Revert PR path. The command and surrounding
   revert procedure are retained here only as historical context; `create-revert`
   is removed from the current CLI and must not be executed as a current pilot
   step.

## Acceptance and cleanup

The controller compares every captured value with the persisted evidence and
the frozen checkout before accepting the pilot. A changed final SHA invalidates
old checks and requires the latest-main/full-suite/push/check sequence again.
Do not remove Worktrees or run cleanup until the controller explicitly accepts
the story evidence; after acceptance, use only the safe, clean-worktree cleanup
path and record each removal.

## Retirement boundary

The retirement runbook is a disposable local probe, not an additional story
for this accepted parallel run. It exercises `agentctl retire RUN`,
`agentctl status RUN --json`, `agentctl resume RUN`, and (only in a copied
state fixture older than seven days) `agentctl cleanup RUN`. It verifies that
Herdr Workspaces are absent after close while Git Worktree paths,
`run.json`, append-only events, and structured evidence remain until cleanup.
The probe's secret scan must pass. Its result must state that no accepted or
diagnostic RUN was retired or cleaned; raw transcript and credentials never
belong in the result.
