---
name: plan-work
description: Use when refining a development request into an approved, version 1 work contract.
---

# Plan work

1. Read `CONTEXT.md` and the relevant `CODEMAP.md` section. Use Serena symbol and reference discovery to verify the affected boundaries and tests. Completion: the evidence identifies the repository areas the request can change.
2. Gather repository `owner`, `name`, `defaultBranch`, `baseCommit`, Task owner identities, branch names, and verification commands from the repository or environment. Stop with a concise missing-facts list before validation or preview when any are unavailable; never use illustrative values. Completion: every contract fact is verified.
3. Draft exactly one contract version `1` JSON object with only `version`, `parent`, `children`, `repository`, `baseCommit`, `tasks`, `protectedPaths`, and `verification`. Parent and Child Issue objects use `key`, `title`, `body`, `acceptanceCriteria`, and `labels`; `children` is optional. Repository uses `owner`, `name`, and `defaultBranch`. Completion: every Task `issueKey` references the Parent or a Child.
4. Give every Task `id`, `issueKey`, `owner`, `role`, `branch`, `allowedPaths`, `dependsOn`, `acceptanceCriteria`, and `verification`. `protectedPaths` contains only protected or excluded path globs and must include `migrations/**`, `authentication/**`, `.github/workflows/**`, and `deployment/**`; it may extend but never replace them. Completion: paths are non-overlapping and every required protected path is present.
5. Keep `allowedPaths` disjoint from `protectedPaths`. If the request needs a protected path, mark the bundle as needing an operator/protected-change decision rather than assigning that path to a Task. Keep risks, assumptions, excluded non-path behavior, and approval context in Parent or Child Issue bodies and the rendered preview; invent no new top-level contract fields. Child Issue keys may be local drafts before external creation; label them as drafts in the body or preview, never as GHES Issue numbers. Completion: the JSON shape matches the contract.
6. Run `agentctl contract validate CONTRACT.json`, then `agentctl contract preview CONTRACT.json`; resolve validation failures. Completion: validation succeeds and the preview is ready to review.
7. Present the preview as one approval bundle. Stop for approval. Completion: do not create Issues, branches, Pull Requests, or other external writes until the bundle is approved.
