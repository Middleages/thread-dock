---
name: publish-work
description: Use when a feature or coordinator must reconcile completed, blocked, or waiting work into GitHub records and ThreadDock local locator state.
---

# Publish work

Make durable records match observed state. GitHub is work truth; `~/.threaddock/sessions.json` is only local execution locator context.

Keep detailed verification in one current primary record: the PR when present, otherwise the Issue/request. Link each original acceptance criterion to implementation/check evidence or its missing item; include candidate revision, command/action, environment, outcome and independent review reference. A short list is enough for small work. Other records summarize and link here. If external writing is unauthorized, use a local/conversational record within scope.

Explain the result to the user as well as recording it: what problem/work was requested, what changed from the user's perspective, why this approach and its meaningful tradeoff, and the actual checks with limits. State completed, waiting, or blocked accurately; include the next action/owner only when something remains. Scale this to the change (a typo may need one sentence), and link existing evidence instead of duplicating a long checklist. A diff or list of commands alone is not the completion explanation.

For a feature, update the PR and linked Issue handoff. Keep the current handoff concise: current result, decisions with reasons/source revisions, remaining work and next owner/action, plus the primary evidence link. When a decision changes, identify what it supersedes and affected features' responses; preserve history without presenting an old summary as current. Discover relevant GitHub Projects and actual field/options; keep unavailable-board updates pending while the Issue/PR flow continues.

Update Wiki or durable repository docs only when the change creates usage, operating, recovery, or architectural knowledge worth preserving. Do not create a permanent docs workflow for routine implementation chatter.

If the discovered Project has no usable field or option for the requested update, leave that update pending with the observed reason rather than inventing an option. When a write result is unclear, read back the target once before deciding whether a retry is needed; keep the original diagnostic context and avoid duplicate writes.

For ThreadDock locator changes, acquire the shared user-level lock, re-read the full locator inside the lock, mutate only the exact coordinator/feature binding, write a temporary file, and atomically rename it. Preserve all unrelated projects. If the lock cannot be acquired safely, leave the locator unchanged and report the update pending.

Never delete a binding only because an Agent is `idle`, `done`, or temporarily missing. Feature cleanup requires all three: Issue closed, related PR work finished with no remaining handoff, and top-level Herdr feature session confirmed absent. Coordinator cleanup requires an explicit decision that the project is no longer managed.

Do not auto-merge. Human merge remains the final transition.
