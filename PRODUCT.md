# Product

<!-- impeccable:product-schema 1 -->

## Purpose

멀티 세션과 서브에이전트로 기능을 개발하고, GitHub 기록과 로컬 실행 상태를 함께 보며 쉽게 이어간다.
첫 사용은 기존 GitHub·Herdr·Agent와 Skill로 시작한다. Monitor는 그 사용 흐름을 편리하게 만든다.

## 책임

| 구성 | 맡길 역할 |
|---|---|
| 중앙 관제 Agent | 목표·큰 계획, 기능 분배, 의존성과 공유 변경 조정, 결과 취합 |
| 기능 세션 리더 | 상세계획, native subagent 구현·독립 리뷰, 수정, 커밋·PR 준비 |
| Herdr | 세션·workspace·tab·pane·상위 Agent 실행과 재접속 |
| GitHub Projects | 여러 프로젝트의 업무 상태·우선순위·대기 항목 |
| Issue | 시작 이유, 문제, 완료 조건, 결정과 변경 이유, handoff |
| 설계 문서·PR | 구현 방법과 선택 근거, 코드 변경, 검증·리뷰 결과 |
| Wiki | 사용법, 설치·운영, 런북, 장애 대응, 현재 구조 |
| ThreadDock Monitor | GitHub 업무와 Herdr 상태를 결합해 보여주고 세션으로 돌아가기 지원 |
| 사용자 | 목적·범위·중요한 결정, 필요한 질문 응답, main 병합 |

## 첫 사용 범위

한 사용자의 Windows/WSL 환경, GitHub와 Herdr, 기존 Codex/OpenCode를 사용한다.
중앙은 독립적인 기능 둘을 나눌 수 있고, 기능별 작성 worktree는 분리한다.
읽기 전용 리뷰 세션이나 중앙 관제 세션이 같은 workspace에 있다고 오류로 처리하지 않는다.
기능 내부 subagent를 Monitor의 Task나 pane으로 복제하지 않는다.

GitHub 기록과 Agent의 Git 작업은 승인 범위에서 gh·GitHub MCP·Git으로 수행한다.
중앙이 이미 승인받은 기능을 배정할 때마다 다시 승인을 요구하지 않는다.
범위 변경, 실제 실행 환경의 승인 요청, 불명확한 외부 쓰기 결과는 별도로 다룬다.

## Monitor 최소 범위

Projects의 업무·상태·우선순위, Issue 목적·최근 handoff, PR 검증·리뷰·병합 상태,
설계·Wiki 링크를 먼저 조회한다. 조회 시각과 누락·부분 조회를 표시한다.
로컬에는 GitHub 업무와 실제 Herdr 위치의 명시적 연결만 둔다.
GitHub만 있는 업무와 연결되지 않은 세션도 표시한다.
중앙 관제는 프로젝트 수준, 기능 세션은 Issue 수준으로 연결한다.

첫 UI는 조회·링크 열기·handoff 복사·Herdr로 돌아가는 안내까지다.
새 채팅, prompt 전송 버튼, 자체 scheduler·복구·승인·발행 엔진은 첫 사용에 포함하지 않는다.
기록 조회 실패와 Herdr 조회 실패를 따로 표시하며 한쪽 실패가 다른 쪽의 관찰을 숨기지 않는다.

## 완료 기준

작은 독립 기능 둘을 중앙이 배정하고 각각 구현·독립 리뷰·PR로 전달한다.
GitHub Projects·Issue·설계·PR·필요한 Wiki 갱신이 이어지고, 다른 프로젝트로 갔다 돌아와
완료 내용·막힌 점·다음 행동을 찾아 같은 세션 또는 handoff로 이어간다.
Skill 경로 사용, 실제 Agent 성공, GitHub 반영, Monitor UI 검증은 각각 따로 보고한다.

## 현재 구현과 재사용

project-template에는 위 흐름의 Skill을 제공한다.
첫 브라우저 Monitor는 기존 React/TypeScript 화면과 로컬 Vite/gh 조회를 사용한다.
Projects·Issue·PR을 읽고 링크·handoff 복사를 제공한다. Wails 앱은 기존 로컬 Work 경로를 유지한다.
Herdr live 통합은 후속 범위이며 지금은 Skill을 통해 해당 세션으로 돌아간다.
기존 Contract v2·Work 상태·Go Task Gate·Final Manifest·Publisher는 필수 절차가 아니다.
관련 실험과 사용자 미커밋 변경은 보존하되 신규 사용에 연결하지 않는다.
