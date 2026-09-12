# Task 7 report: retirement-v2-foundations

## Packet

- taskId: `retirement-v2-foundations`
- baseSHA: `b6fb422bf88f53cef36e38a984532581313e1057`
- deps: Task 6 reviewed 통합 완료 (`b6fb422...`); v2 부모 경로를 공유하므로 직렬 수행
- ownedPaths: `internal/registry/`, `internal/runtime/`, `internal/state/v2/`, `internal/contract/v2/`, `testdata/contracts/v2/`; 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v2-foundations`
- branch: `agent/retirement-v2-foundations`
- forbiddenPaths: ownedPaths 밖 전체, 특히 실제 local Work store/registry와 snapshot/event/config/worktree/Herdr session 데이터
- interface: v2 Work/Contract/Invocation/Artifact 모델과 registry/runtime 코드를 제거하며 대체하지 않음
- acceptance: 외부 importer/fixture consumer 0, 소스와 전용 fixture만 삭제
- tests: Go 1.27 `go list -deps ./...`, Linux/Windows `go list ./monitor`, 외부 import/fixture/diff 검사

## Importer and fixture evidence

삭제 전 exact manifest는 33개 tracked file이었다(Go source/test 32개, 전용 JSON fixture 1개). 삭제 전 exact Go import 검색에서
`internal/registry`, `internal/runtime`, `internal/state/v2`, `internal/contract/v2`를 가져오는 파일은 모두 이번 owned subtree 안에만 있었다.
삭제 전 `go list -deps ./...`에서 module package 자체가 나타나는 것은 `./...` package discovery 때문이며, 제품 package의 역방향 importer는 0건으로 확인했다.
`internal/contract/v2` 테스트만 `testdata/contracts/v2/valid-single-repo.json`을 읽었고, owned subtree 밖의 Go fixture consumer는 0건이었다.
삭제 후 exact external import/fixture scan은 0건이다. 역사 문서의 옛 경로 언급은 이번 code/fixture consumer가 아니며 수정하지 않았다.

## Result

- changedFiles: 33개 tracked file 삭제, 10,131줄 제거
  - `internal/contract/v2/`: 5개 Go source/test
  - `internal/registry/`: 3개 Go source/test
  - `internal/runtime/`: 3개 Go source/test
  - `internal/state/v2/`: 21개 Go source/test
  - `testdata/contracts/v2/valid-single-repo.json`: 1개 fixture
- implementationSHA: `4c181b9a75d94d4f23176ff0dd9eda1d17a7e703`
- candidateSHA: `4c181b9a75d94d4f23176ff0dd9eda1d17a7e703` (fresh Task review 대상; 이 report는 report-only commit으로 뒤따름)
- executedCommands:
  - `git rev-parse HEAD`, `git status --short --branch`로 baseSHA/worktree/branch 확인
  - `git ls-files internal/registry internal/runtime internal/state/v2 internal/contract/v2 testdata/contracts/v2` exact manifest 확인
  - 삭제 전 `git grep` exact internal import/fixture scan
  - Go 1.27 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -deps ./...` 삭제 전 dependency 확인
  - Go 1.27 Linux/Windows `go list ./monitor` 삭제 전 확인
  - `apply_patch`로 owned path exact 33개 삭제
  - 삭제 후 Go 1.27 `go list -deps ./...`
  - 삭제 후 Linux `go list ./monitor`, `GOOS=windows GOARCH=amd64 go list ./monitor`
  - 삭제 후 exact external import/fixture scan, changed-path allow-list, owned manifest 확인
  - `git diff --check`
  - `git add -- internal/registry internal/runtime internal/state/v2 internal/contract/v2 testdata/contracts/v2 && git commit -m 'v2 상태와 runtime foundations 제거'`
- outcomes:
  - 삭제 후 dependency list에 `thread-dock/internal/registry`, `thread-dock/internal/runtime`, `thread-dock/internal/state/v2`, `thread-dock/internal/contract/v2` 4개 package가 없음
  - Linux `go list ./monitor` PASS: `thread-dock/monitor`
  - Windows cross-list `GOOS=windows GOARCH=amd64 go list ./monitor` PASS: `thread-dock/monitor`
  - external Go import scan PASS (0), external fixture/source scan PASS (0)
  - changed path allow-list PASS (33/33 owned), `git diff --check` PASS
  - implementation commit은 v2 source/test와 전용 fixture 삭제만 포함
  - 사용자 local Work store/registry/snapshot/event/config/worktree/Herdr session에는 접근하거나 변경하지 않음
- unverified:
  - fresh Sol Task review
  - 최종 통합 `make check`와 전체 branch review
  - Windows native Wails build/live GitHub·Herdr execution 및 runtime/native model identity
  - 삭제된 package의 기존 테스트 실행 (package 자체를 삭제했으므로 정적 graph/list 검사만 수행)
- blockers: 없음

## Self-review

- [x] packet의 exact taskId, baseSHA, deps, 독립 worktree/branch, owned/forbidden 경계를 확인했다.
- [x] 삭제 전 manifest/importer/fixture 소비자를 확인해 외부 consumer 0을 증명했다.
- [x] exact v2 source/test/fixture 33개만 삭제했다.
- [x] changedFiles를 실제 tracked file 33개(Go 32 + JSON 1), 10,131줄로 기록했다.
- [x] Monitor/runner/template/Makefile/module/shared interface와 사용자 runtime 데이터를 보존했다.
- [x] 대체 엔진, compatibility layer, public interface, 새 회귀 테스트를 추가하지 않았다.
- [x] Go 1.27 dependency 및 Linux/Windows monitor list, import/fixture/diff focused 검증을 통과했다.
- [x] fresh review와 final integrated gate는 unverified로 남겼고 통과로 꾸미지 않았다.
