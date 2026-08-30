# ThreadDock Parallel Execution and Supervised Auto-Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the single-run Orchestrator to two independent Builders, bounded repair and recovery, a deterministic Merge Gate and supervised automatic main merge.

**Architecture:** A contract-independent ID graph feeds a deterministic two-slot scheduler while task-indexed durable state preserves legacy Single-run fields. Builder output remains the existing strict Evidence envelope; Git inspection derives changed files and a bounded patch before immutable commits enter one integration branch. A pure recovery policy and Merge Gate produce auditable decisions, while orchestration owns side effects and reconciliation across Herdr, GitHub.com/GHES, and Git.

**Tech Stack:** Go 1.27.0, Git Worktree, Herdr v0.8.2, OpenCode, GHES checks and Pull Request REST APIs.

**Spec:** `../../../gitops-agent-system-design.md`

**Work records:** Parent #42; recovery #43; DAG/state #44; final orchestration/pilot #45; Merge Gate #46; integration/repair #47.

## Global Constraints

- Complete `2026-08-28-threaddock-single-run.md` first.
- Keep one active Parent Issue and exactly one integration branch.
- Run at most two Builders globally.
- Do not parallelize shared schema, migration, authentication, deployment or public-contract changes.
- Reviewer and CI failures share one repair counter with maximum `2`.
- Recovery attempts have a separate maximum `3` and reset only when the durable progress fingerprint changes: commit SHA, working-tree content hash, completed Task IDs, or verification evidence.
- Never auto-resolve Git conflicts, auto-answer blocked prompts or force-remove dirty Worktrees.
- Preserve the strict Evidence JSON schema. Changed files and patches come only from Git inspection, never Agent prose.
- Prefer provider session identity; treat `herdr-terminal:<terminal_id>` only as an execution identity fallback, not proof that native session resume is possible.
- Use the configured normalized API base for both GitHub.com and GHES; endpoint methods never hard-code `/api/v3` above the REST adapter.
- Keep existing Single-run snapshots decodable and their state-machine behavior unchanged.
- Branch protection remains the final enforcement point.

---

## File map

```text
internal/pathscope/scope.go                  # normalized path ownership rules
internal/pathscope/scope_test.go
internal/dag/graph.go                         # contract-independent dependency graph
internal/dag/graph_test.go
internal/scheduler/scheduler.go               # deterministic two-slot dispatch
internal/scheduler/scheduler_test.go
internal/state/types.go                       # task-indexed durable state, legacy fields retained
internal/state/store_test.go
internal/integration/integrator.go            # merge and final validation
internal/integration/integrator_test.go
internal/review/loop.go                        # shared repair budget
internal/review/loop_test.go
internal/recovery/supervisor.go                # slow-model recovery policy
internal/recovery/supervisor_test.go
internal/mergegate/gate.go                     # pure merge decision
internal/mergegate/gate_test.go
internal/github/rest.go                        # checks, comments, ready, and merge endpoints
internal/github/rest_test.go
internal/revert/service.go                    # safe Git revert branch + draft PR composition
internal/revert/service_test.go
internal/orchestrator/parallel.go               # complete multi-Builder transitions
internal/orchestrator/parallel_test.go
internal/orchestrator/parallel_fakes_test.go
internal/cli/confirm.go                         # Protected Change confirmation
internal/cli/confirm_test.go
docs/operator/parallel-pilot.md
```

## Execution order

Execute Task 5 first because Issue #43 closes the live-pilot recovery gap without changing shared contract/state schemas. Then execute Tasks 1, 2, 3, 4, 6, and 7 in that order. Run Tasks 1 and 5 in separate Builder Worktrees only when their branches do not both modify a shared file; merge and review each result before Task 2 changes durable state.

### Task 1: Normalize path ownership and build a contract-independent DAG

**Files:**
- Create: `internal/pathscope/scope.go`
- Create: `internal/pathscope/scope_test.go`
- Create: `internal/dag/graph.go`
- Create: `internal/dag/graph_test.go`
- Modify: `internal/contract/validate.go`
- Modify: `internal/contract/validate_test.go`

