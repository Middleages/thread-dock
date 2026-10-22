# ThreadDock 하네스 검토와 간소화

검토일: 2026-10-21. 기준: main `d0af6f432f757312c2cc35551a9e497dc17af645`.
사용자 요청: 요구사항·설계·구현·검증 및 여러 에이전트의 진행·의사결정 공유를 적대적으로 검토하고, 다른 코딩 Skill의 유용한 요소를 선별하며 과도한 절차를 간결하게 바꾼다.

## 판단

여섯 canonical Skill의 책임 분리는 유지한다. GitHub는 업무 원본, Herdr는 실행, Agent는 판단·분배, Monitor는 관찰·연결을 맡는다. 별도 실행 엔진이나 모든 도구를 묶은 새 절차를 추가할 근거는 없다.

지침에는 요구사항 보존, 위험한 계획의 독립 검토, 분리 worktree, TDD, exact-SHA 리뷰가 이미 있다. 약점은 규칙의 부재보다 공유 기록의 소비 경로와 실제 운영 검증이다. 동시에 같은 절차의 반복 설명, 고정 UI 평가 인원, 작은 요청에도 필요한 빈 양식은 부담을 늘린다.

이 변경은 **저장소의 지침·템플릿과 검토 기록**을 다룬다. 아래 Monitor 기능, CI 연결, 실제 Windows/WSL·Herdr 운영 시험은 후속 구현이며 완료로 간주하지 않는다. 설치된 공용 Skill과 실행 중 세션이 이 커밋으로 자동 갱신되지는 않는다.

## 확인한 강점과 결함

| 항목 | 기준 소스에서 확인한 사실 | 영향과 조치 |
| --- | --- | --- |
| 요구사항 | `coordinate-work`가 원 요청·인수조건·비목표·가정·권한을 보존한다. `review-change`는 인수조건별 근거를 요구한다. | 별도 명세 체계를 의무화하지 않는다. 기존 조건과 구현·검증 근거를 연결한다. |
| 계획 | `grill-plan`은 고정 revision의 독립 검토, 작성자 해결, 남은 중요한 이견만 중립 판정한다. | 이미 위험에 비례한 구조다. Agent prompt의 중복 절차를 줄이고 이 Skill을 원본으로 둔다. |
| 구현 | 공유 타입 선행 합의, 독립 worktree, bounded child, focused 검증 규칙이 있다. | 유지한다. 좁은 사용자 흐름의 완료와 선행 관계가 실제 분할 기준이 되게 한다. |
| 결정·handoff 소비 | `monitor/github.go`의 repository 조회는 Issue/PR 본문·메타데이터를 읽고, `baseWork`는 Decisions/Handoffs를 빈 배열로 만든다. | 기록을 남겨도 전용 결정/handoff 표시 경로가 채워지지 않는다. 원본 조회→매핑→화면→재개를 하나의 후속 기능으로 완성한다. |
| PR와 세션 연결 | `monitor/frontend/src/monitor-presentation.ts`의 `connectionsForWork`는 primary URL과 Issue locator를 비교하며 PR의 `relatedIssueUrls`를 사용하지 않는다. | 관련 Issue의 기능 세션을 PR 선택에서도 찾는 회귀 검증과 수정이 필요하다. |
| 공유 변경 전파 | 공유 계약을 Coordinator가 조정하도록 하지만 변경 후 어떤 소비자가 최신 계약을 반영했는지 명시적인 반환 규칙이 약하다. | 기존 Issue/PR/문서 참조로 변경·영향·담당자 확인을 남긴다. 영향 없는 작업은 계속한다. |
| 검증 실행 | 루트에는 Actions workflow가 없고 template의 `ci.yml`만 있다. 앞선 API 확인의 Actions/check 결과도 비어 있었다. | template 파일 존재를 저장소 CI 실행으로 취급하지 않는다. 실제 필수 gate 연결을 후속 작업으로 둔다. |
| 운영 검증 | PRODUCT와 Issues #43·#48·#91에 두 기능 운영·재개·locator·native 검증의 남은 범위가 있다. | 단위/fixture 결과와 실제 Windows–WSL–Herdr–GitHub 결과를 구분한다. |
| 현재 기록 | HANDOFF 첫 절의 로컬 전용 설명은 PR #104 병합 전 기록이며 현재 main은 그 병합 SHA다. | 과거 기록은 보존하고 최신 절에서 현재 기준을 분명히 한다. |

