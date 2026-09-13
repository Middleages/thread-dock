# compact Monitor / 전역 Toolbox 최종 ledger

## 현재 상태

- 저장소 `Middleages/thread-dock`, [Draft PR92](https://github.com/Middleages/thread-dock/pull/92), [Issue91](https://github.com/Middleages/thread-dock/issues/91), [Project1](https://github.com/users/Middleages/projects/1).
- 선행 PR90은 main `611bd6a`에 병합 완료. 요청 branch `agent/compact-monitor-toolbox`의 기존03df312 이력은 보존했다.
- 2026-09-14 사용자 리뷰 base: `67b4759725dc33a94639d1f4ffac63e551b8ef62`. PR은 Draft 유지, main 병합/Ready 전환 안 함.
- conflict severity와 legacy aggregate-limit 및 Todo0개 진입점 수정은 통합 `e1fe17f`에 반영됐다. 새 gate는 Go3패키지/UI13파일111tests/build PASS, 같은 SHA의 Windows Wails build/storage tests도 PASS다. 이전95tests를 재사용한 결과가 아니다.
- Go/Wails+React, Go GitHub/Herdr 조회, copy-only Toolbox 경계 유지. 사용자 실험·실제 데이터·기존 worktree/Windows staging은 보존한다.

## 유지할 원본

- [설계](../superpowers/specs/2026-09-11-compact-monitor-and-global-toolbox-design.md), [계획](../superpowers/plans/2026-09-11-compact-monitor-and-global-toolbox.md), [최종 검증 기록](../superpowers/reviews/2026-09-13-compact-monitor-toolbox-verification.md), 이 ledger.
- 이 PR에 추가했던 중간 report10개는 Git 추적에서만 제거했다. 로컬 파일은 그대로이며 [67b4759의 보존 이력](https://github.com/Middleages/thread-dock/tree/67b4759725dc33a94639d1f4ffac63e551b8ef62/.superpowers/sdd/2026-09-11-compact-monitor-and-global-toolbox)에서 원문을 읽을 수 있다. 다른 계획의 report는 건드리지 않았다.
- 새 중간 packet은 ignored `.superpowers/sdd/pr92-review-fixes`, worker 결과/로그는 `/tmp`에 두고 아래에 최종 결과만 합친다.

## 설계 결정

- common Todo는 optional string이며 null/빈 값은 생략 출력. JSON version과 Wails 네 method는 유지한다.
- v1 project를 migration 재개 신호로 사용한다. global-first atomic 저장 후 references-only rewrite, partial failure 재시도·semantic dedupe·ID 재키·미완료 우선은 유지한다.
- 첫 v1 global put은 existing+legacy+caller merge, 정상 v2 put은 전체 교체다. malformed/unsupported와 잘못된 항목은 보존 오류다.
- React global cache는 App 한 곳이며 migration/save는 Go 전용 mutex로 직렬화한다. private presentation helper는 기존 Herdr matching 의미를 유지한다.
- Ruling: 기존 합산200 migration 거부 정책을 폐기한다 — 정상 프로젝트 데이터와 자료 접근을 막는 오류였음 — 큰 기존 데이터는 메모리/렌더링 부담이 있을 수 있으나 silent loss/접근 차단보다 보존을 우선한다.
- Ruling: global read/migration은 aggregate count로 거부하지 않고 새 저장만 종류별10000/현재 보존 개수와 비교한다. 초과한 기존 데이터의 편집·삭제는 허용하고 증가만 거부한다 — 유효한 legacy 전체 보존과 새 추가 제한을 분리 — 10000은 운영 기준이며 향후 성능 측정에 따라 조정할 수 있다.
- Ruling: Todo 진입점은 항상 유지하고 내부 view count만 number|null로 확장한다 — 0개/완료만 있는 프로젝트도 쉽게 진입하되 미조회 숫자를 꾸미지 않음 — 이 UI prop 외 serialized API 변경 없음.

## 이전 구현·리뷰 근거 (역사 기록)

| Task | 최종 독립 리뷰 후보 | 통합 |
|---|---|---|
| storage | b3b9919 | 5dc0abb |
| Wails/TS | b7af8e3 | 894f512 |
| 프로젝트 자료 | 6bae0ca | f7efc27 |
| 전역 Drawer | 2b64369 | 153c196 |
| 화면 통합 | 4fadafd | 2fc9f3f |
| Herdr summary | cc8e4ec | 32f85ce |
| test manifest | 26eb0b1 | 516a40a |
| visual fix | 0917dbf, visual ship | 953db4e |
| 최종 통합 | 78fd14de16ab0be45ec65ff24b723c83cdca273c ACCEPT | 게시67b4759 |

이전 Linux gate `fbefd791cb74009be5caca9f5ea91b6c8a30a1bf`는 Go3패키지/UI13파일95tests/build PASS였다. Windows953db4e의 표준 Wails build·임시 파일 storage tests, browser fixture ship과 실패/서식 수정/미기록 RED 제한은 최종 검증 기록에 남겼다. 이후 발견된 conflict/aggregate 문제 때문에 해당 근거를 이번 수정의 성공으로 확대하지 않는다.

## 2026-09-14 수정 Task / 예약

공통 baseSHA `67b4759725dc33a94639d1f4ffac63e551b8ef62`, deps 사용자 리뷰·위 root 직렬 계약. 구현2슬롯/리뷰1슬롯 예약. Go/UI 파일 소유권은 겹치지 않으며 full suite는 마지막 통합에서만 한다.

### pr92-ui-review-fix

- ownedPaths: frontend/src의 HerdrSummary.tsx/test, ProjectDetail.tsx/test, App.tsx, AppScope.test.tsx, monitor-presentation.ts(label만).
- worktree: `.worktrees/pr92-ui-review-fix`; branch: `agent/pr92-ui-review-fix`.
- forbiddenPaths: Go/bindings/types/CSS/사용자 데이터와 나머지 경로; matching 로직 불변.
- interface: ProjectDetail count number|null 외 API 불변.
- acceptance: conflict/unknown은 정상 아님; 명시적 상태 분류·optional 부재 처리·mixed severity; 0/완료-only/미조회 진입점과 실제 Drawer 필터.
- tests: 해당 UI focused RED/GREEN·필요 typecheck, 전체 suite 없음.
- result.changedFiles: 위 UI7파일; commitSHA `c1011290d448aa6175f41a8170f000e3b6b13b61` (초기3cb2c7, 통합25029fc/e1fe17f).
- result.executedCommands/outcomes: Node26.8.1/npm11.19.0/Vitest5.0.0, 해당4파일 focused39tests+tsc PASS. 첫 결과는 transcript 요약만 있다. 리뷰에서 필수 status 부재가 더 강한 offline을 덮는 문제를 수정한 후 HerdrSummary21tests PASS; `/tmp/pr92-ui-round1-red-20260914.log`는3개 실패/exit1, `...-green-20260914.log`는21개 통과/exit0다. 잘못된 poolOptions 초기 호출은 실행 설정 오류로 분리했다. fresh Sol의 최종 code/spec/result ACCEPT, canonical report SHA도 정정했다.
- result.unverified: runtime identity, 실제 native/live/browser 재검증. blockers: 없음.

### pr92-migration-review-fix

- ownedPaths: monitor/toolbox.go, toolbox_test.go, app_test.go.
- worktree: `.worktrees/pr92-migration-review-fix`; branch: `agent/pr92-migration-review-fix`.
- forbiddenPaths: app.go/frontend/module/실제 사용자 파일과 나머지 경로.
- interface: private validation 정책만 변경, JSON version/mutex/Wails 계약 불변.
- acceptance: 101+101 migration 및 자료 접근, 10000초과 유효 legacy/read 보존, 새 저장 증가만 제한, direct v1 caller/partial retry/원본 보존.
- tests: 직접 영향 Go focused RED/GREEN, 전체 suite 없음.
- result.changedFiles: monitor/toolbox.go, toolbox_test.go, app_test.go; commitSHA `227bce2dfc4a5dc382b79285eeed61e740f3b1de` (통합f240e36).
- result.executedCommands/outcomes: Go1.27 `go test ./monitor -run 'Toolbox|ProjectReferences' -count=1` focused PASS, go fmt/diff-check PASS. 첫 sandbox cache 오류는 assertions 전 실패이며, escalation 후 실제202개 migration RED를 확인했다. worker 단계는 raw 로그 없는 transcript 요약 근거다. fresh Sol은 count/read/growth/partial retry/원본 보존 및 동일 ensureMigrated를 거치는 App 자료 경로를 검토해 ACCEPT했다. root의 새 통합·Windows 원시 로그는 아래에 있다.
- result.unverified: actual runtime identity/native 앱 조작. blockers: 없음.

## 이번 통합 검증

- 코드 SHA `e1fe17ff9eb640f87436339794c7a3787c2664d4`에서 마지막 `make check` 한 번, exit0: Go vet/3 packages, UI13 files/111tests, TypeScript/Vite build. `/tmp/pr92-final.8htgHa/make-check.log`에 실제 출력 보존.
- 명령: `PATH=/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin:$PATH TMPDIR=/tmp/pr92-final.8htgHa NODE_COMPILE_CACHE=/tmp/pr92-final.8htgHa/node-cache VITEST_MAX_WORKERS=1 make check`. cache 쓰기는 sandbox escalation으로 허용받았다.
- 같은 코드 SHA를 새 Windows staging에 export해 Wails2.15.0 표준 build42.619초 PASS. `go test ./monitor -run 'Toolbox|ProjectReferences' -count=1`은1.326초/exit0. `/tmp/pr92-final.8htgHa/windows.log` 보존. 실제 앱을 실행하거나 사용자 Toolbox 파일을 migration한 것은 아니다.
- artifact 위치/hash는 최종 verification 기록에 있다. 기존 staging은 삭제하지 않았다. Linux build가 비운 tracked dist/.placeholder는 원본 내용 그대로 복원했고, 제품 소스 변경은 아니다.
- 본 변경의 browser/native 실행은 새로 수행하지 않았다. 이전 visual ship/스크린샷을 새 conflict/0개 UI의 실제 화면 검증으로 확대하지 않는다. 이번 UI 근거는 컴포넌트·App 통합 테스트다.

## 남은 검증 / 운영

- 후속 변경분 최종 독립 통합 리뷰 `d6aff6bd0a50618b86d82ea4a05320d191b6ecbb`는 ACCEPT/blockers0이다. 이후 변경은 승인·게시 문서뿐이며 코드 gate를 반복하지 않는다. PR92 본문과 Issue91에 새111tests/Windows 결과를 반영한다.
- 실제 Windows 앱 clipboard/파일 열기/재시작/사용자 파일 migration/live GHES·Herdr는 여전히 미검증이다. 새 코드 Windows 결과가 없으면 이전 build를 새 검증으로 표시하지 않는다.
- 현재 sandbox는 Git 메타데이터 read-only다. 작업 worktree 생성·report 추적 해제는 도구 escalation으로 처리했고 우회하지 않았다.
- 모델/effort 설정은 Luna high·Sol medium, 실제 runtime identity는 unverified. 자동 승격 없음.
- PR92/Issue91은 In Progress, 정리 방향 유지. Wiki 페이지 발행은 이번 범위가 아니며 main 병합은 사용자에게 남긴다.
