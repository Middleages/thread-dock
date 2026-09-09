# Herdr Builder Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 승인된 v2 Builder invocation을 기존 coordinator에서 Herdr/OpenCode로 전달하고, 현재 invocation의 구조화 결과를 실제 Git 근거와 대조해 `candidate_ready`로 정착시킨다.

**Architecture:** Task 1이 기존 `runtime.Invocation`/`runtime.ArtifactEnvelope`를 lifecycle port에 연결하고, queue 실행과 restart reconcile이 공유하는 단일 Builder candidate 수용 함수를 만든다. Task 2는 기존 `internal/herdr.CLI`만 조합하는 얇은 adapter이며, 정확한 request/result marker가 없으면 idle/done이나 prompt 성공을 종료 증거로 승격하지 않는다.

**Tech Stack:** Go, `internal/state/v2` durable Store, `internal/worktree.Git`, Herdr CLI v0.8.2, OpenCode v1.18.27

**Spec:** `docs/superpowers/specs/2026-09-09-herdr-builder-bridge-design.md`

## Global Constraints

- 기준 SHA는 `f65491298e08b28cdef2b921b573e1ba74cfb42c`이며 두 Task는 직렬 실행한다.
- 첫 범위는 Builder 실행, 결과 회수, Git 대조, `candidate_ready`까지다. Reviewer, Documenter, GitHub 발행, UI, 새 process/session registry는 제외한다.
- 기존 `runtime.Invocation`과 `runtime.ArtifactEnvelope`를 운반 계약으로 사용한다. 두 번째 envelope나 독립 실행 경로를 만들지 않는다.
- `Runtime.Terminate`의 `nil`은 exact invocation 종료가 확인됐다는 뜻이다. Herdr 0.8.2의 `send-keys esc`, idle/done, `agent prompt --wait` 성공만으로 `nil` 또는 `ended`를 반환하지 않는다.
- `agent prompt --wait`는 turn을 추적하지 않고 이미 진행 중인 turn의 완료와 일치할 수 있으므로, 모호한 prompt 결과를 자동 재전송하지 않는다.
- configuration 원문, terminal transcript, secret을 Store나 진단에 기록하지 않는다.
- Luna high가 구현과 focused test/self-review를 수행하고, 고정 commit SHA를 fresh Sol medium이 검토한다. 모델/effort 자동 승격은 하지 않는다.
- worker별 full suite는 금지한다. 두 Task 검토 후 통합 branch의 마지막 gate로만 `make check`를 한 번 실행한다.
- 첫 stage와 fixture 검증 중에는 live agent를 시작하지 않는다.

## 고정 인터페이스

Task 1에서 아래 계약을 먼저 확정한다. Task 2는 이 계약만 소비하며 변경이 필요하면 구현을 멈추고 재계획한다.

```go
// internal/runtime/types.go
type VerificationResult struct {
    Command  string `json:"command"`
    Outcome  string `json:"outcome"`
    Duration string `json:"duration"`
}

type BuilderResult struct {
    CommitSHA    string               `json:"commitSha"`
    Verification []VerificationResult `json:"verification"`
}

type BuilderPacket struct {
    TaskID             contractv2.TaskID        `json:"taskId"`
    AllowedPaths       []string                 `json:"allowedPaths"`
    AcceptanceCriteria []string                 `json:"acceptanceCriteria"`
    Verification       []contractv2.CommandSpec `json:"verification"`
}

const BuilderOutputSchema = "thread-dock.builder-result.v1"

// internal/coordinator/runtime.go
type RuntimeObservation struct {
    State            string
    ProviderIdentity string
    EndedAt          *time.Time
    Diagnostic       string
    Artifact         *runtime.ArtifactEnvelope
}

type Runtime interface {
    Observe(context.Context, statev2.InvocationState, runtime.Invocation) (RuntimeObservation, error)
    Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity, runtime.Invocation) (string, error)
    Terminate(context.Context, statev2.InvocationState) error
}

type CandidateInspector interface {
    InspectCommit(context.Context, string, string, string, string) (worktree.CommitInspection, error)
}

func NewCoordinator(state State, runtime Runtime, publisher Publisher, inspector CandidateInspector, locker OwnerLocker, ownerID OwnerID, pid int, startedAt time.Time) *Coordinator

// internal/worktree/git.go
type CommitInspection struct {
    CommitSHA    string
    TreeSHA      string
    Branch       string
    ChangedFiles []string
    Patch        string
}
```

