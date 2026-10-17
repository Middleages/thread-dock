# Superpowers 비활성화와 review evidence helper 검증

## Task packet

- `taskId`: `review-evidence-helper`
- `baseSHA`: `01a82fd509842c4eae568280ff72495faf9416ba`
- `deps`: 기존 `review-change`의 substantive code-review 계약, Pi 연구의 native event receipt 원리, A·B planning completion contract
- `ownedPaths`: `project-template/.agents/skills/review-change/{SKILL.md,references/review-evidence-receipts.md,scripts/summarize_review_evidence.py,tests/test_summarize_review_evidence.py}`, 이 검증 기록, HANDOFF와 Pi 연구의 짧은 적용 상태
- `worktree`: `/home/appuser/dev_system/.worktrees/review-evidence-helper`
- `branch`: `agent/review-evidence-helper`
- `forbiddenPaths`: 제품 Go/Wails·React, DB/session adapter, hook/server/scheduler, Pi 설치, model/provider/permission, 다른 Skill/role. Superpowers config는 root가 별도 변경했고 이 branch에서 편집하지 않았다.
- `interface`: `summarize_review_evidence.py --format auto|codex-jsonl|opencode-jsonl|opencode-export <single-file>`가 한 user-supplied regular artifact만 읽고 stdout JSON metadata를 반환한다. 명시된 `td_reviewer`, child ID, native lifecycle/result field 위치만 receipt로 취급한다. prompt/output 문자열, parent self-claim, outer success, empty wait, non-review task를 review proof로 쓰지 않는다.
- `acceptance`: read-only/std library/no network·DB·model·spawn·write·recursive scan. Malformed/unsupported/truncated는 error/unknown과 nonzero. 관찰 metadata는 authenticated attestation이나 review quality/readiness/revision 증명이 아니다. Helper는 optional collector이고 `review-change`가 fixed candidate와 substantive result를 계속 판정한다.
- `tests`: Luna RED/GREEN focused unittest, integrated unittest/py_compile, 실제 Codex/OpenCode JSONL smoke, Skill/template/link/global parity. 제품 전체 suite는 범위 밖이다.
- `result`: worker commits `2bcb049`, `2bb7e4a`, review-fix `3a05d922a82431ea01bd2de62ff6aeb6b854fee9`, source/docs commit `531c5de2e28e72128223c90db43c5c3ca5141806`. `changedFiles`는 owned paths다. `unverified`는 Codex raw CLI가 직렬화하지 않은 child role/result, runtime receipt authenticity, already-open session reload, Windows/GitHub/Herdr/product behavior다. 최종 review는 아래에 기록한다.

## 구현과 설치

Luna `td_implementer`가 독립 worktree/branch에서 helper와 tests를 작성했다. 첫 task review에서 실제 Codex nested `item.collab_tool_call` 미지원, OpenCode non-task false positive, parent ID 누락을 BLOCK했고 같은 Luna가 세 항목만 수정했다. helper는 raw prompt/result 내용을 출력하지 않고 source path, JSONL line 또는 export pointer, explicit parent/child/role/status/result field location과 limitations만 출력한다.

Linux global `/home/appuser/.agents/skills/review-change`는 기존 `SKILL.md` hash를 재확인하고 `/tmp/threaddock-review-evidence.WhfO4x/before/review-change/SKILL.md`에 백업했다. tracked `SKILL.md`, reference, script만 설치하고 parity를 확인했다. tests는 개발 source에만 남겼다. 다른 global Skill/config/hook은 변경하지 않았다.

## Focused verification

