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
