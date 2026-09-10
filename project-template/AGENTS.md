# Repository rules

- Change only the assigned paths and keep unrelated dirty work intact.
- Use GitHub Issues, Projects, PRs, and Wiki as the durable project record.
- A central coordinator assigns independently writing features to top-level Herdr sessions. Each feature leader owns a feature worktree and may use native subagents for detail planning, coding, and independent review.
- Record rationale, acceptance criteria, decisions, handoffs, checks, review results, blockers, and links in GitHub. Discover every relevant Project membership for the Issue/request, read each board's options, and keep status, priority, and waiting fields aligned per board. Keep unavailable-board failures distinct and continue the Issue/PR flow.
- Use native `gh` or connected GitHub MCP tools with existing authorization. Query actual IDs and field options; do not invent external identifiers.
- Do not require Go orchestration, Contract v2, Invocation or Artifact envelopes, publisher/repair services, a monitor, or `.threaddock/sessions.json` to begin work.
- Keep session/worktree/pane associations as explicit locators when available. Preserve every worktree; an operator decides any forced deletion.
- Never claim checks, writes, reviews, or completion without evidence for the exact commit. Humans perform the final merge into `main`.
