# ThreadDock Single-Run Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute one approved contract with one OpenCode Builder and one independent Reviewer, preserving recoverable state and permanent GHES records.

**Architecture:** The Orchestrator is a state machine injected with GitHub, Herdr, Worktree, clock and local-state interfaces. Production adapters wrap explicit external commands or HTTP endpoints; tests replace every external dependency with in-memory adapters and `httptest.Server`.

**Tech Stack:** Go 1.27.0, GHES REST API `2022-11-28`, Herdr v0.8.2 CLI, OpenCode, Git, JSON snapshot plus JSONL events.

**Spec:** `../../../gitops-agent-system-design.md`

## Global Constraints

- Complete `2026-08-28-threaddock-foundation.md` first.
- Keep one active Parent Issue and one Builder in this increment.
- Use `THREADDOCK_GH_TOKEN` only in the `agentctl` process environment; never print it.
- Default config paths are `~/.config/threaddock/config.json` and `~/.local/state/threaddock/`.
- Use explicit repository and Worktree paths; reject `/`, a home directory and the canonical checkout as destructive targets.
- Herdr commands must use actual IDs returned by Herdr, never pane order.
- A Reviewer result is independent only when it starts in a fresh OpenCode session.

---

## File map

```text
internal/config/config.go                  # local non-secret configuration
internal/config/config_test.go
internal/state/types.go                    # run snapshot and events
internal/state/store.go                    # atomic JSON and append-only JSONL
internal/state/store_test.go
internal/runner/runner.go                  # process seam
internal/worktree/git.go                   # safe Git operations
internal/worktree/git_test.go
internal/github/client.go                  # narrow GHES interface
internal/github/rest.go                    # HTTP adapter
internal/github/rest_test.go
internal/herdr/client.go                   # narrow Herdr interface
internal/herdr/cli.go                      # Herdr v0.8.2 CLI adapter
internal/herdr/cli_test.go
internal/herdr/testdata/v0.8.2/...         # captured command outputs
internal/orchestrator/single.go             # single-run state machine
internal/orchestrator/single_test.go
internal/cli/run_commands.go                # start/status/stop/resume
internal/cli/run_commands_test.go
docs/operator/herdr-probe.md                # one-time environment probe
```

### Task 1: Add config and crash-safe local state

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/state/types.go`
- Create: `internal/state/store.go`
- Create: `internal/state/store_test.go`

**Interfaces:**
- Produces: `config.Load(path string) (Config, error)`
- Produces: `state.Store.Create`, `Load`, `Save`, `Append`, `ListRecoverable`, `ListCleanupCandidates`
- Produces: `state.RunSnapshot`, `state.Event`

- [ ] **Step 1: Write failing config and atomic-state tests**

```go
func TestConfigRejectsMissingGHESHost(t *testing.T) {
    _, err := Parse(strings.NewReader(`{"stateDir":"/tmp/thread-dock"}`))
    if err == nil || !strings.Contains(err.Error(), "ghesHost") { t.Fatalf("err=%v", err) }
}

func TestLoadIgnoresUncommittedTemporarySnapshot(t *testing.T) {
    root := t.TempDir()
    store := NewStore(root)
    want := RunSnapshot{RunID: "run-184", Phase: PhaseBuilding}
    if err := store.Save(context.Background(), want); err != nil { t.Fatal(err) }
    temp := filepath.Join(root,"runs","run-184","run.json.tmp")
    if err := os.WriteFile(temp, []byte("{"), 0600); err != nil { t.Fatal(err) }
    got, err := store.Load(context.Background(), "run-184")
    if err != nil { t.Fatal(err) }
    if got.Phase != PhaseBuilding { t.Fatalf("phase=%s", got.Phase) }
}
```

- [ ] **Step 2: Run and verify missing types and store**

Run: `go test ./internal/config ./internal/state -v`

Expected: FAIL.

- [ ] **Step 3: Implement exact config and state types**

```go
type Config struct {
    GHESHost      string        `json:"ghesHost"`
    APIBase       string        `json:"apiBase"`
    APIVersion    string        `json:"apiVersion"`
    StateDir      string        `json:"stateDir"`
    HerdrBinary   string        `json:"herdrBinary"`
    GitBinary     string        `json:"gitBinary"`
    WorkingWait   time.Duration `json:"workingWait"`
    RecoveryLimit int           `json:"recoveryLimit"`
    ProjectID     string        `json:"projectId"`
    ProjectStatusFieldID string `json:"projectStatusFieldId"`
    ProjectStatusOptions map[string]string `json:"projectStatusOptions"`
}