- Luna RED: helper 부재로 8 tests 실패.
- Luna GREEN: 최초 9 tests 통과, review fix 뒤 10 tests 통과; `py_compile`, diff check PASS.
- 통합 후 같은 10 tests가 4.553초에 PASS했고 `py_compile`, `git diff --check`도 PASS했다.
- 실제 `/tmp/threaddock-opencode-behavior.KN5mJM/delegation-events.jsonl`은 parent `ses_ebb1d3cd7ffe1rSPMBeAkLQ1jP`, child `ses_ebb18dc67ffeC5NfwEnVvwzmrr`, explicit `td_reviewer`, completed, `/part/state/output` at line 6으로 `sufficient`를 반환했다. 이는 supplied artifact metadata sufficiency일 뿐 verdict 진위나 reviewed revision 증명이 아니다.
- 실제 `/tmp/threaddock-codex-behavior.XnLnZi/delegation-recorded-events.jsonl`과 아래 no-plugin significant JSONL은 parent/outer completion만 보고 `review_receipts=[]`, `partial`, `reviewer_receipt_not_observed`를 반환했다. 실제 child가 다른 state DB에 있어도 raw CLI artifact에 role/spawn receipt가 없으므로 helper는 추측하지 않는다.
- malformed, unsupported, missing child/role, empty wait, running/failed, self-claim, non-task state, nested prompt/output 문자열을 false receipt로 만들지 않는 회귀 tests가 포함된다.

`make template-check`와 Skill frontmatter/reference/global parity 검사를 final candidate에서 수행한다. 제품 code PR이 아니고 Go/React를 변경하지 않아 통합 `make check`, UI/browser/Wails suite는 실행하지 않는다. 이는 성공으로 간주한 생략이 아니라 isolated Skill helper의 검증 한계다.

## Superpowers 비활성화 관찰

Root가 `/home/appuser/.codex/config.toml`의 `plugins['superpowers@openai-curated-remote'].enabled`만 `true`에서 `false`로 변경했고 `/home/appuser/.codex/backup-superpowers-disabled.Zmgsn7/config.toml`에 백업했다. 변경 전후 parse와 나머지 설정 동일성이 PASS했으며 uninstall, cache 삭제, hook/model/permission 변경은 없었다.

`codex debug prompt-input`의 격리 출력에는 Superpowers catalogue entry와 `using-superpowers/SKILL.md` 참조가 0이었고 ThreadDock `coordinate-work`/`develop-feature`는 남았다. fresh ordinary root `01a149d1-b042-7723-9647-4dcbdb3aa3cf`는 README만 읽고 한 문장으로 답했으며 Skill/child/수정이 없었다.

fresh significant planning root `01a149d2-bb14-7ab0-9307-28be8469343b`는 기존 ThreadDock lifecycle을 수행해 `td_reviewer` child `01a149d3-7ea9-7db0-8c77-794a7141c502`를 실제 실행하고 owner resolution 뒤 reviewed/not-ready를 구분했다. 두 thread의 observed runtime metadata는 기존 `gpt-6-astra`/`medium`이었다. 그러나 raw JSONL에서 모델이 설치된 Superpowers cache의 `using-superpowers`와 `writing-plans` 절대 경로를 직접 읽었다. persistent rollout의 developer catalogue와 host skills body에도 15개 Superpowers 항목이 남아 있었고 local debug renderer와 runtime host injection이 달랐다. 따라서 **local plugin config/catalogue 비활성은 확인했지만 이 runtime에서 Superpowers 지침이 완전히 사용되지 않았다고 보장할 수 없다.** 원인을 모델 기억으로 추정하거나 cache/설치를 삭제해 우회하지 않았다.

중요 run의 per-run usage는 input 335,284 / cached 296,192 / output 4,018이다. 이전 enabled run과 단일 사례로 비용·품질 인과를 주장하지 않는다. fixture raw trace는 `/tmp/threaddock-superpowers-off-significant-20261017/events.jsonl`, root ordinary trace는 `/tmp/threaddock-superpowers-off.VsCRD4/events.jsonl`에 있다.

## 독립 리뷰

최종 고정 SHA의 helper, tests, Skill 경계, 설치/검증 근거와 Superpowers 한계를 fresh `td_reviewer`가 검토한 결과를 여기에 기록한다.
