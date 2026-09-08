# Work Item Coordinator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 한 Work Item owner가 runtime·Git·publication 외부 동작을 중복 없이 dispatch하고, pause와 재시작 뒤 evidence 기반으로 reconcile하게 한다.

**Architecture:** 새 `internal/coordinator` module이 process-lifetime owner lease와 Work별 single command queue를 소유한다. 짧은 `statev2.Store.Apply` lease와 외부 I/O를 분리하고, 모든 외부 동작 직전에 snapshot을 다시 읽어 현재 typed intent/invocation만 실행한다. runtime과 Publisher는 좁은 port 뒤의 adapter이며 state를 직접 쓰지 않는다.

**Tech Stack:** Go 1.27, `syscall.Flock`, goroutine/channel, `context`, `internal/state/v2`.

**Spec:** `docs/superpowers/specs/2026-09-08-workflow-shared-state-design.md`

## Global Constraints

- coordinator owner 하나가 runtime invoke/terminate, Git 변경과 publication/recovery를 실행한다.
- owner lock은 process lifetime 동안 유지하고 TTL로 steal하지 않는다.
- state Apply lease는 local CAS/atomic replacement 동안만 유지하며 외부 I/O 중에는 잡지 않는다.
- Work별 queue는 하나이고 publication 외부 I/O는 동시에 하나만 active다.
- runtime 호출은 durable begin_launch 이후 한 번만 시작하며 long call은 queue를 막지 않는다.
- pending intent, replay receipt와 오래된 queue item은 직접 launch 권한이 아니다.
- 외부 I/O 직전에 owner, WorkControl, current invocation/intent generation/status를 다시 확인한다.
- unknown runtime/remote 결과는 blind retry하지 않고 typed needs_operator/conflict 근거를 저장한다.
- 구현 worker는 focused test/vet만 실행하고 최종 code PR에서 `make check`를 한 번 실행한다.
- GitHub PR·Issue 제목과 본문은 한국어로 작성한다.

---

## File map

| Task | Files | Result |
|---|---|---|
| 1 | owner lock and durable owner record | one process owner without TTL stealing |
| 2 | single queue and publication dispatch | pre-I/O recheck and one publication write |
| 3 | asynchronous runtime dispatch | launch once, responsive terminate/result handling |
| 4 | restart reconcile and pause integration | evidence-based recovery and end-to-end fake flow |

## Task 1: Process owner lease

**Files:**

- Create: `internal/coordinator/owner.go`
- Create: `internal/coordinator/owner_test.go`
- Create: `internal/coordinator/types.go`

**Interfaces:**

```go
type OwnerID string
type OwnerRecord struct {
    WorkID contractv2.WorkID `json:"workId"`
    OwnerID OwnerID `json:"ownerId"`
    PID int `json:"pid"`
    StartedAt time.Time `json:"startedAt"`
}
type OwnerLease interface {
    Record() OwnerRecord
    Release() error
}
type OwnerLocker interface {
    Acquire(context.Context, contractv2.WorkID, OwnerID, int, time.Time) (OwnerLease, error)
}
func NewOwnerLocker(root string) OwnerLocker
```

- [ ] Write a failing helper-process test proving only one process acquires `<root>/v2/work/<workId>/coordinator.lock`, release/crash unlocks it, stale timestamps do not steal a live lock, and `owner.json` is 0600 under 0700 directories.
- [ ] Run RED with `docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -count=1 ./internal/coordinator`.
- [ ] Implement nonblocking flock, strict IDs/UTC time, atomic owner record and idempotent release. The lock file descriptor remains open for the lease lifetime.
- [ ] Run GREEN and `go vet ./internal/coordinator` in Docker Go 1.27.
- [ ] Commit with `git commit -m "feat: Work Item coordinator owner lease 추가"`.

## Task 2: Single queue and publication dispatch

**Files:**

- Create: `internal/coordinator/dispatcher.go`
- Create: `internal/coordinator/dispatcher_test.go`
- Create: `internal/coordinator/publication.go`
- Create: `internal/coordinator/publication_test.go`

**Interfaces:**

