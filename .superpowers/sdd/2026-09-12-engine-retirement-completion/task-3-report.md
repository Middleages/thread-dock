# Task 3 report: retirement-v2-entry-roots

## Packet

- taskId: `retirement-v2-entry-roots`
- baseSHA: `ce3c9ba45c1d5b33f951095afa542999847c0063`
- deps: Task 1 reviewed 통합 SHA에서 시작했다. Task 2와 파일·interface가 독립이며,
  Task 1의 reviewed root cut 결과를 보존했다.
- ownedPaths: `internal/githubpublication/`, `internal/workflow/`,
  `internal/workrun/`, `internal/monitor/`,
  `internal/coordinator/integration_test.go`, 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v2-entry-roots`
- branch: `agent/retirement-v2-entry-roots`
- forbiddenPaths: 소유 밖 모든 경로. 특히 Task 2, 나머지 `internal/coordinator`,
  제품 `monitor/`, `internal/runner/`, `project-template/`, `Makefile`,
  `go.mod`, `go.sum`은 변경하지 않았다.
- interface: 옛 Work/Publication 및 aggregate root를 제거했다. `internal/monitor/`는
  제품 `monitor/`와 다른 옛 집계 package이며, coordinator integration test 전체를
  함께 제거해 `internal/workflow`의 유일한 외부 test importer를 끊었다.
- acceptance: owned source/test 삭제 후 dependency graph가 유효하고, 이번 Task에서
  남겨야 하는 downstream 대체 상태 모델이나 UI/server를 추가하지 않았다.
- tests: Go 1.27 `go list -deps ./...`, 별도 tmpfs의
  `go test ./internal/coordinator`, legacy importer·삭제 manifest·diff 검사.
  worker full suite는 실행하지 않았다.

## Importer and manifest evidence

삭제 전 Go package inventory에서 owned package의 외부 consumer는
`internal/coordinator/integration_test.go`의 `thread-dock/internal/workflow` 하나였고,
`internal/workflow` production source는 `internal/monitor`를 내부 소비했다. 그 밖의
owned package는 production/test importer가 없었다. 삭제 대상은 다음 16개 tracked file이다.

- `internal/coordinator/integration_test.go`
- `internal/githubpublication/parent_issue.go`
- `internal/githubpublication/parent_issue_test.go`
- `internal/monitor/types.go`
- `internal/monitor/types_test.go`
- `internal/workflow/service.go`
- `internal/workflow/service_test.go`
- `internal/workrun/foreground.go`
- `internal/workrun/foreground_integration_test.go`
- `internal/workrun/foreground_test.go`
- `internal/workrun/preparation.go`
- `internal/workrun/preparation_test.go`
- `internal/workrun/review_integration.go`
- `internal/workrun/review_integration_test.go`
- `internal/workrun/verification.go`
- `internal/workrun/verification_test.go`

`internal/coordinator/integration_test.go`는 옛 workflow aggregate와 함께 동작하는
통합 test 전체였으므로 삭제했다. coordinator의 남은 unit test 및 downstream runtime
구현은 수정하지 않았다.

## Result

- changedFiles: 위 16개 source/test 삭제와 이 report
- commitSHA: implementation `f170b7a2dc91ec8d7dc7082130f7e9fd7223cb4b`; report commit은
  이 report를 추가한 후의 후속 SHA다.
- executedCommands:
  - `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go version`
  - `TMPDIR=/dev/shm/retirement-v2-entry-roots.rafD9a /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -deps ./...`
  - `TMPDIR=/dev/shm/retirement-v2-entry-roots-test3.PTKcE2 /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./internal/coordinator`
  - `TMPDIR=/dev/shm/retirement-v2-entry-roots-import.TtNCAZ /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./...`
  - `rg` Go-source importer 검사, `find` 삭제 manifest 검사, `git diff --check`,
    owned-path diff 검사
- outcomes:
  - Go `go1.27.0 linux/amd64`에서 `go list -deps ./...` PASS; 제거된
    `githubpublication`, `workflow`, `workrun`, `internal/monitor` package가 graph에
    남지 않았다.
  - `go test ./internal/coordinator` PASS: `ok thread-dock/internal/coordinator 3.182s`.
  - 삭제 전 importer 증명 및 삭제 후 Go source importer 0건 PASS.
  - deleted manifest count 16, 삭제 파일 부재 및 `git diff --check` PASS.
  - diff는 owned 경로의 16개 삭제만 포함하며 총 6,649줄을 제거했다.
- unverified: 실제 runtime model/effort identity, Windows/native 실행, 전체 `make check`,
  fresh Sol Task review.
- blockers: 없음.

## Self-review

- [x] v2 업무·발행·집계 package와 전용 test를 manifest 그대로 삭제했다.
- [x] coordinator integration test 전체를 삭제해 옛 workflow 외부-test importer를
  제거했다.
- [x] 제품 `monitor/`, `internal/runner/`, shared v2/state/contract, Makefile 및
  module files를 보존했다.
- [x] source graph·focused coordinator test·importer·diff 근거를 확인했다.
- [x] 대체 실행 엔진, UI/server, compatibility layer를 추가하지 않았다.
