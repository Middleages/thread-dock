# ThreadDock Agent Model + Monitor Refinement Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the Agent Model implementation plan so the Monitor has one canonical read model that separates execution, GitHub work, and verification state and presents Agent → Task → Result → Evidence clearly.

**Architecture:** Keep GitHub as durable work truth, Herdr as execution/session truth, and native Codex/OpenCode subagents as transient feature-internal execution. ThreadDock remains read/observation focused. The backend assembles one additive `agentWorkflow` read model; the frontend renders that model without independently reconstructing orchestration state.

**Tech Stack:** Go/Wails, React/TypeScript, existing GitHub/Herdr adapters, existing Go/Vitest tests. No new runtime/database dependency.

**Spec:** `docs/architecture/thread-dock-agent-model.md`

## Global Constraints

- Herdr `idle`/`done` is execution state, not business/work completion.
- Native child invocation/receipt proves execution observation, not substantive completion or verification.
- Verification evidence must be tied to the candidate SHA/revision when available.
- Missing, stale, or ambiguous evidence stays `unverified`/`partial`; never infer success.
- No scheduler, task/session DB, Agent runtime, model router, MCP orchestration, or mutation controls.
- Existing Monitor behavior remains valid when Agent observations are unavailable.

## Review Focus

- Herdr complete + GitHub work open: execution and work remain separate.
- Reviewer invoked without substantive result: verification remains unverified.
- Review evidence for an older SHA: cannot verify a newer candidate.
- Conflicting bindings: show ambiguity instead of inventing ownership.
- Legacy projects: no fabricated Agent activity.
- UI: consume the canonical backend read model; do not duplicate correlation logic.

---

## File/Component Map

| File | Responsibility | Change |
|---|---|---|
| `monitor/agent_model.go` | Canonical Agent/Task/Result/Evidence types and normalization | Create/extend |
| `monitor/agent_model_test.go` | Pure model/evidence tests | Create/extend |
| `monitor/herdr.go` | Adapt existing Herdr observations | Modify |
| `monitor/github_runtime.go` | Correlate GitHub work and Agent observations | Modify |
| `monitor/app.go` | Assemble/expose canonical `agentWorkflow` read model | Modify |
| `monitor/app_test.go` | Read-model/API fallback tests | Modify |
| `monitor/herdr_test.go` | Herdr mapping tests | Modify |
| `monitor/github_test.go` | Correlation tests | Modify |
| `monitor/frontend/src/AgentWorkflow.tsx` | Compact read-only workflow UI | Create |
| `monitor/frontend/src/AgentWorkflow.test.tsx` | Workflow UI tests | Create |
| `monitor/frontend/src/ProjectDetail.tsx` | Integrate Agent workflow beside Herdr summary | Modify |
| `monitor/frontend/src/ProjectDetail.test.tsx` | Integration coverage | Modify |

The backend read model is the boundary. `ProjectDetail`/`AgentWorkflow` must not each derive state from raw GitHub/Herdr data.

---

## Task 1: Canonical read-side Agent model

- [ ] Add failing Go tests for execution/verification normalization, Agent parent-child correlation, task/result identity, candidate revision matching, and unknown-not-completed behavior.
- [ ] Run the focused `go test ./monitor` tests and verify failure.
- [ ] Implement JSON-facing `AgentObservation`, `TaskObservation`, `TaskResult`, `EvidenceItem`, `ExecutionState`, and `VerificationState` types with explicit JSON tags.
- [ ] Keep Agent identity separate from Herdr session/workspace/pane links. Keep Task identity separate from execution identity.
- [ ] Normalize empty/unknown conservatively to `unknown`/`unverified`.
- [ ] Re-run focused tests and commit `feat: add ThreadDock agent observation model`.

## Task 2: Canonical backend Monitor read model