`runtime.Invocation`의 값은 coordinator가 Store snapshot에서 결정적으로 만든다: `RequestID=contractv2.RequestID(invocation.InvocationID)`, `Role=runtime.RoleBuilder`, `ProfileID=invocation.LogicalProfile`, `Worktree=task.Worktree.CanonicalPath`, `OutputSchema=runtime.BuilderOutputSchema`, `ReadOnly=false`, `Packet=json.Marshal(runtime.BuilderPacket{...현재 contract Task...})`. 호출 직전에 RequestID/role/profile/worktree를 durable invocation identity와 다시 대조한다.

`ArtifactEnvelope.Result`는 strict `runtime.BuilderResult` JSON이다. Herdr adapter는 `herdr.Evidence{RequestID, CommitSHA, Verification}`를 이 결과로 정규화하고 `RequestID`, `RoleBuilder`, `Status:"success"`를 채운다. candidate 수용 함수는 `runtime.ValidateEnvelope` 후 unknown field/trailing data를 거부하는 strict decode를 하고, `InspectCommit(worktree.CanonicalPath, worktree.BaseSHA, worktree.Branch, result.CommitSHA)`를 호출한다. `TreeSHA`는 `git rev-parse <sha>^{tree}`의 실제 결과여야 한다.

---

### Task 1: Coordinator transport와 공통 Builder candidate ingestion

**Task packet**

```yaml
taskId: herdr-builder-01-ingestion
baseSHA: f65491298e08b28cdef2b921b573e1ba74cfb42c
deps: []
ownedPaths:
  - internal/runtime/types.go
  - internal/runtime/runtime.go
  - internal/runtime/types_test.go
  - internal/coordinator/runtime.go
  - internal/coordinator/dispatcher.go
  - internal/coordinator/dispatcher_test.go
  - internal/coordinator/reconcile.go
  - internal/coordinator/runtime_test.go
  - internal/coordinator/reconcile_test.go
  - internal/coordinator/integration_test.go
  - internal/coordinator/builder_ingestion.go
  - internal/coordinator/builder_ingestion_integration_test.go
  - internal/worktree/git.go
  - internal/worktree/git_test.go
  - internal/worktree/git_integration_test.go
worktree: /home/appuser/dev_system/.worktrees/runtime-adapter-ingestion
branch: agent/runtime-adapter-ingestion
forbiddenPaths:
  - go.mod
  - go.sum
  - internal/herdr/**
  - internal/orchestrator/**
  - cmd/**
interface: 고정 인터페이스 절 전체
acceptance:
  - queue와 restart reconcile이 동일한 수용 함수를 호출한다.
  - current builder artifact만 exact 종료 확인 뒤 실제 Store에 한 번 기록된다.
  - request/role/attempt 불일치, malformed 결과, 종료 전 결과, 허용 경로 밖 변경, Git relation 실패는 candidate를 쓰지 않고 malformed_artifact 또는 evidence_mismatch blocker가 된다.
  - TreeSHA와 changedFiles는 Agent JSON이 아니라 실제 Git에서 나온다.
tests:
  - go test ./internal/runtime ./internal/worktree ./internal/coordinator
result:
  changedFiles: []
  commitSHA: ""
  executedCommands: []
  outcomes: []
  unverified:
    - 실제 Herdr/OpenCode launch는 이 Task에서 실행하지 않음
  blockers: []
```

