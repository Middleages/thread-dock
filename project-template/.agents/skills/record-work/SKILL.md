---
name: record-work
description: Use when an agent needs to record implementation, review, waiting, or handoff state in GitHub and project documentation.
---

# Record work

Make the durable record match observed state. Use `gh` or GitHub MCP with the session's existing authorization. Update Issue progress and/or the handoff with the rationale, acceptance progress, decisions, changed paths, commit/PR URL, checks and outcomes, review decision, blocker, and next owner. Discover every relevant Project membership, read each board schema and its actual status, priority, and waiting options, and update each board with the returned option IDs or names; never guess them. Keep per-board failures distinct. If one or all Projects are unavailable, preserve the working Issue and PR flow and record each missing board update as its own pending item in the handoff.

Record docs and Wiki changes through the repository's normal GitHub workflow. Keep the Wiki current with usage and runbook instructions, including how to recover a stopped session and where to find the feature handoff. If the Wiki is unavailable, draft the page in the PR and mark publication pending. Link the PR, Issue, Project item, and Wiki page when they exist.

If a write result is ambiguous, look up the Issue, Project item, PR, or Wiki page and reconcile from the observed state before retrying. Do not dump routine transcripts. Quote only the short command output needed to explain a decision. Preserve blockers and unverified checks instead of turning them into success. Human merge remains the final transition.
