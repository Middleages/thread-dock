# ThreadDock 다음 세션 Handoff

## 현재 목표

여러 프로젝트의 요청·의사결정·구현·문서를 연결하고,
프로젝트를 바꿔도 현재 상태와 다음 행동을 즉시 회수하는 MVP를 만든다.

- [현재 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md)
- [제품 범위](PRODUCT.md)
- [용어 모델](CONTEXT.md)
- [전환 결정 ADR 0006](docs/adr/0006-project-workflow-mvp.md)

## 기준선과 변경 상태

- Repository: Middleages/thread-dock.
- 구현 기준 main: ce42f403b0a758acf3e647394b1c04a5f2c447c7.
- 작업 브랜치: agent/runtime-adapters-mvp.
- 최초 runtime-adapters 설계 커밋: f669ec6008ee3ccc14d688aad83700a1da84fceb.
- 이 handoff는 설계 전환을 기록한다. 새 MVP 구현·UI·실사용 검증은 아직 완료되지 않았다.
- 새 세션에서는 실제 branch HEAD와 local/remote 변경을 확인하고 작업을 시작한다.

## 확정된 제품 방향

- Project 하나에 Repository 여러 개, Work Item 하나에 저장소별 실행·PR 여러 개를 지원한다.
- 대표 저장소에 Parent Issue와 Wiki를 두고 공용 Projects 보드에서 업무를 모은다.
- Monitor의 목록·상세, 중간 handoff, 결정 이력과 Wiki는 MVP에 포함한다.
- 여러 Project를 동시에 진행하고 완료 Task를 보존하며 작업을 재개한다.
- 제한된 자동 수정·재검토를 제공하고 모든 main 병합은 사람이 한다.
- 필수 PR 일부의 병합은 전체 업무 완료가 아니다.
- 기존 파일럿 Run을 새 MVP에서 이어갈 요구는 없다.
- 혼동을 만드는 이전 설계·계획 파일은 현재 tree에서 제거하고 Git 이력으로 남긴다.

## 설계 기본값

전역 호출 한도 2, Project별 변경 업무 1개, Task별 자동 수정 2회,
확실한 일시적 호출 실패 재시도 1회를 시작 기본값으로 한다.
이는 무제한 자동 진행을 방지하는 구현 선택이며 실제 사용 근거로 조정할 수 있다.
권한은 Trusted Workstation과 runtime 정책 수준으로 설명하고
완전한 credential 격리를 했다고 주장하지 않는다.

## 다음 구현 순서

1. Project/Repository/Work Item 식별자와 Contract v2, 상태 revision·CLI 계약.
2. Issue·Projects 매핑, 결정·handoff와 Monitor 목록·상세.
3. Go Git·검증, Runtime adapter, Task 리뷰·수정·pause/resume.
4. 다중 저장소 의존성·검증과 PR 준비·부분 병합.
5. repository docs 최종 gate, Wiki 발행·재시도와 전체 Finalize.

각 단계의 파일·명령·검증을 구체화한 구현 계획을 새 설계에 맞춰 작성한다.
삭제된 옛 계획이나 single-run/parallel pilot 절차를 현재 실행 계획으로 되살리지 않는다.

## 재사용과 검증

기존 contract, state, pathscope, worktree, integration과 Herdr의 좁은 기능을 검토해
재사용한다. 기존 자동 병합·세션 복구 상태 기계 전체를 보존할 의무는 없다.
기존 project-template은 v1 자료이며 새 MVP 자동 설치에 사용하기 전에 새 계약에 맞춰 정리한다.

구현 중에는 focused 검증을, 코드 변경 완료 시에는 make check를 실행한다.
새 설계 문서의 인수 조건을 실제 코드 검증으로 대체하지 않는다.
문서 변경만 수행한 세션은 링크·일관성 검사와 코드 실행 검사를 구분해 보고한다.
실사용 파일럿과 GitHub 외부 업무 생성은 승인된 대상·범위에서만 수행한다.

예전 실행 근거가 필요하면 Git 이력의 문서와 기존 외부 기록을 확인한다.
새 v2가 기존 v1 상태 파일을 자동 변환하거나 예전 업무를 다시 실행하게 하지 않는다.
