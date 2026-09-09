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
| 전용 Builder profile | 미구성 | 사용자 opencode.json에 agent 항목 없음; agent list에 전용 profile 없음 |
| 명시적 권한 정책 | 미확인 | 사용자 opencode.json에 permission 항목 없음. 기본 권한의 실제 효과는 검증하지 않음 |
| 사용자 설정의 MCP | 등록 없음 | 사용자 opencode.json의 mcp 목록 비어 있음. 다른 설정 계층이나 Herdr server 환경까지 격리됐다는 의미는 아님 |
| exact invocation 취소 | 미지원 처리 유지 | bridge Terminate는 명시적 오류. ESC/idle을 종료 확인으로 대체하지 않음 |

자격증명 값·설정 원문·사용자 transcript는 기록하지 않았다. live agent 시작, prompt 전송, 사용자 설정 변경은 수행하지 않았다.

## 다음 pilot 조건

실제 호출에는 OpenCode native Builder profile과 그것이 사용하는 권한 정책을 먼저 고정한다. 기본 build profile을 안전성이 검증된 전용 profile로 간주하지 않는다. 기존 검증된 profile이 있으면 이를 재사용하고, 없으면 별도 테스트용 설정을 검토한다.

시험 업무는 전용 임시 Git 저장소의 파일 하나 변경·commit·현재 requestId의 marked Evidence 반환으로 제한한다. GitHub 발행은 포함하지 않는다. bridge에 논리 profile/native agent/fingerprint를 바인딩하고, 실제 Coordinator→Herdr→Artifact→Git 검사→candidate_ready 결과를 확인한다. native profile의 파일/명령/MCP 권한과 Herdr server에서 자식에게 전달되는 환경을 실제 호출 전에 점검한다.

실행 중 취소가 필요하면 현재 bridge가 자동 종료를 확인할 수 없다는 한계를 유지하며, 결과가 모호할 때 새 invocation을 시작하지 않는다. 종료 확인 기능을 대신할 별도 session/process 관리자는 만들지 않는다.

현재 결론: 코드·CLI·실행 문맥은 준비됐다. 권한을 확인한 Builder profile이 정해지기 전에는 live 실행 준비 완료로 표시하지 않는다.
