# ThreadDock Model Profile Routing and OMO Adapter Design

Date: 2026-09-17
Status: Proposed

## 1. Purpose

ThreadDock currently separates durable work truth, execution lifecycle, and agent workflow reasonably well:

- GitHub owns Issues, Projects, PRs, review evidence, and durable work state.
- Herdr owns top-level interactive execution lifecycle.
- ThreadDock owns the workflow that connects GitHub work to `td_coordinator`, `td_feature_leader`, Skills, and review/publish evidence.
- Leaf implementation uses bounded native subagents and focused TDD rather than a ThreadDock-owned scheduler/runtime.

The remaining problem is model selection. Different harnesses expose different model sets and configuration mechanisms. OpenCode may also run with Oh My OpenCode (OMO), whose category-based delegation already performs model routing. ThreadDock must therefore avoid hard-coding one provider/model taxonomy into its workflow.

This design introduces:

1. logical execution profiles that describe task difficulty/risk without naming a model;
2. harness capability discovery for model routing;
3. harness-specific adapters for Codex, native OpenCode, and OpenCode+OMO;
4. an explicit boundary that allows OMO as a leaf delegation/model-routing backend without allowing OMO to become a second top-level orchestrator.

## 2. Goals

- Let `td_feature_leader` classify bounded work by execution needs rather than provider/model name.
- Allow users to map logical profiles to the models actually available in their Codex/OpenCode environment.
- Reuse OMO category routing when OMO is present instead of duplicating its model router.
- Keep ThreadDock orchestration ownership unchanged: GitHub-first, Herdr-first, Feature Leader-owned decomposition, TDD workers, fresh review, human merge.
- Preserve deterministic evidence about requested profile, resolved execution path, and actual model when the harness exposes it.
- Fail visibly when a requested profile cannot be resolved; never silently substitute an unknown model.

## 3. Non-goals

- ThreadDock will not become a model gateway or proxy.
- ThreadDock will not own provider credentials.
- ThreadDock will not introduce a scheduler, lease/recovery engine, task database, or secondary execution protocol.
- OMO Team Mode, `/ulw-loop`, Prometheus-style top-level planning, or any second orchestration graph is not part of this integration.
- ThreadDock will not rewrite project agent files on every dispatch when runtime routing is available.
- Task size alone will not determine model choice.

## 4. Current state

### 4.1 Codex

The project template currently pins long-lived Codex parent agents such as `td_coordinator` and `td_feature_leader` to a specific Sol model/reasoning configuration. Leaf agent definitions primarily encode role and Skill behavior.

### 4.2 OpenCode

The OpenCode project-template agents currently do not pin `Sol` or any equivalent model. They define role/workflow behavior and therefore follow the OpenCode-selected model or inheritance behavior unless later configured otherwise.

ThreadDock Skills such as `develop-feature`, `brainstorm-design`, `grill-plan`, `tdd-task`, `review-change`, and `agent-wait` describe workflow semantics; they do not currently instruct OpenCode to use a Sol-family model.

### 4.3 OMO

Previous compatibility work treated ThreadDock and OMO as competing orchestration layers. That remains true if both try to own planning, task topology, retries, and multi-agent execution.

However, the current ThreadDock design is narrower than the earlier engine-heavy design. A Feature Leader already dispatches bounded leaf work with a matching Skill. This creates a compatible seam for OMO's category-based delegation if OMO is restricted to that seam.

## 5. Core principle: Profile, Skill, and Model are separate

ThreadDock must distinguish three independent concepts:

- **Skill**: what workflow the child performs, e.g. `tdd-task`, `review-change`, `explore-codebase`.
- **Execution Profile**: how much reasoning/risk handling the task needs, e.g. `BUILD`, `DEEP`.
- **Model**: the concrete harness/provider model selected for the profile.

Example:

```yaml
skill: tdd-task
executionProfile: DEEP
routingReasons:
  - shared public interface change
  - concurrency-sensitive state transition
  - rollback behavior required
```

The Feature Leader chooses the Skill and execution profile. It does not choose `gpt-*`, `claude-*`, `gemini-*`, or a corporate LiteLLM model name directly.

## 6. Logical execution profiles

