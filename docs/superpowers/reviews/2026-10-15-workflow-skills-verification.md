# ThreadDock 업무 스킬 개선과 행동 검증

이 문서의 2026-10-15 근거는 당시 local 후보·설치·행동 평가 기록이다. 2026-10-16의 최신 main 통합과 다른 컴퓨터 설치 지침은 마지막 절에서 별도로 다룬다.

## 범위와 선택

사용자는 반복되는 내부망 확인 질문을 없애고, 실제 화면 검증·독립 문구 비교·중요 계획의 반대 검토/중립 판정·간결한 개발 구조·이해 가능한 완료 설명을 스킬과 Agent에 구현하고 시험하도록 요청했다. Astra가 이번 지침/config 작업을 작성했다. 제품 개발용 Sol/Luna 모델·권한 설정은 변경하지 않았다.

여섯 canonical workflow를 유지했다. `develop-feature`의 두 reference에 공통 개발 기준과 조건부 UI 절차를 두고, `grill-plan`과 `publish-work`를 확장했다. 기존 primary 역할과 legacy `plan-work`는 같은 기준으로 연결한다. 별도 실행 엔진·Node 서버·새 역할·필수 plugin은 추가하지 않았다.

- Herdr: 설치 help/skill, 실제 managed context와 반환 ID를 확인한다. 기본 internal-only 인증 요구를 없앴고, 사용자/환경에 명시된 네트워크 제한만 해당 동작에 적용한다. 기존 승인과 모델·권한 경계를 보존한다.
- UI: 화면/flow 단위의 독립 신규 사용자·반복 운영자·편집/접근성 제안 → 작성자 정보를 가린 별도 평가 → 한 editor의 일관된 최종안 → 구현/실제 화면 검사. 오타 수준은 후보 절차를 생략한다. 모든 UI 변경은 이미지 입력으로 실제 screenshot을 열어 보는 검사와 관련 동작 검증이 필요하다.
- 계획: 공유 interface·영속 상태·migration 등 중요한 계획은 고정 초안의 작성자 → fresh adversary → 별도 neutral adjudicator. 채택/기각 근거와 미결정을 남기며, 작은 명확한 작업은 짧게 처리한다.
- 구조/보고: 현재 구조와 단일 원본을 존중하고 필요한 책임만 계층으로 나눈다. 빈 계층·중복 상태·근거 없는 gate를 피한다. 완료/대기/차단 설명에 문제, 사용자 관점 변화, 선택 이유와 tradeoff, 실제 검증/한계를 포함한다.

`skill-creator`와 `writing-for-agents`/`SKILL-MECHANICS`를 실제 편집에 사용했다. 이미 설치된 `impeccable`은 UI 작업, `codebase-design`은 어려운 interface/test seam, `domain-modeling`은 용어·상태 의미 충돌에만 조건부로 연결했다. 이들의 전체 workflow·독자 용어·glossary/ADR 생성을 모든 작업에 강제하지 않는다. 추가 설치는 중복이므로 하지 않았다.

## 설치와 변경 경계

기준 repository HEAD는 `d57beeb5e1218ec39d5053b74e6ae5d1c4b6b5b8`이다. 착수 전 dirty HANDOFF/quickstart/develop-feature/OpenCode primary/AGENTS 및 research 문서를 보존했다. 현 checkout의 제품 코드·테스트는 변경하지 않았다.

현재 Linux 사용자에만 좁게 반영했다: `~/.agents/skills`의 변경된 8개 Skill과 reference 2개, `~/.codex/agents`와 `~/.config/opencode/agents`의 primary 2개씩. 전역 원본은 `/tmp/threaddock-workflow-20261015.N8ggRr/before/global`에 백업했다. 전역본에는 template에 없는 PR #98의 named leaf·진단·원문 조건·Project read-back 지침이 있으므로 통째 덮어쓰지 않았다. leaf 역할 8개와 모델/effort/permission/mode metadata의 바이트/parse 비교를 통과했다.

Windows 사용자 설치, 다른 호스트, 현재 열린 대화의 registry 교체, GitHub 게시와 main 병합은 수행하지 않았다.

