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

Fix candidate: `ca5407f087e09176ca8e2417702e463bc357ec40` (직전 reviewed `6e566c629952ee1ec6c95781d318bd2a817b163e`, implementation `b473239707068dba0b26add0bd7c10dbd6567f68`). report-only 수정과 21/5/1 manifest assertion·diff check PASS, Go tests 재실행 없음. 동일 reviewer에게 fix diff만 재검토 배정했다.

## Task 6 완료 · Task 7 배정

Task 6: complete (fix round1 `ca5407f`에서 두 finding ADDRESSED, scoped spec/quality ACCEPT, 새 문제 없음). 통합 `a8b73af`/`9e5b180`/`9558f71`/`b6fb422`. 26개 tracked 파일/4169줄 삭제. 남은 v2 state21·contract5·fixture1 보존과 focused PASS 근거를 유지한다.

- Task 7 exact base: `b6fb422bf88f53cef36e38a984532581313e1057`.
- Luna `retirement_v2_foundations`, `.worktrees/retirement-v2-foundations`, branch `agent/retirement-v2-foundations`.
- owned는 v2 registry/runtime/state/contract와 전용 fixture이며 사용자 로컬 데이터는 포함하지 않는다. packet의 나머지는 계획 Task7을 따른다.

## Task 7 완료 · Task 8 배정

Task 7: complete (구현 `4c181b9`, report `5afc72c`, fresh `retirement_task7_review` spec/quality ACCEPT, blocking 없음). 통합 `649ad57`/`4a94053`. v2 Go32·JSON1, 총33파일/10131줄 삭제. 외부 importer/fixture 소비자0, Monitor/runner/template/module 불변, 정적 Go graph/Linux·Windows listing PASS.

- Task 8 exact base: `4a94053cbce8edee8e4a74dc866f8324578b6a7d`.
- Luna `retirement_orphan_utilities`, `.worktrees/retirement-orphan-utilities`, branch `agent/retirement-orphan-utilities`.
- owned는 dag/pathscope/opencodeagent의 orphan source/test뿐이다. 최종 package 목록이 monitor, internal/runner, internal/projecttemplate인지 확인하고 runner focused test를 수행한다. 나머지 packet은 계획 Task8을 따른다.

## Task 8 report 수정 round 1

구현 `896c3df`, report `0a333b9`: orphan6파일/423줄 삭제, runner focused PASS0.119s, package 집합 정확3개와 Linux/Windows graph PASS. fresh `retirement_task8_review`에서 코드·scope는 통과했으나 report metadata 두 항목으로 BLOCK했다: candidate를 구현 SHA로만 표시했고 result.commitSHA 필드가 없으며 report를 포함한 총 changedFiles7과 구현 삭제6을 구분하지 않았다.

Luna에게 report-only fix를 배정했다. result.commitSHA는 구현896c3df임을 명시하고 reviewedCandidate0a333b9를 구분하며 새 fix head는 외부 ledger/packet에 고정한다. 총7=source삭제6+report1, manifest/diff만 검사하고 Go 테스트는 재실행하지 않는다. Task6/8의 동일 계열 metadata 혼동을 반영해 공통 계획의 기록 규칙도 명확히 했다. 제품 변경은 없다.

Task8 fix candidate: `b276412b8e67981e75ec82b0dbb7acfe40c4f9fd`, 직전 reviewed `0a333b93f4058e8461bc1a64d97d4874968db568`, implementation `896c3dfb825ef2ec5e6e185c4145608bd795b1ce`. report-only 수정과 total7/implementation 이후 제품 diff0/diff-check PASS, Go 테스트 재실행 없음. 동일 reviewer에게 한정 재검토한다.

## Task 8 완료 · 구현 종료

Task 8: complete (`b276412` scoped rereview ACCEPT, 두 metadata finding ADDRESSED, 새 문제 없음). 통합 `cda1e04`/`7c74f30`/`61cf732`. orphan6파일/423줄 삭제, runner test PASS0.119s와 정적 graph 근거 유지.

통합 `61cf73292f0e1e1d2592a68a1f5a2a7592a5b53b`에서 root Go list는 `thread-dock/internal/projecttemplate`, `thread-dock/internal/runner`, `thread-dock/monitor` 세 개만 반환했다. 삭제된 tracked 파일은167개이며 `2db8f77` 대비 Monitor/runner/projecttemplate Go 코드/go.mod/go.sum diff는0이다. main과 중단된 사용자 실험은 보존했다.

