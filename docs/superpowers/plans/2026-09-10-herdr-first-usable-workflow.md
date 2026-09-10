# Go/Wails 모니터로 바로잡는 구현 계획

> **상태:** Task 1·2와 Task 3의 제품 구현은 PR #69로 main에 병합됐다. 아래 체크는 병합된 구현과
> 검증 근거를 반영하며, 실제 Projects를 사용한 독립 기능 두 개의 end-to-end 운영 검증은 남아 있다.

**Goal:** Windows Go/Wails 앱에서 GitHub 업무와 Herdr 실행 상태를 함께 확인하고 작업 재개를 지원한다.
**Architecture:** 기존 React 화면 → Wails GetMonitorSnapshot → Go 조회·결합 → WSL의 gh/Herdr. 실행·개발 조정은 Herdr와 Agent가 맡는다.
**Tech Stack:** 기존 Go·Wails v2·React/TypeScript·Vite(화면 빌드만)·WSL·gh·Herdr.
**Spec:** [Go/Wails 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md).

## 기준과 작업 정책

- PR #69의 최종 head는 `f35e248`이고 main merge commit은 `3f4bebf`다.
- PR의 옛 Node/Vite 조회 구현은 이식 참고 자료였으며 최종 제품 경로에서 제거됐다.
- Sol medium이 범위·인터페이스·분배·검토를 맡고 Luna high가 구현한다. 모델 자동 승격은 하지 않는다.
- 병렬로 작성하는 기능은 독립 worktree/branch를 사용한다. 공유 Go/TS wire를 먼저 고정하고, 독립 작업만 병렬화한다.
- 구현자는 focused 테스트와 self-review 후 커밋한다. fresh Sol이 고정 diff를 검토한다.
- 동일 코드·명령·환경의 검증을 반복하지 않는다. worker별 full suite는 금지한다.
- reviewed SHA `325db89`의 최종 `make check`와 Windows healthy-path 근거는 반복하지 않는다. 후속 code PR은 마지막 통합 gate를 새 SHA에서 한 번 수행한다.
- 기존 Codex runtime 실험/dirty 파일은 reset·삭제·재개하지 않는다. main 병합은 사람에게 남긴다.

## Task 1. Go/Wails에서 GitHub 업무가 보이는 경로 완성

**Files:** `monitor/main.go`, `monitor/app.go`, `monitor/app_test.go`, 새 `monitor/github.go`, `monitor/github_test.go`,
새 `monitor/snapshot.go`, `monitor/commands.go`, `monitor/commands_test.go`,
`monitor/frontend/src/{bindings.ts,bindings.test.ts,types.ts,App.tsx,monitor.test.tsx}`.
현재 Monitor는 `internal/runner`를 직접 사용한다. 옛 `internal/monitorcli/client.go` bridge는 후속 정리 대상이다.

**공유 계약:** 화면 진입점 `GetMonitorSnapshot`은 유지한다. Go의 새 화면용 snapshot은
현재 TS의 GitHub 프로젝트·업무·링크·검증·관찰 시각에 대응하며 Contract v2 타입을 import하지 않는다.
옛 Task/Publication 모델을 새 Go 모델의 필수 필드로 옮기지 않는다.
설정은 저장소·Projects URL·WSL 배포판·선택 연결 파일 경로로 제한한다.

- [x] 설치된 Go/Wails와 기존 Windows entrypoint·WSL 실행 설정을 확인한다. `monitor/main.go`는 windows build tag를 사용한다.
- [x] Go app 테스트에서 가짜 조회 원본을 주입해 GetMonitorSnapshot이 GitHub 업무를 그대로 전달하는 실패 사례를 작성한다.
- [x] 가짜 command runner로 배포판·명령 인자·timeout을 검증한다. 두 저장소의 같은 Issue 번호, Projects 접근 실패, 마지막 성공 캐시와 시각을 검증한다.
- [x] `monitor/frontend/server/github-monitor.mjs`의 실제 gh 필드 매핑과 회귀 사례를 Go로 옮겼다. gh 인증은 선택 WSL 배포판의 인증을 사용한다.
- [x] Wails App에 Go 원본을 연결하고 TS와 Go JSON 필드를 함께 맞췄다. 브라우저 fetch fallback은 제거했다.
- [x] 선택 업무·GitHub 링크·PR 검증/리뷰·Projects 실제 필드·handoff를 Wails 경로에 연결했다.
- [x] 변경 파일의 focused Go/UI 테스트와 frontend build 결과를 기록했다.
- [x] self-review·커밋과 독립 리뷰를 완료했다. Task 1 결과는 Go/Wails GitHub 조회 범위로 기록했다.

**회귀 사례:** `acme/app#1`과 `acme/api#1`은 다른 URL이다. 한 보드 실패가 저장소 Issue 성공을 지우지 않는다.
본문의 PR 언급을 검증된 연결이나 병합으로 추론하지 않는다. 조회 한도 100개를 숨기지 않는다.

## Task 2. 같은 Go 경로에 Herdr 관찰과 재개 안내 연결

