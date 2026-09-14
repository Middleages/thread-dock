---
name: explore-codebase
description: Use when a bounded task needs code-flow, reuse-point, or affected-test evidence before implementation.
---

# Explore codebase

Answer the concrete repository question that was assigned. Start from the relevant files or symbols, follow their callers and data flow far enough to identify reuse points and affected tests, and distinguish observed facts from provisional assumptions.

## Evidence

Prefer the repository's existing search, navigation, and test names. Follow a path until the behavior, boundary, or ownership that matters to the question is clear; do not inventory unrelated directories. When several candidates exist, explain why a candidate is reusable or why it should remain separate.

Return a compact evidence report with paths and symbols, the relevant flow, reuse or extension points, affected tests, and any unresolved or unverified area. Read-only exploration is the default; do not edit files, delegate work, or publish results. Finish when every requested question has evidence or is explicitly marked unverified.