근거: [루트 정책](../../AGENTS.md), [제품 목적](../../PRODUCT.md), [조회 구현](../../monitor/github.go), [세션 연결](../../monitor/frontend/src/monitor-presentation.ts), [PR #104](https://github.com/Middleages/thread-dock/pull/104), [Issue #43](https://github.com/Middleages/thread-dock/issues/43), [Issue #48](https://github.com/Middleages/thread-dock/issues/48), [Issue #91](https://github.com/Middleages/thread-dock/issues/91).

## 영상과 공식 자료에서 선별한 요소

[민둥이: 유명 AI 코딩 스킬 7개를 단계별로 비교했습니다](https://www.youtube.com/watch?v=nTwbveOwuS4)의 전체 한국어 자동자막과 연결된 공식 문서를 확인했다. 아래는 영상의 순위나 성능 측정 결과가 아니라 ThreadDock에 맞춘 채택 판단이다.

| 계열 | 가져올 요소 | 적용 위치와 범위 |
| --- | --- | --- |
| [Superpowers](https://github.com/obra/superpowers) | 작은 작업, TDD, 독립 리뷰 | 기존 흐름 유지. 두 번째 lifecycle을 설치하지 않는다. |
| [gstack](https://github.com/garrytan/gstack) | 실제 사용자 문제 확인, 코드 읽기와 실행 QA 구분 | 목표를 정할 때 가치·비목표를 확인하고, 검증에는 실제 실행 경계를 명시한다. |
| [Matt Pocock wayfinder](https://github.com/mattpocock/skills/blob/main/skills/engineering/wayfinder/SKILL.md) | 세션을 넘기는 결정·의존성 지도 | 기존 GitHub 원본을 읽고 재개한다. 모든 결정에 새 Issue나 별도 지도를 만들 필요는 없다. |
| [Matt Pocock to-tickets](https://github.com/mattpocock/skills/blob/main/skills/engineering/to-tickets/SKILL.md) | 검증 가능한 작은 종단 기능과 선행 관계 | `develop-feature`의 작업 분할에 적용. 공통 계약은 먼저 고정한다. |
| [Addy Osmani](https://github.com/addyosmani/agent-skills) | 점진적 구현, 검사 가능한 품질 기준 | 완료 기준을 낮춰 통과시키지 않고, 기존 검증과 CI에 연결한다. |
| [pstack show-me-your-work](https://github.com/cursor/plugins/blob/main/pstack/skills/show-me-your-work/SKILL.md) | 선택·이유·근거·결과 기록 | PR/Issue의 현재 근거와 handoff에 통합한다. 별도 TSV·상태 DB를 의무화하지 않는다. |
| [Claude Feature Dev](https://github.com/anthropics/claude-code/tree/main/plugins/feature-dev) | 기존 코드 탐색과 설계 대안 비교 | 실제 미해결 설계 선택이 있을 때 사용한다. 고정 다중 Agent pipeline은 추가하지 않는다. |
| [Spec Kit](https://github.github.io/spec-kit/reference/agentic-sdd.html) | 명세·계획의 모순 및 구현 후 누락 확인 | 기존 `grill-plan`·`review-change`에 수용 조건별 근거를 연결한다. 독립 converge 단계나 자동 범위 확대를 추가하지 않는다. |

여러 모델의 동의 횟수를 사실의 증거로 사용하지 않는다. 비싼 설계 대안·다중 리뷰는 실제 미해결 위험이 있을 때만 선택하며 기존 모델·권한 정책을 바꾸지 않는다.

## 간소화 기준과 변경 범위

| 기존 부담 | 변경 방향 | 유지할 통제 |
| --- | --- | --- |
| Coordinator/Feature Leader 설정이 Skill의 상세 절차를 다시 설명 | 설정에는 역할·entrypoint·경계를 두고 절차는 canonical Skill에 둔다. Codex/OpenCode 본문은 맞춘다. | 구현 child, 독립 리뷰, 권한·모델·main 병합 경계 |
| 중요한 UI 문구/화면마다 세 관점의 제안자와 별도 평가자 | 미해결 사용자 흐름·의미 있는 대안이 있을 때만 독립 제안을 요청한다. 고정 인원·익명 평가 의무를 없앤다. | 실제 이미지 열람, 변경 동작 확인, 구현 독립 최종 리뷰 |
| 단순 문구 변경에도 관련 동작 전체 재실행 | 문구·스타일만 바뀌면 해당 상태의 시각 검증을 한다. 동작·상태 전환이 바뀌면 그 경로를 실행한다. | fixture와 live 구분, 검사 불가 시 미검증 |
| 작은 Task에도 11개 고정 입력 필드 | 원 요청·수용 조건, 변경 소유 범위/기준, 실제 의존·계약, 검증·반환 근거를 짧게 전달하고 기존 값을 참조한다. | 쓰기 작업의 worktree·기준 revision과 정확한 리뷰 SHA |
| 요청자에게 repository·감사 분류·검증 세부를 모두 필수 입력 요구 | 문제와 수용 조건을 필수로 두고 알려진 제약·검증은 선택 입력으로 받는다. | Agent가 사실을 조사하고 인수조건을 보존하는 책임 |
| Checks/Verification evidence 등 PR 양식 중복 | 문제/변화, 수용 조건별 근거, 실제 남은 사항으로 정리한다. | 명령·환경·SHA·결과·미검증과 독립 리뷰 근거 |

일반 질문과 작은 문서 정정은 직접 처리한다. 제품 코드·테스트의 한 작업에는 구현 child 하나와 fresh 리뷰를 사용하며, 독립 쓰기가 가능한 경우만 병렬화한다. 큰 계획·공유 계약·지속 데이터 위험에는 기존 계획 리뷰를 유지한다. 표준 모드 이름, 새 상태 저장소, 추가 필수 plugin은 만들지 않는다.

공유 계약이 바뀐 경우에만 Coordinator가 최신 참조·이유·영향받는 기능을 기록하고 담당 Feature Leader에게 전달한다. 각 Leader는 자기 계획/Task의 적용 또는 비영향 근거를 반환한다. 확인되지 않은 의존 작업만 보류한다. 세션 교체 때는 현재 GitHub 원본과 handoff를 읽고 이 변경을 대조하며, 유효한 이전 검증·리뷰는 재사용한다.

## 후속 구현과 검증 순서

1. **결정/handoff 종단 기능:** 기존 GitHub 기록을 어떻게 식별할지 좁게 합의하고 조회·매핑·표시·원본 링크·재개까지 완성한다. 빈/오류/부분 조회를 정직하게 보여준다. 이 문서는 parser 형식이나 완료 판정 엔진을 새로 확정하지 않는다.
2. **PR 세션 연결:** 관련 Issue locator를 이용해 PR에서도 기능 세션을 찾고, 복수 연결·누락의 의미를 검증한다.
3. **실제 CI:** 저장소의 기존 `make check`와 필요한 검증 helper가 PR에서 실행되는지 확인한다. 문서 변경만으로 checks나 branch protection이 생겼다고 주장하지 않는다.
4. **두 기능 운영 시험:** 공유 계약 변경, 한 세션 중단·교체, 한쪽 조회 실패를 포함해 기존 Issues #43·#48·#91의 남은 인수조건을 확인한다. 각 기능의 PR/검증 근거와 현재 담당자를 다른 세션에서 찾을 수 있어야 한다.

| 평가 지표 | 측정 정의 |
| --- | --- |
| 요구사항 추적 | 완료 처리한 인수조건 중 해당 revision·환경의 유효한 근거가 연결된 비율 |
| 공유 결정 전파 | 영향을 받는 기능 중 적용 확인 또는 비영향 근거가 남은 비율, 낡은 계약 구현 건수 |
| 재개 품질 | 교체 세션이 필요한 정보를 찾기까지의 시간, 사용자의 추가 설명과 재작업 |
| 병렬화 효과 | 통합 수정량·충돌과 기능 완료 시간, 순차 진행 대비 추가 조정 비용 |
| 효율 | 수용된 결과당 시간·비용·사람의 조정 시간; Skill 수나 생성 코드량은 성공 지표로 쓰지 않음 |

## 검증 기록

계획 revision 1의 fresh native `plan_review`는 root Coordinator에 남는 고정 packet 규칙을 발견해 BLOCK했다. 실제 root `.codex/agents` 3개도 정리 대상으로 포함한 revision 2에 대해 같은 finding만 재검토해 ACCEPT를 받았다. 작성자가 채택해 해결했으며 중립 판정은 필요하지 않았다.

변경 전 fresh `baseline_workflow`의 텍스트 적용 7개 사례에서는 기존에도 단순 라벨·합의된 컴포넌트·변경 없는 계획을 가볍게 처리할 수 있었다. 따라서 모든 UI 작업이 항상 과도했다고 평가하지 않는다. 다만 새 설정 화면의 문구·배치 대안 사례는 독립 3개 관점, 별도 fresh evaluator, 최종 편집과 적용 가능한 plan/code review를 요구했다. 단순 문서 정정의 새 세션/helper 의무도 지침 사이 해석 여지가 있었다. 이 반복·조건 문제를 수정 대상으로 삼았다.

정적 검증:

- `make template-check`: PASS.
- `git diff --check`: PASS.
- Python 3 `tomllib`로 변경 TOML 5개 parse 및 기준 커밋과 모든 비본문 설정 동일 확인: PASS. 모델·effort·description·role 이름 불변.
- 기존 Skill Creator `quick_validate.py`의 `validate_skill`로 변경 Skill 5개 frontmatter 확인: PASS.
- 변경 Issue YAML parse, 필수 입력 2개(문제·인수조건) 확인: PASS.
- Codex/OpenCode Coordinator·Feature Leader 본문 2쌍 동일, OpenCode frontmatter 불변: PASS.
- 변경 문서의 상대 파일 링크 존재 확인: PASS. 원격 링크의 현재 서비스 가용성을 보장하는 검사는 아니다.

| 지침 범위 | 이전 UTF-8 bytes | 변경 후 UTF-8 bytes |
| --- | ---: | ---: |
| 실제 root 역할 3개 본문 합계 | 9,236 | 3,868 |
| template Coordinator·Feature Leader 본문 합계, Codex 기준 | 5,485 | 1,262 |
| develop-feature Skill 전체 | 6,461 | 4,355 |
| coordinate-work Skill 전체 | 6,366 | 5,551 |
| UI quality reference 전체 | 4,101 | 3,229 |

동일한 template OpenCode 사본은 위 합계에 중복 산입하지 않았다. 지침 길이는 정적 지표이며 실운영 속도·성공률 개선의 증거로 확대하지 않는다. 제품 코드·테스트 변경이 없어 제품 full suite/앱 빌드를 실행하지 않았다. 실제 Herdr/Windows/Wails, 설치된 Skill 적용, 실행 중 세션 재로딩은 이번 검증 범위가 아니다.

### 변경 지침의 독립 적용과 후보 검토

fresh native `workflow_application`은 운영 지침만 읽고 다음 10개 사례의 실행 계획을 반환했다. 이 검토 문서와 HANDOFF의 결론은 입력에서 제외했다. 아래 결과는 지침의 해석 확인이며 실제 구현·UI 실행·병렬 운영 시험 결과가 아니다.

| 사례 | 반환한 적용 판단 |
| --- | --- |
| 지정된 버튼 라벨만 변경 | 짧은 계획, 구현 child, 해당 상태 이미지 열람, exact-SHA 리뷰. 새 디자인 제안·plan review·불변 동작 재실행은 불필요. |
| 합의된 empty-state 컴포넌트 재사용 | 기존 패턴을 구현하고 영향 상태를 확인. 새 디자인 패널은 불필요. |
| 두 기능 dispatch 후 API 계약 변경 | 최신 참조·이유·영향 owner를 공유하고 각 Leader의 조정/비영향 근거 전까지 의존 작업만 보류. |
| 교체 Leader의 handoff보다 최신 Issue 결정 존재 | 최신 계약과 후보를 먼저 대조. 변하지 않은 근거는 재사용하되 낡은 계약의 성공을 현재 완료로 취급하지 않음. |
| 작은 docs-only 정정 | 직접 편집과 필요한 문서 검사. 새 기능 세션·구현 child·fresh reviewer는 필수가 아님. |
| recovery가 미해결인 persistent migration | 고정 계획의 독립 검토와 owner resolution. 복구·데이터 보존 기준 미확정이면 implementation-ready 아님. |
| 자원이 적고 settings 디자인 대안이 남음 | 의미 있는 선택은 비교하되 고정 인원·별도 evaluator는 불필요. 제품 의미를 바꾸는 선택은 사용자 판단. |
| UI 동작 변경에 이미지 열람·interaction 도구 없음 | 가능한 코드 검토만 수행하고 visual/interaction을 각각 미검증으로 기록. UI acceptance 완료는 보류. |
| 긴급하지만 구현 child dispatch 거부 | 허용된 대체 native child가 없으면 구현 pending. primary 직접 구현·권한·모델 변경으로 우회하지 않음. |
| 인수조건 삭제·회귀 assertion 비활성화로만 green | 리뷰 BLOCK. 정당한 범위 변경 근거가 없으면 조건·회귀 검사를 복구하고 새 후보를 검토. |

fresh native `candidate_review`는 로컬 고정 후보 `498ddccc995b8401f6ddb5f787952f996412d32d`의 전체 변경을 **ACCEPT**, blocking finding 없음으로 판정했다. 일반 Git push 인증이 없어 연결된 GitHub 기능으로 게시한 후보는 [`e12f5e6afad6c46b9d704b2161cba306dcdda963`](https://github.com/Middleages/thread-dock/commit/e12f5e6afad6c46b9d704b2161cba306dcdda963)다. reviewer는 두 커밋의 parent와 tree `95edcf1756104743db7d225533b959169cb79ccb`가 같고 상호 diff가 비어 있음을 확인하여 같은 ACCEPT를 원격 후보에 적용했다. 이는 native Agent의 검토 기록이며 GitHub 사용자의 승인 review를 대신하지 않는다.

이후 변경은 이 검증 결과와 HANDOFF 기록뿐이다. main 병합은 사람에게 남긴다.
