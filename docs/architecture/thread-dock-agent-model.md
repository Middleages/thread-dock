# ThreadDock Agent Model

## 목적

ThreadDock이 여러 Agent의 상태·소통·결과·근거를 관찰할 때 사용할 최소 공통 모델을 정의한다.

이 문서는 `oh-my-pi`의 실행 엔진을 가져오기 위한 설계가 아니다. 현재 ThreadDock의 책임 경계인 GitHub = 업무 원본, Herdr = top-level 실행 lifecycle, native Agent = 실제 작업 실행, ThreadDock Monitor = 관찰·연결을 유지하면서 Agent 간 관계와 결과를 더 구조적으로 표현하기 위한 기준이다.

현재 기준은 `main`의 `AGENTS.md`, `PRODUCT.md`, `docs/architecture/agent-workflow.md`와 2026-10-17 Pi 연구를 따른다.

## 1. 핵심 판단

ThreadDock은 Agent runtime이 아니라 **Agent workflow observer/orchestrator**로 남긴다.

1. 실행은 기존 Codex/OpenCode/native subagent와 Herdr가 담당한다.
2. 업무의 durable source of truth는 GitHub다.
3. ThreadDock은 서로 다른 원본에서 관찰 가능한 상태를 결합하고, parent/child 관계와 task/result/evidence를 이해할 수 있게 한다.
4. 새로운 scheduler, session database, model router, permission engine, execution engine은 만들지 않는다.
5. Agent의 `running`이나 Herdr의 `done`을 업무 완료로 해석하지 않는다.
6. "reviewer를 호출했다"와 "review가 실제 실행되어 결과가 반환됐다"를 구분한다.

## 2. 현재 책임 경계

| 구성 | 원본 책임 | ThreadDock의 역할 |
|---|---|---|
| GitHub | Issue/PR/Projects, 결정·완료·검증 기록 | 업무 truth 조회 및 연결 |
| Herdr | top-level session/workspace/tab/pane lifecycle | 실행 위치와 상태 관찰 |
| Codex/OpenCode/native subagent | 실제 계획·구현·검토 실행 | 실행 결과의 관찰 가능한 receipt 연결 |
| `sessions.json` | GitHub ↔ Herdr 명시적 local binding | locator 조회 |
| ThreadDock Monitor | 관찰 UI | 위 정보를 결합해 Agent workflow를 보여줌 |
| 사용자 | 목적·범위·중요 선택·main merge | 최종 권한 및 판단 |

native leaf를 Herdr top-level session이나 locator에 복제하지 않는다. 현재 Monitor의 Herdr 모델도 `HerdrSession`, `HerdrAgent`, `HerdrConnection`을 통해 실행 위치를 관찰하는 구조이므로, Agent workflow 모델은 이 위에 얹는 projection으로 설계한다.

## 3. Agent identity와 lifecycle

### 3.1 Agent identity

Agent는 실행 위치 자체가 아니라 **작업을 수행하는 논리적 주체**다.

최소 식별자는 다음과 같다.

| 필드 | 의미 |
|---|---|
| `agent_id` | workflow 내 논리적 Agent 식별자 |
| `parent_agent_id` | 생성/위임한 Agent. root면 없음 |
| `role` | Coordinator, Feature Leader, implementer, reviewer 등 |
| `runtime` | Codex, OpenCode, native 또는 기타 관찰된 실행기 |
| `task_id` | 현재 수행하는 논리적 Task |
| `execution_ref` | 실제 runtime/Herdr/native execution을 가리키는 선택적 참조 |

`agent_id`와 `execution_ref`는 같은 개념으로 취급하지 않는다. 하나의 Agent 개념이 runtime마다 다른 locator를 가질 수 있고, 반대로 runtime의 상태가 곧 업무 의미를 결정하지 않기 때문이다.

### 3.2 lifecycle

관찰 가능한 공통 상태는 다음 최소 집합으로 시작한다.

```text
queued
running
waiting
completed
failed
cancelled
unknown
```

상태는 **execution state**다. 다음을 별도로 판단해야 한다.

```text
work status: proposed | active | blocked | ready | completed
verification: unverified | partial | verified
```

즉 `completed + unverified`는 유효한 상태이며, `running`이라고 해서 업무가 미완료라는 것 외의 의미를 부여하지 않는다.

외부 runtime이 위 상태를 직접 제공하지 않으면 ThreadDock이 임의로 성공/실패를 만들어내지 않는다. `unknown` 또는 source-specific status를 보존하고 notice를 표시한다.

## 4. Task model

Task는 Agent가 무엇을 해야 하는지 표현하는 논리 단위다. runtime의 prompt/session과 동일하지 않다.

최소 모델:

```text
task_id
parent_task_id
agent_id
role
objective
input_revision
constraints
acceptance
created_at
```

### 원칙

