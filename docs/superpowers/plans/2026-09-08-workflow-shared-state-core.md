# Workflow Shared State Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Work Item의 승인·Task 실행 근거·publication intent를 하나의 revisioned `WorkSnapshot`과 typed `Apply` interface로 안전하게 관리한다.

**Architecture:** `internal/state/v2`가 authoritative state, transition validation, budget과 aggregate reduction을 소유한다. 기존 arbitrary `Mutate`는 typed `Apply`로 교체하고 `internal/workflow`는 command 작성과 Monitor projection만 담당한다. Work Item coordinator owner·dispatcher는 이 core가 fresh review를 통과한 다음 별도 계획으로 구현한다.

**Tech Stack:** Go 1.27, filesystem state, `syscall.Flock`, JSON, SHA-256.

**Spec:** `docs/superpowers/specs/2026-09-08-workflow-shared-state-design.md`

## Global Constraints

- 기존 `WorkState` 값을 유지하고 두 번째 Work 상태 enum을 만들지 않는다.
- `WorkSnapshot` 하나가 승인, TaskStates, Publications, revision과 replay receipt의 authoritative local record다.
- Work/Task/Publication 중 정확히 하나의 typed transition만 한 `Apply`에서 처리한다.
- validation failure는 write하지 않고 budget exhaustion과 확인된 ambiguity만 durable `needs_operator`로 기록한다.
- repair 기본 한도는 Task별 누적 2회, recovery 기본 한도는 1회다. pause/resume·새 requestId·process restart로 초기화하지 않는다.
- runtime 호출은 durable `begin_launch` 뒤에만 가능하며 positive no-launch proof 없이 reservation을 해제하지 않는다.
- Publication intent의 key·generation·payload·target과 completed receipt는 immutable하다.
- replay result는 receipt를 재귀 포함하지 않는 bounded projection이며 같은 requestId의 다른 payload는 거부한다.
- provider session/pane/process identity는 Monitor·CLI projection과 GitHub 요약에 노출하지 않는다.
- v1 snapshot을 읽거나 변환하지 않고 자동 migration을 만들지 않는다.
- worker는 focused test/vet만 실행하고 전체 `make check`는 통합 code PR의 마지막 gate에서 한 번 실행한다.
- 구현·수정은 Luna high, fresh task review는 Sol medium에 배정한다. 실제 resolved model/effort가 보이지 않으면 `unverified`로 기록한다.

---

## File map and dependency order

| Task | Files owned | Produces |
|---|---|---|
| 1 | state model, initialization, validation | typed snapshot and transition vocabulary |
| 2 | Apply engine, Work transitions, workflow approval | CAS/replay and approve/pause/resume seam |
| 3 | Task invocation lifecycle | reservation, launch, termination and reconcile |
| 4 | Task evidence and budgets | candidate→gate→review→integration and repair/recovery |
| 5 | Publication generations and transitions | immutable intent lifecycle and publication-only recovery |
| 6 | Reducer and aggregate projection | deterministic Work/Monitor status and compatibility gate |

Tasks 1–6 all modify shared state interfaces and therefore run serially. Task 4 Publisher and Task 5 executor from the parent plan do not start until Task 6 review passes.

## Task 1: Typed state model and plan initialization

**Files:**

- Create: `internal/state/v2/model.go`
- Create: `internal/state/v2/transition_types.go`
- Create: `internal/state/v2/model_test.go`
- Modify: `internal/state/v2/types.go`
- Modify: `internal/state/v2/store.go`
- Modify: `internal/state/v2/store_test.go`
- Modify: `internal/workflow/service.go`
- Modify: `internal/workflow/service_test.go`

**Interfaces:**

- Consumes: `contractv2.WorkItemContract.Tasks`, existing `WorkState`, revision and receipt fields.
- Produces these exact public declarations:

