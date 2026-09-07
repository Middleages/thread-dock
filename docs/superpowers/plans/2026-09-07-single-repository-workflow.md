# Single-Repository Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 단일 Project·Repository·Work Item을 승인부터 사람 병합 확인과 Wiki 발행까지 연결하고, 같은 상태를 최소 Monitor 목록·상세에서 읽는 첫 end-to-end 흐름을 만든다.

**Architecture:** 기존 v1 `TaskContract`와 `RunSnapshot`은 그대로 보존한다. 새 `internal/contract/v2`와 `internal/state/v2`가 immutable contract, revision, receipt를 소유하고, `internal/workflow`가 Publisher·runtime·Git adapter를 작은 interface 뒤에서 조합한다. CLI와 Wails Monitor는 동일한 aggregate snapshot wire만 소비한다.

**Tech Stack:** Go 1.27, Git, GitHub API/`gh`, Codex runtime, Wails v2, React, TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md`

## Global Constraints

- 1차 범위는 GitHub Projects·Issue·PR·Wiki와 기존 Wails Monitor이며 DXHub 메뉴·MCP와 공유 실행 제어는 후속이다.
- 첫 흐름은 Project·Repository·Work Item·runtime이 각각 하나이고 runtime은 Codex다.
- Contract v2에는 모델·provider 설정, 로컬 경로, token, 생성된 GitHub 번호를 넣지 않는다.
- 모든 쓰기 명령은 `expectedRevision`과 idempotency `requestId`를 받는다. 같은 requestId의 다른 payload는 거부한다.
- GitHub 쓰기는 Go Publisher만 수행하고 Builder·Reviewer·Documenter·Monitor는 직접 쓰지 않는다.
- Agent 종료 확인 전 commit·검증·통합을 시작하지 않는다. Agent 자체 검증은 authoritative evidence가 아니다.
- Task별 repair budget은 revision 안에서 누적 2회, 확실한 일시적 runtime 재시도는 1회다.
- main 병합은 사람이 GitHub의 Create a merge commit으로 수행한다.
- v2 state는 v1 `runs/`와 분리하며 v1 입력을 migration·재실행하지 않는다.
- Task 검증은 focused 범위만 실행하고 전체 `make check`는 통합 code PR의 마지막 gate에서 한 번만 실행한다.
- Sol medium은 계획·분배·fresh review를, Luna high는 제품 코드·테스트·수정을 소유한다.
- 공용 타입·interface는 Task 1에서 직렬로 고정하고, 이후 독립 Task만 별도 worktree·branch에서 최대 3개 Luna worker로 병렬화한다.

---

## File map and dependency order

| Task | Owns | Depends on | Parallel rule |
|---|---|---|---|
| 1 | `internal/contract/v2`, `internal/runtime`, aggregate wire | none | 반드시 단독 직렬 |
| 2 | `internal/registry`, `internal/state/v2`, workflow plan/approve | 1 | 반드시 직렬 |
| 3 | `internal/cli`, `cmd/agentctl` v2 routing | 2 | 반드시 직렬 |
| 4 | Issue/Projects Publisher | 2 | 3 이후 Task 5와 병렬 가능 |
| 5 | runtime/Git/task gate/review/integration | 2 | 3 이후 Task 4와 병렬 가능 |
| 6 | docs/final manifest | 4, 5 | 직렬 integration |
| 7 | PR/merge/Wiki/finalize | 6 | Task 8과 병렬 가능 |
| 8 | Wails/React Monitor | 3 | Task 7과 병렬 가능 |
| 9 | 단일 저장소 flow integration | 7, 8 | 최종 직렬 gate |

## Task 1: Contract v2, runtime envelope, monitor wire

**Files:**

- Create: `internal/contract/v2/types.go`
- Create: `internal/contract/v2/codec.go`
- Create: `internal/contract/v2/validate.go`
- Create: `internal/contract/v2/codec_test.go`
- Create: `internal/contract/v2/validate_test.go`
- Create: `internal/runtime/types.go`
- Create: `internal/runtime/runtime.go`
- Create: `internal/runtime/types_test.go`
- Create: `internal/monitor/types.go`
- Create: `internal/monitor/types_test.go`
- Create: `testdata/contracts/v2/valid-single-repo.json`

**Interfaces:**

- Consumes: `pathscope.Normalize/Overlaps`, `dag.Build`, RFC3339 timestamps.
- Produces:

```go
package contractv2

