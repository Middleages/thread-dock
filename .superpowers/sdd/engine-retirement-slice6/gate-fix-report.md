# engine-retirement-slice6 gate concurrency test-fix report

## Scope and baseline

- taskId: `slice6-gate-concurrency-test-fix`
- baseSHA: `80c9a0d9d22264e00e78e1389fd5be4bce61eaf4`
- branch/worktree: `agent/engine-retirement-confirm-gate-fix` / `/home/appuser/dev_system/.worktrees/engine-retirement-confirm-gate-fix`
- deps: 최초 통합 gate 실패와 Sol의 test barrier 원인 귀속. state.Store의 nonblocking flock/claim 동작과 `TestRound1StateLockIsProcessSafe`는 불변으로 확인했다.
- ownedPaths: `internal/orchestrator/review_round1_test.go`의 `TestRound1ConcurrentAdvanceSerializesOneAction`, 이 report
- forbiddenPaths: production orchestrator/state/lock/fakes와 나머지 모든 코드·테스트·문서

## Changes

- 첫 `Advance`를 별도 buffered result channel로 실행하고 `startEntered`에서 `StartAgent` 진입을 확인한 뒤 release하지 않은 상태로 유지한다.
- 첫 호출이 lock을 보유한 동안 두 번째 `Advance`의 result channel을 timeout과 함께 수집하고 `ErrRunBusy`를 확인한다.
- 두 번째 결과 확인 후 첫 호출을 release하고 첫 결과가 nil인지와 `StartAgent`가 1회인지 검증한다.
- `sync.Once` deferred release와 first-done timeout cleanup을 사용해 assertion/timeout 경로에서도 첫 goroutine이 release 없이 남지 않도록 했다. sleep은 추가하지 않았다.
- 제품 API, orchestrator/state/lock/fake 구현 및 다른 테스트는 변경하지 않았다.

## Focused verification

지정된 Go 1.27.0 절대 경로와 별도 `/dev/shm` TMPDIR을 사용했다.

```text
GO_BIN=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go
TMPDIR=$(mktemp -d /dev/shm/threaddock-slice6-gate-fix.XXXXXX)
echo "TMPDIR=$TMPDIR"
TMPDIR="$TMPDIR" "$GO_BIN" test ./internal/orchestrator -run '^TestRound1ConcurrentAdvanceSerializesOneAction$' -count=20
actual TMPDIR output: `/dev/shm/threaddock-slice6-gate-fix.rMHGd0`
PASS: target test, 20 repetitions, exit 0

GO_BIN=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go
GOFMT=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/gofmt
TMPDIR=$(mktemp -d /dev/shm/threaddock-slice6-gate-check.XXXXXX)
echo "TMPDIR=$TMPDIR"
"$GOFMT" -w internal/orchestrator/review_round1_test.go
TMPDIR="$TMPDIR" "$GO_BIN" test ./internal/orchestrator -run '^(TestRound1ConcurrentAdvanceSerializesOneAction|TestRound1StateLockIsProcessSafe)$' -count=1
TMPDIR="$TMPDIR" "$GO_BIN" vet ./internal/orchestrator
git diff --check
actual TMPDIR output: `/dev/shm/threaddock-slice6-gate-check.w6iVBl`
PASS: paired tests, exit 0; vet, exit 0; diff check, exit 0
```

The first formatting command also used `"$GO_BIN" fmt -w internal/orchestrator/review_round1_test.go`; the required absolute `gofmt` command above was run afterward.

## Self-review

- The test now proves overlap at the product lock boundary: the first call has entered the blocking adapter before the second call is started.
- The second call is required to report `ErrRunBusy` before the first call can be released, preventing the prior nil/nil scheduling race.
- Result channels are buffered, and deferred `sync.Once` release runs on timeout or assertion failure; first completion is bounded by a timeout during cleanup.
- No sleep, production change, shared interface change, fake change, or unrelated path change was introduced.

## Result

- changedFiles: `internal/orchestrator/review_round1_test.go`, `.superpowers/sdd/engine-retirement-slice6/gate-fix-report.md`
- commitSHA: `be2f0e6` (implementation commit; this report is recorded in a follow-up metadata commit)
- executedCommands: worktree/base verification; absolute Go 1.27.0 target test `-count=20` with `/dev/shm/threaddock-slice6-gate-fix.rMHGd0`; absolute `gofmt`, paired target+`TestRound1StateLockIsProcessSafe` `-count=1`, `go vet ./internal/orchestrator`, and `git diff --check` with `/dev/shm/threaddock-slice6-gate-check.w6iVBl`
- outcomes: deterministic lock-overlap barrier is covered; target repeated 20 times and paired lock tests pass; vet and whitespace checks pass
- unverified: fresh Sol `td_reviewer` task-review; root final integration `make check`; Windows native Wails execution; actual runtime model/effort identity
- blockers: none
