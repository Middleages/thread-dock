# Task 1 report: retirement-root-cut

## Packet

- taskId: `retirement-root-cut`
- brief baseSHA: `38a505a`
- actual worktree baseSHA: `91d37632723ffca8114700498e43efcdba7f0ec8`
- deps: Linux/Windows inventory에서 Monitor의 유일 internal dependency가 `internal/runner`임을 확인했고, CLI root cut 직렬 interface를 사용했다.
- worktree: `/home/appuser/dev_system/.worktrees/retirement-root-cut`
- branch: `agent/retirement-root-cut`
- implementation commitSHA: `07909fcacd446ba51707b131236c3a84ad2340ca`

## Scope and caller evidence

삭제 대상은 `cmd/agentctl/`, `internal/cli/`, `internal/config/`, `internal/pilot/`,
`scripts/single-run-pilot.sh`의 tracked source/test와 Makefile의 해당 진입점 검사다.

삭제 전 Go 1.27 importer inventory에서 `cmd/agentctl`이 `internal/cli`·`internal/config`를
직접 import했고, `internal/pilot`이 `internal/config`와 `single-run-pilot.sh`를 직접 검사했다.
production/test importer는 위 legacy 군집 내부에만 있었다. Windows `GOOS=windows
GOARCH=amd64 CGO_ENABLED=0 go list`에서도 같은 root graph만 확인됐다.

재현한 inventory 명령은 다음과 같다.

```sh
/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -f '{{.ImportPath}} {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./...
rg -n --glob '*.go' 'thread-dock/(cmd/agentctl|internal/(cli|config|pilot))|agentctl|single-run-pilot' cmd internal monitor
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./... | rg 'thread-dock/(cmd/agentctl|internal/(cli|config|pilot))|internal/(cli|config|pilot)'
```

삭제 후 active-reference 확인은 다음 명령으로 재현했다.

```sh
rg -n --hidden --glob '!.git' '(cmd/agentctl|internal/(cli|config|pilot)|single-run-pilot\.sh)' cmd internal monitor Makefile
```

현재 Go source와 Makefile의 active reference 검색 결과는 0건이다. docs의 역사적
`agentctl`/pilot 설명은 packet의 forbiddenPaths에 해당하므로 이번 Task에서 수정하지 않았다.
Monitor 및 `internal/runner`, `project-template`, `go.mod`, `go.sum`은 변경하지 않았다.

정확한 deleted manifest는 23개다.

- `cmd/agentctl`: 3개
- `internal/cli`: 14개
- `internal/config`: 2개
- `internal/pilot`: 3개
- `scripts/single-run-pilot.sh`: 1개

Makefile은 1개 수정했다. Go discovery를 `find internal monitor`로 좁히고, `test`/`check`의
pilot `bash -n` 호출을 제거했으며, focused 명령 도움말 예시를 `./monitor`와
`./internal/runner`로 바꿨다. 삭제된 전용 동작에 대체 테스트를 추가하지 않았다.

## Verification

모든 Go 명령은 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go`
(`go1.27.0 linux/amd64`)와 `/dev/shm` 아래 별도 `TMPDIR`에서 실행했다.

| Command | Outcome |
|---|---|
| `go list ./...` | PASS; legacy package가 목록에서 사라지고 Monitor/runner 및 남은 패키지 목록 생성 |
| `go test ./monitor ./internal/projecttemplate` | PASS |
| `go vet ./monitor ./internal/runner` | PASS |
| `make template-check` | PASS |
| `make -n check` | PASS; 사라진 `cmd`/pilot shell check를 요구하지 않음 |
| `gofmt -l $(find internal monitor -name '*.go' -print)` (Go 1.27 절대경로) | PASS; 출력 없음 |
| `rg` active source/Makefile refs for `cmd/agentctl`, `internal/(cli\|config\|pilot)`, `single-run-pilot.sh` | PASS; 0건 |
| `git diff --check` | PASS |

초기 일반 PATH `gofmt` 호출은 시스템에 `gofmt`가 없어 신뢰 가능한 검증으로 취급하지
않았고, 위 Go 1.27 절대경로 호출로 재검증했다. `make check`/full Go suite/frontend
suite는 worker 정책상 실행하지 않았다. 통합 PR 마지막 gate에서 수행한다.

## Self-review

- CLI binary와 legacy configuration/pilot entrypoint 및 전용 테스트는 모두 삭제됐다.
- 삭제 후 `go list`가 성공하고 production/test source importer가 0이다.
- Makefile은 살아 있는 Monitor/runner 중심 discovery와 focused 예시를 유지한다.
- Monitor/settings/GHES/Toolbox/runner/project-template와 shared module/interface는 보존됐다.
- 변경 범위는 packet의 ownedPaths에만 있다.

## Result

- changedFiles: 24 (23 deleted, 1 Makefile modified)
- commitSHA: `07909fcacd446ba51707b131236c3a84ad2340ca`
- executedCommands: 위 Verification 표의 Go 1.27/tmpfs 명령, `make template-check`,
  `make -n check`, explicit `gofmt`, active-reference `rg`, `git diff --check`
- outcomes: 모든 focused 검증 PASS
- unverified: actual runtime model/effort identity; Windows native runtime 및 native
  degraded/error 상태; 통합 `make check`
- blockers: 없음
