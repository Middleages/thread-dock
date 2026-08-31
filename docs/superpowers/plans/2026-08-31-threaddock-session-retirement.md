# ThreadDock Execution Session Retirement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close completed Herdr Agent Workspaces without deleting Worktrees or structured evidence, then keep the existing seven-day cleanup safe for retired sessions.

**Architecture:** A pure retirement module derives and advances an ordered durable target plan. The Orchestrator persists one observation or close intent per Advance, while narrow Herdr adapters read/close Workspaces and Git adapters prove retained Worktree identity. Retirement closes execution sessions; cleanup later removes only proven clean artifacts.

**Tech Stack:** Go 1.27.0, Herdr v0.8.2 CLI, Git Worktree, ThreadDock atomic snapshot/event store.

**Spec:** `../specs/2026-08-31-threaddock-session-retirement-design.md`

**Work record:** Parent #42; session retirement #48.

## Global Constraints

- Execution Session Retirement never deletes a Worktree, run snapshot, event log, Issue, PR, commit, or structured evidence.
- New completed parallel RUNs auto-retire by default; historical snapshots with no retirement state do not retroactively auto-retire.
- `blocked` retirement requires explicit `--blocked`; `needs_operator`, paused, and active RUNs reject retirement.
- Each Advance performs at most one external Workspace/Git observation or one Workspace close.
- Persist exact expected Workspace, pane, path, branch, repository common directory, and HEAD before the first close.
- `working`, `blocked`, unknown, or conflicting Agent/Workspace identity prevents automatic close.
- Workspace close response loss reconciles `workspace_not_found` as success; identity mismatch never closes another Workspace.
- Herdr `workspace close` makes `worktree remove --workspace` unavailable. Retired cleanup therefore uses verified non-force Git Worktree removal.
- Cleanup retains the completed-plus-seven-days gate and completes every read-only preflight before its first removal.
- No `--force`, reset, symlink escape, home/root/repository deletion, raw terminal persistence, or Codex-subagent runtime replacement.
- Preserve Single-run behavior and legacy snapshot/config decoding.

---

## File map

```text
internal/retirement/plan.go                    # pure target order and decision policy
internal/retirement/plan_test.go
internal/state/types.go                        # additive durable retirement state
internal/state/store_test.go
internal/config/config.go                      # auto-retire and Herdr root configuration
internal/config/config_test.go
internal/herdr/client.go                       # WorkspaceInfo and optional ports
internal/herdr/cli.go                          # workspace get/close adapter
internal/herdr/cli_test.go
internal/worktree/git.go                       # retirement proof and retired removal
internal/worktree/git_test.go
internal/orchestrator/retirement.go            # one-action state transitions
internal/orchestrator/retirement_test.go
internal/orchestrator/ports.go
internal/orchestrator/parallel.go
internal/cli/retire.go                         # manual command and guards
internal/cli/retire_test.go
internal/cli/run.go
internal/cli/run_commands.go                   # retired cleanup path
internal/cli/run_commands_test.go
cmd/agentctl/main.go                           # production wiring
cmd/agentctl/main_test.go
docs/operator/session-retirement.md
internal/pilot/retirement_docs_test.go          # runbook contract assertions
```

### Task 1: Add the pure retirement plan and durable/config state

