# ThreadDock Parallel Execution and Supervised Auto-Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the single-run Orchestrator to two independent Builders, bounded repair and recovery, a deterministic Merge Gate and supervised automatic main merge.

**Architecture:** A validated Task DAG feeds a two-slot scheduler. Builder results merge into one integration branch; the Reviewer and CI consume a shared two-round repair budget. A pure Merge Gate produces an auditable decision before the GHES adapter performs an ordinary merge or waits for Protected Change confirmation.

**Tech Stack:** Go 1.27.0, Git Worktree, Herdr v0.8.2, OpenCode, GHES checks and Pull Request REST APIs.

**Spec:** `../../../gitops-agent-system-design.md`

## Global Constraints

- Complete `2026-08-28-threaddock-single-run.md` first.
- Keep one active Parent Issue and exactly one integration branch.
- Run at most two Builders globally.
- Do not parallelize shared schema, migration, authentication, deployment or public-contract changes.
- Reviewer and CI failures share one repair counter with maximum `2`.
- Recovery attempts have a separate maximum `3` and reset only on measurable progress.
- Never auto-resolve Git conflicts, auto-answer blocked prompts or force-remove dirty Worktrees.
- Branch protection remains the final enforcement point.

---

## File map

```text
internal/dag/graph.go                         # dependency and ownership validation
internal/dag/graph_test.go
internal/scheduler/scheduler.go               # deterministic two-slot dispatch
internal/scheduler/scheduler_test.go
internal/integration/integrator.go            # merge and final validation
internal/integration/integrator_test.go
internal/review/loop.go                        # shared repair budget
internal/review/loop_test.go
internal/recovery/supervisor.go                # slow-model recovery policy
internal/recovery/supervisor_test.go
internal/mergegate/gate.go                     # pure merge decision
internal/mergegate/gate_test.go
internal/github/rest.go                        # checks, merge, revert PR endpoints
internal/github/rest_test.go
internal/orchestrator/parallel.go               # complete multi-Builder transitions
internal/orchestrator/parallel_test.go
internal/orchestrator/parallel_fakes_test.go
internal/cli/confirm.go                         # Protected Change confirmation
internal/cli/confirm_test.go
docs/operator/parallel-pilot.md
```

### Task 1: Validate the Task DAG and path ownership

**Files:**
- Create: `internal/dag/graph.go`
- Create: `internal/dag/graph_test.go`
- Modify: `internal/contract/validate.go`

**Interfaces:**
- Consumes: `[]contract.Task`, contract protected paths
- Produces: `dag.Build(tasks []contract.Task) (Graph, []contract.Violation)`
- Produces: `Graph.Ready(completed map[string]bool) []contract.Task`

- [ ] **Step 1: Write failing graph tests**

```go
func TestReadyReturnsOnlySatisfiedTasksInStableOrder(t *testing.T) {
    g, violations := Build([]contract.Task{
        {ID:"tests", DependsOn:[]string{"api"}, AllowedPaths:[]string{"tests/payments/**"}},
        {ID:"api", AllowedPaths:[]string{"src/payments/**"}},
    })
    if len(violations) != 0 { t.Fatal(violations) }
    if got := ids(g.Ready(nil)); !reflect.DeepEqual(got, []string{"api"}) { t.Fatalf("ready=%v", got) }
}

func TestBuildRejectsCycleAndProtectedOwnership(t *testing.T) {
    _, got := Build([]contract.Task{
        {ID:"a", DependsOn:[]string{"b"}, AllowedPaths:[]string{"migrations/**"}},
        {ID:"b", DependsOn:[]string{"a"}, AllowedPaths:[]string{"src/b/**"}},
    })
    if !hasCodes(got, "dependency_cycle", "protected_parallel_path") { t.Fatal(got) }
}

func ids(tasks []contract.Task) []string {
    out := make([]string,len(tasks)); for i, task := range tasks { out[i] = task.ID }; return out
}

func hasCodes(items []contract.Violation, codes ...string) bool {
    seen := map[string]bool{}; for _, item := range items { seen[item.Code] = true }
    for _, code := range codes { if !seen[code] { return false } }; return true
}
```

