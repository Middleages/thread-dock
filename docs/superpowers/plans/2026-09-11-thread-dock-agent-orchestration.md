# ThreadDock Agent Orchestration Implementation Plan

> 2026-09-12 상태: PR #87로 두 Agent 정의·여섯 canonical Skill·네 migration shim·quickstart·template-check 구현이 main에 반영됐다. 아래 원래 단계 체크는 실행 증거 ledger가 아니며 미체크를 미구현으로 해석하지 않는다. [검증 제한](../reviews/2026-09-11-thread-dock-agent-skill-verification.md)의 pressure scenario와 native E2E는 별도 미완료다. 재구현하지 않는다.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a simple, reusable ThreadDock project template with two top-level Codex agents and six focused skills that coordinate GitHub work through Herdr without turning ThreadDock into an execution engine.

**Architecture:** Target projects receive two persistent agent definitions (`td_coordinator`, `td_feature_leader`) under `.codex/agents` and six cross-runtime skills under `.agents/skills`. Coordinator owns project-level routing and shared locator maintenance; Feature Leader owns one feature and invokes transient native subagents through skills. Legacy five skill names remain as short migration shims so existing prompts do not silently break.

**Tech Stack:** Codex agent TOML, Agent Skills `SKILL.md`, GitHub/Herdr CLI conventions, existing ThreadDock docs and Makefile.

**Spec:** `docs/superpowers/specs/2026-09-11-thread-dock-agent-orchestration-design.md`

## Global Constraints

- Keep exactly two top-level ThreadDock agent roles in the project template: `td_coordinator` and `td_feature_leader`.
- Keep exactly six canonical new skills: `coordinate-work`, `develop-feature`, `grill-plan`, `tdd-task`, `review-change`, `publish-work`.
- Planner, implementer, reviewer, and publish/docs helpers are transient native subagents, not Herdr top-level sessions.
- GitHub Issue/PR/Projects remain durable work truth; Herdr remains execution lifecycle owner; ThreadDock remains monitor + skills + GitHub evidence binding.
- Default local locator is the single user-level `~/.threaddock/sessions.json` shared across projects.
- Locator writes require user-level exclusive locking, re-read inside the lock, exact mutation, temporary file, and atomic rename; no bulk pruning.
- Feature bindings are removed only when Issue closed + related PR work finished + top-level Herdr feature session absent.
- Human merge remains the final transition.
- Worker-level full test suites are not required; focused checks first, repository-wide `make check` once before merge.
- New or materially changed skills follow the installed `writing-skills` guidance: concise `Use when...` descriptions, no workflow summary in frontmatter, and explicit cross-skill references rather than duplicated instructions.

---

### Task 1: Ship the two project-level agent definitions

**Files:**
- Create: `project-template/.codex/agents/td_coordinator.toml`
- Create: `project-template/.codex/agents/td_feature_leader.toml`

**Interfaces:**
- Consumes: canonical skill names defined by Tasks 2-3.
- Produces: project-level Codex agent names `td_coordinator` and `td_feature_leader`.

- [ ] **Step 1: Create the Coordinator agent definition**

Use this contract in `td_coordinator.toml`:

```toml
name = "td_coordinator"
description = "ThreadDock project coordinator. Routes GitHub work into Herdr feature sessions and keeps project-level evidence coherent."
model = "gpt-5.6-sol"
model_reasoning_effort = "medium"
developer_instructions = """
Read repository instructions and the relevant GitHub Issue/Project before dispatch. Use coordinate-work as the orchestration entrypoint and publish-work for durable updates. Treat GitHub as work truth, Herdr as execution lifecycle, and ~/.threaddock/sessions.json as local locator only. Do not implement normal product features yourself when a feature session is warranted. Create or reuse top-level Feature Leader sessions, keep shared-interface conflicts serialized, and report blockers/PR/review evidence. Do not create a scheduler, task database, lease engine, or parallel execution registry. Human merge remains final.
"""
```

- [ ] **Step 2: Create the Feature Leader agent definition**

Use this contract in `td_feature_leader.toml`:

