---
name: plan-work
description: Use when a development request needs an immutable Contract v2 draft before any Issue, branch, PR, or other external write.
---

# Plan work

1. Read the request, `CONTEXT.md`, the relevant `CODEMAP.md` section, and the fixed Contract v2 interface. Verify repository identities, base SHAs, task owners, branches, allowed paths, dependencies, and checks; stop with missing facts rather than inventing values.
2. Draft exactly one immutable JSON object with `version: 2`: `workId`, `projectId`, positive `revision`, `request`, `acceptanceCriteria`, `repositoryPlans`, `tasks`, `interfaceAgreements`, `crossRepoVerification`, `issueDrafts`, `documentation`, `executionProfiles`, and `decisionRefs`. Do not add v1 fields or model/provider settings.
3. For each repository plan record exact `repoKey`, `baseSha`, `targetBranch`, `verification`, `prOrder`, and `mergeOrder`. Commands use exactly one `argv` or explicit `shellScript`, the repository `cwdRepoKey`, and positive `timeoutSeconds`. For each Task record `taskId`, `repoKey`, owner/role/branch, non-overlapping `allowedPaths`, `dependsOn`, acceptance criteria, and the same command shape. Check DAG references, path scope, and repository references against Contract v2 validation semantics.
4. Each `interfaceAgreement` records `agreementId`, `summary`, `repoKeys`, and `decisionRefs`. Use logical `issueDrafts` for the Parent and any independently trackable Child Issue: each has a stable `key`, `title`, `body`, `acceptanceCriteria`, optional `labels`, and target `repoKey`. Put risks, assumptions, excluded behavior, and approval context in the Parent body. Record `documentation` with `required`, `repository` or `wikiTargets`, `allowedPaths`, and an explicit `reason` when no documentation is required; fill `executionProfiles.builder`, `.reviewer`, `.documenter`, and every `decisionRefs` reference.
5. Run `agentctl contract validate CONTRACT.json`, then `agentctl contract preview CONTRACT.json`. Present the preview, validation result, and unresolved assumptions as one approval bundle. Stop for operator approval; create no Issue, branch, PR, or other external write before approval.
