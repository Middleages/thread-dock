# Workflow Shared State Design

## 상태와 목적

이 문서는 [Project Workflow MVP 설계](2026-09-07-project-workflow-mvp-design.md)의
단일 저장소 흐름에서 Publisher와 runtime/Git 실행이 공유할 local state seam을 고정한다.
[단일 저장소 구현 계획](../plans/2026-09-07-single-repository-workflow.md)의 Task 4·5를
병렬화하기 전에 승인, 실행, 검토, 발행, 복구의 원자성 경계를 명확히 한다.

PR #49에서 Contract v2, registry, revision CAS, requestId 멱등성, aggregate status와
project/work CLI foundation을 구현했다. 후속 module이 각자 호출·검토·publication 상태를
만들면 중복 실행 차단, repair budget, publication-only 복구를 한 invariant로 보장할 수 없다.

## 선택

`internal/state/v2.WorkSnapshot`을 Work Item의 유일한 authoritative local record로 사용한다.
여기에 typed Work 상태, `TaskStates`, publication intent와 receipt를 둔다. state/v2가
revision CAS, request replay, typed transition validation, budget과 aggregate reduction을
한 번의 atomic file replacement로 적용한다.

Task별 파일, event log, distributed lock, TTL 기반 lease stealing은 도입하지 않는다.
한 workstation의 Work Item coordinator owner와 짧은 state Apply lease로 범위를 제한한다.

## WorkSnapshot의 권한 경계

`WorkSnapshot`은 다음 자료를 함께 보존한다.

- 승인 계약과 contract hash
- 승인 근거, `PauseRequested`, typed operator blocker를 담는 `WorkControl`
- Contract의 모든 Task에 대한 `TaskExecutionState`
- immutable publication intents와 completed receipts
- revision 및 request replay receipts
- 기존 Work Item status, sync status, nextAction

기존 `WorkState` 값은 유지하고 동일한 상태 enum을 하나 더 만들지 않는다. `WorkControl`의
승인·중단·blocker와 Task/publication 근거를 reducer 입력으로 사용한다. 모든 Task가
`pending`이어도 승인된 Work라는 뜻은 아니다. 승인은 `approve`, 중단 요청은 `pause/resume`,
operator blocker 해제는 `resolve`만 변경한다. 실패 전이는 blocker를 추가할 수 있다.

각 성공 transition에서 state/v2 reducer가 Task/publication 세부 상태와 함께 기존
Work status, sync status, nextAction을 결정적으로 계산해 같은 atomic write에 넣는다.
두 번째 write는 필요하지 않다. workflow module은 이 결과의 projection을 제공할 뿐이며,
state/v2가 workflow를 import하지 않는다.

## Transition interface

공개 store seam은 다음과 같으며 Work/Task/Publication 중 정확히 하나를 받는다.

