# Task 4 report: shared Reviewer and CI repair budget

## Implementation summary

Implemented strict Reviewer evidence parsing and a pure two-round repair loop:

- added `herdr.ReviewFinding` and `herdr.ReviewEvidence` with dedicated review markers and schema example;
- parameterized Builder envelope extraction while preserving the existing Builder wrapper and behavior;
- added `CLI.ReadReviewEvidence` with exact request-ID matching, strict JSON/envelope/size/NUL/raw guards, canonical finding/path validation, shared contract risk-category validation, duplicate checks, and credential scanning;
- added `review.Decide` with one shared Reviewer+CI budget, source validation, negative-counter clamping, and a hard maximum of two repairs; and
- added deterministic `review.BuildRepairPacket` containing only criteria, current integration SHA, blocking findings, allowed paths, and remaining budget.

## Files changed

- `internal/contract/validate.go`
- `internal/contract/validate_test.go`
- `internal/herdr/client.go`
- `internal/herdr/cli.go`
- `internal/herdr/cli_test.go`
- `internal/review/loop.go`
- `internal/review/loop_test.go`

## RED

Command:

```text
docker run --rm -v /home/appuser/dev_system/.worktrees/issue-42-integration:/src -w /src golang:1.27-bookworm go test ./internal/review -v
```

Expected failure before implementation:

```text
undefined: Decide
undefined: ReviewResult
undefined: Repair
undefined: Block
FAIL    thread-dock/internal/review [build failed]
```

The focused tests were written first and failed because the review package and decision API did not yet exist.

## GREEN

Focused verification:

```text
docker run --rm -v /home/appuser/dev_system/.worktrees/issue-42-integration:/src -w /src golang:1.27-bookworm make test-focused PKGS='./internal/herdr ./internal/review ./internal/contract'
```

Result: all tests passed in `internal/herdr`, `internal/review`, and `internal/contract`.

Repository gate:

```text
docker run --rm -v /home/appuser/dev_system/.worktrees/issue-42-integration:/src -w /src golang:1.27-bookworm make check
```

Result: `bash -n`, `gofmt` gate, `go vet ./...`, and `go test ./...` passed; all repository packages passed.

## Decisions

- Review evidence uses `accept`/`block` and `blockingFindings` exactly; accepted evidence cannot carry findings, and blocked evidence must carry at least one.
- Reviewer and CI consume the same `state.RunSnapshot.RepairCount`; no source-specific allowance exists.
- Path values must already equal `pathscope.Normalize` output, allowing exact paths and terminal `/**` scopes only.
- Repair packets accept canonical, validated inputs and preserve caller-provided ordering for deterministic output.
- Orchestration and command wiring were intentionally left unchanged per the task boundary.

## Self-review

- Confirmed Builder evidence tests remain green after marker extraction was generalized.
- Confirmed review payloads never return transcript text.
- Confirmed unknown fields, trailing data, missing/mismatched request IDs, malformed envelopes, oversized/NUL output, raw-only output, duplicate IDs/categories, invalid risks/paths, and credential-bearing fields are rejected.
- Confirmed invalid sources do not increment counters and counters are clamped to `[0,2]`.
- Confirmed repair packet output has no nonblocking recommendation field or content path.
- Ran `git diff --check` and the Go 1.27 `gofmt` check successfully.

## Concerns

None within the requested scope. Reviewer/CI orchestration integration remains intentionally deferred because the task explicitly prohibits modifying orchestration and CLI command wiring.