Initial profiles:

| Profile | Intended use |
| --- | --- |
| `FAST` | repository exploration, docs, mechanical or low-risk bounded changes |
| `BUILD` | ordinary bounded product implementation with clear acceptance criteria and test seams |
| `DEEP` | cross-interface work, state machines, concurrency, migrations, compatibility-sensitive implementation |
| `CRITICAL` | destructive/high-risk changes, security-sensitive paths, repeated complex failures, critical integration fixes |
| `REVIEW` | fresh independent code/review analysis |
| `BRAINSTORM` | design-space exploration before `grill-plan` and implementation |

Long-lived parent roles may also have explicit settings:

- `COORDINATOR`
- `FEATURE_LEADER`

These parent-role settings are configuration identities, not task severity levels.

## 7. Classification policy

`td_feature_leader` classifies each bounded task using risk, ambiguity, and architectural scope rather than changed-file count.

Suggested default policy:

- `FAST`: docs/config validation, exploration, mechanical rename, isolated low-risk edits.
- `BUILD`: default for ordinary implementation.
- `DEEP`: shared/public interface change, concurrency, state transition, migration, multi-module compatibility, complex failure/recovery semantics.
- `CRITICAL`: destructive operation, security/auth boundary, data-loss risk, high-impact integration, or a replan showing ordinary execution is insufficient.
- `REVIEW`: default independent review; elevate to a stronger configured review profile only if policy explicitly allows it.
- `BRAINSTORM`: substantial, novel, or architecture-affecting design exploration.

The Feature Leader must record concise routing reasons in the task packet. File count, diff size, or token estimate may be supporting evidence but must not be the sole criterion.

## 8. User-level model profile configuration

ThreadDock stores model mappings at user level rather than duplicating them in every project.

Illustrative configuration:

```json
{
  "modelProfiles": {
    "codex": {
      "coordinator": {"model": "gpt-5.6-sol", "reasoning": "medium"},
      "featureLeader": {"model": "gpt-5.6-sol", "reasoning": "medium"},
      "fast": {"model": "...", "reasoning": "..."},
      "build": {"model": "...", "reasoning": "..."},
      "deep": {"model": "...", "reasoning": "..."},
      "critical": {"model": "...", "reasoning": "..."},
      "review": {"model": "...", "reasoning": "..."},
      "brainstorm": {"model": "...", "reasoning": "..."}
    },
    "opencode": {
      "fast": {"model": "provider/model-a"},
      "build": {"model": "provider/model-b"},
      "deep": {"model": "provider/model-c"},
      "critical": {"model": "provider/model-d"},
      "review": {"model": "provider/model-c"},
      "brainstorm": {"model": "provider/model-d"}
    }
  }
}
```

The exact persistent file/schema may evolve with the existing ThreadDock settings implementation. The important rule is ownership: project workflows consume logical profiles; user-level ThreadDock settings resolve them.

## 9. Harness capability probe

ThreadDock must not assume every harness supports the same routing mechanism.

Record capabilities independently per harness:

```yaml
opencode:
  available: true
  modelDiscovery: supported
  runtimeModelOverride: supported | unsupported | unknown
  omoDetected: true | false | unknown

codex:
  available: true
  modelDiscovery: supported | unsupported | unknown
  runtimeModelOverride: supported | unsupported | unknown
```

### 9.1 Probe rules

1. Use the installed local harness/tool, not invented provider IDs.
2. Discover available models when the harness exposes them.
3. If practical, launch a harmless probe child requesting a non-default configured model.
4. Mark runtime override `supported` only when the actual child execution can be verified.
5. If verification is unavailable, keep the value `unknown` rather than assuming support.
6. Never change an existing user model configuration merely because discovery temporarily fails.

The Settings UI should surface the distinction between `supported`, `unsupported`, and `unknown`.

## 10. Native runtime routing path

When the harness can select a model at child-spawn time:

```text
Feature Leader
  -> classify task as DEEP
  -> resolve DEEP from ThreadDock user settings
  -> spawn native bounded child with resolved model
  -> run matching Skill (for example tdd-task)
```

No generated per-profile agent files are required in this path.

## 11. Generated-agent fallback