**Interfaces:**
- Produces: `pathscope.Normalize(pattern string) (string, error)`
- Produces: `pathscope.Overlaps(left, right string) (bool, error)` and `pathscope.Contains(pattern, path string) bool`
- Produces: `dag.Build(nodes []dag.Node) (dag.Graph, []dag.Violation)` where `Node` contains only `ID string` and `DependsOn []string`
- Produces: `Graph.Ready(completed map[string]bool) []string` in original contract order
- Adds optional `TaskContract.RiskCategories []string` with allowed values `data`, `authentication`, `authorization`, `deployment`, `supply_chain`, and `public_contract`; absence preserves version-1 Single-run compatibility.
- Constraint: `internal/dag` does not import `internal/contract`; `contract.Validate` maps DAG/pathscope violations to the existing public `contract.Violation` codes.

- [ ] **Step 1: Write failing normalization and graph tests**

```go
func TestNormalizeRejectsParentTraversal(t *testing.T) {
    if _, err := Normalize(`src/../authentication/**`); err == nil { t.Fatal("expected traversal rejection") }
}

func TestReadyReturnsSatisfiedIDsInContractOrder(t *testing.T) {
    graph, violations := Build([]Node{{ID:"tests",DependsOn:[]string{"api"}},{ID:"api"}})
    if len(violations) != 0 { t.Fatal(violations) }
    if got := graph.Ready(nil); !reflect.DeepEqual(got, []string{"api"}) { t.Fatalf("ready=%v", got) }
    if got := graph.Ready(map[string]bool{"api":true}); !reflect.DeepEqual(got, []string{"tests"}) { t.Fatalf("ready=%v", got) }
}

func TestBuildReportsMissingDependencyAndCycle(t *testing.T) {
    _, got := Build([]Node{{ID:"a",DependsOn:[]string{"b"}},{ID:"b",DependsOn:[]string{"a","missing"}}})
    if !hasCodes(got, "dependency_cycle", "missing_dependency") { t.Fatal(got) }
}
```

- [ ] **Step 2: Run tests and verify the new packages are absent**

Run: `go test ./internal/pathscope ./internal/dag -v`

Expected: FAIL because the packages do not exist.

- [ ] **Step 3: Implement normalized scopes and the ID graph**

`Normalize` converts `\` to `/`, removes `.` segments, preserves a terminal `/**`, rejects empty and `..` segments, and returns a repository-relative pattern. `Overlaps` compares normalized static directory prefixes. `Contains` matches an inspected repository-relative file against a normalized scope. `dag.Build` uses Kahn's algorithm only for dependency validity and stores original order for `Ready`.

- [ ] **Step 4: Make contract validation delegate without changing compatibility codes**

Map a DAG cycle to the existing `missing_dependency` code and path overlap/protected overlap to the existing `path_overlap` code. Reject unknown or duplicate risk categories. Add regression tests for Windows separators, `.` cleanup, traversal rejection, optional risk categories, and the current valid/invalid fixtures.

Run: `go test ./internal/pathscope ./internal/dag ./internal/contract -v`

Expected: PASS with no import cycle and unchanged fixture outcomes.

- [ ] **Step 5: Commit the graph seam**

```bash
git add internal/pathscope internal/dag internal/contract/validate.go internal/contract/validate_test.go
git commit -m "feat: 병렬 Task DAG와 경로 소유권 검증"
```

### Task 2: Schedule at most two Builders deterministically

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`
- Modify: `internal/state/types.go`
- Modify: `internal/state/store_test.go`

**Interfaces:**
- Consumes: `dag.Graph`, current task execution states
- Produces: `scheduler.Next(graph dag.Graph, states map[string]scheduler.TaskState, limit int) []string`
- Produces: `state.TaskRunState` with `State`, `Agent`, `Worktree`, `Prompt`, `ProgressFingerprint`, `LastProgressAt`, and `RecoveryCount`
- Adds: `RunSnapshot.Tasks map[string]TaskRunState` while retaining `Builder`, `Reviewer`, `BuilderWorktree`, `ReviewerWorktree`, and their prompt fields for old Single-run JSON.

- [ ] **Step 1: Write failing capacity and dependency tests**

```go
func TestNextNeverExceedsTwo(t *testing.T) {
    graph, violations := dag.Build([]dag.Node{{ID:"a"},{ID:"b"},{ID:"c"}})
    if len(violations) != 0 { t.Fatal(violations) }
    got := Next(graph, map[string]TaskState{}, 2)
    if len(got) != 2 { t.Fatalf("got=%d", len(got)) }
}

func TestNextDoesNotDispatchDependentTask(t *testing.T) {
    graph, violations := dag.Build([]dag.Node{{ID:"api"},{ID:"tests",DependsOn:[]string{"api"}}})
    if len(violations) != 0 { t.Fatal(violations) }
    got := Next(graph, map[string]TaskState{"api": Running}, 2)
    if len(got) != 0 { t.Fatalf("got=%v", got) }
}

func TestLegacySingleRunSnapshotStillDecodes(t *testing.T) {
    var got state.RunSnapshot
    if err := json.Unmarshal([]byte(`{"runId":"run-1","builder":{"name":"builder-run-1"}}`), &got); err != nil { t.Fatal(err) }
    if got.Builder.Name != "builder-run-1" || got.Tasks != nil { t.Fatalf("snapshot=%#v", got) }
}
```

- [ ] **Step 2: Run and verify scheduler absence**

Run: `go test ./internal/scheduler -v`

Expected: FAIL.

- [ ] **Step 3: Implement pure scheduling without goroutines**

`Next` computes desired task IDs only. The Orchestrator resolves IDs to `contract.Task`, persists a `task_dispatch_planned` event, and then starts processes. The scheduler contains no goroutines and does not mutate state.

- [ ] **Step 4: Cover completion, failure and paused states**

Only `Pending` Tasks can dispatch. `Completed` satisfies dependencies. `Running` and `Repairing` consume capacity; `Paused`, `Failed`, and `Blocked` do not dispatch and never create duplicates. Persist task maps with deterministic JSON output and prove a legacy Single-run fixture still decodes.

Run: `go test ./internal/scheduler ./internal/state -v`

Expected: PASS.

- [ ] **Step 5: Commit the scheduler**

```bash
git add internal/scheduler internal/state/types.go internal/state/store_test.go
git commit -m "feat: 두 슬롯 Builder scheduler 추가"
```

### Task 3: Integrate Builder commits and reject unsafe results

**Files:**
- Create: `internal/integration/integrator.go`
- Create: `internal/integration/integrator_test.go`
- Modify: `internal/worktree/git.go`
- Modify: `internal/worktree/git_test.go`

**Interfaces:**
- Consumes: existing strict `state.AgentEvidence` plus Git-derived `worktree.CommitInspection`
- Produces: `integration.ValidateResult(task contract.Task, evidence state.AgentEvidence, inspection worktree.CommitInspection) error`
- Produces: `Integrator.MergeResults(ctx context.Context, integrationPath string, results []integration.Result, checks []string) (integration.IntegrationResult, error)`
- `integration.Result` contains `TaskID string` and `CommitSHA string`; `integration.Check` contains `Command`, `Outcome`, `Duration`, and `ExitCode`; `IntegrationResult` contains final `CommitSHA` and normalized `[]Check`.
- Adds to `worktree.Git`: `MergeCommitNoFF`, `AbortMerge`, `RunChecks`, and `Fingerprint`; these validate explicit managed Worktree paths.

- [ ] **Step 1: Write failing result-contract tests**

```go
func TestValidateResultRejectsChangedPathOutsideOwnership(t *testing.T) {
    owned := contract.Task{ID:"api",AllowedPaths:[]string{"src/payments/**"}}
    evidence := state.AgentEvidence{CommitSHA:"0123456789abcdef0123456789abcdef01234567",VerificationEvidence:[]state.VerificationEvidence{{Command:"go test ./...",Outcome:"passed",Duration:"1s"}}}
    inspection := worktree.CommitInspection{CommitSHA:evidence.CommitSHA,Branch:"agent/api",ChangedFiles:[]string{"src/auth/token.go"},Patch:"bounded"}
    err := ValidateResult(owned, evidence, inspection)
    if !errors.Is(err, ErrPathOwnership) { t.Fatalf("err=%v", err) }
}

func TestMergeResultsStopsOnConflict(t *testing.T) {
    integrator := New(fakeGit{mergeErr: worktree.ErrConflict})
    results := []Result{{TaskID:"api",CommitSHA:"0123456789abcdef0123456789abcdef01234567"},{TaskID:"tests",CommitSHA:"89abcdef0123456789abcdef0123456789abcdef"}}
    _, err := integrator.MergeResults(context.Background(), "/work/integration", results, nil)
    if !errors.Is(err, ErrBlockedConflict) { t.Fatalf("err=%v", err) }
}

type fakeGit struct { mergeErr error }
func (f fakeGit) MergeCommitNoFF(context.Context,string,string) error { return f.mergeErr }
func (f fakeGit) AbortMerge(context.Context,string) error { return nil }
func (f fakeGit) RunChecks(context.Context,string,[]string) ([]Check,error) { return []Check{{Command:"go test ./...",Passed:true}},nil }
```

- [ ] **Step 2: Run and observe missing integration package**

Run: `go test ./internal/integration -v`

Expected: FAIL.

- [ ] **Step 3: Implement evidence validation**

Require a 40-character commit SHA, exact required verification commands with passed outcome and parseable duration, and a matching Git inspection SHA/branch. Validate every Git-derived changed file with `pathscope.Contains`; the Evidence schema remains unchanged and never contains reported changed files.

- [ ] **Step 4: Merge in contract order and rerun full verification**

Run `git merge --no-ff --no-edit COMMIT` for each immutable SHA in contract order. On conflict, run `git merge --abort` and return `ErrBlockedConflict`; do not reset. After all merges, execute global verification commands and return command, outcome, duration, and exit code without storing raw stdout. `Fingerprint` hashes the current commit plus status paths and content object IDs, returning only the digest to orchestration.

Run: `go test ./internal/integration ./internal/worktree -v`

Expected: PASS.

- [ ] **Step 5: Commit integration checks**

```bash
git add internal/integration internal/worktree/git.go internal/worktree/git_test.go
git commit -m "feat: Builder 결과 검증과 통합 추가"
```

### Task 4: Share a two-round repair budget across Reviewer and CI

**Files:**
- Create: `internal/review/loop.go`
- Create: `internal/review/loop_test.go`
- Modify: `internal/herdr/client.go`
- Modify: `internal/herdr/cli.go`
- Modify: `internal/herdr/cli_test.go`

**Interfaces:**
- Adds strict `herdr.ReviewEvidence` with `RequestID`, `Decision`, `BlockingFindings`, and `RiskCategories`, parsed only between `THREADDOCK_REVIEW_BEGIN` and `THREADDOCK_REVIEW_END` markers with the same size, unknown-field, request-ID, and credential guards as Builder Evidence.
- `ReviewResult` contains `Source string`, `Blocking bool`, `Findings []Finding`, and `RiskCategories []string`; `Finding` contains `ID`, `Summary`, and repository-relative `Paths`.
- Produces: `review.Decide(snapshot state.RunSnapshot, result ReviewResult) Decision`
- Produces decisions: `Accept`, `Repair`, `Block`

- [ ] **Step 1: Write failing shared-budget tests**

```go
func TestReviewerThenCIFailureConsumesTwoTotalRounds(t *testing.T) {
    s := state.RunSnapshot{RepairCount:0}
    first := Decide(s, ReviewResult{Source:"reviewer", Blocking:true})
    if first.Kind != Repair || first.NextCount != 1 { t.Fatal(first) }
    s.RepairCount = first.NextCount
    second := Decide(s, ReviewResult{Source:"ci", Blocking:true})
    if second.Kind != Repair || second.NextCount != 2 { t.Fatal(second) }
    s.RepairCount = second.NextCount
    third := Decide(s, ReviewResult{Source:"ci", Blocking:true})
    if third.Kind != Block { t.Fatal(third) }
}
```

- [ ] **Step 2: Run and observe missing decision logic**

Run: `go test ./internal/review -v`

Expected: FAIL.

- [ ] **Step 3: Implement the pure budget decision**

An accepted result never changes the counter. A blocking result returns `Repair` when count is below `2`; at `2` it returns `Block`. Source must be exactly `reviewer` or `ci`.

- [ ] **Step 4: Build repair Task packets from blocking findings**

The repair packet contains original acceptance criteria, current integration commit, numbered blocking findings, allowed paths and the remaining budget. It must not contain recommendations classified as non-blocking. The Reviewer parser rejects `accept` with blocking findings, `block` without findings, stale request IDs, unknown risk categories, oversized envelopes, credentials, and raw-text-only results.

Run: `go test ./internal/review -v`

Expected: PASS.

- [ ] **Step 5: Commit repair budgeting**

```bash
git add internal/review internal/herdr/client.go internal/herdr/cli.go internal/herdr/cli_test.go
git commit -m "feat: 리뷰와 CI 공유 수정 예산 추가"
```

### Task 5: Implement tolerant slow-model recovery

**Files:**
- Create: `internal/recovery/supervisor.go`
- Create: `internal/recovery/supervisor_test.go`

**Interfaces:**
- Produces: `recovery.Decide(now time.Time, policy Policy, agent AgentSnapshot) Decision`
- `Policy` contains `WorkingWait time.Duration` and `Limit int`.
- `AgentSnapshot` contains `State string`, `Alive bool`, `Complete bool`, `LastProgress time.Time`, `ProgressFingerprint string`, `PreviousFingerprint string`, `RecoveryCount int`, and `CanNativeResume bool`.
- Produces: `Wait`, `Continue`, `ResumeSession`, `AskOperator`, `Block`

- [ ] **Step 1: Write failing policy tests with a fake clock**

```go
func TestWorkingAgentWaitsForSixtyMinutes(t *testing.T) {
    base := time.Date(2026,8,28,0,0,0,0,time.UTC)
    d := Decide(base.Add(59*time.Minute), Policy{WorkingWait:time.Hour,Limit:3}, AgentSnapshot{State:"working",LastProgress:base,Alive:true})
    if d.Kind != Wait { t.Fatal(d) }
}

func TestIncompleteIdleGetsContinueInstruction(t *testing.T) {
    base := time.Date(2026,8,28,0,0,0,0,time.UTC)
    d := Decide(base.Add(10*time.Minute), Policy{WorkingWait:time.Hour,Limit:3}, AgentSnapshot{State:"idle",Alive:true,RecoveryCount:0})
    if d.Kind != Continue || d.NextCount != 1 || d.Instruction != ContinuationInstruction { t.Fatal(d) }
}

func TestBlockedNeverReceivesAutomaticInput(t *testing.T) {
    d := Decide(time.Now(), Policy{WorkingWait:time.Hour,Limit:3}, AgentSnapshot{State:"blocked",Alive:true})
    if d.Kind != AskOperator { t.Fatal(d) }
}

func TestTerminalFallbackCannotClaimNativeResume(t *testing.T) {
    d := Decide(time.Now(), Policy{WorkingWait:time.Hour,Limit:3}, AgentSnapshot{Alive:false,CanNativeResume:false})
    if d.Kind != AskOperator || d.NextCount != 0 { t.Fatal(d) }
}

func TestProgressResetAndThirdConsecutiveAttemptBlocks(t *testing.T) {
    policy := Policy{WorkingWait:time.Hour,Limit:3}
    progressed := Decide(time.Now(),policy,AgentSnapshot{State:"idle",Alive:true,RecoveryCount:2,PreviousFingerprint:"old",ProgressFingerprint:"new"})
    if progressed.Kind != Wait || progressed.NextCount != 0 { t.Fatal(progressed) }
    blocked := Decide(time.Now(),policy,AgentSnapshot{State:"done",Alive:true,RecoveryCount:3,PreviousFingerprint:"same",ProgressFingerprint:"same"})
    if blocked.Kind != Block { t.Fatal(blocked) }
}
```

- [ ] **Step 2: Run and observe missing supervisor**

Run: `go test ./internal/recovery -v`

Expected: FAIL.

- [ ] **Step 3: Implement progress and exclusion rules**

Orchestration constructs the progress fingerprint from the immutable commit SHA, `worktree.Fingerprint`, sorted completed Task IDs, and normalized verification evidence. A changed non-empty fingerprint resets `RecoveryCount` to zero. Incomplete `idle/done` returns `Continue`. A live `working` Agent waits for `WorkingWait`; after that it returns `AskOperator` because Herdr v0.8.2 exposes no safe foreground-command or duplicate-process exclusion signal.

- [ ] **Step 4: Cap recovery and preserve evidence**

When the Agent is absent, return `ResumeSession` only if orchestration proved a provider session identity; `herdr-terminal:*` sets `CanNativeResume=false` and returns `AskOperator`. After three consecutive `Continue` or `ResumeSession` decisions without fingerprint progress, return `Block`. Never stop the Herdr server, delete a pane, replace a live process, or remove a Worktree. The continuation text is exactly `Task packet과 현재 변경을 다시 확인하고, 완료되지 않은 수용 조건부터 계속 진행하세요. 이미 완료한 작업은 반복하지 마세요.`

Run: `go test ./internal/recovery -v`

Expected: PASS.

- [ ] **Step 5: Commit the recovery supervisor**

```bash
git add internal/recovery
git commit -m "feat: 느린 모델 자동 복구 정책 추가"
```

### Task 6: Implement the pure Merge Gate and Protected Change check

**Files:**
- Create: `internal/mergegate/gate.go`
- Create: `internal/mergegate/gate_test.go`
- Modify: `internal/github/client.go`
- Modify: `internal/github/rest.go`
- Modify: `internal/github/rest_test.go`
- Create: `internal/revert/service.go`
- Create: `internal/revert/service_test.go`

**Interfaces:**
- Produces: `mergegate.Evaluate(Input) Decision`
- Adds narrow optional ports in `internal/github/client.go`: `CheckReader.GetChecks(ctx, repo, sha)`, `IssueCommenter.CreateIssueComment(ctx, repo, number, body)`, `PullRequestReadier.MarkReadyForReview(ctx, repo, number)`, and `PullRequestMerger.MergePullRequest(ctx, repo, number, exactSHA, method)`. `github.Client` keeps its current methods so existing Single-run fakes continue to compile.
- Extends `github.PullRequest` with exact `HeadSHA string` and nullable `Mergeable *bool`; a null mergeability result evaluates to `Wait`, never `Merge`.
- Produces: `revert.Service.Create(ctx, Request) (github.PullRequest, error)` by composing narrow Git and GitHub ports; the REST adapter never runs Git.
- Decisions: `Wait`, `NeedsOperator`, `Merge`, `Block`

```go
type Input struct {
    AcceptanceMet    bool
    BuildersComplete bool
    ReviewerApproved bool
    Checks           []CheckState
    LatestMainTested bool
    MergeabilityKnown bool
    Mergeable         bool
    ProtectedReasons []string
    OperatorConfirmed bool
}
type CheckState struct { Name, State string }
type Decision struct { Kind Kind; Reasons []string }
```

- [ ] **Step 1: Write a failing decision table**

```go
func TestGateDecisionTable(t *testing.T) {
    cases := []struct{name string; in Input; want Kind}{
        {"ordinary", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,MergeabilityKnown:true,Mergeable:true}, Merge},
        {"protected", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,MergeabilityKnown:true,Mergeable:true,ProtectedReasons:[]string{"authentication/**"}}, NeedsOperator},
        {"check pending", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"pending"}},LatestMainTested:true,MergeabilityKnown:true,Mergeable:true}, Wait},
        {"checks absent", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,LatestMainTested:true,MergeabilityKnown:true,Mergeable:true}, Wait},
        {"mergeability pending", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true}, Wait},
        {"review blocked", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:false,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,MergeabilityKnown:true,Mergeable:true}, Block},
        {"conflict", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,MergeabilityKnown:true,Mergeable:false}, Block},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { if got:=Evaluate(tc.in); got.Kind!=tc.want { t.Fatalf("got=%v", got) } }) }
}
```

- [ ] **Step 2: Run and observe missing gate**

Run: `go test ./internal/mergegate -v`

Expected: FAIL.

- [ ] **Step 3: Implement Protected Change classification**

Mark protected when changed files match `migrations/**`, `authentication/**`, `.github/workflows/**`, `deployment/**`, when the contract explicitly marks data/public contract/supply-chain risk, or when the Reviewer reports one of those categories. Persist all reasons in the decision.

- [ ] **Step 4: Implement GitHub.com/GHES checks and merge endpoints**

Before gate evaluation, fetch latest main, merge it into the integration branch and rerun all global verification commands. A conflict returns `Block`; a changed integration SHA invalidates earlier checks and requires fresh CI. Read checks and PR mergeability only for that final SHA. REST paths use `c.restBasePath`, so GitHub.com calls `/repos/...` and GHES calls `/api/v3/repos/...`; endpoint tests assert both profiles. After Reviewer acceptance, mark the Draft PR ready for review; call the merge endpoint only for `Merge`, with method `merge` and the exact final SHA. A `NeedsOperator` decision writes a PR comment and waits for `agentctl confirm RUN protected-change`.

`revert.Service` creates `revert/<parent>-<merge-sha-prefix>` in a validated managed Worktree, runs `git revert --no-edit MERGE_SHA`, pushes the explicit branch, and calls the existing Draft PR API with a Parent link. A revert conflict aborts the revert and returns `Blocked` without reset, force-push, or main mutation.

Run: `go test ./internal/mergegate ./internal/github ./internal/revert -v`

Expected: PASS.

- [ ] **Step 5: Commit Merge Gate behavior**

```bash
git add internal/mergegate internal/github internal/revert
git commit -m "feat: 보호 변경을 포함한 병합 게이트 추가"
```

### Task 7: Complete the parallel Orchestrator and pilot drills

**Files:**
- Create: `internal/orchestrator/parallel.go`
- Create: `internal/orchestrator/parallel_test.go`
- Create: `internal/orchestrator/parallel_fakes_test.go`
- Modify: `internal/orchestrator/agent_name_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/cli/confirm.go`
- Create: `internal/cli/confirm_test.go`
- Create: `internal/cli/revert.go`
- Create: `internal/cli/revert_test.go`
- Modify: `internal/cli/run.go`
- Create: `docs/operator/parallel-pilot.md`

**Interfaces:**
- Consumes: all previous Tasks in this plan
- Produces: full phases through `completed` or `blocked`
- Produces: `taskAgentName(role string, runID contract.RunID, taskID string) string`, stable and Herdr-compatible within 32 characters
- Adds: explicit `projectAutomationEnabled` config, default `false`; existing Project ID fields remain structurally required for config compatibility, but ProjectV2 mutations occur only when the flag is enabled.
- Produces CLI: `agentctl confirm RUN protected-change`
- Produces CLI: `agentctl create-revert RUN --reason TEXT`

- [ ] **Step 1: Write the failing complete-story tests**

Implement a table test over a `parallelHarness` with in-memory ports:

```go
func TestParallelStories(t *testing.T) {
    cases := []struct{name string; configure func(*parallelHarness); want contract.RunPhase}{
        {"ordinary auto merge", func(h *parallelHarness){}, contract.PhaseCompleted},
        {"third repair blocks", func(h *parallelHarness){ h.reviewFailures=1; h.ciFailures=2 }, contract.PhaseBlocked},
        {"protected waits", func(h *parallelHarness){ h.changedFiles=[]string{"authentication/policy.go"} }, contract.PhaseNeedsOperator},
        {"git conflict blocks", func(h *parallelHarness){ h.mergeConflict=true }, contract.PhaseBlocked},
        {"three recoveries block", func(h *parallelHarness){ h.stallRecoveries=3 }, contract.PhaseBlocked},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { h:=newParallelHarness(t); tc.configure(h); got:=h.runToStable(); if got!=tc.want { t.Fatalf("phase=%s",got) } }) }
}
```

Define the harness in `parallel_fakes_test.go`, reusing the tested fake ports from the single-run plan:

```go
type parallelHarness struct {
    *harness
    reviewFailures int
    ciFailures int
    changedFiles []string
    mergeConflict bool
    stallRecoveries int
}

func newParallelHarness(t *testing.T) *parallelHarness {
    t.Helper()
    base := newHarness(t)
    base.orchestrator = NewParallel(base.Deps)
    return &parallelHarness{harness:base,changedFiles:[]string{"src/payments/retry.go","tests/payments/retry_test.go"}}
}

func (h *parallelHarness) runToStable() contract.RunPhase {
    h.herdr.reviewFailures = h.reviewFailures
    h.herdr.ciFailures = h.ciFailures
    h.herdr.stallRecoveries = h.stallRecoveries
    h.git.changedFiles = append([]string(nil),h.changedFiles...)
    h.git.mergeConflict = h.mergeConflict
    id, err := h.orchestrator.Start(context.Background(),h.contractPath); if err!=nil { h.t.Fatal(err) }
    for i:=0;i<60;i++ { if err:=h.orchestrator.Advance(context.Background(),id);err!=nil { h.t.Fatal(err) }; snapshot,_:=h.store.Load(context.Background(),id); if snapshot.Phase==contract.PhaseCompleted||snapshot.Phase==contract.PhaseBlocked||snapshot.Phase==contract.PhaseNeedsOperator{return snapshot.Phase} }
    h.t.Fatal("60 transitions 안에 안정 상태에 도달하지 못함"); return ""
}
```

The existing base `harness` already contains `t`, `Deps`, and lowercase adapter fields. Add only the failure controls used above. Each fake consumes one configured failure per call and otherwise returns deterministic strict evidence. Add naming tests proving two task IDs in one long RUN produce stable distinct names and that identity reconciliation still compares session/terminal identity plus workspace, pane, and path.

- [ ] **Step 2: Run and observe incomplete phase transitions**

Run: `go test ./internal/orchestrator ./internal/cli -run 'TestParallel|TestProtected|TestRepair|TestRecovery' -v`

Expected: FAIL.

- [ ] **Step 3: Implement one-action-per-advance orchestration**

Persist an intent event before every Issue write, enabled Project status update, Agent start, Git merge, Reviewer prompt, CI observation and main merge. After restart, reconcile the intended action with GitHub, Git and Herdr before retrying it. Set Organization Project states `Ready` after Issue approval, `In Progress` after dispatch, `Review` after final PR creation and `Done` after main merge only when `projectAutomationEnabled=true`; otherwise append an audit event that Project automation was skipped and never treat placeholder IDs as evidence.

The successful transition order is: validate contract and initialize task state; dispatch up to two ready task IDs; collect strict Builder Evidence; inspect and merge immutable SHAs in contract order; run Wave End Verification; collect strict Review Evidence; apply a bounded Reviewer repair when needed; push the integration branch and create one Draft PR; merge latest main and rerun Full Suite; push the resulting final SHA and mark the PR ready; observe checks for that exact SHA; apply a bounded CI repair when needed; re-read main before gate evaluation and repeat latest-main integration, Full Suite, push, and fresh checks if main moved; evaluate Merge Gate; merge ordinary changes or wait for Protected Change confirmation. Each `Advance` performs at most one external action.

- [ ] **Step 4: Run deterministic E2E tests and real pilot drills**

Run: `go test -race ./...`

Then use a disposable repository to execute the six stories documented in `parallel-pilot.md`. Record Issue/PR links, Run IDs, commits, repair/recovery counts and whether manual cleanup was needed.

Also run `agentctl create-revert RUN --reason "pilot regression"` against the ordinary merged story and verify a draft Revert PR is created while main remains unchanged.

Expected: tests pass; ordinary story reaches `completed`; failure stories reach the exact expected `blocked` or `needs_operator` state.

- [ ] **Step 5: Commit the parallel increment**

```bash
git add internal/orchestrator internal/config internal/cli docs/operator/parallel-pilot.md
git commit -m "feat: 병렬 실행과 감독형 자동 병합 완성"
```
