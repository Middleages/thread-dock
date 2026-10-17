# 중요 계획 완료 계약 A·B 검증

## Task packet

- `taskId`: `plan-completion-contract-ab`
- `baseSHA`: `5295ec42d96983a264f31bdbc69b3c3a4c72f446`
- `deps`: 기존 `develop-feature`, `grill-plan`, `td_feature_leader`, 고정 reviewer mode와 2026-10-16 행동 보고서
- `ownedPaths`: `project-template/.agents/skills/{develop-feature,grill-plan}/SKILL.md`, Codex/OpenCode `td_feature_leader`, `project-template/AGENTS.md`, quickstart, Agent workflow 문서, 이 검증 기록과 HANDOFF. Pi 연구 문서는 별도 조사 commit을 그대로 통합하고 최신 적용 결과 한 줄만 연결한다.
- `worktree`: `/home/appuser/dev_system/.worktrees/plan-completion-contract`
- `branch`: `agent/plan-completion-contract`
- `forbiddenPaths`: Go/Wails·React 제품 코드/테스트, hooks, 새 DB/schema/helper, model/provider/permission, Herdr/GitHub 상태
- `interface`: 중요한 새 계획/실질적 위험은 고정 revision → 실제 fresh native plan-review → owner resolution 뒤에만 planning complete다. `reviewed`와 `implementation-ready`는 별도다. 기존 response/handoff 한 줄에 revision, 실제 reviewer result/verdict, 남은 중요 항목, readiness를 남긴다.
- `acceptance`: planning-only는 구현·게시 금지이지 review 생략이 아니다. 명시적 ideas/draft, 작은 명확 작업, 유효한 기존 review만 전체 cycle을 생략한다. 부족한 사실도 assumptions/options는 검토하고 실제 불가능 prerequisite만 정확히 남긴다. neutral adjudicator는 owner와 reviewer의 중요 이견이 남을 때 한 번만 사용한다.
- `tests`: template/parse/link/parity와 이름 없는 fresh root Codex 3건, 기존 기본 모델의 OpenCode 핵심 1건. 이전 전체/UI suite는 반복하지 않는다.
- `result`: A·B source commit `ae11350405657ac705a33b781dfce77999396e09`; `changedFiles`는 위 owned paths와 별도 Pi 연구 문서다. `unverified`는 OpenCode parent의 reviewer result 회수/최종 readiness, 이미 열린 session 재로딩, Windows·GitHub·Herdr·제품 동작이다. 최종 reviewer 결과는 아래에 추가한다.

## 변경과 Linux 설치

`develop-feature`와 `grill-plan`에 완료 조건을 두고 Codex/OpenCode Feature Leader 역할에는 이를 놓치지 않을 짧은 pointer를 넣었다. generic planning helper는 작성 보조일 뿐 owner나 독립 검토를 대체하지 않는다. 별도 상태 파일·identifier schema·scheduler·hook은 만들지 않았다.

Linux 전역 설치 전 네 파일의 hash가 tracked 이전판과 일치함을 재확인했다. 백업은 `/tmp/threaddock-plan-contract.0AHAIa/before`이며 다음 설치본을 tracked source와 맞췄다.

- `/home/appuser/.agents/skills/develop-feature/SKILL.md`
- `/home/appuser/.agents/skills/grill-plan/SKILL.md`
- `/home/appuser/.codex/agents/td_feature_leader.toml`
- `/home/appuser/.config/opencode/agents/td_feature_leader.md`

기존 model/effort/provider/permission과 `/home/appuser/.codex/hooks.json`은 변경하지 않았다. 설치 parity는 현재 파일을 증명하지만 이미 열린 session의 재로딩을 뜻하지 않는다.

## 이름 없는 fresh root 행동 시험

