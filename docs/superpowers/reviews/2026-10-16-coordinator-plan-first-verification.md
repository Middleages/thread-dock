# Coordinator 전체계획 선행 지침 검증

## 변경과 범위

Coordinator가 기능 분배 전에 전체계획을 먼저 작성·검토하도록 기존 `coordinate-work`와 역할 지침을 보강했다. Feature Leader는 해당 전체계획의 결정/계약을 받아 파일·Task·테스트를 상세화한다. 새 planner 역할·Skill·엔진은 추가하지 않았다.

- 전체계획: 목표/비목표, 구조 근거와 선택, 기능 경계/공유 계약, 의존성/순서/직렬화, 기능별 수용 근거, 미결정.
- 중요한 계획: 고정 revision의 작성자 → fresh adversary → 다른 neutral 판정 후 해당 결정 의존 기능을 배정한다. 기존 검토본 재사용과 작은 작업의 짧은 계획을 허용하며 같은 검토를 반복하지 않는다.
- 인계/환류: plan reference/revision과 해당 결정/계약을 packet에 넣는다. 다른 기능 경계·계약·의존성·전체 가정 변경은 Coordinator로 돌려 영향분만 재계획한다.
- 권한/연속성: 계획만 요청하면 구현·새 기능 세션·GitHub 쓰기 허가로 확대하지 않는다. 기존 세션·초안·승인된 독립 업무는 유지하고, 승인된 구현을 계획 검토 후 재승인받지 않는다.

taskId `coordinator-plan-first`, base `f080fa999c0aafe91d06f58083cc15ba502a7d44`, branch `agent/coordinator-plan-first`, worktree `.worktrees/coordinator-plan-first`다. Task packet과 원시 평가·백업은 `/tmp/threaddock-plan-first.qeM5by`에 보존했다. 루트 dirty checkout, 제품 코드·테스트, root 개발 역할·모델, 활성 Herdr 세션은 수정하지 않았다. root AGENTS의 기존 quickstart/grill-plan 포인터를 재사용했다.

## 실제 평가와 검사

`skill-creator`, `writing-for-agents`와 `SKILL-MECHANICS`를 사용했다. 행동 평가자에게 후보 Skill과 격리 fixture의 원본 요청만 전달했으며 사전 판정 기준 `oracles.md`는 보여주지 않았다.

| 확인 | 결과 |
|---|---|
| 계획 재사용/작은 경로 | fresh `reuse_plan_eval`이 기존 검토 r7을 재사용해 F1 packet 파일을 작성. reference/revision, 원래 결정/계약·수용조건·의존성·worktree를 보존하고, 재검토 Agent나 재승인 질문 없이 F2를 그대로 둠. live GitHub/Herdr·구현 호출 없음 |
| 중요한 계획/plan-only | `overall_plan_eval`이 sync-plan-r1 작성 → fresh `td_reviewer` adversary → 다른 fresh `td_reviewer` neutral을 실제 실행하고 r2 전체계획/F1 상세계획을 작성. 실제 미확인 source 신호·구 CLI 호환성·미정 wire 계약을 검토해 의존 기능 배정만 보류. 기존 planning-1/F0 유지, 구현·새 기능 세션·외부 쓰기 없음 |
| focused 구조 | `python /tmp/threaddock-plan-first.qeM5by/validate.py` PASS: template/global 6개 Skill frontmatter/creator 검사, reference/link, primary TOML/YAML parse와 모델·effort·mode·permission 보존, 실제 설치본 parity |
| template/diff | `make template-check`, `git diff --check` PASS. 기존 UI 행동 suite와 전체 `make check`는 변경 범위가 아니므로 재실행하지 않음 |

reuse 평가의 첫 검색 명령은 백틱을 shell 치환해 `command not found` 2건을 냈다. 파일/외부 상태 변화는 없었고 고정 문자열 검색으로 정정했다. 이를 workflow 동작 실패나 검증 성공으로 숨기지 않는다. 최종 packet은 `eval-reuse/f1-feature-leader-packet.md`다.

significant 평가의 원문은 `eval-significant/01-original-plan.md`, `02-adversarial-review.md`, `03-neutral-adjudication.md`, `04-final-plan.md`다. adversary/neutral은 각각 `/root/astra_workflow/overall_plan_eval/adversarial_plan_review`, `.../neutral_plan_adjudication`이며 고정 초안을 받아 결과만 반환했다. planning cycle을 재귀적으로 시작하거나 추가 위임하지 않았다. AP-01/02/03/05를 채택하고, 새 `failureReason` 필드를 범위에 넣을 위험은 AP-04 부분 채택으로 다뤘다. 실제 source/consumer 근거와 미위임된 unknown/호환 계약 결정은 남겨 두었고, 이를 핑계로 독립 F0를 멈추지 않았다. 원본의 계획-only 한계를 지킨 결과이며 실제 구현 배정 성공을 뜻하지 않는다.

## WSL 전역 적용과 활성 작업 경계

수정 전 전역 7파일이 main 원본과 byte 일치함을 확인한 뒤 `/tmp/threaddock-plan-first.qeM5by/before`에 백업했다. 공용 Skill 3개와 Codex/OpenCode primary 2개씩에 같은 좁은 지침 delta만 적용했다. 모델·effort·mode·permission metadata와 config·provider·다른 전역 파일은 변경하지 않았다. 파일 적용이 열린 세션의 이미 로드된 지침을 자동 교체했다는 뜻은 아니다.

원래 dirty checkout의 개발용 local 역할과 quickstart에는 이번 entrypoint 문장을 직접 적용하지 않았다. 그 checkout에서 local 역할이 global을 가리면 이번 전체계획 선행 단계가 자동 반영됐다고 보장할 수 없다. 새 branch의 template/quickstart와 현재 WSL global이 이번 변경 대상이며, 사용자가 지정한 기존 작업 세션은 아래 별도 수신 확인 범위로 구분한다.

root의 별도 관찰 전달에 따르면 사용자 승인으로 기존 작업 중 세션에 임시 지침을 보냈고, visible 응답에서 전체 계약·의존성·완료조건의 선행 검토 및 별도 반대/중립 판정, 기존 첫 기능 초안 재사용, 구현·PR·외부 쓰기 미시작을 확인했다. 세션은 중단하지 않고 작업중 상태를 유지했다. 이는 지침 수신 확인이며 해당 프로젝트의 전체 계획 검토 완료 증거가 아니다. 이번 구현 Agent는 그 세션을 추가 조작하지 않았다.

## 최종 검토/남은 한계

고정 candidate의 fresh scoped 리뷰 결과를 후속 기록한다. 행동 평가는 격리된 native Agent 계획/packet 작업이며 실제 기능 세션 배정·GitHub 게시·제품 실행을 검증한 결과가 아니다. 다른 호스트/Windows 설치, 실제 OpenCode provider 동작과 runtime model identity는 미검증이다. push와 main 병합은 자동 수행하지 않는다.
