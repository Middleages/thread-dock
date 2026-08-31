# OpenCode Role Agent Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route ThreadDock Builder and Reviewer starts/resumes through pinned named OpenCode Agents while leaving model ownership entirely in OpenCode and preserving every legacy contract, config, and snapshot.

**Architecture:** A small `internal/opencodeagent` module owns the shared safe-name grammar. Local config supplies optional Builder/Reviewer Agent names; initial RUN state copies them into additive `AgentEvidence.OpenCodeAgent`; orchestration and the Herdr adapter consume only that durable value. Contracts, Planner, models, variants, and arbitrary OpenCode arguments stay outside this interface.

**Tech Stack:** Go 1.27.0 standard library, Herdr 0.8.2 CLI, OpenCode CLI named Agents, strict JSON config/snapshot tests.

**Spec:** `docs/superpowers/specs/2026-09-01-opencode-agent-routing-design.md`

## Global Constraints

- Contract version remains `1`; contract JSON shape is unchanged.
- OpenCode is the sole authority for Agent definitions, models, providers, and variants.
- ThreadDock stores only the selected OpenCode Agent name and never accepts arbitrary trailing CLI arguments.
- Planner remains outside the RUN; only Builder and Reviewer are routed by ThreadDock.
- New RUNs pin both role names in the initial snapshot before any external action.
- Existing config and snapshots with empty routing retain byte-compatible legacy argv.
- Repair and recovery reuse pinned Builder routing; current config never rewrites an existing RUN.
- Provider/raw command errors remain redacted at existing CLI seams.
- Go verification uses exactly Go `1.27.0`.

---

### Task 1: Add shared name validation, strict config, and additive state

**Files:**
- Create: `internal/opencodeagent/name.go`
- Create: `internal/opencodeagent/name_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/state/types.go`
- Modify: `internal/state/store_test.go`

**Interfaces:**
- Produces `opencodeagent.ValidName(value string) bool` for config and Herdr validation.
- Produces `config.OpenCodeAgents{Builder, Reviewer string}` and `Config.OpenCodeAgents`.
- Produces additive `state.AgentEvidence.OpenCodeAgent string` with JSON key `openCodeAgent,omitempty`.

- [ ] **Step 1: Write failing safe-name tests**

```go
func TestValidNameAcceptsOpenCodeRoleNames(t *testing.T) {
    for _, value := range []string{"build", "threaddock-builder", "review.v2", "review_agent"} {
        if !ValidName(value) { t.Fatalf("ValidName(%q)=false", value) }
    }
}

func TestValidNameRejectsUnsafeOrNonCanonicalValues(t *testing.T) {
    for _, value := range []string{"", " reviewer", "reviewer ", "review/agent", "리뷰어", "reviewer;touch", strings.Repeat("a", 65)} {
        if ValidName(value) { t.Fatalf("ValidName(%q)=true", value) }
    }
}
```

- [ ] **Step 2: Run the name tests and confirm RED**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/opencodeagent -count=1 -v
```

Expected: FAIL because the package and `ValidName` do not exist.

- [ ] **Step 3: Implement the shared grammar**

```go
package opencodeagent

func ValidName(value string) bool {
    if value == "" || len(value) > 64 { return false }
    for _, char := range value {
        if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
            (char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
            return false
        }
    }
    return true
}
```

- [ ] **Step 4: Write failing config and compatibility tests**

Parse `"openCodeAgents":{"builder":"threaddock-builder","reviewer":"threaddock-reviewer"}` and assert both values. Add table cases proving omitted object and empty fields are accepted while whitespace, Unicode, slash, shell punctuation, and 65-byte values are rejected with the exact field name. Add `"model":"provider/model"` inside `openCodeAgents` and assert strict unknown-field rejection.

- [ ] **Step 5: Run config tests and confirm RED**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/config -run 'TestParse(OpenCode|DefaultsRetirement)' -count=1 -v
```

Expected: FAIL because the config shape is absent.

- [ ] **Step 6: Implement optional role routing config**

```go
type OpenCodeAgents struct {
    Builder  string `json:"builder"`
    Reviewer string `json:"reviewer"`
}
```

Add this value to `Config` and `configJSON`, copy it without trimming, and validate each non-empty field through `opencodeagent.ValidName`. Empty means legacy OpenCode default-Agent behavior.

- [ ] **Step 7: Write failing additive snapshot tests**

```go
encoded := marshalSnapshotWithAgent(t, "threaddock-builder")
if !bytes.Contains(encoded, []byte(`"openCodeAgent":"threaddock-builder"`)) { t.Fatal(string(encoded)) }
legacy := marshalSnapshotWithAgent(t, "")
if bytes.Contains(legacy, []byte("openCodeAgent")) { t.Fatal(string(legacy)) }
```

Decode an existing legacy fixture and assert the field is empty.

- [ ] **Step 8: Add the durable field and verify Task 1**

```go
OpenCodeAgent string `json:"openCodeAgent,omitempty"`
```