type AgentEvidence struct {
    Name         string   `json:"name"`
    SessionID    string   `json:"sessionId"`
    CommitSHA    string   `json:"commitSha"`
    ChangedFiles []string `json:"changedFiles"`
    Verification []string `json:"verification"`
}

type RunSnapshot struct {
    ContractVersion int               `json:"contractVersion"`
    RunID           contract.RunID    `json:"runId"`
    Phase           contract.RunPhase `json:"phase"`
    ContractPath    string            `json:"contractPath"`
    RepositoryPath  string            `json:"repositoryPath"`
    IntegrationPath string            `json:"integrationPath"`
    ParentIssue     int               `json:"parentIssue"`
    RepairCount     int               `json:"repairCount"`
    RecoveryCount   int               `json:"recoveryCount"`
    Builder         AgentEvidence     `json:"builder"`
    Reviewer        AgentEvidence     `json:"reviewer"`
    UpdatedAt       time.Time         `json:"updatedAt"`
}
```

Defaults are API version `2022-11-28`, `herdr`, `git`, working wait `60m` and recovery limit `3`. Config validation requires Project option IDs for `Backlog`, `Ready`, `In Progress`, `Review` and `Done`.

- [ ] **Step 4: Implement temp-write, fsync, rename and event append**

`Save` must create `run.json.tmp`, encode, `Sync`, close and rename to `run.json`. `Append` writes one complete JSON line and `Sync`s. `ListRecoverable` returns runs not in `completed` or `blocked`, sorted by `UpdatedAt` descending. `ListCleanupCandidates(now, 7*24*time.Hour)` returns only completed runs older than seven days and never deletes them. Replace the synthetic rename-failure test with a real `t.TempDir()` test: save a valid snapshot, leave a malformed `run.json.tmp` beside it, call `Load`, and assert the committed `run.json` remains readable and unchanged.

Run: `go test ./internal/config ./internal/state -v`

Expected: PASS.

- [ ] **Step 5: Commit local configuration and state**

```bash
git add internal/config internal/state
git commit -m "feat: 복구 가능한 로컬 실행 상태 추가"
```

### Task 2: Implement the narrow GHES adapter

**Files:**
- Create: `internal/github/client.go`
- Create: `internal/github/rest.go`
- Create: `internal/github/rest_test.go`

**Interfaces:**
- Consumes: approved `contract.TaskContract`
- Produces: `github.Client` interface and `github.RESTClient`

```go
type Client interface {
    FindIssueBundle(ctx context.Context, repo Repository, marker string) (IssueBundle, bool, error)
    CreateIssueBundle(ctx context.Context, repo Repository, c contract.TaskContract, marker string) (IssueBundle, error)
    CreateDraftPR(ctx context.Context, repo Repository, req DraftPRRequest) (PullRequest, error)
    UpdateIssueState(ctx context.Context, repo Repository, number int, state string) error
    SetProjectStatus(ctx context.Context, project ProjectRef, issueNodeID string, status string) error
    GetPullRequest(ctx context.Context, repo Repository, number int) (PullRequest, error)
}
```

- [ ] **Step 1: Write failing REST and redaction tests**

```go
func TestCreateIssueBundleIsIdempotent(t *testing.T) {
    server, calls := fakeGHES(t)
    client := NewRESTClient(server.URL, "secret-token", "2022-11-28", server.Client())
    first, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "td:run-184")
    if err != nil { t.Fatal(err) }
    second, err := client.CreateIssueBundle(context.Background(), repo(), testfixture.ValidContract(), "td:run-184")
    if err != nil { t.Fatal(err) }
    if first.Parent != second.Parent || calls.createIssue != 3 { t.Fatalf("first=%v second=%v calls=%d", first, second, calls.createIssue) }
}