```toml
name = "td_feature_leader"
description = "ThreadDock feature leader. Owns one GitHub feature from detailed planning through reviewed PR readiness."
model = "gpt-5.6-sol"
model_reasoning_effort = "medium"
developer_instructions = """
Own exactly one GitHub Issue or independently deliverable feature. Use develop-feature as the entrypoint. Use grill-plan only when requirements are materially ambiguous or cross interfaces; dispatch bounded implementation work to native subagents using tdd-task; obtain a fresh independent review with review-change; finish durable records with publish-work. Internal planner/implementer/reviewer helpers are transient native subagents and are not ThreadDock top-level sessions or sessions.json bindings. Keep worker checks focused and preserve exact SHA/test evidence. Human merge remains final.
"""
```

- [ ] **Step 3: Review agent definitions for forbidden ownership**

Confirm neither agent claims ownership of Herdr lifecycle internals, leases, process recovery, a task DB, or automatic main merge.

- [ ] **Step 4: Commit Task 1**

```bash
git add project-template/.codex/agents/td_coordinator.toml project-template/.codex/agents/td_feature_leader.toml
git commit -m "feat: add minimal ThreadDock project agents"
```

---

### Task 2: Add coordination and feature-orchestration skills

**Files:**
- Create: `project-template/.agents/skills/coordinate-work/SKILL.md`
- Create: `project-template/.agents/skills/develop-feature/SKILL.md`
- Create: `project-template/.agents/skills/grill-plan/SKILL.md`

**Interfaces:**
- `coordinate-work` may invoke/refer to `publish-work` and starts/reuses Herdr Feature Leader sessions.
- `develop-feature` invokes `grill-plan`, `tdd-task`, `review-change`, and `publish-work` conditionally.
- `grill-plan` returns a bounded implementation plan; it creates no orchestration state.

- [ ] **Step 1: Write `coordinate-work` with discovery-first frontmatter**

Frontmatter must be exactly shaped like:

```markdown
---
name: coordinate-work
description: Use when a project coordinator must turn GitHub work into one or more bounded Herdr feature sessions across a repository or Project.
---
```

Body requirements:
- GitHub Issue/PR/Projects are durable truth.
- Reuse an existing exact locator before starting a duplicate session.
- Split only independently deliverable features.
- Write only Coordinator/Feature Leader top-level locators.
- Use `~/.threaddock/sessions.json`, shared lock, re-read, exact upsert, atomic rename.
- Never guess Herdr IDs.
- Serialize only shared interface/file boundaries.
- **REQUIRED SUB-SKILL:** `publish-work` for durable GitHub/locator updates.

- [ ] **Step 2: Write `develop-feature` as the single Feature Leader entrypoint**

Frontmatter:

```markdown
---
name: develop-feature
description: Use when a Feature Leader owns one approved GitHub Issue or independently deliverable feature through implementation and PR readiness.
---
```

Body decision flow:
1. Read Issue acceptance criteria and repository guidance.
2. If materially ambiguous or cross-interface, use `grill-plan`; otherwise skip it.
3. Split only implementation tasks that can safely write independently.
4. Native implementation subagents use `tdd-task`; target 1-3 workers, never fill slots just because they exist.
5. Integrate results and run affected focused verification.
6. Fresh independent reviewer uses `review-change` against exact SHA/diff.
7. Blocking findings return to bounded implementation; two repeats of the same cause trigger replanning rather than blind retry.
8. Use `publish-work` for PR/Issue/Project/Wiki/locator records.

- [ ] **Step 3: Write `grill-plan` for ambiguity pressure**

Frontmatter:

```markdown
---
name: grill-plan
description: Use when a development request is too ambiguous, cross-cutting, or assumption-heavy to hand safely to implementation subagents.
---
```

Body must force concrete output: problem, non-goals, acceptance criteria, hidden assumptions, edge/failure cases, interfaces/dependencies, parallelizable tasks, serialized boundaries, and unresolved human decisions. It must explicitly say not to create a second task database or orchestration protocol.

- [ ] **Step 4: Static review against `writing-skills` rules**

Verify all three descriptions start with `Use when`, describe triggers only, and do not summarize their internal workflow.

- [ ] **Step 5: Commit Task 2**

```bash
git add project-template/.agents/skills/coordinate-work project-template/.agents/skills/develop-feature project-template/.agents/skills/grill-plan
git commit -m "feat: add ThreadDock orchestration skills"
```