```go
type State interface {
    Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error)
    Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error)
}
type PublicationObservation struct {
    State string
    Receipt *statev2.PublicationReceipt
    Diagnostic string
}
type Publisher interface {
    Observe(context.Context, statev2.PublicationState) (PublicationObservation, error)
    Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error)
}
type CommandResult struct { Snapshot statev2.WorkSnapshot; Err error }
type Dispatcher interface {
    SubmitPublication(context.Context, contractv2.WorkID, statev2.PublicationIntentID) <-chan CommandResult
    Close(context.Context) error
}
```

- [ ] Write failing concurrency tests where 20 goroutines submit one intent and the fake Publisher observes/publishes once. Assert state Apply occurs before publish and no state lock is held while the fake blocks.
- [ ] Write failing stale-queue tests: pause, blocker, superseding generation or completed receipt introduced after enqueue prevents Publish; existing remote match is adopted; unknown observation records conflict and never calls Publish.
- [ ] Implement one goroutine queue per Work, one active publication, owner lease check and immediate pre-I/O `Load`. Publisher receives only the current immutable pending intent.
- [ ] Record complete/fail/conflict through `Store.Apply` with new request IDs; do not write state in the Publisher adapter.
- [ ] Run focused coordinator/state tests and vet; commit `feat: publication single dispatcher 추가`.

## Task 3: Asynchronous runtime dispatch

**Files:**

- Create: `internal/coordinator/runtime.go`
- Create: `internal/coordinator/runtime_test.go`
- Modify: `internal/coordinator/dispatcher.go`
- Modify: `internal/coordinator/dispatcher_test.go`

**Interfaces:**

```go
type RuntimeObservation struct {
    State string
    ProviderIdentity string
    EndedAt *time.Time
    Diagnostic string
}
type Runtime interface {
    Observe(context.Context, statev2.InvocationState) (RuntimeObservation, error)
    Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity) (string, error)
    Terminate(context.Context, statev2.InvocationState) error
}
type RuntimeResult struct {
    InvocationID statev2.InvocationID
    Artifact json.RawMessage
    Err error
}
```

- [ ] Write failing tests proving reserve alone does not launch; dispatcher begin_launch is persisted before exactly one Launch; begin_launch replay/restarted dispatcher observes instead of relaunching.
- [ ] Block fake Launch/Observe and prove the queue still processes pause and termination commands. Runtime result returns through an internal event to the queue; worker goroutine never calls Store.Apply directly.
- [ ] Test active→mark_running, timeout/cancel→request_termination→Terminate→confirm_termination, positive no-launch reconcile, and unknown identity→needs_operator without duplicate launch.
- [ ] Implement bounded asynchronous observation with owner/pre-I/O recheck and exact invocation/profile/fingerprint/worktree identity.
- [ ] Run focused tests/vet and commit `feat: 비동기 runtime dispatcher 추가`.

## Task 4: Restart reconcile and integration flow

**Files:**

- Create: `internal/coordinator/reconcile.go`
- Create: `internal/coordinator/reconcile_test.go`
- Create: `internal/coordinator/integration_test.go`
- Modify: `internal/workflow/service.go`
- Modify: `internal/workflow/service_test.go`

**Interfaces:**

```go
type ReconcileResult struct {
    WorkID contractv2.WorkID
    State statev2.WorkState
    NextAction string
    EvidenceRefs []string
}
func (c *Coordinator) Reconcile(context.Context, contractv2.WorkID) (ReconcileResult, error)
```

- [ ] Write failing restart tests for reserved/no-launch proof, launched-active observation, unknown runtime, pending publication remote match, proven not-published retry and ambiguous result conflict.
- [ ] Test pause across reserved/running/publication-pending states: no new launch/write, running invocation terminates, already-started publication settles, resume preserves budgets and evidence.
- [ ] Test owner process loss followed by new owner acquisition: acquiring the lock never implies child termination; reconcile happens before queue activation.
- [ ] Build one fake-backed flow approve→reserve→launch→terminate→candidate-ready handoff plus publication begin→remote receipt, asserting each external call once and status reads remain non-mutating.
- [ ] Run `go test -count=1 ./internal/coordinator ./internal/state/v2 ./internal/workflow` and corresponding vet in Docker Go 1.27; commit `feat: coordinator 재시작 reconcile 완성`.

## Final verification

After every Task has fresh Sol review and fixes:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 make check
```

Record exact owner/queue/runtime/publication test evidence and update `HANDOFF.md`. Actual runtime/GitHub calls remain outside this fake-backed coordinator plan.
