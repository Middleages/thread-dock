---
name: agent-wait
description: Use only by long-lived td_coordinator or td_feature_leader agents while delegated work is still outstanding, so they remain active without busy-polling or duplicating execution state.
---

# Agent wait

Use the harness's native wait mechanism while owned delegated work is still outstanding. This Skill is a parent-agent waiting discipline, not a ThreadDock scheduler, timer service, lease engine, or product runtime.

Only `td_coordinator` and `td_feature_leader` should use this Skill. Leaf implementers, reviewers, explorers, and docs agents must finish their bounded task and return instead of waiting for unrelated work.

While waiting:

1. Do not leave the parent inactive for roughly 25 minutes while it still owns outstanding delegated work. Prefer a native wait interval of about 20-25 minutes rather than relying on a 30-minute boundary.
2. On wake, inspect only the state needed for owned work.
   - Feature Leader: collect its native child results, failures, or blockers.
   - Coordinator: inspect its bound top-level Herdr Feature Leader sessions and corresponding GitHub evidence; do not reach into Feature Leader leaf agents.
3. If nothing materially changed, wait again. Do not rerun tests, reread the repository, rewrite plans, or emit status chatter merely to stay active.
4. Treat `idle` or a temporary observation failure as observation state, not completion. A Herdr `done` observation does not by itself mean GitHub work is complete.
5. Stop waiting when all required delegated results are available, human input is required, execution is materially blocked, or the parent work is actually complete.

When a Herdr CLI operation is required, obey the repository's `herdr-local` gate and use only values returned by the installed tool. Never invent session, workspace, tab, pane, Agent, or cwd identifiers.
