# Codex 실제 모델 행동 추가 검증

## 범위

OpenCode 비교 후, 기존 Codex root 시험에서 부족했던 자동 독립 계획 검토와 이미지 입력을 추가로 확인했다. Codex CLI 0.163.0, 기존 사용자 config/auth와 global natural-entry 지침을 사용했다. 설정상 기본 모델은 `gpt-6-astra`, effort는 `medium`이며 모델/provider/권한을 override하지 않았다. 아래 CLI event에는 실제 backend 모델 identity가 노출되지 않아 설정과 runtime 인증은 구분한다.

기존 일반 읽기·작은 오타·여러 작업 우선순위 검증은 [자연어 진입 검증](2026-10-16-natural-entry-verification.md)의 관찰을 재사용했다. 이번에 다시 실행한 것으로 표시하지 않는다.

`/tmp/threaddock-codex-behavior.XnLnZi`의 격리 Git fixture에서 `codex exec --ephemeral --json -C <fixture>`를 실행했다. 입력·출력 artifact와 원시 events/stderr를 보존했다. 현재 pane을 Herdr에 잘못 보고하지 않도록 시험 프로세스에서만 HERDR_ENV/SOCKET_PATH/PANE_ID/WORKSPACE_ID/TAB_ID를 제외했다. config 무시나 trust 우회는 사용하지 않았다.

## 새 시험 결과

| 시험 | 실제 관찰 | 판정 |
|---|---|---|
| 배정된 v1→v2 migration 계획 | OpenCode와 byte-identical brief 및 같은 한국어 요청. `develop-feature`, 공통 개발 기준, 별도 `superpowers:using-superpowers`와 `writing-plans`를 읽었다. `grill-plan`/독립 subagent 호출 없이 초안 종료 | feature 진입과 planning-only 범위 준수 PASS. 중요한 계획의 자동 독립 검토는 미수행 |
| 동일 PNG 이미지 | OpenCode와 같은 이미지와 한국어 요청을 `-i`로 첨부. 제목 `실행 관찰`, 연한 노란색/주황 테두리, `09:41`, `현재 상태 다시 확인`을 정확히 반환. 도구 호출 없음 | 실제 이미지 입력 확인 PASS. 브라우저 interaction/종합 UX 판정까지 검증한 것은 아님 |

Codex 계획은 기존 `appearance`와의 충돌, 실제 호출부/저장 helper 미제공, 입력/미지원 version 정책의 미정 상태를 구분했다. 원자적 rename이라는 brief 진술만으로 실패 처리까지 검증됐다고 주장하지 않았다. OpenCode 최초 계획의 미래 version 확정/미결정 혼재보다 명확했지만, 이는 단일 fixture 관찰이며 모델 전체 성능 우위의 증거로 확대하지 않는다.

Codex는 마지막에 실제 저장소와 입력·충돌 정책이 구현 전 확정돼야 한다고 명시했다. 그러므로 구현-ready를 거짓 주장한 사례는 아니다. 다만 현 workflow가 요구하는 중요한 migration 계획의 독립 검토는 실행하지 않았고, 그 누락을 명시적으로 설명하지도 않았다. 앞선 자동 feature 진입 PASS를 전체 검토 절차 PASS로 확대하면 안 된다.

| case | thread | run usage |
|---|---|---|
| feature | `01a144f0-9ff0-7391-b103-c1ae6e631c80` | input 86,127 / cached 54,144 / output 1,387 |
| vision | `01a144f0-9fd6-71b1-921c-4597718041e1` | input 27,597 / cached 7,168 / output 62 |

이는 실행 전체 usage이며 단일 요청 크기나 실제 결제액이 아니다. 두 run은 exit 0이며 stderr는 stdin 안내뿐이었다. feature brief와 이미지가 원본과 동일함을 확인했다. 모델·지침·실사용 파일은 변경하지 않았다.

## 명시적 위임 대조

생성된 Codex 계획을 별도 fixture의 `submitted-plan.md`로 전달해, 원 brief와 함께 독립 검토자 한 명에게 구현 전 계획 검토를 맡기도록 새 root 세션에 요청했다. 첫 대조의 요청은 일본어로 전달됐고 결과는 한국어로 요청했다. 한국어 자동 진입 시험과 동일한 프롬프트라고 간주하지 않는다. 독립 검토 결과를 반환했지만 CLI JSONL에는 wait만 있었고 ephemeral 종료 후 로컬 thread/spawn 기록도 없어 그 응답만으로 독립 실행을 PASS 처리하지 않았다.

이를 보완해 별도 `delegation-recorded` fixture에서 **한국어로 동일 목적의 명시적 요청**을 다시 실행했다. 이 한 번의 재실행에서는 `--ephemeral`만 제외하고 기존 설정은 유지했다. read-only SQLite 조회를 해당 시험의 ID로만 한정해 실제 parent→child 관계와 역할을 확인했다.

- parent: `01a144f6-6d63-7333-96f3-b408d878380d`
- child: `01a144f7-246f-7b60-8c5a-05361c632f28`, role `td_reviewer`
- 양쪽 runtime metadata: `model=gpt-6-astra`, `reasoning_effort=medium`
- evidence: `/home/appuser/.codex/state_5.sqlite`의 해당 `threads` 및 `thread_spawn_edges` 행. DB를 수정하거나 다른 업무의 기록을 읽지 않았다.

이는 harness가 기록한 실행 metadata 확인이며 backend 가중치의 별도 인증은 아니다. 부모 실행은 exit 0으로 끝났고, 독립 검토 결과를 회수해 키 충돌·입력 유효성·출처 링크의 보완점을 요약했다. 구현 전 계획에 아직 없는 테스트나 화면 증거를 요구하지 않았다. 명시적 native 위임/회수는 PASS지만 이 대조를 자동 검토 누락의 성공으로 대체하지 않는다. spawn edge의 `open`은 세션 관계 기록이며 시험 세션을 삭제하거나 종료 조작하지 않았다.

## 결론

현재 Codex 설정에서 실제 이미지 읽기와 명시적 독립 위임은 확인됐다. 자동 feature 진입 역시 작동했지만, 같은 migration fixture에서 중요한 계획의 독립 검토는 자동 실행되지 않았다. 따라서 자연어 진입 성공과 필수 후속 절차의 준수를 별도로 평가해야 한다. OpenCode와 모델·harness가 달라 단일 시험으로 일반적 품질 순위를 정하지 않는다.

## 한계

원시 로그는 임시 경로에 있어 환경 정리 시 소실될 수 있다. 이번 작업은 검증이며 workflow/모델/권한 수정, GitHub 게시·Herdr 제어, 실사용 프로젝트 변경은 수행하지 않았다. 이미지 읽기 성공은 독립 UI 검토 전체나 실제 Wails 앱의 검증 성공이 아니다.
