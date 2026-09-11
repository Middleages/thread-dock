---
name: record-work
description: Use when an existing prompt still asks for the legacy ThreadDock work-recording skill.
---

# Record work compatibility

Use **REQUIRED SUB-SKILL:** `publish-work` for PR, Issue handoff, Project, durable docs/Wiki, and shared ThreadDock locator reconciliation.

Preserve the old safety guarantees: use observed GitHub state, never guess Project field values, keep per-board failures distinct, and never treat `idle`, `done`, or a missing Agent as sufficient evidence for locator cleanup.

Do not maintain a separate publication protocol under this legacy name.