**Files:**
- Create: `internal/retirement/plan.go`
- Create: `internal/retirement/plan_test.go`
- Modify: `internal/state/types.go`
- Modify: `internal/state/store_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Adds `contract.PhaseRetiring`.
- Adds `state.RetirementState` and `state.RetirementTarget` exactly as defined in the spec, plus `AgentName string`, `RepositoryCommonDir string`, and `LastError string` on a target.
- Produces `retirement.Build(snapshot state.RunSnapshot, automatic bool, targetPhase contract.RunPhase) (state.RetirementState, error)`.
- Produces `retirement.Next(state.RetirementState, observation *retirement.Observation) retirement.Decision` with `ObserveAgent`, `ProveGit`, `CloseWorkspace`, `Complete`, and `NeedsOperator`.
- Adds config `AutoRetireCompletedSessions bool` from nullable JSON `autoRetireCompletedSessions`, default `true`, and canonical `HerdrWorktreeRoot string`, defaulting to `$HOME/.herdr/worktrees`.

- [ ] **Step 1: Write failing target-order, decision, compatibility, and config tests**

```go
func TestBuildOrdersReviewerThenBuildersInReverseAndDeduplicatesWorkspace(t *testing.T) {
    snapshot := state.RunSnapshot{TaskOrder:[]string{"alpha","beta"}, FinalSHA:validSHA}
    snapshot.Reviewer = state.AgentEvidence{Name:"reviewer"}
    snapshot.ReviewerWorktree = state.WorktreeState{WorkspaceID:"review",PaneID:"review:p1",Path:"/managed/integration",Branch:"agent/integration"}
    snapshot.Tasks = map[string]state.TaskRunState{
        "alpha": {Agent:state.AgentEvidence{Name:"alpha",CommitSHA:alphaSHA},Worktree:state.WorktreeState{WorkspaceID:"a",PaneID:"a:p1",Path:"/herdr/a",Branch:"agent/a"}},
        "beta": {Agent:state.AgentEvidence{Name:"beta",CommitSHA:betaSHA},Worktree:state.WorktreeState{WorkspaceID:"b",PaneID:"b:p1",Path:"/herdr/b",Branch:"agent/b"}},
    }
    got, err := Build(snapshot,true,contract.PhaseCompleted)
    if err != nil { t.Fatal(err) }
    if ids := targetKeys(got.Targets); !reflect.DeepEqual(ids,[]string{"reviewer","builder:beta","builder:alpha"}) { t.Fatalf("targets=%v",ids) }
}

