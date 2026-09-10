# 옛 create-revert 실행 경로 제거 계획

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검사·self-review·commit → fresh Sol Task 리뷰 → 필요한 Luna 수정 → 통합 리뷰·gate 순서로 진행한다.

**Goal:** ADR 0008에서 대체한 자체 실행 엔진의 `create-revert` CLI 경로와 전용 모듈을 제거한다.
**Architecture:** `cmd/agentctl → internal/cli/revert → internal/revert` 경계만 제거한다. 나머지 v1/v2 CLI와 현재 Go/Wails Monitor는 유지한다.
**Tech Stack:** Go 1.27.0, 기존 Wails v2.15.0·React/Vite 유지.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 Go/Wails 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md), [기존 inventory](../../operator/2026-09-10-engine-dependency-inventory.md).

## Global Constraints

- Windows Go/Wails·React Monitor, `internal/runner`, 화면 wire와 링크·경로 기능을 보존한다.
- 새 runtime/scheduler/recovery/Publisher, Node/HTTP bridge를 만들지 않는다.
- `cmd/agentctl/main.go`, `main_test.go`, CLI 공유 파일의 소유자는 이번 단일 Luna다.
- `.worktrees/codex-runtime`의 중단된 실험 4파일과 기존 worktree·Windows staging은 수정·삭제·stage·재개하지 않는다.
- `go.mod`, `go.sum`, config/state/orchestrator/coordinator/github/worktree shared 파일은 변경하지 않는다.
- worker는 focused 검사만 수행한다. root의 마지막 통합 `make check` 한 번은 처음부터 별도 tmpfs TMPDIR을 사용한다.
- 실제 runtime model/effort identity는 노출되지 않으면 unverified. 역할 모델 자동 승격 금지.
- Issue·Projects·PR 기록은 한국어로 수행한다. 이번 main 병합은 사용자에게 남긴다.

## 직렬 인터페이스 결정

Sol 조사 후 root가 다음 경계를 고정했다. 신규 interface를 추가하지 않는다.

1. `agentctl create-revert RUN --reason TEXT`는 지원 목록에서 빠진다. 기존 알 수 없는 명령 처리와 같은 usage/exit 2가 된다.
2. `internal/cli.Dependencies.Reverter`를 제거한다. 전용 CLI adapter와 revert service만 소비하던 의존성이다.
3. 제거된 명령은 `NeedsProductionDependencies`에서 false다. config/token/GHES 인증 및 repository discovery를 시작하지 않는다.
4. 남은 start/resume/confirm/retire/cleanup과 v2 project/work 명령의 계약·동작은 그대로다.
5. Monitor, runner, Go/TS wire와 공용 Git/worktree 계약은 그대로다.

## 삭제 전 증거

base SHA `22dcc6610b34582d9a3a4ee375e7661fde499e5e`에서 production importer는
`cmd/agentctl/main.go`와 `internal/cli/revert.go`다. CLI adapter는 해당 route만 소비한다.
전용 구현·테스트는 약 1,216줄이며 별도 config field·fixture나 scripts 호출은 없다.
현재 operator 문서의 실행 안내는 `docs/operator/parallel-pilot.md` 한 구간에 있다.

`confirm`은 orchestrator 전이에 붙고 retire/cleanup은 공용 cleanup을 사용하므로 이번보다 넓다.
`internal/worktree.CreateManagedWorktree`/`PushBranch`와 `internal/github.FindOpenPullRequest`는
다른 engine 소비자가 있다. `Inspect/ReconcileRevertWorktree`, `RevertMergeCommit`, `AbortRevert`,
`CreateSafeDraftPR` 등의 공용 파일 내 후속 dead helper 정리는 별도 호출 inventory 후 수행한다.
역사 ADR·옛 설계/계획은 과거 기록으로 유지한다.

## Task packet

