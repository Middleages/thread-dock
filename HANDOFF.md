# ThreadDock 새 세션 Handoff

## 이번 세션의 종료점

2026-09-09 기준. GitHub 기반 첫 실제 사용을 위한 단일 저장소 foreground 실행은
**후보 commit의 authoritative Go 검증 gate까지** 연결됐다. 다음 가장 작은 slice는 같은
후보와 SHA에 대한 fresh Reviewer 호출·ReviewEvidence 기록이고, accept된 후보만 integration으로
넘기는 흐름이다.

실제 Herdr/OpenCode Builder 성공은 아직 확인하지 못했다. 아래 end-to-end 근거는 real v2 Store와
임시 Git 저장소를 사용했지만 runtime은 scripted다. GitHub Publisher, repository docs·Finalize,
Wails Monitor와 live Herdr 사용은 남아 있다. DXHub 메뉴·MCP와 공유 실행 제어는 후속 범위다.

## 새 세션에서 먼저 할 일

1. 이 문서와 [AGENTS.md](AGENTS.md), [CONTEXT.md](CONTEXT.md), [PRODUCT.md](PRODUCT.md),
   [현재 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md),
   [준비 계획](docs/superpowers/plans/2026-09-07-implementation-readiness.md)을 읽는다.
2. 착수 checkout의 `git status`, HEAD와 `origin/main`을 확인한다. 아래 기준보다 main이 앞서면
   새 변경과 PR 상태를 먼저 읽는다. 오래된 worktree나 branch를 현재 작업 위치로 가정하지 않는다.