- [ ] Add failing adapter tests proving Herdr completion does not complete GitHub work, deterministic Issue/PR correlation, ambiguity handling, and legacy/no-Agent fallback.
- [ ] Reuse the existing Herdr polling/snapshot path; do not add another polling loop or write to `sessions.json`.
- [ ] Correlate using existing Issue/PR/repository/worktree/session bindings only. Prefer current PR candidate SHA as verification anchor.
- [ ] Assemble a single additive `agentWorkflow` read model containing Agent identity/lineage, tasks/results, execution state, GitHub work state, verification state, candidate revision, evidence refs, and notices.
- [ ] Expose it through the existing Wails read API. Do not add a new server, persistence layer, or task store.
- [ ] Keep source observations and derived verification state distinct in the response.
- [ ] Run `go test ./monitor -v` and commit `feat: correlate agent observations with work`.

## Task 3: Conservative result/evidence mapping

- [ ] Add failing tests for receipt-as-execution-evidence, matching review evidence => verified, stale review evidence => unverified, and incomplete child result => partial.
- [ ] Treat native receipt metadata as execution evidence only; invocation is not substantive review.
- [ ] Permit `verified` only when substantive review evidence exists and its candidate revision matches the current candidate.
- [ ] Preserve timeout/error/incomplete results and evidence source references.
- [ ] Do not reconstruct missing transcripts or claim authenticated attestation.
- [ ] Verify malformed/absent evidence leaves the project observable while producing `unverified` with a notice.
- [ ] Run `go test ./monitor -v` and commit `feat: model agent results and evidence`.

## Task 4: Monitor UI reads the canonical model

- [ ] Add failing tests for independent execution/work/verification states, Agent lineage, candidate SHA, evidence source, stale/unverified notices, ambiguity, and empty legacy state.
- [ ] Implement `AgentWorkflow.tsx` as a compact read-only section comparable to `HerdrSummary`.
- [ ] Consume the backend `agentWorkflow` shape directly or through only a thin typed presentation adapter. Do not query GitHub/Herdr independently.
- [ ] Show lineage as a small tree/ordered list, not a graph in Phase 1.
- [ ] Use explicit verification wording: `확인됨`, `부분 확인`, `미확인`.
- [ ] Show execution state, GitHub work state, verification state, candidate SHA, evidence sources, and the reason for blocked/stale/ambiguous/unverified states.
- [ ] Integrate next to existing Herdr summary; do not replace it.
- [ ] Add no create/stop/retry/approve/reassign controls.
- [ ] Run Vitest and `npm run build`; commit `feat: show structured agent workflow in monitor`.

## Task 5: End-to-end verification

- [ ] Run `make check`.
- [ ] Run `wails build` from `monitor`.
- [ ] Exercise fixtures for: running Agent + active work; completed execution + active work + unverified; matching review + candidate SHA + verified.
- [ ] Verify stale evidence cannot verify a newer candidate and ambiguity remains visible.
- [ ] Run GHES + WSL + Herdr smoke where available and record unavailable checks explicitly.
- [ ] Perform a fresh responsibility review against the design spec; fix only concrete defects.

## Phase 1 Exit Criteria

1. Agent identity and parent/child lineage are visible.
2. Agent → Task → Result → Evidence is explicit in the read-side API without durable task storage.
3. Monitor has one canonical read model.
4. Execution, GitHub work, and verification are independently visible.
5. Invocation without substantive matching evidence remains unverified.
6. Matching candidate SHA plus substantive review evidence can become verified.
7. Stale evidence cannot verify a newer candidate.
8. Missing/ambiguous observations never become fabricated success.
9. Existing projects still work without Agent observations.
10. No scheduler/runtime/persistence/MCP orchestration/mutation layer is introduced.
11. Relevant verification gates pass, with environment limitations recorded.

## Deferred Phase 2

Only after real usage proves need: richer child timeline, richer handoff/result templates, selective intervention gates, deeper native receipt adapters, and task-graph visualization. Do not add Agent-to-Agent chat/rooms unless structured Task/Result proves insufficient.
