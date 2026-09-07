# Runtime Adapters MVP Design

## Status

Approved in conversation on 2026-09-07. This design replaces further
fault-pilot hardening with a small, usable Git-centered harness.

## Goal

ThreadDock coordinates approved work through Git Worktrees, role-scoped agent
invocations, independent review, an Integration Branch, and a human-merged Pull
Request. The same Go Orchestrator can invoke either OpenCode or Codex without
putting provider or model names in the Contract.

The first usable version optimizes for a developer trying the workflow on real
work. It does not attempt to certify every failure mode before use.

## Product boundary

ThreadDock owns:

- Contract validation and Task DAG scheduling;
- Git Worktree, branch, commit and integration state;
- invocation packet construction and structured Artifact validation;
- deterministic verification commands;
- fresh-context review;
- Final Manifest production; and
- durable local Run state.

The Main Agent owns the conversational workflow and GitHub coordination through
`gh`. Builder, Reviewer, Scout and Documenter invocations do not receive
GitHub-write authority.

The human owns merging the final Pull Request into the default branch.

## Deliberate scope reduction

The slim implementation starts from repository `main`, not from the
`agent/parallel-fault-pilot` branch. The fault-pilot branch and its external
diagnostic artifacts remain intact as historical experiments.

Only the generally useful GitHub credential environment scrubbing from that
branch is ported. The following are excluded from the product branch:

- the parallel fault-pilot controller and fixtures;
- deterministic repair-exhaustion, conflict and recovery stories;
- pilot-specific process-group supervisors and test sharding;
- private-tmpfs certification machinery;
- live fault evidence and artifact inventories; and
- fault-pilot release gates.

## Ubiquitous language

- **Execution Profile** is a logical role policy ID stored in a Contract.
- **Agent Runtime** executes one role packet and returns one structured
  Artifact.
- **Agent Invocation** is one execution against one Worktree. Codex invocations
  are ephemeral.
- **CodeMap** is a bounded answer to Planner questions, not a full repository
  dump.
- **Final Manifest** is the only execution summary the Main Agent needs for
  GitHub handoff.
- **Finalize** happens after a human merge and performs Issue, Project and
  documentation publication work.

The root `CONTEXT.md` is the canonical glossary.

## Roles

### Main Agent

The Main Agent interviews the user, invokes planning and execution, creates or
updates GitHub work records with `gh`, presents the Pull Request for human
merge, and performs Finalize. It consumes structured Artifacts rather than raw
agent transcripts.

### Planner

The Planner converts a Development Request and optional CodeMap into a Contract
v2. It runs before the Contract exists, so its profile is local configuration,
not a Contract field.

### Scout

The Planner may invoke a Scout only when it cannot safely identify relevant
paths, symbols, dependencies or risks. The Scout is read-only and returns a
bounded CodeMap. Its profile is local configuration.

### Builder

Each ready Task gets its own Worktree and Builder invocation. The Builder may
modify only allowed paths, run declared verification and create a commit. It
does not push, merge or write GitHub state.

### Reviewer

Each completed Builder result is reviewed in a fresh invocation. The Reviewer
receives the Task, base and head commit, Git-generated diff and verification
results. It does not receive the Builder transcript and is read-only.

### Documenter

Documenter is conditional. After all code Tasks integrate and pass checks, it
receives the Final Manifest, changed paths, public behavior changes and only
the relevant documentation pages. Repository documentation and Wiki work use
separate ephemeral invocations and separate Worktrees. Documenter produces a
commit or patch; it does not publish.

No separate Test, Integrator, GitHub, Recovery or Security Agent is part of the
MVP. Tests and integration are deterministic Go operations. Riskier work
selects a stricter Reviewer Execution Profile.

## Contract v2

Contract v2 adds role-level profile IDs:

```json
{
  "version": 2,
  "executionProfiles": {
    "builder": "build.standard",
    "reviewer": "review.strict",
    "documenter": "docs.standard"
  }
}
```

`builder` and `reviewer` are required nonempty IDs. `documenter` is optional and
is required only when the Contract requests documentation work. IDs use a
small ASCII grammar and a bounded length; they are opaque to Contract
validation.

The profile is role-level, not Task-level. Every Builder Task in one Run uses
the resolved Builder profile. This is an intentional MVP limitation.

Issue drafts retain logical `issueKey` values. GitHub Issue numbers are stored
in Run state after the Main Agent materializes the approved Issue plan. The
immutable Contract is not rewritten with environment-specific Issue numbers.

Contract v1 remains readable and executable through the existing OpenCode
defaults. New planning emits v2. Existing Run snapshots are not migrated.

## Local configuration

Local non-secret configuration maps logical IDs to native runtime profiles:

