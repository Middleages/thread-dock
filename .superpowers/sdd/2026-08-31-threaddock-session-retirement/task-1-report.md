# Task 1 report

Status: complete

Commit: `a37c6ec` (`feat: 실행 세션 은퇴 상태와 정책 추가`)

Implemented:

- Added `contract.PhaseRetiring`.
- Added additive durable `RetirementState`/`RetirementTarget` fields, including AgentName, repository common-directory proof, and safe error retention. Empty retirement state is omitted from legacy snapshot JSON while populated state round-trips.
- Added pure deterministic retirement planning and decisions: reviewer-first ordering, reverse TaskOrder builders, WorkspaceID deduplication, strict identity/expected-HEAD validation, agent safety gating, Git proof/close decisions, missing-Workspace reconciliation, and all-target completion.
- Added nullable `autoRetireCompletedSessions` parsing with default `true` and explicit-false preservation.
- Added canonical, non-filesystem-dependent Herdr worktree-root parsing with the default `$HOME/.herdr/worktrees`.

Verification:

- TDD RED: the initial focused command could not start because the container had no `go` binary (exit 127). The later already-closed Workspace test failed with `close_workspace` before its implementation change, then passed after the fix.
- Focused: `go test ./internal/retirement ./internal/state ./internal/config -v` — pass under Go 1.27.
- Focused vet: `go vet ./internal/retirement ./internal/state ./internal/config ./internal/contract` — pass.
- Full gate: `make check` with Go 1.27 — pass (`go vet ./...`, `go test ./...`; orchestrator suite ~556s).
- Final tree is clean; `git show --check HEAD` passes.

Concerns:

- The retirement module’s provider-neutral `Observation` includes `WorkspaceObserved` to distinguish an explicitly missing Workspace from an omitted workspace field during Git-proof decisions. Downstream orchestration should set it for WorkspaceReader observations.
- `Build` records `RunSnapshot.RepositoryPath` as the available repository identity; the Git adapter/orchestrator must replace or verify it with the canonical common-directory proof before closing sessions.
