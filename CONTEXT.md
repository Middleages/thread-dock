# ThreadDock

여러 프로젝트의 요청, 결정, 구현, 검증과 문서를 Git/GitHub에 연결하고,
어느 프로젝트로 돌아와도 현재 상태와 다음 행동을 회수하는 개발 운영 문맥이다.
현재 제품 정책은 [Herdr-first 첫 사용 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)를 따른다.
이 용어 모델은 새 MVP의 기준이며 기존 Go 코드의 v1 타입과 일대일 대응하지 않는다.

## 사람과 업무

**Operator**:
여러 프로젝트를 진행하며 요청, 범위와 중요한 결정을 승인하고 GitHub에서 PR을 병합하는 사용자.
_Avoid_: 시스템 관리자, Agent

**Project**:
사람이 하나의 제품·시스템으로 관리하는 단위. 하나 이상의 Repository와 대표 저장소·Wiki를 가진다.
_Avoid_: Repository, GitHub Projects 보드, Run

**Repository**:
정확한 GitHub host와 owner/name, default branch로 식별되는 코드 저장소.
_Avoid_: Project, 로컬 Worktree 경로

**대표 저장소 (Primary Repository)**:
Project 전체 Parent Issue와 Wiki를 두는 기본 저장소.
_Avoid_: main branch, 유일한 코드 저장소

**개발 요청 (Development Request)**:
구현하려는 기능, 수정할 문제 또는 정리할 문서를 아직 계약으로 정제하지 않은 설명.
_Avoid_: Task, Issue, Contract

**작업 인터뷰 (Work Interview)**:
요청의 배경, 목적, 범위, 완료 조건과 중요한 선택을 사용자와 명확하게 만드는 대화.
_Avoid_: 프롬프트 보완, 실행 상태 조회

**업무 (Work Item)**:
한 Project에 속하는 승인 대상 업무. 여러 Repository Run과 PR을 묶고 전체 완료 조건을 가진다.
_Avoid_: Task, Repository Run, PR 하나

**Parent Issue**:
Work Item의 배경, 전체 완료 조건, 최근 handoff와 관련 결정·저장소·PR을 연결하는 대표 업무 기록.
저장소별 PR이 여러 개일 수 있으며 첫 PR 병합으로 자동 종료하지 않는다.
_Avoid_: 실행 Task, 최종 PR 하나와의 일대일 대응

**Child Issue**:
별도 추적 가치가 있는 저장소별 작업을 보존하고 Parent Issue에 연결하는 기록.
_Avoid_: 모든 실행 Task, 체크리스트 항목

**Task**:
하나의 저장소에서 허용 경로·완료 조건·검증 명령을 가진 실행 단위.
의존 관계는 같은 저장소 또는 다른 저장소 Task를 가리킬 수 있다.
_Avoid_: Work Item, 반드시 별도 Issue가 필요한 단위

**Contract v2**:
Work Item의 저장소별 계획, Task DAG, interface 합의, 문서 대상과 논리 프로필을 담은 immutable 실행 계약.
범위 변경은 승인 근거와 함께 새 revision으로 남긴다.
_Avoid_: 대화 transcript, 모델 설정, GitHub Issue 번호의 저장소

## 기록과 문맥 회수

**업무 기록 (Work Record)**:
요청부터 결정·실행·검토·완료까지 GitHub Issue, PR, 설계 문서와 Wiki에 연결해 남긴 기록.
_Avoid_: raw terminal output, local process state

**Decision Record**:
질문, 선택, 이유, 주요 대안과 승인 근거를 남기는 결정 기록.
proposed와 accepted를 구분하고 변경된 결정은 supersedes 관계로 연결한다.
_Avoid_: 모든 대화 요약, 승인되지 않은 제안의 확정

**Handoff**:
업무 목적, 검증된 완료 내용, 현재 단계, 최근 결정, blocker와 nextAction을 근거·시각과 함께 묶은 재개 요약.
_Avoid_: 전체 transcript, 근거 없이 생성된 진행률

**GitHub Projects**:
Issue·PR을 모아 프로젝트별 업무 상태와 우선순위를 표시하는 GitHub 보드.
원본 결정 본문은 Issue·문서에 두며 실행 프로세스 상태를 직접 소유하지 않는다.
_Avoid_: ThreadDock Project, local state database

