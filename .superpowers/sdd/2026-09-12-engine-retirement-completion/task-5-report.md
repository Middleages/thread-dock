# Task 5 report: retirement-git-adapters

## Packet

- taskId: `retirement-git-adapters`
- baseSHA: `1babc9727690393698812ff6c2e0184d1bd563b8`
- deps: Task 2·3·4 reviewed 통합 완료 (`1babc972...`)
- ownedPaths: `internal/github/`, `internal/worktree/`, 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-git-adapters`
- branch: `agent/retirement-git-adapters`
- forbiddenPaths: ownedPaths 밖 전체, 실제 Git worktree·사용자 파일·제품 Monitor·runner
- interface: 두 orphan adapter package와 전용 테스트만 제거했다. 제품 GitHub 조회·WSL 경로/링크 열기는 `monitor/` 내부에 보존했다.
- acceptance: 각 package 외부 importer 0, 실제 git/herdr cleanup 실행 0, source/test 삭제만 수행했다.
- tests: Go 1.27 `go list -deps ./...`, Linux/Windows `go list ./monitor`, 외부 importer/diff 확인. worker full suite는 실행하지 않았다.

## Importer and manifest evidence

삭제 전 `git ls-files` exact manifest는 `internal/github/` 5개와 `internal/worktree/` 7개,
총 12개 source/test 파일이었다. 삭제 전 Go package listing은
`thread-dock/internal/github`와 `thread-dock/internal/worktree`만 해당 package를
제공했고, 저장소 Go source의 exact import 검색에서 두 package의 외부 importer는
0건이었다. 두 adapter의 유일한 사용 graph가 옛 엔진 내부였으므로 대체 동작은 추가하지
않고 package와 전용 테스트를 함께 제거했다.

삭제 후 Go source exact import 검색은 다시 0건이었고, 두 owned directory에는 파일이
남지 않았다. 제품 `monitor/github.go`, `monitor/herdr.go`, `internal/runner/`와
shared dependency 파일은 변경하지 않았다. `git` worktree 조작이나 실제 Herdr
session/worktree cleanup 실행은 수행하지 않았다.

## Result

- changedFiles: 12개 삭제, 6,359줄 제거
  - `internal/github/` source/test 5개
  - `internal/worktree/` source/test 7개
- commitSHA: `e6b575596a0361d67d83c2e40c70a2ae014c8a77`
- executedCommands:
  - `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go version`
  - 삭제 전 `git ls-files internal/github internal/worktree`
  - 삭제 전 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./internal/github/... ./internal/worktree/...`
  - 삭제 전 `rg -n --glob '*.go' 'thread-dock/internal/(github|worktree)' --glob '!internal/github/**' --glob '!internal/worktree/**' .`
  - `TMPDIR=/dev/shm/retirement-git-adapters-deps /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -deps ./...`
  - `TMPDIR=/dev/shm/retirement-git-adapters-monitor-linux /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./monitor`
  - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 TMPDIR=/dev/shm/retirement-git-adapters-monitor-windows /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list ./monitor`
  - 삭제 후 `rg -n --glob '*.go' 'thread-dock/internal/(github|worktree)' --glob '!internal/github/**' --glob '!internal/worktree/**' .`
  - `find internal/github internal/worktree -type f -print`
  - `git diff --check`, `git diff --name-status`, `git diff --stat`
  - `git status --short`, `git diff --name-only`
  - `git add -- internal/github internal/worktree && git commit -m '옛 GitHub와 worktree adapter 제거'`
- outcomes:
  - Go `go1.27.0 linux/amd64` dependency graph PASS.
  - Linux `go list ./monitor` PASS: `thread-dock/monitor`.
  - Windows-target `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go list ./monitor` PASS.
  - 삭제 전·후 exact external importer scan PASS: 0건.
  - 삭제 후 owned directory file absence PASS.
  - `git diff --check` PASS; source commit diff는 정확히 12개 owned source/test 삭제.
  - 제품 Monitor/runner 및 shared dependency 변경 없음 확인.
  - 실제 Git worktree·Herdr cleanup 명령 0건.
- unverified:
  - fresh Sol Task review
  - final integrated `make check`
  - Windows native Wails build/live GitHub·Herdr execution
  - runtime model/native identity
  - deleted package tests (static-only acceptance; packages no longer exist)
- blockers: 없음

## Self-review

- [x] packet base SHA `1babc972...`와 독립 worktree/branch를 확인했다.
- [x] 삭제 전 exact manifest와 production/test external importer 0을 증명했다.
- [x] `internal/github/` 및 `internal/worktree/` source/test만 삭제했다.
- [x] 제품 `monitor/`의 GitHub 조회·Herdr 경로, `internal/runner/`, Skills,
  shared dependency, 사용자 session/worktree/runtime 데이터를 보존했다.
- [x] 대체 실행 엔진·compatibility layer·public interface·새 테스트를 추가하지 않았다.
- [x] focused 정적 검증을 통과하고 implementation commit을 기록했다.
