---
name: plan-work
description: Use when refining a development request into an approved, version 1 work contract.
---

# Plan work

1. Read `CONTEXT.md` and the relevant `CODEMAP.md` section. Use Serena symbol and reference discovery to verify the affected boundaries and tests. Completion: the evidence identifies the repository areas the request can change.
2. Draft exactly one contract version `1` JSON object with only `version`, `parent`, `children`, `repository`, `baseCommit`, `tasks`, `protectedPaths`, and `verification`. Parent and Child Issue objects use `key`, `title`, `body`, `acceptanceCriteria`, and `labels`; `children` is optional. Repository uses `owner`, `name`, and `defaultBranch`. Completion: every Task `issueKey` references the Parent or a Child.
3. Give every Task `id`, `issueKey`, `owner`, `role`, `branch`, `allowedPaths`, `dependsOn`, `acceptanceCriteria`, and `verification`; keep allowed paths non-overlapping. Keep excluded scope, risks, and approval context in Issue bodies, `protectedPaths`, and the preview rather than invented top-level keys. Completion: the JSON shape matches the contract and protected paths are explicit.
4. Run `agentctl contract validate CONTRACT.json`, then `agentctl contract preview CONTRACT.json`; resolve validation failures. Completion: validation succeeds and the preview is ready to review.
5. Present the preview as one approval bundle. Stop for approval. Completion: do not create Issues, branches, Pull Requests, or other external writes until the bundle is approved.
