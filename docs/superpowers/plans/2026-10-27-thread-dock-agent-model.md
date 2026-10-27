# ThreadDock Agent Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ThreadDock able to observe and present a structured Agent → Task → Result → Evidence model without becoming an execution runtime.

**Architecture:** Keep GitHub as durable work truth, Herdr as top-level execution/session truth, and native Codex/OpenCode subagents as feature-internal execution. ThreadDock adds a read-side domain model that correlates existing observations and supplied native receipts; it does not persist or schedule Agent work. Execution state, business/work state, and verification state remain separate.

**Tech Stack:** Go/Wails backend, React/TypeScript frontend, existing GitHub and Herdr adapters, existing Vitest/Go tests, no new runtime/database dependency.

**Spec:** `docs/architecture/thread-dock-agent-model.md`

## Global Constraints

- GitHub Issue/PR/Projects remain the durable work source; ThreadDock must not become a second task database.
- Herdr remains the source for top-level session/pane/worktree/Agent execution observation; Herdr `idle`/`done` is not work completion.
- Native subagent execution remains transient and is not copied into `sessions.json` as a new top-level session.
- A child invocation/receipt proves that an execution was requested or observed; it does not by itself prove substantive completion or verification.
- Candidate revision/SHA must be attached to verification evidence whenever available.
- Unsupported or missing evidence is represented as `unverified`/`partial`, never upgraded to success by inference.
- No scheduler, lease/recovery engine, task DB, session DB, model router, permission system, MCP orchestration layer, or agent runtime is introduced.
- Phase 1 is read/observation focused; mutation remains outside the new model.

## Review Focus

- Herdr reports `done` while GitHub work remains open → UI/API must keep execution and work state distinct; covered by Task 2.
- A reviewer child is invoked but produces no usable result → verification remains `unverified`; covered by Task 3.
- Evidence refers to an older SHA than the current candidate → verification must not silently apply to the new revision; covered by Task 3.
- Multiple top-level bindings refer to the same Issue/PR → correlation must remain deterministic and surface ambiguity instead of inventing ownership; covered by Task 2.
- Old projects have no Agent/Task evidence → existing Monitor behavior remains valid and the new section is empty/unknown rather than fabricating state; covered by Task 4.

---

## 1. File/Component Map

The implementation should stay close to the existing Monitor boundaries rather than introducing a new service layer.

| File | Responsibility | Change |
|---|---|---|
| `monitor/agent_model.go` | Canonical read-side Agent/Task/Result/Evidence types and normalization helpers | Create |
| `monitor/agent_model_test.go` | Pure model/correlation tests | Create |
| `monitor/herdr.go` | Adapt existing Herdr observation into Agent observations and preserve existing snapshot semantics | Modify |
| `monitor/github_runtime.go` | Correlate GitHub work with observed Agent/Task data without making GitHub a runtime store | Modify |
| `monitor/app.go` | Expose the structured observation through the existing Wails read API | Modify |
| `monitor/app_test.go` | API contract and fallback tests | Modify |
| `monitor/frontend/src/AgentWorkflow.tsx` | Compact Agent/Task/verification observation UI | Create |
| `monitor/frontend/src/AgentWorkflow.test.tsx` | UI state and evidence rendering tests | Create |
| `monitor/frontend/src/ProjectDetail.tsx` | Place Agent workflow observation beside existing Herdr summary | Modify |
| `monitor/frontend/src/ProjectDetail.test.tsx` | Project-detail integration coverage | Modify |
| `docs/architecture/thread-dock-agent-model.md` | Design/spec already merged to `main`; implementation must conform to it | Reference only |

Do not split existing large Go files merely for aesthetic reasons. The new `agent_model.go` is a focused domain boundary; adapters stay in their current files until actual size/ownership pressure justifies further extraction.

---

## Task 1: Add the canonical read-side Agent model

**Files:**
- Create: `monitor/agent_model.go`
- Test: `monitor/agent_model_test.go`

**Interfaces:**
- Produces `AgentObservation`, `TaskObservation`, `TaskResult`, `EvidenceItem`, `VerificationState`, and `ExecutionState` types used by later tasks.
- Produces deterministic normalization/correlation helpers; these functions must be pure and must not call GitHub, Herdr, Wails, or the filesystem.

