# ThreadDock 다음 세션 Handoff

## 현재 목표

여러 프로젝트의 요청·의사결정·구현·문서를 연결하고,
프로젝트를 바꿔도 현재 상태와 다음 행동을 즉시 회수하는 MVP를 만든다.

- [현재 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md)
- [제품 범위](PRODUCT.md)
- [용어 모델](CONTEXT.md)
- [전환 결정 ADR 0006](docs/adr/0006-project-workflow-mvp.md)
- [구현 준비 계획](docs/superpowers/plans/2026-09-07-implementation-readiness.md)

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
- 1차는 GitHub Projects·Issue·PR·Wiki 기반이며 기존 Wails Monitor를 함께 구현한다.
- DXHub 프로젝트 메뉴·MCP 연동과 공유 실행 제어는 후속 범위다. 1차 실행 소유자는 한 운영자다.

## 설계 기본값

전역 호출 한도 2, Project별 변경 업무 1개, Task별 자동 수정 2회,
확실한 일시적 호출 실패 재시도 1회를 시작 기본값으로 한다.
이는 무제한 자동 진행을 방지하는 구현 선택이며 실제 사용 근거로 조정할 수 있다.
권한은 Trusted Workstation과 runtime 정책 수준으로 설명하고
완전한 credential 격리를 했다고 주장하지 않는다.

## 개발 세션의 모델과 병렬 실행

사용자 지정 역할은 [AGENTS.md](AGENTS.md)와 [.codex/config.toml](.codex/config.toml)에 둔다.
계획·Task 분배·검토는 Sol medium, 구현·테스트·수정은 Luna high다.
독립 Task는 worktree와 파일 소유권을 나눠 최대 3개 구현 worker로 병렬 진행한다.
필요하면 Sol 하위 조정자가 할당받은 범위와 슬롯 안에서 Luna에게 분배한다.
이는 ThreadDock을 만드는 개발 세션 설정이며 제품의 runtime profile·호출 한도와 별개다.

실제 checkout에서 최신 설계 branch를 가져온 뒤 Codex 세션을 시작한다.
프로젝트 설정은 신뢰된 프로젝트에서 로드되며, 사용자/세션 override가 있으면 실제 모델을 확인한다.
모델을 사용할 수 없거나 CLI가 역할 설정을 지원하지 않으면 이유를 보고하고 임의 모델로 대체하지 않는다.
준비 계획 확인 후 Sol이 첫 단일 저장소 흐름의 구체적 구현 계획을 작성하고 Luna에 배정한다.
기존 main 병합은 사람에게 맡긴다.

## 다음 구현 순서

0. 준비 계획에 따라 실제 workstation의 도구·권한·repository 실행 환경을 확인한다.
1. 단일 저장소·runtime 하나로 Issue/결정 → 구현·검증·리뷰 → docs·최종 gate → PR → 사람 병합 → Wiki/완료를 연결한다. 작은 Monitor 목록·상세를 함께 제공한다.
2. 제한 수정·재검토, pause/resume, 불명확한 종료·발행 실패 복구를 확인한다.
3. 여러 프로젝트 진행, 두 runtime의 공통 계약, 물리 저장소 충돌을 검증한다.
4. 다중 저장소 의존성·검증과 PR 준비·부분 병합을 추가한다.

Go Publisher는 Main Agent 대화와 독립적으로 승인된 진행과 발행을 수행한다.
GitHub 수동 편집은 원격 업무 기록과 실행 계약을 구분하고 충돌을 표시한다.
저장소 setup/check/service 명령과 필요한 환경 변수 이름을 승인된 executionProfile에 둔다.
구현 준비 계획은 환경 확인과 첫 구현 범위를 정한다. 전체 MVP를 한 번에 구현하라는 지시가 아니다.
삭제된 옛 계획이나 single-run/parallel pilot 절차를 현재 실행 계획으로 되살리지 않는다.

## 2026-09-08 구현 착수 결과

첫 단일 저장소 흐름의 상세 계획을
[`docs/superpowers/plans/2026-09-07-single-repository-workflow.md`](docs/superpowers/plans/2026-09-07-single-repository-workflow.md)에 추가했다.
기존 v1을 수정하지 않고 다음 직렬 foundation을 구현·통합했다.

- `internal/contract/v2`: strict Contract v2 codec·validation과 단일 저장소 fixture.
- `internal/runtime`: provider-neutral Invocation/Artifact envelope와 identity/schema 검증.
- `internal/monitor`: CLI와 이후 Wails가 함께 사용할 aggregate snapshot wire.
- `internal/registry`, `internal/state/v2`: 별도 v2 경로, immutable contract, process lease,
  atomic persistence, revision CAS와 requestId/payload idempotency. 생성도
  `expectedRevision=0`과 requestId를 사용하며 replay 결과는 중첩되지 않는 bounded projection이다.
- `internal/workflow`: project 등록, work plan→approve→status와 aggregate snapshot.
- `internal/cli`, `cmd/agentctl`: `project register/list/status`, `work plan/approve/status`와
  기존 GHES/Herdr 초기화에서 분리된 v2 dependency 경로.

각 Task는 Luna high 요청으로 구현하고 fresh Sol medium 요청으로 검토했지만,
현재 orchestration metadata가 실제 resolved model·reasoning effort를 노출하지 않아 역할 실행은
`unverified`로 기록했다. Reviewer 지적은 Luna 수정 후 scoped Sol 재검토를 거쳤다.

통합 코드 HEAD `ae5034536969fc8e84463dc6c29e479814f27454`에서 아래 focused 검사와
repository 전체 gate가 통과했다.

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -count=1 \
  ./internal/contract/v2 ./internal/runtime ./internal/monitor ./internal/registry \
  ./internal/state/v2 ./internal/workflow ./internal/cli ./cmd/agentctl