현재 ThreadDock 저장소의 로컬 `.codex/agents` 역할은 동명 전역 역할보다 우선하므로 별도로 연결 경로를 확인했다. coordinator/implementer/reviewer 세 파일 모두 첫 지시에서 root `AGENTS.md`를 읽는다. root `AGENTS.md`에 공통 기준·중요 계획·UI·완료 설명의 조건부 포인터 네 개를 추가해 이 프로젝트에도 적용했다. 기존 로컬 역할 파일은 수정하지 않았으며 전후 SHA-256이 일치한다. 포인터 대상 네 파일의 존재, 고정 후보와 실제 AGENTS의 `cmp`, scoped diff 검사는 통과했다.

후속 delta는 별도 `/tmp/threaddock-workflow-20261015.N8ggRr/root-routing-review`의 `7043f21044bedef7725c3f2ab1b95d9a39b9e281` → `908d76571a70466b468e7ccef63549f205f0ce94`로 고정했다. fresh `td_reviewer`의 `local_routing_delta_review`는 AGENTS 7줄 추가만 검토해 **ACCEPT**, blocker 없음으로 판정했다. 이전 30파일 리뷰와 행동 suite를 반복하지 않았다. 실제 checkout에는 선행 template/reference 변경과 이 포인터가 함께 적용되어 있다.

## 검증 방법과 근거

원시 fixture·검증 script·backup·고정 diff snapshot은 `/tmp/threaddock-workflow-20261015.N8ggRr`에 있다. 사전 판정 기준은 `eval-oracles.md`에 기록하고 행동 평가자에게 전달하지 않았다. 평가자는 실제 요청, 해당 후보 Skill, 최소 원본만 받아 작업했다. 실제 native dispatch를 사용했으며 한 Agent가 여러 역할을 연기한 결과로 대체하지 않았다. 역할/모델은 기존 허용 설정을 사용했고 backend runtime model identity를 독립 인증한 것은 아니다.

| 검증 | 실제 결과와 범위 |
|---|---|
| 구조 | `python /tmp/threaddock-workflow-20261015.N8ggRr/validate.py`: template/global 16개 `quick_validate.py`, 상대 reference 및 quickstart 링크, Codex TOML/OpenCode YAML, 모델·권한 metadata/leaf 보존, 전역 PR #98 고유 지침 보존 PASS |
| template | `make template-check`, `git diff --check` PASS. 문서/config-only이므로 전체 `make check`는 실행하지 않음 |
| OpenCode discovery | `opencode --pure agent list` exit 0, `td_coordinator (primary)`와 `td_feature_leader (primary)` 인식. 실제 provider/native task/vision 실행은 이번 평가에서 미검증 |
| Herdr 정상/제한 | fresh `herdr_behavior_eval`이 설치 help/skill, managed context, 실제 JSON/locator를 읽음. 일반 승인 작업에 내부망 증명을 추가하지 않음. 명시적 internal-only+provider route 미제공 사례는 통신 동작만 pending, 로컬 조회는 허용. 대상 업무가 불명확한 실제 상태에서 임의 resume하지 않음. 실제 Agent start/GitHub 쓰기는 평가 범위 밖 |
| 이미지 부재 | fresh `visual_missing_eval`이 이미지가 사라진 fixture-r3의 DOM/build/capture 진술만으로 visual PASS를 거부. 기존 interaction 근거는 구분해 채택하고 해당 이미지 재검사만 요청. 모델 교체·설치·전체 suite를 요구하지 않음 |
| 작은 작업 | `small_work_eval`이 README의 `보존됩니다다.`만 수정. 독립 계획/문구 후보 절차를 추가하지 않았고 단일 변경과 의미 보존을 확인한 짧은 완료 설명을 반환 |
| 중요한 계획 | `plan_flow_eval`이 고정 R1 계획 → fresh `td_reviewer` adversary → fresh `td_coordinator` neutral을 실제 순차 dispatch. key 충돌 데이터손실, 빠진 호출 지점, mtime 기반 무쓰기 오판, 미지원 version 규약을 찾음. neutral은 finding 채택 이유와 제안 정책의 미확정 부분을 분리. 최종 계획은 없는 근거를 꾸미지 않고 구현 `BLOCKED`를 명시. 계획 평가 PASS는 fixture 구현 ready를 뜻하지 않음 |
| UI flow | 3개 독립 관점 → 익명 fresh 평가 → 단일 editor 정정 → Luna 구현 → fresh UI 리뷰 실제 실행. 후보 HTML SHA-256 `3f54d27b7bbd16d64e791b0ca9883ddbf906e0ddf16accbe2f6f24a422458ad7`에서 pytest `1 passed`, desktop/narrow console/page errors 0, 가로 overflow 0. fresh reviewer가 PNG 4개를 `view_image(detail="original")`로 직접 열고 ACCEPT |