- [ ] `runtime.BuilderPacket`, `runtime.BuilderResult`, `runtime.VerificationResult`, `BuilderOutputSchema`와 strict result decode/validation 테스트를 먼저 작성한다. 빈/비정규 SHA, 빈 verification, unknown field, trailing JSON, 잘못된 outcome/duration을 실패시킨다.
- [ ] `worktree.Git.InspectCommit`의 실제 임시 Git repository 테스트를 추가한다. base 뒤 두 파일을 commit하고 반환 `TreeSHA`가 별도 `git rev-parse <candidate>^{tree}`와 같으며 changed files가 실제 diff와 같은지 검증한 뒤 구현한다.
- [ ] lifecycle interface를 고정 서명으로 변경하고 모든 기존 fake를 컴파일 가능하게 갱신한다. Launch와 Observe 직전 각각 Store에서 만든 동일 `runtime.Invocation` 값이 전달되는 테스트를 둔다.
- [ ] `builder_ingestion.go`에 단일 수용 함수를 만들고 queue의 ended settlement와 restart reconcile의 ended settlement가 termination을 durable하게 확인한 다음 이를 호출하게 한다. active/unknown 관찰에 Artifact가 붙으면 candidate를 기록하지 않고 operator blocker로 fail closed 한다.
- [ ] `builder_ingestion_integration_test.go`는 fake Store/Git을 쓰지 않는다. `statev2.NewStore(t.TempDir())`와 실제 임시 Git repository/worktree를 사용해 다음을 검증한다: current artifact의 `candidate_ready`, stale requestId/role 거부, malformed/trailing/oversized 결과 거부, 종료 전 결과 거부, allowed path 밖 실제 commit 거부, branch에 포함되지 않은 commit 거부, 동일 artifact 재관찰 시 revision/candidate 불변, 다른 candidate 충돌 시 기존 candidate 불변.
- [ ] focused command `go test ./internal/runtime ./internal/worktree ./internal/coordinator`를 한 번 실행하고 결과·미검증 사항을 packet `result`에 기록한 뒤 한국어 commit message로 commit한다.
- [ ] fresh Sol medium reviewer에게 고정 SHA, spec, packet, focused test 근거를 주어 task-review를 받고 block이면 같은 Luna에 수정과 영향 테스트만 재배정한다.

### Task 2: 기존 Herdr CLI를 사용하는 얇은 OpenCode Builder bridge

**Task packet**

```yaml
taskId: herdr-builder-02-bridge
baseSHA: f65491298e08b28cdef2b921b573e1ba74cfb42c
deps:
  - herdr-builder-01-ingestion
ownedPaths:
  - internal/herdr/runtime.go
  - internal/herdr/runtime_test.go
  - internal/herdr/testdata/v0.8.2/**
worktree: /home/appuser/dev_system/.worktrees/runtime-adapter-ingestion
branch: agent/runtime-adapter-ingestion
forbiddenPaths:
  - go.mod
  - go.sum
  - internal/runtime/**
  - internal/coordinator/**
  - internal/state/**
  - internal/orchestrator/**
  - cmd/**
interface: Task 1 고정 Runtime interface와 runtime Invocation/ArtifactEnvelope/BuilderResult
acceptance:
  - OpenWorktree, StartAgent, Prompt, GetInfo, ReadEvidence만 기존 CLI 구현으로 호출하며 새 session/process supervisor를 만들지 않는다.
  - 결정적 agent name과 persisted provider identity가 정확히 대조된다.
  - exact requestId가 있는 strict marked evidence만 ended+Artifact를 만들며 idle/done, prompt --wait 성공, send-keys esc는 종료 증거가 아니다.
  - prompt 전달이 모호하면 재전송하지 않고 unknown/error로 fail closed 한다.
  - exact termination capability가 없으므로 Terminate는 stable unsupported/unknown error를 반환하고 nil을 반환하지 않는다.
tests:
  - go test ./internal/herdr
result:
  changedFiles: []
  commitSHA: ""
  executedCommands: []
  outcomes: []
  unverified:
    - Herdr 0.8.2 native exact invocation stop/termination confirmation 미지원
    - OpenCode profile 권한과 publication credential/MCP 쓰기 제한의 live 적용 미검증
    - fixture 통과는 live capability 검증이 아님
  blockers: []
```

