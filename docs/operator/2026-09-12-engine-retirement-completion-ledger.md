# SDD ledger — plan: docs/superpowers/plans/2026-09-12-engine-retirement-completion.md

## 착수

- main `c487e8c` → `2db8f776d22afd849fcb9a1328f7e8aff9b72e50` fast-forward, clean. 사용자 추가 PR #85/86/87와 compact 설계 보존.
- 독립 통합 worktree `.worktrees/engine-retirement-completion`, branch `agent/engine-retirement-completion`.
- gh auth project scope, Project #1 조회/쓰기 성공. 열린 PR 0. #42/44/45/46/47을 not planned로 종료하고 #43/48을 실사용·locator 검증으로 한국어 재작성했다. 종료는 구현 완료가 아니다.
- 기존 codex-runtime dirty 4개 파일 보존, 다른 worktree 및 Windows staging 삭제 없음.
- `38a505a`에서 stale 문서/Issue 방향 기록을 commit했다.

## 직렬 interface 결정과 사전 점검

전체 제거 방향은 ADR 0008과 사용자 요청으로 승인됐다. 새 엔진/대체 CLI 없이 옛 root부터 제거한다.
전체 그래프 조회에서 Linux/Windows Monitor의 repo-internal import는 runner뿐이며 settings/GHES/toolbox는 legacy package를 소비하지 않는다.

| Task 관계 | 생산/소비 및 자체 정합성 | 결정 |
|---|---|---|
| Task 1 / Monitor | CLI 제거, Monitor는 runner만 사용 | Monitor 전체 금지 경로로 보존 |
| Task 1 / 후속 internal 제거 | root caller를 먼저 끊고 이후 orphan 군집 삭제 | 직전 reviewed SHA에서 순차 배정 |
| Task 1 / root docs | 코드·Makefile 대 문서 | ownedPaths 분리 |
| Task 1 자체 | 삭제 경로와 남은 패키지 focused 검사 | full suite는 root 마지막 gate에만 실행 |

## 슬롯

inventory Sol 1(read-only), 구현 Luna 최대 1(현재 순차), fresh reviewer 1 예약. 모델/effort 실제 runtime identity는 unverified.

Task 1 이후 Task 2/3만 서로 다른 worktree에서 Luna 2개로 병렬화한다. 최대 3개 상한과 reviewer 1개 예약을 유지한다.

## Task 1 배정

