---
name: open-agent-session
description: Use when a coordinator must reuse or start an approved Herdr feature session and record its explicit locator.
---

# Open agent session

Use the installed Herdr `--skill` or `--help` output and the commands it documents. Reuse an existing approved feature session when its locator is known; otherwise start the exact approved feature session with a bounded initial prompt containing the Issue URL, repository, feature scope, acceptance criteria, worktree, reporting cadence, and stop conditions. Never invent a Herdr command or blindly resend a prompt to a stalled session. A stopped session resumes from the latest Issue handoff and local notes, after checking its current state.

Handle session state compactly: when it is working, focus or read its current output without sending another prompt; when it is blocked, inspect the question and obtain the needed answer or approval; when a start or prompt result is ambiguous, inspect the existing session before creating a duplicate or resending.

Native subagents used by a feature leader are part of that session's work and are not separate panes or top-level sessions. A working observation is not completion: completion requires the Issue/PR handoff, checks, review result, and commit or explicit blocker.

Associate the Issue, session, pane, workspace, and worktree explicitly. Persist optional locators locally when practical in `.threaddock/sessions.json`; do not require the file or a monitor before starting work. Absolute worktree paths and Herdr IDs belong in this local file or a local handoff, not in mandatory public Issue content. The binding file is a locator, not a process registry:

```json
{"version":1,"bindings":[{"issueUrl":"https://github.com/OWNER/REPO/issues/123","repository":"github.com/OWNER/REPO","worktree":"/path/to/feature-worktree","session":"actual-session-name","workspaceId":"<actual-workspace-id>","tabId":"<actual-tab-id>","paneId":"<actual-pane-id>","agentName":"<actual-agent-name>","role":"feature"}]}
```

Use actual IDs returned by the CLI or connected tool for optional local fields. `role` is `feature` or `coordinator`. A coordinator binding may omit `issueUrl` and use `repository` or `projectUrl` instead. Public Issue content should carry stable GitHub links and feature scope; keep local session/worktree association in the optional locator or local handoff even when persistence is deferred.
