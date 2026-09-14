# OMO 없는 ThreadDock 전문가 팀

사용자가 역할·도구 검토와 단순성 기준에 합의한 뒤 실제 추가를 요청했다. 이 문서는 그 합의의 1차 구현 범위를 고정한다.

## 역할과 원본

GitHub는 업무 원본, Herdr는 top-level lifecycle, Feature Leader는 범위·분배·결과 취합·게시를 소유한다. Coordinator와 Feature Leader는 그대로 유지한다.

네 leaf 역할을 Codex TOML/OpenCode Markdown으로 제공한다.

| 이름 | 공용 Skill | 책임 |
|---|---|---|
| td_explorer | explore-codebase (신규 helper) | 질문에 필요한 코드 흐름·재사용 지점·영향 테스트를 찾아 반환 |
| td_docs_editor | write-project-docs (신규 helper) | 독자·목적에 맞춰 문서를 구조화하고 의미·근거를 보존 |
| td_implementer | tdd-task | 직접적인 구현·focused tests·진단 가능한 로그 |
| td_reviewer | review-change | fresh exact-SHA 검토, 실제 결함과 불필요한 복잡성 구분 |

leaf는 Herdr pane/locator에 추가하지 않고 추가 위임이나 직접 게시를 하지 않는다. 탐색·문서 편집은 필요한 경우만 사용하며 고정 네 단계 pipeline을 만들지 않는다. 별도 verifier는 필수가 아니다.

## 합의한 구현 품질

기계가 구할 값이나 이미 정해진 공용 값은 사용자에게 다시 묻지 않는다. 하나의 사실을 여러 설정에서 맞추게 하지 않는다. 새 계층·설정·검사는 구체적인 문제를 해결할 때만 추가하고, 이미 책임 지점에서 보장되는 검사를 중복하지 않는다. 단순한 식별자 차이를 실제 호환성 문제처럼 차단하지 않는다. 필요한 위험 경계의 검사를 없애라는 뜻은 아니다.

로그는 작업·대상·실패 지점·확인된 원인을 좁힐 맥락을 보존한다. 표준화가 진단 정보를 없애면 개선으로 평가하지 않는다. 문서는 목적·결정·근거·다음 행동을 독자가 찾을 수 있게 쓰되 고정 양식/장황한 checklist를 강제하지 않는다.

## 구현 경계

- canonical 여섯 Skill 유지. helper는 기존 herdr-local에 explore-codebase/write-project-docs만 추가한다.
- tdd-task는 기존 RED/GREEN/focused 규칙을 자급적으로 제공한다. 같은 내용을 되풀이하며 외부 Superpowers 설치를 필수로 요구하는 의존성은 제거한다.
- 도구별 Agent 파일은 역할과 Skill 진입점 중심으로 짧게 유지한다. 새 leaf에는 model/effort를 고정하지 않고 대상 환경 설정을 따른다. 기존 Codex primary 모델 값은 이번에 바꾸지 않는다.
- OpenCode leaf는 mode: subagent. task: deny로 leaf 위임 제한, explorer/reviewer는 edit: deny. 허용 권한을 자동 확대하지 않는다. Codex의 권한/인증/sandbox는 상속한다. 읽기 전용 역할이 OS 격리를 보장한다고 설명하지 않는다.
- Feature Leader는 지원되는 named leaf를 선택한다. 해당 named 역할이 없는 harness에서는 허용된 native agent에 동일 Skill/범위를 전달할 수 있지만 task 권한 거부를 우회하지 않는다.
- publish-work는 실제 Project 옵션이 없으면 추측하지 않고 pending으로 남긴다. 쓰기 결과가 불명확할 때만 대상 read-back 후 재시도한다.
- OMO 코드/프롬프트 복사, plugin 설치, 모델 fallback, 새 엔진/bridge, 전역 설치, LSP/Playwright/ast-grep 설치는 이번 범위가 아니다. 기본 read/grep/glob/Git/gh/기존 test runner를 사용한다.

## 검증과 전달

문서·설정 변경에 맞춰 format/name/Skill 참조·로컬 링크, make template-check, 격리된 OpenCode --pure 역할 인식을 확인한다. parser 성공을 행동 성공으로 확대하지 않는다. 작은 탐색/문서편집/복잡성·로그 판단 fixture로 실제 동작을 한정 확인한다. 기존 사용자 worktree/dirty 실험을 보존한다. main은 사람이 병합한다.

Codex의 지원 필드 근거: https://learn.chatgpt.com/docs/agent-configuration/subagents . 모델·effort는 optional, name/description/developer_instructions는 required다.