- [Issue #89](https://github.com/Middleages/thread-dock/issues/89), Project #1 In Progress·유지로 추적한다.
- `retirement_root_cut` Luna에 실제 base `91d37632723ffca8114700498e43efcdba7f0ec8`, 독립 `.worktrees/retirement-root-cut`/`agent/retirement-root-cut`를 배정했다.
- 초기 plan의 `38a505a`는 앞 문서 commit이며 구현은 계획 commit `91d3763` 위에서 시작한다. packet에 실제 SHA로 정정했다.
- Inventory 확인: `internal/config`의 소비자는 삭제 대상 cmd/agentctl·internal/pilot뿐이다. 후속 단계의 내부 모듈은 이 첫 Task에서 건드리지 않는다.

## 검증 제한

사용자 추가 PR87의 source-level tests는 아직 실행 성공 근거가 없다. 최종 gate에서 현재 Makefile의 최신 UI 테스트 전부를 포함한다. native Windows/GHES/Herdr 및 Skill pressure scenario는 #43/#48에 남겨 코드 삭제와 구분한다.

## 후속 inventory와 문서 검증

- 전체 역의존 순서를 [최신 inventory](2026-09-12-engine-dependency-inventory.md)에 기록했다. Task 2/3는 독립, 나머지는 shared 부모/interface 때문에 순차다. runner와 projecttemplate 검증 seam만 남긴다.
- hidden tracked 파일도 조사했다. .codex coordinator의 stale 이식 지시와 GitHub Issue/PR template의 옛 gate 용어만 정리했다. 모델/effort 값과 template 입력 필드는 보존했다.
- Python tomllib/PyYAML parse: PASS, model gpt-5.6-sol/medium 및 Issue 8 fields 보존. Markdown 19파일/58 local links와 diff 검사 PASS(이후 새 문서는 최종 재검사).
- 첫 Task 구현 `07909fc`/report `0b3625e`에서 legacy 23파일 삭제·Makefile 1개 수정, focused Go Monitor/projecttemplate/vet/template-check/dry-run/refs/gofmt/diff PASS. fresh Task 리뷰 진행 중.
- 최종 gate용 tmpfs checkout `/dev/shm/threaddock-retirement-final.KMxdTI/checkout`를 분리했다. package.json/lock은 c487e8c와 최신 main 사이 동일함을 확인하고 이전 동일 lock의 node_modules를 복사했다. Node 26.8.1/npm11.19.0. 테스트는 아직 실행하지 않았다.

## Task 1 완료 · Task 2/3 배정

Task 1: complete (`07909fc` + report `0b3625e`, fresh `retirement_task1_review` spec/quality ACCEPT, blocking 없음). 통합은 `68a86a5`/`ce3c9ba`다. legacy 23파일과 Makefile만 변경, Monitor/runner/템플릿/모듈 불변을 독립 확인했다.

- Task 2/3 exact base: `ce3c9ba45c1d5b33f951095afa542999847c0063`.
- Task 2: `retirement_v1_root`, `.worktrees/retirement-v1-root`, `agent/retirement-v1-root`.
- Task 3: `retirement_v2_entries`, `.worktrees/retirement-v2-entry-roots`, `agent/retirement-v2-entry-roots`.
- 나머지 packet/interface/acceptance/tests는 계획과 추출된 각 brief를 따른다. report는 각 Task 이름의 report 파일에 남긴다.

| Task 관계 | 사전 정합성·공유 경로 검사 |
|---|---|
| 2 / 3 | orchestrator 대 v2 roots+coordinator 단일 test, 파일·interface 겹침 없음 |
| 3 / 4 | coordinator integration_test 제거 후 잔여 coordinator/herdr 전체 제거, 순차 |
| 2·3·4 / 5 | GitHub/worktree 역의존 제거 후 adapter 제거, 순차 |
| 5 / 6·7 | adapter 소비자를 먼저 제거, v1/v2 parent 경로 충돌은 직렬 |
| 6 / 7 | exact v1 root 파일만 먼저 삭제하므로 v2 subtree 보존 |
| 7 / 8 | foundation 소비자 제거 후 orphan utility 삭제 |
| 2~8 / root docs | worker report는 각자 분리, root 문서와 제품 소유 겹침 없음 |
| 2·4·5·7 자체 | orphan source 삭제와 정적 graph 검사 일치, 대체 동작·테스트 추가 없음 |
| 3·6·8 자체 | 남은 직접 영향 coordinator/v2/runner focused 검사만 실행 |

## Task 2 완료

Task 2: complete (구현 `c393f66`, report `fa4b5b7`, fresh `retirement_task2_review` spec/quality ACCEPT). 통합 `b901b4d`/`fb1ac52`. orphan orchestrator 17파일/9448줄 삭제, 외부 production/test importer 0, 정적 Go graph/Linux·Windows Monitor listing 통과. 제품 테스트는 재실행하지 않았다.
Report 설명 중 GitHub 저장소 URL형 import 언급은 module의 실제 import path가 아니다. 실제 module과 reviewer의 base/head 검사는 `thread-dock/internal/orchestrator`를 대상으로 했으며 부재를 확인했다.

## Task 3 완료 · Task 4 배정

Task 3: complete (구현 `f170b7a`, report `bda6512`, fresh `retirement_task3_review` spec/quality ACCEPT, findings 없음). 통합 `7d77cca`/`4932988`. legacy 16 source/test 파일/6649줄 삭제, 남은 coordinator focused test PASS 3.182s, 역의존/manifest/graph/diff PASS.

- Task 4 exact base: `4932988b3dbebcd843a9577ceecbe5d22b17ea07`.
- Luna `retirement_v2_adapters`, `.worktrees/retirement-v2-adapters`, branch `agent/retirement-v2-adapters`.
- owned internal/herdr 전체에는 testdata/v0.8.2의 14개 전용 fixture를 포함한다. 제품 monitor/herdr.go와 실제 Herdr session lifecycle은 변경하지 않는다.
- packet의 나머지 fields는 계획 Task 4에 고정했다. 구현 worker 1개, reviewer 1개 예약.

## Task 4 완료 · Task 5 배정

Task 4: complete (구현 `87d4b81`, report `62226de`, fresh `retirement_task4_review` spec/quality ACCEPT, findings 없음). 통합 `1b85898`/`1babc97`. coordinator15·Herdr source/test5·fixture14, 총34파일/9122줄 삭제. 내부 역의존 군집 외 importer0과 Linux/Windows Monitor graph를 확인했고 실제 session/worktree는 변경하지 않았다.

- Task 5 exact base: `1babc9727690393698812ff6c2e0184d1bd563b8`.
- Luna `retirement_git_adapters`, `.worktrees/retirement-git-adapters`, branch `agent/retirement-git-adapters`.
- owned는 internal/github와 internal/worktree 소스뿐이며 실제 Git worktree 삭제·cleanup 명령은 금지한다. 나머지 packet은 계획 Task5를 따른다.

## Task 5 완료 · Task 6 배정

Task 5: complete (구현 `e6b5755`, report `72f7d50`, fresh `retirement_task5_review` spec/quality ACCEPT, findings 없음). 통합 `284f53d`/`dc716be`. GitHub5·worktree7, 12파일/6359줄 삭제, 전후 외부 importer0과 정적 graph PASS. 실제 cleanup 미실행은 worker 기록 근거이며 reviewer가 호스트 전체 명령 이력을 관찰했다고 주장하지 않는다.

- Task 6 exact base: `dc716bea082c1be2503f05335d2f51da0f5da22e`.
- Luna `retirement_v1_foundations`, `.worktrees/retirement-v1-foundations`, branch `agent/retirement-v1-foundations`.
- state/contract 부모 전체가 아닌 계획의 exact v1 root 파일만 삭제하고 v2 subtree를 보존한다. focused v2 state/contract 검증은 별도 tmpfs를 사용한다. 나머지 packet은 계획 Task6를 따른다.

## Task 6 report 수정 round 1

구현 `b473239` + report `8b86693`/`6e566c6`에서 v1 26개 tracked file(24 Go·2 JSON)을 삭제했다. 요구 환경의 v2 state/contract tests는 0.390s/0.004s PASS다. 최초 불필요한 GOCACHE/GOPATH 추가 격리 시도는 종료 불명확으로 통과 근거에서 제외했다.

fresh `retirement_task6_review`는 `6e566c6`의 scope를 ACCEPT, evidence를 BLOCK했다. report가 보존된 state/v2 파일을 실제21개가 아닌26개로 기록했다. contract/v2 5개와 혼동한 수치다. Luna에게 report-only 정정, reviewed candidate와 implementation SHA 구분, exact manifest/diff 검사만 배정했다. final fix SHA는 다음 ledger 항목과 review packet에 기록하며 자기 commit SHA를 본문에 꾸며 넣지 않는다. 제품/Go 테스트 변경·재실행 없음.
