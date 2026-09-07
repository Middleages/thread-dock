# Workflow Shared State Design

## 상태와 목적

이 문서는 [Project Workflow MVP 설계](2026-09-07-project-workflow-mvp-design.md)의
단일 저장소 흐름에서 Publisher와 runtime/Git 실행이 함께 사용할 local state seam을 고정한다.
[단일 저장소 구현 계획](../plans/2026-09-07-single-repository-workflow.md)의 Task 4·5를
병렬화하기 전에 수행하는 직렬 설계다.

PR #49에서 Contract v2, registry, revision CAS, requestId 멱등성, aggregate status와
project/work CLI foundation을 구현했다. 아직 `WorkSnapshot`에는 Task invocation·검증·리뷰와
외부 publication의 pending intent·receipt가 없다. 각 후속 module이 자체 상태를 만들면
중복 호출 차단, repair budget과 publication-only 복구를 하나의 invariant로 보장할 수 없다.

## 선택

`internal/state/v2.WorkSnapshot` 안에 typed `TaskStates`와 `Publications` map을 둔다.
`state/v2` module은 하나의 검증된 transition interface 뒤에서 revision CAS, request replay,
상태 전이와 budget을 함께 적용한다. Work Item의 단일 writer와 원자적 `work.json` 교체를
그대로 활용하며 Task별 파일이나 event-log 원본은 이번 단계에 도입하지 않는다.

검토한 대안:

1. Task·publication별 파일은 독립 쓰기에 유리하지만 Work Item revision과 전체 nextAction을
   원자적으로 맞추기 어렵다.
2. event log를 원본으로 쓰면 이력은 유연하지만 replay projection, compaction과 손상 복구가
   첫 단일 저장소 흐름보다 큰 subsystem이 된다.

## 상태 자료형

`WorkSnapshot`에 다음 필드를 추가한다.

```go
TaskStates   map[contractv2.TaskID]TaskExecutionState `json:"taskStates"`
Publications map[PublicationKey]PublicationState      `json:"publications"`
```

Contract에 선언된 모든 Task는 plan 생성 때 `pending` TaskState를 가진다. map key와 내부
`TaskID`는 같아야 하며 Contract에 없는 key는 거부한다. Publication은 실제 발행 intent가
생길 때 logical key로 추가한다. key는 `parent-issue`, `project-item`, `pr:<repoKey>`,
`wiki:<page>`처럼 Contract·Final Manifest의 논리 대상을 가리키며 원격 번호를 대신하지 않는다.

### TaskExecutionState

```go
type TaskExecutionState struct {
    TaskID        contractv2.TaskID `json:"taskId"`
    Status        TaskStatus        `json:"status"`
    Attempt       uint32            `json:"attempt"`
    RepairCount   uint32            `json:"repairCount"`
    RecoveryCount uint32            `json:"recoveryCount"`
    Worktree      WorktreeIdentity  `json:"worktree"`
    Invocation    *InvocationState  `json:"invocation,omitempty"`
    Candidate     *CandidateEvidence `json:"candidate,omitempty"`
    Gate          *GateEvidence     `json:"gate,omitempty"`
    Review        *ReviewEvidence   `json:"review,omitempty"`
    Integration   *IntegrationEvidence `json:"integration,omitempty"`
}
```

TaskStatus는 다음 값만 허용한다.

`pending`, `invocation_reserved`, `running`, `termination_pending`, `terminated`,
`candidate_ready`, `gate_failed`, `gate_passed`, `review_blocked`, `accepted`, `integrated`,
`needs_operator`.

WorktreeIdentity는 canonical path, Git common-dir, branch, base SHA를 가진다. InvocationState는
requestId, role, logical profile ID, runtime fingerprint, provider invocation ID, 시작·종료 시각과
termination confirmation을 가진다. provider session/pane/process identity는 local state에만 있고
Monitor projection과 GitHub 요약에는 포함하지 않는다.

CandidateEvidence는 candidate SHA, tree SHA와 changed files를 가진다. GateEvidence는 exact
candidate SHA, 명령, outcome과 확인 시각을 가진다. ReviewEvidence는 reviewer requestId,
검토 SHA, accept/block과 bounded findings를 가진다. IntegrationEvidence는 accepted candidate와
integration HEAD를 가진다. Agent의 자체 검증 문구는 이 authoritative evidence에 넣지 않는다.

### PublicationState