모든 Task의 코드·evidence 리뷰는 완료됐고 final make check와 전체 통합 리뷰를 남겼다. 이 단계에서 전체 제품 검증 완료나 main 병합을 주장하지 않는다.

## 최초 최종 gate 실패와 UI 진단

검증 SHA `e630c5a502280ee88b3c4d608fe65255d8381342`, tmpfs checkout `/dev/shm/threaddock-retirement-final.KMxdTI/checkout`, Go1.27.0/Node26.8.1/npm11.19.0, 원래 forks/isolation과 `VITEST_MAX_WORKERS=1`에서 `make check`를 실행했다. TMPDIR은 `/dev/shm/threaddock-retirement-final.KMxdTI`다.

- template-check/gofmt/Go vet/전체 Go3 packages PASS(projecttemplate0.004s, runner0.014s, monitor0.502s).
- UI: 3 files PASS·1 file FAIL·2 worker startup errors. 8 tests PASS·WorkTable1 FAIL(timeout5000ms, 실제36.3s). monitor/AppScope worker는 startup timeout으로 실행하지 못했다. UI duration353.85s, make exit2, frontend build 미실행.
- 로그: 비추적 로컬 `.superpowers/sdd/2026-09-12-engine-retirement-completion/make-check-e630c5a.log`.
- 현재 source/lock/deps와 assertion/timeout/isolation은 변경하지 않았다. tmpfs만으로 충분하지 않은 새 관찰이다. 제품 원인을 단정하지 않고 synchronous WorkTable query와 worker bootstrap을 분리 진단한다.
- taskId `retirement-ui-startup-diagnosis`: Sol read-only, base e630c5a, ownedPaths[], above validation worktree(detached); forbidden writes/full suite repeats/timeout·isolation 완화/process 종료; acceptance 설치 Node/Vitest의 지원되는 cache/초기화 환경을 근거로 한 최소 대조; tests source/help/non-test probe만; result는 후속 기록. 실제 runtime model/effort unverified.

문서104개/로컬 링크141개 검사와 diff check는 PASS다. 이 근거가 UI/full gate 성공을 뜻하지 않는다.

## UI 초기화·query 분리 진단

- Sol read-only probe: 현재 compile cache는 unset/비활성, Node timer24ms·raw IPC36ms·Vitest import44ms·jsdom import817ms. forks entrypoint는 graph를 읽고 예상한 no-IPC 오류로0.38s에 종료했다. 이는 테스트 성공이 아닌 초기화 가능 근거다.
- root DOM probe: jsdom import2077ms/create169ms, 첫 getByRole709ms/다음10ms. WorkTable의 실제 CSS와 유사 DOM을 쓴 추가 probe는 load1917ms, Intl14ms, header query89/4ms, button663ms였다. 실제 WorkTable 테스트 대체 증거로 사용하지 않는다.
- 현재 시점에는 gate의60s startup/36s sync 실행을 재현하지 못했다. 비균일한 자원·스케줄링 지연 가설이며 정확한 병목은 미확정이다.
- 설치 Node26은 compile cache를 지원하고 Vitest5는 parent env를 worker에 전달하며 종료 때 flushCompileCache를 호출한다. coverage는 비활성이다. dedicated tmpfs `NODE_COMPILE_CACHE=/dev/shm/threaddock-node-compile.kOQNbZ` 한 변수만 추가해 반복 compilation을 줄이는 대조를 선택했다. 첫 worker와 host starvation 해결을 보장하지 않는다.
- 첫 대조는 실패했던 WorkTable/monitor/AppScope3파일의 focused 실행이다. 같은 e630c5a·source/lock/deps·forks·isolation·assertion·timeout을 유지한다. full gate 재실행 전 결과를 확인한다. 로그 `ui-failed-files-compile-cache-e630c5a.log`는 같은 비추적 진단 디렉터리에 둔다.

## Cache 환경 대조 결과 · 기존 selector 실패 귀속

같은 e630c5a의 focused3files는 startup error 없이 모두 실행됐다. WorkTable·AppScope는 통과, monitor24 중1개가 실제 assertion으로 실패해 총28 passed/1 failed,102.87s,exit1이다. cache 완화의 인과나 host 병목 해결을 확정하지 않으며 관찰 결과만 채택한다.

실패는 `monitor.test.tsx:222`의 exact name `Second work` 선택자다. PR87의 WorkTable이 접근성 이름에 번호·종류·제목·상태·시각을 모두 넣어 실제 tab 이름은 `—ProjectSecond workneeds operator…`였다. 이 테스트와 제품 WorkTable은 2db8f77에서 변경되지 않아 cleanup 회귀가 아닌 유입 main의 stale 테스트다.

