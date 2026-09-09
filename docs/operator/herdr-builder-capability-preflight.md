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

자격증명 값·설정 원문·사용자 transcript는 기록하지 않았다. live agent 시작, prompt 전송, 사용자 설정 변경은 수행하지 않았다.

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