- [ ] **Step 2: Run and observe missing graph package**

Run: `go test ./internal/dag -v`

Expected: FAIL.

- [ ] **Step 3: Implement normalized ownership and cycle detection**

Normalize separators to `/`, clean `.` segments, keep glob suffixes and reject `..`. Treat one path as overlapping another when their static prefixes are equal or one is a directory prefix of the other. Use Kahn's algorithm with Task ID lexical tie-breaking.

- [ ] **Step 4: Return deterministic ready Tasks and violations**

`Ready` returns uncompleted Tasks whose dependencies are complete, sorted by original contract order. Add graph violations to `contract.Validate`.

Run: `go test ./internal/dag ./internal/contract -v`

Expected: PASS.

- [ ] **Step 5: Commit DAG validation**

```bash
git add internal/dag internal/contract/validate.go
git commit -m "feat: 병렬 Task DAG와 경로 소유권 검증"
```

### Task 2: Schedule at most two Builders deterministically

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`

**Interfaces:**
- Consumes: `dag.Graph`, current Task states
- Produces: `scheduler.Next(graph dag.Graph, states map[string]TaskState, limit int) []contract.Task`

- [ ] **Step 1: Write failing capacity and dependency tests**

```go
func TestNextNeverExceedsTwo(t *testing.T) {
    graph, violations := dag.Build([]contract.Task{{ID:"a",AllowedPaths:[]string{"a/**"}},{ID:"b",AllowedPaths:[]string{"b/**"}},{ID:"c",AllowedPaths:[]string{"c/**"}}})
    if len(violations) != 0 { t.Fatal(violations) }
    got := Next(graph, map[string]TaskState{}, 2)
    if len(got) != 2 { t.Fatalf("got=%d", len(got)) }
}

func TestNextDoesNotDispatchDependentTask(t *testing.T) {
    graph, violations := dag.Build([]contract.Task{{ID:"api",AllowedPaths:[]string{"src/**"}},{ID:"tests",DependsOn:[]string{"api"},AllowedPaths:[]string{"tests/**"}}})
    if len(violations) != 0 { t.Fatal(violations) }
    got := Next(graph, map[string]TaskState{"api": Running}, 2)
    if len(got) != 0 { t.Fatalf("got=%v", got) }
}
```

- [ ] **Step 2: Run and verify scheduler absence**

Run: `go test ./internal/scheduler -v`

Expected: FAIL.

- [ ] **Step 3: Implement pure scheduling without goroutines**

`Next` computes desired dispatches only. The Orchestrator starts processes after persisting a `task_dispatch_planned` event, avoiding hidden concurrency inside the scheduler.

- [ ] **Step 4: Cover completion, failure and paused states**

Only `Pending` Tasks can dispatch. `Completed` satisfies dependencies. `Running`, `Paused`, `Failed`, `Blocked` and `Repairing` consume no new duplicate slot.

Run: `go test ./internal/scheduler -v`

Expected: PASS.

- [ ] **Step 5: Commit the scheduler**

```bash
git add internal/scheduler
git commit -m "feat: 두 슬롯 Builder scheduler 추가"
```

### Task 3: Integrate Builder commits and reject unsafe results

**Files:**
- Create: `internal/integration/integrator.go`
- Create: `internal/integration/integrator_test.go`
- Modify: `internal/worktree/git.go`

**Interfaces:**
- Produces: `integration.ValidateResult(ctx, Task, BuilderResult) error`
- Produces: `integration.MergeResults(ctx, integrationPath string, results []BuilderResult) (IntegrationResult, error)`

- [ ] **Step 1: Write failing result-contract tests**

```go
func TestValidateResultRejectsChangedPathOutsideOwnership(t *testing.T) {
    owned := contract.Task{ID:"api",AllowedPaths:[]string{"src/payments/**"}}
    err := ValidateResult(context.Background(), owned, BuilderResult{ChangedFiles:[]string{"src/auth/token.go"}, CommitSHA:"0123456789abcdef0123456789abcdef01234567", Verification:[]Check{{Command:"go test ./...", Passed:true}}})
    if !errors.Is(err, ErrPathOwnership) { t.Fatalf("err=%v", err) }
}