func TestConfigDefaultsAutoRetirementWithoutBreakingOldJSON(t *testing.T) {
    got := parseConfig(t, validConfigJSONWithoutRetirementFields())
    if !got.AutoRetireCompletedSessions || !strings.HasSuffix(got.HerdrWorktreeRoot,"/.herdr/worktrees") { t.Fatalf("config=%#v",got) }
    var snapshot state.RunSnapshot
    if err := json.Unmarshal([]byte(`{"runId":"legacy","phase":"completed"}`),&snapshot);err!=nil { t.Fatal(err) }
    if snapshot.Retirement.Status != "" { t.Fatalf("retirement=%#v",snapshot.Retirement) }
}
```

- [ ] **Step 2: Run and verify the interfaces are missing**

Run: `go test ./internal/retirement ./internal/state ./internal/config -v`

Expected: FAIL because the retirement package and fields do not exist.

- [ ] **Step 3: Implement the pure plan, additive JSON, and nullable config default**

`Build` rejects incomplete identity before returning targets. It keeps historical identity fields, collapses duplicate Workspace IDs, records `Status="pending"`, and never performs I/O. `Next` only consumes durable state plus one observation. Config parsing resolves and cleans the Herdr root without requiring it to exist during unit parsing.

- [ ] **Step 4: Run focused compatibility tests**

Run: `go test ./internal/retirement ./internal/state ./internal/config -v`

Expected: PASS, including byte-shape checks that omitted retirement fields remain omitted.

- [ ] **Step 5: Commit the retirement domain state**

```bash
git add internal/retirement internal/state/types.go internal/state/store_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat: 실행 세션 은퇴 상태와 정책 추가"
```

### Task 2: Add strict Herdr Workspace observation and close adapters

**Files:**
- Modify: `internal/herdr/client.go`
- Modify: `internal/herdr/cli.go`
- Modify: `internal/herdr/cli_test.go`

**Interfaces:**
- Produces `WorkspaceReader.GetWorkspace(ctx context.Context, workspaceID string) (herdr.WorkspaceInfo, bool, error)`.
- Produces `WorkspaceCloser.CloseWorkspace(ctx context.Context, workspaceID string) error`.
- `WorkspaceInfo` contains `WorkspaceID`, `RootPaneID`, `Path`, and `State`; no terminal text or provider response body.
- `workspace_not_found` maps to `(WorkspaceInfo{}, false, nil)` for reads and an idempotent success for close reconciliation.

- [ ] **Step 1: Write failing exact-argument and safe-error tests**

```go
func TestWorkspaceReaderAndCloserUseExactIDs(t *testing.T) {
    r := fixtureRunner(t,map[string]string{
        "herdr\x00workspace\x00get\x00w7": workspaceFixture("w7","w7:p1","/repo/task","done"),
        "herdr\x00workspace\x00close\x00w7": `{"id":"close","result":{"type":"ok"}}`,
    })
    cli := NewCLI(r,"herdr")
    info,found,err := cli.GetWorkspace(context.Background(),"w7")
    if err!=nil || !found || info.WorkspaceID!="w7" || info.RootPaneID!="w7:p1" { t.Fatalf("info=%#v found=%v err=%v",info,found,err) }
    if err:=cli.CloseWorkspace(context.Background(),"w7");err!=nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run and observe missing adapter methods**

Run: `go test ./internal/herdr -run 'TestWorkspace|TestCloseWorkspace' -v`

Expected: FAIL with undefined methods/types.

- [ ] **Step 3: Implement strict workspace get/close parsing**

Validate canonical Workspace IDs before invoking the runner. Parse exact returned Workspace/root pane/path identity. Treat malformed, mismatched, or provider-body failures as generic safe errors. Close sends exactly `workspace close ID` and never `worktree remove`, pane close, Agent input, or force.

- [ ] **Step 4: Run all Herdr regressions**

Run: `go test ./internal/herdr -v`

Expected: PASS with existing Agent/Evidence behavior unchanged.

- [ ] **Step 5: Commit the Herdr retirement adapter**

```bash
git add internal/herdr/client.go internal/herdr/cli.go internal/herdr/cli_test.go
git commit -m "feat: Herdr Workspace 은퇴 adapter 추가"
```

### Task 3: Prove retirement identity and safely remove retired Worktrees

**Files:**
- Modify: `internal/worktree/git.go`
- Modify: `internal/worktree/git_test.go`

**Interfaces:**
- Produces `Git.InspectRetirementTarget(ctx, repositoryPath, worktreePath, expectedBranch, expectedSHA string) (worktree.RetirementProof, error)`.
- Produces `Git.RemoveRetired(ctx, repositoryPath, trustedHerdrRoot string, proof worktree.RetirementProof) error`.
- `RetirementProof` contains canonical `RepositoryCommonDir`, `Path`, `Branch`, and `HeadSHA`.

- [ ] **Step 1: Write failing real-Git proof/removal tests**

```go
func TestRemoveRetiredRequiresExactRegisteredCleanWorktree(t *testing.T) {
    git,repo,herdrRoot,target,sha := realRetirementRepo(t)
    proof,err := git.InspectRetirementTarget(context.Background(),repo,target,"agent/task",sha)
    if err!=nil { t.Fatal(err) }
    if err:=git.RemoveRetired(context.Background(),repo,herdrRoot,proof);err!=nil { t.Fatal(err) }
    if _,err:=os.Stat(target);!errors.Is(err,os.ErrNotExist) { t.Fatalf("target remains: %v",err) }
}
```

- [ ] **Step 2: Run and observe missing Git operations**

Run: `go test ./internal/worktree -run 'Test.*Retire' -v`

Expected: FAIL because proof/removal methods are undefined.

- [ ] **Step 3: Implement repository/common-dir/registration and clean checks**

Use `git rev-parse --git-common-dir`, `git worktree list --porcelain`, exact branch, exact HEAD, and `status --porcelain=v1`. Resolve symlinks and require strict containment under the configured Herdr root. Removal invokes `git worktree remove PATH` without `--force`; proof mismatch, dirty state, foreign repository, moved branch/HEAD, root/home/repository target, or unregistered checkout returns `ErrUnsafeTarget` before mutation.

- [ ] **Step 4: Run focused and legacy Worktree tests**

Run: `go test ./internal/worktree -v`

Expected: PASS, including dirty/foreign/symlink/response-loss cases and existing cleanup behavior.

- [ ] **Step 5: Commit retirement proof and removal**

```bash
git add internal/worktree/git.go internal/worktree/git_test.go
git commit -m "feat: 은퇴 Worktree identity와 safe cleanup 추가"
```

### Task 4: Orchestrate automatic retirement one action at a time

**Files:**
- Create: `internal/orchestrator/retirement.go`
- Create: `internal/orchestrator/retirement_test.go`
- Modify: `internal/orchestrator/ports.go`
- Modify: `internal/orchestrator/parallel.go`
- Modify: `internal/orchestrator/single.go`

**Interfaces:**
- Adds optional dependency ports `WorkspaceReader`, `WorkspaceCloser`, and `RetirementGitInspector`.
- Adds `Dependencies.AutoRetireCompletedSessions bool` and `Dependencies.HerdrWorktreeRoot string`.
- Produces `Orchestrator.BeginRetirement(ctx, runID, targetPhase, automatic) error` and PhaseRetiring handling through normal `Advance`.

- [ ] **Step 1: Write failing complete-story and crash-window tests**

```go
func TestCompletedParallelRunRetiresReviewerThenBuilders(t *testing.T) {
    h := newRetirementHarness(t)
    id := h.completedParallelRun(true)
    h.advanceToTerminal(id)
    if got:=h.closedWorkspaceIDs;!reflect.DeepEqual(got,[]string{"review","beta","alpha"}) { t.Fatalf("closed=%v",got) }
    snapshot:=h.mustLoad(id)
    if snapshot.Phase!=contract.PhaseCompleted || snapshot.Retirement.Status!="retired" { t.Fatalf("snapshot=%#v",snapshot) }
    if h.removedWorktrees!=0 || h.removedState!=0 { t.Fatal("retirement deleted evidence") }
}

func TestCloseResponseLossReconcilesMissingWorkspace(t *testing.T) {
    h := newRetirementHarness(t)
    h.closeSucceedsThenLosesResponse=true
    id:=h.completedParallelRun(true)
    h.advanceToTerminal(id)
    if h.closeCalls["review"]!=1 { t.Fatalf("close calls=%v",h.closeCalls) }
}
```

- [ ] **Step 2: Run and observe missing retiring phase and coordinator**

Run: `go test ./internal/orchestrator -run 'Test.*Retire|TestCloseResponse' -v`

Expected: FAIL.

- [ ] **Step 3: Implement automatic PhaseRetiring transitions**

After main merge and enabled Project `Done` reconciliation, auto-enabled runs call `BeginRetirement` instead of completing. Persist target identity/proof before close. Separate Agent observation, Git proof, Workspace observation, and close into different Advances. A working/blocked/unknown or identity-mismatched target sets retirement `needs_operator` without deleting evidence. Closing all targets restores `TargetPhase` and appends `sessions_retired`.

- [ ] **Step 4: Cover disabled, legacy, no-target, and failure paths**

Auto-disabled runs complete with `Retirement.Status="active"`. Legacy completed snapshots remain unchanged. Empty target plans complete without external calls. Pending close reconciliation treats missing Workspace as retired and exact existing Workspace as retryable. Run:

`go test ./internal/orchestrator -run 'Test.*Retire|Test.*Retirement|TestParallelStories' -v`

Expected: PASS.

- [ ] **Step 5: Commit automatic retirement orchestration**

```bash
git add internal/orchestrator/retirement.go internal/orchestrator/retirement_test.go internal/orchestrator/ports.go internal/orchestrator/parallel.go internal/orchestrator/single.go
git commit -m "feat: 완료 RUN Agent Session 자동 은퇴"
```

### Task 5: Add manual retire CLI, status, production wiring, and cleanup migration

**Files:**
- Create: `internal/cli/retire.go`
- Create: `internal/cli/retire_test.go`
- Modify: `internal/cli/run.go`
- Modify: `internal/cli/run_commands.go`
- Modify: `internal/cli/run_commands_test.go`
- Modify: `cmd/agentctl/main.go`
- Modify: `cmd/agentctl/main_test.go`

**Interfaces:**
- Produces CLI `agentctl retire RUN` and `agentctl retire RUN --blocked`.
- Extends status JSON/Agent views with active/retiring/retired session lifecycle.
- Adds cleanup port `RemoveRetired(context.Context, repositoryPath, herdrRoot string, target state.RetirementTarget) error`.

- [ ] **Step 1: Write failing CLI guards, idempotency, status, and cleanup tests**

```go
func TestRetireRequiresBlockedFlagAndRejectsNeedsOperator(t *testing.T) {
    service:=newFakeRetireService()
    if code:=RunWithDependencies(ctx,[]string{"retire","blocked-run"},out,errOut,Dependencies{Retirement:service});code==0 { t.Fatal("blocked retired without flag") }
    if code:=RunWithDependencies(ctx,[]string{"retire","blocked-run","--blocked"},out,errOut,Dependencies{Retirement:service});code!=0 { t.Fatalf("code=%d",code) }
    if code:=RunWithDependencies(ctx,[]string{"retire","operator-run"},out,errOut,Dependencies{Retirement:service});code==0 { t.Fatal("needs_operator retired") }
}

func TestCleanupUsesGitPathForRetiredAndHerdrForActiveTargets(t *testing.T) {
    service:=cleanupHarnessWithMixedTargets(t)
    if err:=service.Cleanup(ctx,"old-completed");err!=nil { t.Fatal(err) }
    if service.retiredGitRemovals!=1 || service.activeHerdrRemovals!=1 { t.Fatalf("service=%#v",service) }
}
```

- [ ] **Step 2: Run and observe missing command and cleanup path**

Run: `go test ./internal/cli ./cmd/agentctl -run 'TestRetire|TestCleanup.*Retired|TestStatus.*Session' -v`

Expected: FAIL.

- [ ] **Step 3: Implement command routing and production dependencies**

`retire` initializes or resumes retirement and advances until terminal/needs-operator. Repeated completed+retired calls return success without close calls. Production wiring passes the canonical Herdr Worktree root, Herdr Workspace ports, Git proof/removal adapter, and auto-retire config. Usage and credential routing remain backward compatible.

- [ ] **Step 4: Migrate cleanup with complete preflight**

Build mixed cleanup targets from durable retirement state. Preflight every active Herdr identity, retired Git proof, path, status, age, root, branch, and HEAD before the first removal. Active targets use Herdr removal; retired targets use verified Git removal. Any failure leaves every target and run state untouched. Run:

`go test ./internal/cli ./cmd/agentctl -v`

Expected: PASS.

- [ ] **Step 5: Commit CLI and cleanup migration**

```bash
git add internal/cli/retire.go internal/cli/retire_test.go internal/cli/run.go internal/cli/run_commands.go internal/cli/run_commands_test.go cmd/agentctl/main.go cmd/agentctl/main_test.go
git commit -m "feat: Session retire CLI와 cleanup migration 추가"
```

### Task 6: Document and pilot retirement without touching accepted evidence

**Files:**
- Create: `docs/operator/session-retirement.md`
- Modify: `docs/operator/parallel-pilot.md`
- Create: `internal/pilot/retirement_docs_test.go`

**Interfaces:**
- Documents `retire`, `retiring`, retired status, blocked guard, auto-retire config, and two-stage cleanup.
- Produces a disposable live probe that never targets the accepted parallel pilot RUNs.

- [ ] **Step 1: Write the operator runbook and documentation assertions**

Document exact preconditions, expected snapshots/events, Workspace absence checks, Worktree/state preservation checks, seven-day cleanup simulation, secret scan, and prohibition on retiring evidence awaiting acceptance. Add a docs test that requires every command/guard phrase.

- [ ] **Step 2: Run documentation and full deterministic story tests**

Run: `go test ./internal/orchestrator ./internal/cli ./internal/herdr ./internal/worktree ./internal/config ./internal/pilot -v`

Expected: PASS.

- [ ] **Step 3: Run package-wide race and repository gate**

Run: `go test -race ./...`

Run: `make check`

Expected: PASS with no warnings or tracked runtime artifacts.

- [ ] **Step 4: Execute a frozen disposable retirement pilot**

Create a new disposable completed RUN. Record Workspace IDs and exact Git proofs, retire it, verify Workspaces are absent while Worktrees/state/evidence remain, and run a secret scan. Age only that disposable snapshot in an isolated state directory, execute cleanup, and verify non-force Worktree/state removal. Do not retire or clean current accepted/diagnostic parallel pilot RUNs.

- [ ] **Step 5: Commit the runbook and pilot evidence references**

```bash
git add docs/operator/session-retirement.md docs/operator/parallel-pilot.md internal/pilot
git commit -m "docs: Session retirement 운영과 pilot 기록"
```
