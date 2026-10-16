---
name: grill-plan
description: Use when a Feature Leader's significant plan needs independent challenge or an unresolved material design disagreement needs adjudication.
---

# Grill plan

The Feature Leader owns one feature plan. Goal/product ambiguity is refined by the Coordinator with `coordinate-work`; missing implementation detail alone is not a reason to question the user. Read [the shared development stance](../develop-feature/references/development-stance.md).

Accept an already-agreed plan as the starting point: check feasibility and supporting facts, reuse valid decisions/review evidence, and avoid rewriting or reopening approval of unchanged choices. The owner may revise implementation details with rationale. Route cross-feature contracts/dependencies to the Coordinator and undelegated changes to user-decided product scope/behavior to the user.

A small clear fix needs only a short implementation approach. For a significant new plan or substantive changed risk (shared/public interface, persistent data/migration, cross-module ownership, security/external effects), make scope, evidence/assumptions, interfaces, dependencies, failure/recovery, tasks/ownership and acceptance verification concrete. Complexity or risk warrants this review even when product intent is already clear. A planning-only assignment stays planning-only.

## Review the plan, not future implementation results

1. Freeze a plan revision and send it, the original brief, and relevant raw facts to one fresh independent adversarial reviewer in **plan-review mode**. Ask for concrete feasibility/failure cases and simpler alternatives with consequences, not a quota of objections. The reviewer returns findings without editing or delegating.
2. The plan owner resolves evidence-backed findings: adopt the necessary correction or explain a rejection with source evidence. If owner and reviewer agree, finalize the plan with those decisions; do not dispatch a neutral adjudicator. Recheck only a changed substantive risk or genuinely unresolved finding, not the whole plan or a preference-only difference.
3. Only if a material disagreement remains after that resolution, send the fixed plan, disputed findings, the author's response, and shared evidence to one different fresh reviewer in **adjudication mode**. It records accepted/rejected reasoning and the smallest supported resolution, preserving user authority. One adjudication is enough for that dispute. If it still cannot resolve a material decision, identify the specific missing fact or human choice, hold only dependent work, and continue independent authorized work; do not restart an endless review/debate cycle.

The coordinating plan owner dispatches these bounded reviews. Reviewers/adjudicators consume the supplied fixed plan and return their mode-specific result; they do not become new plan owners or recursively run this workflow. Use existing allowed roles/models without escalation. Unavailable required review is explicitly unverified, not replaced by self-review.

Distinguish a missing technical fact from a user decision when resolving a dispute. If alternatives preserve agreed behavior, compatibility and external effects, the Feature Leader owns the technical choice, including protocol or data-format choices. Missing evidence returns as a concrete fact to obtain with its owner/limit, not a request for the user to choose an implementation. Escalation to the user must identify the undelegated change to product scope, acceptance, behavior or authority that needs their choice; an API-related disagreement alone does not establish one.

## Review inputs and evidence

- **plan-review:** original brief/acceptance, plan reference/revision, relevant source facts and proposed verification. Judge feasibility, scope, dependencies and missing source evidence. Identify a real unknown as such; do not require passing tests, screenshots, or a code SHA for an implementation that does not exist yet.
- **adjudication:** the same plan/evidence plus the specific material disagreement and both positions. Resolve only that dispute, or identify the bounded missing decision.
- **code-review:** a completed candidate's SHA/diff and actual acceptance/check evidence belongs to `review-change`. Its rendered-UI and executed-test requirements still apply at that stage.

Return the feature plan reference/revision, resolved findings with rationale, any remaining material decision, ownership and verification approach to `develop-feature`. This is planning input, not a second execution-state store.
