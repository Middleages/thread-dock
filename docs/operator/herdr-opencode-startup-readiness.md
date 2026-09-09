# Herdr/OpenCode 시작 직후 Prompt 누락 재현 보고서

상태: 2026-09-09 실제 비교 진단 및 tagged source 검토. [Herdr upstream #3813](https://github.com/herdrdev/herdr/issues/3813)에 재현 버그를 등록했다. 공개 보고에는 upstream 템플릿에 따라 관찰 결과·재현 절차·환경만 담고, 아래 소스 분석과 수정 방향은 로컬 조사 기록으로 유지했다.

## 문제

`herdr agent start ... --kind opencode -- --agent build`가 성공하고 `agent get`이 idle/interactive_ready를 반환해도, 즉시 이어지는 Prompt가 `agent_prompt_stalled`로 실패할 수 있다. 실패한 진단 요청은 OpenCode DB에도 메시지로 기록되지 않았다. 시간이 지난 뒤 별도의 짧은 요청은 실제 응답까지 확인됐다.

환경: Linux/WSL, OpenCode 1.18.27, native build. 시험 저장소의 build permission은 모든 도구 deny이며 파일 편집 과제는 없다. 기존 사용자 설정과 실행 중 default Herdr 서버는 변경하지 않았다.

## 재현 결과

| 서버 | 순서 | 결과 |
|---|---|---|
| 설치된 0.8.2 | StartAgent → GetInfo → 즉시 Prompt | 약 6.40초 뒤 exit 1 / agent_prompt_stalled, 해당 요청의 DB part 0건 |
| 설치된 0.8.2 | 시작 후 별도 단계에서 Prompt | 약 9.97초 뒤 exit 0, 정확한 assistant 응답 1건 |
| 별도 named session 0.9.0 | StartAgent(3.639초) → GetInfo(3ms) → 즉시 Prompt | 5.312초 뒤 exit 1 / agent_prompt_stalled, 해당 요청의 DB part 0건 |
| 같은 진단용 0.9.0 | 이후 다른 무도구 Prompt | 3.609초 뒤 exit 0, 정확한 assistant 응답 1건 |

각 Prompt는 서로 다른 진단 표식을 가진 짧은 응답 요청이며 `--wait --timeout 20000`으로 실행했다. 실패한 실제 Builder invocation은 재전송하지 않았다. 오류 수집은 알려진 JSON code만 출력하며 message/body/transcript는 기록하지 않았다. 위 표는 소규모 재현 결과이며 모든 실행에서의 실패율을 측정한 것은 아니다.

0.9.0은 [공식 릴리스](https://github.com/herdrdev/herdr/releases/tag/v0.9.0)의 Linux x86_64 바이너리다. SHA256 `4fa1a01158dd8043da92d31b270780b0dcc10603038d9b61cac4d81ab63fb71f`가 릴리스 asset digest와 일치했다. 기존 설치 파일을 교체하지 않고 임시 경로와 `td-prompt090` named server를 사용했다.

## Tagged source 근거

- [3초 settle 상수](https://github.com/herdrdev/herdr/blob/v0.9.0/src/app/agents.rs#L8): startup의 ready_after를 결정한다.
- [Pending → Active 판정](https://github.com/herdrdev/herdr/blob/v0.9.0/src/terminal/state.rs#L1994): ready_after 경과, 감지된 agent kind 일치, idle 상태를 확인한다. 실제 prompt 입력 수락을 확인하는 조건은 없다.
- [CLI의 준비 대기](https://github.com/herdrdev/herdr/blob/v0.9.0/src/cli/agent.rs#L607): idle/done과 interactive_ready를 확인한다.
- [OpenCode integration](https://github.com/herdrdev/herdr/blob/v0.9.0/src/integration/assets/opencode/herdr-agent-state.js#L169): session 상태를 lifecycle로 전달하며 독립적인 input-ready 신호는 없다.
- [Prompt 진입 검사](https://github.com/herdrdev/herdr/blob/v0.9.0/src/app/api/agents.rs#L138): agent/foreground/launch_pending을 검사한다.

검토한 tag commit은 0.8.2 `9eb521456ac0d19d3ab3d9d7cea3cca10baa8a4c`, 0.9.0 `b99002ac99b09e00b4ca692436cb15a6b0d676f1`이다. 두 버전 모두 같은 startup 판정 구조다. 직접 protocol AgentStart는 Pending 상태에서 응답하며, CLI가 별도로 위 준비 대기를 수행한다. 이번 재현은 그 대기를 포함하는 CLI를 사용했다.

[PR #3506](https://github.com/herdrdev/herdr/pull/3506)은 본문과 Enter 전달 순서 및 완료 확인을 개선하지만, 이번 시작 직후 재현은 0.9.0에서도 남는다. 업그레이드만으로 해결된다고 보고하지 않는다.

## 결론과 수정 방향

Herdr의 interactive_ready가 OpenCode 입력 가능 상태를 보장하지 못하는 false-positive readiness 문제다. 실제 누락되는 입력의 내부 처리 지점까지 추적한 것은 아니지만, 현재 공개 readiness 조건이 약하며 동일 조건을 추가 polling해도 강화되지 않는다는 점은 확인했다. `agent wait` 역시 lifecycle만 확인한다.

Herdr/OpenCode integration에서 입력 준비 완료의 양의 신호 또는 prompt 수락 acknowledgement를 연결하는 방향을 검토해야 한다. ThreadDock에는 별도 화면 감지기·session manager·고정 sleep·자동 prompt 재전송을 추가하지 않았다.

사용자 승인으로 upstream 재현 이슈를 등록했다. Herdr 구현 PR 제출이나 사용자 주 서버 교체는 수행하지 않았다.

비교용 named server는 종료를 확인했다. 기존 default 서버는 0.8.2로 계속 실행 중이며, 전후 OpenCode integration 파일 SHA256도 동일했다.