```json
{
  "executionProfiles": {
    "build.standard": {
      "runtime": "codex",
      "nativeProfile": "builder"
    },
    "review.strict": {
      "runtime": "opencode",
      "nativeProfile": "build"
    },
    "docs.standard": {
      "runtime": "codex",
      "nativeProfile": "documenter"
    },
    "plan.standard": {
      "runtime": "codex",
      "nativeProfile": "planner"
    },
    "explore.fast": {
      "runtime": "codex",
      "nativeProfile": "scout"
    }
  },
  "plannerExecutionProfile": "plan.standard",
  "scoutExecutionProfile": "explore.fast"
}
```

`runtime` is exactly `codex` or `opencode`. `nativeProfile` is a validated
runtime-local name. ThreadDock does not store a model name in the Contract.
Codex and OpenCode native configuration select the actual model, reasoning and
provider settings.

At Run creation, ThreadDock resolves every referenced Execution Profile and
pins the ID, runtime and native profile into the Run snapshot. Later local
configuration changes do not reroute an existing Run.

Credentials never appear in this mapping.

## Deep execution module

The Orchestrator depends on one small interface:

```go
type AgentRuntime interface {
    Invoke(context.Context, Invocation) (Artifact, error)
}
```

`Invocation` contains only:

- role;
- stable request ID;
- resolved profile snapshot;
- canonical Worktree path;
- role packet;
- required output schema; and
- sandbox intent (`read-only` or `workspace-write`).

`Artifact` contains a runtime invocation identity and schema-validated result.
Role modules decode it into `BuildResult`, `ReviewResult`, `CodeMap`,
`PlanResult` or `DocumentationResult`.

The Worktree is created and validated before `Invoke`. Runtime adapters do not
create branches, push, merge, close Issues or update Projects.

### Codex adapter

The Codex adapter invokes the installed Codex CLI non-interactively. Its tested
command contract uses the installed capabilities corresponding to:

```text
codex exec --ephemeral --strict-config --profile PROFILE
  --cd WORKTREE --sandbox MODE --ask-for-approval never
  --output-schema SCHEMA --output-last-message RESULT -
```

The packet is provided on standard input, never argv. The adapter captures a
bounded result file, rejects missing, oversized, malformed or trailing output,
and removes temporary files it owns. It never enables web search or bypasses
approvals and sandboxing.

Every Codex invocation is ephemeral. Builder retry, Reviewer and Documenter
are new invocations. There is no Codex session ID, resume or retirement state.
Git and the Run snapshot carry continuity.

### OpenCode adapter

The OpenCode adapter contains the existing Herdr/OpenCode lifecycle. It may
create and retire provider sessions internally, but Herdr pane, workspace and
terminal identities do not belong in the provider-neutral Orchestrator
interface for Contract v2.

Legacy Contract v1 Runs keep their existing state path. The MVP does not
rewrite or migrate active legacy Runs.

## Role results

Builder result:

```json
{
  "requestId": "run-123:task-api:build:1",
  "commitSha": "40-lowercase-hex",
  "verification": [
    {"command": "go test ./internal/api", "outcome": "passed", "duration": "1.2s"}
  ]
}
```

Reviewer result:

```json
{
  "requestId": "run-123:task-api:review:1",
  "decision": "accept",
  "blockingFindings": [],
  "riskCategories": []
}
```

Documenter result identifies its target (`repository` or `wiki`), commit or
patch identity, changed pages and verification. Raw transcripts and provider
payloads are never accepted as evidence.

## Execution flow

1. Main Agent gathers the Development Request.
2. Planner optionally requests one or more bounded Scout CodeMaps.
3. Planner emits Contract v2 and Issue drafts.
4. User approves the Contract and Issue plan.
5. Main Agent creates GitHub Issues through `gh`; Run state maps issue keys to
   returned numbers.
6. Orchestrator selects ready Tasks from the DAG.
7. Orchestrator creates a Task Worktree and invokes the Builder profile.
8. Orchestrator validates the commit, allowed paths and exact verification.
9. A fresh Reviewer invocation accepts or blocks the Task.
10. Accepted Task branches merge into the Integration Branch.
11. After all Tasks, the Orchestrator runs the Full Suite on the latest
    Integration Branch.
12. Conditional Documenter invocations prepare repository docs and Wiki
    changes.
13. Orchestrator emits a Final Manifest with status `ready_for_pr`.
14. Main Agent pushes the Integration Branch, creates the Pull Request, links
    Issues and sets the Project state to `Review`.
15. A human reviews and merges the Pull Request.
16. Finalize verifies the merged PR and expected merge commit, closes Issues,
    sets Project state to `Done`, publishes prepared Wiki work and reports the
    result.

## Review and retry policy

Reviewer rejection sets the Task to `needs_operator`. The MVP does not run an
automatic repair loop. The Main Agent shows the structured findings. If the
user selects retry, a new ephemeral Builder invocation receives the original
Task plus those findings and works on the existing Task branch.

Runtime failure, invalid Artifact, unauthorized path change, failed declared
verification, integration conflict or Full Suite failure also produces
`needs_operator`. No model-driven recovery Agent is started automatically.