func TestErrorNeverContainsToken(t *testing.T) {
    client := NewRESTClient("http://127.0.0.1:1", "secret-token", "2022-11-28", http.DefaultClient)
    _, err := client.FindIssueBundle(context.Background(), repo(), "td:run-184")
    if strings.Contains(fmt.Sprint(err), "secret-token") { t.Fatal("token leaked") }
}
```

Define the test repository and server helpers in `rest_test.go`:

```go
func repo() Repository { return Repository{Owner:"platform", Name:"payments-api"} }

type callCounts struct { createIssue int }

func fakeGHES(t *testing.T) (*httptest.Server, *callCounts) {
    t.Helper()
    calls := &callCounts{}
    issues := []map[string]any{}
    mux := http.NewServeMux()
    mux.HandleFunc("/api/v3/repos/platform/payments-api/issues", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type","application/json")
        if r.Method == http.MethodPost {
            var body struct { Title, Body string }
            if err := json.NewDecoder(r.Body).Decode(&body); err != nil { t.Fatal(err) }
            calls.createIssue++
            issue := map[string]any{"number":calls.createIssue,"node_id":fmt.Sprintf("I_%d",calls.createIssue),"title":body.Title,"body":body.Body}
            issues = append(issues, issue)
            w.WriteHeader(http.StatusCreated)
            if err := json.NewEncoder(w).Encode(issue); err != nil { t.Fatal(err) }
            return
        }
        if err := json.NewEncoder(w).Encode(issues); err != nil { t.Fatal(err) }
    })
    return httptest.NewServer(mux), calls
}
```

- [ ] **Step 2: Run and verify the adapter is absent**

Run: `go test ./internal/github -v`

Expected: FAIL.

- [ ] **Step 3: Implement only required endpoints**

Use `/api/v3/repos/{owner}/{repo}/issues`, `/pulls`, `/issues/{number}` and `/api/graphql`. Every request sends `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28` and Bearer authorization. Embed `<!-- threaddock:{marker} -->` in Issue bodies for idempotent lookup. `SetProjectStatus` sends `updateProjectV2ItemFieldValue` using the configured Project, status field and option IDs; fake the GraphQL endpoint and assert the exact status option for every phase.

- [ ] **Step 4: Handle 200/201/401/403/404/422 and retry hints**

Return typed errors `AuthError`, `NotFoundError`, `ConflictError` and `TemporaryError{RetryAfter}`. Do not retry inside this adapter; Orchestrator owns retry policy.

Run: `go test ./internal/github -v`

Expected: PASS.

- [ ] **Step 5: Commit the GHES adapter**

```bash
git add internal/github
git commit -m "feat: 멱등 GHES 작업 기록 adapter 추가"
```

### Task 3: Implement safe command and Worktree adapters

**Files:**
- Create: `internal/runner/runner.go`
- Create: `internal/runner/runner_test.go`
- Create: `internal/worktree/git.go`
- Create: `internal/worktree/git_test.go`

**Interfaces:**
- Produces: `runner.Runner.Run(ctx, cwd, executable string, args ...string) (Result, error)`
- Produces: `worktree.Git.Create`, `Status`, `Commit`, `Merge`, `RemoveSafe`

- [ ] **Step 1: Write failing argument-preservation and safety tests**

```go
func TestRunnerPreservesArgumentBoundaries(t *testing.T) {
    got := captureArgs(t, []string{"agentctl-test", "value with spaces", "$(not-run)"})
    if diff := strings.Join(got, "|"); diff != "value with spaces|$(not-run)" { t.Fatal(diff) }
}

