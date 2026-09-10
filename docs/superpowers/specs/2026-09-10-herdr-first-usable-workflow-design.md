# Go/Wails 기반 GitHub·Herdr 모니터 설계

2026-09-10 사용자 정정: **ThreadDock은 Go 모니터 도구로 만든다.**
“심플하게”는 자체 실행 관리 기능을 줄이라는 의미다. 브라우저 전용 제품으로 바꾸라는 승인이 아니다.
이 문서는 같은 날짜의 브라우저/Vite 서버 중심 설계를 대체한다.

## 제품과 책임

ThreadDock은 Windows의 Go/Wails 데스크톱 모니터와 프로젝트 Skill로 구성한다.
화면은 기존 React/TypeScript를 재사용한다. Vite는 화면 개발·빌드 도구이며 제품의 조회 서버가 아니다.

| 구성 | 책임 |
|---|---|
| 중앙 관제 Agent | 목표·계획, 병렬 구현할 기능 분배, 공유 변경·의존성 조정, 결과 취합 |
| 기능별 Herdr 세션의 상위 Agent | 상세계획, native subagent를 통한 구현·독립 리뷰, 커밋·PR·handoff |
| Herdr | session·workspace·tab·pane·worktree와 상위 Agent 실행·재접속 |
| GitHub Projects | 여러 프로젝트의 진행 상태·우선순위·대기 항목 |
| Issue | 시작 이유·문제·완료 조건·결정과 변경 이유 |
| 설계 문서·PR | 구현 방법과 선택 근거·코드 변경·검증·리뷰 |
| GitHub Wiki | 사용법·설치·운영·런북·장애 대응·현재 구조 |
| Go/Wails Monitor | GitHub 기록과 로컬 관찰을 연결해 표시하고 세션 복귀·재개를 지원 |
| Skills | 위 작업을 Agent가 수행하는 공통 지침 |

## 구현 경로

React 화면 → Wails `GetMonitorSnapshot` → Go 조회·결합 → GitHub와 Herdr.
화면은 GitHub·Herdr 명령이나 인증 정보를 직접 다루지 않는다.
Go는 메모리 캐시와 마지막 성공 조회 시각을 관리한다. 영구 업무 기록은 GitHub에 남는다.

기존 Windows/WSL 경계는 유지한다. Windows Go 백엔드에서 명시적으로 설정한 WSL 배포판의
`gh`·`herdr` 읽기 명령을 인자 배열로 실행하고 결과를 Go에서 해석한다.
기존 `internal/runner`와 `internal/monitorcli`의 프로세스 호출·timeout 패턴 중 필요한 부분만 재사용한다.
기존 `agentctl project status`와 로컬 Work store를 새 데이터 공급자로 사용하지 않는다.
별도 Node 서버, HTTP bridge, 상주 daemon, 새 작업 실행 CLI를 만들지 않는다.

- GitHub: WSL의 기존 gh 인증 사용. 선택 저장소의 Issue·PR, 선택 Projects의 항목·실제 필드 조회.
- Herdr: 같은 WSL 배포판에서 사용자가 연결한 실제 세션만 읽기 전용 조회.
- Windows 경로와 WSL 경로를 혼용하지 않는다. 연결 파일·worktree 경로 확인은 그 경로를 소유하는 WSL에서 수행한다.
- 명령 문자열을 shell에 이어 붙이지 않는다. timeout·출력 제한을 두고 원문 stderr의 비밀값을 화면에 전달하지 않는다.
- 앱 설정은 저장소 목록·Projects URL·WSL 배포판·연결 파일 경로만 필요하다. 기존 엔진의 실행 profile·계약 설정을 요구하지 않는다.

**Herdr 실행 조건은 실제 환경에서 확인해야 한다.** Windows에서 `wsl.exe --exec`를 호출한다고
기존 Herdr pane의 환경이 상속되는 것은 아니다. 설치 버전의 CLI 도움말·공개 조회 계약과 정상 세션으로
데스크톱에서의 읽기 접근을 먼저 확인한다. `HERDR_ENV=1`을 임의로 넣거나 환경 조건을 우회하지 않는다.
설치 환경에서 접근이 불가능하면 Herdr 조회 불가와 이유를 표시하고 GitHub 조회는 유지한다.
이 상태를 Herdr 통합 완료로 보고하거나 브라우저 전용 제품으로 바꾸지 않는다.
기존 Node fixture의 성공은 이 Windows/WSL 경로의 증거가 아니다.

## 화면과 조회

프로젝트/저장소·업무 목록, 선택 업무의 GitHub 기록, 해당 Herdr 세션 상태·위치,
링크 열기·handoff 복사·재개 안내만 제공한다.
선택 업무의 Herdr 연결은 한 곳에 표시하고, 연결되지 않은 관찰 세션도 확인할 수 있게 한다.
아직 동작하지 않는 메뉴와 옛 엔진의 Task/Publication 표시를 새 사용 화면에서 제거한다.

