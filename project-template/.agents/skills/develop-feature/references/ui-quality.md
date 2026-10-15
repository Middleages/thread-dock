# UI quality

Read for changes to rendered screens, controls, layout, or UI copy. The Feature Leader owns orchestration; implementation remains with the assigned implementation child. Review by screen or user flow, not by individual word.

## Choose a coherent direction

A typo or an exact, unambiguous label correction can go directly to implementation and the rendered check below. For a new or materially revised flow, information hierarchy, or body of copy:

1. Give independent native agents the same brief, product facts, current screen, original acceptance/edge cases, and constraints. Ask for candidate screen/flow wording and hierarchy from new-user, frequent-operator, and editor/accessibility perspectives. Each returns a proposal only, without product edits or seeing the others. These are temporary assignments to allowed agents, not new Herdr sessions or model configurations. Run sequentially if capacity is limited while keeping contexts independent.
2. Shuffle proposals into neutral A/B/C labels and withhold author identity, model, and revealing filenames where practical. Send them to a fresh evaluator who authored none of them. Compare task clarity, preserved meaning (especially error/unknown states), consistency, accessibility, and visual fit; identify a preferred direction with reasons and any critical defect. A majority vote or blending every suggestion is not a decision rule.
3. Assign one editor (which may be the chosen author) the selected direction and feedback for a coherent final revision. Preserve product truth and all acceptance conditions; resolve conflicting suggestions explicitly. The implementation child applies that version and verifies it in the actual rendered UI. Revisit only defects demonstrated by the check.

For any required perspective/evaluator that cannot run, record that missing evaluation explicitly; a single agent adopting several personas is not independent evidence. Continue permitted implementation/checks and report the limitation rather than claiming completion of the comparison.

## Inspect what users see and do

Every UI change requires both a rendered screenshot opened and visually inspected by an image-capable agent, and execution of the relevant interaction flow. Screenshot capture, image filenames, DOM/accessibility snapshots, unit tests, and builds alone do not constitute visual review.

Use the available browser/image path on Codex. On OpenCode, prefer an already-configured Playwright MCP or existing Playwright CLI; existing `agent-browser` is an alternative. Inspect actual tool support before choosing; do not install redundant tools or automatically switch models to obtain vision. A browser tool returning only DOM text still needs an image-capable agent to open the screenshot. An available image-reading tool is useful evidence only after it successfully opens the rendered image.

Capture affected normal and relevant empty/error/loading states at representative supported sizes; inspect readability, clipping, hierarchy, contrast, spacing, and the final copy in context. Exercise the changed action and visible result, including relevant keyboard/focus and error/recovery behavior. Keep checks proportional: a typo needs its affected screen and control flow, not a full product tour. Fix observed defects and recheck affected evidence.

Record candidate revision/SHA, tool and render environment/fixture, viewport/state, screenshot artifact, who actually opened it and their concrete visual observations, interaction actions/results, observed console/runtime errors, and unresolved limits. Browser-rendered React with mocked bindings is UI evidence, not native Wails or live GitHub/Herdr evidence; record those separately.

If rendering, image input, the selected model's vision, or interaction tooling is unavailable, state exactly which verification is unverified and why. Keep the implemented work and available checks, but do not report UI acceptance or visual verification as passed. Escalate only the actual missing capability or a scope-changing choice; do not invent an additional approval gate.