---

### Task 3: Add execution, review, and publication skills

**Files:**
- Create: `project-template/.agents/skills/tdd-task/SKILL.md`
- Modify: `project-template/.agents/skills/review-change/SKILL.md`
- Create: `project-template/.agents/skills/publish-work/SKILL.md`

**Interfaces:**
- `tdd-task` consumes a bounded task packet and returns changed files, exact commands/outcomes, candidate SHA or blocker.
- `review-change` consumes exact candidate SHA/diff + acceptance evidence and returns accept/block findings.
- `publish-work` consumes final feature state and updates durable GitHub records plus shared locator lifecycle.

- [ ] **Step 1: Write `tdd-task` as a leaf implementation skill**

Frontmatter:

```markdown
---
name: tdd-task
description: Use when an implementation subagent receives one bounded coding task with clear acceptance criteria and an isolated feature worktree.
---
```

Required rules:
- **REQUIRED SUB-SKILL:** `superpowers:test-driven-development` when an applicable production test seam exists.
- Work only assigned scope; no further subagent spawning.
- RED -> GREEN -> REFACTOR for applicable product behavior; docs/config-only use parser/check instead.
- Focused tests only; no worker full suite.
- Shared/public interface changes return to Feature Leader for replanning.
- Report changed files, exact commands, outcomes, unverified checks, blocker, and candidate SHA when available.

- [ ] **Step 2: Tighten `review-change` for transient fresh reviewer use**

Keep the existing exact-SHA independent review behavior, but add explicit triggers/constraints:
- reviewer is a transient native subagent, not a top-level ThreadDock binding;
- no edits or merge;
- acceptance-by-acceptance evidence;
- do not rerun already trustworthy identical SHA/command/environment checks without cause;
- concrete file/symbol/behavior for every blocking finding.

Keep its frontmatter trigger-only.

- [ ] **Step 3: Write `publish-work`**

Frontmatter:

```markdown
---
name: publish-work
description: Use when a feature or coordinator must reconcile completed, blocked, or waiting work into GitHub records and ThreadDock local locator state.
---
```

Required responsibilities:
- create/update PR with Issue, behavior, exact checks, review result, follow-up;
- Issue handoff with verified result, decisions, blockers, next owner;
- discover each relevant Project and use actual field options rather than guessing;
- Wiki/docs only when the change creates durable usage/operations knowledge;
- locator exact upsert/cleanup under shared lock;
- feature cleanup requires Issue closed + PR work finished + top-level session absent;
- no auto merge.

- [ ] **Step 4: Commit Task 3**

```bash
git add project-template/.agents/skills/tdd-task project-template/.agents/skills/review-change project-template/.agents/skills/publish-work
git commit -m "feat: add ThreadDock execution and publication skills"
```

---

### Task 4: Convert the old five skills into compatibility shims

**Files:**
- Modify: `project-template/.agents/skills/plan-work/SKILL.md`
- Modify: `project-template/.agents/skills/open-agent-session/SKILL.md`
- Modify: `project-template/.agents/skills/implement-task/SKILL.md`
- Modify: `project-template/.agents/skills/record-work/SKILL.md`
- Keep canonical implementation in: `project-template/.agents/skills/review-change/SKILL.md`

**Interfaces:**
- Old names remain discoverable for existing prompts, but delegate conceptually to canonical new skills.
- No duplicated long workflows in shims.

- [ ] **Step 1: Replace `plan-work` body with migration guidance**

Keep trigger compatible with existing requests, then state:
- project-level routing -> use `coordinate-work`;
- feature-level implementation planning -> use `develop-feature`, and `grill-plan` only when ambiguity warrants it.

- [ ] **Step 2: Replace `open-agent-session` body with migration guidance**

State that new project coordinators use `coordinate-work`, which owns session reuse/start and shared locator exact upsert. Preserve the warning to never guess Herdr IDs.

- [ ] **Step 3: Replace `implement-task` body with migration guidance**

State that bounded implementation workers use `tdd-task`.

- [ ] **Step 4: Replace `record-work` body with migration guidance**

State that durable PR/Issue/Project/Wiki/locator reconciliation uses `publish-work`.

- [ ] **Step 5: Verify shims are short**

