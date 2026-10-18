---
name: agent-wait
description: Use only by long-lived td_coordinator or td_feature_leader agents while delegated work is still outstanding, so they remain active without busy-polling or duplicating execution state.
---

# Agent wait

Wait on the mechanism that owns the delegated work. This Skill is a parent-agent waiting discipline, not a scheduler or cache keepalive service.

Only `td_coordinator` and `td_feature_leader` should use this Skill. Leaf implementers, reviewers, explorers, and docs agents must finish their bounded task and return instead of waiting for unrelated work.

While waiting:

1. For native children, use the harness's native result/mailbox wait. For a working bound Herdr top-level Agent, use `herdr agent wait <exact-target> --timeout <milliseconds>`; a newly authorized prompt may combine submission with `agent prompt ... --wait`. Follow `herdr-local` first. Waiting does not authorize sending a new prompt.
2. Use bounded timeouts within the harness's responsiveness limits; yield a running tool when supported rather than blocking user input. With multiple bound Agents, observe each fairly instead of waiting indefinitely on one. If native waiting is unavailable, use a bounded harness wait followed by a scoped read, not a tight polling loop or a universal 20-25 minute timer.
3. On wake, collect only owned results or blockers: Feature Leaders collect native child results; Coordinators inspect bound top-level sessions and corresponding GitHub evidence, not Feature Leader leaf agents. Herdr settled states (`idle`, `done`, `blocked`) are observations, not proof that the requested turn, background children, or GitHub work finished. An already-settled target can return immediately; reconcile it before waiting again, and use the bounded fallback if work remains without a new activity signal.
4. A timeout or stalled prompt does not prove non-delivery. Inspect the exact target and evidence before any retry; do not resubmit blindly or answer an approval dialog. If nothing materially changed, wait without rerunning tests, rereading the repository, rewriting plans, or emitting keepalive chatter.
5. Stop when all required results are available, human input is required, execution is materially blocked, or the parent work is complete. Report an observation failure as unknown, not completion.

When a Herdr CLI operation is required, obey the repository's `herdr-local` gate and use only values returned by the installed tool. Never invent session, workspace, tab, pane, Agent, or cwd identifiers.