**Wiki**:
Project 전체 사용법, 구조, 런북과 재사용할 장애 대응 지식의 기본 발행 위치.
코드 저장소와 별도 Git history를 가지며 코드 PR 병합을 Wiki 발행으로 간주하지 않는다.
_Avoid_: Task output, 미병합 구현의 확정된 사용법

**Runbook**:
특정 운영 작업이나 장애 상황의 확인·조치·검증 절차를 재사용할 수 있도록 정리한 Wiki 문서.
_Avoid_: 단일 장애 사건의 전체 경과, 구현 계획

**Audit Comment**:
Decision, Dispatch, Review, Verification, Blocker, Integration 범주의 요약과 근거 링크를 남기는 업무 comment.
_Avoid_: transcript, token·secret, polling마다 생성하는 comment

## 실행과 역할

**Herdr Session**:
재접속해도 유지되는 하나의 대화형 terminal 환경이며 Herdr Workspace·Tab·Pane의 최상위 묶음이다.
_Avoid_: ThreadDock Project, Work Item, Agent 대화 하나

**Herdr Workspace**:
하나의 작업 directory 또는 Git Worktree를 중심으로 Tab과 Pane을 묶는 Herdr의 공간이다.
_Avoid_: ThreadDock Project, Repository, 운영자 전체 작업 공간

**Herdr Tab**:
한 Herdr Workspace 안에서 관련 Pane을 시각적으로 묶는 전환 단위다.
_Avoid_: 브라우저 탭, Work Item, Agent Session

**Herdr Pane**:
shell이나 하나의 Top-level Agent가 점유하는 Herdr의 terminal 자리다.
_Avoid_: Task, Internal Subagent, transcript

**Top-level Agent**:
Operator가 Herdr Pane에서 직접 시작하거나 다시 연결해 대화하는 Codex/OpenCode 세션이다.
자기 작업을 위한 Internal Subagent의 생성·조정·종료를 소유한다.
_Avoid_: Main Agent, Builder 호출, Herdr Pane 자체

**Internal Subagent**:
Top-level Agent가 자신의 native agent 기능으로 제한된 하위 작업에 배정하는 내부 실행자다.
ThreadDock은 개별 생명주기를 소유하거나 모두 표시하지 않고 요약 근거만 연결한다.
_Avoid_: 별도 Herdr Session, ThreadDock Task와의 필수 일대일 대응

**Live Session Observation**:
Herdr에서 읽은 Session·Workspace·Tab·Pane과 Top-level Agent의 시점성 상태다.
GitHub 업무 상태를 대체하지 않으며 읽지 못하면 마지막 관찰 시각과 오류 상태를 유지한다.
_Avoid_: 업무 완료 근거, 영속 실행 원장

**ThreadDock Work Evidence**:
Work Item의 목적·결정·Issue·PR·검증·문서·handoff를 GitHub 링크와 로컬 receipt로 연결한 영속 근거다.
_Avoid_: terminal transcript, Herdr topology, Agent의 진행률 추정

**Work Session Binding**:
Work Item의 Repository와 승인된 canonical Git Worktree를 live Herdr Workspace·Pane에 정확히 연결한 관계다.
label이나 최근 focus로 추정하지 않는다. exact Workspace 0개는 missing, 유일한 Workspace에 Top-level Agent가 없으면 ended, Workspace나 Agent 후보가 둘 이상이면 conflict다.
_Avoid_: Herdr Workspace 이름, 현재 선택된 Pane, fuzzy path match

**Repository Run**:
한 Work Item이 한 Repository에서 진행한 변경·검증·PR 근거의 묶음이다.
_Avoid_: Work Item 전체, 영속 모델 대화

**Agent Invocation**:
선택적인 ThreadDock 실행 adapter가 한 역할 packet을 한 번 실행해 구조화된 Artifact를 반환한 기록이다.
Herdr Session이나 Top-level Agent의 기본 생명주기를 뜻하지 않는다.
_Avoid_: Task, Work Item, 영구 대화 세션

**Execution Profile**:
역할별 실행 정책의 논리 ID. 로컬 설정이 Runtime·nativeProfile을 해석하고 snapshot이 실행 정책을 고정한다.
_Avoid_: 모델 이름, agent identity

**Agent Runtime**:
선택적인 Agent Invocation을 실행하고 구조화 결과를 evidence module에 돌려주는 adapter.
Herdr Session이나 Top-level Agent의 지속성과 native Internal Subagent를 관리하지 않는다.
_Avoid_: Herdr, Orchestrator, 모델