- taskId: `engine-retirement-create-revert`
- baseSHA: `22dcc6610b34582d9a3a4ee375e7661fde499e5e`
- deps: PR #70 병합, `engine-retirement-slice2-inventory`, 위 직렬 interface 결정
- ownedPaths:
  - 삭제 `internal/revert/service.go`, `internal/revert/service_test.go`
  - 삭제 `internal/cli/revert.go`, `internal/cli/revert_test.go`, `internal/cli/final_review_test.go` (revert 전용 테스트 한 개)
  - 수정 `internal/cli/run.go`, `internal/cli/run_commands.go`, `internal/cli/run_commands_test.go`, `internal/cli/confirm_test.go`
  - 수정 `cmd/agentctl/main.go`, `cmd/agentctl/main_test.go`
  - 수정 `docs/operator/parallel-pilot.md`
  - 보고서 `.superpowers/sdd/engine-retirement-slice2/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-create-revert`
- branch: `agent/engine-retirement-create-revert`
- forbiddenPaths: 소유 밖 모든 경로. 특히 `monitor/**`, `internal/runner/**`, `internal/config/**`, `internal/state/**`, `internal/coordinator/**`, `internal/orchestrator/**`, `internal/github/**`, `internal/worktree/**`, `go.mod`, `go.sum`, 다른 worktree.
- interface: 위 5개 계약. 새로운 shared 변경이 필요하면 구현 전 root/Sol에 귀속한다.
- acceptance: route·help·production dependency 구성·전용 service/test 제거, 제거 명령은 외부 의존성 없이 usage/exit 2, 나머지 CLI 회귀 없음, 현재 operator 안내 정정, config/fixture/shared 확장 없음.
- tests:
  - 삭제 전 경계와 테스트 참조를 report에 기록한다.
  - `internal/cli/run_commands_test.go`에 제거 명령의 usage/exit 2/stdout empty 사례를 추가하고, `confirm_test.go`에서 confirm production=true와 create-revert=false를 검사한다. `cmd/agentctl/main_test.go`의 discovery list에서 create-revert를 no-discovery 분류로 옮겨 config/GHES 의존성 시작 전 거부를 검증한다. 단순 파일 삭제를 고정하는 새 테스트는 만들지 않는다.
  - production 참조 검사에서는 `*_test.go`를 제외한다. negative test에 남긴 `create-revert` 문자열은 정상이다.
  - `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./...`
  - 별도 `TMPDIR`에서 `go test ./internal/cli ./cmd/agentctl`, `go vet ./internal/cli ./cmd/agentctl`
  - `git diff --check`와 `internal/revert` production 패키지 부재 확인
- result: `changedFiles`, `commitSHA` (실제 Git 출력), `executedCommands`, `outcomes`, `unverified`, `blockers`를 report에 기록한다.

## 수행 순서

- [x] 기존 command/dependency test의 거부 경계를 먼저 확인하고 필요 사례를 작성했다.
- [x] 전용 구현·테스트를 `apply_patch`로 제거하고 CLI composition·usage·운영 안내를 정정했다.
- [x] 기존 CLI/cmd 테스트와 vet, 문서 수정 후 직접 영향받는 pilot 문서 테스트를 통과하고 commit했다.
- [x] fresh Sol이 수정 head `2f5c841`에서 spec compliance와 code quality 모두 ACCEPT했다.
- [ ] root는 독립 운영 문서 Task와 결합하고 전체 리뷰·새 통합 gate 후 PR을 작성한다.

## 문서·통합

운영 문서 Task는 별도 worktree에서 `HANDOFF.md`와 `github-first-quickstart.md`의 보드·Wiki 상태를
현재 근거에 맞춘다. 코드 Task가 소유한 `parallel-pilot.md`와 겹치지 않는다.
`parallel-pilot.md`는 Legacy v1 문서이므로 해당 story를 역사 설명으로 표시하고 제거 명령을 현재 실행 지침으로 남기지 않는다.
자세한 Task packet·slot·GitHub 근거는 [이번 ledger](../../operator/2026-09-11-engine-retirement-slice2-ledger.md)에 기록한다.
