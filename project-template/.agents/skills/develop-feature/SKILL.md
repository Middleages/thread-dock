---
name: develop-feature
description: Use when a Feature Leader owns one approved GitHub Issue or independently deliverable feature through implementation and PR readiness.
---

# Develop feature

Own one feature from accepted scope to reviewed PR readiness. GitHub is the durable record; the Feature Leader coordinates native subagents inside one Herdr top-level session.

1. Read the Issue, acceptance criteria, repository instructions, dependencies, and shared-interface warnings.
2. If requirements are materially ambiguous, assumption-heavy, or cross interfaces, use `grill-plan`. Skip it for small, well-specified work.
3. Split only implementation tasks that can write independently. Prefer 1-3 workers; never create workers just to fill capacity.
4. Dispatch implementation subagents with bounded paths, acceptance criteria, dependency inputs, and **REQUIRED SUB-SKILL:** `tdd-task`.
5. Integrate returned changes in the feature worktree and run only affected focused verification. Preserve exact commands, outcomes, and candidate SHA.
6. Dispatch a fresh implementation-independent reviewer with **REQUIRED SUB-SKILL:** `review-change` against the exact candidate SHA/diff and acceptance evidence.
7. If review blocks, assign the smallest bounded fix and re-review the new SHA. If the same root cause repeats twice, replan instead of blindly retrying.
8. When ready, use **REQUIRED SUB-SKILL:** `publish-work` for PR, Issue handoff, Project fields, durable docs when warranted, and locator reconciliation.

Internal planner, implementer, reviewer, and publish/docs helpers are transient native subagents. Do not store them as ThreadDock top-level bindings. Worker full-suite runs are not the default; reserve the repository-wide gate for final integration when required. Human merge remains final.
