---
name: publish-work
description: Use when a feature or coordinator must reconcile completed, blocked, or waiting work into GitHub records and ThreadDock local locator state.
---

# Publish work

Make durable records match observed state. GitHub is work truth; `~/.threaddock/sessions.json` is only local execution locator context.

For a feature, create or update the PR with the linked Issue, behavior changed, exact checks/outcomes, review result, blockers, and follow-up. Update the Issue handoff with verified results, important decisions, remaining work, and next owner. Discover every relevant GitHub Project and use its actual field/options; never guess status, priority, or waiting values. If one board is unavailable, keep the Issue/PR flow moving and report that board update separately as pending.

Update Wiki or durable repository docs only when the change creates usage, operating, recovery, or architectural knowledge worth preserving. Do not create a permanent docs workflow for routine implementation chatter.

For ThreadDock locator changes, acquire the shared user-level lock, re-read the full locator inside the lock, mutate only the exact coordinator/feature binding, write a temporary file, and atomically rename it. Preserve all unrelated projects. If the lock cannot be acquired safely, leave the locator unchanged and report the update pending.

Never delete a binding only because an Agent is `idle`, `done`, or temporarily missing. Feature cleanup requires all three: Issue closed, related PR work finished with no remaining handoff, and top-level Herdr feature session confirmed absent. Coordinator cleanup requires an explicit decision that the project is no longer managed.

Do not auto-merge. Human merge remains the final transition.
