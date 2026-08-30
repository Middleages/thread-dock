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
3. **Protected wait and confirmation.** Change a file under a protected path
   (for example `authentication/`). Verify `needs_operator` contains the
   canonical protected reason and no merge call occurs. Run
   `agentctl confirm RUN protected-change` twice; the first persists the intent
   and audit record and resumes, while the second is idempotent. Merge only
   after the exact final SHA/check/mergeability gate is true.
4. **Git conflict.** Make immutable integration report a confirmed merge
   conflict. Verify the run is `blocked`, conflict evidence is recorded, and
   there is no reset, automatic conflict resolution, push, or main merge.
5. **Three recoveries.** Stall an eligible agent with no changed durable
   fingerprint. Verify the durable fingerprint includes commit SHA, managed
   Git fingerprint, sorted completed task IDs, and verification evidence. The
   third recovery blocks; live stale/blocked/unknown and absent terminal-only
   identities are operator-owned, and terminal fallback never enables native
   resume.
6. **Revert the ordinary merge.** On the completed ordinary run, execute
   `agentctl create-revert RUN --reason "pilot regression"`. Verify the command
   uses the exact persisted merge SHA and completed state, creates a deterministic
   managed revert branch and Draft Revert PR, and leaves main unchanged. Record
   the Revert PR link/number, branch, merge SHA, reason, and secret-scan result.

## Acceptance and cleanup

The controller compares every captured value with the persisted evidence and
the frozen checkout before accepting the pilot. A changed final SHA invalidates
old checks and requires the latest-main/full-suite/push/check sequence again.
Do not remove Worktrees or run cleanup until the controller explicitly accepts
the story evidence; after acceptance, use only the safe, clean-worktree cleanup
path and record each removal.