```go
type TaskStatus string
const (
    TaskPending TaskStatus = "pending"
    TaskInvocationReserved TaskStatus = "invocation_reserved"
    TaskRunning TaskStatus = "running"
    TaskTerminationPending TaskStatus = "termination_pending"
    TaskTerminated TaskStatus = "terminated"
    TaskCandidateReady TaskStatus = "candidate_ready"
    TaskGateFailed TaskStatus = "gate_failed"
    TaskGatePassed TaskStatus = "gate_passed"
    TaskReviewBlocked TaskStatus = "review_blocked"
    TaskAccepted TaskStatus = "accepted"
    TaskIntegrated TaskStatus = "integrated"
    TaskNeedsOperator TaskStatus = "needs_operator"
)

type WorkControl struct {
    ApprovedContractHash string            `json:"approvedContractHash,omitempty"`
    ApprovalRef          string            `json:"approvalRef,omitempty"`
    PauseRequested       bool              `json:"pauseRequested"`
    Blocker              *OperatorBlocker  `json:"blocker,omitempty"`
}

type TaskExecutionState struct {
    TaskID          contractv2.TaskID `json:"taskId"`
    Status          TaskStatus        `json:"status"`
    BuilderAttempt  uint32            `json:"builderAttempt"`
    RepairCount     uint32            `json:"repairCount"`
    RepairLimit     uint32            `json:"repairLimit"`
    RecoveryCount   uint32            `json:"recoveryCount"`
    RecoveryLimit   uint32            `json:"recoveryLimit"`
    LogicalWork     *LogicalWorkState `json:"logicalWork,omitempty"`
    Worktree        *WorktreeIdentity `json:"worktree,omitempty"`
    Invocation      *InvocationState  `json:"invocation,omitempty"`
    Candidate       *CandidateEvidence `json:"candidate,omitempty"`
    Gate            *GateEvidence      `json:"gate,omitempty"`
    Review          *ReviewEvidence    `json:"review,omitempty"`
    Integration     *IntegrationEvidence `json:"integration,omitempty"`
    PriorAttempts   []AttemptSummary   `json:"priorAttempts"`
}

type PublicationIntentID string
type PublicationKey string
type PublicationState struct {
    IntentID PublicationIntentID `json:"intentId"`
    Key PublicationKey `json:"key"`
    Generation uint32 `json:"generation"`
    Kind PublicationKind `json:"kind"`
    Status PublicationStatus `json:"status"`
    PayloadHash string `json:"payloadHash"`
    PayloadRef string `json:"payloadRef"`
    Target PublicationTarget `json:"target"`
    Receipt *PublicationReceipt `json:"receipt,omitempty"`
    Attempts uint32 `json:"attempts"`
    LastError string `json:"lastError,omitempty"`
}
```

`WorkSnapshot` gains `Control WorkControl`, `TaskStates map[TaskID]TaskExecutionState`, and `Publications map[PublicationIntentID]PublicationState`. `transition_types.go` defines the remaining evidence, blocker, action and transition payload structs named by the spec with lower-camel JSON tags. Constants are `DefaultRepairLimit=2`, `DefaultRecoveryLimit=1`, `MaxPriorAttempts=5`, `MaxDiagnosticBytes=1024`.

- [ ] **Step 1: Write failing initialization and strict-validation tests**

Add tests that break when plan creation omits a Contract Task, uses a mismatched map key/internal TaskID, accepts a foreign Task, nil Publications, zero limits, unknown enum, more than five prior summaries, or a diagnostic over 1024 bytes. Add a compatibility fixture where both new maps are absent and decode yields non-nil empty maps only for an unstarted foundation snapshot.

- [ ] **Step 2: Verify RED**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -count=1 ./internal/state/v2 ./internal/workflow
```

Expected: compile failure for missing model types and failing plan initialization assertions.

- [ ] **Step 3: Implement model types and initialization**

`workflow.PlanWork` constructs one `TaskPending` entry per Contract Task with limits 2/1, empty non-nil `PriorAttempts`, empty Publications and unapproved WorkControl. `state/v2` validates map identity and canonical contract membership. Compatibility normalization happens immediately after strict JSON decoding and before validation; it applies only when both fields are absent and no Task has begun.

- [ ] **Step 4: Verify GREEN and vet**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 gofmt -w internal/state/v2 internal/workflow
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -count=1 ./internal/state/v2 ./internal/workflow
docker run --rm -v "$PWD":/src -w /src golang:1.27 go vet ./internal/state/v2 ./internal/workflow
```

- [ ] **Step 5: Commit**

```bash
git add internal/state/v2 internal/workflow
git commit -m "feat: 공유 워크플로 상태 모델 추가"
```

## Task 2: Apply engine and Work control transitions

**Files:**