Run:

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/opencodeagent ./internal/config ./internal/state -count=1
/tmp/threaddock-go-1.27.0/bin/go vet ./internal/opencodeagent ./internal/config ./internal/state
```

Expected: PASS.

- [ ] **Step 9: Commit Task 1**

```bash
git add internal/opencodeagent internal/config internal/state
git commit -m "feat: OpenCode 역할 Agent routing 상태 추가"
```

---

### Task 2: Extend the Herdr adapter with exact safe Agent argv

**Files:**
- Modify: `internal/herdr/client.go`
- Modify: `internal/herdr/cli.go`
- Modify: `internal/herdr/cli_test.go`
- Modify: `internal/orchestrator/parallel_fakes_test.go`

**Interfaces:**
- Consumes `opencodeagent.ValidName`.
- Extends `StartAgentRequest` and `ResumeAgentRequest` with `OpenCodeAgent string`.
- Empty routing preserves existing argv; configured routing adds only `--agent NAME` after `--`.

- [ ] **Step 1: Write failing exact start argv tests**

Add one test expecting legacy argv `agent start builder --kind opencode --pane pane-1`, and one expecting suffix `-- --agent threaddock-builder`. Assert the complete runner argument slice.

- [ ] **Step 2: Write failing resume and zero-call rejection tests**

Expect resume suffix `-- --session ses_123 --agent threaddock-builder`. For `" review"`, `"review/agent"`, and `"review;touch"`, assert Start/Resume return an error and the runner receives zero calls.

- [ ] **Step 3: Run Herdr tests and confirm RED**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/herdr -run 'Test(StartAgent|ResumeAgent|AgentRouting)' -count=1 -v
```

Expected: FAIL because request fields and routed argv are absent.

- [ ] **Step 4: Implement additive request fields and argv construction**

```go
type StartAgentRequest struct { Name, PaneID, OpenCodeAgent string }
type ResumeAgentRequest struct { Name, PaneID, SessionID, OpenCodeAgent string }
```

Start uses an ordinary argument slice and conditionally appends:

```go
if req.OpenCodeAgent != "" {
    if !opencodeagent.ValidName(req.OpenCodeAgent) { return errors.New("OpenCode Agent name is invalid") }
    args = append(args, "--", "--agent", req.OpenCodeAgent)
}
```

Resume retains provider-session validation, appends `--`, `--session`, and the session ID, then conditionally appends `--agent` and the pinned name. Never join a shell string.

- [ ] **Step 5: Preserve fake forwarding and verify Task 2**

Update parallel fake resume forwarding to include `OpenCodeAgent`. Run:

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/herdr ./internal/orchestrator -run 'Test(StartAgent|ResumeAgent|AgentRouting|Round2)' -count=1
/tmp/threaddock-go-1.27.0/bin/go vet ./internal/herdr
```

Expected: PASS.

- [ ] **Step 6: Commit Task 2**

```bash
git add internal/herdr internal/orchestrator/parallel_fakes_test.go
git commit -m "feat: Herdr에 OpenCode Agent routing 전달"
```

---

### Task 3: Pin routing in initial state and reuse it across orchestration

**Files:**
- Modify: `internal/orchestrator/ports.go`
- Modify: `internal/orchestrator/single.go`
- Modify: `internal/orchestrator/single_test.go`
- Modify: `internal/orchestrator/parallel.go`
- Modify: `internal/orchestrator/parallel_test.go`
- Modify: `internal/orchestrator/parallel_round4_test.go`
- Modify: `internal/orchestrator/fakes_test.go`

**Interfaces:**
- Adds `BuilderOpenCodeAgent` and `ReviewerOpenCodeAgent` to `orchestrator.Dependencies`.
- Initial snapshot is the only seam where dependency routing enters state.
- Start/resume actions consume only `AgentEvidence.OpenCodeAgent` from state.

- [ ] **Step 1: Write failing initial snapshot tests**

Configure dependencies with `threaddock-builder` and `threaddock-reviewer`. Immediately after single and parallel `Start`, assert legacy Builder, Reviewer, and every Task Agent contain the correct pinned name before any Agent start action.

- [ ] **Step 2: Run snapshot tests and confirm RED**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/orchestrator -run 'Test.*OpenCodeAgent.*Snapshot' -count=1 -v
```

Expected: FAIL because dependencies and state wiring do not exist.

- [ ] **Step 3: Add dependencies and persist them before `Store.Create`**

```go
BuilderOpenCodeAgent  string
ReviewerOpenCodeAgent string
```

Initialize legacy Builder/Reviewer evidence and every parallel Task's nested Agent evidence. Never consult dependency routing again for an existing snapshot.

- [ ] **Step 4: Write failing Builder/Reviewer start routing tests**

Drive single and parallel RUNs far enough to start both roles. Assert fake Herdr requests use `threaddock-builder` for Builders and `threaddock-reviewer` for Reviewer. Add a zero-dependency legacy case and assert empty routing.