```go
type TransitionRequest struct {
    WorkID           contractv2.WorkID
    ExpectedRevision contractv2.Revision
    RequestID        contractv2.RequestID
    PayloadHash      string
    Work             *WorkTransition
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

호출자가 snapshot을 직접 바꾸는 raw `Mutate`, map 전체 교체, arbitrary closure 기반 저장은
남기지 않는다. 기존 caller 전환과 제거를 같은 변경에서 완료한다.

`Apply`는 짧은 state lease 안에서 다음을 한 번에 수행한다.

1. snapshot과 immutable contract/hash 확인
2. transition request ID와 payload hash replay 확인
3. expected revision CAS 확인
4. typed transition 및 evidence/budget invariant 검증
5. 세부 상태와 aggregate 필드 reduction
6. revision 증가, replay receipt 기록, atomic replacement

payload hash는 action과 typed payload 전체의 canonical hash로 검증한다. 같은 transition
request ID와 같은 payload는 최초 응답과 동일한 bounded command 결과만 반환하며 receipt를
재귀적으로 포함하지 않는다.
replay receipt나 `pending` 상태는 runtime, Git, GitHub 호출 권한이 아니다. 외부 작업을
시작할 권한은 아래 coordinator dispatch 절차에서만 얻는다.

검증 오류는 snapshot을 바꾸지 않는다. 반면 budget 소진이나 명시적 operator 전환은
근거와 남은 한도를 보존하는 durable `needs_operator` 결과다.

## Work 전이

`WorkTransition`은 다음 action만 허용한다.

| action | 허용 source | 결과와 조건 |
|---|---|---|
| `approve` | `awaiting_approval` | approvalRef와 승인 contract hash를 고정하고 `queued` |
| `pause` | 미완료 승인 상태 | `PauseRequested`를 즉시 저장하고 새 dispatch 차단; 종료 확인 뒤 reducer가 `paused` 표시 |
| `resume` | `paused` | reconcile 완료와 실행 가능 증거가 있을 때 `queued` 또는 계산된 다음 상태 |
| `resolve` | `needs_operator` 또는 publication conflict가 있는 승인 상태 | 대상 blocker만 typed evidence로 해제; 일반 sync conflict는 실행 상태 보존 |

승인 전에는 Task invocation 예약과 publication intent 생성을 모두 거부한다. paused,
pause 요청 중, needs_operator, completed에서도 새 실행을 막으며 resolution·종료·receipt
확정처럼 진행 중 동작을 정리하는 전이는 허용한다.
`approve` replay만 원래 승인 결과를 반환하며 새 실행을 시작하지 않는다.

`pause` 요청 시 active invocation이 있으면 먼저 termination을 요청하고 확인한다.
종료가 확인되기 전에는 새 invocation, candidate 검사, Git write, publication write를 막는다.
이미 시작된 publication은 receipt 또는 불명확한 결과를 기록한 뒤 멈추며 취소된 것으로 추정하지 않는다.
`resume`은 budget을 초기화하거나 불명확한 runtime을 종료된 것으로 간주하지 않는다.

`resolve` kind는 `runtime_not_started`, `runtime_terminated`, `retry_verified_stage`,
`extend_budget`, `publication_reconciled`다. operator reference, 대상 Task/Invocation 또는 intent, 검증 증거와
필요한 승인 근거를 받는다. 증거에 맞는 재개 stage를 reducer가 정하며 caller가 임의 상태를
지정하지 않는다. unknown runtime이 남으면 해제를 거부하고 종료된 Task의 근거·count는
보존한다. 한도 증가는 승인된 ceiling만 늘리고 사용한 count를 줄이지 않는다.
`runtime_not_started`는 아래 reconcile과 같은 증거 검사·Task 복원을 같은 Apply에서 수행한다.
`publication_reconciled`는 exact intent/target의 원격 일치 증거와 receipt가 있으면 completed로
채택하고, 이전 요청이 더 이상 실행 중이지 않으며 미발행이 입증되면 failed로 복원해 동일
intent 재시도를 허용한다. 증거가 모호하면 conflict를 유지한다. 원격 내용을 덮어쓰거나
payload/target을 바꾸는 resolution은 없으며, Task evidence와 budget은 변경하지 않는다.

## Task와 시도 identity

Contract의 모든 Task는 plan 생성 시 `pending` state를 가진다. map key와 내부 Task ID는
같아야 하고 Contract에 없는 key는 거부한다.

`TaskExecutionState`는 다음 개념을 분리해 가진다.

- `BuilderAttempt`: candidate를 만드는 builder 시도 번호
- `RepairCount`: 검사 실패와 reviewer block이 공유하는 누적 수정 budget
- `RecoveryCount`: 확실한 일시적 runtime 실패의 누적 retry budget
- 현재 `InvocationState`
- 현재 논리 작업(`LogicalWorkID`, role, BuilderAttempt, 호출 목적, budget 차감 내역)
- candidate, gate, review, integration evidence
- bounded prior-attempt summary

WorktreeIdentity의 canonical path, Git common-dir, branch, base SHA도 보존한다. 최초 builder는
`pending`에서 예약하며 의존 Task가 모두 `integrated`이고 해당 baseline이 반영되어야 한다.
profile/runtime fingerprint는 승인된 실행 설정과 대조하고 재시도로 몰래 변경하지 않는다.

`InvocationState`는 invocation ID, LogicalWorkID, transition request ID, role, return stage, logical profile,
runtime fingerprint, provider identity, 시작·종료 시각, termination confirmation을 가진다.
role은 `builder` 또는 `reviewer`이며 invocation ID는 builder attempt 번호와 별개다.
runtime packet/Artifact의 requestId는 이 invocation ID와 일치해야 한다. 각 lifecycle
transition의 RequestID는 별도이며 payload에 대상 invocation ID와 builder attempt를 명시한다.
reserve payload도 LogicalWorkID를 받는다. 최초 builder·새 repair·새 reviewer는 새 논리 작업을
만들고, 미시작 재예약과 일시 실패 recovery는 기존 작업 ID/role/attempt를 대조한다. 논리 작업과
차감 내역은 Task에 저장되어 invocation의 abandonment·교체 뒤에도 남으며 임의 ID 재사용은 거부한다.

`returnStage`는 미시작 예약을 해제할 때 복원할 논리 단계다. builder는 `pending` 또는
수정 대기 단계, reviewer는 `gate_passed`다. 정상 종료는 `terminated`를 유지한다. reviewer도 같은
`invocation_reserved` → `running` → `termination_pending` → `terminated` lifecycle을 사용한다.
review 전용 running status를 추가하지 않는다.

TaskStatus는 다음 값만 허용한다.

`pending`, `invocation_reserved`, `running`, `termination_pending`, `terminated`,
`candidate_ready`, `gate_failed`, `gate_passed`, `review_blocked`, `accepted`, `integrated`,
`needs_operator`.

Candidate evidence는 builder attempt, candidate SHA, tree SHA, changed files를 가진다.
Gate evidence는 같은 builder attempt와 exact candidate SHA, commands, outcomes, observedAt을
가진다. Review evidence는 reviewer invocation ID, 같은 builder attempt와 review SHA,
accept/block, bounded findings를 가진다. Integration evidence도 같은 builder attempt와
accepted candidate 및 integration HEAD를 가진다.

provider session/pane/process identity는 local state에만 보존하고 Monitor나 GitHub 요약에는
노출하지 않는다. Agent가 쓴 자체 성공 문구는 authoritative evidence로 취급하지 않는다.

## Invocation 전이

TaskTransition의 호출 lifecycle action은 역할과 관계없이 같다.

| action | 허용 source | 결과와 조건 |
|---|---|---|
| `reserve_invocation` | 역할에 맞는 실행 가능 stage | role, returnStage, 새 invocation ID/profile/runtime 고정 |
| `begin_launch` | `invocation_reserved`, 아직 launch 미요청 | 같은 상태에 `LaunchRequested=true`를 durable하게 기록; runtime 호출 전에 필수 |
| `mark_running` | `invocation_reserved` | provider identity와 startedAt 기록 |
| `request_termination` | `running` | `termination_pending`, timeout/cancel/error 원인 기록 |
| `confirm_termination` | `running`, `termination_pending` | 종료 identity와 시각 기록 후 `terminated` |
| `reconcile_not_started` | `invocation_reserved`, positive no-launch proof | 예약을 abandoned로 보존하고 returnStage 복원; 이미 차감한 count와 논리 작업 보존 |

모든 action은 현재 invocation ID를 대조한다. `mark_running`은 begin_launch 이후만 허용한다.
dispatcher는 LaunchRequested=false를 확인한 현재 command에서만 begin_launch 후 한 번
호출한다. true인 예약이나 begin_launch replay는 재호출하지 않고 reconcile한다.
`needs_operator`에서 확보한 종료 증거는 Work `resolve(runtime_terminated)`가 같은 Task에
적용한다. 명시적 pause로 취소한 호출도 종료 확인 뒤 같은 논리 작업으로 resume할 수 있으며,
이 경우 count를 늘리거나 초기화하지 않고 새 invocation ID를 사용한다.

builder 최초 예약은 `BuilderAttempt`를 시작하지만 repair budget을 쓰지 않는다. `gate_failed`
또는 `review_blocked`에서 repair builder를 예약하는 atomic transition은 다음을 함께 수행한다.

1. budget 잔여 확인 후 `RepairCount` 증가; 소진이면 count 증가·새 예약 없이 needs_operator 기록
2. `BuilderAttempt` 증가
3. 현재 candidate, gate, review, integration evidence 무효화
4. 무효화 전 attempt의 SHA, 결과, 실패 이유를 bounded summary로 보존
5. 새 builder invocation 예약

이 원자성 때문에 budget을 쓴 뒤 이전 candidate가 active로 남거나, invocation만 예약되고
count가 누락될 수 없다. 동일 repair 예약의 request replay는 count를 다시 올리지 않는다.
미시작이 입증된 예약을 해제한 뒤 같은 논리 작업을 새 invocation ID로 예약해도 이미
차감한 budget과 BuilderAttempt를 재사용한다. 이를 위해 미시작 예약의 논리 작업 ID를 보존한다.

reviewer 예약은 `BuilderAttempt`나 `RepairCount`를 바꾸지 않고 candidate와 gate를 보존한다.
reviewer는 exact candidate/gate evidence를 packet으로 받고, 종료 확인 뒤에만 review 결과를
기록한다. reviewer block 후 repair builder 예약이 이루어질 때 그 review를 함께 무효화한다.

## Candidate, gate, review, integration 전이

| action | 허용 조건 | 결과 |
|---|---|---|
| `record_candidate` | `terminated`, 정상 builder 결과, current invocation/attempt 일치 | `candidate_ready` |
| `record_gate` | current attempt의 `candidate_ready` | pass면 `gate_passed`, fail이면 `gate_failed` |
| reviewer `reserve_invocation` | current attempt의 `gate_passed` | 공통 invocation lifecycle 시작 |
| `record_review` | `terminated`, 정상 reviewer 결과, current invocation/attempt/SHA 일치 | accept면 `accepted`, block이면 `review_blocked` |
| `record_integration` | current attempt의 `accepted` | 관계 확인 후 `integrated` |
| `needs_operator` | 모든 미완료 상태 | typed reason/evidence/budget을 보존하고 자동 진행 중단 |

candidate, gate, review, integration이 참조하는 builder attempt와 candidate SHA는 같아야 한다.
integration HEAD 자체는 별도 SHA이며 accepted candidate와의 Git 관계를 확인한다.
이전 attempt의 reviewer 결과나 gate 결과를 현재 candidate에 적용할 수 없다.

기본 repair budget은 Task별 누적 2회이며 검사 실패와 reviewer block이 공유한다.
기본 recovery budget은 확실한 일시적 runtime 실패에 1회다. pause/resume, 새 transition
request ID, reviewer invocation, process restart는 count를 초기화하지 않는다.
종료가 확인된 일시 실패의 `terminated`에서 recovery 예약은 같은 role/BuilderAttempt와
candidate/gate를 보존하고 새 invocation ID와 RecoveryCount 증가를 원자적으로 기록한다.

한도 소진을 발견하면 transition은 remaining budget, 원인, last evidence를 기록하면서
Task와 Work를 `needs_operator`로 durable하게 전환한다. 이는 잘못된 source, stale revision,
payload mismatch 같은 no-write validation error와 구분된다.

## 예약 reconcile과 runtime 불확실성

재시작 뒤 `invocation_reserved`를 reconcile할 때 `not_started`로 되돌릴 수 있는 조건은
해당 invocation이 실제 runtime에 전달되지 않았다는 positive proof가 있을 때뿐이다.
모든 runtime 호출은 durable `begin_launch` 이후에만 이루어진다. 이전 owner 종료가 확인되고
`LaunchRequested=false`이면 미시작 증거다. true이거나 상태가 불명확하면 단순 provider ID
누락으로 해제하지 않는다. 시작이 확인되면 identity를 기록해 기존 호출을 관찰·종료한다.

runtime이 active이면 기존 invocation을 관찰하거나 종료한다. runtime 상태가 unknown이거나
provider identity가 불충분하면 새 invocation을 예약하지 않는다. Task와 Work를 typed
`needs_operator`로 전환하고 마지막 known identity, 조회 결과, 시간, budget을 보존한다.

`reconcile_not_started` 뒤 같은 논리 작업을 새 ID로 예약하는 것은 새 builder attempt, repair,
recovery로 세지 않는다. 반대로 단순 timeout, owner process 재시작, 오래된 timestamp는
미시작 증거가 아니며 TTL 만료로 live owner나 runtime을 steal하지 않는다.

malformed/stale Artifact도 자동 성공이나 새 invocation으로 바꾸지 않는다. 변경된 파일과
원인을 보존한 `needs_operator` resolution 대상이다.

## Publication identity와 세대

Publication은 다음 identity를 구분한다.

- `PublicationKey`: `project-item`, `parent-issue`, `pr:<repoKey>`, `wiki:<page>` 같은
  stable remote resource의 logical key
- `Generation`: 같은 logical key에 대한 단조 증가 intent 세대
- `PublicationIntentID`: key와 generation을 식별하는 immutable ID
- transition `RequestID`: begin/complete/fail 같은 local command의 replay ID

`Publications`는 intent ID로 key를 두며 각 intent는 logical key와 generation을 가진다.
같은 key-generation은 하나만 존재한다. 최신 세대는 저장된 intent의 key별 최대 generation으로
계산하며 별도 index를 저장하지 않는다.

각 intent의 kind, payload hash, target은 생성 뒤 immutable하다. 실제 payload는 bounded
canonical payload 또는 immutable 승인 artifact reference로 재구성하고 hash를 대조한다.
kind는 `parent_issue`, `child_issue`, `project_item`, `handoff`, `pull_request`, `wiki`다.
동일 logical key의 원격 host/resource identity는 세대 사이에도 유지한다.
Target은 host/repository, logical key와 필요한 base identity만 가지며 credential은 저장하지
않는다. completed receipt는 원격 node/number/URL, base/head identity와 publishedAt을 보존한다.

새 board status나 handoff 내용을 발행할 때 같은 logical key의 다음 generation을 만들 수 있다.
이때 이전 completed receipt를 덮어쓰거나 삭제하지 않는다. 이전 generation의 늦은 retry는
최신 generation의 target/content/status를 변경할 수 없고 remote write도 수행할 수 없다.
그 요청은 typed `stale_generation` 결과로 기록하거나 이미 기록된 결과를 replay한다.

PublicationStatus는 `pending`, `completed`, `failed`, `conflict`, `superseded`다.
LastError는 credential과 provider raw output을 제외한 1 KiB 이하 진단이다.

## Publication 전이

PublicationTransition은 `begin`, `complete`, `fail`, `conflict`, `supersede`를 허용한다.
모든 action은 대상 intent ID를 받고, begin 이후에는 저장된 payload/target을 바꿀 수 없다.

1. `begin`은 외부 쓰기 전에 immutable intent ID/key/generation/payload/target을 저장한다.
2. dispatcher가 현재 generation을 다시 읽고 remote marker/identity를 조회한다.
3. 동일 쓰기가 확인되면 채택하고, 없다는 확실한 결과일 때 한 번 수행한다.
4. exact remote identity를 얻은 뒤 `complete`로 해당 intent의 receipt를 저장한다.
5. 일시 실패는 `fail`, 안전한 채택이 불가능한 내용/base 차이는 `conflict`로 남긴다.
6. 이전 intent가 완료됐거나 원격 쓰기 미발생이 확인된 경우에만 다음 generation을 만든다.
   후자의 미완료 intent는 `superseded` 처리와 새 begin을 같은 atomic transition에 묶는다.
   in-flight 또는 원격 결과가 unknown이면 먼저 reconcile하며 새 generation으로 건너뛰지 않는다.

`begin`은 새 intent 생성 또는 안전한 `failed → pending` 재개에 사용한다. 재개 command는
새 transition RequestID와 같은 intent ID를 사용하고 attempts는 누적 보존한다. pending은
새 시도로 만들지 않고 현재 owner가 reconcile한다. completed는 receipt만 반환하고,
conflict는 명시적 resolve 전 재시도하지 않으며 superseded는 remote write를 거부한다.

completed receipt는 immutable하다. retry는 동일 intent만 재개하며 Task evidence, builder attempt,
repair/recovery count를 변경하지 않는다. 병합 후 또는 코드 PR 없는 문서 업무의 검토 후
필수 Wiki/Issue/Projects 발행 실패는 Work를 `publication_pending`으로 두고, 실행 중 일반
sync 실패는 sync status만 갱신한다. 이 설계는 merge/final-review 근거 자체를 대신하지 않는다.

외부 호출 뒤 receipt 저장 전에 crash가 나면 marker를 조회한다. 동일 결과를 확인하면 채택하고,
다르면 conflict다. 조회 결과가 timeout, partial response, unknown처럼 모호하면 쓰기를 다시
보내지 않고 `needs_operator` 또는 publication conflict로 근거를 남긴다.
과거 요청이 아직 원격에서 처리 중일 수 있다면 marker 미발견만으로 미발행을 확정하지 않는다.

## Work Item coordinator와 외부 write

runtime invoke/terminate, Git 변경, GitHub/Wiki publication과 그 recovery의 실행 소유자는
Work Item coordinator 하나다. owner는 OS process lock을 생존 기간 동안 보유하므로
lookup → 외부 write/dispatch → local completion receipt 사이에 소유자가 바뀌지 않는다.

같은 owner process 안의 중복 goroutine도 막아야 한다. Work Item별 local dispatcher는 single
queue를 사용하며 publication은 한 번에 하나만 active다. publication begin/supersede와 세대
선택도 이 queue에서 처리하여 이전 원격 I/O 중 다음 generation이 끼어들지 못하게 한다.
dispatcher는 remote I/O 직전에
snapshot을 다시 읽어 owner, Work control state, current intent generation, pending status를
확인한다. 이 확인을 통과한 현재 command만 외부 작업을 수행한다.

긴 runtime 호출은 owner가 비동기로 관찰한다. command/event 처리 queue가 호출 종료를
기다리며 막히지 않아 pause·terminate와 결과 수신을 처리할 수 있다. 동일 Task의 새 호출은
기존 호출의 종료 또는 positive no-launch proof 없이는 dispatch하지 않는다.
owner 정상 해제 또는 process 종료로 OS lock이 풀린 뒤 새 owner가 획득한다. 획득만으로
자식 runtime 종료를 추정하지 않고 Git/runtime/remote evidence를 reconcile한 후 재개한다.
TTL만으로 live owner를 steal하지 않는다.

state `Apply` lease는 local CAS와 atomic replacement 동안만 잡는 별도의 짧은 lease다.
외부 I/O 동안 Apply lease를 유지하지 않으므로 status 조회와 허용된 Task 진행을 막지 않는다.
짧은 lease와 coordinator owner lock은 서로 대체하지 않는다.

pending intent, replay receipt, 오래된 queue item을 본 caller는 직접 launch할 수 없다.
항상 coordinator dispatcher를 거쳐야 하며 caller에는 저장된 command result만 반환한다.

## Module별 사용

- Publisher는 PublicationTransition과 coordinator dispatcher만 사용한다.
- executor는 TaskTransition을 적용하고 runtime adapter는 state를 직접 쓰지 않고 Artifact만 반환한다.
- Git adapter도 state를 직접 바꾸지 않고 coordinator가 evidence를 transition으로 기록한다.
- workflow는 WorkSnapshot의 aggregate projection을 읽어 Monitor/CLI에 제공한다.
- polling과 status 조회는 transition이나 외부 호출을 시작하지 않는다.

Task 4와 Task 5는 이 shared-state 변경이 fresh review를 통과한 동일 base SHA에서 분기한다.
`internal/state/v2`, shared types, aggregate reducer는 병렬 worker 소유에서 제외한다.

## 오류와 복구

typed 오류/결과는 최소한 다음을 구분한다.

- stale revision/current state, request conflict, invalid transition
- unterminated 또는 unknown invocation, evidence/attempt/SHA mismatch
- publication conflict, stale publication generation, ambiguous remote result
- budget exhausted의 durable `needs_operator`

잘못된 source/identity/payload, stale revision과 evidence mismatch 같은 validation failure는
write하지 않는다. 확인된 runtime ambiguity, malformed Artifact, publication conflict와
budget exhaustion은 명시적 typed transition으로 진단을 저장한다. 해제는 Work resolve를 거친다.

v1 snapshot은 읽거나 변환하지 않는다. 기존 v2 foundation state의 새 map 누락은 이번 개발
자료에 한해 empty map으로 decode할 수 있지만 승인된 새 Work Item은 non-nil map과 명시적인
WorkControl을 기록한다. 자동 migration은 만들지 않는다.

## Focused 인수 조건

- plan의 pending Task와 `awaiting_approval` Work가 분리되고 승인 전 invocation/publication 거부.
- `approve`가 `awaiting_approval`에서 `queued`로 한 번만 전이하며 replay는 실행하지 않음.
- `pause`, 종료 확인, reconcile, `resume`, typed `resolve`가 evidence와 budget을 보존.
- builder와 reviewer가 같은 reservation/running/termination lifecycle을 사용하고 role/returnStage로 구분.
- reviewer 예약 동안 candidate/gate가 보존되고 reviewer 결과의 attempt/SHA mismatch가 거부됨.
- repair 예약이 count·builder attempt 증가와 candidate/gate/review/integration 무효화를 원자 적용.
- 동일 repair 예약 replay와 reservation-not-started 재전송이 budget을 중복 차감하지 않음.
- current attempt의 candidate/gate/review/integration SHA 연결과 bounded prior summary 확인.
- coordinator owner와 in-process single dispatcher 경쟁에서도 외부 write/dispatch가 한 번만 수행됨.
- dispatcher가 remote I/O 직전 current intent를 다시 읽고 오래된 queue item을 거부함.
- 외부 I/O 동안 Apply lease가 해제되어 status/허용된 Task 전이가 가능하고 pending/replay만으로 launch할 수 없음.
- live owner TTL stealing이 없고 ambiguous publication 결과를 blind resend하지 않음.
- intent ID와 key-generation uniqueness, immutable payload/target, immutable completed receipt 확인.
- 같은 board logical key의 후속 generation 허용과 이전 retry의 최신 세대 overwrite 차단.
- reserved reconcile은 positive no-launch proof일 때만 not_started로 처리.
- active/unknown runtime에서 새 invocation을 막고 typed needs_operator evidence/budget을 보존.
- budget exhaustion은 durable needs_operator이고 stale/invalid request는 no-write error임.
- 각 transition이 Work status, sync status, nextAction을 같은 revision에서 결정적으로 갱신.
- 종료를 기다리는 runtime이 있어도 pause/terminate를 처리하며 provider identity는 Monitor/CLI projection에 없음.
- 기존 v2 plan/approve/status 및 v1 suite 호환 유지; 문서 수정만으로 실제 구현 완료를 주장하지 않음.

shared-state 변경은 state/workflow focused parse·vet·test 범위로 검증한다. Task 4·5는 각 변경
package만 검사하고, 통합 code PR의 마지막 gate에서 `make check`를 한 번 실행한다.

## 제외

- 실제 runtime process manager, Git 명령, GitHub/Wiki API 구현.
- PR merge relation, Wiki Git 발행과 Final Manifest의 전체 구현.
- distributed coordinator, fencing token, TTL owner takeover.
- event sourcing, state sharding, 자동 v1/v2 migration.
- 다중 저장소 verification과 여러 Project scheduler.