- Create: `internal/state/v2/apply.go`
- Create: `internal/state/v2/apply_test.go`
- Create: `internal/state/v2/work_transition.go`
- Create: `internal/state/v2/work_transition_test.go`
- Create: `internal/state/v2/reducer.go`
- Modify: `internal/state/v2/transition_types.go`
- Modify: `internal/state/v2/types.go`
- Modify: `internal/state/v2/store.go`
- Modify: `internal/state/v2/store_test.go`
- Modify: `internal/workflow/service.go`
- Modify: `internal/workflow/service_test.go`

**Interfaces:**

- Consumes: Task 1 model and existing CAS/replay persistence helpers.
- Produces:

```go
type TransitionRequest struct {
    WorkID contractv2.WorkID
    ExpectedRevision contractv2.Revision
    RequestID contractv2.RequestID
    PayloadHash string
    Work *WorkTransition
    Task *TaskTransition
    Publication *PublicationTransition
}
type Store interface {
    CreatePlan(context.Context, WorkSnapshot, contractv2.RequestID, string) (WorkSnapshot, error)
    Load(context.Context, contractv2.WorkID) (WorkSnapshot, error)
    List(context.Context) ([]WorkSnapshot, error)
    Apply(context.Context, TransitionRequest) (WorkSnapshot, error)
}
```

`WorkAction` values are `approve`, `pause`, `resume`, `resolve`. Task 2 implements `extend_budget` with `BudgetKind` values `repair` and `recovery`; runtime-specific resolution becomes active in Task 3, evidence-derived `retry_verified_stage` in Task 4, and publication resolution in Task 5.

- [ ] **Step 1: Write failing Apply invariant tests**

Cover exactly-one transition, canonical transition hash mismatch, replay, changed-payload conflict, stale revision, invalid transition no-write, revision increment once, bounded receipt projection and immutable contract/hash. Reuse real `t.TempDir` stores.

- [ ] **Step 2: Write failing Work transition tests**

Cover approve only from awaiting_approval with matching contract hash and nonempty approvalRef; pre-approval Task/publication rejection; pause setting `PauseRequested`; resume only when no active/unknown invocation and blocker is absent; extend_budget only with operatorRef, TaskID, valid BudgetKind and a strictly higher ceiling.

- [ ] **Step 3: Verify RED**

Run the Task 1 focused test command. Expected: missing `Apply` and Work reducer failures.

- [ ] **Step 4: Implement Apply and replace arbitrary Mutate**

Move lease/load/replay/CAS/write orchestration from `Mutate` into `Apply`. Recompute `PayloadHash` from canonical JSON of the non-nil typed transition and compare it with the request. Remove `Mutation`, `Transition`, `Mutate` and every caller in the same commit. `workflow.ApproveWork` creates `WorkTransition{Action: WorkApprove, ApprovalRef: request string, ContractHash: snapshot.ContractHash}` after reading current status and calls Apply.

- [ ] **Step 5: Implement initial deterministic reducer**

`reduce(snapshot)` derives Work state and nextAction from Control and TaskStates: unapproved→awaiting_approval/approve; blocker→needs_operator/resolve; pause requested with no active invocation→paused/resume; approved all pending→queued/run. SyncStatus remains unchanged unless a Publication transition changes it.

- [ ] **Step 6: Verify GREEN, vet, and commit**

Use Task 1 focused commands, then:

```bash
git add internal/state/v2 internal/workflow
git commit -m "feat: typed Apply와 Work 제어 전이 추가"
```

## Task 3: Shared builder/reviewer invocation lifecycle

**Files:**

- Create: `internal/state/v2/task_transition.go`
- Create: `internal/state/v2/task_invocation_test.go`
- Modify: `internal/state/v2/transition_types.go`
- Modify: `internal/state/v2/apply.go`
- Modify: `internal/state/v2/work_transition.go`
- Modify: `internal/state/v2/work_transition_test.go`
- Modify: `internal/state/v2/reducer.go`

**Interfaces:**

- Consumes: Task 2 Apply.
- Produces Task actions `reserve_invocation`, `begin_launch`, `mark_running`, `request_termination`, `confirm_termination`, `reconcile_not_started`, `needs_operator`.

Every `TaskTransition` contains `TaskID`, `Action`, `InvocationID`, `LogicalWorkID`, `Role`, `BuilderAttempt`, `At` and the action-specific evidence. Role values are only builder/reviewer. ReturnStage is pending/gate_failed/review_blocked for builder and gate_passed for reviewer.

