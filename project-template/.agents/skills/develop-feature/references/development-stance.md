# Shared development stance

Apply these criteria while planning, implementing, and reviewing; include task-relevant constraints in the first packet.

- Solve the stated behavior in the current architecture. Reuse existing names, interfaces, and sources of truth before adding a mechanism.
- When new structure is needed, prefer the project's conventional layers such as model/schema, service, repository, and UI over introducing feature folders. Create only layers with actual responsibilities; a small change need not acquire every layer. Preserve an established feature-based structure unless reorganization is part of the task.
- Prefer a direct path with few concepts. New abstraction, configuration, state, validation, or workflow gates need a concrete present requirement or failure case. Derive values the code/tool already knows instead of asking for duplicate input. Preserve protections for actual risks and enough operation/target/cause context to diagnose failures.
- Keep public/shared interfaces explicit before parallel implementation. Preserve the original request's conditions, negative cases, uncertainty, and user-visible meaning through delegation and editing.

Use installed complementary skills only where they add needed work: `impeccable` for UI critique/copy/layout (Operate mode for tools), `codebase-design` for a difficult interface or test seam, `domain-modeling` for conflicting domain terms/state meaning, and `writing-for-agents` when changing agent instructions. Read the relevant skill when that branch applies. Their availability is not a prerequisite for routine work; keep the same outcome criteria if one is absent. Do not install duplicates, change model settings, reorganize folders, or add glossary/ADR documents merely to follow a skill.
