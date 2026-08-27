---
name: plan-work
description: Use when refining a development request into an approved, version 1 work contract.
---

# Plan work

1. Read `CONTEXT.md` and the relevant `CODEMAP.md` section. Use Serena symbol and reference discovery to verify the affected boundaries and tests. Completion: the evidence identifies the repository areas the request can change.
2. Draft contract version `1`: one Parent Issue for the outcome, only independently valuable Child Issues, and Tasks with one owner, non-overlapping allowed paths, acceptance criteria, dependencies, and verification. Completion: excluded and protected paths are explicit.
3. Run `agentctl contract validate CONTRACT.json`, then `agentctl contract preview CONTRACT.json`; resolve validation failures. Completion: validation succeeds and the preview is ready to review.
4. Present the Parent Issue, Child Issues, Tasks, excluded scope, verification, risks, and approval boundary as one bundle. Stop for approval. Completion: do not create Issues, branches, Pull Requests, or other external writes until the bundle is approved.
