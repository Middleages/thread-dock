# ThreadDock 새 세션 Handoff

## 이번 세션의 종료점

2026-09-09 기준. 현재 오류 처리 작업을 마무리하고 **새 세션에서 첫 실제 사용까지 이어간다**.
이번 세션에서는 새 기능이나 live pilot을 더 시작하지 않는다. 기반 코드와 Builder 연결은 구현됐지만,
사용자 명령부터 GitHub 발행·사람 병합·문서 완료까지의 실제 사용 흐름은 아직 완성되지 않았다.

## 새 세션에서 먼저 할 일

1. 이 문서와 [AGENTS.md](AGENTS.md), [CONTEXT.md](CONTEXT.md), [PRODUCT.md](PRODUCT.md),
   [현재 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md),
   [준비 계획](docs/superpowers/plans/2026-09-07-implementation-readiness.md)을 읽는다.
2. root와 아래 worktree의 `git status`, 실제 HEAD, remote main, PR #56의 병합 여부를 확인한다.
   오래된 `.worktrees/runtime-adapters-mvp`가 현재 작업 위치라고 가정하지 않는다.
3. [Herdr #3813](https://github.com/herdrdev/herdr/issues/3813)의 응답과 재현 해결 여부를 읽기 전용으로 확인한다.
4. 아래 순서에서 아직 끝나지 않은 가장 작은 slice를 Sol이 계획하고 Luna에 배정한다.
   기존 준비 계획은 환경 확인용이며 이미 완료된 foundation을 다시 구현하는 지시가 아니다.

| 항목 | 종료 시점 기준선 |
|---|---|
| 저장소 | `Middleages/thread-dock` |
| 확인한 main | `70e2e677a0d792d22392598c32906690622eba37` — PR #55 병합 |
| 현재 worktree | `/home/appuser/dev_system/.worktrees/herdr-builder-pilot` |
| 현재 branch | `agent/herdr-builder-pilot` |
| 진행 PR | [#56](https://github.com/Middleages/thread-dock/pull/56), main 병합은 사람의 GitHub 작업 |
| 최종 제품 코드 | `df7960f009efc7e6410a5d4666b885f33c9b9818` — 뒤의 문서 커밋과 구분 |
| 실행 도구 | native Go 없음. Docker `golang:1.27`로 검증 가능 |

PR 병합 뒤에는 실제 merge SHA로 기준선을 갱신한다. worktree를 정리하기 전 dirty 파일과
ignored `.superpowers` 보고서를 확인한다. 필요한 근거를 보존하지 않고 worktree를 삭제하지 않는다.

## 무엇을 구현했는가

| 범위 | 구현 상태 | 아직 남은 연결/검증 |
|---|---|---|
| Project/Work foundation — #49 | strict Contract v2, runtime envelope, registry, atomic/CAS/idempotent state, plan/approve/status | 실제 실행 진입점 |
| typed state — #50 | invocation, candidate, gate/review/integration evidence, pause/resume, budget, publication lifecycle | 실제 runner/publisher가 evidence를 생산하는 흐름 |
| coordinator — #51~54 | Work owner lease, publication queue, async runtime, one-shot reconcile, 같은 lease의 Activate | 실제 외부 시스템과 end-to-end 연결 |
| Herdr Builder — #55 | 기존 Herdr CLI 기반 bridge, profile/fingerprint binding, Git TreeSHA·branch·허용 경로 검증, crash 뒤 candidate 회수 | live 파일 편집·candidate_ready·행동 수준 권한 검증 |
| 오류 처리 — #56 | GetInfo stderr 처리, 안전한 Prompt code 전달, 실제 Launch 실패의 needs_operator 정착 | upstream startup 문제 해결은 별도 |
| 사용자 CLI | `project register/list/status`, `work plan/approve/status` | 실행·pause/resume/reconcile의 CLI 연결. Service 메서드는 일부 존재 |
| GitHub | 기존 v1 client와 v2 Publisher port/queue/state | v2 Issue·Projects·PR·Wiki 실제 adapter 연결 |
| Monitor | `internal/monitor` aggregate snapshot wire | 실제 Wails 화면·Windows→WSL 연결과 사용자 검증 |

현재 tree에서 Wails/frontend 실행 진입점은 확인하지 못했다. 설계의 “기존 Wails Monitor”를
사용 가능한 UI가 이미 있다는 뜻으로 해석하지 않는다. Reviewer·Documenter runtime 연결,
Go의 authoritative verification gate, 최종 문서/발행 완료 흐름도 아직 연결되지 않았다.
`work run` 같은 실행 명령이 이미 존재한다고 안내하지 않는다.

## 방금 끝낸 작업과 근거

- `070a348` 및 `df7960f`: `PromptError.Code()`는 고정 allowlist만 노출하고 provider message/body는 저장하지 않는다.
  `ErrRuntimePrompt`와 typed code를 함께 보존한다. malformed/mixed/unknown 오류는 기존 safe generic 오류다.
- 실제 `Runtime.Launch` 이후 실패만 현재 invocation의 `runtime_unknown`/`needs_operator`로 정착한다.
  caller 오류와 settlement 오류를 숨기지 않으며 pause/stale pre-I/O 거부와 cancellation 전용 처리를 유지한다.
- 실제 Store + public Coordinator + scripted Herdr CLI 테스트로 prompt 1회, waiter 완료,
  durable blocker, candidate 부재, secret 비영속화, replay 시 provider 재호출 없음까지 검증했다.
- Luna 구현·수정 → fresh Sol 검토. 최종 제품 SHA `df7960f009efc7e6410a5d4666b885f33c9b9818`는 **ACCEPT**, blocking 없음.
  역할 설정은 Luna high / Sol medium이며 실제 resolved model/effort telemetry는 `unverified`다.
- Docker Go 1.27 focused `go test -count=1 -timeout=120s ./internal/herdr ./internal/coordinator`,
  대응 `go vet`, formatting/diff check 통과. cancellation 회귀의 실제 RED→GREEN 근거가 있다.
- 최종 전체 gate 결과는 아래 검증 기록에 남긴다. scripted 테스트 성공은 live Builder 성공이 아니다.

과거 일부 foundation/ingestion 테스트의 사전 RED 누락은 audit gap으로 남는다.
과거 shared-state의 일부 matrix assertion 정밀도 보강은 해당 영역을 변경할 때 함께 다룬다.
원래 절차 누락이 후속 GREEN만으로 해소됐다고 주장하지 않는다.

## 실제 사용을 막는 문제와 보존 상태

[실행/권한 점검 기록](docs/operator/herdr-builder-capability-preflight.md)과
[Herdr 시작 readiness 보고서](docs/operator/herdr-opencode-startup-readiness.md)가 상세 근거다.

- 최초 live Builder는 Launch 1회 후 실패했다. 파일 편집·commit·candidate는 없으며 최초 Prompt의 정확한 code는 유실됐다.
- 별도 무도구 비교에서 시작 직후 Start→Get→Prompt는 Herdr 0.8.2와 임시 0.9.0 모두
  `agent_prompt_stalled`; 별도 단계의 Prompt는 정확한 응답을 확인했다. 최초 실패 code를 소급 확정하지 않는다.
- 설치된 default 서버는 0.8.2 그대로다. 임시 0.9.0 named server는 중지했다.
  upstream 이슈 제출은 완료했지만 응답/해결은 확인되지 않았다. 서버 업그레이드나 upstream 구현 PR은 별도 권한이다.
- native OpenCode `build`를 재사용했다. 저장소별 권한 합성은 확인했지만 실제 도구 거부 동작과 OS credential 격리는 검증하지 못했다.
- Herdr exact invocation 종료는 미지원이다. bridge `Terminate`는 명시적 미지원 오류를 반환한다.

원본 시험 root: `/tmp/threaddock-herdr-live.7YEhfV`.
state: `state/v2/work/WorkPilot/work.json`; Git worktree: `worktree`, branch `agent/pilot`, remote 없음.
관찰 전용 helper를 검토 후 한 번 실행해 **revision 5, Work/Task needs_operator, blocker runtime_unknown**으로 정착했다.
GetInfo 2회/Read 1회만 호출했으며 Prompt 재전송은 없다. LaunchRequested=true, candidate 없음,
Git HEAD `add7724386f95b3dfa03942d55d8e1909bd694dd`, `result.txt`는 `pending`, Git clean이다.

원본 invocation/agent를 자동 종료·재승인·재예약·재전송하지 않는다. helper를 다시 실행할 필요도 없다.
`/tmp`가 사라졌다면 같은 ID를 재생성해 복구인 것처럼 취급하지 않는다. 새 pilot은 원인 대응과 실행 조건을
먼저 확인하고 독립 root/ID로 진행한다. 단순 sleep/retry나 별도 readiness 감지기를 ThreadDock에 추가하지 않는다.

## 첫 실제 사용까지의 작은 구현 순서

전체 설계를 한꺼번에 구현하지 않는다. 다음은 우선순위이며 각 단계 착수 시 현재 코드를 대조해 작은 Task로 구체화한다.

1. **현재 PR 닫기와 live blocker 확인.** PR #56의 사람 병합을 확인한다. upstream 대응 또는 검증 가능한 입력 준비 방법이
   확보된 뒤 새 Builder pilot에서 실제 파일 변경→strict 결과→Git 검증→candidate_ready를 확인한다.
   upstream 대기 중에는 다음 CLI 연결을 scripted runner로 준비할 수 있지만 실사용 성공으로 보고하지 않는다.
2. **단일 저장소 실행 진입점 연결.** 기존 workflow/coordinator/Herdr bridge를 사용자 CLI에 얇게 연결한다.
   저장소 경로와 승인 profile/fingerprint를 명시적으로 바인딩한다. 재시작·pause/resume/reconcile도 기존 seam을 쓴다.
   종료 기준: 사용자가 plan/approve 뒤 같은 Work를 실행·조회·안전하게 재개할 수 있고 중복 Launch가 없다.
3. **검증과 독립 리뷰 연결.** Go가 승인된 검증 명령을 실행해 GateEvidence를 만든다. Builder의 자기보고는 gate가 아니다.
   기존 review evidence parser를 재사용해 정확한 request/SHA의 Reviewer 결과를 수용한다.
   readonly는 실제 runtime 권한으로 검증한다. v1에서 두 역할을 build에 매핑한 것은 readonly 근거가 아니다.
4. **GitHub 발행 연결.** 기존 client를 재사용해 v2 Issue·Projects·PR·Wiki adapter를 작은 순서로 연결한다.
   대상/권한/marker를 먼저 확인한다. 이전 점검의 Projects scope 부족·Wiki 비활성 상태는 재확인이 필요하다.
   모호한 write 결과는 conflict로 남기고 blind retry하지 않는다. 개발용 PR 작성 성공과 제품 Publisher 구현은 다르다.
5. **완료 흐름 연결.** docs와 최종 HEAD의 gate/review→PR→사람 병합→필수 문서/Wiki receipt를 연결한다.
   발행 실패는 pending/conflict로 보존하며 코드를 다시 실행하지 않는다. 정확한 취소가 불가능한 경우도 안전하게 드러낸다.
6. **최소 Monitor와 실제 한 건 사용.** UI 위치/실행환경부터 확인하고 목록·상세·blocker·다음 행동만 붙인다.
   Windows→WSL 호출을 검증한 뒤 지정된 저장소의 작은 실제 Work Item 한 건을 완료한다.

첫 실제 사용의 완료 기준은 **단일 Project/저장소에서 요청·승인→실제 구현→Go 검증→독립 리뷰→PR→사람 병합→필수 문서 완료**를
사용자 명령과 최소 Monitor로 확인하는 것이다. Builder smoke나 fake-backed 테스트만으로 이 기준을 충족하지 않는다.
여러 Project/저장소, 두 번째 runtime, DXHub 메뉴·MCP·공유 실행 제어는 이 흐름 이후로 둔다.

## 다음 세션의 작업 규칙

- Herdr가 제공하는 시작/조회/입력 기능을 재사용한다. 별도 session/process registry나 중복 실행 관리자를 만들지 않는다.
- public interface/shared types는 직렬 합의 후 고정한다. 독립 Task만 별도 worktree/branch로 병렬화한다.
- AGENTS의 전체 Task packet을 사용한다. 구현 worker 총 3개 이내, fresh reviewer 슬롯을 남긴다.
  Sol은 계획·문서·Git 통합, Luna는 제품 코드·테스트·수정 소유자다. 지원 안 되는 모델을 임의 대체하지 않는다.
- Task별 focused 검증, 마지막 통합 `make check` 한 번. 동일 SHA/command/환경의 성공 근거를 중복 실행하지 않는다.
  docs-only는 링크/일관성 검증으로 구분한다. 네이티브 Go가 없다는 이유로 Docker 검증을 생략하지 않는다.
- PR·Issue 제목/본문은 한국어. 승인된 push 범위는 유지하되 main 병합은 사람이 한다.
- 기존 v1의 전체 상태 기계를 복제하거나 예전 Work를 v2로 자동 재실행하지 않는다.

## 검증 기록

제품 SHA `df7960f009efc7e6410a5d4666b885f33c9b9818`에서 다음 최종 전체 gate를 한 번 실행해
exit 0을 확인했다. formatting, shell syntax, 전체 vet와 전체 Go test가 통과했다.

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 make check
```

이후 변경은 HANDOFF와 operator 기록뿐이다. 문서의 로컬 링크와 `git diff --check`를 별도 확인한다.
