# Project Workflow MVP Design

## 상태와 기준선

2026-09-07 대화에서 사용자가 승인한 프로젝트 중심 방향을 정리한 설계다.
제품 방향은 승인되었으며, 아래의 기본값과 명령 계약은 그 방향을 구체화한
설계 선택이다. 이 문서가 새 MVP의 기준이고 기능 구현 완료를 뜻하지 않는다.

- 기준 main: ce42f403b0a758acf3e647394b1c04a5f2c447c7.
- 최초 runtime-adapters 설계: f669ec6008ee3ccc14d688aad83700a1da84fceb.
- 작업 브랜치: agent/runtime-adapters-mvp.
- [이전 Runtime Adapters 설계](https://github.com/Middleages/thread-dock/blob/f669ec6008ee3ccc14d688aad83700a1da84fceb/docs/superpowers/specs/2026-09-07-runtime-adapters-mvp-design.md)를 대체한다. 충돌하는 옛 설계·계획 파일은 현재 tree에서 삭제하고 Git 이력으로 남긴다.
- 정책 전환 이유는 [ADR 0006](../../adr/0006-project-workflow-mvp.md)에 남긴다.
- [CONTEXT.md](../../../CONTEXT.md)는 용어, [PRODUCT.md](../../../PRODUCT.md)는 제품 범위의 기준이다.
- 기존 파일럿 Run을 새 MVP에서 재개할 필요는 없다. 기존 코드, 브랜치와 외부 근거는 보존한다.

## 1. 해결할 문제

Operator는 여러 프로젝트를 동시에 진행한다. 프로젝트마다 저장소 하나만 있을 수도,
frontend와 backend처럼 여러 저장소로 나뉠 수도 있다. 한 프로젝트의 Agent가
구현하는 동안 다른 프로젝트를 진행하고 돌아오면 목적, 최근 결정과 남은 일을
다시 떠올리는 데 시간이 든다.

ThreadDock은 Git과 GitHub를 활용해 요청, 의사결정, 구현, 검증과 문서화를 연결한다.
프로젝트를 다시 열거나 새 대화에서 이어갈 때 다음 질문에 근거를 붙여 답해야 한다.

1. 무엇을 왜 시작했는가?
2. 어떤 방법을 선택했고 이유는 무엇인가?
3. 어디까지 검증된 상태로 끝났는가?
4. 무엇이 막혀 있고 내가 지금 해야 할 일은 무엇인가?
5. 사용법과 운영 지식은 어디에 남았는가?

성공 기준은 실제 업무를 끝까지 수행하고 문맥 회수 부담을 줄이는 것이다.
장애 주입 파일럿의 모든 실패 경로를 인증하는 것을 제품 완료 조건으로 삼지 않는다.

## 2. MVP 범위

포함:

- 한 Operator, Windows 11 + WSL, 여러 프로젝트 등록과 프로젝트 간 동시 진행.
- 프로젝트마다 하나 이상의 저장소, 대표 저장소와 대표 Wiki 지정.
- 요청 인터뷰, 승인된 업무 계약, 결정 기록과 중간 handoff.
- 저장소별 Worktree, Task DAG, 검증, 독립 리뷰와 제한된 자동 수정.
- Codex/OpenCode 역할 프로필과 실행 상태에 근거한 작업 재개.
- GitHub Issues, 공용 Projects 보드와 저장소별 PR 연결.
- 프로젝트 목록과 상세 화면으로 구성된 ThreadDock Monitor.
- GitHub Wiki에 사용법, 구조, 런북과 장애 대응 지식 작성·갱신.
- 사람의 PR 병합 확인과 여러 저장소 업무의 전체 완료 판정.

제외:

- 기존 v1 Run 실행 호환성, 자동 migration과 이전 세션 재현.
- 파일럿 전용 controller, 대규모 fault matrix, tmpfs 인증과 필수 live-fault gate.
- 자동 main 병합, 여러 저장소의 원자적 병합·자동 rollback.
- production 배포 버튼, 전체 workflow catalog, production 관측과 자동 업데이트.
- 팀 다중 사용자 실행 서버, 분산 scheduler와 모니터 안의 새 채팅 제품.
- 전용 Recovery/Test/Security Agent와 모든 대화·terminal transcript의 영구 저장.

기존 Git 검사·상태 저장·프로필 처리에서 필요한 코드는 재사용할 수 있다.
파일럿 재현을 위해 기존 상태 기계 전체를 유지하지 않는다.

## 3. 업무 모델과 식별자

| 단위 | 책임과 관계 |
|---|---|
| Project | 사람이 하나로 인식하는 제품·시스템. repositoryRefs 1개 이상을 가진다. |
| Repository | 정확한 GitHub host, owner/name, default branch와 로컬 경로로 식별한다. |
| Work Item | 요청의 목적·범위·완료 조건을 가진 업무. Project에 속하고 Parent Issue 하나를 가진다. |
| Repository Run | 한 Work Item이 한 저장소에 만드는 변경 실행. 해당 저장소의 Task와 Integration Branch를 소유한다. |
| Task | 정확히 한 저장소의 허용 경로에서 수행하는 실행 단위. 다른 저장소 Task에 의존할 수 있다. |
| Agent Invocation | 하나의 역할 packet을 실행하는 한 번의 호출. 업무 식별자를 대신하지 않는다. |

Work Item 하나는 변경이 필요한 저장소별 Repository Run과 PR을 가진다.
정상 진행에서는 저장소별 PR 하나를 사용한다. 재계획으로 새 PR이 필요하면 이전 PR
정체성과 대체 관계를 남긴다. 문서만 다루는 업무는 코드 PR 없이 끝날 수 있다.

대표 저장소는 Parent Issue와 프로젝트 전체 Wiki의 기본 위치다.
Child Issue는 독립 추적 가치가 있을 때 실제 변경 저장소에 만든다.
Task 하나마다 Issue를 생성하지 않는다. 저장소별 PR은 항상 Parent Issue로 연결된다.

프로젝트 설정은 version 관리되는 non-secret 등록 문서와 로컬 설정을 분리한다.
대표 저장소에는 repoKey·host/repository·Wiki·Projects 매핑을 기록하고 로컬 경로·credential은
Workstation에만 둔다. GitHub 기록으로 업무 문맥은 회수할 수 있지만 로컬 미커밋 파일까지
다른 PC에서 복구한다고 보장하지 않는다.

GitHub 식별자는 host + repository + issue/PR number로 저장한다.
Projects 항목은 Project node ID + item ID로 저장한다. 번호만으로 대상을 추정하지 않는다.
Project 등록 정보가 바뀌어도 승인된 Work Item의 저장소 대상을 자동으로 바꾸지 않는다.

## 4. 기록의 역할과 갱신 책임

| 기록 | 원본 내용 | 쓰는 주체와 시점 |
|---|---|---|
| Parent Issue | 시작 배경, 목적, 범위, 완료 조건, 관련 저장소와 결정 링크 | 승인된 업무 등록, 범위 변경, 주요 단계 전환 |
| Child Issue / PR | 저장소별 구현 범위, 진행, 검증·리뷰와 병합 결과 | 작업 배정, 검증·리뷰 완료, PR 준비·병합 |
| Decision Record | 질문, 선택, 이유, 중요한 대안, 승인 근거와 대체 관계 | 사용자가 방향을 결정하거나 변경할 때 |
| Repository docs | 코드와 함께 검토해야 하는 상세 설계·개발 설명 | 해당 저장소의 PR에 포함 |
| Wiki | 현재 사용법·구조·운영 절차·재사용할 장애 대응 | 관련 변경 반영 뒤 또는 독립 문서 업무 완료 시 |
| Local state | 호출·Task·Run 상태, Git 근거, 동기화 receipt와 handoff | Orchestrator의 상태 전환 시 |
| GitHub Projects | 업무 상태·우선순위·프로젝트 분류와 관련 Issue/PR | 원본 업무 상태를 발행·동기화할 때 |

Main Agent가 기록 내용과 대화를 관리하고, Go 명령이 구조·대상·진행 조건을 검증한다.
GitHub 쓰기는 승인된 업무 범위에서 Main Agent가 호출하는 publication 명령으로만
진행한다. 이 명령이 gh를 사용하며 receipt를 Go 상태 저장소에 돌려준다.
Builder, Reviewer, Scout, Documenter와 Monitor는 GitHub를 직접 변경하지 않는다.

공용 Projects 보드 하나에 프로젝트별 보기와 업무 상태를 둔다. 기본 필드는
Project, Status, Priority다. Project 필드는 ThreadDock Project와 명시적으로 매핑한다.
긴 결정 본문은 Issue 또는 설계 문서에 저장하고 Projects는 해당 항목으로 연결한다.
대표 Parent 항목을 전체 업무 상태의 기준으로 삼고 Child 항목은 개별 진행을 표시한다.

GitHub.com/GHES의 host와 Projects/Wiki 지원 및 접근 권한을 등록 때 실제로 확인한다.
공용 보드의 node ID, 필드 ID, option ID를 저장한다. 호환되지 않는 endpoint를
조용히 무시하거나 임의의 보드를 새로 만들지 않고 설정 문제를 보여준다.
여러 host의 업무는 Monitor에서 모으되 공용 보드 매핑은 host마다 별도로 둔다.

### 결정 기록

최소 필드는 decisionId, workId, question, choice, rationale, alternatives,
status, approvalRef, createdAt, supersedes다. status는 proposed / accepted / superseded다.
승인되지 않은 제안에 accepted를 붙이지 않는다. 작은 구현 선택은 승인된 계약의
범위 안에서 구현 근거로 기록하며 매번 사람에게 묻지 않는다.
범위·공개 API·데이터 처리·업무 완료 조건이 바뀌면 새 결정과 계약 revision을 승인받는다.

중요 결정은 대표 저장소의 docs/decisions에 문서로 남기고 Parent Issue에 링크한다.
Issue에는 승인 시점의 결정 요약을 즉시 남기므로 문서 PR 병합을 기다려 이력이
비는 일이 없어야 한다. 기존 결정을 삭제·덮어쓰기보다 새 결정이 대체한 관계를 남긴다.

### 중간 handoff

Task 완료, 차단, 사용자 결정, pause, PR 준비와 문서 발행 때 갱신한다.
workId, 목적, 검증된 완료 내용, 현재 단계, 최근 승인 결정, blocker,
nextAction, evidenceRefs, stateRevision, observedAt, publishedAt을 가진다.
실행 상태는 Go 근거에서 산출하고 설명 문구는 Main Agent가 작성할 수 있다.
요약을 생성하지 못해도 구조화된 현재 상태와 다음 행동은 보여준다.
GitHub에는 milestone 요약을 남기며 polling마다 comment를 만들지 않는다.

## 5. Monitor와 프로젝트 간 진행

기존 Windows Wails + React/TypeScript, WSL Go CLI 방향을 유지한다.
새 UI framework나 HTTP 서버를 도입하는 일은 MVP 요구가 아니다.

### 프로젝트 목록

각 Project의 진행 중 업무 제목, 마지막 검증 완료 내용, 현재 단계, 다음 행동,
최근 활동 시각과 마지막 동기화 시각을 보여준다. 사용자 결정 대기와 실패를 위에
배치하고 최근 변경순 보기를 제공한다. 수치 근거 없는 진행률은 표시하지 않는다.
예전 snapshot을 현재 실행 중인 사실로 표시하지 않도록 stale/offline을 구분한다.

### 프로젝트 상세

- 업무 배경, 범위와 완료 조건.
- 저장소별 Task, PR, 검증·리뷰·병합 상태.
- 결정 timeline, 최근 handoff와 필요한 답변.
- 관련 Issues, Projects, 설계 문서와 Wiki 링크.
- pause, resume, retry와 Main Agent에 전달할 구조화된 handoff 열기/복사.
- GitHub PR 열기. 병합은 GitHub에서 사람이 한다.

승인 인터뷰와 범위 변경 대화는 기존 Main Agent 대화창에서 진행한다.
Monitor는 상태 파일 직접 편집이나 별도의 승인 경로를 만들지 않는다.
프로세스 ID, provider session, request ID와 raw transcript는 기본 화면에 노출하지 않는다.

### 동시 실행과 백그라운드

프로젝트 사이의 동시 진행은 MVP 필수다. 기본 전역 Agent 호출 한도는 2이며 설정할 수 있다.
같은 프로젝트의 변경 업무는 MVP에서 하나씩 실행한다. 다른 업무는 대기열에 남는다.
같은 물리 저장소를 여러 Project에 등록해도 canonical Git common-dir 및 원격 정체성으로
충돌을 검사하여 서로 다른 업무의 변경 실행을 동시에 열지 않는다.

한 Work Item은 백그라운드 coordinator 프로세스를 가진다. UI와 대화창을 닫아도
진행은 유지된다. 짧은 CLI status 호출이 scheduler를 대신하거나 실행을 시작하지 않는다.
전역 슬롯은 로컬 lease로 관리하고, 살아 있는 실행의 슬롯을 polling이 임의 회수하지 않는다.
각 Work Item은 상태 전환을 단일 writer로 직렬화하며 snapshot은 원자적으로 교체한다.
긴 Agent 호출 동안 status 읽기나 다른 프로젝트 진행을 막는 전역 lock을 잡지 않는다.

Monitor는 3~5초마다 aggregate status 명령 한 번을 호출한다.
GitHub 동기화는 별도 refresh 작업으로 캐시하고 기본 60초 간격 또는 명시적 refresh에
수행한다. 화면 polling마다 GitHub API를 호출하지 않는다.
PC 재시작 뒤에는 reconcile 후 resume으로 이어가며 자동 재실행으로 추정하지 않는다.

## 6. 승인된 Contract v2

Contract v2는 Work Item 전체의 논리적 실행 계약이다.
이전 runtime-adapters 문서의 단일 저장소 v2 초안은 구현 호환 대상이 아니다.

| 필드 | 규칙 |
|---|---|
| version, workId, projectId, revision | version=2, 안정 ID, 양의 revision |
| request, acceptanceCriteria | 시작 배경·목적·범위·전체 완료 조건 |
| repositoryPlans | repoKey, baseSha, targetBranch, 저장소별 검증과 PR 순서 |
| tasks | taskId, repoKey, allowedPaths, dependsOn, 완료 조건과 검증 명령 |
| interfaceAgreements | 저장소 간 API·자료 형식·호환성 합의와 결정 참조 |
| crossRepoVerification | 관련 repoKey 목록, 작업 디렉터리와 정확한 검증 명령 |
| issueDrafts | 대표·Child Issue의 논리 key와 목표 repoKey |
| documentation | repository/wiki target, 허용 페이지·경로, 갱신 이유와 required/none 사유 |
| executionProfiles | builder, reviewer, documenter의 논리 ID |
| decisionRefs | 승인된 결정과 승인 근거 |

Contract에는 모델·provider 설정, 로컬 경로, 토큰과 생성된 Issue 번호를 넣지 않는다.
repoKey는 등록 정보로 해석한 정확한 host/repository/branch와 함께 승인 미리보기에 표시하고
Run snapshot에 고정한다. 생성된 GitHub 번호와 로컬 경로는 Run state에 별도로 연결한다.
명령은 argv와 cwdRepoKey로 정의하며 shell이 필요하면 명시적으로 shell script를 선언한다.
승인되지 않은 Agent 제안 명령을 Orchestrator가 검증 명령으로 승격하지 않는다.

전체 DAG에 대해 누락 참조·순환·허용 경로 충돌과 merge 순서 순환을 검사한다.
필요한 저장소가 등록되지 않았거나 base가 존재하지 않으면 실행 전에 거부한다.
계약 변경은 새 immutable revision을 만들고 영향을 받는 검증·리뷰를 무효화한다.

## 7. 실행과 통합

1. Main Agent가 요청을 인터뷰하고 관련 결정·Wiki·코드 근거를 읽는다.
2. Planner가 필요한 경우 제한된 Scout 탐색을 사용해 계약·Issue 초안을 만든다.
3. 사용자 승인 후 Go가 계약을 저장하고 GitHub 업무를 등록한다.
4. Orchestrator가 실행 가능한 Task의 기반 commit과 Worktree를 준비한다.
5. Builder가 허용 범위를 구현하고 구조화된 결과를 반환한다.
6. Orchestrator가 변경 범위를 확인하고 후보 commit을 만든 뒤 선언된 Task 검증을 실행한다.
7. fresh Reviewer가 해당 후보 SHA·diff·Task 조건·검증 결과를 읽고 accept/block을 반환한다.
8. 허용된 수정 한도 안에서 재구현·재검증·새 리뷰를 수행한다.
9. 승인된 후보 commit을 저장소별 Integration Branch에 통합한다.
10. Integration Summary로 Documenter가 repository docs와 Wiki 변경안을 준비한다.
11. repository docs를 통합한 최종 HEAD에서 Full Suite를 실행한다.
12. 고정된 저장소별 HEAD 묶음으로 cross-repository 검증을 수행한다.
13. fresh 최종 Reviewer가 전체 업무 조건, 각 저장소 diff, 결정과 Wiki 변경안을 검토한다.
14. 검증된 HEAD 묶음·문서 변경안을 Final Manifest에 고정하고 PR을 발행한다.
15. 사용자가 표시된 순서대로 각 저장소 PR을 GitHub에서 병합한다.
16. Finalize가 병합 관계와 문서 발행을 확인한 뒤 전체 업무를 완료한다.

Git branch와 commit은 Go가 소유한다. Builder는 파일 변경과 결과 설명을 반환한다.
이 선택은 Codex sandbox에 Git 공용 metadata 쓰기 권한을 주는 요구를 줄이고,
Agent가 보고한 commit을 그대로 신뢰하지 않게 한다.
Orchestrator는 tracked/untracked 변경, rename과 symlink를 포함한 범위를 확인하고
허용된 파일만 stage한다. Agent 종료가 확인되기 전에는 commit·검증·통합하지 않는다.
검증 전후 HEAD와 working tree를 확인해 검사 중 변경된 결과를 통과로 인정하지 않는다.

의존 Task는 선행 Task의 승인된 commit이 통합된 baseline에서 시작한다.
다른 저장소 의존성은 승인된 interface와 commit reference로 전달하고 한 Task가
다른 저장소를 직접 수정하지 않는다. 영향을 주는 선행 Task가 수정되면 의존 Task와
cross-repo 검증을 stale 처리한다. 이미 병합한 결과에 영향이 있으면 사람에게 알리고
후속 업무로 처리하며 기존 PR history를 재작성하지 않는다.

단일 저장소 업무의 crossRepoVerification은 비어 있을 수 있다.
여러 저장소 업무는 합의된 연결 검증이 필요하며 미리 선언된 수동 확인도 허용한다.
수동 확인 항목은 exact HEAD 묶음에 대한 사용자 근거가 생기기 전까지 pending이다.
테스트 코드가 없다는 이유로 전체 수용 조건이 자동 충족되지는 않는다.

## 8. 역할과 런타임

- Main Agent: 인터뷰·승인 근거·업무 기록·handoff·publication 호출.
- Planner: 계약과 Task DAG, interface 합의 초안.
- Scout: 질문에 필요한 경로·symbol·위험만 읽어 CodeMap 반환.
- Builder: 할당된 Worktree 파일 구현.
- Reviewer: Task 및 최종 업무를 fresh context에서 독립 검토.
- Documenter: 관련 코드 근거와 승인된 결정에 맞는 문서 변경안 작성.
- Go Orchestrator: 식별자, 상태, Git, 검증 실행, 한도와 다음 행동 결정.

AgentRuntime의 역할은 Invoke(context, Invocation) → Artifact다.
Invocation은 requestId, role, 고정 프로필, canonical Worktree, 제한된 packet,
output schema와 read-only/workspace-write intent를 가진다.
Artifact envelope는 requestId, role, status, bounded typed result를 가진다.
BuildResult의 자체 검증 주장은 참고 정보이며 authoritative checks는 Go가 실행한다.
ReviewResult는 accept/block, blockingFindings와 검토한 SHA 묶음을 가진다.
DocumentationResult는 target, base identity, patch identity, changed pages를 가진다.
결과의 크기·스키마·requestId를 확인하고 누락·trailing JSON·이전 호출 결과를 거부한다.

실행 프로필은 논리 ID를 codex/opencode + nativeProfile로 해석한다.
Planner/Scout 프로필은 계약 전 로컬 설정이고 Builder/Reviewer/Documenter ID는 계약에 둔다.
read-only Reviewer 예시는 별도 reviewer 프로필을 사용하며 OpenCode build로 매핑하지 않는다.
runtime, nativeProfile, 실행 binary 버전, 비밀 없는 유효 설정의 fingerprint를 snapshot에
기록한다. 로컬 설정 변경으로 기존 Run을 몰래 reroute하지 않는다. fingerprint 변경 시
영향과 새 설정을 제시하고 명시적 재설정을 받은 뒤 새 호출을 시작한다.
이 고정은 실행 정책 추적을 위한 것이며 모델의 응답 재현성을 보장하지 않는다.

Codex는 ephemeral 호출과 새 문맥으로 이어간다. stdin packet, 결과 schema와 bounded
결과 파일을 사용하고 설치 버전의 CLI 기능을 preflight한다.
OpenCode/Herdr는 이미 만든 Worktree를 열고 adapter 내부에서 session lifecycle을 관리한다.
정확한 살아 있는 호출 식별이 가능할 때만 기존 호출을 관찰·회수하며, 재시도와 리뷰는
이전 결과를 현재 결과로 혼동하지 않도록 새 requestId를 사용한다.
Orchestrator의 업무 모델에 pane/workspace/session ID를 노출하지 않는다.

### 신뢰와 권한 범위

한 사용자의 Trusted Workstation을 가정한다. Worktree는 변경 분리이고 계정 간 보안
sandbox가 아니다. runtime 권한 설정으로 read-only/write intent를 적용하고 적용할 수
없으면 그 프로필을 실행하지 않는다. prompt의 금지 문구만으로 read-only를 주장하지 않는다.

publication 자격증명은 runtime 및 검증 명령에 명시적으로 주입하지 않는다.
GH_TOKEN, GITHUB_TOKEN, GH_ENTERPRISE_TOKEN, GITHUB_ENTERPRISE_TOKEN,
THREADDOCK_GH_TOKEN 등 GitHub 관련 환경 전달과 runtime MCP 쓰기 도구를 제한한다.
사용자 홈의 credential helper나 SSH agent까지 OS 수준에서 완전 격리한다고 주장하지 않는다.
고강도 자격증명 격리는 별도 실행 계정·container가 필요한 후속 범위다.
계약·packet·Artifact·GitHub 요약에 credential과 전체 환경·transcript를 넣지 않는다.
Codex의 search 비활성화와 승인 정책은 실제 설정으로 적용하며 strict-config를 설정
격리 기능으로 해석하지 않는다. 결과 파일은 owner-only이고 adapter 소유 임시 파일만 정리한다.

## 9. 재개·재시도·자동 수정

완료된 작업과 근거를 보존하는 것이 재개 보장이다. 같은 모델 대화를 복원하는 것은
공통 계약이 아니다. 새 호출은 원래 Task, 현재 Git diff/commit, 최근 검사·리뷰,
완료/미완료 항목과 남은 한도를 받는다.

| 상황 | 기본 동작 |
|---|---|
| 확실한 일시적 호출 실패 | 이전 호출 종료 확인 후 같은 논리 호출에 자동 재시도 최대 1회 |
| malformed/stale Artifact | 자동 성공 처리 없이 needs_operator. 원인과 보존된 변경 표시 |
| Task 검사 실패 또는 Reviewer block | 소유 Task를 특정할 수 있으면 수정 호출, 누적 최대 2회 |
| 최종 검사·리뷰 실패 | 소유 Task가 명확하면 해당 Task의 같은 수정 budget 사용; 재통합·영향 검사·최종 리뷰 |
| 충돌, 허용 경로 위반, 계약 변경 | needs_operator; 임의 경로 확장·충돌 해결·scope 변경 없음 |
| 일시 정지 | 진행 중 호출 취소·종료 확인 후 paused; 파일과 완료 기록 보존 |
| 프로세스/PC 재시작 | Git·receipt·호출 상태 reconcile 후 사용자가 resume |
| Wiki/Projects 발행 실패 | publication_pending; 코드 Task를 재실행하지 않고 미완료 발행만 재시도 |

Repair budget은 Work Item revision 안의 Task별 누적 2회이며 테스트와 리뷰가 공유한다.
pause/resume이나 runtime 실패 재시도로 초기화하지 않는다. 사용자 retry는 실패 원인과
남은 한도를 보여주며, 소진된 한도 증가는 새 사용자 승인 기록이 있어야 한다.
문서 수정도 target별 최대 2회로 제한하고 코드 Task budget과 구분한다.

취소와 timeout은 adapter가 자신이 시작한 실행의 종료까지 확인해야 완료된다.
프로세스 정체성이 불명확하면 needs_operator로 남겨 같은 Worktree에서 중복 실행하지 않는다.
정상 결과와 깨끗한 Git 상태가 이미 있으면 reconcile은 이를 채택하고 완료 Task를 반복하지 않는다.
그 외의 미완료 호출은 새 requestId로 재개하며 이전 결과를 재사용해 성공 처리하지 않는다.

## 10. 상태와 명령 계약

| 상태 | 의미 |
|---|---|
| draft / awaiting_approval | 요청 구체화 또는 계약 승인 대기 |
| queued / running | 실행 슬롯 대기 또는 Task 진행 |
| paused / needs_operator | 사용자 중단 또는 사용자 판단·환경 조치 필요 |
| ready_for_pr / review | 로컬 최종 검증 완료 / PR 발행 후 검토·CI 대기 |
| partially_merged | 필수 PR 중 일부만 병합 |
| publication_pending | 병합 후 또는 문서 업무의 필수 기록·문서 발행 미완료 |
| completed | 모든 필수 결과·병합·문서·업무 기록 반영 완료 |

실제 state와 GitHub 동기화 상태는 별도 필드다.
실행 중 Issue 요약 발행이 실패해도 실제 실행을 publication_pending으로 바꾸지 않고
syncStatus=pending과 마지막 성공 시각을 보여준다.
ready_for_pr는 CI 완료 전일 수 있다. mergeReady는 최신 HEAD/base, 전체 리뷰와 필수 CI
상태를 재확인한 별도 판정이다. Github Projects의 Review만으로 병합 준비를 판단하지 않는다.

아래는 구현할 CLI 계약이며 현재 binary에 존재한다고 가정하지 않는다.
모든 쓰기 명령은 expectedRevision과 idempotency requestId를 받고 Go가 상태를 바꾼다.
새 항목 생성의 expectedRevision은 0이다. pause/resume은 멱등적 상태 전이이며 stale revision은
현재 상태와 함께 거부한다.

| 명령군 | 입력·출력 책임 |
|---|---|
| project register / list | repo·host·대표 저장소·Wiki·보드 매핑 검증, projectId 반환 |
| project status --all --json | 모든 프로젝트의 aggregate snapshot과 freshness, nextAction |
| work plan / approve | 계약 미리보기·hash·approvalRef와 immutable revision 저장 |
| work publish-issues | 승인된 Issue 초안을 gh로 생성/조회하고 정확한 번호·marker receipt 저장 |
| work run / status / pause / resume / retry | 백그라운드 실행, 상태 조회, 한도와 종료 확인을 포함한 제어 |
| work decision / handoff | 제안·승인·대체 결정과 근거 있는 중간 요약 저장·발행 |
| work publish-prs / refresh | 검증된 Manifest의 branch push·PR 생성, 최신 원격 상태 조회 |
| work finalize | 실제 병합 검증, Wiki 발행과 Issue/Projects 완료 처리 |

GitHub 결과를 Main Agent가 임의로 snapshot JSON에 써넣지 않는다.
응답은 schemaVersion, IDs, revision, state, syncStatus, nextAction과 evidenceRefs를 가진다.
pending 외부 동작은 실행 전에 기록한다. interrupted write는 정확한 marker/remote identity를
조회한 뒤 채택 또는 재시도한다. 같은 requestId에 다른 payload는 거부한다.

## 11. PR·병합·Finalize

첫 MVP는 사람이 GitHub의 Create a merge commit으로 병합하는 흐름을 지원한다.
지원하지 않는 squash/rebase가 관찰되면 코드를 되돌리지 않고 needs_operator로 표시한다.
저장소 설정을 ThreadDock이 임의로 변경하지 않는다.

Final Manifest는 Work Item 단위이며 repositoryResults 배열을 가진다.
각 결과는 host/repo, Task commit, Integration HEAD, 검증한 base SHA, tree,
검증·리뷰 identity, PR identity, 병합 순서와 문서 target을 담는다.
모든 최종 검사는 repository docs까지 포함한 HEAD에 연결된다.
Wiki 변경은 baseWikiSha와 patch/tree identity를 별도로 연결한다.

Finalize는 예상 Integration HEAD와 merge_commit_sha를 동일 비교하지 않는다.
정확한 저장소·PR·대상 branch, merged 상태, 승인된 PR HEAD, 실제 merge commit의
부모 관계(검증한 base와 승인된 HEAD), merge tree가 검증 결과 tree와 같은지를 확인한다.
검증한 base는 Integration HEAD의 ancestor여야 한다. merge commit의 첫째 부모가
검증한 base이고 둘째 부모가 승인된 PR HEAD인지 확인한다.
원격 기본 branch가 앞서 갔으면 새 base를 통합하고 영향을 받는 검사·리뷰를 다시 수행한다.
이미 병합된 PR의 관찰 시점 기본 branch가 더 앞서 있는 것은 허용하며, 실제 merge commit이
그 branch 이력에 존재하는지를 확인한다. 근거 없는 ready 상태는 유지하지 않는다.

PR HEAD/base/필수 검사나 승인된 계약 revision이 바뀌면 관련 readiness를 무효화한다.
최종 검증 tuple에는 repository별 HEAD/base와 Wiki patch를 포함한다.
사용자가 readiness 밖에서 수동 병합한 경우 실제 상태를 보존하고 불일치를 알린다.
자동 merge나 main rewrite로 수정하지 않는다.

MVP에서는 연결 저장소의 PR들을 모두 준비한 뒤 순서대로 사람에게 병합을 요청한다.
한 PR만 병합된 업무는 partially_merged다. 다음 PR에 문제가 생기면 앞서 병합한 결과를
그대로 표시하고 후속 조치를 요청한다. 원자적 병합·자동 rollback은 제공하지 않는다.
Parent Issue와 전체 Project 항목은 필수 PR·전체 조건·필수 문서 발행까지 끝나야 Done이다.
Child Issue는 그 범위의 완료 조건을 만족하면 먼저 닫을 수 있다.
Parent에 자동 종료 키워드를 넣지 않고 연결 링크를 사용해 첫 PR 병합으로 닫히지 않게 한다.

## 12. Wiki와 문서 업무

대표 Wiki는 한 프로젝트 전체의 사용법, 시스템 구조, 운영 절차와 런북의 기본 장소다.
frontend/backend 등 저장소별 페이지는 이 안에서 구분하고 관련 코드·설계로 링크한다.
같은 본문을 repository docs와 Wiki 양쪽에서 독립 편집하는 구조를 만들지 않는다.
코드에 결합된 상세 설계는 repository docs, 운영 독자용 현재 설명은 Wiki가 원본이다.

코드 업무에서는 관련 Wiki 변경안을 미리 만들고 최종 Reviewer가 함께 검토한다.
변경안이 참조하는 필수 PR이 모두 병합된 뒤 발행한다. 기본은 전체 업무 PR의 병합 이후다.
Wiki는 별도 Git 저장소이므로 clone/commit/push하는 publication 경로를 두며 코드 PR과
동일한 병합이 일어난 것으로 간주하지 않는다.
최초 Wiki가 없으면 Main Agent가 초기화 필요 상태를 알리고 재개 가능하게 유지한다.

검토 이후 원격 Wiki가 바뀌면 force push나 자동 덮어쓰기하지 않는다.
최신 Wiki 기준으로 재작성·문서 검증·독립 리뷰를 한 뒤 새 patch identity를 발행한다.
발행 receipt에는 Wiki commit과 근거 PR/decision identity를 남긴다.

코드 변경이 없는 런북·사용법 정리도 Work Item으로 다룬다.
이 경우 코드 PR 요건은 없고 승인된 문서 범위 → Documenter → 독립 리뷰 →
Wiki 발행 → 업무 완료 흐름을 사용한다.
장애 사건의 시점·영향·원인·해결 근거는 Issue에, 재사용할 대응 절차는 Wiki에 남긴다.

## 13. 재사용과 구현 순서

| 순서 | 실제 사용할 수 있는 결과 |
|---|---|
| 1. 프로젝트·업무·기록 | registry, v2 상태·계약, 대표 Issue·Projects 매핑, 결정·handoff, CLI aggregate status |
| 2. Monitor 기본 화면 | 프로젝트 목록·상세·다음 행동·링크·freshness. 아직 없는 실행 기능은 명시 |
| 3. 저장소 실행 | Go Git/검증, Codex·OpenCode adapter, fresh 리뷰·제한 수정·pause/resume |
| 4. 다중 저장소 업무 | 의존 Task·interface 합의, cross-repo 검증, 저장소별 PR과 부분 병합 |
| 5. 문서와 전체 완료 | repository docs 최종 gate, Wiki 검토·발행·재시도, 전체 Finalize |

이는 한 MVP 안의 개발 순서다. Monitor와 Projects/Wiki는 후속 버전으로 미루지 않는다.
새 v2 state directory를 사용하고 v1 자료는 수정 없이 보존한다.
v1을 입력하면 unsupported_legacy 설명과 기존 기록 위치를 반환하고 실행하지 않는다.
기존 CLI를 사용해야 하는 역사 자료는 고정된 옛 버전에서만 참고한다.

internal/contract, state, pathscope, worktree, integration과 Herdr의 좁은 기능은 검토 후
재사용한다. 기존 parallel.go의 GitHub 병합·recovery 정책을 새 흐름에 그대로 연결하지 않는다.
제품의 새 도메인·CLI 계약을 먼저 정하고 파일럿 테스트 유지 때문에 설계를 역으로 맞추지 않는다.

## 14. 검증과 인수 조건

필수 검증은 실제 사용자 흐름과 명확한 실패 위험에 집중한다.

- Contract/registry 식별자, DAG, revision, 상태 전이·budget·프로필 고정 단위 검사.
- fake runtime으로 requestId, schema, read-only 설정, 취소·timeout과 중복 호출 방지.
- 임시 로컬 Git 저장소 두 개에서 Task → 수정 → 통합 → 최종 HEAD 검증.
- fake GitHub로 중복 발행 방지, HEAD/base 변경, 부분 병합과 publication 재시도.
- UI에서 프로젝트 전환, stale 표시, 사용자 대기·resume과 근거 링크 확인.
- 코드 변경 시 focused 검사 후 최종 make check 한 번. 실패하면 재현 가능한 원인을
  확인하고 해결하며, 결과를 통과로 바꾸기 위해 suite를 제외하지 않는다.
- 실제 외부 쓰기는 승인된 작은 실사용 업무에서 수행하고 결과를 기록한다.
  별도 live-fault 인증 프로젝트를 구축하지 않는다.

첫 사용 완료 조건:

1. 서로 다른 프로젝트 두 개의 업무를 진행하고 모니터에서 각각 현재 상태·다음 행동을 본다.
2. 단일 저장소 업무와 frontend/backend 두 저장소 업무를 모두 표현한다.
3. 새 대화가 Issue·결정·handoff를 읽고 목적, 최근 결정과 다음 행동을 근거와 함께 설명한다.
4. 작업 중단 뒤 완료 Task를 반복하지 않고 이어가며, 리뷰 지적을 수정·재검토한다.
5. 문서까지 반영된 최종 HEAD와 전체 업무 조건을 검증하고 저장소별 PR을 준비한다.
6. 하나의 PR만 병합되면 업무를 완료로 표시하지 않는다.
7. 사람의 올바른 병합 뒤 Wiki·Issue·Projects를 반영한다. 발행 실패는 코드 재실행 없이 복구한다.
8. 구현·의사결정·운영 문서가 서로 연결되고, 제안과 승인된 결정이 구분된다.

## 참고

- [GitHub Projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/learning-about-projects/about-projects)
- [GitHub Wiki](https://docs.github.com/en/communities/documenting-your-project-with-wikis/about-wikis)
- [Wiki Git 편집](https://docs.github.com/en/communities/documenting-your-project-with-wikis/adding-or-editing-wiki-pages)
- [PR 병합 결과 API](https://docs.github.com/en/rest/pulls/pulls#get-a-pull-request)
