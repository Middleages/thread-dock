# 미사용 worktree revert API 정리

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검증·self-review → fresh Sol Task 리뷰 → root 통합 gate·전체 리뷰 → PR 전달.

**Goal:** create-revert service 삭제 뒤 production 호출자가 없는 worktree revert API와 전용 테스트를 제거한다.
**Architecture:** `internal/worktree/git.go`의 미사용 타입·메서드만 줄이고 같은 파일의 사용 중인 worktree/merge/push/cleanup 동작을 보존한다.
**Tech Stack:** Go 1.27.0, 기존 Go/Wails·React/Vite 유지.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## Global Constraints

- root/Sol이 아래 여섯 exported symbol 제거를 직렬 합의했다. 그 밖의 shared interface는 바꾸지 않는다.
- Monitor/runner·CLI·GitHub client·config/state·orchestrator/workrun·go.mod/go.sum은 변경하지 않는다.
- 새 엔진·Node/HTTP 경로를 만들지 않으며 기존 실험·worktree·Windows staging을 삭제·재개하지 않는다.
- Luna high 한 명이 두 코드 파일을 단독 소유하고 fresh Sol medium이 고정 SHA를 리뷰한다. actual runtime identity는 미노출이면 unverified, 승격 금지.
- worker는 focused 검사만, root는 분리된 tmpfs에서 마지막 `make check`를 한 번 수행한다.
- 승인된 기록·push는 진행하고 새 PR의 main 병합은 사용자에게 남긴다.

## 호출 근거와 제거 계약

기준 `b2acf4c8f15c48901a0bfd0e6e50800b89a215f9`에서 정의 외 production 참조는 0개다.
제거 대상은 `RevertWorktreeStatus`, `RevertWorktreeInspection`, `ReconcileRevertWorktree`,
`InspectRevertWorktree`, `RevertMergeCommit`, `AbortRevert`다. 참조는 `git_test.go`의 전용 테스트에만 남는다.

`CreateManagedWorktree`는 workrun, `PushBranch`는 orchestrator가 사용한다.
`ErrConflict`와 `hasUnmergedPaths`는 `MergeCommitNoFF`와 conflict 처리에서 사용한다.
`gitCommonDir`, `resolveCreatePath`, `resolveRemovalPaths`, `validateManagedWorktreePath` 등 private helper도
실제 소비자가 있으므로 전부 보존한다. 기존 import들도 다른 구현에서 사용된다.
config/fixture/scripts에 직접 참조는 없고 완료 계획/ledger의 언급은 역사 기록으로 남긴다.

`TestAbortRevertAndPushBranchNeverForce`는 통째로 삭제하지 않는다. `TestPushBranchNeverForce`로 좁히고
Abort 호출과 그 첫 expected call만 제거한다. active PushBranch의 정확한 `branch:branch`와 no-force
검증을 보존한다. CreateManagedWorktree 및 다른 active 테스트는 그대로 둔다.

## Task packet

- taskId: `engine-retirement-slice4-worktree-revert`
- baseSHA: `b2acf4c8f15c48901a0bfd0e6e50800b89a215f9`
- deps: PR #74 병합, slice4 call inventory, 위 직렬 API 제거 계약
- ownedPaths: `internal/worktree/git.go`, `internal/worktree/git_test.go`, `.superpowers/sdd/engine-retirement-slice4/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-worktree-revert`
- branch: `agent/engine-retirement-worktree-revert`
- forbiddenPaths: 소유 밖 모든 경로 및 다른 worktree. 특히 Monitor/runner/CLI/GitHub/config/state/orchestrator/workrun/module 파일.
- interface: 여섯 미사용 exported 타입·메서드만 제거, 나머지 shared 계약 유지.
- acceptance: 참조 0, 전용 테스트 제거와 PushBranch 검증 보존, private helper/ErrConflict 보존. 코드를 다른 파일로 옮기거나 재구현하지 않는다.
- tests: 별도 tmpfs TMPDIR과 명시한 Go 1.27.0으로 `go test ./internal/worktree`, `go vet ./internal/worktree`, `go list ./...`, `git diff --check`. symbol 검색 `rg -n 'RevertWorktree(Status|Inspection)|ReconcileRevertWorktree|InspectRevertWorktree|RevertMergeCommit|AbortRevert' internal --glob '*.go'`는 exit 1 예상. 삭제 자체를 고정하는 새 테스트는 만들지 않고 기존 PushBranch 검증을 유지한다.
- result: changedFiles, 실제 commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록한다.

## 단계

- [x] 참조와 보존 경계를 확인하고 여섯 symbol·전용 테스트만 제거했다.
- [x] 결합 테스트를 PushBranch-only로 좁히고 기존 package focused 검사·self-review 후 commit했다.
- [x] fresh Sol이 `537bda2`에서 spec/quality 모두 ACCEPT했다. 수정 요구 없음.
- [ ] root가 문서와 결합해 단일 통합 gate·전체 리뷰 후 PR을 준비한다.

[이번 ledger](../../operator/2026-09-11-engine-retirement-slice4-ledger.md)에 검증·GitHub 근거를 기록한다.
