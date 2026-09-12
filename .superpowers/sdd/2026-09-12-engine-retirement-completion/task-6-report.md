# Task 6 report: retirement-v1-foundations

## Packet

- taskId: `retirement-v1-foundations`
- baseSHA: `dc716bea082c1be2503f05335d2f51da0f5da22e`
- deps: Task 4·5 reviewed 통합 완료 (`dc716bea...`); Task 7과 shared 부모 디렉터리가 있어 직렬 수행
- ownedPaths: `internal/mergegate/`, `internal/recovery/`, `internal/retirement/`, `internal/review/`, `internal/scheduler/`, v1 `internal/state/`·`internal/contract/` 파일, `internal/testfixture/`, `internal/version/`, `testdata/contracts/valid.json`, `testdata/contracts/invalid-overlap.json`, 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v1-foundations`
- branch: `agent/retirement-v1-foundations`
- forbiddenPaths: ownedPaths 밖 전체, 특히 `internal/state/v2/`, `internal/contract/v2/`, `testdata/contracts/v2/`, 사용자 snapshot/event/config 파일
- interface: v1 foundations만 제거하고 v2 부모 경로 전체와 제품 runtime 데이터를 보존
- acceptance: 폐기 package/fixture의 외부 소비자 0, v2 graph 유지, 사용자 runtime 데이터 변경 없음
- tests: Go 1.27 `go list -deps ./...`, 고유 tmpfs `go test ./internal/state/v2 ./internal/contract/v2`, manifest/import/fixture/diff 검사

## Importer and fixture evidence

삭제 전 exact manifest는 v1 source/test 26개였다. 삭제 전 Go dependency listing에서 v1
package는 `internal/mergegate`, `internal/recovery`, `internal/retirement`,
`internal/review`, `internal/scheduler`, `internal/state`, `internal/contract`,
`internal/testfixture`, `internal/version`으로만 나타났다. exact Go import 검색에서
v1 package의 소비자는 서로의 owned source/test와 fixture뿐이었고, 제품 `monitor/`,
`internal/registry/`, `internal/runtime/`은 v2 package만 소비했다.

삭제 후 exact v1 import/fixture 경로 검색은 0건이다. `internal/state/v2/` 26개,
`internal/contract/v2/` 5개, `testdata/contracts/v2/valid-single-repo.json`은
그대로 남아 v2 graph와 fixture 경계를 보존한다. 실제 사용자 snapshot/event/config,
worktree 및 Herdr session에는 접근하거나 변경하지 않았다.

## Result

- changedFiles: 26개 삭제, 4,169줄 제거
  - `internal/contract/` v1 source/test 7개
  - `internal/mergegate/` 2개
  - `internal/recovery/` 2개
  - `internal/retirement/` 2개
  - `internal/review/` 2개
  - `internal/scheduler/` 2개
  - `internal/state/` v1 source/test 4개
  - `internal/testfixture/contract.go`, `internal/version/` source/test 3개
  - `testdata/contracts/valid.json`, `testdata/contracts/invalid-overlap.json` 2개
- implementation commitSHA: `b473239707068dba0b26add0bd7c10dbd6567f68`
- executedCommands:
  - `git rev-parse HEAD`, `git status --short --branch`로 baseSHA/worktree/branch 확인
  - 삭제 전 `git ls-files` exact manifest 및 `git grep` v1 importer/fixture scan
  - 삭제 전 Go 1.27 `go list -deps ./...` (`/dev/shm/threaddock-task6-before.vvpoKK`, 133 deps)
  - exact owned path 삭제 후 `git diff --check`, changed path allow-list, v2 file manifest 검사
  - 삭제 후 Go 1.27 `TMPDIR=/dev/shm/threaddock-task6-list.sHXXB9 go list -deps ./...` (124 deps)
  - 삭제 후 v1 exact graph/import assertion 및 v2 subtree preservation 검사
  - `TMPDIR=/dev/shm/threaddock-task6-test2.amB4p1 /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go test ./internal/state/v2 ./internal/contract/v2`
  - `git add` exact owned paths 및 `git commit -m 'v1 상태와 계약 foundations 제거'`
- outcomes:
  - Go `go1.27.0 linux/amd64` 삭제 후 dependency manifest PASS; deleted v1 package graph 0건
  - v1 exact production/test import 및 fixture reference scan PASS: 0건
  - v2 focused tests PASS: `internal/state/v2` 0.390s, `internal/contract/v2` 0.004s
  - v2 source/test/fixture manifest preservation PASS
  - `git diff --check` 및 exact 26-file owned path allow-list PASS
  - implementation commit은 v1 owned source/test/fixture 삭제만 포함
- unverified:
  - fresh Sol Task review
  - final integrated `make check`
  - Windows native Wails build/live GitHub·Herdr execution 및 runtime model/native identity
  - 삭제된 package의 기존 테스트 실행 (package가 제거되어 focused v2 테스트와 정적 graph만 실행)
- blockers: 없음

## Self-review

- [x] packet의 baseSHA, deps, 독립 worktree/branch 및 owned/forbidden 경계를 확인했다.
- [x] 삭제 전 manifest/importer/fixture 소비자를 확인해 v1 외부 소비자 0을 증명했다.
- [x] exact v1 source/test/fixture 26개만 삭제했다.
- [x] `internal/state/v2/`, `internal/contract/v2/`, `testdata/contracts/v2/`와 사용자 runtime 데이터를 보존했다.
- [x] 대체 엔진, compatibility layer, public interface, 새 회귀 테스트를 추가하지 않았다.
- [x] focused Go 1.27 검사와 diff/path 검사를 통과하고 implementation commit을 기록했다.
