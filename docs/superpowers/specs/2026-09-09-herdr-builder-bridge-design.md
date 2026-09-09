# Herdr Builder 연결 설계

## 목적과 범위

Herdr가 관리하는 OpenCode Builder 하나를 ThreadDock v2 업무 흐름에 연결한다. 기존 Herdr 호출·결과 parser·Git 검사를 재사용하고, 승인된 invocation과 실제 candidate를 연결하는 부분만 구현한다.

기준 코드는 `2e3d4c8250052ae1b4e67e65eb60c881f8d710e7`이다. 사용자가 Herdr 우선 경로와 중복 구현 축소를 선택했다. 이는 [구현 준비 계획](../plans/2026-09-07-implementation-readiness.md)의 Codex 우선 기본값에 대한 이번 slice의 변경이다. [제품 설계](2026-09-07-project-workflow-mvp-design.md)와 [shared state 설계](2026-09-08-workflow-shared-state-design.md)의 업무 identity·검증·승인 정책은 유지한다.

첫 범위는 기존 Worktree에서 Builder 실행 → 결과 회수 → Git 대조 → `candidate_ready`다. Reviewer·Documenter 연결, GitHub 발행, UI, 새 process registry는 후속이다. 제품 코드 구현 전 사용하는 설계이며 live capability 검증 완료를 의미하지 않는다.

## 중복 구현 검토

| 책임 | 현재 코드와 확인 결과 | 이번 결정 |
|---|---|---|
| Herdr workspace와 agent 조작 | `internal/herdr/cli.go`의 OpenWorktree, StartAgent, Prompt, GetInfo | 기존 CLI를 호출한다. 별도 session manager를 만들지 않는다. |
| Codex/OpenCode 지원 | Herdr와 달리 현재 Go StartAgent/ResumeAgent는 `--kind opencode`를 고정한다. GetInfo는 provider identity를 읽는다. | Herdr 자체의 지원과 Go wrapper의 지원을 구분한다. 첫 Builder는 OpenCode로 연결한다. |
| 구조화 결과 추출 | ReadEvidence와 ReadReviewEvidence가 marker·크기·JSON을 검사한다. Builder ReadEvidence는 예상 requestId와의 일치까지 검사하지 않는다. | parser를 재사용하고 bridge가 현재 invocation ID와 정확히 대조한다. |
| runtime 계약 | `internal/runtime.Invocation`에 packet/profile/schema/권한 intent가 있다. `coordinator.Runtime`은 lifecycle만 표현한다. | 두 번째 packet/envelope를 새로 만들지 않고 기존 Invocation·ArtifactEnvelope를 연결한다. |
| 업무 실행 제어 | coordinator가 owner lease, 중복 실행 억제, pause, 취소, state 전이를 담당한다. | Herdr의 agent 상태 감지와 다른 책임이므로 유지한다. bridge에 업무 상태 기계를 복제하지 않는다. |
| 실행 중·재시작 처리 | `coordinator/runtime.go`와 `reconcile.go`가 identity 검사·종료·unknown 처리를 각각 구현한다. | 내부 중복 후보다. 이번 결과 수용 로직은 한곳에 두고 두 경로가 호출한다. 기존 전체 모듈 재작성은 별도 작업이다. |
| v1 실행 정책 | `internal/orchestrator`가 Herdr/Git 호출과 기존 Run 정책을 결합한다. | v2에 통째로 연결하지 않는다. v1 호환 코드는 이번에 삭제하지 않는다. |
| Git 근거 | `worktree.Git.InspectCommit`은 commit 존재·branch 포함 관계·changed files·patch를 확인한다. TreeSHA는 반환하지 않는다. | 기존 검사를 재사용한다. 필요한 TreeSHA 조회와 허용 경로 검사를 같은 Git 검사 경로에 보완한다. |
| 종료 | 현재 Herdr wrapper에는 exact invocation 중단·종료 확인 메서드가 없다. CloseWorkspace는 workspace 정리 기능이다. | workspace close를 invocation 종료로 대신하지 않는다. 설치 capability를 확인한 후 좁은 종료 호출만 보완한다. |

결론: provider session 관리 자체를 중복 구현한 증거는 확인하지 못했다. 다만 runtime 계약 두 개와 coordinator 내부 실행/재시작 settlement는 통합 시 중복을 만들기 쉬운 지점이다. 이전 설명의 “Herdr lifecycle이 전부 연결되어 있다”는 판단은 과했다. 종료·권한 적용은 아직 확인이 필요하다.

## 책임과 데이터 흐름

