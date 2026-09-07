# ThreadDock

개발자의 초기 요청을 구체적인 작업으로 정제하고, GitHub에 기록하며, 로컬 에이전트가 구현과 검증을 수행하는 개발 운영 문맥이다. 배포·관측 체계는 후속 범위이며 개인 비서 체계는 이 문맥에 포함하지 않는다.

## Language

**Operator**:
깊은 Git·인프라 지식을 전제로 하지 않고 작업 인터뷰 승인, 실행 관찰과 production 배포 결정을 수행하는 사용자.
_Avoid_: 시스템 관리자, Agent

**개발 요청 (Development Request)**:
개발자가 구현하려는 기능이나 수정하려는 버그를 아직 정제하지 않은 형태로 설명한 것.
_Avoid_: Task, Issue, 요구사항

**작업 인터뷰 (Work Interview)**:
에이전트가 개발 요청의 목적, 범위, 수용 조건과 위험을 사용자와 함께 명확하게 만드는 대화.
_Avoid_: 프롬프트 보완, 요구사항 입력

**업무 기록 (Work Record)**:
작업의 목적, 결정, 진행 상태와 결과를 GitHub Issue와 Pull Request에 지속적으로 남긴 기록.
_Avoid_: 실행 상태, 에이전트 상태

**실행 세션 (Execution Session)**:
개발자 PC에서 특정 작업을 수행하는 에이전트 프로세스와 격리된 작업 공간의 일시적인 실행 단위.
_Avoid_: 업무 상태, Issue

**에이전트 호출 (Agent Invocation)**:
하나의 역할 packet을 하나의 Worktree에서 실행하고 구조화된 결과를 반환하는 일회성 실행. 영속 세션을 만들 수도 있지만 Codex 호출은 완료 후 세션을 남기지 않는다.
_Avoid_: 실행 세션, RUN, Task

**실행 프로필 (Execution Profile)**:
Contract가 역할별 실행 정책을 지칭하는 논리 ID. 실제 Runtime, native profile과 모델 설정은 각 Workstation의 로컬 구성에서 해석한다.
_Avoid_: 모델 이름, Agent 이름, provider 설정

**에이전트 런타임 (Agent Runtime)**:
역할 packet을 실행하고 구조화된 Artifact를 반환하는 로컬 실행 체계. Codex와 OpenCode는 같은 실행 interface 뒤의 서로 다른 Runtime이다.
_Avoid_: 모델, Orchestrator, Adapter

**실행 세션 은퇴 (Execution Session Retirement)**:
수정이나 복구가 더 필요하지 않은 RUN에서 에이전트 프로세스와 terminal workspace를 닫되 Worktree, 실행 근거와 업무 기록은 보존하는 lifecycle 전환.
_Avoid_: cleanup, Issue 종료, Worktree 삭제

**실행 근거 정리 (Execution Artifact Cleanup)**:
보존 기간이 지난 완료 RUN의 clean Worktree와 로컬 실행 상태를 명시적으로 제거하는 운영 행위.
_Avoid_: 실행 세션 은퇴, Agent 종료, 자동 삭제

**감독형 자율 실행 (Supervised Autonomous Run)**:
사용자가 작업 인터뷰 결과와 Issue 묶음을 승인하면 에이전트가 구현, 검증, 리뷰와 Pull Request 준비까지 수행하고 사람이 main 병합을 결정하는 실행 방식.
_Avoid_: 완전 무인 운영, 수동 실행

## Work Structure

**Parent Issue**:
하나의 개발 요청이 의도한 전체 결과와 공통 수용 조건을 보존하는 최상위 업무 기록. 최종 Pull Request 하나와 대응한다.
_Avoid_: Epic, 실행 Task

**Child Issue**:
Parent Issue의 결과를 위해 독립적으로 구현하고 검증할 수 있으며 별도 추적 가치가 있는 작업 단위.
_Avoid_: 세부 실행 단계, 체크리스트 항목

## Agent Roles

