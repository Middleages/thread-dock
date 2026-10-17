# OpenCode 자연어 진입과 모델 행동 검증

## 범위

설치·역할 발견 확인에 그쳤던 OpenCode에서 실제 모델 요청을 실행했다. OpenCode 1.19.0의 기존 설정을 사용했으며 model/provider/권한/default agent/hook을 변경하지 않았다. 실제 해석된 기본 모델은 `opencode/big-pickle`, 기본 agent는 `build`다. 이미지 시험의 session export에서도 assistant의 providerID/modelID/agent가 이를 확인했다. 그 별칭 뒤의 실제 제공 모델까지 확인한 것은 아니다.

스킬 이름이나 `--agent`를 지정하지 않은 새 세션 5개를 `/tmp/threaddock-opencode-behavior.KN5mJM`의 격리 fixture에서 실행했다. `opencode run --format json --dir <fixture>`로 실제 도구 호출과 최종 답변을 기록했다. 기존 설정과 plugin을 로드하고 `--pure`, 권한 완화, 모델 대체는 사용하지 않았다. 단, 자식 시험 프로세스에서만 HERDR_ENV/SOCKET_PATH/PANE_ID/WORKSPACE_ID/TAB_ID를 제외했다. 설치된 Herdr plugin이 현재 작업 pane의 세션·상태를 덮어쓰는 일을 막기 위한 격리이며, Herdr 연동 검증은 범위 밖이다.

## 결과

| 시험 | 실제 관찰 | 판정 |
|---|---|---|
| 일반 읽기 질문 | README를 읽고 두 문장 설명. read/glob/ls만 수행 | PASS: 불필요한 수정·계획·세션 없음 |
| 명백한 오타 | README의 `teh`만 `the`로 edit하고 grep 확인. 정확한 예상 파일 내용과 일치 | PASS: 작은 작업의 경량 처리 |
| 세 작업의 우선순위 | 정확성 수정 A, A에 의존하는 B, 독립 C를 구분. 외부 행동·파일 변경 없음. `coordinate-work` 호출/본문 읽기 없음 | 결과·범위 준수 PASS, 명시된 자동 스킬 진입은 확인되지 않음 |
| 배정된 migration 계획 | `develop-feature` skill과 공통 개발 기준을 스스로 읽음. planning-only 유지. 실제 `grill-plan`이나 native task 호출 없이 계획 종료 | 자동 feature 진입 PASS, 중요한 계획의 독립 검토 누락 |
| 실제 PNG 첨부 | session export에 `image/png`, `screen.png` 파일 part 확인. 모델은 `실제 이미지를 볼 수 없습니다.`라고 답하고 다른 파일 탐색·추측 없음 | 비전 능력/전달 경로 미통과, 한계 보고 PASS |

마이그레이션 계획은 본문에서 `version >= 2`를 무쓰기 처리로 확정하면서 마지막에는 `version > 2` 정책을 미결정으로 남겼다. 오류 타입/형식 같은 구현 선택까지 제품 선택으로 묶었으며, v1에 이미 `appearance`가 있을 때 unknown-key 보존과 충돌하는 경우도 다루지 않았다. 실제 코드 결함을 재현한 것이 아니라 생성된 계획의 일관성·누락을 관찰한 결과다.

이미지 시험은 기존 합성 UI fixture를 첨부했다. 원래 화면의 제목, 강조 영역 색, 마지막 성공 시각, 버튼 문구를 식별하도록 요청했지만 읽지 못했다. 이어 `opencode models opencode --verbose`의 현재 `big-pickle` 항목에서 `capabilities.attachment=false`, `capabilities.input.image=false`를 확인했다. 현재 모델 경로는 이미지 입력 지원으로 등록되지 않았으며 실제 응답도 이에 부합한다. provider 별칭 뒤의 모델 자체 능력을 별도로 인증한 것은 아니고, 다른 모델로 임의 변경하지 않았다.

## 명시적 위임 대조

자동 계획 검토가 누락된 원래 feature 세션에 후속 요청으로 독립 검토자 한 명을 사용하도록 요청했다. 이 대조 결과는 자동 실행과 별도로 기록하며, 원래 자동 누락을 통과로 바꾸지 않는다. session export에서 실제 native `task` 호출과 `subagent_type=td_reviewer`, 별도 child session ID `ses_ebb18dc67ffeC5NfwEnVvwzmrr`, 기존 `opencode/big-pickle` 모델을 확인했다.

호출은 정상 완료됐다. 독립 reviewer가 plan-review 모드로 기존 appearance/theme 충돌, version 정책과 테스트 기대값, 임시파일 정리 및 검증 경로 보완을 반환했고 부모가 이를 요약했다. 아직 없는 구현 SHA·스크린샷·테스트 성공을 요구하지 않았고 실제 파일 수정도 하지 않았다. reviewer의 표현은 `수용 (소규모 보완 후 확정 가능)`이었으나 충돌 정책 등 결정이 남으므로 이 보고서는 구현-ready로 승격하지 않는다. native 위임은 명시 요청 시 동작했고, 자동 실행 누락과 기능 가용성을 구분했다.

## 관찰 경계

원시 이벤트는 각 case의 `*-events.jsonl`, stderr는 `*-stderr.log`에 있다. 최초 5개 run은 CLI exit 0, error event 0, stderr 비어 있음이었다. 도구 이벤트는 fixture 읽기, 오타 한 단어 수정, 설치된 feature skill 읽기에 한정됐다. 실행 성공(exit 0)이 workflow 준수를 의미하지 않음을 위 판정에서 구분했다.

전역 설정·스킬·역할, 실사용 저장소와 기존 작업 세션은 수정하지 않았다. OpenCode의 새 시험 세션 기록과 fixture는 보존한다. 실제 GitHub/Herdr/Wails 통합, 브라우저 interaction, 다른 provider/model의 행동은 미검증이다. 한 번씩의 합성 시험이며 일반적인 성공률이나 다른 모델의 품질을 추정하지 않는다.

최종 결론: 기본 작업·작업 범위 준수와 명시적 독립 위임은 확인됐다. 스킬 자동 선택과 필수 검토의 완전한 실행은 부분 성공에 그쳤고, 현재 기본 모델 경로는 비전 검토를 제공하지 않는다. 이번 작업은 검증이며 정책·모델 설정을 수정하는 후속 작업은 수행하지 않았다.