3. [Herdr #3813](https://github.com/herdrdev/herdr/issues/3813)의 새 응답·상태를 읽기 전용으로 확인한다.
   2026-09-09 마지막 확인에는 OPEN, comment 없음, `maintainer-needed`였다. 보존된 invocation은
   조회·재시도·종료하지 않는다.
4. Reviewer slice의 public interface와 shared type 변경 필요성을 먼저 대조한다. 변경이 필요하면
   Sol이 직렬로 재계획하고, 아니면 아래 고정 경계 안에서 Luna에 작은 Task를 배정한다.

| 항목 | 현재 기준선 |
|---|---|
| 저장소 | `Middleages/thread-dock` |
| main / origin/main | `e4c1be8e0e940dcc55052a4499900ffaa7291188` — PR #61 병합 |
| 열린 PR | 없음 |
| 최근 병합 | [#56](https://github.com/Middleages/thread-dock/pull/56)~[#61](https://github.com/Middleages/thread-dock/pull/61) |
| 실행 도구 | native Go 없음. Docker `golang:1.27` 검증 경로 사용 |

독립 Task는 최신 main에서 새 worktree와 branch를 만든다. 정리 전 dirty/ignored 근거를 확인하고,
필요한 근거를 보존하지 않은 채 이전 worktree를 삭제하지 않는다. main 병합은 사람의 GitHub 작업이다.

## 현재 구현 상태

| 범위 | 구현·검증된 상태 | 아직 남은 연결/검증 |
|---|---|---|
| Project/Work foundation — #49 | strict Contract v2, registry, atomic/CAS/idempotent state, plan/approve/status | 실제 GitHub 업무 등록 |
| typed state — #50 | invocation, candidate, gate/review/integration evidence, pause/resume, budget, publication lifecycle | 남은 runner/publisher가 evidence를 생산하는 흐름 |
| coordinator — #51~54 | Work owner lease, publication queue, async runtime, one-shot reconcile, 같은 lease의 Activate | background 사용자 흐름과 외부 adapter |
| Herdr Builder — #55~56 | profile/fingerprint binding, strict Artifact와 Git 검사, crash 뒤 candidate 회수, safe Prompt 오류 처리 | startup blocker 해결 뒤 live 변경·candidate 확인, 행동 수준 권한 검증 |
| 사용자 CLI — #57, #60 | `project register/list/status`, `work plan/approve/status/pause/resume/reconcile/run`; 단일 저장소 foreground production wiring | background 실행, Reviewer 이후 단계와 사용자 실제 사용 |
| Worktree 준비 — #58~59 | durable preparation 상태와 CAS, exact Git identity 검사, 안전한 PreparationService | 실제 multi-step resume 범위 확대 |
| 검증 gate — #61 | 승인된 명령 실행, candidate SHA와 전후 Git identity 확인, GateEvidence 기록 | fresh Reviewer 호출·ReviewEvidence와 integration |
| GitHub | 기존 v1 client와 v2 Publisher port/queue/state | v2 Issue·Projects·PR·Wiki 실제 adapter |
| 문서·완료 | state와 설계 seam | Documenter, repository docs, final gate/review, PR/merge/Wiki Finalize |
| Monitor | `internal/monitor` aggregate snapshot wire | 실제 Wails 화면·Windows→WSL 연결과 사용자 검증 |

현재 tree에서 Wails/frontend 실행 진입점은 확인하지 못했다. 설계의 “기존 Wails Monitor”는
유지할 기술 방향이지 사용 가능한 UI가 이미 있다는 뜻이 아니다. `work run`은 이제 존재하지만
foreground Builder→candidate→Go gate까지만 production wiring되었으며 전체 Work Item 완료 명령이 아니다.

## 이번 세션에 병합된 작업과 근거

| PR | merge SHA | 결과 |
|---|---|---|
| [#56](https://github.com/Middleages/thread-dock/pull/56) | `83dd9a617debc0c1dd22c9b23f5fe9512ab579e1` | Herdr stderr·Prompt 오류 관찰성과 runtime failure 정착 |
| [#57](https://github.com/Middleages/thread-dock/pull/57) | `afb463aea4d07aa8a93196b18d8c5390551869f1` | `work pause/resume/reconcile` CLI 연결 |
| [#58](https://github.com/Middleages/thread-dock/pull/58) | `95587e1bccd2d3b8dafb859a1caae0689e0918cf` | durable worktree preparation 상태 전이 |
| [#59](https://github.com/Middleages/thread-dock/pull/59) | `31d33152ce8765930369272f1cde7b66a5e899d0` | safe PreparationService와 exact Git inspection |
| [#60](https://github.com/Middleages/thread-dock/pull/60) | `c1ac3dd8c5f0dc04e5f073477170dd7a3ecb28bf` | foreground `work run` production wiring |
| [#61](https://github.com/Middleages/thread-dock/pull/61) | `e4c1be8e0e940dcc55052a4499900ffaa7291188` | authoritative Go verification gate |

real v2 Store + 임시 Git 저장소 + PreparationService + Coordinator + scripted runtime으로 다음을 확인했다.

- 첫 실행은 Builder를 한 번 Launch하고 running에 도달한다.
- 같은 상태의 replay는 runtime을 중복 Launch하지 않는다.
- 중단 경계의 reconcile은 scripted 결과와 정확한 Git candidate를 회수한다.
- 검증 service는 계약에서 승인된 명령만 실행하고 candidate SHA, HEAD와 working tree의 전후 identity를
  확인한 뒤 gate evidence를 기록한다. Builder의 자체 검증 보고는 이 gate를 대신하지 않는다.

이는 production 구성 요소들의 scripted end-to-end 근거다. 실제 Herdr/OpenCode 실행, Reviewer readonly,
GitHub write 또는 Wails 동작의 성공 근거가 아니다.

## 실제 사용을 막는 문제와 보존 상태

[실행/권한 점검 기록](docs/operator/herdr-builder-capability-preflight.md)과
[Herdr 시작 readiness 보고서](docs/operator/herdr-opencode-startup-readiness.md)가 상세 근거다.

- 최초 live Builder는 Launch 1회 후 실패했다. 파일 편집·commit·candidate는 없으며 최초 Prompt의
  정확한 code는 유실됐다.
- 별도 무도구 비교에서 시작 직후 Start→Get→Prompt는 Herdr 0.8.2와 임시 0.9.0 모두
  `agent_prompt_stalled`였다. 별도 단계 Prompt 성공과 함께 관찰했지만 최초 오류 code를 소급 확정하지 않는다.
- 설치된 default 서버는 0.8.2 그대로고 임시 0.9.0 named server는 중지됐다.
  [Herdr #3813](https://github.com/herdrdev/herdr/issues/3813)은 2026-09-09 마지막 확인 기준
  OPEN/comment 없음/`maintainer-needed`다. 업그레이드나 upstream 구현 PR은 별도 권한이다.
- native OpenCode `build`와 저장소별 권한 합성은 확인했지만 실제 도구 거부 동작과 OS credential
  격리는 검증하지 못했다. Herdr exact invocation 종료는 미지원이며 bridge `Terminate`는 명시적 오류다.

원본 시험 root는 `/tmp/threaddock-herdr-live.7YEhfV`다. state는
`state/v2/work/WorkPilot/work.json`, Git worktree는 `worktree`, branch는 `agent/pilot`, remote는 없다.
관찰 전용 helper를 검토 후 한 번 실행해 **revision 5, Work/Task needs_operator,
blocker runtime_unknown**으로 정착했다. GetInfo 2회/Read 1회만 호출했고 Prompt 재전송은 없다.
LaunchRequested=true, candidate 없음, Git HEAD `add7724386f95b3dfa03942d55d8e1909bd694dd`,
`result.txt`는 `pending`, Git clean이다.

이후 이번 세션에서도 원본 invocation/agent를 조회·종료·재승인·재예약·재전송하지 않았다.
helper도 다시 실행하지 않았다. `/tmp`가 사라졌다면 같은 ID를 재생성해 복구인 것처럼 취급하지 않는다.
새 pilot은 원인 대응과 실행 조건을 확인한 뒤 독립 root/ID로 진행한다. 고정 sleep, blind retry,
별도 readiness 감지기나 session/process registry를 ThreadDock에 추가하지 않는다.

## 다음 작은 구현 순서

전체 설계를 한꺼번에 구현하지 않는다. 다음은 우선순위이며 각 단계 착수 시 최신 main과 public
interface를 대조해 하나의 작은 Task packet으로 구체화한다.

1. **Reviewer invocation과 integration.** gate를 통과한 정확한 candidate SHA/diff/수용 조건/gate evidence를
   fresh Reviewer에 전달한다. strict result의 requestId·role·reviewed SHA를 검증해 ReviewEvidence를 기록하고,
   accept된 같은 SHA만 integration한다. block은 같은 Task의 repair budget과 새 Reviewer requestId로 돌린다.
   Reviewer readonly는 실제 runtime 설정으로 강제·검증하며 prompt 문구나 v1 `build` 매핑을 근거로 삼지 않는다.
2. **GitHub Publisher.** 기존 client를 재사용해 v2 Issue·Projects·PR·Wiki adapter를 작은 순서로 연결한다.
   대상/권한/marker를 먼저 확인하고 ambiguous write는 conflict로 남기며 blind retry하지 않는다.
   이전 점검의 Projects scope 부족과 Wiki 비활성 상태는 실제 대상에서 재확인한다.
3. **문서와 완료 흐름.** Integration Summary→Documenter→repository docs 포함 최종 HEAD gate/review→PR→
   사람 merge 관계 확인→필수 Wiki/Issue/Projects receipt를 연결한다. 발행 실패는 코드 재실행 없이
   publication pending으로 보존한다.
4. **최소 Monitor와 실제 한 건 사용.** UI 위치와 실행환경부터 확인하고 목록·상세·blocker·next action만
   연결한다. Windows→WSL 호출을 검증한 뒤, Herdr blocker에 대응 가능한 새 독립 pilot 또는 확인된 runtime으로
   실제 작은 Work Item 한 건을 수행한다.

첫 단일 저장소 흐름 완료 기준은 **요청·승인→실제 구현→Go 검증→독립 리뷰→통합/docs→PR→사람 병합→
필수 문서·업무 기록 완료**를 사용자 명령과 최소 Monitor에서 확인하는 것이다. scripted runtime 성공이나
Builder smoke만으로 충족하지 않는다. 여러 Project/저장소, 두 번째 runtime, DXHub 메뉴·MCP와 공유 실행
제어는 이 흐름 이후다.

## 작업 규칙과 audit gap

- public interface/shared types는 직렬 합의 후 고정한다. 독립 Task만 별도 worktree/branch로 병렬화한다.
- AGENTS의 전체 Task packet과 result 필드를 사용한다. 구현 worker는 전체 트리 합산 최대 3개이고
  fresh reviewer 슬롯을 남긴다. Sol은 계획·문서·Git 통합, Luna는 제품 코드·테스트·수정을 소유한다.
- 역할 배정은 Luna high / Sol medium이었지만 실제 resolved model/effort telemetry는 계속 `unverified`다.
  이 역할 설정을 제품 Execution Profile이나 runtime capability 보장으로 해석하지 않는다.
- TD-WVG는 최초 배정 때 정식 packet/예약 ledger 기록이 누락됐다. 이후 ignored `.superpowers`의 세션
  ledger에서 보정했지만, 최초 절차 누락 자체는 audit gap으로 유지한다.
- TD-WVG 최초 구현의 정확한 `gofmt` 명령은 보존되지 않았다. fix round의 exact `gofmt` 실행과 결과는
  보존됐으며, 후속 검증 성공이 최초 증거 누락을 소급해 없애지는 않는다.
- foreground fix 검증 중 중복 Docker test container가 발견돼 둘 다 중단했다. 이후 검사를 직렬화했고
  focused 검증과 최종 gate가 통과했다. 중단된 중복 실행을 성공 근거로 세지 않는다.
- 과거 일부 foundation/ingestion 테스트의 사전 RED 누락과 shared-state 일부 matrix assertion 정밀도는
  audit gap이다. 해당 영역을 다시 변경할 때만 필요한 회귀 검사를 보강한다.
- Task별 focused 검증만 수행하고 같은 SHA·command·환경 증거를 중복 실행하지 않는다. 통합 code PR의
  마지막 `make check`만 전체 gate로 한 번 실행한다. docs-only는 링크·구문 검증으로 구분한다.
- PR·Issue 제목/본문은 한국어다. 승인된 push 범위는 유지하되 main 병합은 사람이 한다.
- v1 상태 기계를 복제하거나 예전 Work를 v2로 자동 재실행하지 않는다.

## 검증 기록

PR #56의 최종 제품 SHA `df7960f009efc7e6410a5d4666b885f33c9b9818`에서 Docker Go 1.27
focused test/vet와 아래 전체 gate가 통과했다. 이후 #57~#61은 각 Task의 focused 검증·fresh task review를
거쳤고, 최신 code 통합 결과도 직렬화한 같은 전체 gate에서 exit 0을 확인했다. 동일 SHA·command·환경의
검사를 다시 실행하지 않는다.

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 make check
```

이 handoff 갱신은 docs-only다. 로컬 Markdown 링크 존재 검사와 `git diff --check`만 수행하며
Go suite나 `make check`를 다시 실행하지 않는다.
