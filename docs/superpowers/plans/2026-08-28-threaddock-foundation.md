# ThreadDock Foundation and Work Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a host-independent Go CLI that validates and previews ThreadDock work contracts and ships the repository template used by OpenCode.

**Architecture:** `internal/contract` is the single authority for Parent/Child Issue drafts, Task ownership and verification requirements. `agentctl` is a thin caller of that module; repository templates and the OpenCode Skill produce the same versioned JSON contract consumed by later plans.

**Tech Stack:** Go 1.27.0 standard library, JSON, Markdown repository templates, shell-free Go tests.

**Spec:** `../../../gitops-agent-system-design.md`

## Global Constraints

- The Go module path is `thread-dock`; do not invent a GHES hostname.
- Use Go 1.27.0 and standard library only in this plan.
- Every contract error must include a stable machine code and a Korean human message.
- Contract preview performs no GitHub or filesystem writes.
- A Parent Issue is required; Child Issues are optional.
- Every Task requires an owner, allowed paths, acceptance criteria and verification commands.
- Protected paths default to `migrations/**`, `authentication/**`, `.github/workflows/**` and `deployment/**`.

---

## File map

```text
go.mod                                      # Go module and language version
Makefile                                    # repeatable local checks
cmd/agentctl/main.go                        # process entry point only
internal/cli/run.go                         # argument routing and exit codes
internal/cli/contract.go                    # contract validate/preview commands
internal/cli/contract_test.go               # CLI behavior
internal/contract/types.go                  # versioned contract types
internal/contract/validate.go               # all invariants and error codes
internal/contract/validate_test.go           # table-driven contract tests
internal/contract/codec.go                  # strict JSON read/write
internal/contract/codec_test.go              # compatibility and unknown-field tests
internal/contract/preview.go                # Korean Issue bundle preview
internal/contract/preview_test.go            # stable preview golden test
internal/version/version.go                 # build and contract versions
internal/version/version_test.go            # version contract
internal/projecttemplate/template_test.go    # required template inventory
internal/testfixture/contract.go             # shared canonical test contract
project-template/...                        # files installed into product repos
testdata/contracts/valid.json               # canonical fixture
testdata/contracts/invalid-overlap.json      # conflicting task fixture
```

### Task 1: Bootstrap the Go module and executable seam

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `internal/version/version_test.go`
- Create: `internal/version/version.go`
- Create: `internal/cli/run.go`
- Create: `cmd/agentctl/main.go`

**Interfaces:**
- Produces: `version.Build string`, `version.Contract int`
- Produces: `cli.Run(ctx context.Context, args []string, stdout, stderr io.Writer) int`

- [ ] **Step 1: Create the module and failing version test**

```go
// go.mod
module thread-dock

go 1.27.0
```

```go
// internal/version/version_test.go
package version

import "testing"

func TestDefaults(t *testing.T) {
    if Build != "dev" { t.Fatalf("Build = %q", Build) }
    if Contract != 1 { t.Fatalf("Contract = %d", Contract) }
}
```

- [ ] **Step 2: Run the test and observe the missing identifiers**

Run: `go test ./internal/version -run TestDefaults -v`

Expected: FAIL because `Build` and `Contract` are undefined.

- [ ] **Step 3: Add the minimal version and CLI implementation**

```go
// internal/version/version.go
package version

var Build = "dev"
const Contract = 1
```

```go
// internal/cli/run.go
package cli

import (
    "context"
    "fmt"
    "io"
    "thread-dock/internal/version"
)

func Run(_ context.Context, args []string, stdout, stderr io.Writer) int {
    if len(args) == 1 && args[0] == "version" {
        fmt.Fprintf(stdout, "thread-dock %s contract=%d\n", version.Build, version.Contract)
        return 0
    }
    fmt.Fprintln(stderr, "사용법: agentctl version | contract <validate|preview> <file>")
    return 2
}
```

```go
// cmd/agentctl/main.go
package main

import (
    "context"
    "os"
    "thread-dock/internal/cli"
)

func main() {
    os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Add repeatable repository checks and run them**

```make
.PHONY: test check
test:
	go test ./...
check:
	gofmt -w $$(find cmd internal -name '*.go')
	go vet ./...
	go test ./...