```go
type PublicationState struct {
    Key         PublicationKey    `json:"key"`
    Kind        PublicationKind   `json:"kind"`
    Status      PublicationStatus `json:"status"`
    RequestID   contractv2.RequestID `json:"requestId"`
    PayloadHash string            `json:"payloadHash"`
    Target      PublicationTarget `json:"target"`
    Receipt     *PublicationReceipt `json:"receipt,omitempty"`
    Attempts    uint32            `json:"attempts"`
    LastError   string            `json:"lastError,omitempty"`
}
```

PublicationKind는 `parent_issue`, `child_issue`, `project_item`, `handoff`, `pull_request`,
`wiki`를 허용한다. PublicationStatus는 `pending`, `completed`, `failed`, `conflict`다.
Target은 host/repository, logical key와 필요한 base identity만 가지며 credential은 저장하지 않는다.
Receipt는 원격 node/number/URL, base/head identity와 publishedAt을 bounded field로 저장한다.
LastError는 credential·provider raw output을 제외한 1 KiB 이하 진단이다.

## Transition interface

호출자가 arbitrary closure로 snapshot을 바꾸지 않도록 public store seam을 다음과 같이 고정한다.

```go
type TransitionRequest struct {
    WorkID           contractv2.WorkID
    ExpectedRevision contractv2.Revision
    RequestID        contractv2.RequestID
    PayloadHash      string
    Task             *TaskTransition
    Publication      *PublicationTransition
}

type Store interface {
    CreatePlan(context.Context, WorkSnapshot, contractv2.RequestID, string) (WorkSnapshot, error)
    Load(context.Context, contractv2.WorkID) (WorkSnapshot, error)
    List(context.Context) ([]WorkSnapshot, error)
    Apply(context.Context, TransitionRequest) (WorkSnapshot, error)
}
```

Task와 Publication 중 정확히 하나만 있어야 한다. `Apply`는 기존 lease를 획득하고 immutable
contract/hash를 확인한 뒤 requestId/payload replay, revision CAS, typed transition validation,
revision 증가와 atomic write를 한 번에 수행한다. 최초 호출과 replay는 receipt 안의 bounded
client-result projection을 동일하게 반환한다.

기존 arbitrary `Mutate`는 새 interface와 모든 caller가 전환된 같은 Task에서 제거한다.
호출자가 raw snapshot을 직접 저장하거나 Task/Publications map을 통째로 교체하는 method는 두지 않는다.

## Task 전이와 invariant

TaskTransition action은 다음 값과 payload를 가진다.

| action | 허용 source | 결과와 조건 |
|---|---|---|
| `reserve_invocation` | `pending`, 수정 시 `gate_failed`·`review_blocked`, recovery 시 `terminated` | Worktree와 새 requestId/profile/runtime fingerprint 고정. 의존 Task는 `integrated`여야 함 |
| `mark_running` | `invocation_reserved` | provider invocation identity와 시작 시각 기록 |
| `request_termination` | `running` | `termination_pending`; timeout/cancel/error 원인 기록 |
| `confirm_termination` | `running`, `termination_pending` | 종료 identity와 시각 기록. 이후에만 candidate 검사나 새 호출 허용 |
| `record_candidate` | termination confirmed | exact SHA/tree/files 기록, `candidate_ready` |
| `record_gate` | `candidate_ready` | authoritative checks가 모두 통과하면 `gate_passed`, 아니면 `gate_failed` |
| `record_review` | `gate_passed` | fresh review accept면 `accepted`, block이면 `review_blocked` |
| `record_integration` | `accepted` | candidate와 integration relation 확인 후 `integrated` |
| `needs_operator` | 모든 미완료 상태 | 근거와 nextAction을 보존하고 자동 진행 중단 |

새 invocation은 이전 invocation의 termination confirmation 없이는 예약할 수 없다.
검사/review 수정은 Task별 `RepairCount`를 함께 사용하고 기본 2회를 넘으면 needs_operator다.
확실한 일시적 runtime 실패만 `RecoveryCount`를 증가시키며 기본 1회를 넘으면 needs_operator다.
pause/resume, 새 requestId 또는 process 재시작으로 count를 초기화하지 않는다.
candidate/gate/review/integration evidence의 SHA가 단계 사이에서 달라지면 전이를 거부한다.

## Publication 전이와 invariant

PublicationTransition action은 `begin`, `complete`, `fail`, `conflict`다.

1. Publisher는 외부 쓰기 전에 `begin`을 적용해 pending intent, target, requestId와 payloadHash를
   저장한다.
