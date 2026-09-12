# Task 8 report: retirement-orphan-utilities

## Packet

- taskId: `retirement-orphan-utilities`
- baseSHA: `4a94053cbce8edee8e4a74dc866f8324578b6a7d`
- deps: Task 7 reviewed 통합 완료 (`4a94053...`); v2 foundation 제거 후 orphan utility를 직렬 수행
- ownedPaths: `internal/dag/`, `internal/pathscope/`, `internal/opencodeagent/`; 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-orphan-utilities`
- branch: `agent/retirement-orphan-utilities`
- forbiddenPaths: ownedPaths 밖 전체, 특히 `internal/runner/`, `internal/projecttemplate/`, 제품 `monitor/`, `project-template/`, `Makefile`, `go.mod`, `go.sum`, 사용자 runtime/data
- interface: orphan utility package를 삭제하며 대체 구현·public interface·dependency/module 변경은 추가하지 않는다. `internal/runner`는 Monitor process boundary, `internal/projecttemplate`은 template 검증 seam으로 보존한다.
- acceptance: 삭제 후 Go package는 `monitor`, `internal/runner`, `internal/projecttemplate` 세 개만 남긴다.
- tests: Go 1.27 `go list -deps ./...`, `go test ./internal/runner`, `go list ./...`, Windows `GOOS=windows GOARCH=amd64 go list ./monitor`, importer/ref/diff 검사

## Importer and manifest evidence

삭제 전 exact tracked manifest는 6개였다(Go source 3개, 전용 test 3개).

```text
internal/dag/graph.go
internal/dag/graph_test.go
internal/opencodeagent/name.go
internal/opencodeagent/name_test.go
internal/pathscope/scope.go
internal/pathscope/scope_test.go
```

삭제 전 `git grep` exact Go import 검색에서 세 package를 가져오는 owned subtree 밖 파일은 0건이었다. 저장소 전체의 동일 경로 textual reference도 계획 문서의 ownedPaths 언급 1건 외에는 0건이었다. 삭제 전 Go 1.27 `go list -deps ./...`에서 세 package가 dependency graph에 나타났고, 삭제 후에는 모두 사라졌다.

## Result

- changedFiles: 6개 tracked file 삭제, 423줄 제거
  - `internal/dag/`: 2개
  - `internal/opencodeagent/`: 2개
  - `internal/pathscope/`: 2개
- implementationSHA: `896c3dfb825ef2ec5e6e185c4145608bd795b1ce`
- candidateSHA: `896c3dfb825ef2ec5e6e185c4145608bd795b1ce` (fresh Task review 대상; 이 report는 뒤따르는 report-only commit에 포함)
- executedCommands:
  - `git rev-parse HEAD`, `git status --short --branch`로 baseSHA/worktree/branch 확인
  - `git ls-files internal/dag internal/pathscope internal/opencodeagent` exact manifest 확인
  - 삭제 전 `git grep` exact internal import 및 전체 tracked reference scan
  - Go 1.27 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -deps ./...` 삭제 전 dependency 확인
  - `apply_patch`로 위 exact 6개 파일 삭제
  - `TMPDIR=/dev/shm/threaddock-task8`에서 Go 1.27 `go list -deps ./...`
  - 같은 고유 tmpfs에서 `go test ./internal/runner`
  - 같은 고유 tmpfs에서 `go list ./...` 및 exact expected package-set 비교
  - 같은 고유 tmpfs에서 `GOOS=windows GOARCH=amd64 go list ./monitor`
  - 삭제 후 external Go import/ref scan, changed-path allow-list, `git diff --check`
  - `git add -- internal/dag internal/pathscope internal/opencodeagent && git commit -m 'orphan utility package 제거'`
- outcomes:
  - 삭제 후 `go list -deps ./...`에 `internal/dag`, `internal/pathscope`, `internal/opencodeagent`가 없음
  - `go test ./internal/runner` PASS (`ok thread-dock/internal/runner 0.119s`)
  - `go list ./...` exact PASS: `thread-dock/internal/projecttemplate`, `thread-dock/internal/runner`, `thread-dock/monitor`
  - Windows cross-list PASS: `thread-dock/monitor`
  - 삭제 후 external Go import/ref scan PASS (0)
  - changed path allow-list PASS (6/6 owned), forbidden path check PASS, `git diff --check` PASS
  - Go 1.27.0 Linux/amd64 환경 확인
  - Monitor, runner, projecttemplate, template, Makefile, go.mod/go.sum, 사용자 runtime/data는 변경하지 않음
- unverified:
  - fresh Sol Task review
  - 최종 통합 `make check`와 전체 branch review
  - Monitor/projecttemplate 테스트 및 Windows native Wails build/live GitHub·Herdr execution
  - 실제 runtime/native model identity
- blockers: 없음

## Self-review

- [x] packet의 exact taskId, baseSHA, deps, 독립 worktree/branch, owned/forbidden 경계를 확인했다.
- [x] 삭제 전 manifest와 외부 importer 0을 확인하고, exact owned 6개만 삭제했다.
- [x] dependency/module 변경, 새 구현, compatibility layer, public interface 변경을 추가하지 않았다.
- [x] `internal/runner` focused test와 Linux/Windows Monitor graph를 통과시켰다.
- [x] 삭제 후 남은 Go package가 Monitor, runner, projecttemplate 세 package뿐임을 확인했다.
- [x] changedFiles를 실제 tracked file 6개, 423줄 제거로 기록했다.
- [x] fresh review와 final integrated gate/native 검증은 unverified로 남겼고 통과로 꾸미지 않았다.
