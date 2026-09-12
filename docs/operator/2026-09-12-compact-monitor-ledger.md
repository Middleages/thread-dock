# SDD ledger — plan: docs/superpowers/plans/2026-09-11-compact-monitor-and-global-toolbox.md

## 착수와 Git

- 사용자 지시로 PR90을 `611bd6aec528ed63106bd3ef3167c2f9c95e0d01`에 병합했다. merge tree는 reviewed PR head `4d52026`과 동일하다. Issue89 closed, 보드 Issue/PR Done 반영.
- 요청 branch의 기존 remote head `03df312`와 설계 이력을 보존해 `.worktrees/compact-monitor-toolbox`에 `agent/compact-monitor-toolbox`를 만들고 main을 merge했다. 결과 `4c52f646f5c55539147343a1e426621831143bc1`, tree는 main611bd6a와 동일하다.
- 지정 spec348줄·plan928줄을 모두 읽었다. baseline 검증은 PR90의 기존 Go3/UI37/build와 recipe 동등성 근거를 채택하며 같은 full gate를 반복하지 않는다.
- 중단된 codex-runtime·기존 worktree·Windows staging·사용자 데이터는 보존한다. 제품 데이터 migration은 테스트의 임시 파일에서만 검증하며 실제 사용자 파일을 수동 변환하지 않는다.

## 사전 검토와 결정

Sol `compact_preflight`는 read-only로 기존 toolbox/app/bindings/소비자를 검토했고 쓰기·테스트는 하지 않았다. 실제 runtime model/effort는 unverified다.

| 관계/Task | 생산·소비·검증 정합성 |
|---|---|
| 1 자체 | refs/global schema와 migration 실패/재시도 검증. putReferences가 v1을 직접 rewrite하지 않게 보호 |
| 1→2 | Go store methods/types를 Task2 App이 소비. 임시 old methods를 유지해야 Task1 focused test가 compile 가능 |
| 2 자체 | 새 Wails/TS contracts와 project-key helper를 검증. helper 추출 App.tsx를 ownedPaths에 포함 |
| 2→3 | old component가 old bindings를 계속 소비하므로 Task2에서는 함께 유지 |
| 3 자체 | 미완료 기본·완료 toggle/unknown reassign/실패 입력 보존은 spec 기준. controlled cache props를 고정 |
| 3→4 | old component 삭제는 App의 마지막 import 교체 뒤 수행 |
| 4 자체 | App single cache/count, TopBar/Settings/ProjectDetail layout. 임시 Go/TS/old component도 이 단계 종료 시 제거 |
| 4→5 | ProjectDetail/App/styles를 공유하므로 Herdr density 변경은 직렬 |
| 5 자체 | matching helper 이동은 의미 불변, collapsed severe state와 자세히 검증 |
| 2·3·4·5→6 | 새 테스트를 Makefile에 반영, 기존 영향 증거를 중복하지 않고 final gate 한 번 |
| 6 자체 | Linux fixture/visual와 실제 Windows package/smoke를 분리해 기록 |

Ruling: common Todo는 optional-string wire로 정규화 — 두 문서의 null 의미와 명시된 Go/TS interface를 함께 지킴 — 잘못 선택하면 외부 payload 호환 조정 비용이 있으므로 null 입력도 수용한다.

Ruling: migration은 project v1을 재개 신호로 사용하고 refs write guard/단일 App mutex/결정적 ID·dedupe/합산 limit 오류를 둠 — 부분 실패와 첫 refs 저장의 데이터 유실 방지 — limit 초과 사용자는 명시적 오류를 해결하기 전 새 Toolbox를 쓰지 못하지만 원본은 보존된다.

Ruling: 완료 toggle·unknown project 재지정은 설계 우선, App single cache를 controlled Drawer가 소비 — 문서 충돌과 stale count/초기 empty 덮어쓰기 방지 — props가 늘지만 상태 원본은 한 곳이다.

Ruling: old API/components는 Task4 마지막 consumer 교체 뒤 제거 — Task1~3을 compile-valid 상태로 유지 — 임시 호환 표면이 존재하므로 Task4/최종 review에서 부재를 확인한다.

