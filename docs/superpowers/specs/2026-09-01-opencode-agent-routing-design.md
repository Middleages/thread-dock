# OpenCode Role Agent Routing Design

**Date:** 2026-09-01

**Status:** Accepted design

## Objective

Route ThreadDock's Builder and Reviewer runtime roles to named OpenCode Agents
without making ThreadDock a second authority for providers, models, variants,
or arbitrary OpenCode arguments.

OpenCode remains the sole authority for Agent definitions and their model
selection. ThreadDock records and reuses only the selected OpenCode Agent name
needed to keep one RUN's role identity stable across restart, repair, and
recovery.

## Role model

The product has three LLM roles and one non-LLM execution module:

- `threaddock-planner` is selected by the operator in OpenCode before a RUN.
  It interviews the operator, uses OpenCode's built-in `explore` subagent when
  code discovery is needed, and produces the approved contract. ThreadDock
  does not launch or supervise Planner.
- `threaddock-builder` is launched by the Go Orchestrator for each contract
  Task. Repair and recovery continue the same Builder Agent/session.
- `threaddock-reviewer` is launched in the independent Reviewer Workspace
  after integration.
- The Go Orchestrator is a deterministic state machine. It validates and
  schedules approved Tasks, limits concurrent Builders, integrates results,
  applies repair/recovery policy, drives PR/CI/Merge Gate, and retires
  sessions. It is not an LLM Agent and has no model.

The Planner owns semantic Task decomposition. The Go Orchestrator distributes
only Tasks already present in the approved contract; it does not invent or
reinterpret Task boundaries.

## Explicit non-goals

- No `executionProfile`, model, provider, variant, or OpenCode Agent field is
  added to contract version 1.
- ThreadDock does not create or edit OpenCode Agent definitions.
- ThreadDock does not install a Planner or Explorer Agent.
- ThreadDock does not accept arbitrary OpenCode command arguments.
- ThreadDock does not capture or promise an exact model across OpenCode Agent
  configuration changes.
- No Wails/desktop contract editor is included.
- No LLM Orchestrator role is introduced.

## Configuration interface

Local ThreadDock configuration gains one optional object:

```json
{
  "openCodeAgents": {
    "builder": "threaddock-builder",
    "reviewer": "threaddock-reviewer"
  }
}
```

Both fields are optional. An omitted object or empty field preserves current
behavior: ThreadDock starts OpenCode without `--agent`, so OpenCode chooses its
configured default Agent.

Configured names must be trimmed, at most 64 bytes, and contain only ASCII
letters, digits, `.`, `_`, or `-`. Validation is local and syntactic; config
loading does not invoke OpenCode or require an Agent definition to exist.
Unknown JSON fields remain rejected by the existing strict decoder.

ThreadDock deliberately has no `planner` field because it never launches the
Planner. The operator selects `threaddock-planner` in OpenCode, outside the RUN
lifecycle.

## Durable routing identity

`state.AgentEvidence` gains additive field:

```go
OpenCodeAgent string `json:"openCodeAgent,omitempty"`
```

When a new RUN creates its initial snapshot, the Orchestrator copies both
resolved local role names into durable evidence before any external action,
even though the Reviewer process starts later. Task-indexed Builders store it in
`snapshot.Tasks[id].Agent.OpenCodeAgent`; legacy single Builder and Reviewer
fields use the same `AgentEvidence` representation.

The persisted Agent name, not current config, is authoritative for every
later start reconciliation, native resume, repair, and recovery action in that
RUN. This pins the OpenCode role/persona but not the model behind it. Changing
the OpenCode Agent's model is allowed and does not make ThreadDock mark the RUN
as inconsistent.

Legacy snapshots omit the field and continue without `--agent`. A restart
must never retrofit a newly configured role name into an existing empty
snapshot.

## Herdr adapter interface

The narrow Herdr request values become:

```go
type StartAgentRequest struct {
    Name          string
    PaneID        string
    OpenCodeAgent string
}

type ResumeAgentRequest struct {
    Name          string
    PaneID        string
    SessionID     string
    OpenCodeAgent string
}
```

The adapter continues to construct an argument vector, never a shell string.
When `OpenCodeAgent` is empty, argv remains byte-for-byte compatible with the
current command. When present, start appends:

```text
-- --agent OPEN_CODE_AGENT
```

Native resume appends the same pinned Agent selection beside the existing
session argument:

```text
-- --session SESSION_ID --agent OPEN_CODE_AGENT
```

The adapter validates the name again before invoking Herdr. It never accepts
model, provider, variant, prompt, shell fragment, or arbitrary trailing args.

## Data flow

For a new RUN:

1. Production configuration parses optional Builder and Reviewer OpenCode
   Agent names.
2. Orchestrator dependencies receive those two local routing defaults.
3. Initial snapshot creation copies Builder routing into every Task/legacy
   Builder evidence and Reviewer routing into Reviewer evidence before any
   external action.
4. Every start/resume request is built from the snapshot value.
5. Repair and recovery reuse the original Builder evidence and therefore the
   same OpenCode Agent name.
6. Reviewer uses its separately persisted Reviewer Agent name.

Planner and built-in Explorer remain completely outside this data flow.

## Failure and safety behavior

- Invalid configured names fail config loading before state, Herdr, GitHub, or
  Git mutation.
- Invalid persisted names fail before a Herdr command is invoked.
- A syntactically valid name that OpenCode does not define is a provider start
  failure. Existing durable intent remains recoverable; no prompt is sent and
  provider output remains redacted by current CLI error policy.
- Builder and Reviewer routing cannot be swapped during a RUN by editing local
  config because subsequent actions read the snapshot.
- Raw OpenCode Agent definitions and resolved model names are never copied
  into contract, state, events, GitHub comments, or pilot evidence.

## Compatibility

- Contract version remains `1` with identical JSON shape.
- Existing ThreadDock config remains valid and retains current OpenCode
  default-Agent behavior.
- Existing snapshots decode with empty `OpenCodeAgent` and resume using legacy
  argv.
- Existing Herdr fakes compile after the additive request field and may ignore
  its zero value.
- OpenCode Agent configuration changes do not require a contract migration.

## Verification

Tests must establish:

1. Config accepts omitted, empty, and valid role names and rejects whitespace,
   overlong, Unicode, separators, and shell-shaped values.
2. Strict JSON still rejects unknown configuration fields.
3. Herdr start/resume argv is exact for both legacy-empty and configured Agent
   names, and invalid persisted names cause zero runner calls.
4. Parallel Builders persist and route the Builder Agent name independently
   for every Task.
5. Reviewer persists and routes the Reviewer Agent name.
6. Repair and recovery reuse the pinned Builder name after config changes.
7. A restarted RUN with empty legacy evidence does not adopt new config.
8. Single-run compatibility remains intact.
9. Full Go 1.27 race tests and `make check` pass.

## Rejected alternatives

### Contract execution profiles

Rejected because they duplicate OpenCode's Agent/model configuration, couple
approved work intent to local provider inventory, and require ThreadDock to
own model resolution and migration.

### Raw model or variant in ThreadDock config

Rejected because OpenCode already owns those values. ThreadDock needs only a
stable role routing name.

### Dedicated Explorer runtime role

Rejected because OpenCode already supplies an `explore` subagent and discovery
belongs before contract approval. Turning it into a RUN role would duplicate
work and permit late challenges to already-approved Task boundaries.

### LLM Orchestrator

Rejected because it would create a second decision authority beside the Go
state machine and weaken deterministic scheduling, crash reconciliation,
bounded repair/recovery, and auditability.