Herdr 상세 원시 관찰에는 다른 프로젝트 locator가 포함되므로 이 문서에는 복사하지 않았다. 정리된 결과는 `eval-herdr/result.md`, 이미지 부재 결과는 `eval-visual-missing/result.md`, UI 과정은 `fixtures/ui/evidence/design-process.md`에 보존한다.

중요 계획의 입력은 `fixtures/plan/{brief.md,storage.go}`, 실제 산출물은 `original-plan.md`, `review.md`, `adjudication.md`, `final-plan.md`다. 기존 `Save`의 atomic 저장 근거가 있는 상황에서 새 RepositoryAdapter/TransactionService가 필요하다는 의견은 채택하지 않았다. neutral은 collision 검출·무쓰기 보호는 수용하되 미제공된 충돌/버전 정책을 제품 결정으로 확정하지 않았고, 호출 지점은 실제 저장소 근거로 먼저 해결할 대상으로 분리했다.

## 실제 평가에서 드러난 문제

초기 UI editor가 내부 디자인 mode `Operate`를 화면 제목으로 넣고 현재 상태 전체를 live region으로 만든 제안을 냈다. UI Feature Leader가 제품 문구/접근성 회귀로 판단해 같은 editor에게 좁게 정정을 맡겼다. 작성자의 외부 개입 없이 최종 편집 절차가 이 문제를 발견했다. 제안이 처음부터 모두 정확했다고 보고하지 않는다.

최종 화면에서 현재 미확인 경고와 과거 `09:41` 기록을 분리했고, 재시도 후 기존 정보가 유지되며 결과만 단일 live region에 추가됐다. 실제 Enter 활성화, busy/disabled, 시각 보존, focus 복귀를 검사했다. reviewer는 desktop 1280px와 narrow 390px의 초기/재시도 이미지를 직접 열어 경고·시각 구분, 긴 문구 줄바꿈, 잘림 없음, 버튼 focus ring을 확인했다. screenshot capture나 DOM 검사만으로 통과시킨 결과가 아니다.

UI worker의 최초 RED는 기존 상태 문구가 수용 조건과 달라 실패했고, 최종 GREEN은 `1 passed in 6.16s`였다. 중간 CSS/검증 수정 후 focused 재실행을 했으며 reviewer는 동일 후보 결과를 다시 실행하지 않았다. Impeccable detector는 parser 부재로 degraded/regex fallback이었고 side-tab 경고 후 스타일을 수정했다. 최종 detector 재실행은 없으므로 clean detector라고 보고하지 않는다. 최종 시각 수용은 실제 이미지 검사로 별도 충족했다.

사전 독립 설계 adversary의 8개 지적은 별도 neutral이 이유와 함께 수용했다. 이에 따라 UI orchestration은 Leader에만 두고 leaf의 재위임 금지를 보존했으며, 제안자와 구현 파일 소유권·유의미한 계획 기준·이미지/동작 근거·전역 좁은 병합·상태에 맞는 완료 설명·상대 link 검사를 명시했다.

## 최종 고정 리뷰와 한계