docker run --rm -v "$PWD":/src -w /src golang:1.27 go vet \
  ./internal/contract/v2 ./internal/runtime ./internal/monitor ./internal/registry \
  ./internal/state/v2 ./internal/workflow ./internal/cli ./cmd/agentctl
docker run --rm -v "$PWD":/src -w /src golang:1.27 make check
```

첫 `make check`는 다른 repository의 동시 Docker 검증 부하 중 기존 orchestrator 1초 대기 테스트가
실패했고, 같은 테스트를 단독 실행했을 때 0.24초에 통과했다. 해당 자원 집약 검증이 끝난 뒤 허용된
전체 gate 재실행이 통과했다. final whole-branch review가 찾은 생성 멱등성, 원래 결과 replay와
immutable contract/hash 대조 문제를 수정·재검토한 뒤, 변경으로 무효화된 전체 gate도 다시 통과했다.
foundation 변경은 [PR #49](https://github.com/Middleages/thread-dock/pull/49)에 push했다.
확인 시점의 PR HEAD는 `4e2a91fb9043f3c926382247ad4b40ea718fd7ba`이고 GitHub 판정은
`MERGEABLE/CLEAN`이었다. main 병합은 Operator가 Create a merge commit으로 수행한다.
WSL native Go, Windows Go/Wails/WSL bridge, 실제 Codex invocation, GitHub Projects와 Wiki 쓰기도
아직 검증하지 않았다. 현재 GitHub token은 `read:project` scope가 없고 대상 저장소 Wiki는
비활성화 상태이므로 live publication은 진행하지 않았다.

다음 행동은 계획 Task 4·5를 바로 병렬 구현하는 것이 아니라, 먼저 공용 state interface를
작게 재계획하는 것이다. Task 4에는 pending→completed 외부 발행 receipt lifecycle이,
Task 5에는 Task별 invocation·termination·candidate/review evidence 상태가 필요하지만 현재
`state/v2.WorkSnapshot`에는 아직 이 공유 구조가 없다. Sol이 이를 단독 소유 Task로 고정하고
fresh review를 통과시킨 뒤, 경로가 분리된 Publisher와 runtime/Git 실행 Task를 병렬 배정한다.

## 재사용과 검증

### Shared-state core 구현

PR #49 merge commit `1d7fa1b`를 기준으로 `agent/workflow-shared-state`에서 typed Work/Task/publication state core를 구현했다. 구현 계획은 [`docs/superpowers/plans/2026-09-08-workflow-shared-state-core.md`](docs/superpowers/plans/2026-09-08-workflow-shared-state-core.md), 다음 coordinator 계획은 [`docs/superpowers/plans/2026-09-08-work-item-coordinator.md`](docs/superpowers/plans/2026-09-08-work-item-coordinator.md)다.

core는 typed `Store.Apply`, 승인·pause/resume·resolution, builder/reviewer invocation lifecycle, candidate/gate/review/integration evidence, repair/recovery budget, publication generation/reconcile와 privacy-safe Monitor projection을 포함한다. final review와 residual review의 production finding을 수정한 최종 통합 코드 HEAD는 `95831b4715325dd84ed84bf435bc06ec44ae9ea4`다.

Docker Go 1.27 `make check`가 최종 통합 코드에서 통과했다. native WSL Go, Windows/Wails bridge, 실제 runtime과 GitHub Projects/Wiki publication은 아직 `unverified`다. 다음 행동은 coordinator 계획을 검토하고 owner lease→publication queue→async runtime→restart reconcile 순서로 실행하는 것이다. 이 계획이 끝나기 전에는 Publisher/runtime adapter를 독립적으로 launch하지 않는다.

Task review breaker에서 남은 항목은 production blocker가 아니라 검증 기록·test precision이다.
Task 4의 초기 RED 일부는 최종 동작을 증명하지 못하므로 audit gap으로 남겼다. Task 5의 일부
publication matrix는 named guard보다 앞선 guard에서 실패하며, Task 6의 task count·empty-array
assertion도 더 엄밀하게 만들 수 있다. final review가 찾은 실제 authorization·lineage 결함은
별도 residual Task와 fresh review로 수정했다. 후속 변경이 이 영역을 건드리면 해당 parked test를
먼저 보강하고 동일한 성공 근거로 간주하지 않는다.

기존 contract, state, pathscope, worktree, integration과 Herdr의 좁은 기능을 검토해
재사용한다. 기존 자동 병합·세션 복구 상태 기계 전체를 보존할 의무는 없다.
기존 project-template은 v1 자료이며 새 MVP 자동 설치에 사용하기 전에 새 계약에 맞춰 정리한다.

각 구현 Task는 변경과 의존 영향에 해당하는 focused 검사만, 수정은 해당 covering 검사만 수행한다.
Sol 검토는 동일 SHA·명령·환경에서 이미 통과한 검사를 반복하지 않는다.
전체 make check는 통합된 코드 PR의 최종 검증 때 한 번 실행한다. worker마다 full suite를 실행하지 않는다.
전체 재실행은 필수 gate 또는 기존 검증을 무효화한 구체적 변경 근거가 있을 때만 한다.
새 설계 문서의 인수 조건을 실제 코드 검증으로 대체하지 않는다.
문서 변경만 수행한 세션은 링크·일관성 검사와 코드 실행 검사를 구분해 보고한다.
실사용 파일럿과 GitHub 외부 업무 생성은 승인된 대상·범위에서만 수행한다.

예전 실행 근거가 필요하면 Git 이력의 문서와 기존 외부 기록을 확인한다.
새 v2가 기존 v1 상태 파일을 자동 변환하거나 예전 업무를 다시 실행하게 하지 않는다.
