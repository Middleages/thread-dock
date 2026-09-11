---
name: plan-work
description: Use when an existing prompt still asks for the legacy ThreadDock planning skill.
---

# Plan work compatibility

This is a compatibility entrypoint for older ThreadDock prompts.

For project-level GitHub routing, feature decomposition, Herdr top-level session reuse/start, and shared locator maintenance, use **REQUIRED SUB-SKILL:** `coordinate-work`.

For one Feature Leader's implementation lifecycle, use **REQUIRED SUB-SKILL:** `develop-feature`. Use `grill-plan` only when the feature is materially ambiguous, cross-cutting, or assumption-heavy.

Do not maintain a separate legacy planning protocol or duplicate orchestration state under this skill name.
