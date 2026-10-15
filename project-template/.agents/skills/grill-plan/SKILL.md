---
name: grill-plan
description: Use when a development plan changes shared interfaces, persistent state, cross-module ownership, or consequential assumptions, or needs ambiguity resolved before implementation.
---

# Grill plan

Turn ambiguity into an implementation-ready decision record. Do not create a second task database, workflow engine, or orchestration state.

Read [the shared development stance](../develop-feature/references/development-stance.md). A bounded local fix, typo, or routine docs/config edit with clear acceptance needs only a short plan; skip the independent planning cycle below. Significant plans change public/shared interfaces, persistent data or migrations, cross-module ownership, security/external side effects, or a consequential unresolved tradeoff. A clear desired outcome does not by itself make these small.

Interrogate the request until the Feature Leader can state:

- problem and why it matters;
- explicit non-goals;
- testable acceptance criteria;
- hidden assumptions and which are verified versus provisional;
- edge cases and failure/recovery behavior;
- interfaces, shared files, data/state transitions, and compatibility constraints;
- dependencies and prerequisites;
- tasks that can truly run in parallel;
- boundaries that must stay serialized;
- unresolved decisions that require the human rather than agent guessing.

Prefer the smallest question or repository/GitHub inspection that removes a meaningful ambiguity. Do not expand a clear task merely to produce a longer plan.

## Independent review for significant plans

The coordinating author owns the following native-agent dispatch; an assigned leaf reviewer returns its result without delegating.

1. Author a concrete plan with source facts, assumptions, alternatives, ownership, and acceptance evidence. Freeze a revision so every review refers to the same plan.
2. Dispatch a fresh adversarial reviewer with that plan and the raw relevant evidence. Ask for concrete failure scenarios, overlooked dependencies, and simpler alternatives with consequences; no quota of objections. Unsupported preferences and invented problems are not blockers. The reviewer did not author the plan and returns findings without changing it.
3. Dispatch a different fresh neutral adjudicator with the same plan/evidence and the adversarial findings. It records each accepted or rejected finding and its reason, the smallest justified revision, and any unresolved human decision. It does not rubber-stamp either side, broaden scope, or decide a product preference the user has not delegated.
4. The author applies accepted changes and records the final plan with the rationale for rejected findings. Re-review only changed material or unresolved concrete risks. Use this plan for implementation packets.

Sequential dispatch is sufficient; independence means separate contexts, not concurrency or model diversity. Use existing allowed roles/models, never automatic escalation. If independent dispatch is unavailable, state which review is missing and continue only work that does not depend on the unresolved decision; do not relabel self-review as independent.

Return a compact plan to the Feature Leader with task boundaries, ownership, acceptance evidence, and remaining human decisions. The output is planning input for `develop-feature`, not durable execution state by itself.
