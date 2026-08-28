# Herdr v0.8.2 probe

Probe date: 2026-08-28 (Asia/Seoul). Binary: `/home/appuser/.local/bin/herdr`.
All commands completed with exit code 0; measured command duration was `0.00 s`
for each command using `/usr/bin/time -f 'duration=%e s'`. No mutating Herdr
command was run during this probe.

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

Command: `herdr pane list --workspace w5` (exit 0, `0.00 s`). This is the
actual response from the Builder Luna worktree; the live workspace, pane,
terminal, session and path values are retained here as probe evidence.

```json
{"id":"cli:pane:list","result":{"panes":[{"agent":"codex","agent_session":{"agent":"codex","kind":"id","source":"herdr:codex","value":"01a045b9-9f18-7ac0-9009-5aa5a852d357"},"agent_status":"working","cwd":"/home/appuser/.herdr/worktrees/dev_system/agent-6-herdr-adapter","focused":false,"foreground_cwd":"/home/appuser/.herdr/worktrees/dev_system/agent-6-herdr-adapter","pane_id":"w5:p1","revision":2,"scroll":{"max_offset_from_bottom":43,"offset_from_bottom":0,"viewport_rows":50},"tab_id":"w5:t1","terminal_id":"term_65a105c7932837","terminal_title":"⠼ agent-6-herdr-adapter","terminal_title_stripped":"agent-6-herdr-adapter","workspace_id":"w5"}],"type":"pane_list"}}
```

Command: `herdr agent get builder-6` (exit 0, `0.00 s`).

```json
{"id":"cli:agent:get","result":{"agent":{"agent":"codex","agent_session":{"agent":"codex","kind":"id","source":"herdr:codex","value":"01a045b9-9f18-7ac0-9009-5aa5a852d357"},"agent_status":"working","cwd":"/home/appuser/.herdr/worktrees/dev_system/agent-6-herdr-adapter","focused":false,"foreground_cwd":"/home/appuser/.herdr/worktrees/dev_system/agent-6-herdr-adapter","interactive_ready":true,"name":"builder-6","pane_id":"w5:p1","revision":2,"state_change_seq":110,"tab_id":"w5:t1","terminal_id":"term_65a105c7932837","terminal_title":"⠴ agent-6-herdr-adapter","terminal_title_stripped":"agent-6-herdr-adapter","workspace_id":"w5"},"type":"agent_info"}}
```

The successful create fixture in `internal/herdr/testdata/v0.8.2/` is the
provided exact v0.8.2 `worktree-create` response. The read-only fixtures retain
the same response envelope and use only stable fixture values where tests need
portable examples.

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