**Files:** 새 `monitor/herdr.go`, `monitor/herdr_test.go`, Task 1의 `commands.go`, `snapshot.go`, `app.go`,
`monitor/frontend/src/{types.ts,App.tsx,monitor.test.tsx}`, `docs/operator/github-first-quickstart.md`.
Task 1의 Wails 진입점·명령 경계를 재사용한다. 새 runtime/daemon/HTTP bridge는 만들지 않는다.

- [x] 정상 세션으로 설치 Herdr의 버전·도움말·agent list 응답과 Windows Go→WSL 읽기 접근을 확인했다. 실패한 옛 invocation은 재사용하지 않았다.
- [x] 데스크톱 프로세스의 실제 접근 조건을 확인했다. HERDR_ENV를 임의 설정하지 않았고 fixture와 live 결과를 구분했다.
- [x] Node Herdr source의 회귀 사례를 Go 테스트로 옮겼다. 실제 응답에서 명시한 이름·상태·workspace/tab/pane·cwd만 남긴다.
- [x] WSL 연결 파일과 canonical worktree/cwd를 WSL에서 확인한다. Windows에서 Linux 경로를 realpath 처리하지 않는다.
- [x] Issue→Project→저장소 중앙 연결의 배타적 우선순위와 session/workspace/tab/pane/name 전체 일치를 적용했다.
- [x] GitHub와 Herdr의 캐시·실패·성공 시각을 독립 유지하고, GitHub가 비어도 로컬 관찰을 표시한다.
- [x] 상태별 위치·시각·다음 행동을 화면과 handoff에 함께 넣었다. 시작/입력/종료 버튼은 추가하지 않았다.
- [x] Herdr/Snapshot focused Go 테스트와 영향받는 화면 테스트, self-review·커밋·독립 리뷰를 완료했다.

**필수 회귀 사례:** 같은 저장소의 Issue A/B 양쪽 binding에 repository와 issueUrl이 모두 있어도 서로 섞이지 않는다.
A 본문의 B 링크는 연결 근거가 아니다. 두 세션의 같은 pane ID는 다른 대상이다.
중앙의 worktree 생략은 허용하지만 Agent 식별자 생략은 connected가 아니다.
실패 전 캐시가 빈 배열이어도 현재 missing으로 보고하지 않는다. blocked handoff는 질문 확인과 성공 관찰 시각을 포함한다.

## Task 3. 잘못 추가한 제품 경로 제거와 실제 앱 인수

**Files:** 삭제 `monitor/frontend/server/`의 Node adapter·전용 테스트,
수정 `monitor/frontend/vite.config.ts`, `src/{App.tsx,styles.css,safe-url.ts,bindings.ts}` 및 영향 테스트,
`Makefile`, `README.md`, `HANDOFF.md`, `docs/operator/github-first-quickstart.md`.
Node source 삭제 전 Task 1/2에서 필요한 회귀 동작이 Go/UI 검사로 옮겨졌는지 확인한다.

- [x] Vite middleware와 `/api/github-monitor` fetch 경로를 제거했다. React/Vite 빌드와 Wails 외부 링크 기능은 유지한다.
- [x] 기능이 없는 메뉴·옛 Work/발행 화면·중복 Herdr 패널을 정리했다. 업무·기록·세션·handoff만 남겼다.
- [x] Makefile에 실제 Go 검사와 Node/UI 검사·프런트엔드 빌드를 포함했다. 제거한 Node server 테스트는 더 이상 호출하지 않는다.
- [x] 전체 Go 트리에 남은 기존 엔진 테스트를 숨기지 않고 통합 gate에 포함했다. 관련 엔진 삭제는 아래 후속 범위다.
- [x] reviewed SHA `325db89`에서 최종 `make check`를 한 번 실행하고, Windows 표준 `wails build`와 빌드 앱의 healthy 실행을 별도 확인했다.
- [x] Vite/Node 조회 서버 없이 앱이 실제 GitHub·Herdr를 조회하고 handoff를 복사하는지 확인했다.
- [ ] 중앙에서 독립 기능 둘을 배정해 각 Herdr 세션의 구현·리뷰·PR·Projects/Issue 기록을 확인한다. 다른 프로젝트를 보고 돌아와 세션 위치와 남은 일을 찾는다.
- [x] 설치·실행 방법과 검증 제한을 실제 결과로 문서화했다. PR #69는 최종 head `f35e248`에서 main의 `3f4bebf`로 병합됐다.

## 후속 정리: 기존 실행 엔진

Go/Wails 실사용 경로 완성 후 `cmd/agentctl`과 internal의 coordinator/scheduler/runtime/recovery/publication/state 등
옛 엔진에서 새 모니터가 참조하지 않는 모듈을 import·테스트·설정·문서 참조 기준으로 분류한다.
필요한 runner·경로/링크 기능은 유지하고 죽은 코드·전용 테스트·fixture·옛 계획을 함께 정리한다.
dependency inventory로 production/test importer가 없는 경계를 증명하고 작은 삭제 slice부터 진행한다.
