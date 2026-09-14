---
name: herdr-local
description: Use when coordinating or developing work requires inspecting or controlling a local Herdr session, workspace, tab, pane, or Agent.
---

# Herdr local

This is a local-only policy reference for the installed Herdr executable (Herdr 0.8.2 is the observed baseline). It is a helper for the six canonical workflows, not a seventh workflow. Read it before every Herdr CLI operation.

## Gate every operation

1. Establish local syntax before any live access with the installed executable's `--help` and `--skill` output:

   ```text
   herdr --help
   herdr --skill
   ```

   Do not use bare `herdr` for discovery, call a mutating subcommand without arguments as help, or fetch external documentation. If local help is insufficient, use checked-in template guidance or installed man, completion, or source documentation; otherwise stop and report syntax unavailable.
2. Before state or control, test whether the managed pane actually reports `HERDR_ENV=1`. Never export or inject it. If it is absent or unavailable, report Herdr state as unknown and stop.
3. Establish identity from a successful local read-only result with actual JSON. Use only exact session, workspace, tab, pane, and Agent IDs returned there; use returned cwd and worktree paths as paths, not guessed IDs. Cached locators and guessed values are not identity.
4. Default to read-only inspection. A session, layout, or Agent action requires an explicit approved operation, exact IDs, bounded arguments, and evidence that it preserves unrelated sessions, worktrees, and the shared locator. Keep native subagents inside the feature session; do not create Herdr panes or registry entries for them.
5. Treat an executable on the local PATH as insufficient egress evidence. Agent start or prompt, session-start hooks, pane run, and send-keys execution require evidence that the provider, fallback, plugin, MCP, and tool path are approved internal-only. A localhost or private hostname alone is not proof. Unknown means stop; do not probe endpoints or contact them to test.

## Closed-network boundary

This local-only scope excludes update, download, install, remote SSH (including internal SSH), external URLs, uploads, remote transfers, and equivalent work hidden behind a shell, helper, or Agent. Do not bypass policy or select models automatically, and never disclose credentials. GitHub publication remains the responsibility of the other skills, but this boundary carries through delegation: use an approved internal path only; otherwise leave the work pending for handoff. Policy guidance is not proof that a firewall prevents egress.

After any permitted observation or action, record the exact command, IDs, sanitized relevant output, and evidence boundary. Remove credentials and other secrets from recorded output. If any gate is missing, report the blocker instead of testing by contact or filling gaps from memory.