2. remote marker/identity를 조회해 이미 같은 쓰기가 있으면 채택하고, 없으면 한 번 수행한다.
3. 정확한 remote identity를 얻은 뒤 `complete`로 receipt를 저장한다.
4. 일시 실패는 `fail`로 진단과 attempts를 보존한다. retry는 같은 logical intent만 재개한다.
5. remote 내용·base가 달라 안전하게 채택할 수 없으면 `conflict`와 nextAction을 남긴다.

같은 requestId와 payload는 원래 transition 결과를 replay한다. 같은 requestId의 다른 payload,
같은 key의 다른 target, completed receipt 덮어쓰기는 거부한다. Issue/Projects 동기화 실패는
Work Item 실행 상태를 publication_pending으로 바꾸지 않고 sync status만 갱신한다.
병합 후 필수 Wiki/Issue/Projects 발행 실패만 Work Item을 publication_pending으로 둔다.
publication retry는 TaskState, repair/recovery count와 candidate evidence를 변경하지 않는다.

## Module별 사용

- Task 4 Publisher module은 Publication transition만 사용한다. GitHub adapter는 상태 파일을
  직접 읽거나 runtime을 호출하지 않는다.
- Task 5 executor module은 Task transition만 사용한다. runtime adapter는 state를 직접 쓰지 않고
  구조화된 Artifact만 반환한다.
- workflow module은 두 결과에서 Work Item state, sync status와 nextAction을 계산한다.
- Monitor와 CLI status는 workflow aggregate projection만 읽는다. polling은 transition을 호출하지 않는다.

Task 4와 Task 5는 이 shared-state Task가 fresh review를 통과한 동일 base SHA에서 별도 worktree와
branch로 분기한다. `internal/state/v2`, shared types와 workflow projection은 병렬 worker 소유에서 제외한다.

## 오류와 복구

typed 오류는 stale revision/current state, request conflict, invalid transition, budget exhausted,
unterminated invocation, evidence mismatch, publication conflict를 구분한다. 오류는 snapshot을 바꾸지 않는다.
crash가 외부 publication 전이면 pending intent를 읽고 시작하며, 외부 쓰기 뒤 receipt 저장 전이면
remote marker를 조회해 채택 또는 conflict 처리한다. runtime 종료가 불명확하면 같은 Worktree의 새
호출·commit·검증을 막고 needs_operator로 남긴다.

v1 snapshot은 읽거나 변환하지 않는다. 기존 v2 foundation state에는 새 map이 없을 수 있으므로
이번 개발 자료에 한해 decode 시 empty map으로 취급할 수 있지만, 실제 승인된 Work Item 생성은
두 map을 non-nil로 기록한다. 이미 실행 중인 v2 업무가 없으므로 별도 자동 migration은 만들지 않는다.

## 테스트와 인수 조건

TDD focused 검사는 다음을 실제 임시 state store에서 확인한다.

- plan 생성 시 Contract Task와 정확히 일치하는 pending TaskStates 및 빈 Publications.
- 모든 정상 Task 전이와 잘못된 source/action/payload 거부.
- termination confirmation 전 중복 호출·candidate 처리 거부.
- repair 2회, recovery 1회 누적과 budget 소진 후 needs_operator.
- candidate/gate/review/integration SHA 불일치 거부.
- publication intent가 fake 외부 호출보다 먼저 durable한지 확인.
- 중단된 publication 채택, request replay/conflict, target conflict와 publication-only retry.
- publication retry가 Task evidence와 budget을 byte-for-byte 보존.
- stale revision과 concurrent lease에서 외부 호출이 한 번만 예약됨.
- aggregate projection에 상태·sync·nextAction은 보이지만 provider/process/session identity는 없음.
- 기존 v2 foundation codec/state/CLI와 v1 suite가 유지됨.

shared-state worker는 `internal/state/v2`와 `internal/workflow` focused test/vet만 실행한다.
Task 4·5는 각자 변경 package만 검사하며 worker별 full suite를 실행하지 않는다.
통합 code PR 마지막에 `make check`를 한 번 실행한다.

## 제외

- 실제 Codex process 관리, Git commit/check와 GitHub API 구현 자체.
- PR merge relation, Wiki Git 발행과 Final Manifest의 전체 구현.
- 다중 저장소 cross-repository verification과 여러 Project scheduler.
- event sourcing, state file sharding과 자동 v1/v2 migration.
