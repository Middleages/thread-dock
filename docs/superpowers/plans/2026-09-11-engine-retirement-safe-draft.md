# 미사용 GitHub safe-draft API 정리

> **For agentic workers:** Use superpowers:subagent-driven-development. Luna 구현·focused 검사·self-review → fresh Sol Task 리뷰 → 통합 gate와 전체 리뷰 → PR 전달.

**Goal:** PR #72의 revert service 제거 뒤 호출자가 사라진 GitHub safe-draft API 세 개와 전용 테스트를 제거한다.
**Architecture:** `internal/github`의 사용되지 않는 API만 줄인다. 나머지 GitHub client, 옛 orchestrator와 현재 Monitor는 그대로 둔다.
**Tech Stack:** Go 1.27.0, 기존 Go/Wails·React/Vite.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## Global Constraints

- Monitor/runner, 현재 wire·링크·경로, 기존 동작을 유지하며 새 실행 엔진·Node/HTTP 경로를 추가하지 않는다.
- 공유 API 제거는 아래 세 symbol로 root/Sol이 직렬 합의했다. 다른 shared 변경은 하지 않는다.
- 기존 codex-runtime dirty 4파일, 모든 기존 worktree·Windows staging은 수정·삭제·재개하지 않는다.
- worker는 focused 검사만 실행하고, root는 별도 tmpfs에서 마지막 `make check`를 한 번 수행한다.
- Luna high / fresh Sol medium 역할을 사용한다. 실제 runtime identity는 미노출이면 unverified, 자동 승격 금지.
- 승인된 GitHub 기록·push는 진행하며 새 PR의 main 병합은 사용자에게 남긴다.

## 삭제 전 증거와 경계

기준 main `a2f35f67ce58da5d787bd4c03165e9292624e15b`에서 아래 symbol은 정의 외 production 참조가 0개다.

- `SafeDraftPRCreator`, `ValidateSafeDraftPRRequest`: `internal/github/client.go:187-203`
- `(*RESTClient).CreateSafeDraftPR`: `internal/github/rest.go:534-557`
- 전용 테스트: `internal/github/rest_test.go:1249-1311`
- `rest.go`의 `var _ SafeDraftPRCreator = (*RESTClient)(nil)` assertion과 `client.go`에서 불필요해지는 `errors`·`strings` import도 함께 제거한다. 다른 파일에서 사용 중인 import는 유지한다.

`DraftPRRequest`는 orchestrator와 `Client.CreateDraftPR`이 사용한다. `FindOpenPullRequest`는
orchestrator가 사용하고, `MaxDraftPRTitleBytes`/`MaxDraftPRBodyBytes`는 issue-draft validation에서도 사용한다.
이들과 `CreateDraftPR`, `doSafeJSON`, `EndpointError` 및 나머지 client 동작은 보존한다.
config/fixture/scripts 참조는 없다. 이전 완료 계획의 후속 후보 언급은 역사 기록으로 보존한다.

worktree의 revert helper도 후속 후보지만 `AbortRevert` 테스트가 사용 중인 `PushBranch`와 결합되어
더 넓은 검토가 필요하다. 이번 Task에 섞지 않는다.

## Task packet

- taskId: `engine-retirement-slice3-safe-draft`
- baseSHA: `a2f35f67ce58da5d787bd4c03165e9292624e15b`
- deps: PR #72 병합, call-level inventory, root/Sol의 세 symbol 제거 계약
- ownedPaths: `internal/github/client.go`, `internal/github/rest.go`, `internal/github/rest_test.go`, `.superpowers/sdd/engine-retirement-slice3/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-safe-draft`
- branch: `agent/engine-retirement-safe-draft`
- forbiddenPaths: 소유 밖 모든 경로. 특히 Monitor/runner/orchestrator/workrun/worktree/config/state/CLI/go.mod/go.sum 및 다른 worktree.
- interface: 위 세 exported symbol만 제거하고 나머지 shared/public 동작은 유지한다.
- acceptance: 세 정의와 전용 테스트 제거, 참조 0 증명, 사용 중인 DraftPR API·크기 제한 보존. 테스트나 로직을 다른 파일로 옮기지 않는다.
- tests: `rg -n 'CreateSafeDraftPR|SafeDraftPRCreator|ValidateSafeDraftPRRequest' internal --glob '*.go'`는 exit 1 예상. 별도 tmpfs TMPDIR에서 `go test ./internal/github`, `go vet ./internal/github`, `go list ./...`, `git diff --check` 수행. 순수 미사용 삭제이므로 새 테스트를 추가하지 않는다.
- result: `changedFiles`, `commitSHA` (실제 Git 출력), `executedCommands`, `outcomes`, `unverified`, `blockers`를 report에 기록한다.

## 단계

- [ ] 삭제 전 참조를 확인하고 세 symbol 및 전용 테스트만 `apply_patch`로 제거한다.
- [ ] 영향받는 기존 GitHub 테스트/vet와 참조·package·diff 검사를 통과하고 self-review 후 commit한다.
- [ ] fresh Sol이 고정 SHA의 Task를 검토한다.
- [ ] root가 문서와 결합해 마지막 통합 gate 및 전체 리뷰 후 PR을 준비한다.

[진행 ledger](../../operator/2026-09-11-engine-retirement-slice3-ledger.md)에 검증 SHA·결과·GitHub 기록을 남긴다.
