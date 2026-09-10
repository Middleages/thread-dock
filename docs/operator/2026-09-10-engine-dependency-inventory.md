# 옛 실행 엔진 의존성 inventory

기준 SHA: `3f4bebf08bb099637d1b2bfff44a9ba56e91a5df`.
목적은 [ADR 0008](../adr/0008-github-first-skills-before-engine.md)의 새 Monitor 경계를 보존하면서
옛 엔진을 작은 단위로 정리하는 것이다. 아래 경로가 Monitor에서 쓰이지 않는다는 사실만으로
기존 CLI의 호출까지 없다고 판단하지 않는다.

## 유지할 현재 Monitor 경계

`monitor/main.go`는 `internal/runner.OSRunner{}`를 `NewGitHubMonitor`와 `NewHerdrMonitor`에 주입한다.
현재 `thread-dock/monitor`의 유일한 repository internal import는 `internal/runner`다.

| 경로 | 실제 소비 기능 |
|---|---|
| `internal/runner/runner.go`, `runner_test.go` | 프로세스 실행·timeout 경계. 현재 Monitor와 legacy 양쪽이 사용하므로 보존 |
| `monitor/snapshot.go` | `SnapshotSource`, `CommandRunner`, 화면용 wire/Link. shared interface 보존 |
| `monitor/commands.go` | 출력 한도·timeout을 적용하는 명령 경계 |
| `monitor/github.go` | WSL gh 조회와 독립 cache·성공 시각·오류 |
| `monitor/herdr.go` | 실제 Herdr 관찰, 연결 파일, session/pane/name, canonical cwd/worktree 대조, 위치 링크와 handoff |
| `monitor/app.go`, `main.go` | `GetMonitorSnapshot`, Wails wiring·native clipboard·외부 링크 경계 |
| `monitor/frontend/src/{types.ts,bindings.ts,App.tsx}` 및 테스트 | Wails wire, 업무와 관찰 상태 분리, 링크·위치 안내·복사 |

`monitor/herdr.go`, `monitor/github.go`와 `internal/herdr`, `internal/github`는 다른 구현이다.
이름이 같다는 이유로 current 파일을 old-engine 삭제에 포함하지 않는다.
현재 Monitor는 `agentctl`, `internal/config`, coordinator/scheduler/runtime/recovery/state,
`internal/monitor`, `internal/monitorcli`, orchestrator/herdr/github package를 import하지 않는다.

## 삭제 전 호출 관계

| 경로 | 역할과 직접 소비자 | 삭제 조건 |
|---|---|---|
| `cmd/agentctl/{main.go,main_test.go,publication.go}` | v1 start/status/stop/resume/cleanup/retire/confirm/create-revert와 v2 project/work 명령의 composition root. cli/config/contract/coordinator/githubpublication/herdr/orchestrator/state/workflow/workrun 결합 | v1/v2 CLI 범위를 나눠 사용법·pilot·config 테스트까지 함께 정리. 이번 slice에서 보존 |
| `internal/coordinator/` (16 Go 파일) | v2 owner lease·dispatcher·runtime·candidate/reviewer ingestion·publication reconcile. cmd/agentctl, workflow, workrun, internal/herdr runtime, githubpublication이 소비 | runtime/state-v2/shared interface와 호출자 먼저 직렬 정리 |
| `internal/scheduler/` (2 Go 파일) | v1 DAG의 두 Builder 선택. production importer는 `internal/orchestrator/parallel.go` | orchestrator 호출이 살아 있으므로 단독 삭제 불가 |
| `internal/recovery/` (2 Go 파일) | wait/continue/resume_session/ask_operator/block 정책. production importer는 `internal/orchestrator/parallel.go` | v1 실행 경로와 함께 제거; Monitor 재개 안내로 옮겨 구현하지 않음 |
| `internal/runtime/` (3 Go 파일) | Invocation/ArtifactEnvelope/BuilderResult/ReviewerResult codec. coordinator·internal/herdr runtime, workrun 테스트가 소비 | v2 호출자 정리와 직렬 수행 |
| `internal/publication` | 디렉터리 없음 | 아래 분산된 실제 발행 경로를 기준으로 추적 |
| `internal/state/` (v1, 4 Go 파일) | RunSnapshot/event/local store. cli/orchestrator/review/retirement/integration/cmd가 소비 | v1 CLI·config·fixture 범위와 함께 정리 |
| `internal/state/v2/` (21 Go 파일) | WorkSnapshot/Task invocation/evidence/budget/publication/store/reducer. coordinator/workflow/workrun/herdr runtime/githubpublication/cmd가 소비 | v2 route·공유 타입과 직렬 정리 |
| `internal/monitor/` | 옛 Work 상태 projection. current `monitor/`와 별개 | workflow/CLI 및 전용 테스트 소비 확인 후 후속 삭제 |
| `internal/monitorcli/{client.go,client_test.go}` | WSL `agentctl project status --all --json` bridge, Go importer 0개·자체 테스트만 존재 | 첫 slice로 두 파일 제거. current runner/Monitor 변화 없음 |

v1과 v2 state는 서로 직접 import하지 않는 별도 aggregate지만 agentctl이 양쪽을 조립한다.
v1은 `cli → orchestrator → scheduler/recovery/state`와 review/retirement/integration/contract/github/herdr/worktree에 연결된다.
v2는 `cli project_work → workflow/workrun → coordinator → runtime/state-v2`와
herdr runtime/githubpublication/registry/contract-v2/internal monitor에 연결된다.

