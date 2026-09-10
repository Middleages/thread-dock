# ThreadDock 용어와 책임

현재 기준은 [첫 사용 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)다.

| 용어 | 의미 |
|---|---|
| 프로젝트 | 하나의 제품·시스템. 저장소가 하나 또는 여러 개일 수 있다 |
| 중앙 관제 Agent | 사용자와 큰 계획을 정하고 기능 세션에 배정·결과 취합하는 상위 세션 |
| 기능 업무 | 독립적으로 배정하고 검토할 기능·수정. GitHub Issue로 추적한다 |
| 기능 세션 리더 | 배정된 기능의 상세계획·구현·리뷰·GitHub 기록을 책임지는 상위 Agent |
| 내부 subagent | 기능 리더가 native 기능으로 분배한 실행자. 별도 Herdr pane을 요구하지 않는다 |
| GitHub Projects | 여러 저장소의 Issue·PR을 모아 업무 상태·우선순위를 보여주는 보드 |
| Issue | 목적·문제·완료 조건·결정·현황·handoff의 원본 |
| PR | 코드 변경·검증·리뷰·병합 기록 |
| 설계 문서 | 코드와 함께 검토할 구현 선택과 근거 |
| Wiki | 현재 사용법·구조·설치·운영·장애 대응 지식 |
| handoff | 완료 내용·남은 일·blocker·다음 행동·관련 링크. 전체 대화 복사본이 아니다 |
| Herdr session/workspace/tab/pane | Herdr가 관리하는 실행 위치. 업무의 완료 상태와 별개다 |
| 세션 연결 | Issue 또는 프로젝트와 실제 Herdr 위치·worktree의 명시적 대응 |
| Monitor | Windows Go/Wails 앱. Go가 GitHub·Herdr를 조회·결합하고 기존 React 화면에 표시 |

기능과 내부 Task는 일대일이 아니다. 내부 구현 Task는 체크리스트로 충분하며 독립 추적 가치가 있을 때만 Issue를 추가한다.
한 프로젝트를 중앙 관제 하나가 보고 여러 기능 세션을 운영할 수 있다.
같은 workspace에 Agent가 둘 이상 있어도 지정된 pane 연결이 명확하면 정상이다.

## 원본과 상태

업무 상태·우선순위는 GitHub, 실행 상태는 Herdr가 원본이다.
관찰 결과에는 각 원본의 마지막 성공 시각을 따로 보관한다.
idle/done은 업무 완료가 아니고, Issue가 닫혔다고 Agent를 종료하지 않는다.
연결 없음은 미연결, 조회 실패는 확인 불가, 지정 대상 부재는 대상 없음으로 구분한다.
대상이 불명확하면 현재 위치를 확인해 연결을 수정하며 새 Agent를 자동 중복 시작하지 않는다.

Vite는 화면 개발·빌드 도구다. Node 조회 서버나 브라우저 전용 실행은 제품 경로가 아니다.

## 이전 용어

Contract v2, Repository Run, Invocation, Artifact, Repair Budget, Go Task Gate,
Final Manifest, Finalize는 기존 Go 실행기 코드의 용어다.
신규 Skill·GitHub 업무 조회·세션 연결의 필수 입력이나 완료 조건으로 사용하지 않는다.
과거 설계와 코드는 이력·재사용 참고용이며 신규 경로를 규정하지 않는다.