- [ ] **Step 1: Write failing model tests**
  - `TestNormalizeExecutionState`
  - `TestNormalizeVerificationState`
  - `TestTaskResultRequiresMatchingRevisionForVerifiedEvidence`
  - `TestCorrelateChildToParentByTaskID`
  - `TestUnknownObservationDoesNotBecomeCompleted`

  Pin the exact state vocabulary from the spec:
  - execution: `queued`, `running`, `waiting`, `completed`, `failed`, `cancelled`, `unknown`
  - verification: `unverified`, `partial`, `verified`
  - task result must retain `taskID`, `status`, and optional `revision`/evidence references.

- [ ] **Step 2: Run the focused Go tests and verify failure**
  - Run: `go test ./monitor -run 'Test(Normalize|TaskResult|Correlate|Unknown)' -v`
  - Expected: FAIL because the model types/helpers do not yet exist.

- [ ] **Step 3: Implement `monitor/agent_model.go`**
  - Define JSON-facing structs with explicit `json` tags.
  - Keep Agent identity separate from Herdr location: Agent identity is `agentID`, `role`, `parentAgentID`; Herdr session/workspace/pane remain observation links.
  - Define Task identity separately from execution identity: `taskID`, `parentTaskID`, `role`, `objective`, `inputRevision`, `acceptance`.
  - Define Result with `status`, `revision`, `findings`, `verification`, `nextAction`, and evidence references.
  - Define Evidence with a source kind (`github`, `herdr`, `native_event`, `test`, `review`), locator/reference, candidate revision, and observation timestamp where known.
  - Make normalization conservative: empty/unknown input maps to `unknown` or `unverified`; never infer success.

- [ ] **Step 4: Run the focused tests and verify pass**
  - Run: `go test ./monitor -run 'Test(Normalize|TaskResult|Correlate|Unknown)' -v`
  - Expected: PASS.

- [ ] **Step 5: Commit**
  - `git add monitor/agent_model.go monitor/agent_model_test.go`
  - `git commit -m "feat: add ThreadDock agent observation model"`

## Task 2: Correlate existing Herdr/GitHub observations into the model

**Files:**
- Modify: `monitor/herdr.go`
- Modify: `monitor/github_runtime.go`
- Modify: `monitor/app.go`
- Modify: `monitor/app_test.go`
- Test: `monitor/herdr_test.go`
- Test: `monitor/github_test.go`

**Interfaces:**
- Consumes Task 1's model types.
- Produces the existing project/workflow response with an additive structured `agentWorkflow` field. Existing fields and existing Herdr/GitHub semantics remain backward compatible.

- [ ] **Step 1: Add failing adapter tests**
  - `TestHerdrObservationMapsTopLevelAgentStateWithoutClaimingWorkCompletion`
  - `TestGitHubRuntimeCorrelatesAgentWorkflowToIssueAndPR`
  - `TestAmbiguousBindingProducesNoticeInsteadOfInventingOwner`
  - `TestMissingAgentEvidenceProducesUnknownWorkflow`

  Assertions must prove:
  1. Herdr `done` can yield `execution=completed` while GitHub remains `work=active`.
  2. A matching Issue/PR binding attaches the Agent observation deterministically.
  3. Multiple conflicting matches create a notice/ambiguity state rather than choosing one silently.
  4. No evidence yields `verification=unverified`.

- [ ] **Step 2: Run adapter tests to verify failure**
  - Run: `go test ./monitor -run 'Test(HerdrObservation|GitHubRuntime|AmbiguousBinding|MissingAgentEvidence)' -v`
  - Expected: FAIL because `agentWorkflow` is not yet produced.

- [ ] **Step 3: Adapt `HerdrMonitor`**
  - Reuse existing `HerdrAgent`, `HerdrSession`, `HerdrConnection`, and observation timestamps.
  - Build `AgentObservation` from the existing top-level Agent/session records.
  - Do not add a second Herdr polling loop.
  - Do not write back to Herdr or `sessions.json`.