- [ ] **Step 5: Implement start routing without replacing evidence**

Replace whole assignments such as `snapshot.Builder = state.AgentEvidence{Name: name}` with field updates that preserve `OpenCodeAgent`. Pass the persisted field in all single Builder, single Reviewer, task Builder, and parallel Reviewer `StartAgentRequest` values.

- [ ] **Step 6: Write failing config-drift repair/recovery tests**

Create a snapshot pinned to `threaddock-builder`, rebuild the Orchestrator with dependency default `changed-builder`, and drive one repair plus one native-resume recovery. Both requests must still use `threaddock-builder`. A legacy snapshot with an empty pinned value must remain empty rather than adopt `changed-builder`.

- [ ] **Step 7: Implement pinned resume and audit evidence assignments**

Pass `taskState.Agent.OpenCodeAgent` in `ResumeAgentRequest`. Audit every `state.AgentEvidence{...}` assignment in `single.go` and `parallel.go`; convert only assignments that can overwrite pinned routing to field-wise mutation. Prompt generation, repair, evidence collection, fingerprinting, and recovery must retain the field.

- [ ] **Step 8: Run orchestration verification**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./internal/orchestrator -count=1
/tmp/threaddock-go-1.27.0/bin/go test -race ./internal/orchestrator -count=1
```

Expected: PASS across ordinary, repair, recovery, Reviewer, retirement, and legacy stories.

- [ ] **Step 9: Commit Task 3**

```bash
git add internal/orchestrator
git commit -m "feat: RUN에 OpenCode 역할 Agent 고정"
```

---

### Task 4: Wire production, document setup, and run a disposable probe

**Files:**
- Modify: `cmd/agentctl/main.go`
- Modify: `cmd/agentctl/main_test.go`
- Create: `docs/operator/opencode-role-agents.md`
- Modify: `README.md`
- Modify: `internal/pilot/retirement_docs_test.go`

**Interfaces:**
- Production consumes `Config.OpenCodeAgents` and supplies the two Orchestrator dependency values.
- Operator docs keep OpenCode as model authority and ThreadDock as name router.
- No contract command, model resolution, or GUI is added.

- [ ] **Step 1: Write a failing production wiring test**

Add package-private `roleAgentRouting(cfg config.Config) (builder, reviewer string)` and first write `TestProductionDependenciesRouteOpenCodeRoleAgents` expecting both configured values. Use the helper result in the production `orchestrator.Dependencies` literal so the test covers the same wiring seam.

- [ ] **Step 2: Run the production test and confirm RED**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./cmd/agentctl -run TestProductionDependenciesRouteOpenCodeRoleAgents -count=1 -v
```

Expected: FAIL because production routing is absent.

- [ ] **Step 3: Wire config into production dependencies**

```go
BuilderOpenCodeAgent:  cfg.OpenCodeAgents.Builder,
ReviewerOpenCodeAgent: cfg.OpenCodeAgents.Reviewer,
```

Do not read OpenCode config, invoke `opencode agent list`, or resolve a model during dependency construction.

- [ ] **Step 4: Write operator docs and assertions**

Document that Planner is selected manually and may use built-in `explore`; Builder/Reviewer definitions and models live in OpenCode; `opencode agent list` is the preflight; `openCodeAgents` fields are optional; existing RUNs retain snapshot routing; and the Go Orchestrator has no model. Add docs assertions requiring those exact role names and constraints.

- [ ] **Step 5: Run focused production/docs tests**

```bash
/tmp/threaddock-go-1.27.0/bin/go test ./cmd/agentctl ./internal/config ./internal/herdr ./internal/pilot -count=1
/tmp/threaddock-go-1.27.0/bin/go vet ./cmd/agentctl ./internal/config ./internal/herdr ./internal/pilot
```

Expected: PASS.

- [ ] **Step 6: Execute a disposable routing probe**

From a reviewed frozen checkout, create a temporary repository, state root, Herdr Worktree, and config whose Builder/Reviewer routes use installed OpenCode Agent `build`. Record frozen SHA and tool versions, disposable RUN/Workspace/pane/Agent identities, snapshot `openCodeAgent` before start, successful structured Agent observation, proof that contract JSON has no Agent/model field, secret scan, and safe retirement disposition. Never touch accepted/diagnostic `wY`, `wZ`, or `w0`; never record prompt text, transcript, raw provider output, or credentials.

- [ ] **Step 7: Run final gates**

```bash
export PATH=/tmp/threaddock-go-1.27.0/bin:$PATH
go version
go test -race ./...
make check
git diff --check
git status --short
```

Expected: Go reports `go1.27.0`; all gates pass; only intended tracked changes exist; execution reports under `.superpowers` remain ignored.

- [ ] **Step 8: Commit Task 4**

```bash
git add cmd/agentctl docs/operator/opencode-role-agents.md README.md internal/pilot/retirement_docs_test.go
git commit -m "docs: OpenCode 역할 Agent routing 운영 절차 추가"
```
