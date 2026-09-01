# OpenCode Role Agent Routing

ThreadDock routes the Builder and Reviewer role names to OpenCode. OpenCode is the model authority: Agent definitions, their instructions, provider choices,
and models live in OpenCode. ThreadDock is the name router: it records the
configured OpenCode Agent name in a new RUN snapshot and passes that name to
Herdr. The Go Orchestrator has no model and does not resolve one.

## Role ownership

- Planner is selected manually by the operator. It is not a ThreadDock routing
  setting and may use the built-in OpenCode `explore` Agent.
- Builder and Reviewer are ThreadDock's routed roles. Define their OpenCode
  Agents and models in OpenCode, then configure the Agent names below.

Before a production or disposable pilot RUN, run this preflight in the same
environment that will start the Agents:

```sh
opencode agent list
```

Confirm that each configured Builder and Reviewer name appears in the list.
Do not add model names to ThreadDock configuration and do not expect
`agentctl` to read OpenCode configuration or invoke `opencode agent list`.

## Configuration

Set optional `openCodeAgents` fields in the ThreadDock configuration file:

```json
{
  "openCodeAgents": {
    "builder": "build",
    "reviewer": "build"
  }
}
```

`openCodeAgents`, `builder`, and `reviewer` are optional. Omit the object or
leave either field empty when that role has no explicit OpenCode Agent route.
Use only Agent names accepted by ThreadDock configuration validation and
listed by the OpenCode preflight.

## Snapshot and change safety

Routing is captured when a RUN starts. After configuration changes, existing RUNs retain snapshot routing; only newly started RUNs use the new Builder or
Reviewer names. Never edit stored RUN state to change an in-progress role.

For a live routing check, use a new disposable repository, state root, Herdr
Workspace, and RUN. Record structured identifiers and snapshot routing only;
do not record prompts, transcripts, raw provider output, models, or
credentials. Keep accepted and diagnostic pilot artifacts out of the probe's
scope.