- `objective`는 한 작업의 목적을 표현한다.
- `input_revision`은 Task가 무엇을 기준으로 시작했는지 고정한다. 코드 작업이면 commit SHA/branch/PR revision 등이 될 수 있다.
- `constraints`는 scope, 권한, 읽기 전용 여부 등 실제 작업 경계를 표현한다.
- `acceptance`는 결과를 판단할 최소 조건이다.
- Task 자체는 durable 업무 원본이 아니다. 필요하면 GitHub Issue/PR을 참조한다.

### 현재 구조와의 대응

- Coordinator → Feature Leader brief는 parent Task → child Task 관계로 표현할 수 있다.
- Feature Leader → bounded implementation child도 같은 모델을 사용한다.
- fresh reviewer는 별도의 child Task이며 구현 Agent와 동일한 Task로 합치지 않는다.

이 모델은 현재 Skill의 Task packet을 대체하지 않는다. 기존 packet을 구조화해서 표현할 수 있게 하는 것이 목적이다.

## 5. Result model

Result는 Task 실행이 반환한 구조화된 결과다.

최소 모델:

```text
result_id
task_id
agent_id
status
output_revision
summary
findings
verification
next_action
created_at
```

`status`는 실행 결과를, `verification`은 결과의 근거 수준을 나타낸다.

예:

```text
Task: review PR candidate
input_revision: 9c715df
agent: td_reviewer
status: completed
output_revision: 9c715df
verification: verified
findings: []
next_action: merge-ready
```

반대로 reviewer child가 시작되었지만 result를 회수하지 못했다면:

```text
status: unknown
verification: partial
next_action: rerun-or-inspect
```

이를 `ACCEPT`로 승격하지 않는다.

## 6. Agent-to-Agent communication

1단계에서는 자유 형식 chat/room 시스템을 만들지 않는다.

ThreadDock이 구조화해야 하는 소통은 우선 다음 세 종류다.

### 6.1 Dispatch

Parent Agent가 Child Agent에 Task를 넘긴다.

```text
parent_task_id
child_task_id
objective
input_revision
constraints
acceptance
```

### 6.2 Result handoff

Child Agent가 Parent Agent에 Result를 반환한다.

```text
child_task_id
result_id
status
findings
verification
next_action
```

### 6.3 Escalation

Child가 스스로 해결할 수 없는 blocker나 제품 선택을 Parent에 반환한다.

```text
reason
blocker
required_decision
scope_impact
```

이 세 가지면 현재 Coordinator → Feature Leader → leaf 구조의 핵심 소통을 표현할 수 있다. 자유 대화 기록, room, message broker, durable mailbox는 필요성이 실제로 확인될 때 별도 설계한다.

## 7. Evidence model

Evidence는 Result가 실제로 무엇을 근거로 하는지 연결한다.

최소 관계는 다음이다.

```text
Task
  └─ Agent execution
       └─ Result
            └─ Evidence
```

Evidence는 다음 유형을 우선 지원한다.

| 유형 | 예 |
|---|---|
| revision | commit SHA, PR head SHA |
| execution | native child lifecycle, Herdr execution reference |
| command | `go test ./...`, focused test, lint |
| review | reviewer result, verdict, finding |
| artifact | supplied native event/result artifact |
| record | GitHub Issue/PR/comment/handoff |

Evidence에는 최소한 `source`, `observed_at`, `subject/reference`, `claim`이 필요하다.

### Evidence의 원칙

- Evidence 수집기는 review 자체를 대신하지 않는다.
- receipt가 있다는 사실과 작업 품질은 별개다.
- 같은 SHA와 같은 command/environment의 신뢰 가능한 기존 근거는 재사용할 수 있다.
- 검증이 불가능하면 `unverified`로 남긴다.
- 파일 존재, prompt 문구, child 생성 성공만으로 실제 실행 완료를 주장하지 않는다.

이는 기존 `review-evidence-helper`의 역할과도 일치한다. helper는 supplied artifact에서 명시된 metadata를 요약할 뿐 substantive review나 authenticated attestation을 제공하지 않는다.

## 8. 현재 Monitor에 적용하는 방법

1단계에서는 새 저장소를 만들지 않고 **read-only projection**부터 만든다.

현재 Monitor가 가진 다음 정보를 재사용한다.

- Herdr session/agent 상태
- GitHub Issue/PR/Project 연결
- `sessions.json`의 명시적 binding
- 기존 handoff/verification 기록

여기에 다음 개념을 계산 가능한 형태로 연결한다.

```text
GitHub work
   │
   ├── Task
   │     ├── Agent
   │     │     └── execution reference → Herdr/native runtime
   │     └── Result
   │            └── Evidence → SHA / test / review / record
   │
   └── durable Issue / PR record
```

Monitor 화면에서 첫 번째로 보여줄 정보는 다음 정도면 충분하다.

1. 누가 무엇을 하고 있는가 — Agent/role/task
2. 누구에게서 파생되었는가 — parent/child
3. 현재 실행 상태는 무엇인가 — execution state
4. 업무 상태는 무엇인가 — work status
5. 결과가 반환되었는가 — Result
6. 결과를 뒷받침하는 근거가 있는가 — Evidence
7. 다음 행동은 무엇인가 — next action

