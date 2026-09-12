# Task 4 report: retirement-v2-adapters

## Packet

- taskId: `retirement-v2-adapters`
- baseSHA: `4932988b3dbebcd843a9577ceecbe5d22b17ea07`
- deps: Task 3 reviewed 통합 완료 (`4932988…`)
- ownedPaths: `internal/herdr/`, `internal/coordinator/`, 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v2-adapters`
- branch: `agent/retirement-v2-adapters`
- forbiddenPaths: ownedPaths 밖 전체, 특히 제품 `monitor/herdr.go`, `internal/runner/`,
  `project-template/`, shared dependency files, 사용자 runtime/session/worktree/dirty 실험
- interface: Go runtime coordinator와 옛 Herdr 실행 adapter를 제거했다. 제품 Monitor의
  `monitor/herdr.go`와 Skills의 Herdr 소유권은 보존했다.
- acceptance: coordinator의 유일 외부 consumer가 같은 Task의 `internal/herdr`임을
  증명하고 두 package 및 전용 fixture를 제거했다. 실제 session/worktree를 종료·삭제하지
  않았다.
- tests: Go 1.27 `go list -deps ./...`, Linux/Windows `go list ./monitor`, 외부
  importer/diff 검사. worker full suite 및 제품 테스트는 실행하지 않았다.

## Importer and manifest evidence

삭제 전 `git ls-files` manifest는 coordinator source/test 15개, Herdr source/test 5개,
`internal/herdr/testdata/v0.8.2/` fixture 14개였다. Go source exact-import 검색과
Go dependency import listing에서 `internal/coordinator`의 유일한 외부 Go consumer는
같은 Task의 `internal/herdr/runtime.go` 및 `runtime_test.go`였고, 다른 production/test
package는 두 package를 import하지 않았다. Herdr는 coordinator의 실행 adapter였으므로
두 package와 14개 fixture를 함께 제거했다.

삭제 후에는 Go source 외부 importer 0건, Go package listing에서
`thread-dock/internal/coordinator`와 `thread-dock/internal/herdr` 부재, 두 owned
directory의 파일 부재를 확인했다. 문서의 과거 Herdr/coordinator 언급은 ownedPaths 밖이며
역사 문서 보존 범위이므로 수정하지 않았다.

## Result

- changedFiles: 34개 삭제, 9,122줄 제거
  - `internal/coordinator/` source/test 15개
  - `internal/herdr/` source/test 5개
  - `internal/herdr/testdata/v0.8.2/` fixture 14개
  - 제품 `monitor/`와 shared dependency 파일 변경 없음
- commitSHA: `87d4b815f9d3b489925437e847c90469cc333639`
- executedCommands:
  - `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go version`
  - `TMPDIR=/dev/shm/retirement-v2-adapters-deps.<pid> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -deps ./...`
  - `TMPDIR=/dev/shm/retirement-v2-adapters-monitor-linux.<pid> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./monitor`
  - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 TMPDIR=/dev/shm/retirement-v2-adapters-monitor-windows.<pid> /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./monitor`
  - `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./...`
  - `rg` exact external Go importer scan; `find` owned-file absence check
  - `git diff --name-status`, owned-path exclusion check, `git diff --check`, `git diff --stat`
  - `git add -- internal/coordinator internal/herdr && git commit -m '옛 coordinator와 Herdr 실행 adapter 제거'`
- outcomes:
  - Go `go1.27.0 linux/amd64` dependency graph PASS.
  - Linux `go list ./monitor` PASS: `thread-dock/monitor`.
  - Windows-target `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go list ./monitor` PASS.
  - post-delete Go package listing and external importer scan PASS; both owned directories
    have no files on disk.
  - diff is exactly 34 deletions under ownedPaths; `git diff --check` PASS.
  - session/worktree/runtime data were not inspected for mutation or deleted.
- unverified:
  - fresh Sol Task review
  - final integrated `make check`
  - Windows native Wails build/live Herdr execution
  - runtime model/native identity
  - deleted package tests (static-only acceptance; package no longer exists)
- blockers: 없음

## Self-review

- [x] Task3 base SHA와 packet 경계를 확인했다.
- [x] coordinator 외부 consumer가 같은 Task의 Herdr뿐임을 production/test exact import와
  Go dependency graph로 증명했다.
- [x] coordinator/Herdr source·test 및 v0.8.2 fixture만 삭제했다.
- [x] 제품 `monitor/herdr.go`, `internal/runner/`, Skills, shared dependency와 사용자
  session/worktree/dirty runtime 실험을 보존했다.
- [x] 대체 실행 엔진, compatibility layer, 새 테스트·public interface를 추가하지 않았다.
- [x] focused 정적 검증을 통과하고 implementation commit을 기록했다.