If a harness cannot reliably override the model at spawn time, ThreadDock may generate stable named agents from the user's profile configuration.

Example:

```text
td_implementer_fast
td_implementer_build
td_implementer_deep
td_implementer_critical
td_reviewer_review
```

Each generated agent retains the same ThreadDock Skill contract but pins the configured model/profile.

Rules:

- Generate/synchronize only when capability requires it.
- Do not rewrite unrelated agent definitions.
- Configuration changes should have an explicit sync/apply operation rather than hidden project mutation during unrelated work.
- Missing profile models must block or use only an explicitly configured fallback.

## 12. OpenCode + OMO adapter

### 12.1 Revised compatibility judgment

ThreadDock and OMO remain poor peers as competing top-level orchestrators, but OMO can be a suitable OpenCode-specific leaf delegation/model-routing backend.

Allowed ownership:

```text
GitHub
  -> durable work truth

Herdr
  -> top-level interactive session lifecycle

ThreadDock
  -> coordinator
  -> feature leader
  -> design/grill/decomposition
  -> task ownership and bounded paths
  -> TDD/review/publish policy

OMO
  -> bounded leaf delegation
  -> category-to-model routing
  -> loading the ThreadDock Skill requested by the Feature Leader
```

### 12.2 Forbidden ownership overlap

The ThreadDock integration must not invoke OMO as a second top-level workflow owner. In particular, the adapter must not hand a whole feature to OMO planning/team orchestration and then allow OMO to create a competing task graph.

Avoid patterns equivalent to:

```text
ThreadDock Feature Leader
  -> OMO top-level planning loop
  -> OMO team orchestration
  -> separate task/state graph
```

This would duplicate:

- plan ownership;
- decomposition ownership;
- worktree/path ownership;
- retry/replan policy;
- reviewer selection;
- completion state.

### 12.3 Profile-to-category translation

When OMO is detected and its category delegation is usable, ThreadDock translates logical profiles to OMO categories rather than selecting a concrete model itself.

Illustrative mapping only:

```yaml
FAST: quick
BUILD: general
DEEP: deep
CRITICAL: ultrabrain
REVIEW: deep
BRAINSTORM: ultrabrain
```

The final mapping must be configurable because OMO installations and category names may differ. ThreadDock must not assume these exact category identifiers exist.

The dispatch shape becomes conceptually:

```text
profile = DEEP
skill = tdd-task

-> OMO adapter
-> configured category for DEEP
-> bounded child delegation with tdd-task loaded
```

OMO owns category-to-model resolution. ThreadDock still owns task scope and acceptance evidence.

## 13. Adapter selection

Execution backend selection:

```text
Codex
  -> ThreadDock profile resolver
     -> runtime override if verified
     -> otherwise generated-agent fallback

OpenCode without OMO
  -> ThreadDock profile resolver
     -> runtime override if verified
     -> otherwise generated-agent fallback

OpenCode with compatible OMO
  -> ThreadDock profile
     -> OMO category adapter
     -> OMO category/model routing
```

The Feature Leader should not need separate planning logic for these paths. It emits the same bounded task packet plus logical profile and Skill.

## 14. Evidence contract

Every routed child should retain enough evidence to diagnose mismatches without turning model configuration into GitHub work state.

Recommended execution evidence:

```yaml
requestedProfile: DEEP
routingReasons:
  - shared interface
  - concurrency
harness: opencode
backend: omo-category
resolvedCategory: deep
resolvedModel: provider/model-x   # only when observable
modelVerification: verified | unverified
```

For native routing:

```yaml
backend: native-model-override
resolvedModel: provider/model-x
```

For generated-agent fallback:

```yaml
backend: generated-agent
resolvedAgent: td_implementer_deep
resolvedModel: provider/model-x
```

Do not claim an actual model when the harness does not expose reliable evidence. Mark it `unverified`.

## 15. Failure and fallback behavior

