# UI quality

Read for changes to rendered screens, controls, layout, or UI copy. The Feature Leader owns orchestration; implementation remains with the assigned implementation child. Review by screen or user flow, not by individual word.

## Choose a coherent direction

Reuse agreed copy, layouts and existing components. A specified edit or a known pattern goes directly to implementation and the relevant rendered check.

When an unresolved user-flow or design choice has meaningfully different outcomes, compare the viable directions against task clarity, preserved meaning (especially error/unknown states), consistency and accessibility. The Leader may request independent proposals for the uncertain aspect. Choose the needed perspectives; no fixed panel size, anonymization ceremony or separate evaluator is required. The Leader records a coherent choice and reasons, not a majority vote or a blend of every suggestion. Reuse that reasoning in any required plan review.

Final code review remains independent of implementation. If a specific independent design evaluation was required by the user or acceptance, obtain it or mark it missing; one agent adopting several personas is not independent evidence.

## Inspect what users see and do

Every UI change requires a rendered screenshot opened and visually inspected by an image-capable agent. For copy/style-only changes, inspect the affected state. When behavior, state transitions, navigation, keyboard/focus or recovery changes, execute the changed paths and relevant failure cases. Screenshot capture, image filenames, DOM/accessibility snapshots, unit tests and builds alone do not constitute visual review.

Use the available browser/image path on Codex. On OpenCode, prefer an already-configured Playwright MCP or existing Playwright CLI; existing `agent-browser` is an alternative. Inspect actual tool support before choosing; do not install redundant tools or automatically switch models to obtain vision. A browser tool returning only DOM text still needs an image-capable agent to open the screenshot. An available image-reading tool is useful evidence only after it successfully opens the rendered image.

Capture affected normal and relevant empty/error/loading states at representative supported sizes; inspect readability, clipping, hierarchy, contrast, spacing and copy in context. Keep checks proportional to the change and fix observed defects before rechecking affected evidence.

Record candidate revision/SHA, tool and render environment/fixture, viewport/state, screenshot artifact, who actually opened it and their concrete visual observations, interaction actions/results, observed console/runtime errors, and unresolved limits. Browser-rendered React with mocked bindings is UI evidence, not native Wails or live GitHub/Herdr evidence; record those separately.

If rendering, image input, the selected model's vision, or interaction tooling is unavailable, state exactly which verification is unverified and why. Keep the implemented work and available checks, but do not report UI acceptance or visual verification as passed. Escalate only the actual missing capability or a scope-changing choice; do not invent an additional approval gate.
