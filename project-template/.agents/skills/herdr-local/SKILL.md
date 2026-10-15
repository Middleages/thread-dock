---
name: herdr-local
description: Use when coordinating or developing work requires inspecting or controlling a local Herdr session, workspace, tab, pane, or Agent.
---

# Herdr local

Use the installed Herdr executable to inspect or control local sessions. This helper supports the six canonical workflows; it does not set a network policy for GitHub or model providers. Read it before using Herdr in a workflow and reuse still-valid observations within that session.

## Local operation

1. Establish local syntax before any live access with the installed executable's `--help` and `--skill` output:

   ```text
   herdr --help
   herdr --skill
   ```

   Do not use bare `herdr` for discovery, call a mutating subcommand without arguments as help, or fetch external documentation. If local help is insufficient, use checked-in template guidance or installed man, completion, or source documentation; otherwise stop and report syntax unavailable.
2. Before state or control, test whether the managed pane actually reports `HERDR_ENV=1`. Never export or inject it. If it is absent or unavailable, report Herdr state as unknown and stop.
3. Establish identity from a successful local read-only result with actual JSON. Use only exact session, workspace, tab, pane, and Agent IDs returned there; use returned cwd and worktree paths as paths, not guessed IDs. Cached locators and guessed values are not identity.
4. Inspect read-only first. Perform session, layout, or Agent actions within the user's already-authorized task using exact existing target IDs and bounded arguments. Creation returns new IDs; read them back before subsequent control. Preserve unrelated sessions, worktrees, and locator entries. Keep native subagents inside the feature session.

## Network constraints

Use the user's and environment's actual network restrictions. Ordinary authorized work may use the configured GitHub service and model provider; local Herdr operation does not require an internal-only certification or renewed permission for an already-authorized action. Herdr's local executable does not imply that its Agents communicate locally.

When an explicit offline, internal-only, or approved-endpoint restriction applies, carry that restriction into delegated work. Check the relevant configured route only as needed for the proposed action (including hooks or tools it will invoke). If that route is unknown or conflicts with the restriction, leave that action pending with the specific missing fact; continue independent permitted work. A private hostname alone does not establish compliance. Do not test a prohibited endpoint by contact or bypass a restriction via another tool, shell, or Agent.

This helper does not install/update Herdr, operate remote SSH sessions, change providers/models/permissions, or establish a firewall. GitHub publication belongs to the calling workflow under its existing authorization.

Record commands, actual IDs, sanitized relevant output, and limitations. Report unavailable syntax, managed context, or target identity for the affected operation rather than guessing or exposing credentials.