- Missing configured profile: block dispatch and report the missing profile.
- Configured model no longer available: report unavailable; do not silently choose another model.
- OMO detected but category mapping missing: either use the explicitly configured native OpenCode path or block; never guess a category.
- OMO tool/delegation failure: preserve the task as pending/blocked. Do not automatically move orchestration ownership to OMO or create a retry engine.
- Runtime override probe becomes stale/unknown: prefer safe re-probe or configured fallback path before claiming support.
- Repeated implementation failure: follow existing ThreadDock rule—replan after repeated same-root-cause failure. Profile escalation may be an outcome of replanning but must not be a blind automatic retry loop.

## 16. Settings UX

Add an `Agent Model Profiles` section under ThreadDock Settings.

For each harness:

- harness availability;
- model discovery status;
- runtime override capability (`verified`, `unsupported`, `unknown`);
- OMO detected/compatible status for OpenCode;
- profile mapping controls;
- refresh/probe action;
- last verification timestamp/result;
- explicit warnings for configured-but-unavailable models/categories.

For OMO mode, show profile-to-category mapping rather than duplicating OMO's category-to-model settings when OMO already owns those settings.

## 17. Skill integration

Introduce a routing Skill or shared routing contract consumed by the Feature Leader. Its responsibilities are limited to:

1. validate the Feature Leader's requested logical profile;
2. read the user-level ThreadDock routing configuration;
3. select the harness adapter;
4. produce the bounded native/OMO dispatch request;
5. return routing evidence.

It must not reinterpret the feature goal, widen task scope, create a scheduler, or invent models/categories.

Expected workflow:

```text
Issue / request
  -> td_coordinator
  -> td_feature_leader
  -> explore-codebase when needed
  -> brainstorm-design when needed
  -> grill-plan when needed
  -> bounded task decomposition
  -> execution profile classification
  -> harness adapter routing
  -> tdd-task / docs / exploration
  -> agent-wait while delegated work is outstanding
  -> integration
  -> fresh review-change
  -> agent-wait
  -> publish-work
  -> human merge
```

## 18. Rollout plan

### Phase 1: Configuration and capability discovery

- Add user-level profile schema.
- Add harness capability state.
- Discover OpenCode/Codex model availability without changing current execution behavior.
- Detect OMO presence conservatively.

### Phase 2: Profile classification evidence

- Extend Feature Leader task packets with `executionProfile` and `routingReasons`.
- Do not yet change concrete model selection.
- Verify classifications against real tasks.

### Phase 3: Native adapter

- Enable verified runtime override where supported.
- Implement generated-agent fallback only where required.

### Phase 4: OMO adapter pilot

- Reuse the existing `thread-dock-omo-validation` fixture.
- Validate that a Feature Leader remains the owner while OMO receives only one bounded leaf task and one ThreadDock Skill.
- Verify that no OMO top-level/team planning state becomes required for ThreadDock completion.
- Verify routing evidence and actual model/category where observable.

### Phase 5: Settings UI and defaults

- Expose mappings and capability status in ThreadDock Settings.
- Keep defaults conservative and editable.
- Do not overwrite existing OMO category model configuration.

## 19. Acceptance criteria

The design is successfully implemented when:

1. Feature Leader task packets use logical profiles rather than direct provider model names.
2. The same ThreadDock workflow can run under Codex, native OpenCode, and OpenCode+OMO without changing feature decomposition semantics.
3. ThreadDock can distinguish verified runtime override support from unknown/unsupported capability.
4. OpenCode+OMO uses OMO only for bounded leaf delegation/model routing, never as a competing top-level orchestrator.
5. Missing/unavailable model or category configuration is visible and never silently substituted.
6. Execution evidence records requested profile, selected backend, and verified model/category information when available.
7. Existing TDD, focused-test, fresh-review, agent-wait, GitHub-first, Herdr-first, and human-merge rules remain intact.

## 20. Decision summary

ThreadDock will treat model selection as a harness adapter concern, not a workflow concern.

`td_feature_leader` decides **what kind of execution is required**. ThreadDock/OMO decides **how that profile maps to the current harness**. The harness executes the child.

OMO compatibility is therefore revised from "not suitable together" to the narrower statement:

> OMO is not a second ThreadDock orchestrator. When present under OpenCode, it may be used as a bounded leaf delegation and category/model-routing backend while ThreadDock retains plan, scope, TDD, review, publication, and completion ownership.
