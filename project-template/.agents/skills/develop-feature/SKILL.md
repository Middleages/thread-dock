---
name: develop-feature
description: Use when a Feature Leader owns one approved GitHub Issue or independently deliverable feature through implementation and PR readiness.
---

# Develop feature

Own one feature from accepted scope to reviewed PR readiness. GitHub is the durable record; the Feature Leader coordinates native subagents inside one Herdr top-level session.

If this workflow needs any Herdr CLI operation, use **REQUIRED SUB-SKILL:** `herdr-local` first and satisfy its local-only gate before session, layout, pane, or Agent discovery/control. If the gate cannot be satisfied, leave the operation pending and report the blocker.

1. Read the Issue, acceptance criteria, repository instructions, dependencies, and shared-interface warnings.
2. If requirements are materially ambiguous, assumption-heavy, or cross interfaces, use `grill-plan`. Skip it for small, well-specified work.
3. Split only implementation tasks that can write independently. Prefer 1-3 workers; never create workers just to fill capacity. For each task, preserve the source Issue/request link, original acceptance criteria, and edge-case output or failure meaning. Include only its bounded owned paths, feature worktree/branch, dependency inputs, and shared-interface warnings.
4. Dispatch each packet with the matching Skill and named native leaf when the harness supports it: `td_explorer` with `explore-codebase`, `td_docs_editor` with `write-project-docs`, `td_implementer` with `tdd-task`, and `td_reviewer` with `review-change`. If a harness has no named leaf, pass that same Skill, packet, and bounded scope to an allowed native agent; do not reinterpret the request, bypass a permission denial, add a fallback model, or turn a helper into a top-level session. Use exploration or documentation helpers only when the feature needs them; this is not a fixed pipeline.
5. Integrate returned changes in the feature worktree and run only affected focused verification. Preserve exact commands, outcomes, and candidate SHA.
6. Dispatch a fresh implementation-independent reviewer with **REQUIRED SUB-SKILL:** `review-change` against the exact candidate SHA/diff and acceptance evidence.
7. If review blocks, assign the smallest bounded fix and re-review the new SHA. If the same root cause repeats twice, replan instead of blindly retrying.
8. When ready, use **REQUIRED SUB-SKILL:** `publish-work` for PR, Issue handoff, Project fields, durable docs when warranted, and locator reconciliation.

Internal planner, implementer, reviewer, and publish/docs helpers are transient native subagents. Do not store them as ThreadDock top-level bindings. Worker full-suite runs are not the default; reserve the repository-wide gate for final integration when required. Human merge remains final.