fresh `td_reviewer`의 `/root/astra_workflow/workflow_final_review`는 기준 `b3a09bb935d4807d907199aa2a71c0a64dec7b81` → 후보 `7f9604835e7b7bc1634db0a04eae63ea69d60ef8`의 30개 정책/설치 경로를 검토해 **ACCEPT**, blocking finding 없음으로 판정했다. 이 SHA는 사용자 checkout을 commit한 것이 아니라 `/tmp/threaddock-workflow-20261015.N8ggRr/review`의 독립 snapshot이다. baseline에 착수 전 dirty 내용과 서로 다른 전역본을 그대로 넣어 이번 변화만 고정했다. 후보와 실제 적용된 30파일의 `cmp`도 exit 0이다. 이 보고서와 HANDOFF의 결과 기록은 정책 리뷰 후 별도 문서 검증 대상이다.

요청된 지침 구현·Linux 설치·행동 검증은 완료했다. 브라우저 fixture는 실제 Windows/Wails·live GitHub/Herdr 제품 E2E 근거가 아니다. OpenCode는 primary discovery만 확인했으며 실제 provider/native task/vision 실행은 미검증이다. 현재 열린 대화의 registry가 교체됐다고 주장하지 않으며 새 세션은 갱신된 설치 파일을 읽는다. 원시 artifact와 backup은 위 `/tmp` 경로에 남아 있으므로 환경 정리 시 소실될 수 있고, 이 문서는 그 결과와 한계를 저장소에 남긴 기록이다.

## 2026-10-16: 최신 main 통합과 전역 설치 안내

사용자가 커밋·push와 다른 컴퓨터의 Agent용 설치 안내를 승인했다. taskId는 `workflow-quality-global-setup`, baseSHA는 `c52ac8b3bd9282ba72d3ae7a462d3d22b0a2bb85`, branch는 `agent/workflow-quality-global-setup`이다. 독립 worktree에서 기존 승인된 정책을 의미 단위로 이식했으며 원래 dirty checkout·전역 설치본·제품 코드는 수정하지 않았다.

이번 diff의 소유 범위는 root/template AGENTS, 기존 workflow Skill/reference, primary 역할 지침, quickstart, [global-agent-setup](../../operator/global-agent-setup.md), 이 검증 기록과 새 HANDOFF 절이다. 최신 main의 `brainstorm-design`, `agent-wait`와 역할 6개·helper를 보존하고 UI 후보 비교는 같은 질문의 brainstorm 입력으로 재사용하게 했다. source 역할의 모델·permission metadata는 유지했으며 대상 컴퓨터에는 해당 컴퓨터의 설정을 보존해 설치하도록 안내한다. model-profile/OMO 설계 문서를 실행하거나 연구/과거 dirty HANDOFF 덩어리를 가져오지 않았다.

설치 안내는 선택한 source ref/SHA, 실제 HOME/CODEX_HOME/XDG/OpenCode 경로, supporting files를 포함한 디렉터리, 백업·좁은 병합, 프로젝트 override, 로컬 parse/발견과 실제 provider/native/UI 실행의 구분을 다룬다. 원격 또는 다른 OS에 실제 설치한 결과를 주장하지 않는다.

통합 검증은 `make template-check`, `git diff --check`, `python /tmp/threaddock-workflow-20261015.N8ggRr/validate-integration.py`를 실행해 PASS했다. 실제 tracked source의 Skill 15개 creator/frontmatter, Codex/OpenCode 역할 6쌍 parse와 model/permission metadata 보존, leaf 8개 및 brainstorm-design/agent-wait 원본 byte 보존, 모든 Skill reference·설치/빠른 시작 링크, root 개발 역할과 PRODUCT/CONTEXT 보존을 확인했다. 새 primary 지침에도 brainstorm/wait 경로가 남아 있다. Codex/OpenCode의 현재 `--help`와 공식 설정/Skill/Agent 문서로 설치 지침의 경로·발견 설명을 교차 확인했다.

이전 행동 평가를 반복하지 않았고 전체 `make check`도 실행하지 않았다. 새 설치 안내와 최신 main 통합 diff는 고정 candidate commit의 fresh 독립 리뷰로 마무리한다. 원래 dirty checkout과 기존 Linux globals는 이번 commit에 포함하거나 변경하지 않는다.
