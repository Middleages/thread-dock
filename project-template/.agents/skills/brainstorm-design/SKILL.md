---
name: brainstorm-design
description: Use when a substantial feature or architectural change needs design-space exploration before committing to an implementation direction.
---

# Brainstorm design

Explore viable implementation directions before `grill-plan` hardens one into an implementation-ready plan. This is for substantial, novel, or architecture-affecting work; skip it for small, well-specified changes.

Start from the source Issue/request, repository guidance, relevant code, and known constraints. Prefer repository evidence over invented assumptions.

Produce 2-4 meaningfully different viable approaches when alternatives truly exist. For each approach, compare:

- core mechanism and affected boundaries;
- compatibility and migration impact;
- concurrency, data/state, failure and recovery behavior;
- operational and maintenance cost;
- testability and observability;
- what can remain simple now versus what would prematurely add infrastructure.

Reject options that violate fixed repository constraints or add a second scheduler, task database, execution protocol, or other architecture the request does not require.

Converge on the smallest approach that satisfies the known constraints, but do not hide unresolved trade-offs. Record the chosen direction, rejected alternatives with brief reasons, provisional assumptions, and decisions that still require a human.

Return design input to the Feature Leader. The result is not durable execution state by itself. If important requirements remain ambiguous, follow with `grill-plan` before implementation.