func TestMergeResultsStopsOnConflict(t *testing.T) {
    integrator := New(fakeGit{mergeErr: worktree.ErrConflict})
    results := []BuilderResult{{TaskID:"api",CommitSHA:"0123456789abcdef0123456789abcdef01234567"},{TaskID:"tests",CommitSHA:"89abcdef0123456789abcdef0123456789abcdef"}}
    _, err := integrator.MergeResults(context.Background(), "/work/integration", results)
    if !errors.Is(err, ErrBlockedConflict) { t.Fatalf("err=%v", err) }
}

type fakeGit struct { mergeErr error }
func (f fakeGit) ChangedFiles(context.Context,string,string) ([]string,error) { return []string{"src/payments/retry.go"},nil }
func (f fakeGit) Merge(context.Context,string,string) error { return f.mergeErr }
func (f fakeGit) AbortMerge(context.Context,string) error { return nil }
func (f fakeGit) RunChecks(context.Context,string,[]string) ([]Check,error) { return []Check{{Command:"go test ./...",Passed:true}},nil }
```

- [ ] **Step 2: Run and observe missing integration package**

Run: `go test ./internal/integration -v`

Expected: FAIL.

- [ ] **Step 3: Implement evidence validation**

Require a 40-character commit SHA, at least one passed verification check and actual changed files from `git diff --name-only BASE..COMMIT`. Compare actual files to reported files and Task ownership; any mismatch blocks integration.

- [ ] **Step 4: Merge in contract order and rerun full verification**

Run `git merge --no-ff --no-edit COMMIT` per Task order. On conflict, run `git merge --abort` and return `ErrBlockedConflict`. After all merges, execute the contract's global verification commands and record stdout summaries and exit codes.

Run: `go test ./internal/integration ./internal/worktree -v`

Expected: PASS.

- [ ] **Step 5: Commit integration checks**

```bash
git add internal/integration internal/worktree/git.go
git commit -m "feat: Builder 결과 검증과 통합 추가"
```

### Task 4: Share a two-round repair budget across Reviewer and CI

**Files:**
- Create: `internal/review/loop.go`
- Create: `internal/review/loop_test.go`

**Interfaces:**
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

The repair packet contains original acceptance criteria, current integration commit, numbered blocking findings, allowed paths and the remaining budget. It must not contain recommendations classified as non-blocking.

Run: `go test ./internal/review -v`

Expected: PASS.

- [ ] **Step 5: Commit repair budgeting**

```bash
git add internal/review
git commit -m "feat: 리뷰와 CI 공유 수정 예산 추가"
```

### Task 5: Implement tolerant slow-model recovery

**Files:**
- Create: `internal/recovery/supervisor.go`
- Create: `internal/recovery/supervisor_test.go`

**Interfaces:**
- Produces: `recovery.Decide(now time.Time, agent AgentSnapshot, run state.RunSnapshot) Decision`
- Produces: `Wait`, `Continue`, `ResumeSession`, `AskOperator`, `Block`

- [ ] **Step 1: Write failing policy tests with a fake clock**

```go
func TestWorkingAgentWaitsForSixtyMinutes(t *testing.T) {
    base := time.Date(2026,8,28,0,0,0,0,time.UTC)
    d := Decide(base.Add(59*time.Minute), AgentSnapshot{State:"working", LastProgress:base, ProcessAlive:true}, state.RunSnapshot{})
    if d.Kind != Wait { t.Fatal(d) }
}

