# ThreadDock 작업 정책

## 착수와 범위

- 먼저 HANDOFF.md, PRODUCT.md, CONTEXT.md, docs/operator/github-first-quickstart.md와 현재 2026-09-10 설계·계획을 읽는다.
- 중앙 관제·기능별 Herdr 세션·native subagent·GitHub 기록이 기본 사용 흐름이다. 제품은 Go/Wails 모니터와 기존 React 화면이다. Go 백엔드에서 GitHub·Herdr를 조회·결합한다.
- 과거 2026-09-07 spec/준비 계획과 초기 herdr-first-01-status-wire는 신규 구현의 지침이 아니다. PR #69의 Go/Wails 이식과 현재 트리의 옛 실행 엔진 제거를 반복하지 않는다. 남은 Go package는 monitor, internal/runner, internal/projecttemplate다. 검증·병합 상태는 HANDOFF.md와 docs/operator/2026-09-12-engine-retirement-completion-ledger.md를 따른다. 브라우저 전용 실행 지침은 폐기한다.
- Vite는 화면 개발·빌드만 담당한다. 별도 Node 조회 서버·HTTP bridge·새 실행 엔진을 추가하지 않는다.
- 기존 Work/Contract v2, Invocation/Artifact, Go Task Gate/Publisher를 필수 입력으로 요구하지 않는다.
- 개발 판단·기능 분배는 Agent, 세션 관리는 Herdr, 업무/문서 원본은 GitHub가 맡는다. 작은 관찰·연결 기능만 코드로 만든다.
- 사용자가 승인한 범위의 통상 작업은 진행한다. 명시적 중단은 새 실행 지시가 있을 때까지 유지하며 반복 중단 알림을 보내지 않는다.
- 사용자 호스트의 Codex 실험/dirty 파일은 별도 지시 없이 reset·삭제·재개하지 않는다.
- 이 저장소의 Sol/Luna 역할은 ThreadDock 개발용이며 대상 프로젝트에 모델 설정을 자동 복사하지 않는다.

## 역할과 모델

- `td_coordinator`: Sol, medium. 계획·분배·상태/근거 ledger·문서·Git 통합을 조정한다.
- `td_reviewer`: Sol, medium. 고정 SHA와 diff를 읽는 fresh 독립 리뷰를 수행한다.
- `td_implementer`: Luna, high. 할당된 worktree에서 제품 코드·테스트를 구현하고 수정한다.
- 모델/effort 자동 승격은 금지한다. 환경이 모델을 지원하지 않으면 정확히 보고하고 임의 대체하지 않는다.
- Sol은 제품 코드·테스트·충돌 수정의 소유자가 아니다. Luna가 구현하고, 사람은 main 병합을 수행한다.
- reviewer의 읽기 전용은 역할 지침이다. sandbox·승인·인증 정책은 기존 실행 환경을 상속하며, 이 설정이 OS 수준의 쓰기 차단을 보장한다고 주장하지 않는다.

## 계획·분배

- 최초 public interfaces와 shared types는 직렬로 먼저 합의한다. 그 뒤 의존성이 없는 Task를 병렬화한다.
- 모든 Task packet에 정확한 `taskId`, `baseSHA`, `deps`, `ownedPaths`, `worktree`, `branch`, `forbiddenPaths`, `interface`, `acceptance`, `tests`, `result`를 넣는다. `result`에는 `changedFiles`, `commitSHA`, `executedCommands`, `outcomes`, `unverified`, `blockers`를 포함한다.
- 구현 worker는 전체 트리 합산 최대 3개인 목표 상한이며, 항상 3개를 실행할 의무는 없다. 예약 ledger에서 관리한다.
- `agents.max_concurrent_threads_per_session=6`은 주 세션을 제외한 열린 thread 예산이다. root는 대기·종료 thread를 정리하고 필요하면 평탄화해 슬롯을 확보하며, reviewer용 1슬롯 여유를 권장한다.
- root가 경로·슬롯·범위를 배정한 Sol subcoordinator만 한 번 하위분배할 수 있다: `root→Sol→Luna` 또는 `root→Luna`.
- 독립 Task는 각자 독립 worktree와 branch를 사용한다. 공유 checkout에서 parallel checkout/add/commit을 하지 않는다.
- `go.mod`, `go.sum`, shared types, CLI 공유 파일은 동시에 여러 worker가 소유하지 않는다. public API 변경은 Sol이 재계획한다.
- 한 Task가 막혀도 다른 독립 Task는 계속한다. 상태와 근거를 ledger에 남겨 중복 분배를 막는다.

## 구현·검토

- 개발 과정에서 GitHub PR·Issue를 만들 때 제목과 본문은 한국어로 작성한다. 코드 식별자·명령·고유명사는 원문을 유지한다.
- 흐름은 `Luna 구현+테스트+self-review → fresh Sol td_reviewer task-review → Luna fixes`다. coordinator 자기검토로 reviewer를 대체하지 않는다.
- 통합 `make check`가 실패하면 Sol이 재현 원인과 소유 Task를 귀속해 Luna에 수정 배정한다. blocking 사항을 완료로 꾸미지 않는다.
- 동일 원인 수정이 두 번 반복되면 Sol이 새 정보가 반영된 brief로 재분해·재배정한다. 새 정보 없는 반복은 금지한다.
- Task 검증은 직접 변경과 의존 영향에 필요한 focused 범위만 한다. 수정 후에는 영향받는 기존 테스트만 재실행하며 새 회귀 위험이 있을 때만 테스트를 추가한다.
- 동일 SHA·command·환경 증거가 있으면 재실행하지 않는다. 동일 SHA·검토 범위의 중복 리뷰는 하지 않되, Task 리뷰와 전체 통합 리뷰는 범위를 구분한다. worker별 full suite는 금지한다.
- 통합 code PR의 마지막 `make check`만 전체 gate로 한 번 수행하고, 명확한 gate 또는 검증 무효화 근거가 있을 때만 재실행한다.
- 병렬 테스트는 cache·port·resource를 분리하고 root가 중복 실행을 제거한다. 자원 집약 테스트는 직렬 또는 소규모 batch로 실행한다.
- docs/config-only 변경은 parse/link 검증이면 충분하며, 같은 tuple의 신뢰할 수 있는 CI full 결과는 채택할 수 있다. suite를 조용히 생략하거나 성공으로 꾸미지 않는다.
- 실행 불가능하거나 제품 범위를 바꾸는 결정만 사용자에게 문의한다.
- 승인된 push 등 기존 범위는 보존한다. main 병합은 사람의 GitHub 작업이다.
- 문제·blocker·결과와 근거 링크를 handoff/ledger에 기록한다.