**Orchestrator**:
승인된 Contract의 Task DAG, Worktree, 실행 순서, 검증과 통합 상태를 결정적으로 관리하는 역할.
_Avoid_: Planner, Main Agent, Builder

**Planner**:
개발 요청과 필요한 CodeMap을 바탕으로 Contract와 Task DAG를 만드는 역할.
_Avoid_: Orchestrator, Builder

**Scout**:
대규모 작업에서 Planner가 지정한 질문에 필요한 경로, symbol, 의존성과 위험만 찾아 CodeMap으로 반환하는 조건부 역할.
_Avoid_: Planner, Builder, 전체 저장소 요약

**Builder**:
할당된 Child Issue 또는 실행 작업의 수용 조건을 구현하고 검증 근거를 만드는 역할.
_Avoid_: Orchestrator, Reviewer

**Reviewer**:
구현 세션과 분리된 문맥에서 변경 결과와 검증 근거를 독립적으로 판단하는 역할.
_Avoid_: Builder, 사람 승인자

**Documenter**:
승인된 통합 결과와 관련 문서만 입력으로 받아 저장소 문서 또는 Wiki 변경안을 만드는 조건부 역할.
_Avoid_: Publisher, Reviewer, 전체 대화 요약기

**Main Agent**:
Planner와 Orchestrator를 호출하고 Final Manifest를 근거로 GitHub 업무 기록과 사람용 handoff를 관리하는 대화 역할.
_Avoid_: Orchestrator, Builder, GitHub Adapter

**CodeMap**:
Planner의 구체적인 질문에 답하는 관련 경로, symbol, 의존성과 위험의 제한된 탐색 Artifact.
_Avoid_: 전체 코드베이스 덤프, 실행 계획

## Integration

**Integration Branch**:
한 Parent Issue에 속한 여러 Builder 결과를 모아 전체 수용 조건과 회귀를 검증하는 임시 변경선.
_Avoid_: main, Builder branch

**Merge Gate**:
최종 Pull Request를 사람에게 병합 가능 상태로 제시하기 전에 반드시 만족해야 하는 수용 조건, 독립 리뷰, 최신 HEAD 검증과 저장소 보호 규칙의 집합.
_Avoid_: 권고사항, 프롬프트 체크리스트

**Final Manifest**:
승인된 Task commit, review, integration commit, 검증 결과와 문서 필요 여부를 Main Agent에 전달하는 구조화된 실행 Artifact.
_Avoid_: terminal transcript, Audit Comment, Contract

**Finalize**:
사람이 Pull Request를 병합한 뒤 Main Agent가 예상 merge SHA를 확인하고 Issue 종료, Project Done과 준비된 문서·Wiki 반영을 완료하는 후속 단계.
_Avoid_: main 병합, 실행 세션 은퇴, cleanup

**Protected Change**:
데이터 손실, 인증·권한, 배포, 공급망 또는 공개 계약에 중대한 영향을 줄 수 있어 Merge Gate를 통과해도 사람의 병합 확인이 필요한 변경.
_Avoid_: 일반 변경, 테스트 실패

**Blocked Run**:
두 번의 자동 수정으로도 Reviewer의 차단 의견을 해소하지 못했거나 작업 계약의 재승인이 필요해 자동 진행을 종료한 감독형 자율 실행.
_Avoid_: 실패한 Issue, 일시 정지

**Repair Round**:
Reviewer 또는 CI가 발견한 변경 결함을 Builder가 고치는 한 번의 자동 수정 주기. 한 실행에서 합계 두 번까지만 허용한다.
_Avoid_: Recovery Attempt, 최초 구현

**Recovery Attempt**:
미완료 작업의 Agent가 예기치 않게 idle·done 상태가 되거나 종료됐을 때 계속 지시 또는 session 재개로 실행을 되살리는 시도. 실제 진전이 생기면 횟수를 초기화하며 연속 세 번 실패하면 Blocked Run이 된다.
_Avoid_: Repair Round, 재구현

