# ThreadDock 진행 설계 정정과 다음 작업

## 2026-10-21: 하네스 후속 구현 — CI와 PR 세션 연결

- 사용자의 구현 요청에 따라 PR #105 기준 `f8d7467`에서 `agent/harness-followups-20261021`을 분리했다. [구현·검증 기록](docs/research/2026-10-21-followup-implementation.md)에 범위·독립 검토·검증 한계를 기록한다. main 병합은 사람에게 남긴다.
- PR/main push의 단일 CI와 역할·Skill 구조 검사를 추가했다. 선택한 PR의 관련 Issue를 통해 기능 세션을 찾도록 Go/Wails 상세 조회와 React 연결·복사 흐름을 구현했다. Projects-only와 GHES, 캐시·실패·선택/설정 전환을 함께 다룬다. 상세는 60초 TTL을 사용하며 새 관찰이나 수동 새로고침에 다시 확인한다.
- CI Task `ff99a9b`와 PR 연결 Task `95a3ae3`는 각각 독립 ACCEPT다. 오연결·캐시·설정 경합 finding을 해결했다. 통합 후보 `f33c5c9`에서 마지막 `make check`가 통과했다(Go 3 packages, frontend 14 files/124 tests, build). 최종 통합 리뷰·원격 CI 결과와 게시 SHA는 후속 PR에 기록한다.
- 로컬 브라우저는 소켓 권한으로 실행되지 않았고 권한 확대는 환경 정책상 자동 거절됐다. cloud browser도 로컬 Vite에 접근하지 못했다. 실제 이미지 열람·UI 조작과 Windows/WSL/Herdr 운영은 미검증이다. #43·#48·#91의 별도 운영 완료 조건을 이번 코드 검사로 닫지 않는다.

## 2026-10-21: 하네스 적대적 검토와 지침 간소화

