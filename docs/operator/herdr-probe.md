# Herdr v0.8.2 probe

Probe date: 2026-08-28 (Asia/Seoul). Binary: `herdr` (v0.8.2).
The authoritative fresh read-only probe run completed in about `0.1 s`; every
command exited 0. No mutating Herdr command was run during that probe.

## Version and command contracts

```text
$ herdr --version
herdr 0.8.2

$ herdr worktree create --help
Create and open a Git worktree

Usage: herdr worktree create [OPTIONS]

Options:
      --workspace <ID>
      --cwd <PATH>
      --branch <NAME>
      --base <REF>
      --path <PATH>
      --label <TEXT>
      --focus
      --no-focus

$ herdr pane list --help
List panes

Usage: herdr pane list [OPTIONS]

Options:
      --workspace <WORKSPACE_ID>

$ herdr agent start --help
Start a supported interactive agent in an existing pane

Usage: herdr agent start <NAME> --kind <KIND> --pane <ID> [OPTIONS] [-- [AGENT_ARG]...]

Options:
      --kind <KIND>
      --pane <ID>
      --timeout <MS>

next: herdr agent prompt <TARGET> <TEXT> --wait

$ herdr agent prompt --help
Submit a prompt to an agent

Usage: herdr agent prompt <TARGET> <TEXT> [OPTIONS]

Options:
      --wait
      --until <STATUS>
      --timeout <MS>

$ herdr agent get --help
Show an agent

Usage: herdr agent get <target>

$ herdr agent read --help
Read agent terminal output

Usage: herdr agent read <TARGET> [OPTIONS]

Options:
      --source <SOURCE>
      --lines <N>
      --format <FORMAT>
      --ansi
```

The relevant captured help output was inspected during implementation; the
adapter relies only on the options shown above and does not infer undocumented
output.

## Read-only workspace evidence

Command: `herdr pane list --workspace <workspace-id>` (exit 0). This is the
actual response shape from the Builder Luna worktree. Live identifiers, title
and path values are redacted below.

```json
{"id":"cli:pane:list","result":{"panes":[{"agent":"codex","agent_session":{"agent":"codex","kind":"id","source":"herdr:codex","value":"session-redacted"},"agent_status":"working","cwd":"/redacted/worktree","focused":false,"foreground_cwd":"/redacted/worktree","pane_id":"pane-redacted","revision":2,"scroll":{"max_offset_from_bottom":43,"offset_from_bottom":0,"viewport_rows":50},"tab_id":"tab-redacted","terminal_id":"terminal-redacted","terminal_title":"redacted-title","terminal_title_stripped":"redacted-title","workspace_id":"workspace-redacted"}],"type":"pane_list"}}
```

Command: `herdr agent get <agent-name>` (exit 0).

```json
{"id":"cli:agent:get","result":{"agent":{"agent":"codex","agent_session":{"agent":"codex","kind":"id","source":"herdr:codex","value":"session-redacted"},"agent_status":"working","cwd":"/redacted/worktree","focused":false,"foreground_cwd":"/redacted/worktree","interactive_ready":true,"name":"agent-redacted","pane_id":"pane-redacted","revision":2,"state_change_seq":110,"tab_id":"tab-redacted","terminal_id":"terminal-redacted","terminal_title":"redacted-title","terminal_title_stripped":"redacted-title","workspace_id":"workspace-redacted"},"type":"agent_info"}}
```

## Worktree-create provenance

Source command: `herdr worktree create --cwd <redacted-path> --branch
agent/6-herdr-adapter --base agent/2-integration --label issue-6-herdr-adapter
--no-focus`. It exited 0, was controller-authorized, and completed in about
`0.1 s`. The response was supplied as the exact successful v0.8.2 response from
this Worktree and is preserved in the fixture with its wire field names and
relationships.

Redaction procedure: replace absolute paths with `/redacted/...`, replace
workspace/pane/tab/session/terminal IDs with relationship-preserving labels,
and replace terminal titles and agent names only where they identify the live
session. Keep booleans, numbers, field names and nesting unchanged. The raw
output fixture retains only representative line/text shape and no live output.

## Adapter contract and verification

The adapter invokes Herdr through `internal/runner.Runner` with one argument
per slice element:

```text
worktree create --cwd REPO --branch BRANCH --base BASE --label LABEL --no-focus
pane list --workspace WORKSPACE_ID
agent start NAME --kind opencode --pane PANE_ID
agent prompt NAME PACKET --wait --timeout 3600000
agent get NAME
agent read NAME --source recent-unwrapped --lines 120
```

`working`, `blocked`, `idle`, `done` and `unknown` are lifecycle states only.
They are not completion proof; commit and verification evidence remain a
separate concern.
