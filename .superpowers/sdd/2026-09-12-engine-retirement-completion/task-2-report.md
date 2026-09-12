# Task 2 report: retirement-v1-root

## Packet

- taskId: `retirement-v1-root`
- baseSHA: `ce3c9ba45c1d5b33f951095afa542999847c0063`
- deps: Task 1 reviewed 통합 후의 base에서 시작했다. 삭제 전 production/test reverse importer는 0건이었다.
- ownedPaths: `internal/orchestrator/`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-2-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v1-root`
- branch: `agent/retirement-v1-root`
- forbiddenPaths: ownedPaths 밖 전체, 특히 Task 3·Monitor·runner·project-template·Makefile·go.mod/go.sum·사용자 데이터
- interface: orphan `internal/orchestrator` 전체를 제거한다. v1 state/contract와 다른 downstream package는 이 Task에서 유지했다.
- acceptance: 17개 source/test 파일 삭제, 외부 importer 0, 남은 Go graph 유효. 로직 이동·대체 엔진은 추가하지 않았다.
- tests: `go list -deps ./...`, Linux/Windows `go list ./monitor`, `git diff --check`, 외부 import 검색. 단순 orphan 삭제이므로 제품 테스트는 재실행하지 않았다.

## Manifest and reverse-import proof

삭제 전 `git ls-files internal/orchestrator` manifest는 정확히 17개였다.

```text
internal/orchestrator/agent_name_test.go
internal/orchestrator/builder_packet_test.go
internal/orchestrator/fakes_test.go
internal/orchestrator/final_review_test.go
internal/orchestrator/parallel.go
internal/orchestrator/parallel_atomic_test.go
internal/orchestrator/parallel_fakes_test.go
internal/orchestrator/parallel_round4_test.go
internal/orchestrator/parallel_round5_test.go
internal/orchestrator/parallel_test.go
internal/orchestrator/ports.go
internal/orchestrator/retirement.go
internal/orchestrator/retirement_round3_test.go
internal/orchestrator/retirement_test.go
internal/orchestrator/review_round1_test.go
internal/orchestrator/single.go
internal/orchestrator/single_test.go
```

삭제 전 `go list -deps ./...` 결과에서 `github.com/Middleages/thread-dock/internal/orchestrator`가
외부 package dependency로 나타나지 않았고, Go source 전체의
`internal/orchestrator|orchestrator/` 외부 importer 검색도 0건이었다. 따라서 이 package는
Task 1 이후 production/test reverse importer가 없는 orphan이었다. 삭제 후 dependency graph에는
`thread-dock/internal/orchestrator`가 없고 남은 graph가 생성됐다.

## Implementation and self-review

`apply_patch`로 위 manifest의 17개 파일만 삭제했다. 로직 이동·대체 실행기·public/shared
interface 변경은 없으며, Monitor/settings/GHES/Toolbox/runner/template/CLI/Makefile 및
module 파일은 변경하지 않았다. `git diff --stat`는 17 files changed, 9448 deletions였고,
삭제 commit 직전 `git diff --name-only`의 모든 경로가 `internal/orchestrator/` 아래였다.

## Verification

모든 Go 명령은 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go`로 실행했다.

| Command | Outcome |
|---|---|
| `go list -deps ./...` | PASS; 228개 dependency 목록 생성, `thread-dock/internal/orchestrator` absent |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go list ./monitor` | PASS; `thread-dock/monitor` |
| Linux `go list ./monitor` | PASS; `thread-dock/monitor` |
| `git diff --check` | PASS |
| `rg -n --glob '*.go' 'internal/orchestrator\|orchestrator/' --glob '!internal/orchestrator/**' .` | PASS; external importers none |
| filesystem `rg --files internal/orchestrator` | PASS; no remaining files |

제품 테스트/full suite와 `make check`는 worker 정책 및 Task brief에 따라 실행하지 않았다. 동일
SHA·환경의 통합 gate는 root의 마지막 조정된 한 번에서 수행해야 한다.

## Result

- changedFiles: 17 source/test files deleted (report artifact is separate)
- commitSHA: `c393f669ac479402af3becef30347079bb00f519`
- executedCommands: 삭제 전 manifest/importer 및 package 목록 확인; 삭제 후 Go 1.27 `go list -deps ./...`, Linux/Windows `go list ./monitor`, importer search, filesystem path search, `git diff --check`, `git diff --stat/name-only`
- outcomes: acceptance 충족; orphan package와 테스트가 제거되고 남은 dependency graph 및 Monitor package listing이 유효함
- unverified: actual runtime model/effort identity; Windows native runtime/live GHES/Herdr; 제품 테스트/full suite; 통합 `make check`
- blockers: 없음