GitHub 관찰과 Herdr 관찰의 성공 시각·오류를 분리한다. 한쪽 실패가 다른 쪽 결과를 숨기지 않는다.
첫 버전은 GitHub 60초, Herdr 5초 메모리 캐시를 사용하며 동일 조회의 중복 실행을 합친다.
조회 실패 시 마지막 성공 자료와 시각을 유지한다. 실패·조회 제한·인증/설정 문제는 구체적으로 표시한다.
첫 조회 한도는 저장소별 Issue/PR 및 보드 항목 각각 100개이며 부분 결과임을 표시한다.
Issue 본문과 연결 PR·검증·리뷰·문서 링크를 보여준다. Issue 댓글 전체·Wiki 본문 동기화는 범위 밖이다.
상세 기록과 Wiki 갱신은 Skills가 GitHub에서 수행한다.

## 명시적 세션 연결과 재개

[빠른 시작](../../operator/github-first-quickstart.md)의 로컬 연결 메모 형식을 재사용한다.
파일은 Git에 커밋하지 않으며, PID·lease·실행 상태·재시도 명령을 저장하는 registry로 확장하지 않는다.

- 기능: 정확한 Issue URL, 저장소, WSL worktree, session/workspace/tab/pane/Agent 이름으로 연결한다.
- 중앙: Issue를 억지로 만들지 않고 Projects 또는 저장소에 연결한다. 중앙의 worktree는 선택 사항이다.
- 선택 순서: Issue가 지정되면 그 Issue만, 아니면 지정 Project만, 아니면 저장소 수준 중앙 연결만 적용한다.
- 본문에 언급된 다른 Issue나 같은 저장소라는 이유로 기능 세션을 연결하지 않는다.
- session과 workspace/tab/pane/name 전체를 확인한다. 불완전·중복 식별자는 확인 불가로 표시한다.
- 기능 worktree와 실제 cwd는 WSL에서 canonical 경로로 대조한다. cwd의 하위 폴더는 허용하고, 경로 불일치는 불일치, 확인 실패는 확인 불가로 구분한다.
- 같은 workspace의 여러 Agent와 한 Issue의 구현·리뷰 연결을 허용한다.
- 최근 성공 목록의 대상 부재와 조회 실패를 구분한다. 캐시된 빈 목록으로 현재 부재를 단정하지 않는다.
- provider session·terminal ID·tokens·raw transcript·내부 subagent 목록은 Monitor에 전달하지 않는다.

| 관찰 | 첫 버전의 재개 지원 |
|---|---|
| working | 위치를 안내하고 해당 Herdr 세션으로 돌아가 관찰 |
| blocked | 실제 질문·승인 내용을 먼저 확인하도록 안내 |
| idle/done | 최신 GitHub 기록과 남은 일을 확인한 뒤 이어가기 |
| 최근 성공 조회에서 대상 없음 | 보존된 Git 변경·완료 기록·handoff를 확인하고 Agent가 새 세션 준비 |
| stale/offline/unknown/conflict | 먼저 실제 상태·위치를 확인하도록 안내 |

handoff에는 위치·연결/Agent 상태·관찰 시각 또는 관찰 없음·다음 행동을 넣는다.
첫 버전의 복귀 지원은 위치 안내와 handoff 복사다. 앱의 자동 prompt·Agent 생성·종료는 구현하지 않는다.
실제 실행은 기존 Herdr와 open-agent-session Skill이 맡는다. idle/done이나 Issue 닫힘을 업무 완료로 추정하지 않는다.

## 남길 것과 덜어낼 것

유지: Go/Wails 앱, React 화면, GitHub/Herdr 조회, 필요한 프로세스 경계·링크 처리, 프로젝트 Skills.
전환 구현에서 제거: `monitor/frontend/server/`의 별도 Node 조회 서버와 전용 테스트,
Vite API middleware, 브라우저 fetch fallback, 브라우저 제품 실행 안내.
Node 구현의 유효한 데이터 매핑·연결 회귀 사례는 Go 테스트로 옮긴 뒤 제거한다.

자체 scheduler/coordinator/runtime/recovery/Publisher·로컬 Work/Contract는 새 경로에 연결하지 않는다.
옛 Go 엔진 삭제는 호출·테스트·설정 의존성을 확인해 후속 정리한다. Go라는 이유로 묶어서 지우지 않는다.
사용자 호스트의 중단된 Codex 실험과 미커밋 변경은 보존한다.

## 현재 상태와 완료 조건

PR #69의 기존 Node/Vite 경로는 방향이 잘못된 구현이며 아직 Go/Wails 경로로 교체되지 않았다.
이번 변경은 설계·계획·지침 정정뿐이다. 제품 코드 제거·이식 완료를 주장하지 않는다.
Go/Wails 앱에서 실제 GitHub 업무와 Herdr 세션을 함께 보고 handoff로 돌아가는 동작을 확인해야 한다.
중앙이 독립 기능 둘을 나누고 각 세션이 구현·리뷰·PR·GitHub 기록까지 이어가는 사용 검증도 남아 있다.
Go focused 테스트, UI 테스트, Windows Wails 빌드·실행, live gh/Herdr 결과를 구분해 기록한다.