**Go Publisher**:
승인된 계약·대상·검토 결과를 확인하고 GitHub/Wiki 쓰기 및 receipt를 소유하는 Go 구성 요소.
Top-level Agent나 Operator의 명시적 요청에 같은 검증·멱등성 규칙을 적용하는 선택적 evidence module이다.
_Avoid_: 별도 의사결정 Agent, 무제한 GitHub 쓰기 권한
_Avoid_: 상태 JSON 직접 편집자, GitHub 자동 병합 Agent

**Planner**:
요청과 필요한 근거에서 Work Item 계약, Task DAG와 interface 합의 초안을 만드는 역할.
_Avoid_: Orchestrator, Builder

**Scout**:
Planner의 구체적인 질문에 필요한 경로·symbol·의존성·위험을 제한된 CodeMap으로 반환하는 읽기 전용 역할.
_Avoid_: 전체 저장소 덤프, 구현 Agent

**CodeMap**:
Scout의 질문 범위에 해당하는 탐색 Artifact.
_Avoid_: 전체 코드베이스 요약, 승인된 실행 계획

**Builder**:
Top-level Agent가 native subagent로 할당할 수 있는 구현 역할이다. 허용 경로와 검증 근거를 지킨다.
_Avoid_: GitHub writer, default branch merger

**Reviewer**:
Builder 대화 없이 고정된 SHA·diff·조건·검증 근거를 읽고 accept/block을 반환하는 fresh 역할이다.
Top-level Agent의 native subagent 또는 선택적 adapter가 수행할 수 있다.
_Avoid_: Builder 자기평가, 사용자 승인자

**Documenter**:
승인된 결정·Integration Summary·관련 문서를 입력받아 repository docs 또는 Wiki 변경안을 만드는 역할.
_Avoid_: Publisher, 전체 대화 요약기

**Orchestrator / Local Orchestrator**:
기존 Go 코드에서 계약, DAG, Git, 검증과 상태 전이를 관리하는 선택적 evidence module.
Herdr Session·Pane·Top-level Agent·Internal Subagent의 생명주기 소유자가 아니다.
_Avoid_: Herdr, Top-level Agent, Monitor

**Conversation Interface**:
Herdr Pane의 Top-level Agent 대화에서 요청·승인·범위 변경을 처리하는 interface.
_Avoid_: Monitor의 새 채팅 제품

**Orchestrator CLI**:
Top-level Agent, Monitor와 사람이 ThreadDock Work Evidence를 읽거나 명시적으로 발행하는 interface.
Herdr topology 제어는 Herdr CLI와 project skill에 맡긴다.
_Avoid_: Herdr 대체 제어면, 상태 파일 직접 수정, raw terminal protocol

## 검증·병합·완료

**Integration Branch**:
한 Repository Run의 승인된 Task와 repository docs를 모으는 임시 변경선.
_Avoid_: main, Work Item 전체를 대표하는 단일 branch

**Integration Summary**:
문서 작성 전에 확보한 코드 통합 결과·행동 변화·결정·근거 묶음.
_Avoid_: 아직 생성되지 않은 Final Manifest

**Focused Verification**:
변경한 범위의 빠른 구현 검증.
_Avoid_: Full Suite

**Task Gate**:
후보 commit에 대해 Go가 실행한 Task 검증·관련 static check와 변경 범위 확인.
_Avoid_: Agent가 주장한 passed JSON, 최종 전체 업무 리뷰

**Full Suite**:
repository docs까지 반영한 저장소 최종 HEAD의 전체 test·static check 묶음.
_Avoid_: Task 검사만의 합계

**Cross-repository Verification**:
고정된 저장소별 HEAD 묶음이 전체 업무 조건과 interface 합의를 만족하는지 확인하는 검증.
_Avoid_: 개별 저장소 test의 단순 합계

**Merge Gate**:
최신 PR HEAD/base와 필수 CI, Task·전체 리뷰, 전체 조건의 근거를 확인해 mergeReady를 결정하는 규칙.
_Avoid_: 자동 병합 권한, Projects의 Review 상태만으로 내리는 판정

**Protected Change**:
데이터·인증·권한·공개 계약 등에 중요한 영향을 주어 더 명확한 사전 결정과 검토가 필요한 변경.
모든 PR은 변경 종류와 관계없이 사람이 병합한다.
_Avoid_: 일반 변경에 자동 병합을 허용하기 위한 구분