- [ ] **Step 4: Adapt `github_runtime.go`**
  - Correlate only using existing Issue/PR/repository/worktree/session binding data.
  - Keep GitHub status as `workState` and Herdr status as `executionState`.
  - When a PR exists, use its candidate SHA/revision as the preferred verification anchor.
  - Preserve current behavior when no Agent data exists.

- [ ] **Step 5: Expose the additive response in `app.go`**
  - Extend the existing read-side response rather than adding a new HTTP server or endpoint.
  - Keep response versioning/schema conventions consistent with existing snapshots.
  - Return `agentWorkflow` even when empty/unknown so the frontend can distinguish “not observed” from an API failure.

- [ ] **Step 6: Run backend tests**
  - Run: `go test ./monitor -v`
  - Expected: PASS with existing tests unchanged except for additive response assertions.

- [ ] **Step 7: Commit**
  - `git add monitor/herdr.go monitor/github_runtime.go monitor/app.go monitor/app_test.go monitor/herdr_test.go monitor/github_test.go`
  - `git commit -m "feat: correlate agent observations with work"`

## Task 3: Add structured result/evidence ingestion without creating persistence

**Files:**
- Modify: `monitor/agent_model.go`
- Modify: `monitor/app.go`
- Modify: `monitor/app_test.go`
- Modify: existing evidence/receipt helper file identified by current `main` implementation if its responsibility already owns native receipt parsing
- Test: `monitor/agent_model_test.go`
- Test: `monitor/app_test.go`

**Interfaces:**
- Consumes supplied native event/receipt artifacts through the existing read-only evidence path.
- Produces `TaskResult` and `EvidenceItem` attached to the matching Task/Agent observation.
- Does not store a new durable task/result database.

- [ ] **Step 1: Add failing evidence tests**
  - `TestReceiptProducesExecutionEvidenceButNotAutomaticVerification`
  - `TestReviewEvidenceWithMatchingRevisionCanBecomeVerified`
  - `TestReviewEvidenceForStaleRevisionRemainsUnverified`
  - `TestIncompleteChildResultRemainsPartial`

- [ ] **Step 2: Run focused tests and verify failure**
  - Run: `go test ./monitor -run 'Test(Receipt|ReviewEvidence|StaleRevision|IncompleteChild)' -v`
  - Expected: FAIL until evidence mapping is implemented.

- [ ] **Step 3: Implement evidence mapping**
  - Treat native child receipt metadata as execution evidence only.
  - Treat substantive review result + matching candidate revision as the minimum for `verification=verified` for a review task.
  - Preserve explicit incomplete/timeout/error results.
  - Keep source references so the UI can explain where the observation came from.
  - Do not attempt to reconstruct missing native transcript content or claim authenticated attestation.

- [ ] **Step 4: Add app-level fallback tests**
  - Verify malformed/absent evidence is surfaced as `unverified` with a notice, not as an API error when the rest of the project observation is healthy.

- [ ] **Step 5: Run backend tests**
  - Run: `go test ./monitor -v`
  - Expected: PASS.

