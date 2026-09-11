---
name: open-agent-session
description: Use when an existing prompt still asks for the legacy ThreadDock Herdr session-opening skill.
---

# Open agent session compatibility

New ThreadDock coordinators use **REQUIRED SUB-SKILL:** `coordinate-work`, which owns exact Herdr session reuse/start and shared locator upsert.

Never guess Herdr session, workspace, tab, pane, Agent name, worktree, or cwd values. Inspect the installed Herdr tool and actual session state first.

Keep one user-level `~/.threaddock/sessions.json` across projects. Preserve unrelated bindings and use the shared lock/re-read/atomic-write rules defined by `coordinate-work` and `publish-work`.