**Final Manifest**:
Work Item의 저장소별 최종 HEAD/base·검증·리뷰·PR·문서 patch와 병합 순서를 고정한 Artifact.
_Avoid_: PR 하나, terminal transcript

**Partial Merge**:
필수 PR 중 일부만 병합된 상태. 앞선 병합을 보존하고 전체 업무를 완료 처리하지 않는다.
_Avoid_: completed, 여러 저장소 원자적 병합

**Finalize**:
실제 PR·병합 관계를 검증하고 필수 Wiki·Issue·Projects 발행까지 수행하는 후속 단계.
Integration HEAD와 실제 merge SHA의 단순 동일 비교를 사용하지 않는다.
_Avoid_: main 병합, 자동 rollback, 실행 근거 삭제

**Publication Pending**:
코드 병합 또는 문서 업무 검토 후 필수 기록·Wiki 발행이 남은 상태.
실행 중의 일시적 동기화 실패는 별도 syncStatus로 표시한다.
_Avoid_: 코드 재실행 필요, completed

## 재개·관찰·신뢰

**Repair Round**:
검사 실패나 리뷰 지적을 고치는 호출. Work Item revision의 Task별 자동 수정 합계는 기본 2회다.
pause/resume과 일시적 runtime 재시도로 한도를 초기화하지 않는다.
_Avoid_: 최초 구현, Recovery Attempt

**Recovery Attempt**:
확실한 일시적 호출 실패 후 종료를 확인하고 같은 논리 호출을 새 requestId로 재시도하는 행위.
기본 자동 재시도는 최대 1회다. 세션 보존을 공통 보장으로 삼지 않는다.
_Avoid_: 코드 결함 수정, 무제한 continuation

**Work Resume**:
Work Evidence, Work Session Binding과 최신 handoff를 확인해 완료 Task를 반복하지 않고 Herdr에서 미완료 작업을 이어가는 행위.
기존 Top-level Agent는 focus/attach하거나 명시적으로 handoff를 전달하고, 없거나 끝났으면 승인된 Worktree에서 새 Top-level Agent를 연다.
_Avoid_: 항상 같은 모델 세션 복원, 처음부터 재실행, blind prompt retry

**Needs Operator**:
반복 실패, 범위 변경, 충돌 또는 불명확한 실행 정체성으로 사람의 조치가 필요한 상태.
현재 변경과 근거를 보존하고 필요한 다음 행동을 함께 보여준다.
_Avoid_: 모든 일시적 실패, 전체 업무 삭제

**ThreadDock Monitor**:
여러 Project의 GitHub 중심 Work Evidence와 Herdr Live Session Observation을 함께 보여주는 Windows 화면.
업무 기록 상태와 로컬 실행 상태를 분리하고 exact Work Session Binding으로 연결한다.
Session·Pane 열기/초점·attach·handoff 전달 같은 승인된 얇은 행동만 Herdr skill에 위임하고 PR 병합은 GitHub에서 수행한다.
_Avoid_: 원본 상태 저장소, 새 채팅 제품, production 배포 콘솔

**Trusted Workstation**:
한 Operator의 PC·WSL을 신뢰 경계로 삼는 환경. 같은 사용자 프로세스 사이의 완전한 보안 격리를 보장하지 않는다.
_Avoid_: 다중 사용자 sandbox

**Worktree Isolation**:
Task마다 Worktree를 배정해 파일 변경을 분리하는 방식.
_Avoid_: credential 접근 차단, 프로세스 보안 격리

**Runtime Permission Policy**:
runtime에서 read-only/workspace-write intent와 도구 접근을 적용하는 정책.
적용 불가능하면 실행을 거부한다. prompt의 금지 문구만으로 보장을 주장하지 않는다.
_Avoid_: Worktree 경로 지정만으로 얻는 sandbox

## 기존 코드와 후속 범위의 용어

기존 v1 코드의 Run, Execution Session Retirement, Blocked Run과 PhaseMerging은
이전 구현의 식별자다. 새 업무 정책의 원본으로 해석하지 않는다.
기존 자료는 Git 이력과 legacy 운영 문서를 통해 참고한다.

CI Run은 PR의 검사 실행이다. Production Deployment, CD Run과 Workflow Catalog는
후속 범위이며 이번 Monitor의 인수 조건에 포함하지 않는다.