```

`.gitignore` must contain `/bin/`, `/dist/`, `/frontend/node_modules/`, `.DS_Store` and `*.test`.

Run: `make check`

Expected: PASS.

- [ ] **Step 5: Commit the bootstrap**

```bash
git add go.mod .gitignore Makefile cmd/agentctl internal/cli internal/version
git commit -m "chore: ThreadDock Go 기반 구성"
```

### Task 2: Define and validate the versioned Task contract

**Files:**
- Create: `internal/contract/types.go`
- Create: `internal/contract/validate.go`
- Create: `internal/contract/validate_test.go`

**Interfaces:**
- Produces: `contract.TaskContract`, `IssueDraft`, `RepositoryRef`, `Task`
- Produces: `contract.Validate(TaskContract) []Violation`
- Produces: violation codes `required`, `duplicate`, `path_overlap`, `unsafe_base`, `missing_dependency`

- [ ] **Step 1: Write failing validation tests**

```go
package contract

import "testing"

func TestValidateValidContract(t *testing.T) {
    got := Validate(validContract())
    if len(got) != 0 { t.Fatalf("violations = %#v", got) }
}

func TestValidateRejectsOverlappingOwnedPaths(t *testing.T) {
    c := validContract()
    c.Tasks[1].AllowedPaths = []string{"src/payments/**"}
    got := Validate(c)
    if !hasCode(got, "path_overlap") { t.Fatalf("violations = %#v", got) }
}

func TestValidateRejectsTaskPathsOverlappingProtectedPaths(t *testing.T) {
    tests := []struct { name, allowedPath string }{
        {name:"exact", allowedPath:"migrations/**"},
        {name:"task parent prefix", allowedPath:".github/**"},
        {name:"protected parent prefix", allowedPath:"deployment/services/**"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            c := validContract()
            c.Tasks[0].AllowedPaths = []string{tt.allowedPath}
            got := Validate(c)
            if !hasViolation(got, "path_overlap", "tasks[0].allowedPaths") { t.Fatalf("violations = %#v", got) }
        })
    }
}

func TestValidateRejectsMainAsBuilderBranch(t *testing.T) {
    c := validContract()
    c.Tasks[0].Branch = "main"
    got := Validate(c)
    if !hasCode(got, "unsafe_base") { t.Fatalf("violations = %#v", got) }
}
```

The helper `validContract()` must include Parent Issue `결제 실패 재시도 개선`, two Child Issues, two non-overlapping Tasks, base commit `0123456789abcdef0123456789abcdef01234567` and two verification commands.

- [ ] **Step 2: Run validation tests and verify they fail**

Run: `go test ./internal/contract -run TestValidate -v`

Expected: FAIL because contract types and `Validate` do not exist.

- [ ] **Step 3: Implement the contract types**

```go
package contract

type TaskContract struct {
    Version      int           `json:"version"`
    Parent       IssueDraft    `json:"parent"`
    Children     []IssueDraft  `json:"children"`
    Repository   RepositoryRef `json:"repository"`
    BaseCommit   string        `json:"baseCommit"`
    Tasks        []Task        `json:"tasks"`
    Protected    []string      `json:"protectedPaths"`
    Verification []string      `json:"verification"`
}

type IssueDraft struct {
    Key                string   `json:"key"`
    Title              string   `json:"title"`
    Body               string   `json:"body"`
    AcceptanceCriteria []string `json:"acceptanceCriteria"`
    Labels             []string `json:"labels"`
}

type RepositoryRef struct {
    Owner         string `json:"owner"`
    Name          string `json:"name"`
    DefaultBranch string `json:"defaultBranch"`
}

type Task struct {
    ID                 string   `json:"id"`
    IssueKey           string   `json:"issueKey"`
    Owner              string   `json:"owner"`
    Role               string   `json:"role"`
    Branch             string   `json:"branch"`
    AllowedPaths       []string `json:"allowedPaths"`
    DependsOn          []string `json:"dependsOn"`
    AcceptanceCriteria []string `json:"acceptanceCriteria"`
    Verification       []string `json:"verification"`
}

type Violation struct {
    Code    string `json:"code"`
    Field   string `json:"field"`
    Message string `json:"message"`
}

type RunID string
type RunPhase string