새로운 실행 버튼이나 scheduler는 추가하지 않는다.

## 9. oh-my-pi에서 가져올 것과 가져오지 않을 것

### 가져올 것

- Agent identity와 parent/child lineage를 명시적으로 취급하는 방식
- 실행 event와 최종 result를 구분하는 방식
- 구조화된 child task/result
- 독립 context에서 수행된 작업을 결과/근거로 연결하는 방식
- 필요한 정보만 단계적으로 노출하는 작은 기본 surface

### 가져오지 않을 것

- coding-agent runtime 자체
- model/provider routing
- 자체 session persistence/runtime database
- scheduler/retry/re-execution engine
- permission/approval system
- 자체 tool/MCP execution layer
- Pi Durable의 장기 실행 storage model
- ThreadDock 내부에 별도 TUI/agent chat을 만드는 것

핵심은 구현물을 가져오는 것이 아니라 **책임 경계와 관찰 모델을 차용하는 것**이다.

## 10. 1단계 구현 범위

### 포함

1. 이 Agent Model 문서
2. Go 내부의 최소 read-only domain model 검토 및 필요한 타입 정의
3. 현재 GitHub/Herdr observation에서 Agent/Task/Result/Evidence projection 생성
4. parent/child lineage 표시
5. execution/work/verification 상태 분리
6. 기존 Monitor snapshot에 필요한 최소 필드만 추가
7. focused unit tests

### 제외

- Agent 실행/생성/취소 API
- scheduler
- retry engine
- 새로운 database/file persistence
- 자유형 Agent chat
- 모델 라우팅
- 권한/승인 변경
- MCP orchestration
- native subagent를 Monitor pane으로 복제
- oh-my-pi 코드 fork/import

## 11. 구현 순서

### Phase 1-A — 공통 모델

public/shared interface를 먼저 최소화한다.

- Agent identity
- Task
- Result
- Evidence
- lifecycle/status enum

shared type은 한 곳에서 먼저 결정하고 이후 독립적인 projection 작업을 병렬화할 수 있다.

### Phase 1-B — projection

현재 GitHub/Herdr adapter에서 관찰 가능한 사실을 공통 모델로 변환한다.

- Herdr status → execution state
- GitHub Issue/PR/handoff → work/result/evidence reference
- existing session binding → execution reference
- child relation은 실제 근거가 있을 때만 연결

### Phase 1-C — Monitor

기존 snapshot을 깨뜨리지 않는 범위에서 Agent tree와 상태를 표시한다.

UI가 표시할 수 없는 정보는 억지로 채우지 않고 `unknown`/`unverified`로 표현한다.

### Phase 1-D — 검증

최소 검증은 다음과 같다.

- projection unit tests
- stale/missing Herdr 상태
- GitHub 연결 없는 session
- session 없는 GitHub work
- child result 미회수
- 동일 revision의 verified evidence 재사용
- runtime 완료와 업무 완료가 다른 사례

실제 Codex/OpenCode 실행 시험은 모델 자체가 아니라 현재 workflow 검증 범위가 필요해지는 시점에 별도 수행한다.

## 12. 단순화 기준

다음 중 하나가 필요해지지 않는 한 모델을 확장하지 않는다.

- 현재 Monitor에서 실제로 표현할 수 없는 관계가 반복적으로 발생한다.
- parent/child 결과를 구조화하지 않아 같은 오류가 반복된다.
- review/verification 근거를 연결하지 않아 완료 판정이 흔들린다.
- Herdr/GitHub observation만으로는 실제 workflow를 구분할 수 없는 사례가 누적된다.

반대로 단순히 `oh-my-pi`에 존재한다는 이유만으로 field, event, storage, protocol을 추가하지 않는다.

## 13. 성공 기준

1단계가 성공했다고 말하려면 다음을 실제로 확인할 수 있어야 한다.

- 하나의 feature에 대해 Coordinator → Feature Leader → leaf/reviewer 관계를 관찰할 수 있다.
- 각 실행의 execution state와 업무의 work status를 혼동하지 않는다.
- child가 실행되었다는 사실과 child result가 실제로 반환되었다는 사실을 구분한다.
- 결과의 candidate revision과 verification evidence를 함께 볼 수 있다.
- GitHub/Herdr 중 한쪽이 없어도 다른 쪽의 관찰 사실을 숨기지 않는다.
- 새 scheduler/session DB/runtime을 추가하지 않고 구현된다.

## 14. 다음 단계의 판단 기준

1단계 구현 후 실제 사용에서 다음 문제가 반복될 때만 2단계를 검토한다.

- Agent 간 메시지 자체가 필요하다 → communication model 확장
- execution event를 지속적으로 놓친다 → native event/receipt adapter 검토
- 여러 feature의 dependency를 관찰하기 어렵다 → graph/dependency projection 검토
- Monitor에서 개입이 필요하다 → intervention gate를 별도 설계

그 전에는 관찰 모델을 더 크게 만들지 않는다.
