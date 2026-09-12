# compact Monitor와 전역 Toolbox 검증

## 범위와 상태

- 작업: [Issue91](https://github.com/Middleages/thread-dock/issues/91), branch `agent/compact-monitor-toolbox`.
- 선행 PR90은 main `611bd6a`에 병합 완료했다. 이번 UI/저장소 변경의 main 병합과는 별개다.
- 저장소/바인딩/자료/Drawer/화면 통합/Herdr/검증 manifest는 Luna 구현과 독립 Sol 리뷰를 거쳤다. 고정 원본·통합 SHA와 실패/수정은 [ledger](../../operator/2026-09-12-compact-monitor-ledger.md)에 있다.
- Task별 focused 결과는 전체 제품 gate 또는 native 성공을 뜻하지 않는다. 최종 gate·Windows build와 최종 화면 판정은 아래 별도로 기록한다.

## 경계별 근거

| 경계 | 현재 근거 | 제한 |
|---|---|---|
| Go JSON 저장·migration | Linux 임시 파일 tests; global-first/재시도/잘못된 파일 보존/최초 App 저장 검증 | 실제 사용자 파일 migration 아님 |
| Wails ↔ TS | 정확한 네 method/payload, missing binding 오류·fetch fallback 부재 tests | 브라우저에서 native Go를 호출한 E2E 아님 |
| React UI | Task별 focused tests와 typecheck, manifest13파일 독립 확인 | 마지막 full gate는 아직 미실행 |
| Browser fixture | 코드 `516a40a`에서 주요 사용자 흐름 PASS, console errors0, viewport414/scrollWidth414, Drawer380.875px | Chrome140/Playwright1.55, `--disable-gpu`, 합성 Wails boundary |
| Visual review | desktop1280×900/narrow414×896 캡처를 fresh td_reviewer가 검토, `disposition: fix` | 다섯 수정과 재캡처 판정 대기 |
| Windows 도구 | 실제 Go1.27.0/Node26.8.1/Wails2.15.0 version 명령 exit0 | 새 SHA package build 아님 |
| Windows native | 미실행 | native computer-use 미노출; Windows GPT app에서 실제 앱 확인 필요 |

Browser fixture는 실제 GitHub/Herdr 결과가 아님을 화면에도 표시했다. 메모리 내 Wails 대역으로 자료 열기·경로 복사, 명령 copy-only, 저장 실패 draft/data 보존과 재시도, 프로젝트 바로가기와 Todo key/count, 완료 포함·unknown 재지정, focus 복원·닫기, 설정 진입, Herdr explicit detail/mixed severity를 확인했다. 제품의 Node/HTTP 조회 서버나 browser fetch fallback을 추가하지 않았다.

## 실패와 환경 구분

- 첫 기본 headless Chromium은 `page.goto`의 DOMContentLoaded 60초 timeout, screenshot 획득도 실패했다. Vite HTML/entry script는 HTTP200이었고 GPU process의 높은 CPU 사용을 관찰했다. `--disable-gpu` 환경 대조에서 렌더링이 가능해졌다. 근본적인 호스트 GPU 문제 해결을 주장하지 않는다.
- 다음 검증에서는 완료 체크 직후 미완료 목록에서 제거된 checkbox를 Playwright `check()`가 계속 기다려 timeout이 났다. 캡처에는 이미 Todo 감소와 목록 갱신이 보였다. `click()` 뒤 저장된 fixture 상태/key/count를 검사하도록 harness를 고쳤고 전체 흐름은 통과했다. 제품 버그 수정으로 기록하지 않는다.
- Task3b worker가 언급한 중간 timeout은 정확 invocation/로그가 제출되지 않아 `indeterminate environment event`다. 별도로 확인된 최종21tests GREEN을 이 미기록 실패와 혼합하지 않는다.
- Task4 AppScope의 최초 새 assertion 실패는 throwing `getByRole(...).not` 사용 오류였다. `queryByRole`로 고친 coverage-only 증거이며 제품 regression RED가 아니다.
- 원 Task1 historical RED runtime path와 Task3a RED shell exit 미기록은 그대로 제한에 남긴다.

## 시각 판정과 detector

전문 Impeccable reviewer role은 노출되지 않아 사용자 지정 fresh `td_reviewer` Sol을 독립 하위 Agent로 사용했다(인라인 자기검토 아님). 승인된 code-led 구조와 기존 cobalt/slate/cloud·Segoe UI 계열을 기준으로 판단했다. 새로운 comp/무작위 seed/시각 정체성은 만들지 않았다.

수정 요청: toolbar cascade/label wrapping, 불필요한 Drawer eyebrow, close glyph→SVG, 자간 하한, 비동기 첫 목록 autoFocus. 같은 Luna에게 단일 fix를 배정했고, 마지막 항목은 열린 Drawer focus를 비동기 목록이 빼앗지 않는 회귀로 검증한다.
Detector는 한 번 실행해 기존 3px 주의 안내 border 다섯 곳을 warning으로 보고했다(exit2). 사용자 지정 기존 시각 체계의 상태 표현으로 유지하며 clean detector라고 주장하지 않는다. detector를 재실행하지 않는다.

## 최종 검증 기록

- 최종 Linux `make check`: 대기.
- Windows 표준 Wails build: 대기.
- Windows 임시 파일 storage/migration focused tests: 대기.
- 수정 후 동일 viewport 화면 판정: 대기.
- 실제 Windows 앱의 오류/clipboard/파일 열기/재시작·migration 및 live GHES/Herdr: 미검증.

실제 앱 검증 전 사용자 데이터 백업을 보존해야 한다. 기존 worktree·Windows staging·중단된 codex-runtime dirty 파일은 삭제/초기화하지 않는다. 이 문서의 fixture/build 근거로 과거 `325db89` native healthy 결과를 새 SHA에 옮겨 적지 않는다.