func TestIncompleteIdleGetsContinueInstruction(t *testing.T) {
    base := time.Date(2026,8,28,0,0,0,0,time.UTC)
    d := Decide(base.Add(10*time.Minute), AgentSnapshot{State:"idle", Complete:false, ProcessAlive:true}, state.RunSnapshot{RecoveryCount:0})
    if d.Kind != Continue || d.NextCount != 1 { t.Fatal(d) }
}

func TestBlockedNeverReceivesAutomaticInput(t *testing.T) {
    base := time.Date(2026,8,28,0,0,0,0,time.UTC)
    d := Decide(base.Add(time.Hour), AgentSnapshot{State:"blocked", ProcessAlive:true}, state.RunSnapshot{})
    if d.Kind != AskOperator { t.Fatal(d) }
}
```

- [ ] **Step 2: Run and observe missing supervisor**

Run: `go test ./internal/recovery -v`

Expected: FAIL.

- [ ] **Step 3: Implement progress and exclusion rules**

Progress is a new commit, changed file hash, completed Task or verification result. A running build/test command excludes the interval. Any progress resets `RecoveryCount` to zero. `working` with a live process waits for 60 minutes before `ResumeSession`; incomplete `idle/done` immediately returns `Continue`.

- [ ] **Step 4: Cap recovery and preserve evidence**

After three consecutive recovery attempts without progress, return `Block`. Never stop the Herdr server, delete a pane or remove a Worktree. The continuation text is exactly `Task packet과 현재 변경을 다시 확인하고, 완료되지 않은 수용 조건부터 계속 진행하세요. 이미 완료한 작업은 반복하지 마세요.`

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

**Interfaces:**
- Produces: `mergegate.Evaluate(Input) Decision`
- Adds: `github.Client.GetChecks`, `MergePullRequest`, `CreateRevertPR`
- Decisions: `Wait`, `NeedsOperator`, `Merge`, `Block`

```go
type Input struct {
    AcceptanceMet    bool
    BuildersComplete bool
    ReviewerApproved bool
    Checks           []CheckState
    LatestMainTested bool
    Mergeable        bool
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
        {"ordinary", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,Mergeable:true}, Merge},
        {"protected", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,Mergeable:true,ProtectedReasons:[]string{"authentication/**"}}, NeedsOperator},
        {"check pending", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"pending"}},LatestMainTested:true,Mergeable:true}, Wait},
        {"review blocked", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:false,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,Mergeable:true}, Block},
        {"conflict", Input{AcceptanceMet:true,BuildersComplete:true,ReviewerApproved:true,Checks:[]CheckState{{Name:"ci",State:"success"}},LatestMainTested:true,Mergeable:false}, Block},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { if got:=Evaluate(tc.in); got.Kind!=tc.want { t.Fatalf("got=%v", got) } }) }
}
```

- [ ] **Step 2: Run and observe missing gate**

Run: `go test ./internal/mergegate -v`

Expected: FAIL.

- [ ] **Step 3: Implement Protected Change classification**

Mark protected when changed files match `migrations/**`, `authentication/**`, `.github/workflows/**`, `deployment/**`, when the contract explicitly marks data/public contract/supply-chain risk, or when the Reviewer reports one of those categories. Persist all reasons in the decision.

- [ ] **Step 4: Implement GHES checks and merge endpoints**

Before gate evaluation, fetch latest main, merge it into the integration branch and rerun all global verification commands. A conflict returns `Block`; a changed integration SHA invalidates earlier checks and requires fresh CI. Read checks and PR mergeability only for that final SHA, then call `PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/merge` only for `Merge`. Use merge method `merge` and exact final SHA. A `NeedsOperator` decision writes a PR comment and waits for `agentctl confirm RUN protected-change`. Implement `CreateRevertPR` by creating branch `revert/<parent>-<merge-sha-prefix>`, running `git revert --no-edit MERGE_SHA` in a safe Worktree, pushing the branch and opening a draft PR linked to the Parent Issue; test conflict returns `Blocked` without force-reset.

Run: `go test ./internal/mergegate ./internal/github -v`

Expected: PASS.

- [ ] **Step 5: Commit Merge Gate behavior**

```bash
git add internal/mergegate internal/github
git commit -m "feat: 보호 변경을 포함한 병합 게이트 추가"
```

### Task 7: Complete the parallel Orchestrator and pilot drills

**Files:**
- Create: `internal/orchestrator/parallel.go`
- Create: `internal/orchestrator/parallel_test.go`
- Create: `internal/cli/confirm.go`
- Create: `internal/cli/confirm_test.go`
- Create: `internal/cli/revert.go`
- Create: `internal/cli/revert_test.go`
- Modify: `internal/cli/run.go`
- Create: `docs/operator/parallel-pilot.md`

**Interfaces:**
- Consumes: all previous Tasks in this plan
- Produces: full phases through `completed` or `blocked`
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
    base.Orchestrator = NewParallel(base.Deps)
    return &parallelHarness{harness:base,changedFiles:[]string{"src/payments/retry.go","tests/payments/retry_test.go"}}
}

func (h *parallelHarness) runToStable() contract.RunPhase {
    h.Herdr.reviewFailures = h.reviewFailures
    h.Herdr.ciFailures = h.ciFailures
    h.Herdr.stallRecoveries = h.stallRecoveries
    h.Deps.Git.(*fakeGit).changedFiles = append([]string(nil),h.changedFiles...)
    h.Deps.Git.(*fakeGit).mergeConflict = h.mergeConflict
    id, err := h.Orchestrator.Start(context.Background(),h.ContractPath); if err!=nil { h.t.Fatal(err) }
    for i:=0;i<30;i++ { if err:=h.Orchestrator.Advance(context.Background(),id);err!=nil { h.t.Fatal(err) }; snapshot,_:=h.Store.Load(context.Background(),id); if snapshot.Phase==contract.PhaseCompleted||snapshot.Phase==contract.PhaseBlocked||snapshot.Phase==contract.PhaseNeedsOperator{return snapshot.Phase} }
    h.t.Fatal("30 transitions 안에 안정 상태에 도달하지 못함"); return ""
}
```

Add `t *testing.T` to the prior plan's base `harness`, and give `fakeHerdr` and `fakeGit` the fields used above. Each fake consumes one configured failure per call and otherwise returns deterministic successful evidence.

- [ ] **Step 2: Run and observe incomplete phase transitions**

Run: `go test ./internal/orchestrator ./internal/cli -run 'TestParallel|TestProtected|TestRepair|TestRecovery' -v`

Expected: FAIL.

- [ ] **Step 3: Implement one-action-per-advance orchestration**

Persist an intent event before every Issue write, Project status update, Agent start, Git merge, Reviewer prompt, CI observation and main merge. After restart, reconcile the intended action with GHES, Git and Herdr before retrying it. Set Organization Project states `Ready` after Issue approval, `In Progress` after dispatch, `Review` after final PR creation and `Done` after main merge.

- [ ] **Step 4: Run deterministic E2E tests and real pilot drills**

Run: `go test -race ./...`

Then use a disposable repository to execute the six stories documented in `parallel-pilot.md`. Record Issue/PR links, Run IDs, commits, repair/recovery counts and whether manual cleanup was needed.

Also run `agentctl create-revert RUN --reason "pilot regression"` against the ordinary merged story and verify a draft Revert PR is created while main remains unchanged.

Expected: tests pass; ordinary story reaches `completed`; failure stories reach the exact expected `blocked` or `needs_operator` state.

- [ ] **Step 5: Commit the parallel increment**

```bash
git add internal/orchestrator internal/cli docs/operator/parallel-pilot.md
git commit -m "feat: 병렬 실행과 감독형 자동 병합 완성"
```