- 기준 main은 [PR #104](https://github.com/Middleages/thread-dock/pull/104)의 병합 SHA `d0af6f432f757312c2cc35551a9e497dc17af645`다. 아래 10월 18일 절의 로컬 커밋·미게시 설명은 병합 전 이력이다.
- 사용자 요청에 따라 [검토·채택·간소화 기록](docs/research/2026-10-21-harness-review-and-simplification.md)을 작성하고 `agent/harness-simplification-20261021`에서 실제 root 역할과 Codex/OpenCode template의 반복 지침, 고정 UI 평가 인원, 상세 필수 양식을 줄였다. 여섯 canonical Skill, 제품 코드/test child와 fresh 리뷰, 위험한 계획 검토, 기존 모델·권한·사람의 main 병합은 유지한다.
- 공유 계약 변경의 영향받는 Leader 확인, 재개 시 최신 결정 대조, 원 인수조건별 근거는 기존 GitHub 기록으로 연결한다. Monitor 결정/handoff 조회·PR 연결 수정·실제 CI·두 기능 운영 시험은 후속 범위다. Go/React 제품 코드와 사용자 전역 설치·실행 세션은 변경하지 않았다.
- 계획의 독립 검토와 owner resolution, template/parse/frontmatter/링크/설정 불변 검사가 통과했다. 운영 지침만 읽은 fresh Agent의 적용 사례 10개에서 작은 작업의 절차 축소와 위험·미검증 보류를 확인했다. 실제 실행 시험은 아니다. fresh `candidate_review`는 게시 후보 `e12f5e6afad6c46b9d704b2161cba306dcdda963`를 ACCEPT했고 blocking finding은 없었다. 이후 변경은 검증 결과 기록뿐이다. 상세 근거는 위 기록을 따르고 현재 게시·병합 상태는 GitHub branch/PR에서 확인한다. main 병합은 사람에게 남긴다.

## 2026-10-18: Herdr native 지침 단순화와 snapshot·metadata 검증

- PR #103은 main `437d4d2`에 병합됐다. 이번 후속 작업은 그 SHA에서 분리한 `agent/herdr-native-simplification` branch다. `agent-wait`는 native child의 harness 대기와 Herdr top-level의 exact-target 대기를 구분하며 고정 20–25분 규칙을 제거했다. `herdr-local`은 승인된 worktree 공간 생성·재사용에 native create/open, on-demand explain/process-info 진단을 우선하도록 연결한다. 모델·권한·live session·locator는 바꾸지 않는다.
- 현재 client/server는 모두 Herdr 0.9.5, protocol 22, endpoint generation 1이며 restart가 필요 없는 상태였다. 이전 0.8.2 조사 뒤 갱신된 환경을 사용했으며 이번 작업에서 update/restart를 수행한 것은 아니다.
- [snapshot 검증](docs/research/2026-10-18-herdr-snapshot-verification.md): read-only default session의 agent 6개/workspace 8개에서 list와 snapshot의 의미상 projection이 일치했다. raw snapshot 13,002 bytes는 512 KiB 한도 이내다. `agent` 종류와 optional 고유 `name`은 별개이며 alias로 바꾸지 않는다. Go 실행 파일 부재로 기존 Go 실패/캐시 경로와 Wails·다중 session·대규모 payload는 미검증이다. 제품 adapter는 변경하지 않았다.
- [metadata 판단](docs/research/2026-10-18-herdr-metadata-decision.md): schema와 help는 display-only annotation이며 durability·source 충돌·원자적 binding 근거가 없다. 실제 seq/TTL 동작은 미검증이다. `sessions.json` 유지, 신규 metadata writer 도입 보류로 결론냈다. Codex/OpenCode integration이 새 내장본보다 오래된 것도 관찰했지만 자동 갱신하지 않았다.
- `make template-check`, 변경 Skill 3개의 `quick_validate`, 관련 링크 존재와 diff 검사가 통과했다. fresh `td_reviewer`가 고정 SHA `3f8aea0fc7cc1e35a7a40d0eb8d64ea6c8d2568e`를 ACCEPT했고 blocking finding은 없었다. WSL 공용 Skill 3개는 `/tmp/threaddock-herdr-guidance.5AP9vL`에 백업한 뒤 적용했고 source와 `cmp` 일치를 확인했다. 이미 열린 session의 재로딩은 보장하지 않는다. 제품 코드 변경이 없어 full suite·앱 재빌드는 실행하지 않았으며, 새 생성·prompt·wait 동작의 live 실행 성공을 주장하지 않는다. 이번 branch는 로컬 커밋만 했고 push·PR·병합은 하지 않았다.

## 2026-10-17: 새 top-level 세션의 실행 정책 보존

- `d4f9aad`에서 `herdr-local`에 새 Herdr top-level 프로세스와 native subagent의 상속 차이를 명시하고 `coordinate-work`의 생성 단계에 연결했다. 실제 승인 정책과 sandbox를 각각 명시적으로 전달하고 생성 후 적용값을 확인한다. `never`는 sandbox 완화가 아니며 target의 경로·네트워크·managed 제한도 보존한다. 불명확·불일치는 보고하고 임의 기본값·우회·실행 중 세션 재시작으로 해결하지 않는다.
- 두 Skill의 `quick_validate`와 `git diff --check`가 통과했고 fresh `td_reviewer`가 고정 SHA `d4f9aad1c54c245d9928ad6a9ad669edac316ca9`를 ACCEPT했다. WSL 공용 `~/.agents/skills`의 두 파일을 `/tmp/threaddock-session-policy.3E3p29`에 백업 후 적용했고 `cmp`로 원본과 일치를 확인했다.
- 실제 새 Herdr 세션 생성 행동은 시험하지 않았다. 기존 세션·승인 dialog·global runtime 설정·모델·제품 코드는 변경하지 않았고 앱 재빌드는 필요 없다. 로컬 `agent/session-policy-preservation` branch 변경이며 push·PR·병합은 하지 않았다.

## 2026-10-17: 선택적 native review receipt 수집기

- Pi 연구의 typed native receipt 원리를 작은 read-only helper로 적용했다. `review-change/scripts/summarize_review_evidence.py`는 user-supplied Codex/OpenCode artifact 한 파일에서 explicit parent/child/`td_reviewer`/lifecycle/result field 위치만 stdout JSON으로 요약한다. DB·network·model·spawn·write·recursive scan이 없고 review quality/readiness/revision을 판정하지 않는다. Skill 본문은 반복 수집 때만 쓰는 optional collector로 연결한다.
- Luna 구현 후 독립 리뷰에서 실제 형식·최신 상태 선택·잘못된 입력·fallback 근거 위치를 보완했다. 최종 source `9c715df`는 14 focused tests 근거와 scoped 독립 ACCEPT를 확보했다. 실제 OpenCode completed task artifact는 sufficient receipt, actual Codex raw CLI는 serialized role/spawn이 없어 partial/unverified로 반환했다. Linux global `review-change`는 `/tmp/threaddock-review-evidence.WhfO4x/before`에 백업 후 pointer/reference/script만 설치했고 최종 source와 일치한다. 상세 실패·수정·검증은 [검증 기록](docs/superpowers/reviews/2026-10-17-review-evidence-helper-verification.md)을 따른다.
- 사용자 요청에 따라 root가 Codex Superpowers plugin을 local config에서 비활성화하고 backup·parse/equality를 확인했다. 격리 prompt input catalogue와 일반 root 요청에서는 빠졌지만, 중요한 fresh root의 runtime host catalogue에는 15개 항목이 남아 모델이 설치 cache의 두 Superpowers 문서를 직접 읽었다. 따라서 local config off는 완료됐으나 이 runtime의 완전한 지침 미사용은 보장하지 않는다. uninstall/cache 삭제/다른 설정 변경은 하지 않았다.

## 2026-10-17: 중요한 계획의 완료 계약 A·B 적용

- `ae11350405657ac705a33b781dfce77999396e09`에서 `develop-feature`·`grill-plan`과 두 harness의 Feature Leader 역할을 좁게 보강했다. planning-only는 구현/게시를 막지만 중요한 계획의 실제 독립 review와 owner resolution은 완료한다. 명시적 ideas/draft, 작은 명확 작업, 유효한 기존 review는 가볍게 유지한다. `reviewed`와 `implementation-ready`를 분리하고 기존 response/handoff 한 줄에 revision·실제 reviewer result/verdict·남은 중요 항목·readiness만 남긴다.
- Linux global Skill 2개와 Feature Leader role 2개는 `/tmp/threaddock-plan-contract.0AHAIa/before`에 백업 후 tracked source와 parity를 맞췄다. model/provider/permission/hook, Go/Wails·React, GitHub/Herdr 상태는 변경하지 않았다. 새 helper·DB·schema·scheduler는 없다.
- [행동 검증](docs/superpowers/reviews/2026-10-17-plan-completion-contract-verification.md)의 이름 없는 Codex root 3건은 충분한 중요 계획의 실제 child review+owner resolution, 명시적 small draft의 review 생략, source가 부족한 중요 계획의 assumptions/options review와 reviewed/not-ready 분리를 확인했다. OpenCode는 자동 skill 선택과 reviewer child 시작까지 확인했지만 제한 시간 내 result 회수/parent final이 없어 incomplete다. 성공으로 꾸미거나 모델을 바꾸지 않았다.
- [Pi 연구](docs/research/2026-10-17-pi-agent-harness.md)는 현재 공식 source에서 작은 core·progressive context·typed event receipt를 참고 원리로 확인했다. Pi 설치/이식, Durable runtime, hook 도입은 하지 않았다. fresh `td_reviewer`는 고정 SHA `322a72ec27a5fd28e8f8751fe2e9343eaa53e40d`를 ACCEPT했고 finding은 없었다. 이후 변경은 이 review 결과 기록뿐이다.

## 2026-10-17: 전체 Agent workflow 아키텍처와 개선안 문서화

- [전체 아키텍처와 개선 검토](docs/architecture/agent-workflow.md)에 자연어 진입, Coordinator/Feature Leader 소유권, native leaf, GitHub·Herdr·Go/Wails의 책임과 실제 검증 근거를 정리했다. 원격 main `b04cf2a`와 로컬 현행 구성 `9622d0e`, 전역 설치·이미 열린 session의 차이를 구분한다.
- [Codex](docs/superpowers/reviews/2026-10-16-codex-model-behavior.md)와 [OpenCode](docs/superpowers/reviews/2026-10-16-opencode-natural-entry-behavior.md)의 실제 행동 보고서를 함께 보존했다. 자동 feature 진입과 명시적 위임은 확인됐지만 자동 계획 검토는 두 fixture에서 미수행이었다. 미완성 초안 종료를 거짓 ready 선언이나 특정 Skill의 인과적 결함으로 확대하지 않는다.
- 개선안은 FL의 계획-only/명시적 초안/검토 완료/구현 준비 구분을 먼저 명확히 하고, 기존 결과에 최소 검토 근거를 연결하는 방향이다. hook·도구는 반복 누락이나 반복 수집 비용이 확인될 때 후속 검토한다. 이번에는 문서만 작성했으며 workflow·전역 설정·모델·권한·실사용 세션을 변경하지 않았다.
- 로컬 링크와 diff 검사를 통과했고, 고정 후보 `47dc5a208f998a1b4c53e7b2d32b80ddafe9b99d`의 문서 5개를 fresh `td_reviewer`가 ACCEPT했다. blocking finding은 없으며 reviewer runtime model/effort는 미검증이다. 제품 코드 변경이 없어 전체 suite와 모델 행동 시험을 반복하지 않았다. 로컬 문서화 branch에만 저장했으며 푸시·병합은 하지 않았다.

## 2026-10-16: 자연어 요청의 짧은 자동 진입

- `07ceb8455faa28884b379dc428c4c31baadc674d`에서 `project-template/global/AGENTS.md`를 canonical source로 추가하고 Codex/OpenCode 전역 시작 경로와 다른 컴퓨터 설치 안내를 연결했다. 모든 root를 Coordinator로 고정하지 않으며, 자연스러운 조정 요청은 `coordinate-work`, 배정된 단일 기능은 `develop-feature`, 작은 명확 작업과 일반 질문은 가벼운 경로를 사용한다. root의 무조건적 장문 시작 읽기도 관련성 기반으로 좁혔다.
- 현재 WSL의 신규 `/home/appuser/.codex/AGENTS.md`와 `/home/appuser/.config/opencode/AGENTS.md`는 source와 동일하다. 기존 파일이 없어 backup은 없고 Skill·role·model/provider/permission·hook은 보존했다. 이는 routing 지침 설치이며 runtime 모델/역할 변경이 아니다.
- [검증 기록](docs/superpowers/reviews/2026-10-16-natural-entry-verification.md)의 이름 없는 fresh Codex root 4건에서 일반 질문과 typo는 lightweight, 다중 우선순위는 `coordinate-work`, 단일 migration planning은 `develop-feature`로 처리됐다. 실제 OpenCode model 실행과 runtime model/effort identity는 미검증이다. 반복 기계 작업이 없어 helper/hook은 추가하지 않았다. fresh `td_reviewer`는 `b02b1d099857acdb9cd9917f3c251c894cf4141c`의 `d3b2b1e` 이후 범위를 ACCEPT했고 finding은 없었다. 이후 변경은 이 리뷰 결과 기록뿐이다.

## 2026-10-16: 작은 책임·brief·검증 기록 보완

- `3518798`에서 UI 후보 제안/평가 배정을 Feature Leader로 명확히 하고, 사용자 행동+보이는 성공의 선택적 brief 예시와 상세 검증의 현재 주기록 한 곳(PR, 없으면 Issue/요청, 미허가 외부 쓰기는 local/conversation)을 추가했다. 다른 기록은 요약·참조만 남긴다. UI 절차·hook/router·모델·권한은 변경하지 않았다.
- global Skill2를 `/tmp/threaddock-small-guidance.vJp0Rj/before`에 백업 후 반영했고 frontmatter/parse/link/parity/UI 불변·diff 검사는 PASS다. 상세 근거는 이번 요청 기록을 따른다. fresh `td_reviewer`는 동일 모델로 두 번 모두 capacity 오류로 실행 전 실패했으므로 이 작은 delta의 독립 리뷰는 미검증이다. 임의 모델 대체·자기검토 통과 처리는 하지 않았고 원격 게시도 하지 않았다.

## 2026-10-16: 전체계획 gate를 목표 brief·FL 단일 계획 소유로 정정

- 사용자 정정에 따라 이전 절의 필수 Coordinator 전체계획/삼자 검토는 superseded다. Coordinator는 문맥을 읽어 제품 모호함·사실 미확인·구현 재량·가역 가정을 구분하고 dispatch-ready brief를 넘긴다. Feature Leader가 기능 계획을 소유하며 합의된 계획은 재작성/재승인 없이 인수·재사용한다.
- 중요한 새 계획/실질적 위험은 독립 반대 검토 후 owner가 근거로 해결하고, 중요한 이견이 남을 때만 다른 중립 판정을 한 번 사용한다. reviewer의 plan/adjudication/code mode를 분리했다. 계획-only 권한과 기존 독립 업무·세션을 유지한다.
- `b04cf2a` 기준 branch `agent/coordinator-ambiguity-brief`에서 문서/지침만 수정했다. 현재 WSL Skill4/role6을 백업 후 반영하고 root dirty에는 허용된 AGENTS 한 줄·reviewer mode 지침만 맞췄다. 모델·권한·제품 코드·locator는 보존했다. [행동 평가와 검증 기록](docs/superpowers/reviews/2026-10-16-ambiguity-brief-verification.md)을 따르며 이번 작업은 원격 게시 없이 로컬 commit으로 마무리한다.
- 모호함 triage·계획 재사용/합의·조건부 중립 판정의 세 행동 묶음을 실행했다. 기술 선택을 사용자에게 전가한 초기 실패를 좁게 수정하고 두 소유권 사례만 재검사해 PASS했다. Skill/role parse·metadata/전역 일치·link/template 검사가 PASS했고 fresh 리뷰는 `fb3b0eb4606531ff0300204bcc2b12c2f10125de`에서 ACCEPT, blocker 없음이다. 이후 변경은 결과 기록뿐이며 전체/UI suite는 반복하지 않았다.

## 2026-10-16: Coordinator 전체계획을 기능 분배 전에 검토

- `f080fa9`에서 독립 branch `agent/coordinator-plan-first`를 만들고 기존 Skill3·primary4·template AGENTS·quickstart에 전체계획 선행과 Feature Leader 상세계획의 경계를 명시했다. 중요한 전체계획의 독립 반대 검토/중립 판정, 기존 검토본 재사용, plan revision/계약 인계와 영향분 환류를 요구한다. 계획만 요청한 작업은 구현/세션 시작/외부 쓰기 허가로 확대하지 않는다.
- 기존 세션·초안·승인된 독립 업무를 유지하고 기존 root dirty 파일과 제품 코드를 보존했다. 현재 WSL 전역 Skill3/primary4를 백업 후 좁게 반영했으며 모델·권한 metadata는 유지했다. 근거와 실제 평가·한계는 [검증 기록](docs/superpowers/reviews/2026-10-16-coordinator-plan-first-verification.md)을 따른다.
- focused Skill/role parse·link·설치본/백업 비교와 template 검사가 PASS했다. 실제 독립 계획-only 및 기존 검토본 재사용 평가를 완료했고, fresh `td_reviewer`는 candidate `a5b25129825d080713a3ca2c7aec9f4eb971c679`의 11파일·global7 적용을 ACCEPT했다. 이후 변경은 결과 기록뿐이며 기존 UI/전체 suite는 반복하지 않았다. 현재는 로컬 commit 상태로 push/main 병합하지 않았다.

## 2026-10-16: 업무 품질 기준 통합·다른 컴퓨터 설치 지침

- 사용자 승인에 따라 `origin/main`의 `c52ac8b`에서 branch `agent/workflow-quality-global-setup`, worktree `.worktrees/workflow-quality-global-setup`을 만들었다. 기존 dirty checkout과 중단 실험은 그대로 보존하고 이번 Skill/Agent 품질 기준만 의미 단위로 통합했다. 최신 `brainstorm-design`·`agent-wait`, native 역할 6개와 기존 leaf/helper 개선을 유지했다. model-profile/OMO 설계는 실행 설정으로 적용하지 않았다.
- Herdr의 undefined internal-only gate 제거, 중요한 계획의 독립 반대 검토·중립 판정, UI 문구 후보·익명 비교·실제 이미지/동작 검사, 공통 개발 기준과 완료 설명을 기존 workflow에 연결했다. root AGENTS에도 조건부 포인터를 넣어 로컬 역할이 전역 역할을 가리는 이 저장소에서 기준이 적용되게 했다. 기존 모델·권한과 제품 코드는 변경하지 않았다.
- 다른 컴퓨터의 Agent에게 저장소 URL/ref와 [전역 설치 지침](docs/operator/global-agent-setup.md)을 주면 tracked template에서 Skill 전체와 역할을 설치할 수 있다. 기존 모델·provider·권한을 보존하며 backup/compare/merge, 실제 사용자 홈·custom 설정 경로, 프로젝트 override, 설치/실행 검증 구분을 설명한다. 이번 후속 작업에서 현재 호스트의 전역 설치를 다시 변경하지 않았다.
- [이전 실제 행동 평가와 이번 통합 근거](docs/superpowers/reviews/2026-10-15-workflow-skills-verification.md)를 구분한다. 기존 Codex 행동/UI fixture 평가를 반복하지 않았고 최신 main 통합·설치 안내의 template/Skill15/역할6쌍 parse·metadata/reference 검사가 PASS했다. fresh `td_reviewer`는 `6bd454fe576bf57f96f5fa4cb5d519b2c2db2da1`의 20파일 통합 diff를 ACCEPT, blocker 없음으로 판정했다. 이후 변경은 이 검증 결과 기록뿐이다. branch push는 root가 담당하고 main 병합은 사람에게 남긴다.

## 현재 작업: OMO 없는 native 전문가 팀

- [PR98](https://github.com/Middleages/thread-dock/pull/98)을 agent/native-specialist-team → main으로 게시했다. Issue97/Project1은 사람 병합 대기이며 main을 병합하지 않았다. worktree와 동작 probe의 실패·성공 근거를 보존한다.

- [Issue97](https://github.com/Middleages/thread-dock/issues/97), [설계](docs/superpowers/specs/2026-09-14-native-specialist-team-design.md), [계획](docs/superpowers/plans/2026-09-14-native-specialist-team.md)에 따라 Codex/OpenCode leaf4개와 공용 helper2개를 추가한다. 두 primary와 canonical6개를 유지한다.
- Task 구현7f5e2a 및 review-change 보강adf1a58, 통합점검의 tdd 단순성/Quickstart 권한 설명 수정d1b826e는 각각 focused 검증과 fresh Sol scoped 리뷰를 통과했다. 통합 코드 a2b2d91. OMO·외부tool·전역설정·Go/React는 추가/변경하지 않았다.
- 실제 OpenCode1.18.30 --pure에서 primary2/leaf4/Skill 참조와 permission을 확인했다. 새 leaf는 model/effort 미지정. Codex는 TOML/schema 근거이며 실제 Codex 모델 실행은 별도 미검증이다.
- [동작 검증](docs/superpowers/reviews/2026-09-14-native-specialist-team-verification.md)은 /tmp/threaddock-native-team.pJWGKz의 독립 fixture에서 수행했다. 네 named leaf 호출과 문서/탐색/제안 검토를 확인했으나 최초 구현은 parent가 header-only 조건을 바꿔 FAIL이었다. 원문 packet 전달을 복원한f4220cf(scopedACCEPT, 통합2781c09)와 더 명확한 원본 fixture에서 구현8tests/fresh code review accept를 확인했다. setup 오류와 초기 잘못된7PASS를 보존한다.
- whole-branch native_team_final_review는0bf4461에서 ACCEPT, finding 없음이다. 최종 template-check/paired6역할parse도 PASS. 제품 Go/React·전역설정·현재세션registry는 바꾸지 않았다. 기존 main dirty 문서/중단된 사용자 실험은 보존하고 main 병합은 사람이 수행한다.

## 현재 작업: PR #94·#95·#96 병합

- 사용자 지시로 #94(OpenCode 지원)를 `cad7df16ce4134bf80b0a69820cf0ac0cfa7b005`, #96(Herdr 로컬 Skill)을 `33fc281fb399e2c7454f34151285909d60a54229`에 병합했다. 아래 미게시/미병합 표현은 당시 기록이다. 실제 전역 설치는 수행하지 않았다.
- #95의 원래 head `6f9059d`는 fresh Sol 리뷰에서 BLOCK이었다: stderr scope 문자열 반향, 병렬 진단 순서 비결정성, 옛 TopBar 문구를 찾는 통합 테스트 실패. 첫 `make check`는 Go3 통과 후 UI113/114에서 실패했고 build 단계에는 도달하지 않았다. 원본 로컬 로그는 `/tmp/pr95-merge-make-check.log`, [GitHub 기록](https://github.com/Middleages/thread-dock/pull/95#issuecomment-5658233513)을 따른다.
- Luna test-only 수정 `df9873a`는 GitHub 정상과 Herdr 주의를 각각 확인하고 focused24개를 통과했다. backend `b8deff9`는 안전한 scope allowlist/일반 오류 fallback과 configured target·command 순서의 진단 집계를 적용하고 focused/race 검사를 통과했다. `agent/pr95-merge-check`에 main과 두 수정을 통합했다.
- 최종 코드 `977cde2044c2ce97fa95e599139ab8bb31d18459`에서 `make check`가 Go3 packages·UI14파일114tests·TypeScript/Vite build까지 PASS(exit0)했다. Linux Go1.27.0/Node26.8.1, `VITEST_MAX_WORKERS=1` 환경이며 로그는 `/tmp/pr95-merge-final-make-check.log`다. 원래 실패 tuple은 테스트/backend 수정으로 무효화되어 이 최종 gate를 실행했다. 같은 SHA의 scoped 독립 재리뷰는 ACCEPT, blocker 없음이다. 이후 변경은 이 기록뿐이며 사용자 승인에 따라 PR #95 병합을 진행한다. 실제 병합 SHA는 GitHub PR 기록이 원본이다.
- Windows build/native·live GitHub/GHES/Herdr는 이번 변경에서 미검증이며 과거 #92 검증 결과로 승격하지 않는다. 실제 runtime model/effort identity는 unverified다. 기존 worktree·중단 실험·Windows staging은 보존한다.

## 이전 작업: 폐쇄망 Herdr 보조 Skill

- 승인된 `herdr-local`을 공용 `.agents/skills`에 추가했다. 여섯 canonical workflow는 유지하고 `coordinate-work`·`develop-feature`에서 Herdr CLI가 필요할 때 먼저 호출한다. 설치는 [Quickstart의 폐쇄망 절](docs/operator/github-first-quickstart.md)을 따른다. 사용자 전역 파일은 변경하지 않았다.
- 기준은 OpenCode 지원 branch의 `739d082`이며 독립 branch는 `agent/herdr-local-skill`이다. 구현 `621b0d7`을 `073f7a4`로 통합했다. Monitor·Go/TS·도구별 Agent 모델 설정은 불변이다.
- 검증: helper 생성 전 `make template-check`의 missing-helper 실패(RED), 생성 후 PASS(GREEN), frontmatter·라우팅·gate 순서·canonical/helper 분리 정적 검사 PASS. Quickstart 로컬 링크도 PASS다. 문서·템플릿 검사 변경에 필요한 focused 범위만 실행했고 전체 `make check`·Windows build는 반복하지 않았다.
- 기존 지침만 읽은 7개 dry-run은 대체로 보수적이었다. 이를 행동 실패나 새 스킬의 개선 효과로 주장하지 않는다. 새 스킬은 내장 `--skill` 사용법과 업데이트·원격 운영 제외 범위를 명문화한다. 실제 Herdr state/control·provider 호출·방화벽/egress·Windows 실행은 미검증이다.
- fresh Sol 고정 SHA 리뷰 `073f7a4829ad88ba10b00f965fff7d47e35f91e6`는 ACCEPT, finding 없음이다. 독립 텍스트 적용 8건에서 위험/불명확 요청 6건 차단, 근거 있는 조회·작업 2건 허용을 확인했다. 이후 변경은 이 handoff 기록뿐이다.
- 외부 접속 CLI 없이 로컬 작성·검증만 수행했다. GitHub 게시와 전역 설치는 미수행이며 main은 변경하지 않았다. 지정 Luna high 구현·fresh Sol medium 리뷰의 실제 runtime identity는 unverified다.

## 이전 작업: Codex·OpenCode 템플릿 병행 지원

- [Issue #93](https://github.com/Middleages/thread-dock/issues/93): `agent/opencode-support`에서 OpenCode primary agent 두 개와 [프로젝트·전역 설치 안내](docs/operator/github-first-quickstart.md)를 추가한다. Codex TOML과 공용 `.agents/skills`는 그대로 보존한다. 모델·권한 override와 사용자 전역 설치는 수행하지 않는다.
- 구현 `eed7904`(통합 `2a12c57`)의 frontmatter/기존 prompt 비교 및 격리된 OpenCode `1.18.30 --pure agent list`에서 두 primary 역할 인식을 확인했다. 문서 로컬 링크와 `git diff --check`도 통과했다. 기존 `make template-check`는 Codex·공용 Skill 검사이며 새 OpenCode 검사의 대체 근거가 아니다.
- 문서·설정 전용 diff이므로 전체 `make check`와 Windows build는 재실행하지 않는다. 실제 모델/effort identity, 모델 호출·Herdr 배정·GitHub 업무 수행과 Windows 실행은 새 검증 범위가 아니다. 지정 Luna high 구현 뒤 fresh Sol medium 고정 SHA 리뷰를 진행하며 실제 runtime identity는 unverified다.
- 최종 fresh Sol 리뷰는 `385131f8b8ccb9b8c7ec2474ed5e1e77d72c8426`에서 ACCEPT, finding 없음이다. 동일 템플릿과 공용 Skill을 `/tmp/td-opencode.XtPoc2/project`에 복사하고 `XDG_CONFIG_HOME=/tmp/td-opencode.XtPoc2/config opencode --pure debug skill`의 경로·이름을 검사해 canonical 6개와 shim 4개 인식을 확인했다. 첫 원문 출력 잘림 이후 이름만 추린 확인은 exit 0이었다. 이 절 이후 변경은 검증 기록뿐이며 전역 설치나 모델 실행 결과로 확대하지 않는다.
- PR #92는 사용자 지시로 main `8f03ba6c376f27d1110075297cf21484b059f3d2`에 병합됐다. 아래 Draft 표현은 병합 전 기록이다. Issue #91의 실제 native 검증은 별도로 남는다.

## 이전 작업: compact Monitor와 전역 Toolbox

2026-09-14 사용자 리뷰 보완: conflict/낯선 Herdr 상태 오표시, legacy 합산200 migration 차단, Todo0개 진입점을 수정했다. 코드e1fe17f에서 새 gate(Go3/UI111/build)와 Windows Wails build/임시 파일 storage tests가 통과했고, 후속 최종 리뷰d6aff6b는 ACCEPT다. PR92는 Draft 유지. 아래95tests/Windows953 근거는 이전 기준이며 현재 결과는 최종 ledger/verification의 최신 절을 따른다. 중간 report10개는 추적만 해제하고 로컬/Git 이력을 보존했다.

- PR90은 사용자 지시로 main `611bd6aec528ed63106bd3ef3167c2f9c95e0d01`에 병합됐다. Issue89와 보드는 완료다. 아래 엔진 정리의 미병합 표현은 이전 게시 시점 기록이다.
- 요청 branch `agent/compact-monitor-toolbox`의 기존03df312 이력을 보존하고 main을 merge한 `4c52f64`에서 시작했다. 독립 worktree는 `.worktrees/compact-monitor-toolbox`이며 초기 tree는 main과 동일하다.
- [Issue91](https://github.com/Middleages/thread-dock/issues/91), [지정 설계](docs/superpowers/specs/2026-09-11-compact-monitor-and-global-toolbox-design.md), [지정 계획](docs/superpowers/plans/2026-09-11-compact-monitor-and-global-toolbox.md), [새 ledger](docs/operator/2026-09-12-compact-monitor-ledger.md)를 따른다.
- Tasks1–5와 최종 test manifest·화면 수정 구현은 완료했다. references/global JSON migration, 자료/전역 Drawer, TopBar/Settings/ProjectDetail, Herdr summary가 반영됐고 old Toolbox UI/API는 제거했다. command 실행·Agent 자동 주입·SQLite/새 engine은 없다.
- 최종 `make check`는 `fbefd79`에서 Go3패키지·UI13파일95tests·TypeScript/Vite build PASS다. 첫953db4e gate의 gofmt 실패는 두 줄 정렬 수정으로 해결했다. 같은 전체 suite를 이후 반복하지 않는다.
- Windows 표준 Wails build와 Windows 임시 파일 migration focused tests는953db4e에서 PASS다. 이후 제품 diff는 gofmt 공백뿐이며 정규화 해시가 같다. actual native 앱 조작은 여전히 미검증이고 Windows GPT app 검증을 요청했다. [정확한 검증·artifact 기록](docs/superpowers/reviews/2026-09-13-compact-monitor-toolbox-verification.md)을 따른다.
- Browser fixture는 console0·narrow overflow0, 독립 visual verdict는 `ship`이다. 새 DESIGN.md는 기존 시각 체계를 기록한다. PR 게시·최종 통합 리뷰 상태는 ledger를 확인하며, 새 PR main 병합은 사용자에게 남긴다.
- [Draft PR #92](https://github.com/Middleages/thread-dock/pull/92)를 게시했고 Project1에 연결했다. 최종 독립 통합 리뷰78fd14d는 ACCEPT다. Issue91/PR92는 In Progress이며 남은 실제 Windows 앱 검증 뒤 ready/완료 여부를 판단한다. Task1–5와 옛 엔진 삭제를 다시 구현하지 않는다.

## 과거 기록: 2026-09-12 엔진 제거 재착수

- main을 `2db8f776d22afd849fcb9a1328f7e8aff9b72e50`로 fast-forward했다. PR #82는 `6aced75`, #84는 `c487e8c`에 병합됐다.
- 사용자 추가 PR #85·#86·#87은 settings 저장/UI, GHES host, 열린 업무 중심 화면, Markdown, 프로젝트 Toolbox 및 새 Agent/Skill 구조를 포함한다. 삭제 작업에서 이 기능과 데이터는 보존한다.
- [현재 Agent/Skill 설계](docs/superpowers/specs/2026-09-11-thread-dock-agent-orchestration-design.md)와 [빠른 시작](docs/operator/github-first-quickstart.md)이 Coordinator/Feature Leader, 여섯 canonical Skill과 전역 locator를 설명한다.
- PR #88의 [compact Monitor 설계](docs/superpowers/specs/2026-09-11-compact-monitor-and-global-toolbox-design.md)는 추가 설계다. main에 설계가 있다는 이유로 global Toolbox migration/UI가 구현됐다고 간주하지 않는다. 이번 작업은 UI 재설계가 아니라 옛 엔진 제거다.
- PR #87의 [Toolbox 검증 기록](docs/superpowers/reviews/2026-09-11-project-toolbox-verification.md)과 [Skill 검증 기록](docs/superpowers/reviews/2026-09-11-thread-dock-agent-skill-verification.md)은 실행되지 않은 검증을 명시한다. 아래 `325db89` Windows healthy 증거는 과거 SHA의 증거이지 최신 기능의 검증이 아니다.
- 옛 실행 엔진 전체 제거는 production/test/config/docs 의존성을 증명한 순차 Task로 진행한다. 사용자 실험과 저장된 runtime 데이터는 삭제·이관하지 않는다.
- 현재 작업은 [Issue #89](https://github.com/Middleages/thread-dock/issues/89), [완료 계획](docs/superpowers/plans/2026-09-12-engine-retirement-completion.md), [진행 ledger](docs/operator/2026-09-12-engine-retirement-completion-ledger.md)에서 추적한다. #42/44/45/46/47은 superseded 종료했고 #43/48은 실제 재개·locator 검증으로 재작성했다.

## 제품 기준

### 옛 엔진 제거 구현 완료

독립 branch `agent/engine-retirement-completion`에서 8개 Task의 Luna 구현·fresh Sol 리뷰를 완료하고
옛 소스·전용 테스트·fixture 167개 파일을 제거했다. 현재 Go package는 `monitor`, `internal/runner`,
`internal/projecttemplate`뿐이다. 최신 main `2db8f77`의 Monitor/settings/GHES/Toolbox/Markdown/open-only 코드,
runner·템플릿 검증 Go 코드와 go.mod/go.sum은 그대로다. GitHub template의 문구와 Agent의 stale 지시만 정리했다.
아래 이전 slice에서 보존했던 state/gate/retirement 등은 당시의 중간 보존 범위이며 최종 트리에 남은 엔진이 아니다.
최종 gate·전체 리뷰·PR 상태는 [ledger](docs/operator/2026-09-12-engine-retirement-completion-ledger.md)에 이어 기록한다.
사용자 데이터·기존 worktree·중단된 실험은 보존했다. 이후 PR90은 main `611bd6a`에 병합 완료했다.

최초 통합 gate는 Go 통과 후 UI timing/startup 오류로 실패했다. 지원되는 Node compile cache 환경의
focused 대조에서 유입 main의 오래된 테스트 선택자 한 곳을 발견해 Task9로 수정했다. 제품 코드는
그대로 두고 `monitor.test.tsx`의 row name 탐색만 현재 업무 표에 맞췄다. fresh Sol은 `1e3cbab`에서
ACCEPT했다. 실패와 환경 대조·최종 gate 결과는 ledger에 분리 기록한다.
최종 `1e427ed`의 tmpfs·Node compile cache 환경에서 `make check`가 통과했다: Go3 packages,
UI6파일/37tests, TypeScript/Vite build. Windows/native·실제 GHES/Herdr·Skill pressure scenario는
여전히 별도 미검증이며 #43/#48에서 추적한다. 전체 통합 리뷰·게시 상태는 ledger를 확인한다.
전체 리뷰의 Makefile 오류 예시 두 곳도 수정했고 `5823740`에서 scoped 재리뷰 ACCEPT를 받았다.
`make check` 실행 명령과 검증 입력은 gate `1e427ed`와 같아 기존 통과 근거를 채택했다. 남은 코드/리뷰 blocker는 없다.
[PR #90](https://github.com/Middleages/thread-dock/pull/90)을 main base로 게시했다. engine 제거와 문서·Issue 정리는
이 PR에서 전달했고 사용자 지시로 병합했다. 새 compact Monitor PR의 병합은 별도 사용자 작업이다.

사용자 요구는 **Go 모니터 도구**다. Windows Go/Wails 앱과 기존 React 화면을 유지한다.
“심플하게”는 ThreadDock 자체 실행 엔진을 줄이라는 의미이며 브라우저 전용 전환은 승인되지 않았다.
현재 기준: [ADR 0008](docs/adr/0008-github-first-skills-before-engine.md),
[설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md),
[계획](docs/superpowers/plans/2026-09-10-herdr-first-usable-workflow.md).

## PR #69에서 완료된 상태

- PR #69의 최종 head `f35e248`은 main의 merge commit `3f4bebf`로 병합됐다.
- 2026-09-10 PR #69 직후 재확인에서 `gh auth status`는 `read:project` scope를 포함했고 Projects 조회는 성공했다. 당시 `Middleages`의 Projects와 저장소 ProjectsV2는 각각 0개, 열린 PR은 0개이며 Issue #42~48의 project item도 모두 0개였다. 당시 Wiki도 비활성화 상태였다. 이후 현재 상태는 아래에 따로 기록한다.
- Task 1·2가 Go/Wails `GetMonitorSnapshot` 경로에 GitHub와 Herdr 관찰을 연결했다. 공유 Go/TS wire는 유지한다.
- Task 3가 `monitor/frontend/server/`의 Node adapter·전용 테스트, Vite middleware와 HTTP monitor endpoint를 제거했다. Vite는 React 화면 개발·빌드만 담당한다.
- 화면에는 GitHub 업무·근거, 단일 선택 업무 Herdr 연결, 관찰 세션·미연결 Agent, degraded/notices, 안전한 외부 링크와 handoff 복사만 남겼다. 비기능 상단 메뉴, 자동화 작업, 옛 Work/발행 표시는 제거했다.
- Linux fixture/UI/build 근거와 managed-pane live gh/Herdr 근거는 서로 구분한다. 상위에서 전달된 외부 관찰(원본 transcript 없음)으로 실제 Windows→WSL workstation의 `gh auth status`는 성공했고, bare `wsl.exe --exec herdr`는 PATH lookup에 실패했으며, absolute `/home/appuser/.local/bin/herdr`의 status와 agent list는 성공했다. 이 component 관찰 자체는 Windows→WSL Monitor 읽기 검증이 아니며, 당시 Task 3 문서 worker는 GitHub Issue/PR/Projects/Wiki 쓰기를 수행하지 않았다. 이번 후속 세션의 쓰기는 아래 ledger에 따로 기록한다.
- Final reviewed product/config/dependency SHA는 `325db89`다. Windows native 값은 `THREADDOCK_REPOS=Middleages/thread-dock`, `THREADDOCK_PROJECTS` unset, `THREADDOCK_WSL_DISTRIBUTION=Ubuntu`, `THREADDOCK_SESSIONS_FILE=/tmp/threaddock-aeca770-sessions.json`이다.
- Windows user-local toolchain은 Go `1.27.0 windows/amd64`, Node `26.8.1`, Wails CLI/runtime `v2.15.0`이며, 공식 Go/Node checksum은 제공된 범위에서 일치했다. final SHA의 matching `wails build`는 exit 0, `1m9.285s`에 완료됐고 `monitor\build\bin\ThreadDockMonitor.exe`와 bindings/frontend/assets/app stages `Done`을 확인했다. native child console은 표시되지 않았다.
- Healthy packaged live acceptance는 GitHub 69개 work item의 약 14초 동기화, Herdr 기본 session과 3개 Agent의 약 30초 관찰, handoff 성공 toast와 실제 clipboard 길이 `188`(repository name 포함), 초록 점과 `로컬 연결 정상` 문구의 일치를 확인했다. 앱 종료 후 Monitor process 수는 `0`, `ThreadDockValidation66158d9` scheduled task는 없음, 관련 process도 `0`이었다. WSL interop은 복구됐고 computer-use는 파일이나 worktree를 수정하지 않았다.
- Native run에서 오류가 발생하지 않아 native error/degraded 상태는 검증하지 않았다. 결합 degradation은 fixture/UI 테스트 근거만 있다. `UtilAcceptVsock:281: accept4 failed 110`은 superseded historical diagnostic이고, 이전 `aeca770` plain `go build` 관찰과 `66158d9` staging build는 최종 표준 package evidence가 아니다.
- 기존 Wiki 링크는 문서 참고이며 Wiki 실제 반영과 구분한다.

## PR #70 이후 GitHub 운영 상태

- PR #70은 main의 merge commit `22dcc66`으로 병합됐고 이번 착수 시 열린 PR은 0개였다.
- 저장소는 public이며 Wiki가 활성화됐다. 사용자가 접근을 확인했지만 `thread-dock.wiki.git`의
  `git ls-remote`는 `Repository not found`였으므로 Wiki 페이지 발행은 수행하거나 검증하지 않았다.
- 비공개 사용자 Project [ThreadDock 개발 보드](https://github.com/users/Middleages/projects/1)가 생성됐고
  저장소에 연결됐다. 기본 [Board view](https://github.com/users/Middleages/projects/1/views/2)에는
  착수 당시 9개 항목이 있었으며 Issue #42~48은 `Todo`, PR #69·#70은 `Done`이었다.
- `정리 방향` 필드는 #43·#48이 재작성 후보, 나머지 Issue가 superseded 종료 후보인 원문 값으로
  확인했다. CLI의 한글 field key 표시 문제를 제품 버그로 판정하지 않는다.
- GitHub Projects 쓰기는 `project` scope로 성공했다. 보드 업무 상태는 GitHub가 원본이고,
  Herdr의 session·Agent 상태와 관찰 시각은 별도 실행 원본이다.

## 여덟 번째 엔진 정리

[Issue #83](https://github.com/Middleages/thread-dock/issues/83)에서 미사용 retirement alias/constructor와
bulk cleanup 후보 조회 API를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-unused-cleanup-api.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice8-ledger.md)에 caller·보존 경계·검증 결과를 기록한다.
exact-ID retire/cleanup·7일 guard·현재 composite inspector·state persistence/ListRecoverable는 보존한다.
당시 PR #82의 `00cd561` head 위에서 작업했다. 이후 사용자 지시로 #82를 병합하고 #84의 base를 main으로
전환한 뒤 `c487e8c4076d9ea804b096213cc8728a6b17e919`에 병합했다. Issue #83과 보드는 완료 상태다.
제품 세 파일의 미사용 API·전용 테스트 60줄 삭제를 완료했다. Task `a1e7da3` 및 전체 통합 `bfbd6dc`는
fresh Sol ACCEPT다. 같은 코드 `23906ad`의 tmpfs checkout에서 최종 `make check`가 Go 전체·UI 26개·build까지
통과했다. 앞선 UI startup/threads 실패와 환경 대조는 ledger에 기록했으며 정확한 환경 병목은 미확정이다.
Windows/native와 live gh/Herdr 제품 E2E를 새로 검증한 결과는 아니다.
[PR #84](https://github.com/Middleages/thread-dock/pull/84)를 `agent/engine-retirement-slice7` base로 게시했다.

## PR #82의 일곱 번째 정리 결과

[Issue #81](https://github.com/Middleages/thread-dock/issues/81)에서 미사용 backend ConfirmProtectedChange
진입점 두 개를 제거한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-backend-confirm.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice7-ledger.md)에 caller·mixed-test·state 보존 경계와 검증 결과를 기록한다.
현재 protected state/gate/comment/event와 저장된 confirmation 상태의 대기·재개 검증은 유지한다.
Task는 `ec80575`에서 ACCEPT했다. 최초 gate의 UI worker startup timeout을 기록했고 source/config 변경 없이
`VITEST_MAX_WORKERS=1` 환경의 `d40b693` gate에서 전체 Go·UI 26개·frontend build가 통과했다.
정확한 환경 병목은 미확정이다. 전체 branch 리뷰는 `f44082c7bcc8cab6cbb1fc75a62b92e14a736cca`에서
ACCEPT했으며 남은 finding은 없다. [PR #82](https://github.com/Middleages/thread-dock/pull/82)는
`6aced75a44d01b36f9ac632750609cd80c3dbe7a`에 병합됐다. Issue #81과 보드는 완료 상태다.

## PR #80의 여섯 번째 정리 결과

[Issue #79](https://github.com/Middleages/thread-dock/issues/79)에서 옛 confirm CLI adapter/wiring을 제거한다.
[계획](docs/superpowers/plans/2026-09-11-engine-retirement-confirm-cli.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice6-ledger.md)에 계약·보존 경계·검증 상태를 기록한다.
orchestrator의 protected-change 구현·상태와 나머지 CLI는 보존한다.
CLI Task는 `7234f66`에서 ACCEPT했다. 최초 gate `80c9a0d`에서 기존 동시 Advance 테스트의
동기화 결함을 발견해 테스트만 수정했고, 20회 반복·잠금 테스트 및 `eb62e6e`의 독립 리뷰를 통과했다.
수정 후 `98861c8`의 gate는 전체 Go·UI 26개·frontend build까지 통과했다. 제품 lock 구현은 불변이며
전체 리뷰는 `60df27dbbcf69c901caa39d4e832f69bfe1bb14c`에서 ACCEPT했다. 남은 blocking 사항은 없다.
PR #78을 main에 병합한 뒤 PR #80의 base를 main으로 바꿔 병합했다. 사용자 지시로 수행했으며
PR #80 merge SHA는 `f9fb7256e625e116493729c5accc390a2b8ec953`다. Issue #79와 보드 항목도 완료로 갱신했다.

## PR #78의 다섯 번째 정리 결과

[Issue #77](https://github.com/Middleages/thread-dock/issues/77)은 호출자가 없는 옛 `internal/integration`
구현과 전용 테스트를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-v1-integration.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice5-ledger.md)에 정확한 경계와 검증 상태를 기록한다.
현재 `internal/workrun.ReviewIntegrationService`, shared Git/state/CLI 및 Monitor는 유지한다.
Task 리뷰는 `6a4bb17`에서 ACCEPT했고 통합 `e1efc5d`의 단일 tmpfs make check는
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch 리뷰는
`3022baba83bebcb2a77b3f895100829b95d77a7b`에서 ACCEPT했으며 남은 finding은 없다.
[PR #78](https://github.com/Middleages/thread-dock/pull/78)은 `13b20d3dc1e75f55016dadb87993884402ee8088`에 병합됐다.
Issue #77은 닫혔고 보드의 Issue/PR도 Done으로 갱신했다.

## PR #76의 네 번째 정리 결과

[Issue #75](https://github.com/Middleages/thread-dock/issues/75)에서 미사용 worktree revert API 여섯 개와
전용 테스트를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-worktree-revert.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice4-ledger.md)에 소유 경로·호출 근거·검증 상태를 기록한다.
사용 중인 CreateManagedWorktree/PushBranch, conflict/path helper와 PushBranch의 no-force 검증은 유지한다.
Task 리뷰는 `537bda2`에서 ACCEPT했고 통합 `7a620c4`의 단일 tmpfs `make check`는
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch 리뷰는
`acfbf7be0c22fd333874f1e26539debb74abfd12`에서 ACCEPT했으며 남은 finding은 없다.
[PR #76](https://github.com/Middleages/thread-dock/pull/76)은 사용자 지시로
`5f5b31e519074f599a1d951cf6c7278e88a4cfef`에 병합됐다. Issue #75는 닫혔고 보드의 Issue/PR도 Done으로 갱신했다.

## PR #74의 세 번째 정리 결과

[Issue #73](https://github.com/Middleages/thread-dock/issues/73)은 PR #72 이후 호출자가 없어진
GitHub safe-draft API 세 개와 전용 테스트를 정리한다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-safe-draft.md)과
[ledger](docs/operator/2026-09-11-engine-retirement-slice3-ledger.md)에 정확한 경계와 검증 상태를 기록한다.
사용 중인 DraftPR 요청 타입·크기 제한·PR 생성/조회 기능은 유지하고 worktree helper 정리는 후속으로 남긴다.
Task는 `fea410c`에서 독립 리뷰 ACCEPT를 받았고 통합 `0325d41`의 단일 tmpfs `make check`는
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch 리뷰는
`bc1959226b3abd88f21d9ba92863fcf5e7be937d`에서 ACCEPT했으며 남은 finding은 없다.
[PR #74](https://github.com/Middleages/thread-dock/pull/74)는 사용자 지시로
`b2acf4c8f15c48901a0bfd0e6e50800b89a215f9`에 병합됐다. Issue #73은 닫혔고 보드의 Issue/PR을 Done으로 갱신했다.

## PR #72의 두 번째 정리 결과

[Issue #71](https://github.com/Middleages/thread-dock/issues/71)의 범위는 옛 `create-revert` CLI 경로와
전용 adapter/service다. [계획](docs/superpowers/plans/2026-09-11-engine-retirement-create-revert.md)과
[진행 ledger](docs/operator/2026-09-11-engine-retirement-slice2-ledger.md)에 정확한 소유 경로·공유 계약·검증 상태를 기록한다.
제거 명령은 production 설정·인증·저장소 조회 없이 기존 usage/exit 2로 끝나는 계약이다.
현재 Monitor·runner·공용 Git/worktree 및 나머지 CLI 동작은 보존한다.
Task 리뷰는 수정 head `2f5c841`에서 ACCEPT했고, 통합 SHA `be4b44b`의 tmpfs `make check`가
전체 Go·UI 26개·frontend build까지 통과했다. 전체 branch의 fresh Sol 리뷰는
`61745bac6f8500b3239a73314f0b26a3691bc2c3`에서 ACCEPT했으며 남은 blocking 사항은 없다.
[PR #72](https://github.com/Middleages/thread-dock/pull/72)는 사용자 지시로 main의
`a2f35f67ce58da5d787bd4c03165e9292624e15b`에 병합됐다. Issue #71은 닫혔고 보드의 Issue/PR도
Done·완료 근거로 갱신했다. 후속 작업은 남은 revert helper의 호출 여부를 별도로 확인해 정한다.

## PR #70의 첫 정리 결과

첫 작업은 [의존성 inventory](docs/operator/2026-09-10-engine-dependency-inventory.md),
[Issue 분류](docs/operator/2026-09-10-engine-retirement-issues.md),
[첫 삭제 계획](docs/superpowers/plans/2026-09-10-engine-retirement.md),
[검증·GitHub ledger](docs/operator/2026-09-10-engine-retirement-ledger.md)에서 추적한다.
현재 Monitor의 `internal/runner` 직접 사용을 보존하고 Linux/Windows 그래프에서 importer가 없는
옛 `internal/monitorcli` 두 파일을 제거했다. Luna focused 검사와 fresh Sol Task 리뷰를 통과했으며,
통합 SHA `ff977a2`의 첫 `make check`는 ext4 `/tmp`에서 fixture fsync 누적으로 timeout 실패했다.
코드·테스트·Makefile·dependency를 바꾸지 않고 `TMPDIR`만 `/dev/shm`으로 옮긴 같은 SHA의 새 tuple은
shell·gofmt·vet·전체 Go·UI 26 tests·frontend build까지 통과했다. 전체 branch 리뷰의 기존 BLOCK은
gate 미완료와 ledger 근거 누락 두 건이었으며, 문서 수정 후 fresh Sol의 scoped 재리뷰는
`ca2cae680a627aee222f2279d866fa5b24e52ee7`에서 ACCEPT를 받았다.
build가 제거한 tracked `dist/.placeholder`는 원본 바이트로 복원했고 `git diff --exit-code`로
`ff977a2` 코드 트리와 동일함을 확인했다. 통합 branch의 `58a59a6`은 재리뷰 SHA와 전체 tree가 동일하다.
현재 blocking 사항은 없으며 나머지 engine은 v1/v2 호출 군집으로 남는다.
Issue #42·44·45·46·47은 superseded 종료 후보, #43·48은 새 방향 재작성 후보로 한국어 댓글을 게시했다.
기존 본문·열린 상태는 보존했다. 당시 Projects 조회 성공/0개와 Wiki 비활성 상태는 이후 생성·활성화
이전의 과거 근거이며, 현재 보드와 Wiki 상태는 위 운영 상태가 기준이다.

## 남은 제품 검증과 후속 구현

현재 계획 Task 1·2 구현, Task 3 정리와 Wails runtime `v2.15.0` alignment를 반영했다. Linux 통합
`make check`는 final reviewed SHA `325db89`에서 통과했다. 표준 Windows Wails build와 healthy
packaged live 화면, Windows→WSL read path, clipboard/status 및 process/task cleanup도 확인됐다.
남은 제품 검증 범위는 native 오류 상태의 실제 재현이며, fixture/UI degradation 근거를 native live
성공으로 확대하지 않는다. 독립 기능 두 개를 Issue·Projects 기록과 함께 끝까지 운용하는 완료 기준도
아직 검증하지 않았다. `325db89`의 같은 Linux gate tuple은 반복하지 않으며, 후속 code PR은 마지막
통합 `make check`를 새 SHA에서 한 번 수행한다.
Vite는 화면 개발·빌드에 남긴다. Go를 없애거나 브라우저 제품으로 다시 전환하지 않는다.
옛 엔진 제거 구현은 위 완료 범위와 ledger를 따른다. 남은 native/E2E 검증은 엔진 제거 완료와 별개다.

Windows의 Go→WSL 호출은 기존 Herdr pane 환경을 자동 상속하지 않는다.
실제 읽기 접근을 검증하고 환경값을 위조하지 않는다. 접근 실패는 명시적 blocker이며 새 실행 엔진을 만들 이유가 아니다.

## 보존

사용자 호스트 /home/appuser/dev_system/.worktrees/codex-runtime의 중단된 실험과 미커밋 파일은 보존한다.
사용자 기록의 cmd/agentctl/main.go, cmd/agentctl/main_test.go, internal/config/config.go,
internal/config/config_test.go를 reset·삭제·commit하거나 실험을 재개하지 않는다.
Herdr #3813 해결을 가정하지 않고 /tmp/threaddock-herdr-live.7YEhfV의 실패 invocation을 재사용하지 않는다.
PR #69는 Go 경로 이식과 Node 경로 제거를 포함해 이미 main에 병합됐다. 후속 PR의 main 병합은 사용자에게 남긴다.

## Codex 다음 세션 프롬프트

~~~text
/home/appuser/dev_system의 ThreadDock 작업을 이어가.
실제 Git 상태와 origin/main을 확인하고 미커밋 실험을 보존한 독립 worktree에서 작업해.
AGENTS.md, HANDOFF.md, PRODUCT.md, CONTEXT.md, ADR 0008,
2026-09-11 compact-monitor-and-global-toolbox 설계·계획과 운영 문서를 읽어.

제품은 Go/Wails 데스크톱 모니터와 기존 React 화면이다. 브라우저 전용으로 바꾸지 마.
Go가 GitHub·Herdr 조회·결합을 담당하고, Herdr가 세션 실행을, Agent와 Skills가 개발·기록을 맡아.
Task 1·2의 Go/Wails GitHub·Herdr 경로와 Task 3의 Node 경로 제거가 반영되어 있다.
PR #70의 monitorcli 제거와 PR #72의 create-revert CLI/service 제거는 main에 병합됐다.
PR #74의 미사용 GitHub safe-draft 제거와 PR #76의 worktree revert 제거도 병합됐다.
PR #78·#80·#82·#84와 최종 엔진 제거 PR90도 main에 병합됐다. 확인된 merge는 `611bd6a`다.
최신 settings/GHES/Toolbox와 여섯 canonical Skill을 보존한다. 2026-09-11 추가 설계·검증 제한도 읽는다.
현재 Project는 https://github.com/users/Middleages/projects/1 이다.
옛 CLI·엔진과 전용 fixture 제거는 완료됐다. Issue91의 compact Monitor 구현을 요청 branch
agent/compact-monitor-toolbox와 독립 worktree .worktrees/compact-monitor-toolbox에서 이어간다.
2026-09-12-compact-monitor-ledger.md와 .superpowers/sdd의 Task packet/report를 확인한다.
현재 HANDOFF/ledger의 PR·검증·병합 상태를 먼저 확인하고 완료된 삭제를 반복하지 마.
Linux 통합 gate와 표준 Windows package/live acceptance는 reviewed SHA `325db89`에서 완료됐다.
healthy native run은 GitHub 69개 work item, Herdr 기본 session/3 agents, clipboard 188, `로컬 연결 정상`,
process/task cleanup을 확인했으며 native error-state만 unverified다.
Windows→WSL Herdr 읽기 접근은 실제 설치 조건으로 확인하고 HERDR_ENV를 임의 설정하지 마.
새 scheduler/runtime/Publisher나 로컬 Work 계약을 만들지 마.
Wiki는 활성화됐지만 페이지 발행은 아직 검증되지 않았다. 완료된 엔진 삭제를 재개하지 마.
현재 새 기능의 Windows build/native smoke는 과거 SHA의 결과로 통과 처리하지 마.

Sol medium이 작은 Task를 계획·분배하고 Luna high가 구현해.
독립 작업만 worktree로 병렬화하고 고정 변경은 fresh Sol medium이 검토해.
수정 범위 focused 테스트를 사용해. `325db89`에서 통과한 같은 Linux 통합 tuple은 반복하지 말고,
후속 code PR의 마지막 `make check`만 새 통합 SHA에서 한 번 실행해. root-owned Windows Wails
build/live evidence와 component CLI 관찰 및 fixture 결과를
구분해. native error-state는 아직 unverified이며, 과거 `UtilAcceptVsock:281: accept4 failed 110`은
superseded diagnostic으로만 기록한다.
PR·Issue는 한국어로 작성하고 main 병합은 나에게 남겨.
~~~