type ProjectID string
type WorkID string
type RepoKey string
type TaskID string
type RequestID string
type Revision uint64

type RepositoryIdentity struct {
	Host, Owner, Name, DefaultBranch string
}

type CommandSpec struct {
	Argv           []string `json:"argv,omitempty"`
	CwdRepoKey     RepoKey  `json:"cwdRepoKey"`
	TimeoutSeconds uint32   `json:"timeoutSeconds"`
	ShellScript    string    `json:"shellScript,omitempty"`
}

type WorkItemContract struct {
	Version int `json:"version"`
	WorkID WorkID `json:"workId"`
	ProjectID ProjectID `json:"projectId"`
	Revision Revision `json:"revision"`
	Request string `json:"request"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	RepositoryPlans []RepositoryPlan `json:"repositoryPlans"`
	Tasks []Task `json:"tasks"`
	InterfaceAgreements []InterfaceAgreement `json:"interfaceAgreements"`
	CrossRepoVerification []CommandSpec `json:"crossRepoVerification"`
	IssueDrafts []IssueDraft `json:"issueDrafts"`
	Documentation DocumentationPlan `json:"documentation"`
	ExecutionProfiles ExecutionProfiles `json:"executionProfiles"`
	DecisionRefs []string `json:"decisionRefs"`
}
```

```go
package runtime

type Role string
const (RoleBuilder Role = "builder"; RoleReviewer Role = "reviewer"; RoleDocumenter Role = "documenter")
type Invocation struct { RequestID contractv2.RequestID; Role Role; ProfileID, Worktree, OutputSchema string; ReadOnly bool; Packet json.RawMessage }
type ArtifactEnvelope struct { RequestID contractv2.RequestID; Role Role; Status string; Result json.RawMessage }
type AgentRuntime interface { Invoke(context.Context, Invocation) (ArtifactEnvelope, error) }
```

```go
package monitor

type SnapshotSource interface { FetchAll(context.Context) (Snapshot, error) }
type Snapshot struct { SchemaVersion int; Revision contractv2.Revision; ObservedAt time.Time; Freshness Freshness; Projects []Project }
```

- [ ] **Step 1: Write contract codec and validation tests first**

Add table tests that name these breaks: unknown/trailing JSON accepted, version other than 2 accepted, non-positive revision accepted, missing repoKey reference accepted, cyclic/missing Task dependency accepted, overlapping allowed paths accepted, both/neither argv and shell accepted, local path/model/token/generated Issue number leaked into the contract fixture.

- [ ] **Step 2: Verify RED**

Run:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/contract/v2
```

Expected: FAIL because the new package/types do not exist.

- [ ] **Step 3: Implement the minimal strict codec and validator**

Use `json.Decoder.DisallowUnknownFields`, reject trailing values, validate exact repository references and Task DAG, and reuse `pathscope` for repository-relative ownership. Keep all v1 files untouched.

- [ ] **Step 4: Write runtime and monitor wire tests, verify RED, then implement**

Tests must reject requestId/role mismatches and prove JSON keys are lower camel case; Monitor JSON must expose `schemaVersion`, `revision`, `state`, `syncStatus`, `nextAction`, `evidenceRefs` while omitting provider session/process/raw transcript identity.

- [ ] **Step 5: Verify GREEN and vet**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./internal/contract/v2 ./internal/runtime ./internal/monitor
docker run --rm -v "$PWD":/src -w /src golang:1.27 go vet ./internal/contract/v2 ./internal/runtime ./internal/monitor
```

- [ ] **Step 6: Commit**

```bash
git add internal/contract/v2 internal/runtime internal/monitor testdata/contracts/v2
git commit -m "feat: define project workflow v2 interfaces"
```

## Task 2: Registry, revisioned state, and plan/approve workflow

**Files:**

- Create: `internal/registry/types.go`, `internal/registry/store.go`, `internal/registry/store_test.go`
- Create: `internal/state/v2/types.go`, `internal/state/v2/store.go`, `internal/state/v2/store_test.go`
- Create: `internal/workflow/service.go`, `internal/workflow/service_test.go`

**Interfaces:**

- Consumes: Task 1 ID, contract, state, sync, next-action and evidence types.
- Produces:

```go
type WorkStore interface {
	Plan(context.Context, contractv2.WorkItemContract, string) (statev2.WorkSnapshot, error)
	Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error)
	Mutate(context.Context, statev2.Mutation) (statev2.WorkSnapshot, error)
}
type Mutation struct { WorkID contractv2.WorkID; ExpectedRevision contractv2.Revision; RequestID contractv2.RequestID; PayloadHash string; Transition Transition }
```

- [ ] Write failing tests for atomic project registration, immutable contract revision 1, approval to revision 2/`queued`, same request+payload replay, different payload conflict, stale revision rejection with current revision/state, and `unsupported_legacy` without modifying v1 files.
- [ ] Run RED with `go test ./internal/registry ./internal/state/v2 ./internal/workflow` in the Go 1.27 container.
- [ ] Implement 0700 directories, 0600 files, temp+fsync+rename, process lease, CAS and receipt-before-side-effect schema. State root is `<root>/v2`, never `<root>/runs`.
- [ ] Run focused tests and vet for the three packages, then commit `feat: add revisioned project workflow state`.

## Task 3: `project` and `work` CLI vertical slice

**Files:**

- Create: `internal/cli/project_work.go`, `internal/cli/project_work_test.go`
- Modify: `internal/cli/run.go`, `cmd/agentctl/main.go`, `cmd/agentctl/main_test.go`

**Interfaces:**

- Consumes: `workflow.Service` methods `RegisterProject`, `ListProjects`, `PlanWork`, `ApproveWork`, `Status`.
- Produces commands `project register`, `project list --json`, `project status --all --json`, `work plan`, `work approve`, `work status --json`.

- [ ] Write failing routing/wire tests proving malformed/read-only commands avoid v1 GHES/Herdr setup, aggregate status is one JSON document, and polling invokes neither scheduler nor GitHub refresh.
- [ ] Verify RED, implement a separate v2 dependency constructor, and preserve all v1 command behavior.
- [ ] Verify with focused test/vet for `./internal/cli ./cmd/agentctl ./internal/workflow`; commit `feat: expose project workflow commands`.

## Task 4: Issue and Projects Publisher

**Files:**

- Create: `internal/publisher/ports.go`, `internal/publisher/issues.go`, `internal/publisher/issues_test.go`
- Create: `internal/github/workflow_publisher.go`, `internal/github/workflow_publisher_test.go`

**Interfaces:**

- Consumes: approved contract revision and Task 2 pending/complete receipt.
- Produces `PublishIssues(context.Context, PublishIssuesRequest) (PublicationResult, error)`.

- [ ] Test first that marker lookup adopts interrupted writes, exact request replay returns the receipt, changed payload conflicts, manual body text is preserved, Project field IDs are exact, and sync failure does not change execution state.
- [ ] Implement only fake-backed Issue/Projects calls; no live write in automated tests.
- [ ] Run focused test/vet for publisher, GitHub and state/v2; commit `feat: publish approved work records idempotently`.

## Task 5: Codex invocation, scoped Git candidate, Task gate, review, integration

**Files:**

- Create: `internal/runtime/codex/client.go`, `internal/runtime/codex/client_test.go`
- Create: `internal/worktree/scoped_commit.go`, `internal/worktree/scoped_commit_test.go`
- Create: `internal/worktree/command.go`, `internal/worktree/command_test.go`
- Create: `internal/workflow/executor.go`, `internal/workflow/executor_test.go`
- Create: `internal/integration/workflow.go`, `internal/integration/workflow_test.go`

**Interfaces:**

- Consumes: Task 1 `AgentRuntime`, Task 2 store, approved Task packet.
- Produces `ExecuteNext`, `ApplyReview`, and candidate/integration evidence bound to exact SHA.

- [ ] Test fake runtime requestId/schema/read-only intent, timeout/cancel/ambiguous termination, no duplicate invocation, allowed-path-only staging including rename/symlink/untracked files, argv execution without implicit shell, and review accept/block with repair budget 2.
- [ ] Implement the Codex adapter using installed CLI help-confirmed flags, owner-only result files, secret environment filtering, and explicit termination confirmation.
- [ ] Reuse only narrow Git inspection/worktree operations; do not reuse `git add -A`, string `bash -lc` checks, or v1 `parallel.go` policy.
- [ ] Run focused test/vet for runtime/codex, workflow, worktree and integration; commit `feat: execute and review single repository tasks`.

## Task 6: Documenter, final checks, final review, manifest

**Files:**

- Create: `internal/finalization/manifest.go`, `internal/finalization/service.go`, `internal/finalization/service_test.go`
- Modify: `internal/workflow/service.go`, `internal/workflow/service_test.go`

**Interfaces:**

- Consumes: integration summary, repository docs patch, Wiki patch, exact HEAD/base.
- Produces immutable `FinalManifest{WorkID, Revision, RepositoryResults, WikiPatches, ReviewIdentity}`.

- [ ] Test first that docs changes invalidate earlier checks, final check/review bind to the docs-inclusive HEAD, changed HEAD/base or contract revision invalidates readiness, and manual cross-repo items remain pending.
- [ ] Implement documenter invocation and finalization state transitions without publishing.
- [ ] Run focused tests/vet for finalization, workflow and runtime; commit `feat: freeze reviewed final manifests`.

## Task 7: PR, merge relation, Wiki publication, completion

**Files:**

- Create: `internal/publisher/pr.go`, `internal/publisher/wiki.go`, `internal/publisher/finalize.go`
- Create: `internal/publisher/pr_test.go`, `internal/publisher/wiki_test.go`, `internal/publisher/finalize_test.go`
- Create: `internal/workflow/finalize.go`, `internal/workflow/finalize_test.go`

**Interfaces:**

- Consumes: Task 6 manifest and Publisher receipts.
- Produces PR receipt, verified merge relation, Wiki commit receipt, final `completed`/`publication_pending` state.

- [ ] Test first that PR HEAD/base changes invalidate readiness, merge commit parents/tree are verified, partial merge never completes, Wiki base conflicts never force-push, and Wiki failure retries publication without rerunning code Tasks.
- [ ] Implement fake GitHub/Wiki adapters first; live targets require separate explicit approval.
- [ ] Run focused test/vet for publisher, workflow and state/v2; commit `feat: finalize published project work`.

## Task 8: Minimal Wails Monitor list and detail

**Files:**

- Create: `internal/monitorcli/client.go`, `internal/monitorcli/client_test.go`
- Create: `monitor/wails.json`, `monitor/main.go`, `monitor/app.go`, `monitor/app_test.go`
- Create: `monitor/frontend/package.json`, TypeScript/Vite/Vitest configuration and `src/` list/detail files.
- Modify: `Makefile`

**Interfaces:**

- Consumes: Task 1 `monitor.Snapshot` and Task 3 `agentctl project status --all --json`.
- Produces one Wails `GetMonitorSnapshot()` binding and React list/detail with links.

- [ ] Test the exact `wsl.exe --exec agentctl project status --all --json` argv, timeout/nonzero/malformed schema, last-good snapshot with stale/offline status, needs-operator ordering, project selection, detail fields and evidence links.
- [ ] Implement one 3–5 second aggregate poll; do not read state files, invoke GitHub, advance scheduler, or expose process/session/raw transcript identifiers.
- [ ] Run Go focused tests, frontend Vitest/build, and Windows Wails smoke when the host tools are available; record unavailable checks as `unverified`.
- [ ] Commit `feat: add project workflow monitor`.

## Task 9: Single-repository integration gate and handoff

**Files:**

- Create: `internal/workflow/single_repository_integration_test.go`
- Modify: `HANDOFF.md` only with verified results and blockers.

**Interfaces:**

- Consumes: Tasks 1–8.
- Produces one fake-backed request→approval→Issue→build→review→docs→manifest→PR→merge→Wiki→completed scenario and Monitor snapshot.

- [ ] Write the integration test before any missing glue and verify it fails at the first absent transition.
- [ ] Add only the minimal glue through the existing interfaces; keep external writes fake unless a live target is separately approved.
- [ ] Run focused integration tests and affected frontend checks.
- [ ] Run the repository-wide final gate exactly once:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 make check
```

- [ ] Record exact SHA/commands/outcomes, fresh final Sol review, live GitHub/Wiki/Wails checks as passed/failed/unverified, and commit `docs: hand off single repository workflow`.