- [ ] 기존 runner seam과 v0.8.2 fixture를 사용하는 table test를 먼저 작성한다. Launch가 기존 worktree를 열고 OpenCode agent를 한 번 시작한 뒤 packet을 한 번만 Prompt하며, 이름/WorkspaceID/PaneID/SessionID/path가 invocation identity와 어긋나면 실패하는지 검증한다.
- [ ] 이름 생성은 invocation ID에서 결정적으로 만들고 Herdr 허용 문자/길이를 만족시킨다. 같은 이름이 다른 workspace/pane/session/path에 이미 대응하면 충돌 오류를 반환하며 임의 suffix나 새 session으로 회피하지 않는다.
- [ ] Observe fixture 테스트를 작성한다: working/blocked는 exact identity일 때만 active, idle/done인데 current marked evidence가 없으면 unknown, malformed 또는 다른 requestId evidence는 Artifact 없이 error, exact identity와 exact current requestId의 strict evidence만 `RuntimeObservationEnded`와 `ArtifactEnvelope`를 함께 반환한다. `EndedAt`은 adapter가 관찰 시각을 임의 생성하지 말고 exact evidence completion timestamp가 없으면 nil로 두며 coordinator 정책을 따른다.
- [ ] Prompt 오류/timeout 및 receipt 문자열만 있는 fixture에서 Prompt를 두 번 호출하지 않았음을 runner call count로 증명한다. `agent prompt --wait`의 성공만으로 ended를 만들지 않는다.
- [ ] `Terminate`는 v0.8.2에 exact invocation stop+confirmation 명령이 없다는 stable sentinel error(예: `ErrExactTerminationUnsupported`)를 반환한다. `CloseWorkspace`, `send-keys esc`, agent idle/done을 대체 종료로 호출하는 테스트/구현을 두지 않는다.
- [ ] focused command `go test ./internal/herdr`를 한 번 실행하고 결과·미검증 사항을 packet `result`에 기록한 뒤 한국어 commit message로 commit한다.
- [ ] fresh Sol medium reviewer에게 Task 1 dependency SHA와 Task 2 고정 SHA, fixture call log, spec을 주어 task-review를 받고 block이면 같은 Luna에 수정과 영향 테스트만 재배정한다.

## 통합과 live capability gate

- [ ] 두 task-review가 accept인 뒤 통합 branch에서 `make check`를 최종 gate로 한 번만 실행한다. 실패 시 원인을 소유 Task에 귀속해 Luna에 focused fix를 배정하고, 명확히 검증이 무효화된 경우에만 gate를 다시 실행한다.
- [ ] 첫 live pilot 전에 운영 prerequisite를 별도로 확인한다: Herdr `0.8.2`, OpenCode `1.18.27`, 기존 worktree를 여는 권한, 선택한 OpenCode Builder profile, 사용자 전용 Herdr state 권한, `pane_history=false`, 허용된 provider network/credential 정책. raw configuration 내용은 출력하거나 ledger에 복사하지 않는다.
- [ ] exact marked result가 invocation 완료 증거로 운영 승인되는지 실제 작은 worktree에서 확인하기 전에는 “종료 지원” 또는 end-to-end 성공으로 보고하지 않는다. native stop이 필요한 시나리오는 현재 capability blocker로 남기며 새 supervisor를 추가하지 않는다.

## Self-review 결과

- 설계의 첫 두 구현 단위, 공통 ingestion, request/role correlation, Git TreeSHA/allowed path, 멱등/충돌, fail-closed 종료 의미를 모두 Task에 매핑했다.
- Herdr/OpenCode의 알려지지 않은 명령은 계획에 넣지 않았다. 확인된 `send-keys esc`는 명시적으로 종료 증거에서 제외했다.
- Task 2는 Task 1 고정 interface에 직렬 의존하며 shared types의 동시 소유가 없다.
- 실제 Store와 실제 Git repository 검증을 요구해 fake가 transition/Git proof를 대신 통과시키는 false positive를 막았다.