- [ ] **Step 6: Commit**
  - `git add monitor/agent_model.go monitor/app.go monitor/app_test.go <existing evidence helper files>
  - `git commit -m "feat: model agent results and evidence"`

## Task 4: Present Agent workflow observation in Monitor UI

**Files:**
- Create: `monitor/frontend/src/AgentWorkflow.tsx`
- Test: `monitor/frontend/src/AgentWorkflow.test.tsx`
- Modify: `monitor/frontend/src/ProjectDetail.tsx`
- Test: `monitor/frontend/src/ProjectDetail.test.tsx`

**Interfaces:**
- Consumes the additive `agentWorkflow` API shape from Task 2/3.
- Produces a compact read-only workflow section; no new controls that create, stop, retry, approve, or reassign Agent work.

- [ ] **Step 1: Write failing component tests**
  - `renders_execution_work_state_separately`
  - `renders_task_lineage`
  - `renders_unverified_when_evidence_is_missing`
  - `renders_stale_revision_notice`
  - `renders_empty_state_without_fabricating_agent_activity`

  Assert that the UI visibly distinguishes:
  - Agent role/name and parent relationship
  - execution state
  - work state
  - verification state
  - candidate revision/SHA when present
  - evidence source when present
  - blocked/stale/unverified notices

- [ ] **Step 2: Run focused frontend tests and verify failure**
  - Run: `cd monitor/frontend && npm test -- AgentWorkflow.test.tsx ProjectDetail.test.tsx --run`
  - Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement `AgentWorkflow.tsx`**
  - Keep the default view compact, similar to `HerdrSummary`.
  - Show lineage as a small tree/ordered list rather than a graph in Phase 1.
  - Make verification wording explicit: `확인됨`, `부분 확인`, `미확인` rather than interpreting execution status as success.
  - Link to existing GitHub/Herdr destinations using existing safe-link helpers; do not introduce arbitrary command execution.
  - Keep accessibility and keyboard behavior consistent with existing Monitor components.

- [ ] **Step 4: Integrate into `ProjectDetail.tsx`**
  - Place the Agent workflow section next to existing Herdr/work information.
  - Do not replace the existing Herdr summary; the new section explains task/result/evidence relationships that Herdr alone cannot express.

- [ ] **Step 5: Run frontend tests and build**
  - Run: `cd monitor/frontend && npm test -- --run`
  - Run: `npm run build`
  - Expected: all tests PASS and production build succeeds.

- [ ] **Step 6: Commit**
  - `git add monitor/frontend/src/AgentWorkflow.tsx monitor/frontend/src/AgentWorkflow.test.tsx monitor/frontend/src/ProjectDetail.tsx monitor/frontend/src/ProjectDetail.test.tsx`
  - `git commit -m "feat: show structured agent workflow in monitor"`

## Task 5: Full verification and dogfooding gate

**Files:**
- Modify only if verification exposes a concrete defect; otherwise no source changes.
- Test: repository-wide existing test suites.

**Interfaces:**
- Consumes all Phase 1 outputs.
- Produces a verified candidate SHA and an explicit list of any environment-limited checks.

- [ ] **Step 1: Run project gate**
  - Run: `make check`
  - Expected: PASS.

- [ ] **Step 2: Build Windows Monitor**
  - Run from `monitor`: `wails build`
  - Expected: successful Windows package build.

- [ ] **Step 3: Run focused runtime fixture**
  - Use deterministic fixtures to demonstrate at least three states:
    1. running Agent + active GitHub work
    2. completed Herdr execution + active GitHub work + unverified result
    3. completed review + matching candidate SHA + verified result
  - Expected: UI shows all three without conflating them.

- [ ] **Step 4: Run GHES + WSL + Herdr smoke where the environment is available**
  - Verify real Issue/PR + Herdr connection observation.
  - Record unavailable environment checks explicitly instead of treating local fixture success as equivalent.

- [ ] **Step 5: Run a fresh-review pass against the design spec**
  - Check for accidental responsibility creep: no scheduler, persistence, execution, or mutation control introduced.
  - Check every `verified` state has a concrete revision/evidence path.

- [ ] **Step 6: Commit only verification/documentation corrections**
  - If fixes are needed, commit them separately from feature implementation with the relevant test evidence.

---

## Phase 1 Exit Criteria

Phase 1 is complete only when all of the following are true:

1. Monitor can show Agent identity and parent/child lineage using existing Herdr/native observations.
2. Task/Result structure is explicit in the read-side API without becoming a durable task store.
3. Execution, work, and verification states are independently visible.
4. Reviewer invocation without substantive matching evidence remains `unverified`.
5. Matching candidate revision + substantive review evidence can produce `verified`.
6. Missing/old evidence cannot silently verify a newer candidate.
7. Existing Monitor behavior remains valid when Agent observations are unavailable.
8. No new scheduler, runtime, persistence database, or MCP orchestration layer exists.
9. `make check`, frontend build, and available Wails/GHES/WSL smoke checks pass for the candidate SHA.

## Phase 1 Follow-up, explicitly deferred

After Phase 1, reassess whether actual Agent-to-Agent communication needs a new durable transport. The default should remain structured parent/child task handoff through native subagent results and GitHub records. Do not build a chat/room system merely because oh-my-pi has one.

Potential Phase 2 candidates, only if real usage demonstrates the need:
- richer child task timeline/event stream
- explicit handoff/result templates in the UI
- selective intervention gates
- deeper native receipt adapters
- task graph visualization if the simple lineage view becomes insufficient