### 분산된 발행 경로

`cmd/agentctl/publication.go`, `internal/coordinator/publication.go`와 dispatcher/reconcile,
`internal/githubpublication/{parent_issue.go,parent_issue_test.go}`,
`internal/state/v2/{publication_transition.go,publication_transition_test.go,publication_round5_test.go}`,
workflow projection과 CLI `work publish-issues`가 실제 추적 대상이다.
이 기능만 먼저 제거해도 여러 패키지·CLI 계약이 바뀌므로 이번 두 파일 slice에 섞지 않는다.

## 설정·테스트·fixture·문서

| 범위 | 확인한 의존성 | 첫 slice 처리 |
|---|---|---|
| `internal/config/{config.go,config_test.go}` | agentctl/pilot 설정. GHESHost/APIBase/StateDir/HerdrBinary/GitBinary/WorkingWait/RecoveryLimit/ProjectAutomationEnabled/AutoRetireCompletedSessions/HerdrWorktreeRoot/Project IDs/OpenCodeAgents/BuilderRuntimeFingerprint가 옛 composition에 연결 | 보존. Monitor는 별도 THREADDOCK_REPOS/PROJECTS/WSL_DISTRIBUTION/SESSIONS_FILE 사용 |
| `testdata/contracts/{valid.json,invalid-overlap.json,v2/valid-single-repo.json}` | contract와 옛 orchestration fixture | 보존. Monitor fixture라고 간주하지 않음 |
| `scripts/single-run-pilot.sh`, `internal/pilot` | agentctl lifecycle과 operator/config 근거를 검사 | 보존. 후속 CLI 삭제 시 함께 귀속 |
| `Makefile` | `find cmd internal monitor`로 Go 파일 발견, test/check에서 pilot shell syntax 검사 | package 삭제가 자동 반영되므로 변경 없음; 남은 suite 숨기지 않음 |
| `docs/operator/{foundation-pilot,parallel-pilot,single-run-pilot,session-retirement,opencode-role-agents}.md`, README 링크 | 옛 운영과 pilot 테스트의 문서 참조 | 이번에는 보존. 후속 삭제 시 링크·테스트와 함께 정리 |
| 2026-09-07/08/09 설계·계획, ADR 0002 | 과거 실행 엔진의 이력 | 역사 기록으로 보존, 신규 구현 지침 아님 |
| 현재 2026-09-10 설계·계획 | monitorcli를 경계 참고 대상으로 언급 | current runner 직접 사용과 bridge 제거 상태로 정정 |
| `go.mod`, `go.sum` | runner/Monitor 및 남은 engine 공용 dependency | 변경 없음 |

## 첫 slice의 증거와 검증

읽기 전용 Sol 조사는 필수 문서, `rg` 참조, Go 1.27.0 `go list`의 Imports/TestImports/XTestImports를 확인했다.
재현 가능한 확인 명령:

```sh
rg -n 'thread-dock/internal/monitorcli' --glob '*.go' .
go list -f '{{.ImportPath}} {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./...
rg -n 'monitorcli|agentctl project status' monitor internal cmd docs Makefile
```

첫 명령의 exit 1은 importer가 없다는 뜻이다. 문서의 역사 참조는 Go importer가 아니다.
삭제 전 `internal/monitorcli/client.go`는 WSL에서 옛 agentctl aggregate를 읽었고,
현재 `monitor/main.go`는 runner를 직접 주입하므로 adapter 제거가 실행 동작을 바꾸지 않는다.
새 테스트로 삭제 자체를 고정하지 않고 기존 `go test ./monitor`, `go vet ./monitor`로 유지할 경로를 검증한다.
고정 SHA와 명령 결과는 [ledger](2026-09-10-engine-retirement-ledger.md)에 기록한다.

Windows build-tag 그래프도 조사했다. 다음 명령은 exit 0이며 monitorcli 자체 행 외 이를 import하는
package 행은 0개였다. Windows Monitor의 내부 의존성도 runner뿐이다. 이는 native 실행 검증이 아니다.

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 /home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./... | rg 'thread-dock/(monitor|internal/monitorcli)|internal/monitorcli'
```

조사 result: changedFiles `[]`, commitSHA 없음, executedCommands는 위 참조/그래프 검사와 필수 문서 읽기,
outcomes는 Linux·Windows importer 0과 두 legacy 군집 확인, unverified는 runtime model/effort와 native 오류 상태,
blockers는 없음이다. 이 문서 작성은 root 통합 문서 Task에 속한다.

## 후속 순서와 제한

1. importer 없는 bridge를 제거한다. public/shared interface 변화 없음.
2. v1 route 하나와 연결된 orchestrator 동작·config·pilot 문서를 함께 제거할 수 있는지 조사한다.
3. v2 publication/runtime/coordinator/state 정리는 shared interface와 CLI 계약부터 직렬로 계획한다.

이 inventory는 engine 전체 삭제 완료가 아니다. scheduler/recovery를 orchestrator에 인라인해 코드만
이동시키거나 Monitor 안에 새 실행 엔진을 만드는 방식은 채택하지 않는다.
사용자 실험·기존 worktree·Windows staging은 inventory 삭제 대상에 포함하지 않는다.
runtime model/effort identity와 이번 SHA의 Windows native 실행은 unverified다.