const (
    PhaseRegistered RunPhase = "registered"
    PhaseAnalyzing  RunPhase = "analyzing"
    PhaseBuilding   RunPhase = "building"
    PhaseIntegrating RunPhase = "integrating"
    PhaseReviewing  RunPhase = "reviewing"
    PhaseCI         RunPhase = "ci"
    PhaseNeedsOperator RunPhase = "needs_operator"
    PhaseMerging    RunPhase = "merging"
    PhaseCompleted  RunPhase = "completed"
    PhaseBlocked    RunPhase = "blocked"
    PhasePaused     RunPhase = "paused"
)

type NextAction struct { Kind, Label string }
type AgentView struct { Name, Role, State, Summary string }
type GitHubView struct { ParentIssue, PullRequest int; URL, CI string }
type StatusView struct {
    ContractVersion int         `json:"contractVersion"`
    RunID           RunID       `json:"runId"`
    Phase           RunPhase    `json:"phase"`
    Summary         string      `json:"summary"`
    NextAction      *NextAction `json:"nextAction,omitempty"`
    Agents          []AgentView `json:"agents"`
    GitHub          GitHubView  `json:"github"`
    UpdatedAt       time.Time   `json:"updatedAt"`
}
```

Add `import "time"` to `types.go` for `StatusView.UpdatedAt`.

Define the shared test fixture in `validate_test.go` exactly once:

```go
func validContract() TaskContract {
    return TaskContract{
        Version: 1,
        Parent: IssueDraft{Key:"parent", Title:"결제 실패 재시도 개선", Body:"실패한 결제를 안전하게 재시도한다.", AcceptanceCriteria:[]string{"중복 결제가 없다"}},
        Children: []IssueDraft{
            {Key:"api", Title:"재시도 정책 구현", Body:"API 변경", AcceptanceCriteria:[]string{"재시도 한도를 지킨다"}},
            {Key:"tests", Title:"회귀 검증", Body:"집중 회귀 검사", AcceptanceCriteria:[]string{"중복 결제를 검증한다"}},
        },
        Repository: RepositoryRef{Owner:"platform", Name:"payments-api", DefaultBranch:"main"},
        BaseCommit: "0123456789abcdef0123456789abcdef01234567",
        Tasks: []Task{
            {ID:"api", IssueKey:"api", Owner:"api-builder", Role:"builder", Branch:"agent/api", AllowedPaths:[]string{"src/payments/**"}, AcceptanceCriteria:[]string{"재시도 한도를 지킨다"}, Verification:[]string{"go test ./internal/payments"}},
            {ID:"tests", IssueKey:"tests", Owner:"test-builder", Role:"builder", Branch:"agent/tests", AllowedPaths:[]string{"tests/payments/**"}, AcceptanceCriteria:[]string{"중복 결제를 검증한다"}, Verification:[]string{"go test ./tests/payments"}},
        },
        Protected: []string{"migrations/**","authentication/**",".github/workflows/**","deployment/**"},
        Verification: []string{"go test ./...","go vet ./..."},
    }
}

func hasCode(v []Violation, code string) bool {
    for _, item := range v { if item.Code == code { return true } }
    return false
}
```

After the validation tests pass, copy the same values into an exported test fixture for later packages:

```go
// internal/testfixture/contract.go
package testfixture

import "thread-dock/internal/contract"

