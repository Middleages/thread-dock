# Herdr Builder 실제 실행 준비 점검

확인일: 2026-09-09. 기준 main: `70e2e677a0d792d22392598c32906690622eba37` ([PR #55](https://github.com/Middleages/thread-dock/pull/55) 병합).

## 확인 결과

| 항목 | 결과 | 근거 |
|---|---|---|
| Builder bridge 코드 | 병합 완료 | PR #55의 HEAD eefab89에서 Docker Go 1.27 make check 통과 |
| Herdr 실행 문맥 | 확인 | HERDR_ENV=1, pane current 조회 성공 |
| Herdr 설정 문법 | 확인 | herdr config check → config: ok |
| Herdr 버전 | 0.8.2 | herdr --version |
| OpenCode 버전 | 1.18.27 | opencode --version |
| native agent 목록 | 기본 제공 profile만 확인 | build, compaction, explore, general, plan, summary, title |
| Builder native profile | 기존 `build` 재사용 | 현재 agent list에 build (primary)가 있고, 2026-09-04 커밋 2fe28b5에서 전용 pilot agent를 내장 build로 전환함 |
| 명시적 권한 정책 | 미확인 | 사용자 opencode.json에 permission 항목 없음. 기본 권한의 실제 효과는 검증하지 않음 |
| 사용자 설정의 MCP | 등록 없음 | 사용자 opencode.json의 mcp 목록 비어 있음. 다른 설정 계층이나 Herdr server 환경까지 격리됐다는 의미는 아님 |
| exact invocation 취소 | 미지원 처리 유지 | bridge Terminate는 명시적 오류. ESC/idle을 종료 확인으로 대체하지 않음 |

최초 사전 점검에서는 자격증명 값·설정 원문·사용자 transcript를 기록하지 않았으며 live agent 시작과 prompt 전송을 수행하지 않았다. 이후 한 번 수행한 실제 시험 결과는 아래에 구분해 기록한다. 사용자 전역 설정은 변경하지 않았다.

## 다음 pilot 조건

Builder의 native agent는 기존 `build`를 명시적으로 바인딩한다. 전용 custom agent의 부재는 blocker가 아니며 새 profile을 만들 이유가 아니다. 권한 정책의 실제 적용 확인은 profile 이름 선택과 별개로 남아 있다.

### 기존 설정 회수 근거

- Git 이력의 `2fe28b5`(2026-09-04)는 `pilot-repair-builder`, `pilot-repair-reviewer` 등 custom agent 템플릿을 제거하고 내장 `build`로 전환했다. 해당 커밋은 현재 main의 조상이다.
- 전환 설계 `80284d3`은 custom primary agent가 idle로 감지돼도 prompt receipt가 관찰되지 않는 문제를 기록하고 있다.
- 현재 [v1 운영 참고 문서](opencode-role-agents.md)도 `openCodeAgents.builder=build` 매핑을 설명한다. v1 Reviewer의 build 매핑까지 v2에 적용하는 것은 이번 범위가 아니다.
- 일부 예전 fault-pilot worktree에 `.opencode/agents/pilot-repair-builder.md`가 남아 있으나 과거 시험 산출물이다. 현재 profile로 복사하거나 활성화하지 않는다.

초기 점검은 사용자 설정의 전용 agent 유무만 확인해 이미 채택한 내장 build 경로를 놓쳤다. 이 문서는 그 판단을 정정한다. 기존 pilot의 실행 근거는 재사용 방향의 근거이며 v2 bridge의 live 권한·결과 검증을 대신하지 않는다.

시험 업무는 전용 임시 Git 저장소의 파일 하나 변경·commit·현재 requestId의 marked Evidence 반환으로 제한한다. GitHub 발행은 포함하지 않는다. bridge에 논리 profile/native agent/fingerprint를 바인딩하고, 실제 Coordinator→Herdr→Artifact→Git 검사→candidate_ready 결과를 확인한다. native profile의 파일/명령/MCP 권한과 Herdr server에서 자식에게 전달되는 환경을 실제 호출 전에 점검한다.

실행 중 취소가 필요하면 현재 bridge가 자동 종료를 확인할 수 없다는 한계를 유지하며, 결과가 모호할 때 새 invocation을 시작하지 않는다. 종료 확인 기능을 대신할 별도 session/process 관리자는 만들지 않는다.

현재 결론: 코드·CLI·실행 문맥과 Builder native profile 이름은 확인됐다. 다음 점검은 기존 build의 실제 권한 적용과 v2 결과 프로토콜이다.

## 2026-09-09 실제 pilot 결과: 실패

시험 root는 `/tmp/threaddock-herdr-live.7YEhfV`, worktree branch는 `agent/pilot`이다. remote 없는 별도 Git 저장소를 만들고 `result.txt`의 `pending`을 `complete`로 바꾸는 단일 Builder 업무를 실행했다. 결과·state는 원인 확인을 위해 보존한다.

### 권한 준비

기존 native `build`를 유지하고 시험 저장소의 `opencode.json`에서 대부분의 도구를 deny, result.txt 읽기·편집과 지정된 Git 명령만 allow로 설정했다. `opencode debug agent build`로 최종 합성 규칙을 확인했다. MCP 등록 없음, share disabled, autoupdate false도 확인했다. OpenCode 자체 tool-output 디렉터리 허용은 남아 있다. 읽을 수 있는 Herdr 프로세스 환경에서 점검한 GitHub token 변수 이름은 발견되지 않았다. 이는 OS 격리나 실제 도구 거부 동작의 검증을 뜻하지 않는다.

공식 참고: [OpenCode permissions](https://opencode.ai/docs/permissions/), [configuration](https://opencode.ai/docs/config/). 전역 설정을 변경하거나 새 custom agent를 만들지 않았다.

### 실행 전 발견·수정한 결함

실제 Herdr 0.8.2의 없는 agent 조회는 exit 1과 stderr `agent_not_found` JSON을 반환한다. 기존 GetInfo는 stdout만 읽어 bridge 시작 검사가 실패할 수 있었다. 기존 sole-channel helper와 strict stderr decoder를 재사용해 고쳤다. stdout 호환은 유지하고 혼합·malformed·trailing·다른 오류는 generic error로 처리한다. 변경은 cli.go/test 두 파일이며 Luna 구현, fresh Sol review ACCEPT, focused test/vet를 통과했다.

### 실행과 관찰

- 수정된 소스 `00ef52238e286d7a180e0fd97dfc4df89b1e57a6`로 disposable driver를 빌드했다. 새 제품 CLI나 session manager는 추가하지 않았다.
- driver SHA256: `f91873bc5bac3a8155953f7f80f12bc0631cf70d9fe7443311a17dc980b97c9d`.
- 비밀을 제외한 effective model/permission/native agent/버전 fingerprint: `7b2132f61c4236b6d5c51aceea1f953964b9fd6792b01d1f2dd3c975731e4f21`.
- WorkPilot plan/approve/activate/reserve 뒤 Launch 1회. 실제 CLI 호출 횟수는 GetInfo 2, OpenWorktree 1, StartAgent 1, Prompt 1, ReadEvidence 0이다.
- driver는 launch 단계에서 `runtime_error`로 종료했다. Task는 `invocation_reserved`, LaunchRequested=true이며 provider identity와 candidate는 저장되지 않았다. 상위 Work projection은 running이다.
- 후속 읽기 전용 관찰에서 시험 agent는 idle/interactive ready였다. 현재 requestId와 Evidence marker는 관찰되지 않았다. result.txt는 여전히 `pending\n`, worktree는 clean이며 HEAD는 초기 `add7724386f95b3dfa03942d55d8e1909bd694dd`였다.
- prompt는 재전송하지 않았다. 시험 agent/workspace를 자동 종료하거나 다른 invocation을 시작하지 않았다.

### 판정과 다음 진단

Herdr 연결·agent 시작·권한 규칙 합성 확인까지 진행했다. 실제 파일 편집·결과 회수·candidate_ready·행동 수준 권한 적용은 검증하지 못했다. idle 상태만으로 invocation 종료 또는 prompt 미전달을 확정하지 않는다.

당시 bridge/driver가 오류를 정적으로 축약해 최초 Prompt의 정확한 Herdr 오류 코드는 수집되지 않았다. `agent_prompt_stalled`나 인증 문제라고 단정할 근거가 없다. 후속으로 안전한 code 전달과 관찰 정착을 완료했다(아래 절). 재시도 권한이나 종료 증거 없이 같은 packet을 다시 보내지 않는다.

## 2026-09-09 Prompt 전달 비교 진단

최초 pilot invocation은 재전송하지 않았다. Herdr server/client 로그에서 해당 target/request와 연결되는 오류 code를 찾지 못했다. OpenCode DB를 원래 시험 directory로 한정해 조회한 결과 session 0, user message 0, requestId를 포함한 part 0이었다. 이 조회는 본문을 출력하지 않고 개수만 반환했다.

별도 `/tmp/threaddock-prompt-diag.5PLepy/worktree`에서 기존 build에 모든 도구 deny 정책을 적용했다. 파일 변경/commit 과제 없이 고정 문장 응답만 요청했다. 오류 수집기는 stdout/stderr JSON에서 알려진 code만 출력하고 provider message와 본문은 버렸다.

| 비교 | 수행·관찰 | 결과 |
|---|---|---|
| 시작 후 별도 단계로 짧은 Prompt | StartAgent 성공 뒤 별도 CLI 호출로 전송 | exit 0, 약 9.97초. DB에서 user message 1건과 정확한 assistant 응답 1건 확인 |
| 시작 직후 즉시 짧은 Prompt | 같은 스크립트에서 StartAgent → GetInfo → Prompt, 성공한 Start/Get 뒤 즉시 전송 | 약 6.40초 후 exit 1, `agent_prompt_stalled`. DB에 해당 진단 문장이 포함된 part 0건 |

두 번째 agent는 실패 뒤에도 idle/interactive_ready로 표시됐다. 설치된 Herdr help는 `prompt --wait`가 turn 자체를 추적하지 않으며, non-working 상태에서 전송한 뒤 5초 안에 상태 변화를 관찰하지 못하면 `agent_prompt_stalled`를 반환한다고 설명한다.

판정: 동일 build와 무도구 정책에서 전달 성공 사례가 있으므로 일반적인 인증/권한 부재만으로 설명되지 않는다. 시작 완료 직후의 입력 준비와 Herdr readiness 판정 사이의 경합이 의심된다. 이는 비교 관찰에 따른 추론이며 Herdr/OpenCode 내부 원인을 소스 수준에서 확정한 것은 아니다. 최초 pilot의 유실된 오류 코드도 소급 확정하지 않는다.

다음 수정 검토 대상은 Herdr의 agent-start 준비 완료 판정과 실제 입력 전달 시점이다. ThreadDock의 기존 invocation 재전송이나 자동 재시도는 수행하지 않았다. 이번 비교는 Prompt 전달 진단이며 Builder candidate_ready나 exact 취소 기능의 성공 검증은 아니다.

비교용 workspace는 경로를 대조한 뒤 닫았다. 이는 임시 진단 프로세스 정리이며 bridge의 Terminate 성공 근거로 사용하지 않는다. 원래 Builder pilot workspace/state와 두 진단용 Git 저장소는 보존했다.

후속으로 checksum 검증한 임시 Herdr 0.9.0 named server에서도 같은 startup 경합을 재현했고, 두 tagged source의 준비 판정을 대조했다. 상세 근거와 upstream 수정 방향은 [시작 readiness 보고서](herdr-opencode-startup-readiness.md)에 기록했다. 설치된 default 서버는 0.8.2 그대로이며, 업그레이드만으로 해결됐다고 주장하지 않는다.

재현 증상은 [Herdr #3813](https://github.com/herdrdev/herdr/issues/3813)으로 보고했다. 기존 실행 상태는 보존하며, upstream 응답과 입력 준비 확인 방법이 확정되기 전까지 실패한 Builder packet을 자동 재전송하지 않는다.

## 보존된 invocation의 관찰 정착

후속 진단에서 원본 WorkPilot에 `Coordinator.Reconcile`을 한 번 적용했다. 임시 helper의 Herdr runner는 정확한 `agent get`과 `agent read`만 허용하며 Activate/Submit/Prompt 경로가 없다. 실행 전 Sol 검토를 받았다.

- 실행 바이너리 SHA256: `037a2d74ea48bc476f707e5508848d66dd6d29237f6f38a7e03e7d03512dd843`.
- 실제 provider 호출: GetInfo 2회, Read 1회, 금지된 호출 시도 0회.
- 정착 결과: revision 5, Work/Task 모두 needs_operator, blocker kind runtime_unknown.
- LaunchRequested=true와 invocation identity는 보존했고 candidate는 없다. Git HEAD는 초기 add7724386f95b3dfa03942d55d8e1909bd694dd 그대로이며 worktree는 clean이다.

이는 조회 결과를 기존 Store transition으로 기록한 것이며 최초 Prompt 오류 코드를 소급 복원하거나 실행 성공을 인정한 것이 아니다. 원본 packet은 재전송하지 않았다.

## Prompt 오류 관찰성 수정 완료

제품 SHA `df7960f009efc7e6410a5d4666b885f33c9b9818`에서 CLI는 sole-channel strict envelope의
허용된 오류 code만 `PromptError.Code()`로 전달한다. provider message/body는 보존하지 않는다.
bridge caller는 `ErrRuntimePrompt`와 typed code를 함께 확인할 수 있다. 실제 Launch 호출 뒤의 실패는
기존 Store transition을 통해 `needs_operator`/`runtime_unknown`으로 정착하며 cancellation 전용 처리는 유지한다.

실제 Store/public Coordinator/scripted Herdr CLI 통합 테스트로 caller 오류, durable blocker,
waiter 완료, secret 비영속화와 replay no-call을 검증했다. Luna 수정 후 fresh Sol은 최종 SHA를 ACCEPT했다.
Docker Go 1.27 focused test/vet와 최종 전체 `make check`가 통과했다.
이는 향후 오류 처리의 코드 검증이다. 원본 오류 복원, startup readiness 해결 또는 live Builder 성공을 뜻하지 않는다.
새 세션의 구현 순서는 [HANDOFF](../../HANDOFF.md)를 따른다.