func TestRemoveSafeRejectsDirtyWorktree(t *testing.T) {
    git := New(fakeRunner{status: " M src/pay.go\n"})
    err := git.RemoveSafe(context.Background(), "/work/issue-184")
    if !errors.Is(err, ErrDirtyWorktree) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run and observe missing adapters**

Run: `go test ./internal/runner ./internal/worktree -v`

Expected: FAIL.

- [ ] **Step 3: Implement `exec.CommandContext` without shell expansion**

```go
func (OSRunner) Run(ctx context.Context, cwd, executable string, args ...string) (Result, error) {
    cmd := exec.CommandContext(ctx, executable, args...)
    cmd.Dir = cwd
    var out, errOut bytes.Buffer
    cmd.Stdout, cmd.Stderr = &out, &errOut
    err := cmd.Run()
    return Result{Stdout: out.String(), Stderr: errOut.String(), ExitCode: exitCode(err)}, err
}
```

- [ ] **Step 4: Implement explicit Git operations and target checks**

`Create` runs `git worktree add -b BRANCH PATH BASE`. `Status` runs `git status --porcelain=v1`. `RemoveSafe` rejects empty paths, `/`, the user's home, repository root and dirty status, then runs `git worktree remove PATH` without `--force`.

Run: `go test ./internal/runner ./internal/worktree -v`

Expected: PASS.

- [ ] **Step 5: Commit process and Git adapters**

```bash
git add internal/runner internal/worktree
git commit -m "feat: 안전한 명령과 Worktree adapter 추가"
```

### Task 4: Probe and wrap Herdr v0.8.2

**Files:**
- Create: `docs/operator/herdr-probe.md`
- Create: `internal/herdr/client.go`
- Create: `internal/herdr/cli.go`
- Create: `internal/herdr/cli_test.go`
- Create: `internal/herdr/testdata/v0.8.2/worktree-create.txt`
- Create: `internal/herdr/testdata/v0.8.2/pane-list.txt`
- Create: `internal/herdr/testdata/v0.8.2/agent-get.txt`

**Interfaces:**
- Produces: `herdr.Client` with `CreateWorktree`, `StartAgent`, `Prompt`, `Get`, `ReadRecent`
- Consumes: `runner.Runner`

- [ ] **Step 1: Capture the real v0.8.2 command contracts**

Run on the internal mirrored binary:

```bash
herdr --version
herdr worktree create --help
herdr pane list --help
herdr agent start --help
herdr agent prompt --help
herdr agent get --help
herdr agent read --help
```

Record the version, exit codes and redacted output in `docs/operator/herdr-probe.md`. Create one disposable repository and capture redacted successful outputs for the three testdata files. If the binary does not expose stable machine-readable IDs, stop this plan and add a deterministic parser fixture before continuing.

- [ ] **Step 2: Write failing fixture-driven adapter tests**

```go
func TestCreateWorktreeReturnsActualIDs(t *testing.T) {
    cli := NewCLI(fixtureRunner("testdata/v0.8.2/worktree-create.txt"), "herdr")
    got, err := cli.CreateWorktree(context.Background(), CreateWorktreeRequest{Repo: "/repo", Branch: "agent/184-integration", Base: "main", Label: "issue-184-integration"})
    if err != nil { t.Fatal(err) }
    if got.WorkspaceID == "" || got.PaneID == "" { t.Fatalf("result=%#v", got) }
}
```

- [ ] **Step 3: Implement explicit Herdr commands**

Use `herdr worktree create --cwd REPO --branch BRANCH --base BASE --label LABEL --no-focus`, then `herdr pane list --workspace ID`. Start OpenCode with `herdr agent start NAME --kind opencode --pane PANE`. Prompt with `herdr agent prompt NAME PACKET --wait --timeout 3600000`. Never construct one shell string.

- [ ] **Step 4: Map Herdr states without claiming completion**

Map `working`, `blocked`, `idle`, `done`, `unknown` to `AgentState`, but require commit and verification evidence separately. `ReadRecent` runs `agent read NAME --source recent-unwrapped --lines 120` only for diagnosis.

Run: `go test ./internal/herdr -v`

Expected: PASS against captured v0.8.2 fixtures.

- [ ] **Step 5: Commit the probed adapter**

```bash
git add docs/operator/herdr-probe.md internal/herdr
git commit -m "feat: Herdr v0.8.2 실행 adapter 고정"
```

### Task 5: Implement the one-Builder state machine

**Files:**
- Create: `internal/orchestrator/ports.go`
- Create: `internal/orchestrator/single.go`
- Create: `internal/orchestrator/single_test.go`
- Create: `internal/orchestrator/fakes_test.go`

**Interfaces:**
- Consumes: `contract.TaskContract`, `state.Store`, `github.Client`, `herdr.Client`, `worktree.Git`
- Produces: `orchestrator.Start(ctx, contractPath string) (contract.RunID, error)`
- Produces: `orchestrator.Advance(ctx, runID) error`

- [ ] **Step 1: Write the failing happy-path state test**

```go
func TestSingleRunReachesReviewWithEvidence(t *testing.T) {
    h := newHarness()
    id, err := h.Orchestrator.Start(context.Background(), h.ContractPath)
    if err != nil { t.Fatal(err) }
    for i := 0; i < 8; i++ { if err := h.Orchestrator.Advance(context.Background(), id); err != nil { t.Fatal(err) } }
    got := h.Store.mustLoad(id)
    if got.Phase != contract.PhaseReviewing { t.Fatalf("phase=%s", got.Phase) }
    if got.Builder.CommitSHA == "" || len(got.Builder.Verification) == 0 { t.Fatalf("builder=%#v", got.Builder) }
    if h.Herdr.started[0].Kind != "opencode" { t.Fatalf("agent=%#v", h.Herdr.started[0]) }
}
```

`fakes_test.go` defines in-memory adapters with recorded calls, plus:

```go
type harness struct {
    t            *testing.T
    Orchestrator *Orchestrator
    Deps         Dependencies
    Store        *state.Store
    Herdr        *fakeHerdr
    GitHub       *fakeGitHub
    ContractPath string
}

func newHarness(t *testing.T) *harness {
    t.Helper()
    dir := t.TempDir()
    contractPath := filepath.Join(dir,"contract.json")
    f, err := os.Create(contractPath); if err != nil { t.Fatal(err) }
    if err := contract.Write(f, testfixture.ValidContract()); err != nil { t.Fatal(err) }
    if err := f.Close(); err != nil { t.Fatal(err) }
    store := state.NewStore(filepath.Join(dir,"state"))
    gh, hd, git := &fakeGitHub{}, &fakeHerdr{builderResult:validBuilderEvidence()}, &fakeGit{}
    deps := Dependencies{Store:store,GitHub:gh,Herdr:hd,Git:git,Clock:fixedClock()}
    return &harness{t:t,Orchestrator:New(deps),Deps:deps,Store:store,Herdr:hd,GitHub:gh,ContractPath:contractPath}
}
```

The same file defines `fakeGitHub`, `fakeHerdr`, `fakeGit`, `validBuilderEvidence` and `fixedClock` by implementing every method in `ports.go`; unconfigured methods call `t.Fatalf` so unexpected effects fail the test.

- [ ] **Step 2: Write failing pause and external-write tests**

Test that an unreadable contract creates no Issue, a failed Issue creation leaves phase `registered` with an event, and `Stop` changes phase to `paused` without deleting a Worktree.

- [ ] **Step 3: Implement explicit transitions**

Allowed phases are `registered → analyzing → building → integrating → reviewing`. `Start` validates the contract, creates the local run first, then creates or finds the Issue bundle. `Advance` performs at most one external action and saves an event before returning. For `github.TemporaryError`, schedule at most three attempts using delays `2s`, `5s`, `10s` and persist each attempt before waiting. A fourth failure changes the summary to `GitHub 연결 문제`, preserves the current phase and makes no main merge attempt. Add a fake-clock test asserting exactly three calls and no duplicate Issue or Worktree.

- [ ] **Step 4: Start Reviewer in a fresh session**

After Builder commit and verification evidence are validated, create a Reviewer Worktree or reuse the integration Worktree read-only, start a new agent name `reviewer-<runID>`, and prompt only with Parent acceptance criteria, final diff, verification results and review result schema. Do not include Builder transcript.

Run: `go test ./internal/orchestrator -v`

Expected: PASS.

- [ ] **Step 5: Commit the single-run Orchestrator**

```bash
git add internal/orchestrator
git commit -m "feat: 단일 Builder 실행 상태 머신 추가"
```

### Task 6: Expose start, status, stop and resume through `agentctl`

**Files:**
- Create: `internal/cli/run_commands.go`
- Create: `internal/cli/run_commands_test.go`
- Create: `internal/cli/fakes_test.go`
- Modify: `internal/cli/run.go`
- Modify: `cmd/agentctl/main.go`
- Create: `docs/operator/single-run-pilot.md`

**Interfaces:**
- Consumes: `orchestrator.Start`, `Advance`, local state
- Produces: `agentctl start CONTRACT`, `status [RUN] --json`, `stop RUN`, `resume RUN`, `cleanup RUN`
- Produces: `cli.RunWithDependencies(ctx, args, stdout, stderr, Dependencies) int`

```go
type RunService interface {
    Start(ctx context.Context, contractPath string) (contract.RunID,error)
    Status(ctx context.Context, runID contract.RunID) (contract.StatusView,error)
    Stop(ctx context.Context, runID contract.RunID) error
    Resume(ctx context.Context, runID contract.RunID) error
    Cleanup(ctx context.Context, runID contract.RunID) error
}
type Dependencies struct { Runs RunService }
```

- [ ] **Step 1: Write failing stable-JSON tests**

```go
func TestStatusJSONContract(t *testing.T) {
    h := newCLIHarness(t)
    var out, errOut bytes.Buffer
    code := h.Run([]string{"status", "run-184", "--json"}, &out, &errOut)
    if code != 0 { t.Fatalf("code=%d stderr=%s", code, errOut.String()) }
    var view contract.StatusView
    if err := json.Unmarshal(out.Bytes(), &view); err != nil { t.Fatal(err) }
    if view.ContractVersion != 1 || view.RunID != "run-184" { t.Fatalf("view=%#v", view) }
}
```

`internal/cli/fakes_test.go` defines the harness without production adapters:

```go
type fakeRunService struct { view contract.StatusView; calls []string }
func (f *fakeRunService) Start(_ context.Context,path string)(contract.RunID,error){f.calls=append(f.calls,"start:"+path);return "run-184",nil}
func (f *fakeRunService) Status(_ context.Context,id contract.RunID)(contract.StatusView,error){f.calls=append(f.calls,"status:"+string(id));return f.view,nil}
func (f *fakeRunService) Stop(_ context.Context,id contract.RunID)error{f.calls=append(f.calls,"stop:"+string(id));return nil}
func (f *fakeRunService) Resume(_ context.Context,id contract.RunID)error{f.calls=append(f.calls,"resume:"+string(id));return nil}
func (f *fakeRunService) Cleanup(_ context.Context,id contract.RunID)error{f.calls=append(f.calls,"cleanup:"+string(id));return nil}

type cliHarness struct { service *fakeRunService }
func newCLIHarness(t *testing.T) *cliHarness {
    t.Helper()
    return &cliHarness{service:&fakeRunService{view:contract.StatusView{ContractVersion:1,RunID:"run-184",Phase:contract.PhaseReviewing,Summary:"독립 확인 중",UpdatedAt:time.Date(2026,8,28,0,0,0,0,time.UTC)}}}
}
func (h *cliHarness) Run(args []string,out,errOut io.Writer) int {
    return RunWithDependencies(context.Background(),args,out,errOut,Dependencies{Runs:h.service})
}
```

- [ ] **Step 2: Run and observe missing command routing**

Run: `go test ./internal/cli -run 'Test(Start|Status|Stop|Resume)' -v`

Expected: FAIL.

- [ ] **Step 3: Implement commands with Korean summaries**

`start` returns the Run ID. `status --json` writes JSON only. Human `status` writes phase, summary, last progress and next action. `stop` pauses without killing or deleting evidence. `resume` first reconciles local snapshot, Git and GHES state, then continues. `cleanup` works only for a completed run older than seven days, verifies every Worktree is clean, removes Worktrees without force and then removes only that run's local state; otherwise it refuses in Korean and changes nothing.

- [ ] **Step 4: Run fake E2E and one disposable real pilot**

Run: `go test ./...`

Then follow `docs/operator/single-run-pilot.md` against a disposable GHES repository and trivial one-file change. Disconnect GHES once, stop WSL once, resume and confirm no duplicate Issue or Worktree is created.

Expected: the final state is `reviewing` with Builder and Reviewer evidence; main is unchanged.

- [ ] **Step 5: Commit the verified single-run increment**

```bash
git add internal/cli cmd/agentctl docs/operator/single-run-pilot.md
git commit -m "feat: 단일 실행 CLI와 복구 흐름 완성"
```