func ValidContract() contract.TaskContract {
    return contract.TaskContract{
        Version:1,
        Parent:contract.IssueDraft{Key:"parent",Title:"결제 실패 재시도 개선",Body:"실패한 결제를 안전하게 재시도한다.",AcceptanceCriteria:[]string{"중복 결제가 없다"}},
        Children:[]contract.IssueDraft{{Key:"api",Title:"재시도 정책 구현",Body:"API 변경",AcceptanceCriteria:[]string{"재시도 한도를 지킨다"}},{Key:"tests",Title:"회귀 검증",Body:"집중 회귀 검사",AcceptanceCriteria:[]string{"중복 결제를 검증한다"}}},
        Repository:contract.RepositoryRef{Owner:"platform",Name:"payments-api",DefaultBranch:"main"},
        BaseCommit:"0123456789abcdef0123456789abcdef01234567",
        Tasks:[]contract.Task{{ID:"api",IssueKey:"api",Owner:"api-builder",Role:"builder",Branch:"agent/api",AllowedPaths:[]string{"src/payments/**"},AcceptanceCriteria:[]string{"재시도 한도를 지킨다"},Verification:[]string{"go test ./internal/payments"}},{ID:"tests",IssueKey:"tests",Owner:"test-builder",Role:"builder",Branch:"agent/tests",AllowedPaths:[]string{"tests/payments/**"},AcceptanceCriteria:[]string{"중복 결제를 검증한다"},Verification:[]string{"go test ./tests/payments"}}},
        Protected:[]string{"migrations/**","authentication/**",".github/workflows/**","deployment/**"},
        Verification:[]string{"go test ./...","go vet ./..."},
    }
}
```

- [ ] **Step 4: Implement all invariants and pass the tests**

`Validate` must check required strings and slices, contract version `1`, 40-character lowercase hexadecimal `BaseCommit`, unique Issue keys and Task IDs, existing dependencies, acyclic dependencies, non-main Builder branches and overlapping path prefixes after removing a trailing `/**`. It must compare every Task `AllowedPaths` entry with every other Task path and every contract `Protected` path through the same `pathsOverlap` helper. Exact matches and either-direction directory-prefix matches emit `path_overlap` at `tasks[i].allowedPaths` with a Korean message.

Run: `go test ./internal/contract -run TestValidate -v`

Expected: PASS.

- [ ] **Step 5: Commit the contract model**

```bash
git add internal/contract/types.go internal/contract/validate.go internal/contract/validate_test.go internal/testfixture/contract.go
git commit -m "feat: 작업 계약 검증 모델 추가"
```

### Task 3: Add strict JSON codec and compatibility fixtures

**Files:**
- Create: `internal/contract/codec.go`
- Create: `internal/contract/codec_test.go`
- Create: `testdata/contracts/valid.json`
- Create: `testdata/contracts/invalid-overlap.json`

**Interfaces:**
- Consumes: `TaskContract`, `Validate`
- Produces: `contract.Read(io.Reader) (TaskContract, error)`
- Produces: `contract.Write(io.Writer, TaskContract) error`
- Produces: `contract.DiagnosticError`, `contract.ErrorViolations(error)`
- Produces: codes `invalid_json`, `unreadable` and the Korean messages `작업 계약 JSON 형식이 올바르지 않습니다`, `작업 계약 파일을 읽을 수 없습니다`

- [ ] **Step 1: Write failing codec tests**

```go
func TestReadMapsStructuralJSONErrorsToInvalidJSONDiagnostic(t *testing.T) {
    _, err := Read(strings.NewReader(`{"version":1,"unexpected":true}`))
    var diagnosticErr DiagnosticError
    if !errors.As(err, &diagnosticErr) { t.Fatalf("err = %T %v", err, err) }
    want := Violation{Code:CodeInvalidJSON, Field:"$", Message:InvalidJSONMessage}
    if diagnosticErr.Violation != want { t.Fatalf("violation = %#v", diagnosticErr.Violation) }
}

func TestWriteReadRoundTrip(t *testing.T) {
    want := validContract()
    var buf bytes.Buffer
    if err := Write(&buf, want); err != nil { t.Fatal(err) }
    got, err := Read(&buf)
    if err != nil { t.Fatal(err) }
    if !reflect.DeepEqual(want, got) { t.Fatalf("want=%#v got=%#v", want, got) }
}
```

Add `errors` and `reflect` to the test imports. The structural-error table must cover malformed JSON, unknown fields, a trailing JSON document and trailing non-JSON garbage. Every case must assert the exact typed root diagnostic and confirm that no raw decoder text appears in `Error()`.

- [ ] **Step 2: Run and observe missing codec functions**

Run: `go test ./internal/contract -run 'Test(Read|Write)' -v`

Expected: FAIL.

- [ ] **Step 3: Implement strict decode and atomic-friendly encode**

```go
const (
    CodeInvalidJSON    = "invalid_json"
    CodeUnreadable     = "unreadable"
    InvalidJSONMessage = "작업 계약 JSON 형식이 올바르지 않습니다"
    UnreadableMessage  = "작업 계약 파일을 읽을 수 없습니다"
)

type DiagnosticError struct {
    Violation Violation
    cause     error
}

func (e DiagnosticError) Error() string {
    return fmt.Sprintf("[%s] %s", e.Violation.Code, e.Violation.Message)
}

func Read(r io.Reader) (TaskContract, error) {
    dec := json.NewDecoder(r)
    dec.DisallowUnknownFields()
    var c TaskContract
    if err := dec.Decode(&c); err != nil { return c, invalidJSONError(err) }
    var extra any
    if err := dec.Decode(&extra); err != io.EOF { return c, invalidJSONError(err) }
    if v := Validate(c); len(v) > 0 { return c, ValidationError{Violations: v} }
    return c, nil
}

func Write(w io.Writer, c TaskContract) error {
    if v := Validate(c); len(v) > 0 { return ValidationError{Violations: v} }
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(c)
}
```

`invalidJSONError` must set field `$`, use only `CodeInvalidJSON` and `InvalidJSONMessage` in its public text, and retain the decoder error only as an internal unwrap cause. `NewUnreadableError` must do the same with `CodeUnreadable` and `UnreadableMessage`. `ErrorViolations` must return violations from both `ValidationError` and `DiagnosticError`, so callers never parse error strings.

- [ ] **Step 4: Add canonical fixtures and run the full package**

`valid.json` must be the JSON serialization of `validContract()`. `invalid-overlap.json` differs only by giving both Tasks `src/payments/**`.

Run: `go test ./internal/contract -v`

Expected: PASS and no fixture drift.

- [ ] **Step 5: Commit codec and fixtures**

```bash
git add internal/contract testdata/contracts
git commit -m "feat: 작업 계약 JSON 호환성 고정"
```

### Task 4: Implement Korean validate and preview CLI commands

**Files:**
- Create: `internal/contract/preview.go`
- Create: `internal/contract/preview_test.go`
- Create: `internal/cli/contract.go`
- Create: `internal/cli/contract_test.go`
- Modify: `internal/cli/run.go`

**Interfaces:**
- Consumes: `contract.Read`
- Produces: `contract.Preview(TaskContract) string`
- Produces CLI: `agentctl contract validate FILE`, `agentctl contract preview FILE`

- [ ] **Step 1: Write failing CLI behavior tests**

```go
func TestContractValidateJSON(t *testing.T) {
    var out, errOut bytes.Buffer
    code := Run(context.Background(), []string{"contract", "validate", "../../testdata/contracts/valid.json"}, &out, &errOut)
    if code != 0 { t.Fatalf("code=%d stderr=%s", code, errOut.String()) }
    if got := out.String(); !strings.Contains(got, `"valid":true`) { t.Fatalf("stdout=%s", got) }
}

func TestContractPreviewContainsApprovalSections(t *testing.T) {
    var out, errOut bytes.Buffer
    code := Run(context.Background(), []string{"contract", "preview", "../../testdata/contracts/valid.json"}, &out, &errOut)
    if code != 0 { t.Fatalf("code=%d stderr=%s", code, errOut.String()) }
    for _, want := range []string{"전체 목표", "하위 작업", "제외 범위", "검증 방법", "승인 후 자동 진행"} {
        if !strings.Contains(out.String(), want) { t.Fatalf("missing %q", want) }
    }
}
```

- [ ] **Step 2: Run tests and observe command routing failure**

Run: `go test ./internal/cli -v`

Expected: FAIL with usage exit code `2`.

- [ ] **Step 3: Implement stable preview content**

`Preview` must render, in this order: repository and base commit, Parent title/body, acceptance criteria, Child Issue list, Task ownership and dependencies, protected paths, verification commands and the sentence `승인 후 Agent가 구현·독립 확인·자동 검사·일반 변경의 기본 브랜치 반영까지 진행합니다.`

- [ ] **Step 4: Route commands, use exit codes and run tests**

Use exit code `0` for valid, `1` for invalid contract or unreadable file and `2` for command misuse. Read the complete file before decoding so open/read failures become the `unreadable` diagnostic. Successful validation output is `{"valid":true}`. Semantic, structural and unreadable failures all use `{"valid":false,"violations":[...]}` with no `error` field. Invalid preview writes only the first concise `[code] Korean message` line to stderr; successful preview output is Markdown.

Run: `go test ./internal/cli ./internal/contract -v`

Expected: PASS.

- [ ] **Step 5: Commit CLI contract commands**

```bash
git add internal/cli internal/contract/preview.go internal/contract/preview_test.go
git commit -m "feat: 작업 묶음 검증과 미리보기 추가"
```

### Task 5: Create the deterministic product-repository template

**Files:**
- Create: `project-template/AGENTS.md`
- Create: `project-template/CONTEXT.md`
- Create: `project-template/docs/architecture/CODEMAP.md`
- Create: `project-template/.github/ISSUE_TEMPLATE/development-request.yml`
- Create: `project-template/.github/pull_request_template.md`
- Create: `project-template/.github/workflows/ci.yml`
- Create: `project-template/.agents/skills/plan-work/SKILL.md`
- Create: `project-template/.agents/skills/implement-task/SKILL.md`
- Create: `project-template/.agents/skills/review-change/SKILL.md`
- Create: `internal/projecttemplate/template_test.go`

**Interfaces:**
- Consumes: contract version `1` and preview sections from Task 4
- Produces: repository files used by OpenCode and GHES

- [ ] **Step 1: Write a failing template inventory test**

```go
func TestRequiredFiles(t *testing.T) {
    required := []string{
        "AGENTS.md", "CONTEXT.md", "docs/architecture/CODEMAP.md",
        ".github/ISSUE_TEMPLATE/development-request.yml",
        ".github/pull_request_template.md", ".github/workflows/ci.yml",
        ".agents/skills/plan-work/SKILL.md",
        ".agents/skills/implement-task/SKILL.md",
        ".agents/skills/review-change/SKILL.md",
    }
    for _, name := range required {
        if _, err := os.Stat(filepath.Join("..", "..", "project-template", name)); err != nil {
            t.Errorf("missing %s: %v", name, err)
        }
    }
}
```

- [ ] **Step 2: Run and see every required file reported missing**

Run: `go test ./internal/projecttemplate -run TestRequiredFiles -v`

Expected: FAIL.

- [ ] **Step 3: Add the rule and GitHub templates**

`AGENTS.md` must contain only: assigned path boundary, no production credentials, Korean commit context, no force Worktree deletion and required result contract. The Issue form must collect problem, desired outcome, excluded scope, acceptance criteria, risk and repository. The PR template must collect Issue links, changed/excluded scope, checks, deployment impact and recovery.

- [ ] **Step 4: Add the three Skills and CI workflow**

`plan-work` must read `CONTEXT.md` and the relevant `CODEMAP.md` section, use Serena for symbol/reference discovery, produce contract version `1`, run `agentctl contract validate`, show `agentctl contract preview` and stop for approval before external writes. `implement-task` must consume one Task only. `review-change` must use a separate context and report blocking findings before recommendations. `ci.yml` must run configured repository checks rather than hard-code ThreadDock's own commands. Extend `template_test.go` to assert the `plan-work` source contains the strings `CONTEXT.md`, `CODEMAP.md`, `Serena`, `contract validate` and `contract preview`.

Run: `go test ./internal/projecttemplate -v`

Expected: PASS.

- [ ] **Step 5: Commit the repository template**

```bash
git add project-template internal/projecttemplate
git commit -m "feat: 제품 저장소 작업 계약 템플릿 추가"
```

### Task 6: Verify the complete foundation story

**Files:**
- Modify: `README.md`
- Create: `docs/operator/foundation-pilot.md`

**Interfaces:**
- Consumes: all previous Task outputs
- Produces: exact operator commands for the first contract-only pilot

- [ ] **Step 1: Write the operator walkthrough before running it**

The walkthrough must use `testdata/contracts/valid.json` and these exact commands:

```bash
make check
go run ./cmd/agentctl contract validate testdata/contracts/valid.json
go run ./cmd/agentctl contract preview testdata/contracts/valid.json
```

- [ ] **Step 2: Execute the walkthrough from a clean shell**

Expected: checks pass, validation emits `{"valid":true}`, preview contains the five approval sections and no file outside the repository changes.

- [ ] **Step 3: Add README links and version prerequisites**

README must state Go `1.27.0`, contract `1`, that this increment performs no GHES write and link the spec, roadmap and pilot walkthrough.

- [ ] **Step 4: Run the full verification gate**

Run: `make check`

Expected: `go vet ./...` and every Go test pass.

- [ ] **Step 5: Commit the verified foundation**

```bash
git add README.md docs/operator/foundation-pilot.md
git commit -m "docs: 작업 계약 pilot 절차 추가"
```
