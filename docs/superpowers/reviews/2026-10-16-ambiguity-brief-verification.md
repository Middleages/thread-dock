# 목표 모호함 판정과 단일 기능 계획 소유 검증

## 바뀐 정책

사용자 정정에 따라 Coordinator의 필수 전체계획·삼자 검토 gate를 없앴다. Coordinator는 목표·범위·우선순위·모호함·의존성·공유 제약을 정리한 brief를 넘기고, 기능 계획은 Feature Leader가 한 명의 소유자로 유지한다. 이미 합의한 계획은 인수·실행가능성 확인 후 재사용하며 같은 재작성·재승인 과정을 반복하지 않는다.

모호함은 제품 선택, 자료로 확인할 사실, FL의 구현 선택, 무해한 가역 가정으로 구분한다. 기존 문맥을 읽고도 두 해석이 수용조건/범위를 달리할 때만 영향과 선택지를 좁혀 묻는다. 구조·파일·테스트 내부나 숫자 목표가 없다는 이유로 목표를 모호하다고 판정하지 않는다. 명확하지만 위험한 migration은 FL의 독립 계획 검토 대상이지 반복 제품 질문 대상이 아니다.

중요한 새 계획/실질적 위험 변경은 독립 adversary 한 명의 검토 후 작성자가 근거로 해결한다. 중요한 이견이 남을 때만 다른 neutral 한 명을 사용하고, 해결되지 않으면 정확한 사실/제품 결정과 의존 작업만 남긴다. 계획 리뷰·이견 판정·코드 리뷰의 입력과 근거를 기존 reviewer 역할에 구분해 미구현 screenshot/test 성공을 계획 단계에서 요구하지 않게 했다. 실제 코드 리뷰의 UI/테스트 근거 요구는 유지한다.

taskId `coordinator-ambiguity-brief`, base `b04cf2a3c04bbf0036230025707948003fc83d65`, branch `agent/coordinator-ambiguity-brief`, worktree `.worktrees/coordinator-ambiguity-brief`다. task packet·backup·원시 fixture는 `/tmp/threaddock-ambiguity.r7xROw`에 있다. 제품 코드·UI pipeline·모델·권한·인증·locator/schema/GHES는 변경하지 않았다.

## 행동 평가

`skill-creator`와 `writing-for-agents`를 사용했다. 평가자는 후보 Skill/role 계약과 격리된 사용자 요청·원본만 받아 실제 응답/brief/계획·판정 기록을 만들었다. 사전 `oracles.md`와 작성자 논의는 전달하지 않았다. 이전 전체 UI/계획 평가를 반복하지 않고 세 묶음으로 새 분기를 확인했다.

| 사례 | 실제 관찰 |
|---|---|
| 모호한 단어지만 문맥이 충분 | `ambiguity_triage_eval`이 ‘목록을 빠르게’의 기존 Issue에서 중복 조회 제거·새로고침 의미를 확인하고 숫자 목표나 추가 질문 없이 A brief 작성 |
| 실제 상충 제품 결과 | 같은 평가가 ‘보고서 자동 정리’를 보관/영구 삭제와 기간의 두 질문으로 좁힘. B 배정만 보류하고 독립 F0는 유지 |
| 명확한 목표/미정 기술 | C migration은 제품 목표가 명확해 구조·JSON 방식 질문 없이 FL brief로 넘김. 최초 결과의 위험 검토 표시는 약해 해당 문구를 보강하고 아래 한정 재검사 |
| 기존 계획 재사용/검토 합의 | `plan_agreement_eval`은 기존 R3를 재사용하고 추가 interface 위험 R4만 fresh `td_reviewer` plan-review 1회. 기존 sort allowlist를 덮을 수 있는 결함을 owner가 R4.1에서 해결, 선택적 병렬화 제안은 근거로 기각. 중립 판정·사용자 재승인·전체 재검토 없음. 미래 UI screenshot/실행 tests를 통과 조건으로 요구하지 않음 |
| 남은 중요한 이견 | `plan_dispute_eval`은 이미 완료된 adversary/owner 응답 뒤 다른 fresh reviewer를 adjudication mode로 정확히 한 번 호출. v1에 근거 없이 새 enum을 방출하는 위험을 수용하고 F0 유지. 다만 남은 versioning/capability 선택을 곧바로 사람의 제품 결정으로 분류한 부분은 실패로 판단해 수정 |