Task9 직렬 scope 결정: 생산 코드는 그대로 두고 해당 row 제목을 포함하는 name regex 한 곳만 수정한다. 테스트의 선택 전환·heading·link 검증, role, timeout과 isolation은 보존한다. 이 구체적인 테스트 소스 수정으로 이전 gate tuple은 무효화되며 수정 후 새 최종 통합 gate를 수행한다. 초기 gate 실패와 focused 실패는 모두 이력에 보존한다.

Task9 exact base: `e7ce6e6a9c7dc7d7665f8882e989ae34a442146e`; Luna `retirement_gate_selector_fix`, `.worktrees/retirement-gate-selector-fix`, branch `agent/retirement-gate-selector-fix`. 기존 same-lock tmpfs node_modules를 읽는 ignored symlink를 준비했고 이 시간에는 다른 테스트를 병렬 실행하지 않는다. ownedPaths는 테스트 선택자 한 곳과 Task9 report뿐이다. 이 Task 이후 Monitor **production** diff0와 테스트 한 곳 정정을 구분해 보고한다.

Task9 implementation `4bfa9653b42886be32dae40a00d2a83385019b8a`, review candidate `1e3cbab5ad87c7ad681ae3aa63a750465323e550`. 총2파일(test1+report1), action selector 한 줄만 수정했다. 첫 worker wrapper 호출의 종료 상태는 불명확해 unverified로 남겼고 새 전용 tmpfs/cache 환경의 한정 테스트는1 passed/23 filtered,6.56s,explicit exit0이다. production/assertion/timeout/isolation 변경 없음. fresh Sol 리뷰에 넘겼다.
검증 후 root가 준비했던 untracked node_modules **symlink만** 확인 후 unlink했다. 가리키던 tmpfs dependency 파일과 기존 worktree는 삭제하지 않았다. 정확 통과 명령은 Task9 report에 보존한다.

Task9: complete (`1e3cbab` fresh `retirement_task9_review` spec/quality ACCEPT, findings 없음). 한정 selector 수정 외 role/후속 click/heading/link/assertion 및 production/config/timeout/isolation은 보존됐다. 다음 gate는 이 테스트 수정과 NODE_COMPILE_CACHE 환경을 포함하는 새 tuple에서 수행한다. 이전 e630c5a의 실패는 숨기거나 성공으로 바꾸지 않는다.

## 최종 통합 gate 통과

- SHA: `1e427eda673a997245c58420df83dd2f6070d06a`.
- worktree: `/dev/shm/threaddock-retirement-final.KMxdTI/checkout` (detached, 이 SHA와 tracked diff0).
- command: `NODE_COMPILE_CACHE=/dev/shm/threaddock-node-compile.kOQNbZ VITEST_MAX_WORKERS=1 TMPDIR=/dev/shm/threaddock-retirement-final.KMxdTI PATH=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin:$PATH make check`.
- 환경: Go1.27.0 Linux/amd64, Node26.8.1, npm11.19.0, 동일 package.json/lock/dependencies. 원래 forks/isolation/assertion/test timeout 유지.
- 결과: exit0. template-check/gofmt/Go vet·전체 Go3 packages PASS(변경 없는 Go tests cache 채택), UI6files/37tests PASS(24.08s), TypeScript/Vite build PASS(8.63s).
- Vite의 비치명적 `[PLUGIN_TIMINGS]` 진단은 있었으며 오류로 숨기거나 출력이 전혀 없었다고 표시하지 않는다. hook timing이 겹친다는 도구 진단이고 build exit0이다.
- 로그: 비추적 로컬 `.superpowers/sdd/2026-09-12-engine-retirement-completion/make-check-1e427ed-compile-cache.log`.
- build가 제거한 tracked dist placeholder를 원래 한 줄로 복구했고 validation checkout의 `git diff --exit-code`를 확인했다. generated assets는 commit하지 않는다.
- 이 성공은 초기 실패와 focused 환경 대조를 대체 삭제하지 않는다. 정확한 host 지연 원인은 미확정이며 cache만으로 해결됐다고 인과를 단정하지 않는다. 이전 gate 이후 실제 테스트 선택자 수정과 환경 변화가 있어 새 tuple로 실행했다.
- Windows native/Wails build·실제 GHES/Herdr E2E·Skill pressure scenario·Wiki 페이지 발행 및 runtime model/effort identity는 이번 성공으로 확대하지 않는다.