## GitHub coordination

The Main Agent is the only agent role with GitHub-write authority. It uses
`gh` against exact repository and item identities from approved Issue drafts
or a Final Manifest.

Workers and Reviewer packets explicitly forbid push and GitHub mutation.
GitHub operations use stable ThreadDock markers so an interrupted Main Agent
can inspect current state and continue without duplicate Issues or comments.

Before human merge:

- create or update the Pull Request;
- link Child Issues;
- set Project state to `Review`; and
- keep Issues open and Wiki changes unpublished.

Finalize requires the Pull Request to be merged and its merge commit to match
the expected Integration result. Only then does it close Issues, set Project
state to `Done` and publish prepared Wiki changes. A partial GitHub failure
sets `publication_pending`; it does not rerun code Tasks.

The Main Agent does not invoke automatic default-branch merge.

## Security and process rules

- Port the general `THREADDOCK_GH_TOKEN` and `GH_TOKEN` child-environment
  scrubbing from the archived pilot branch into the slim branch.
- Runtime children receive no GitHub credential.
- Codex receives the selected sandbox and only the assigned Worktree.
- Reviewer and Scout are read-only.
- Packets contain no credential, environment dump or unrelated transcript.
- Structured Artifact files are owner-only and size-bounded.
- Git-generated diffs and commit inspection, not agent claims, establish
  changed paths and commit identity.
- Default-branch merge remains a human action.

## State and resumability

Run state stores Contract identity, issue-key mapping, resolved role profiles,
Task Worktree/branch/commit status, Review results, Integration commit, checks,
documentation results, Pull Request identity and publication status.

It does not store Codex session state. On restart, the Orchestrator reconciles
Git and Run state. An incomplete invocation without a validated Artifact is
`needs_operator`; it is not silently repeated.

OpenCode provider session state remains adapter-private for v2 and legacy state
for v1.

## Testing strategy

Required automated coverage is intentionally small:

- Contract v2 codec and validation;
- v1 compatibility;
- local profile parsing, resolution and snapshot pinning;
- AgentRuntime conformance tests shared by adapters;
- Codex argv/stdin/sandbox/output-schema and malformed-result tests using a
  fake executable;
- OpenCode adapter compatibility with the existing Herdr fake;
- Builder and fresh Reviewer flow;
- conditional Documenter flow;
- temporary local Git repository end-to-end through Final Manifest; and
- GitHub publication planning/idempotent marker tests without live writes.

One manual smoke test may create a small Pull Request in a dedicated pilot
repository after the local suite and independent review pass. It is not a
fault-injection certification system.

The clean `main` baseline was checked before this document was written. The
repository-wide `make check` reached Go's existing ten-minute package timeout
while `internal/orchestrator` was blocked in a directory `fsync`; the exact
reported test passed alone in 6.228 seconds. This is retained as an environment
observation, not permission to add sharding, tmpfs or fault-pilot machinery to
the MVP. The implementation plan uses focused tests during development and a
single repository check under a quiet host before completion.

## File structure

The implementation plan may refine names, but the intended modules are:

```text
internal/execution/          profile snapshot, Invocation, Artifact, Runtime
internal/execution/codex/    ephemeral Codex adapter
internal/execution/opencode/ Herdr/OpenCode adapter
internal/roles/              typed Planner/Scout/Builder/Reviewer/Documenter
internal/orchestrator/       provider-neutral Git/DAG state transitions
internal/publication/        Final Manifest and finalize planning
```

Each runtime is an adapter at the `AgentRuntime` seam. Role packet and result
validation belongs to role modules, not adapters.

## Migration

1. Preserve `agent/parallel-fault-pilot` and every existing external artifact.
2. Develop on `agent/runtime-adapters-mvp` created from clean `main`.
3. Port only the general process credential scrubbing.
4. Add Contract v2 and profile resolution without changing v1 behavior.
5. Add Codex adapter and provider-neutral role invocation.
6. Wrap the current Herdr/OpenCode behavior behind the same interface for v2.
7. Add Final Manifest, conditional Documenter and human-merge Finalize.
8. Run local tests and one explicitly authorized smoke task.

## Acceptance

The MVP is ready for first use when:

- one Contract v2 selects role profiles without provider/model fields;
- local config can mix Codex and OpenCode by role;
- Codex Builder completes through an ephemeral invocation and leaves no saved
  session;
- Reviewer runs in a fresh read-only invocation and accepts or blocks with a
  strict result;
- accepted Task commits integrate and produce a Final Manifest;
- optional Documenter produces a separate docs or Wiki change;
- Main Agent can create a PR and move work to Review without merging main;
- Finalize refuses an unmerged or mismatched PR and completes post-merge work
  for a matching PR;
- v1 OpenCode Contracts still run; and
- no parallel fault-pilot controller, fixture or mandatory live-fault gate is
  present on the slim branch.
