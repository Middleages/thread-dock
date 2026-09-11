---
name: grill-plan
description: Use when a development request is too ambiguous, cross-cutting, or assumption-heavy to hand safely to implementation subagents.
---

# Grill plan

Turn ambiguity into an implementation-ready decision record. Do not create a second task database, workflow engine, or orchestration state.

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

Return a compact plan to the Feature Leader with task boundaries, ownership, acceptance evidence, and remaining human decisions. The output is planning input for `develop-feature`, not durable execution state by itself.