- [ ] **Step 1: Write failing lifecycle transition-table tests**

Cover valid builder and reviewer paths; role/returnStage mismatch; foreign Task; pre-approval/paused/blocked/completed reservation; mark_running before begin_launch; mismatched invocation/logical-work/attempt; duplicate reservation; termination confirmation; and candidate/next invocation rejection before termination.

- [ ] **Step 2: Write failing reconcile tests**

`reconcile_not_started` requires `LaunchRequested=false`, positive `OwnerTerminated=true` evidence, matching invocation and no provider identity. It preserves an abandoned summary and LogicalWork budget debit, restores ReturnStage, and a subsequent reservation reuses LogicalWorkID/BuilderAttempt without increasing counts. `LaunchRequested=true`, TTL age or missing provider ID is insufficient.

Add Work resolution tests for `runtime_not_started` and `runtime_terminated`. Both require an operatorRef, matching Task/Invocation and positive evidence. `runtime_not_started` performs the same restoration as reconcile_not_started in one Apply. `runtime_terminated` records termination confirmation and clears only the matching runtime blocker; unknown or mismatched runtime evidence keeps needs_operator unchanged.

- [ ] **Step 3: Verify RED, implement minimal lifecycle, verify GREEN**

Use focused state/workflow tests. Store provider identity only in InvocationState. Reducer maps reserved/running/termination states to Work running, and pause request blocks new dispatch while allowing termination/confirmation.

- [ ] **Step 4: Vet and commit**

```bash
git add internal/state/v2
git commit -m "feat: builder reviewer 호출 lifecycle 추가"
```

## Task 4: Candidate evidence, reviewer result, repair and recovery budgets

**Files:**

- Create: `internal/state/v2/task_evidence_test.go`
- Create: `internal/state/v2/task_budget_test.go`
- Modify: `internal/state/v2/task_transition.go`
- Modify: `internal/state/v2/reducer.go`
- Modify: `internal/state/v2/transition_types.go`
- Modify: `internal/state/v2/work_transition.go`
- Modify: `internal/state/v2/work_transition_test.go`

**Interfaces:**

- Consumes: Task 3 terminated invocation.
- Produces actions `record_candidate`, `record_gate`, `record_review`, `record_integration`, repair/recovery forms of `reserve_invocation`.
- Produces evidence-derived Work resolve kind `retry_verified_stage` without accepting a caller-selected target state.

- [ ] **Step 1: Write failing evidence-chain tests**

Use literal 40-character SHA fixtures. Prove candidate requires normal terminated builder; gate uses current candidate/attempt; reviewer reservation preserves candidate/gate; review requires normal terminated reviewer and matching reviewer invocation/attempt/SHA; integration requires accepted review and records a distinct IntegrationHEAD with verified candidate relation.

- [ ] **Step 2: Write failing repair tests**

From gate_failed and review_blocked, repair reservation atomically increments RepairCount and BuilderAttempt, invalidates active candidate/gate/review/integration, saves a bounded prior summary, and reserves the builder. Replay changes nothing. Third repair request records needs_operator without incrementing beyond 2 or reserving an invocation.

- [ ] **Step 3: Write failing recovery tests**

Only a terminated invocation explicitly marked transient can reserve recovery. It preserves role, BuilderAttempt and candidate/gate, increments RecoveryCount once and uses a new InvocationID. A second recovery records needs_operator. Pause/resume and reviewer invocation never reset or consume the wrong budget.

Add `retry_verified_stage` tests that require operatorRef, matching blocker/Task and evidence, then derive the restored Task stage from current candidate/gate/review evidence. Caller input never names the restored TaskStatus; mismatched evidence is a no-write error.

- [ ] **Step 4: Verify RED, implement transitions and reducer, verify GREEN**

Reducer maps gate/review/integration states to running/review/ready-for-PR as supported by current evidence and keeps needs_operator durable.

- [ ] **Step 5: Vet and commit**

```bash
git add internal/state/v2
git commit -m "feat: Task 검증 리뷰 budget 전이 추가"
```

## Task 5: Publication intent generations and recovery

**Files:**