Ruling: 이미 검증된 묶음을 Task6에서 다시 full suite로 실행하지 않음 — 사용자 검증 정책 우선 — 최종 gate와 실제 native smoke는 별도로 남긴다.

## 역할·검증

root는 docs/Git/공유 경계를 조정, Luna high는 코드·테스트, fresh Sol medium은 고정 SHA 리뷰. 자동 모델 승격 없음. 최초 storage/contract는 직렬, 독립 UI 작업만 경로를 분리할 수 있다. 구현 worker 최대3/reviewer1 슬롯 유지.
Go1.27.0 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin`, Node26.8.1. 테스트 TMPDIR는 고유 `/dev/shm` 경로, UI는 단일 worker+전용 NODE_COMPILE_CACHE를 사용하고 timeout/isolation을 완화하지 않는다.
현재 native computer-use와 Browser plugin은 노출되지 않았다. 후반 visual은 로컬 browser fixture 검증으로 분리하고 Windows Wails/native smoke는 사용자 Windows GPT app 근거를 요청한다.
Impeccable context는 한 번 실행했다. 승인된 Operate 레이아웃과 기존 token/style을 따르며 별도 Superdesign canvas/새 시안은 생성하지 않는다. UI 완성 후 detector 한 번·desktop/narrow 캡처와 fresh Sol review를 수행한다.

## Task1 배정

- Issue91을 생성했다. 요청 remote branch는 초기 통합·계획 정합성 commit까지 정상 push했다.
- taskId `compact-toolbox-storage`, exact base `33d279ab59653a42a997b4f9c835c7857ce075dd`, Luna `compact_storage`.
- worktree `.worktrees/compact-toolbox-storage`, branch `agent/compact-toolbox-storage`.
- ownedPaths는 toolbox.go/toolbox_test.go와 Task1 report뿐이며, app/frontend/실제 사용자 데이터는 forbidden이다. 나머지 packet과 추가 데이터 보존 테스트는 추출된 Task1 brief에 고정했다.
- Task1 pending; 현재 구현 worker1, fresh reviewer1 슬롯 예약. root는 문서·UI 검증 준비를 병행한다.

## Task1 review BLOCK · fix round1

후보 `6e77d7939f09729abfe152fc3fc38f3491b4c414`(구현a03b2db)를 fresh `compact_storage_review`가 BLOCK했다. global.put이 migration 후 caller값만 저장해 legacy를 잃고, project absent/v2 때 malformed global을 검증 없이 덮으며, existing-global semantic duplicate를 실제 compact하지 않는다. 필수 regression 사례와 정확한 command/TMPDIR 근거가 빠졌고 report 끝 빈 줄도 diff-check에 걸렸다. 후보는 통합하지 않았다. 실제 사용자 파일은 접근하지 않았다.

Ruling: migration을 동반한 최초 global put은 existing global+legacy+caller를 함께 merge해 global-first로 저장/반환하고, 이미 v2인 정상 put만 전체 교체한다 — 첫 호출에서 보이지 않은 legacy 데이터의 유실 방지 — v1 상태에서 직접 replace를 기대하는 호출자는 반환된 merged state를 다시 사용해야 하며 새 UI는 load 성공 후에만 save한다.

Task2 SaveGlobalToolbox는 별도 ensure로 migration 여부를 지우지 않고 mutex 안에서 migration-aware put으로 직접 위임한다. refs methods는 ensure 뒤 reference 접근, GetGlobal은 get을 호출한다. 저장 경로 전체의 existing-file 검증, 완전한 dedupe, missing regression tests와 재현 가능한 새 RED/GREEN evidence를 같은 Luna에 배정한다. old test tuple은 기록이 불충분하므로 새 수정 tuple의 focused 근거로 대체하며 repo-wide suite는 실행하지 않는다.

## Browser 준비 근거

Browser plugin/agent-browser CLI는 없지만 cached Playwright1.55.0과 matching Chromium1187이 존재한다. 첫 headless launch는 /tmp profile에서180초 timeout으로 실패했다. 실제 UI 검증이 아니며 재현·환경 확인 전 통과로 기록하지 않는다. native computer-use는 여전히 없다.

후속 capability probe는 `TMPDIR=/dev/shm/threaddock-compact-browser.Y3WqwB`에서 같은 Playwright1.55.0/headless browser가 Chrome140.0.7339.16으로 시작·종료해 exit0이다. missing shared library와 남은 최초 probe process는 없었다. 이는 실제 UI 검증이 아니라 browser 준비 근거이며 package를 새로 설치하지 않았다.

## Task1 완료 · Task2 배정

Task1: complete (구현a03b2db/report6e77d79 → fix2fd25e3/report `b3b9919de9e6ee49bff1b7580b4e69ba33d745b8`에서 scoped spec/quality ACCEPT). migration put/data overwrite/compaction/누락 tests/검증 로그/diff whitespace findings 모두 addressed. 최초 RED의 runtime path는 미기록으로 명시하고 새 GREEN3tuple의 실제 로그/exit0를 확인했다. tests 재실행한 reviewer는 없다.
통합은4a8fc08/35b78fc/cd5b5a6/5dc0abb다. Task2 exact base `5dc0abb5cc473dc80512ecec0d21219f25942473`, `.worktrees/compact-toolbox-bindings`, `agent/compact-toolbox-bindings`.
Task2 packet은 기존 소비자 임시 유지와 App helper 추출 범위를 포함한다. App의 별도 mutex와 first-save migration의 public 경로 회귀를 검증한다. Todo/cache/layout 변경은 아직 배정하지 않았다.

## Task2 후보 · Task3 준비 (2026-09-13)

Task2 후보는 `57bb3b29780cce2ac5e9f041229f1ca3adc726fc`이며 fresh `compact_bindings_review`의 고정 SHA 검토 중이다. worker report는 Go focused, bindings/project-key 5개, AppScope 4개, TypeScript compile 통과와 잘못된 cwd에서 실행한 최초 tsc 실패를 구분한다. 아직 승인·통합으로 표시하지 않는다. root가 준비한 node_modules symlink만 테스트 종료 후 제거했고 연결 대상 dependency cache와 기존 worktree는 보존했다.

Task3의 자료와 Drawer는 독립 파일로 나눌 수 있으며 계약·base를 확정한 뒤 배정한다. 공유 App/cache/bindings 변경은 Task4까지 직렬 소유한다. 무거운 UI 검증은 한 worker씩 실행한다.

Impeccable의 UI 착수 계약은 새 시안이 아닌 사용자 지정 설계의 기록이다. Task3a는 index.html 첫 body child에 150단어 이하 계약 주석을 먼저 남긴다. 기존 cobalt/slate/cloud token과 Segoe UI 계열, 간결한 목록·구분선을 유지한다. 상단 상태/Toolbox/설정, 최대1440px 본문, 좌우 고정 rail 제거, 폭420–480px 우측 overlay와 업무/자료 구분이 첫 화면 구조다. 도구는 복사 전용이며 프로젝트 Todo에서 전역 Drawer의 해당 필터로 연결한다. FORM key는 `user-pinned-compact-monitor`로 명시하고 무작위 seed를 실행했다고 주장하지 않는다. Task6는 build 결과의 주석 유지·화면 검토·기존 시각 체계 문서를 확인한다.

Task2는 App boundary coverage 두 건 누락으로 BLOCK 후 같은 Luna가 app_test/report만 수정했다. `b7af8e3d6d51b554840275e6ecf535ecf3f54865`에서 scoped ACCEPT, 새 focused Go 로그 `/dev/shm/td-compact-toolbox-bindings-fix.ZEdOb6/app-focused.log`와 exit0를 확인했다. reviewer는 테스트를 재실행하지 않았다. 통합3a726fc/894f512 완료.
Task3a/3b exact base는 `894f51229b9ec897592462c3ee9b5c6d42eb6042`다. Sol packet 준비 결과를 root가 검토·확정했다. 일반 Toolbox 진입은 명령어 탭/전체 필터, 프로젝트 바로가기는 할 일 탭/해당 key 필터로 고정했다. 계약 주석은 overlay가 본문을 일부 가릴 수 있으나 본문 폭을 재배치하지 않는다는 의미다. Task3a 먼저 계약을 기록하고 root 확인 뒤 Task3b UI를 시작한다. 구현2슬롯/리뷰1슬롯과 UI 테스트1슬롯을 예약한다.

Task3a의 `9f248274`에서 index.html 첫 body child 계약을 root가 확인했고 Task3b UI 구현을 시작했다. 두 worker는 별도 branch/worktree이며 기존 API/UI 삭제는 아직 하지 않는다.

Ruling: Task4에 private `monitor-presentation.ts`/test 한 쌍을 허용한다 — ProjectDetail의 handoff와 Task5 HerdrSummary 사이 cycle·matching 복제 방지 — 파일 한 쌍이 늘지만 Task5는 read-only 소비하며 public wire는 바뀌지 않는다. 실제 base 코드의 matching은 issue 결과가 있으면 exclusive 반환, 없으면 trusted project 결과, 그것도 없으면 exact repository coordinator fallback이다. 기존 연결이 없는데 항상 project/repo 연결을 섞거나, 반대로 fallback을 일괄 금지하는 해석 모두 채택하지 않는다. Task4 전용 SettingsShell test와 의도적 AppScope/monitor DOM 변경도 같은 Luna가 직렬 소유한다. 상세 packet의 base는 Task3 통합 후 고정한다.

## Task3 리뷰와 수정 배정

Task3a 후보 `473b55a8567cdcdd90cc3789a0274b1a8fcaaf27` (opening contract9f248274)는 focused8tests/유효한 단일 tsconfig typecheck 통과다. RED shell exit는 미기록으로 남았다. fresh Sol은 Windows/WSL 자료 add/delete·native clipboard 실패 회귀 누락과 input/select의 기존 focus token 미사용으로 BLOCK했다. load/save key-switch guard는 적합하며 unmount cleanup은 non-blocking 관찰이다.
Task3b 후보 `c9068f30be7d8950336765369b06ceb737f0f204`(구현05cefa558)는 focused12tests/typecheck 통과 후 fresh review 중이다. root는 `updateTodo({done:...})`가 기존 projectKey를 지우는 버그를 발견해 reviewer에 전달했다. 두 UI 후보는 아직 통합하지 않았다. 모두 임시 fixture 테스트이며 실제 사용자 데이터는 접근하지 않았다.

운영 blocker와 대응: 이전 references 구현/review thread가 live 목록에서 사라지고 복원 followup이 `agent thread limit reached`로 거부됐다. 완료된 옛 thread interrupt로도 회수되지 않았고 thread-close 도구는 노출되지 않았다. 모델 승격이나 root 코드 수정 없이 live `compact_drawer` Luna에 Task3a test/css/report 수정만 기존 references worktree/base473b55a에서 직렬 재배정했다. Drawer branch는 frozen review 동안 편집하지 않는다. 재배정 packet은 followup에 전체 필드로 전달했으며 UI test slot도 단일로 유지한다. 이후 독립 Sol 리뷰는 구현에 참여하지 않은 사용 가능한 reviewer로 유지한다.

Task3a는 test/css fix55c95fa/report `6bae0ca511d4f8e702d356136a855a99399f323f`에서 독립 td_reviewer `slice3_integration_review`의 ACCEPT를 받았다(옛 engine 작업 재개가 아닌 새 후보 리뷰). focused11tests 로그를 채택하고 재실행하지 않았다. 통합a9a7d37/66c764b/399a95f/f7efc27 완료. 원 후보를 accepted로 잘못 부른 report 한 문구는 reviewer 허용대로 docs-only 정정했다. unmount cleanup 관찰은 blocker가 아니며 native/통합 visual은 여전히 미검증이다.
Task3b fresh review는 root 발견 key 유실, 필수 경로 테스트 누락, form focus token, controlled-close focus cleanup을 BLOCK했다. 같은 Drawer Luna에 네 항목을 하나의 fix로 배정했다. report 최종 SHA 자기참조 요구는 사전 외부 고정 정책과 충돌하므로 채택하지 않고 root ledger에 후보 SHA를 기록한다. `pending` 표현만 외부 고정 설명으로 바꾼다.

Task3b는 fix27dc8d9/report `2b6436977adc3259fa921dd2e8c60ecffc0a881b`에서 same independent reviewer scoped ACCEPT다. 실제 RED3회귀/최종 GREEN21tests·typecheck를 채택했다. 통합ea59e7b/551bb54/d921f5f/153c196 완료. worker가 언급한 중간 timeout의 상세 invocation은 보고서에서 빠져 별도 확인 요청 중이며 최종 성공 tuple과 혼합하지 않는다. 이전 untracked root node_modules는 Vitest `.vite/results.json` cache뿐이고 보존했다. frontend prepared symlink만 제거했고 연결 대상 dependency cache는 유지했다.
Task4 exact base `153c196054c27f0d0addbcd829e9e9985b105992`, worktree `.worktrees/compact-monitor-integration`, branch `agent/compact-monitor-integration`에 live Luna를 배정했다. Task3 경로는 read-only 의존성, App/Settings/private helper/old Go·TS API 제거는 이 worker 하나가 소유한다. UI test slot1/구현1, reviewer용 여유를 유지한다.

Windows 검증 가능 범위 추가 확인: `/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe`가 실행 가능하며 사용자 지정 경로의 실제 버전 명령이 exit0으로 Go1.27.0 windows/amd64, Node26.8.1, Wails2.15.0을 반환했다. 최종 고정 SHA를 새 NTFS 임시 staging에 export해 표준 Wails build를 실행할 수 있다. 이전 staging은 삭제·수정하지 않는다. native computer-use는 여전히 노출되지 않아 앱 조작/clipboard/실제 파일 migration smoke는 Windows GPT app 확인이 필요하다. 버전 probe는 새 제품 build 성공 근거가 아니다.

## Task4 후보와 review fix

후보 `86a0ad4d3860cfb8e49f375d6ed6e322f1c6fc30`(구현1481e0d)는 UI7파일42tests, 추가 AppScope7tests, Go monitor package, typecheck 통과 근거를 제출했다. fresh `compact_integration_review`가 1911줄 고정 diff를 읽고 BLOCK했다: 기존 Project 필드의 별도 section/per-field row를 합치며 정확 assertion을 약화함; ProjectDetail에 unused copy/Herdr detail 함수를 복제함; migration용 reference validation test까지 삭제함; App의 성공 save/cache/count와 실제 shortcut 연결/중복 조작 근거 누락.
같은 Luna에 원 표시/assertion/validation 복원, dead duplicates 제거, public UI 기반 deferred binding 통합 회귀만 배정했다. 도달 불가능한 race를 강제로 만들 public hook/API는 추가하지 않으며 loading/save guard가 겹침을 막는다면 실제 방지 경로로 증명한다. 새 Go 검증은 복원 validator 한 테스트로 한정하고 전체 monitor/package suite를 반복하지 않는다. 후보는 아직 통합하지 않았다.

Task4는 fixb33dd8b/report `4fadafdb3de585fc8d3f15b1bceb74781362c381`에서 scoped ACCEPT를 받았다. 정확한 Project field section/assertion·migration validator 복원과 중복 함수 제거를 확인했고, AppScope10tests/ProjectDetail+monitor28tests/Go exact validator/typecheck를 채택했다. 새 App 회귀의 최초 실패는 getByRole를 부재 assertion에 사용한 테스트 작성 오류였지 제품 regression이 아니므로 report를 root docs-only 정정했다. 통합74690dc/59c6119/10993e4/2fc9f3f 완료.
Task5 exact base `2fc9f3f820b664845d7440dd08038b92334cfc81`, worktree `.worktrees/compact-herdr-summary`, branch `agent/compact-herdr-summary`에 같은 Luna를 배정했다. private matching helper와 Go/Toolbox 경로는 read-only다. README는 승인된 Task4 사용 흐름으로 갱신했고 usability checklist의 Herdr 항목은 아직 실행하지 않은 수동 확인 목록으로 유지한다. Task6 전체 gate·Windows build·browser visual은 아직 실행하지 않았다.

Task5 후보 `09645503b3fff1a728a289d0fe243cf99841ffd4`(구현500da557)는 focused9tests/영향47tests/typecheck 통과 후 fresh review BLOCK이다. missing/unknown에 success(녹색) tone을 붙이며, 연결 status의 첫 severe 값만 선택해 더 심각한 agent blocked가 다른 offline 연결 뒤에 숨는다. 같은 Luna에 HerdrSummary/test/report만 수정 배정했다. matching helper 원본과 선택 의미는 바꾸지 않는다.
독립 Task6 manifest는 base0964550에서 `.worktrees/compact-final-test-manifest`/`agent/compact-final-test-manifest`가 Makefile FRONTEND_TESTS만13파일로 갱신했다. 후보 `26eb0b1b6c322c9c06de9827e7d3252604780787`(구현02b277bc), static path/unique/`make -n check`만 통과했고 실제 suite는 실행하지 않았다. fresh manifest review 중이며 Herdr 수정과 경로가 겹치지 않는다.

Task5 수정cc8e4ecc에서 scoped ACCEPT, manifest26eb0b1에서 fresh ACCEPT를 받았다. 통합02f3640/938cf95/7c9fda4/32f85ce 및57fb379/516a40a 완료. 구현과 테스트 목록의 blockers는 해소됐다.

## 최종 화면 확인

source516a40a의 browser fixture 전체 흐름은 exit0, console0, narrow414px 가로 넘침0/Drawer380.875px를 확인했다. 기본 Chromium GPU 경로 timeout과 사라진 체크박자를 기다린 harness 오류, disable-gpu 대조 및 수정 후 성공을 새 [검증 기록](../superpowers/reviews/2026-09-13-compact-monitor-toolbox-verification.md)에 분리했다.
독립 visual review는 `disposition: fix`로 toolbar/eyebrow/close icon/자간/자동 focus 다섯 항목을 지정했다. `.worktrees/compact-visual-fix`/`agent/compact-visual-fix`, base516a40a에서 같은 Luna가 한 batch로 수정한다. 루트 원안의 기존 alert3px warning 다섯 건은 시각 체계 보존 결정으로 유지하며 detector는 한 번만 실행했다. final gate와 Windows build는 시각 수정 뒤 고정 SHA에서 수행한다.

## 최종 gate · 게시 준비

Visual fix 후보c22873f(원 구현1ee5609)의 새 focus 회귀는 통과했지만 ext4 AppScope 전체는 세 timeout으로 실패했다. root가 개발 서버를 종료하고 동일 후보를 `/dev/shm/threaddock-compact-final.f5rFZF/checkout`에 격리한 tuple에서11tests가 통과했다. 코드 변경 없이 조건을 분리한 근거이지 특정 OS 원인 확정은 아니다. 첫 RED 로그에는 focus 실패+1timeout이었는데 report의3timeout 표현도 정정했다.
첫 post-fix visual 판정은 active scope dark-on-dark를 remaining으로 남겼다. CSS-only6f474fe/report0917dbf(통합a2e94db/953db4e)가 중복 theme를 제거했고, 같은 캡처 경로 재검토에서 모든5항목 resolved/remaining clear/`disposition: ship`을 받았다. 최종 대비15.6589:1/refresh119.328px/414px overflow0/console0. 추가 detector는 실행하지 않았다.
953db4e에서 Windows 표준 Wails build48.277초·임시 파일 storage tests0.675초 PASS. 새 staging만 만들었고 이전 staging은 보존했다. artifact 위치·hash와 time.Time generator 진단은 검증 기록에 있다. native computer-use가 없어 Windows GPT app 검증을 비동기로 요청했으며 아직 native 결과는 없다.
953db4e의 첫 `make check`는 Makefile39 gofmt에서 멈췄다(Go/UI tests 시작 전). Task4 App fields의 두 줄 정렬을 Luna e705975/reportf60ce2c로 수정해 통합a55134d/fbefd79했다. 정규화 hash 일치로 Windows953과 실행 코드가 같음을 확인했다. 최종 fbefd79 gate는 exit0: Go vet/3packages, UI13files95tests, TypeScript/Vite build. 이후 동일 suite는 반복하지 않는다.
기존 시각 체계 문서는 coordinator의9861699/3fcf633/63835a8을 ac98ea2/24a08a8/e4203b8로 통합했다. SVG/status snippet도 실제 코드와 일치하게 docs-only 보정했다. 이후 문서 변경은 parse/link 검증과 최종 통합 리뷰로 확인하며 제품 gate를 재실행하지 않는다. final integration review와 PR 게시 대기; main merge는 사용자 작업이다.