Fixture와 raw JSONL/stderr는 `/tmp/threaddock-plan-contract-tests-20261017`에 보존했다. 프롬프트는 Skill/reviewer 이름을 쓰지 않았고 구현·파일 수정·외부 기록·프로젝트 session을 금지했다. Codex는 기존 설정으로 `exec --json --sandbox read-only`를 사용했으며 independent review case는 `--ephemeral`을 쓰지 않았다. OpenCode는 기존 `opencode/big-pickle`과 plugin을 유지하고 테스트 자식 환경에서 Herdr pane 변수만 제거했다.

| case | actual evidence | verdict |
|---|---|---|
| Codex, 충분한 JSON→SQLite planning-only | parent `01a149b9-6f64-7470-86e3-07c75250ca61` → child `01a149ba-55f0-7863-afcb-b1cdac0a44b7`, role `td_reviewer`; 둘 다 `gpt-6-astra`/`medium`. P1 조건부 review의 두 finding을 owner가 P2에 반영하고 neutral을 호출하지 않았다. 마지막에 P1 review→P2 반영 관계, 남은 v1 계약/환경 근거, reviewed지만 착수 조건 미충족을 기록했다. usage input 369,712/cached 331,008/output 4,121 | PASS |
| Codex, 명시적 small ideas draft | child/tool delegation 없음. `미검토 아이디어 초안`, alias/release 결정 미정, 완성 계획 아님을 표시하고 파일 불변. usage input 57,031/cached 18,688/output 398 | PASS: 불필요 review 없음 |
| Codex, 중요한 계획이나 source 부족 | parent `01a149bd-03db-7163-8bd3-655dc27283f2` → child `01a149bd-bcac-7202-8ccc-7ffe2c5055e0`, role `td_reviewer`; 둘 다 `gpt-6-astra`/`medium`. assumptions/options를 R1로 검토하고 세 finding을 R2에 반영했다. 제품·기술 미결 항목을 소유자별로 남기고 `reviewed, implementation-ready 아님`으로 종료했다. usage input 224,916/cached 174,464/output 2,945 | PASS |
| OpenCode, 동일한 충분한 planning-only | root `ses_eb640d4faffeD73bSMmK06JKwU`가 자동으로 `develop-feature`를 읽고 `task(subagent_type=td_reviewer)`를 호출해 child `ses_eb63926aeffeHgn9EExxfvJQRr`를 만들었다. parent/child 모두 기존 `opencode/big-pickle`. 그러나 넓은 `/` 탐색이 먼저 120초 timeout됐고 제한된 전체 대기 안에 child verdict 회수와 parent final이 없었다. 테스트 프로세스만 종료해 exit 130; 파일·전역 설정은 바꾸지 않았다. | INCOMPLETE: 자동 child 시작은 확인, 완료 계약 준수는 미검증 |

Codex parent-child 관계와 model metadata는 `/home/appuser/.codex/state_5.sqlite`에서 해당 두 parent ID만 read-only 조회했다. OpenCode는 `opencode db`로 해당 session/child와 running task만 조회했다. 문구만으로 independent review를 성공 처리하지 않았다. OpenCode 결과는 review 미수행의 반증도, 완료 성공의 근거도 아니다.

## 정적 검증과 도구화 판단

실행한 focused 검사는 `git diff --check`, Codex TOML parse, tracked↔Linux global 네 파일 `cmp`, local link 존재 확인, `make template-check`다. 모두 exit 0이었다. fixture git 상태도 입력 파일 외 변경이 없었다. 제품 코드가 없어 `make check`, UI/browser/Wails 검증은 실행하지 않았다.

Codex 두 중요 case에서 수작업 receipt 수집 비용은 실제로 컸지만 한 번의 변경 검증이며, OpenCode trace도 아직 완결되지 않았다. 따라서 이번에는 helper/hook을 추가하지 않는다. 반복 누락이나 반복 수집 비용이 운영에서 확인될 때만 Pi 연구가 설명한 typed native event receipt를 read-only 후보로 검토한다.

## 독립 리뷰

최종 고정 SHA의 source, Pi 문서 통합, 실제 trace와 한계를 fresh `td_reviewer`가 검토한 결과를 여기에 기록한다.