실패 fixture의 제품 계약은 v1 호환과 v2 추가 값 제공으로 이미 고정되어 있었다. 이를 만족하는 기술 대안의 선택은 FL 책임이며, 부족한 client/parser 근거가 곧 사용자에게 기술 방식을 선택하게 할 이유는 아니다. `grill-plan`에 기술 근거 부족과 제품 권한 결정을 구별하고, 사용자 escalation에는 바뀌는 수용조건/범위/권한을 명시하도록 좁게 보완했다. clear-risk migration의 FL 계획 검토 경로도 명확히 했다.

한정 재검사는 기존 전체 review cycle을 반복하지 않고 fresh `technical_ownership_recheck`에게 migration brief와 이미 끝난 adjudication 이후 남은 소유자 판단 두 건만 맡겼다. migration은 추가 제품 질문 없이 FL의 독립 plan-review 1회로 넘겼다. 로그 API는 v1 불변/v2 partial 계약을 만족하는 명시적 versioned response를 FL 기술 선택으로 정하고, 부족한 client 근거는 소유자·조사 한계와 관련 작업 보류로 남겼다. 사용자에게 기술 방식을 고르게 하거나 neutral을 다시 호출하지 않았고 F0는 계속했다. 한정 재검사 PASS이며 원래 실패 응답은 `dispute/brief.md`에 보존한다. 최종 재검사 출력은 `targeted/`에, agreement의 원안/검토/해결/호출 기록은 `agreement/`에, triage 응답과 읽기 기록은 `triage/`에 있다.

## 설치와 focused 검증

WSL 변경 대상만 `/tmp/threaddock-ambiguity.r7xROw/before`에 백업했다. 공용 Skill4(coordinate/grill/develop/review-change), Codex/OpenCode 역할3개씩(coordinator/feature_leader/reviewer)에 같은 지침 delta를 반영했다. 모델·effort·mode·permission은 보존하고 reviewer 설명만 계획/판정/코드를 포괄하도록 바꿨다. source와 실제 global 파일은 일치한다.

원래 dirty checkout에는 허용된 root AGENTS의 기존 삼자 검토 문장 한 줄과 root dev reviewer의 mode 분기 지침만 좁게 적용했다. 다른 dirty 내용·세션은 보존했다. 새 branch에서도 같은 변경을 관리하며 원래 checkout 전체를 stage/commit하지 않았다.

- `python /tmp/threaddock-ambiguity.r7xROw/validate.py`: template/global Skill8 creator/frontmatter·reference, role6 TOML/YAML·metadata 보존·설치본 일치, 두 checkout의 root reviewer 모델 보존과 문서 링크 PASS.
- 좁은 보완 후 coordinate/grill frontmatter·설치본 parity와 diff를 재확인했다. 다른 동일 검사는 반복하지 않았다.
- `make template-check`, `git diff --check` PASS. 전체 `make check`와 기존 UI suite는 실행하지 않았다.

## 최종 검토와 한계

고정 candidate의 fresh scoped 리뷰 결과를 후속 기록한다. 행동 평가는 명시적으로 전달한 후보 지침을 읽은 native Agent의 격리된 계획 작업이다. 현재 열린 모든 세션의 registry가 갱신됐다는 증거는 아니며 실제 Herdr 배정/OpenCode provider/Windows 설치/제품 UI 실행은 별도 미검증이다. 외부 세션 메시지·GitHub 게시·push·PR·merge는 수행하지 않는다.