**ThreadDock Monitor**:
감독형 자율 실행의 Issue 관계, 단계, Agent 활동, 변경선, 검증·병합 결과와 연결된 CI/CD 상태를 보여주며 중단·재개·재시도만 제공하는 관찰 화면.
_Avoid_: Orchestrator, 작업 인터뷰 UI, terminal transcript

**Local Orchestrator**:
WSL에서 작업 계약, 실행 상태와 정책을 소유하고 Herdr·OpenCode·GitHub Enterprise Server를 연결하는 실행 모듈.
_Avoid_: Run Monitor, Agent, GitHub Project

**Conversation Interface**:
Operator가 OpenCode와 자연어로 개발 요청을 구체화하고 Issue 묶음을 승인하는 사람용 interface.
_Avoid_: Orchestrator CLI, Run Monitor

**Orchestrator CLI**:
Local Orchestrator의 시작, 상태 조회, 중단, 재개와 재시도를 결정적 명령과 구조화된 결과로 제공하는 interface. OpenCode와 Run Monitor가 함께 사용한다.
_Avoid_: OpenCode 대화, terminal transcript, 별도 HTTP 서버

**CI Run**:
특정 commit 또는 Pull Request의 변경을 검증하고 그 결과를 업무 기록에 연결하는 GitHub Actions 실행.
_Avoid_: Agent 로컬 검증, Deployment

**Focused Verification**:
구현 loop에서 변경된 범위와 직접 관련된 package를 대상으로 수행하는 빠른 검증.
_Avoid_: Full Suite

**Task Gate**:
작업을 다음 단계로 넘기기 전에 Task verification과 관련 static check를 확인하는 검증 관문.
_Avoid_: Merge Gate, Reviewer 판단

**Full Suite**:
저장소 전체에 적용되는 test와 static check의 검증 묶음.
_Avoid_: Focused Verification

**Wave End Verification**:
여러 Task를 한 실행 wave에서 통합한 뒤 Full Suite를 수행하는 검증 시점.
_Avoid_: Task Gate

**Audit Comment**:
Issue·Pull Request에 결정과 실행 결과를 요약하고 근거를 연결하는 범주화된 업무 기록.
_Avoid_: terminal transcript

**Production Deployment**:
사용자가 Run Monitor의 명시적 버튼이나 GHES 화면에서 제품 저장소의 GitHub Actions workflow를 dispatch하여 main의 검증된 변경을 production 환경에 적용하는 운영 행위.
_Avoid_: main 병합, 로컬 실행, 자동 배포

**CD Run**:
제품 저장소에서 사용자가 시작한 Production Deployment의 진행과 결과를 보존하는 GitHub Actions 실행.
_Avoid_: CI Run, Agent 실행

**Trusted Workstation**:
한 Operator가 소유하는 Windows PC와 그 WSL 환경을 하나의 신뢰 경계로 취급하는 실행 환경. 같은 환경 안의 프로세스 사이에 다중 사용자용 보안 격리를 제공하지 않는다.
_Avoid_: 보안 sandbox, 공용 실행 호스트

**Builder Sandbox**:
Builder가 할당된 Worktree와 필요한 내부 서비스에만 접근하도록 실행 범위를 제한하는 격리 환경.
_Avoid_: Worktree, 다중 사용자 계정, Herdr session

**Worktree Isolation**:
Builder마다 별도 Worktree와 branch를 배정해 변경 충돌을 막는 v0.1의 기본 실행 분리 방식. 프로세스의 파일·자격증명 접근을 차단하는 보안 sandbox는 아니다.
_Avoid_: Builder Sandbox, 보안 격리

**Workflow Catalog**:
등록된 제품 저장소에서 Run Monitor가 발견해 상태와 수동 실행 진입점을 보여주는 GitHub Actions workflow 전체 목록.
_Avoid_: production workflow 하나, 외부 pipeline

**Organization Project**:
여러 제품 저장소의 개발 요청, Pull Request, CI와 Production Deployment 결과를 함께 추적하는 조직 공용 GitHub Project.
_Avoid_: devops-control 저장소, Local Orchestrator 상태