1. Coordinator가 승인된 Contract와 현재 InvocationState로 기존 `internal/runtime.Invocation`을 만든다. RequestID는 현재 InvocationID와 같고 role은 Builder, profile과 worktree는 저장된 실행 identity에 고정한다. Packet에는 해당 Task의 허용 경로·수용 조건과 결과 schema를 담는다.
2. Lifecycle Launch 입력에 이 Invocation을 전달할 수 있도록 기존 interface를 확장한다. state identity는 실행 승인 근거, Invocation은 provider 전달물이다. 두 값의 ID/profile/worktree를 호출 직전에 대조한다.
3. Herdr bridge가 기존 Worktree를 열고 선택된 OpenCode profile로 agent를 시작해 packet을 전달한다. 이름은 invocation별로 결정적으로 대응시키되 Herdr 이름 길이·문자 제약과 충돌 검사를 적용한다. 재시작 회수는 Herdr의 조회 결과와 저장된 정확한 identity로 한다.
4. Bridge는 Herdr 상태를 관찰하고 완료된 호출의 ReadEvidence 결과를 기존 ArtifactEnvelope로 변환한다. Observe 결과에 optional Artifact를 전달하며 기존 RuntimeResult를 통해 queue로 돌려보낸다. Bridge는 Store.Apply를 호출하지 않는다.
5. 결과 수용 처리는 현재 invocation/role/attempt와 종료 확인을 검증하고 Git inspector로 candidate를 대조한다. 검증된 TreeSHA·changed files·허용 경로를 사용해 TaskRecordCandidate를 한 번 적용한다. 실행 중 결과와 재시작 후 결과는 같은 수용 함수를 사용한다.

기존 AgentRuntime.Invoke와 lifecycle Runtime을 각각 독립 실행 경로로 구현하지 않는다. Invocation/ArtifactEnvelope 타입과 envelope 검사를 재사용하고, 이번 실행 진입점은 Coordinator다.

## 완료와 실패 의미

- Herdr의 idle/done 표시는 현재 packet의 실행 종료 증거와 구분한다. session이 살아 있어도 invocation이 끝날 수 있다. 정확한 session·invocation 대응과 현재 결과를 확인해야 종료로 정착한다.
- Terminate의 nil은 해당 invocation 종료 확인 완료를 뜻한다. signal 전달 성공만으로 nil을 반환하지 않는다. 정확한 중단/확인이 불가능하면 오류 또는 unknown으로 남긴다.
- Prompt 전달 결과가 모호하면 같은 packet을 자동 재전송하지 않는다. ReadPromptReceipt의 단순 문자열 포함 검사는 보조 정보이며 수신·완료의 단독 증거가 아니다.
- Artifact의 requestId·role 불일치, malformed/trailing/oversized JSON은 candidate로 수용하지 않는다. 현재 호출과 연관된 잘못된 결과는 기존 malformed_artifact/evidence_mismatch blocker로 기록한다. 다른 호출의 늦은 결과는 현재 evidence를 덮어쓰지 않는다.
- Agent의 verification 문구는 authoritative gate가 아니다. Git 근거와 후속 Go 검사 결과를 각각 검증한다.
- 중복 결과는 동일 candidate에 대해 멱등 처리한다. 기존 candidate와 충돌하는 결과는 자동 덮어쓰기하지 않는다.

## 구현 전 capability 확인

로컬 Herdr 0.8.2의 버전·help만 확인됐다. 실제 agent 실행, native profile 권한, 정확한 invocation 중단/종료 확인은 미검증이다.

구현계획은 설치된 Herdr/OpenCode 도움말과 기존 CLI 테스트를 읽어 다음을 고정한다: 기존 Worktree 열기, profile 선택, packet 전달 완료 의미, agent/session 재조회, 현재 invocation 중단과 종료 확인, publication credential·MCP 쓰기 제한 적용 방법. 지원하지 않는 명령을 추측하지 않는다. 필요한 capability가 없으면 해당 연결의 구체적 제약을 보고하며 session/process 관리 대체 시스템을 추가하지 않는다.

## 구현 단위와 검증

첫 Task는 기존 Invocation/ArtifactEnvelope가 coordinator를 통과하도록 interface와 공통 Builder 결과 수용 함수를 연결한다. 실제 Store와 임시 Git 저장소로 current/stale/malformed/중복 결과, 종료 전 결과 거부, 허용 경로·TreeSHA 검증을 테스트한다.

두 번째 Task는 기존 Herdr CLI를 사용하는 bridge다. 기록된 응답 fixture로 Start/Prompt/GetInfo/ReadEvidence 호출과 identity mapping, 모호한 prompt 결과의 재전송 금지, 종료 확인을 검증한다. 첫 Task에 의존하므로 직렬 구현한다.

Luna high가 제품 코드·테스트를 구현하고 fresh Sol medium이 검토한다. 수정 검증은 영향받는 범위만, 최종 code PR에서 make check를 한 번 수행한다. PR·Issue는 한국어로 작성하며 main 병합은 사용자에게 맡긴다.

실제 Herdr capability 시험은 구현·검토 후 지정된 작은 Worktree와 Task에서 수행한다. fixture 통과를 live 검증으로 보고하지 않는다. 성공 기준은 Builder 한 번 실행, 현재 requestId 결과 회수, Git 대조, candidate_ready 정착이며 Reviewer·PR·Wiki 완료는 포함하지 않는다.