- Create: `internal/state/v2/publication_transition.go`
- Create: `internal/state/v2/publication_transition_test.go`
- Modify: `internal/state/v2/transition_types.go`
- Modify: `internal/state/v2/apply.go`
- Modify: `internal/state/v2/work_transition.go`
- Modify: `internal/state/v2/reducer.go`

**Interfaces:**

- Consumes: Task 2 Apply and approved WorkControl.
- Produces Publication actions `begin`, `complete`, `fail`, `conflict`, `supersede` and Work resolve kind `publication_reconciled`.

- [ ] **Step 1: Write failing identity/generation tests**

Cover unique IntentID and key-generation, generation starting at 1, immutable kind/payloadRef/payloadHash/target, maximum generation derived from map, next generation only after completed or proven-no-write supersede, and stale generation refusing remote-write eligibility.

- [ ] **Step 2: Write failing publication lifecycle tests**

Begin requires approved/non-paused/non-blocked Work and records pending before any external adapter exists. Complete requires matching pending intent and immutable receipt identity. Fail increments attempts and bounds LastError to 1024 bytes. Conflict blocks retry. Completed receipt is immutable. Retry reuses the same intent with a new transition RequestID and preserves TaskStates byte-for-byte.

- [ ] **Step 3: Write failing reconciliation tests**

`publication_reconciled` adopts an exact remote match as completed, restores proven-not-published to failed, and preserves conflict for ambiguous evidence. A late request for an older generation cannot change the current generation. Running sync failure changes SyncStatus only; required post-merge failure derives publication_pending without changing Task evidence.

- [ ] **Step 4: Verify RED, implement transitions/reducer, verify GREEN**

All credential/provider raw output fields are rejected or omitted. Payload identity is a bounded immutable reference plus SHA-256; this core does not call GitHub/Wiki.

- [ ] **Step 5: Vet and commit**

```bash
git add internal/state/v2
git commit -m "feat: publication intent 세대와 복구 전이 추가"
```

## Task 6: Aggregate projection and shared-state integration gate

**Files:**

- Create: `internal/state/v2/reducer_test.go`
- Modify: `internal/workflow/service.go`
- Modify: `internal/workflow/service_test.go`
- Modify: `internal/monitor/types.go`
- Modify: `internal/monitor/types_test.go`
- Modify: `internal/cli/project_work_test.go`

**Interfaces:**

- Consumes: Tasks 1–5 state and reducer.
- Produces current Monitor/CLI projection without provider identity and a reviewed base for coordinator planning.

- [ ] **Step 1: Write failing reducer priority tests**

Use multiple Work Items to prove priority `needs_operator` > `paused` > `publication_pending` > `review` > `running` > `ready_for_pr` > `queued` > `completed` > `draft`, then recent evidence time and WorkID for deterministic ties. Empty projects expose draft/current/no-action with non-nil WorkItems.

- [ ] **Step 2: Write failing projection privacy tests**

Status includes Task status, verification/review/merge summaries, Work blocker and publication sync/nextAction. JSON must not contain provider invocation ID, session, pane, process, raw diagnostic or canonical local path.

- [ ] **Step 3: Write failing compatibility tests**

Existing foundation plan/approve/status command flow remains valid with initialized maps. Missing-map compatibility works only for unstarted foundation snapshots. v1 tests compile unchanged. Status polling calls only list/projection paths and never Apply.

- [ ] **Step 4: Implement projection and verify focused packages**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 gofmt -w internal/state/v2 internal/workflow internal/monitor internal/cli
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -count=1 ./internal/state/v2 ./internal/workflow ./internal/monitor ./internal/cli
docker run --rm -v "$PWD":/src -w /src golang:1.27 go vet ./internal/state/v2 ./internal/workflow ./internal/monitor ./internal/cli
```

- [ ] **Step 5: Commit**

```bash
git add internal/state/v2 internal/workflow internal/monitor internal/cli
git commit -m "feat: 공유 상태 aggregate projection 완성"
```

## Post-plan handoff

After Task 6 fresh review, write `docs/superpowers/plans/2026-09-08-work-item-coordinator.md` against the exact reviewed `Store.Apply` and transition types. That plan owns the OS owner lease, in-process single queue, pre-I/O recheck, asynchronous runtime observation, publication dispatch and reconcile tests. Only after that coordinator plan is implemented and reviewed may the parent-plan Publisher and executor adapters run in parallel.
