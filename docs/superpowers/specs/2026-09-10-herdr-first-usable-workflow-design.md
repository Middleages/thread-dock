# GitHub 중심 첫 사용 설계

2026-09-10 사용자의 역할 표와 “심플하게 구현해서 당장 사용” 요청을 반영한 현재 기준이다.
[ADR 0008](../../adr/0008-github-first-skills-before-engine.md)이 이전 엔진 의존성을 대체한다.

## 목적과 역할

[PRODUCT.md](../../../PRODUCT.md)의 역할 표가 최상위 기준이다.
중앙 관제는 큰 계획·기능 분배·통합 조정을, 각 기능 세션은 상세계획·native subagent 구현·독립 리뷰를 맡는다.
Herdr는 세션을 관리하고 GitHub는 업무와 문서를 보관한다.

## 지금 사용할 흐름

1. 기존 프로젝트·저장소·Projects 보드를 확인하고 Issue의 목적·완료 조건을 읽는다.
2. 중앙은 공유 인터페이스를 먼저 정한 뒤 독립 기능을 나눈다. Issue URL, 범위, 의존성, branch/worktree, 완료 조건을 기능 리더에게 준다.
3. 기능별 Herdr 세션을 재사용하거나 승인 범위에서 연다. 사용 가능한 실제 ID를 Issue와 로컬로 연결한다.
4. 기능 리더가 내부 subagent로 구현하고, 별도 문맥의 reviewer가 변경 SHA·diff·완료 조건·검사 결과를 검토한다.
5. 기능 리더/구현자는 변경을 커밋하고 검증·리뷰 결과와 함께 PR을 준비한다. 중앙은 공통 변경·통합 순서를 조정한다.
6. Issue에는 주요 결정·blocker·handoff, Projects에는 실제 상태·우선순위, PR에는 검증·리뷰를 남긴다.
7. 사람이 병합한 뒤 현재 사용법·운영 지식을 Wiki에 반영한다. 문서 변경이 불필요하면 이유를 기록한다.
8. 새 대화는 Issue·PR·handoff를 읽고 Git 변경·실제 세션을 확인한 뒤 미완료 부분을 이어간다.

세션 내부 Task마다 Issue, JSON 계약, Go 승인, 발행 receipt를 만들지 않는다.
이미 승인된 계획의 통상적인 세션 배정·구현·기록은 다시 묻지 않는다.
실제 도구 승인, 대상 변경, 범위 확장과 불명확한 쓰기 결과는 별도 판단한다.

## 첫 Monitor 데이터 경로

입력은 사용자가 지정한 GitHub Projects URL, 관련 저장소 URL, 선택적 로컬 세션 연결이다.
읽기 adapter는 설치된 gh 또는 지원되는 GitHub API를 사용한다.
첫 브라우저 경로는 기존 React 화면, Vite의 로컬 GET endpoint, gh 조회로 구성한다.
THREADDOCK_REPOS에 OWNER/REPO 목록, THREADDOCK_PROJECTS에 선택적 보드 URL 목록을 지정한다.
현재 Wails binding이 있으면 기존 데스크톱 경로를 유지하며 브라우저 경로와 완성 범위를 구분한다.
호스트의 gh 인증을 사용하고 API를 loopback에서만 제공한다. GitHub 쓰기나 Agent 실행 endpoint는 만들지 않는다.
Projects의 항목·상태·우선순위, Issue 본문·최근 기록, 연결된 PR checks/reviews/merge, 설계·Wiki 링크를 읽는다.
모든 페이지를 읽거나 조회 제한과 부분 결과임을 표시한다. PR의 단순 언급은 연결/병합의 근거로 추정하지 않는다.

캐시는 원본이 아니다. GitHub read가 성공한 시각과 Herdr observe가 성공한 시각을 각각 유지한다.
Herdr가 없어도 GitHub 업무를 보여주고, GitHub가 실패해도 현재 Herdr 관찰과 마지막 업무 캐시를 구분해서 보여준다.
한쪽 재조회 성공으로 다른 쪽의 시각을 갱신하지 않는다.
Projects를 사용할 수 없는 호스트/권한이면 설정 문제를 표시하고 해당 저장소 Issue 조회로 제한됨을 명시한다.
로컬 Work store, Contract v2와 Go Publisher는 이 경로의 dependency가 아니다.

## 세션 연결

로컬의 선택적 .threaddock/sessions.json에 사람이/중앙 Agent가 확인한 위치를 적는다.
[빠른 시작](../../operator/github-first-quickstart.md)에 최소 형식과 보관 방법을 둔다.
이 파일은 연결 메모다. lease, PID, 실행 명령, 자동 재시도, 실행 상태의 원본을 담지 않는다.
첫 Skill 사용은 파일 없이도 가능하며 확인한 위치를 로컬 메모로 남길 수 있다.

기능은 Issue URL과 정확한 host/repository, worktree, session/workspace/tab/pane/Agent 이름으로 연결한다.
중앙 관제는 프로젝트 또는 저장소에 연결하며 가짜 Issue·Task를 만들지 않는다.
하나의 Issue가 구현/검토 등 여러 명시적 연결을 가질 수 있고, 같은 workspace에 여러 Agent가 있어도 정상이다.
경로는 연결의 일치 확인에 사용하며 경로나 이름만으로 업무를 자동 배정하지 않는다.
현재 연결이 더 이상 존재하지 않으면 위치 재선택을 안내한다. 미연결과 관찰 실패를 대상 종료로 단정하지 않는다.

## 재개

- working: 해당 세션으로 돌아가 관찰한다.
- idle/done: 최신 기록과 미완료 부분을 확인하고 이어갈 메시지를 준비한다.
- blocked: 실제 화면의 질문·승인을 먼저 확인한다.
- 대상 부재: Git 변경과 완료 기록을 보존하고 새 기능 세션에 handoff한다.
- 조회 실패/오래된 관찰/불명확한 위치: 먼저 조회하거나 정확한 대상을 선택한다.

Monitor 첫 UI는 GitHub 링크, handoff 복사, Herdr 위치 안내를 제공한다.
Agent에게 시작·재개를 맡기면 open-agent-session Skill이 설치된 Herdr 도움말과 실제 환경에 따라 실행한다.
prompt 전송 실패는 성공으로 표시하지 않고 자동 반복하지 않는다.
완료 기록은 재개 자료에 남기되 완료된 작업을 다시 구현하도록 배정하지 않는다.

## 범위와 검증

먼저 Skill 운영 경로를 제공하고, 기존 UI에 GitHub 조회, 이후 명시적 Herdr 연결을 추가한다.
새 scheduler·process registry·내부 subagent UI·범용 발행/복구 엔진·Monitor 채팅을 추가하지 않는다.
Go 재사용이 더 많은 상태·계약을 요구하면 첫 구현에서 사용하지 않는다.

Skill은 실제 요청 시나리오와 구조 검사를 한다. 문구 일치 테스트로 행동을 보증하지 않는다.
Monitor는 GitHub-only 업무, 두 프로젝트/여러 페이지, 같은 workspace의 두 Agent,
양쪽 독립 실패, 실제 재개 경로를 확인한다.
최종 사용 확인은 중앙이 작은 독립 기능 둘을 배정해 구현·리뷰·PR·GitHub 기록까지 이어가는 것이다.
실제 Windows/Herdr 검증과 fixture 테스트를 별도로 보고한다.