Each shim should be a small migration note rather than a second implementation of the canonical skill. Target under ~150 words each.

- [ ] **Step 6: Commit Task 4**

```bash
git add project-template/.agents/skills/plan-work project-template/.agents/skills/open-agent-session project-template/.agents/skills/implement-task project-template/.agents/skills/record-work
git commit -m "refactor: route legacy skills to minimal orchestration"
```

---

### Task 5: Make installation and first use obvious

**Files:**
- Modify: `docs/operator/github-first-quickstart.md`
- Modify: `README.md`
- Modify: `Makefile`

**Interfaces:**
- Documentation exposes only the two agents and six canonical skills to new users.
- Existing users retain legacy skill compatibility.

- [ ] **Step 1: Rewrite the quickstart entrypoint**

Document project setup as:
1. Copy `project-template/.agents/skills` into target `.agents/skills`.
2. Copy `project-template/.codex/agents` into target `.codex/agents` when using Codex.
3. If target Codex config does not already enable agents, merge this stanza without overwriting existing model/project settings:

```toml
[agents]
enabled = true
max_concurrent_threads_per_session = 6
default_subagent_model = "gpt-5.6-sol"
default_subagent_reasoning_effort = "medium"
```

4. Use one global `/home/<user>/.threaddock/sessions.json` in Monitor settings.
5. Start the project agent with the short prompt:

```text
ThreadDock coordinator로 이 프로젝트를 맡아줘.
GitHub Issue/Project를 업무 원본으로 사용하고 coordinate-work로 열린 일을 정리해.
```

- [ ] **Step 2: Document the two-agent mental model**

Add a compact flow:

```text
Coordinator -> Herdr Feature Leader -> transient planner/implementer/reviewer -> PR/handoff
```

Explicitly state only Coordinator and Feature Leader are stored in ThreadDock locator/Monitor.

- [ ] **Step 3: Update README product usage summary**

Mention `2 top-level agents + skills + native subagents`, global sessions file, and that ThreadDock remains a monitor rather than runtime owner.

- [ ] **Step 4: Add a lightweight template validation target**

Extend `Makefile` with a shell-only `template-check` that verifies the two agent TOMLs and six canonical `SKILL.md` files exist and that each canonical skill contains `description: Use when`. Invoke `template-check` from `check` without adding Python or third-party dependencies.

Expected shell logic:

```make
CANONICAL_SKILLS := coordinate-work develop-feature grill-plan tdd-task review-change publish-work

template-check:
	@test -f project-template/.codex/agents/td_coordinator.toml
	@test -f project-template/.codex/agents/td_feature_leader.toml
	@for skill in $(CANONICAL_SKILLS); do \
		test -f "project-template/.agents/skills/$$skill/SKILL.md" || exit 1; \
		grep -q '^description: Use when' "project-template/.agents/skills/$$skill/SKILL.md" || exit 1; \
	done
```

- [ ] **Step 5: Run focused static validation**

Run:

```bash
make template-check
```

Expected: exit 0 with no missing agent/skill or invalid trigger description.

- [ ] **Step 6: Run repository integration gate once**

Run:

```bash
make check
```

Expected: exit 0. If the environment cannot execute it, record the exact missing tool/dependency instead of claiming success.

- [ ] **Step 7: Commit Task 5**

```bash
git add docs/operator/github-first-quickstart.md README.md Makefile
git commit -m "docs: simplify ThreadDock project onboarding"
```

---

## Self-review

- Spec coverage: the plan covers the two top-level agents, six canonical skills, transient subagent boundaries, global locator locking/cleanup, legacy skill migration, short user entrypoint, multiple projects, and factual ThreadDock runtime boundary.
- Placeholder scan: no TBD/TODO or undefined future implementation steps remain.
- Type/name consistency: canonical names are exactly `td_coordinator`, `td_feature_leader`, `coordinate-work`, `develop-feature`, `grill-plan`, `tdd-task`, `review-change`, and `publish-work` throughout.
- Skill pressure-scenario verification remains a runtime validation item because this ChatGPT tool environment cannot dispatch fresh Codex/Herdr subagents. Do not claim those skill behavior tests passed until run in a subagent-capable Codex/Herdr environment.
