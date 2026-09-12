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
