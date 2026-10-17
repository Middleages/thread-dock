# Herdr Projects와 ThreadDock 비교 조사

조사일은 2026-10-17이며, upstream `main`의 고정 commit
[`963d81b4523f725ee028b893521811afd72d1369`](https://github.com/eliasstravik/herdr-projects/tree/963d81b4523f725ee028b893521811afd72d1369)을
기준으로 README·공식 문서·Rust 소스를 읽었다. 설치, remote script 실행, live Herdr 조작은 하지 않았다.

## 결론

Herdr Projects는 “Herdr용 관제 화면”보다 넓다. 로컬 `task.json`을 board가 소유하는 업무 원본으로 두고,
Coordinator 입력부터 worktree 생성, commit·main 동기화·check, 별도 reviewer, PR 승인·병합·정리까지 수행하는
자체 orchestration engine이다. README도 Herdr를 engine이라고 소개하지만, 실제 소스에서 board가 이 lifecycle을
직접 소유한다([README 17행](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/README.md#L17),
[store 원본 선언](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/core/src/store/mod.rs#L1-L3),
[flow 모듈 구성](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/lib.rs#L1-L36)).

따라서 GitHub를 durable work truth, Herdr를 top-level 실행 lifecycle, native leaf를 feature 내부 실행 수단으로 두고
Go/Wails가 둘을 읽어 결합하되 scheduler·승인·실행 engine이 되지 않는 현재 ThreadDock
([agent workflow](../architecture/agent-workflow.md), [PRODUCT](../../PRODUCT.md))에 통째로 채택할 수는 없다.
하지만 “Skill을 읽었다”는 말에 의존하지 않고 고정 revision·검사·독립 review·merge 조건을 기계적으로 연결한
방식은 사용자가 반복해서 우려한 지침 미준수를 실질적으로 줄인다. 구조 차이를 이유로 이 효용을 폄하하면 안 된다.

## 실제 구현으로 확인한 lifecycle

- task의 board-owned state는 `~/.herdr-projects/data/tasks/<repo>/<n>/task.json`, 메시지는 `inbox.jsonl`, 근거는
  `artifacts/`에 저장된다. Herdr agent status는 live로 읽어 합성한다
  ([store](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/core/src/store/mod.rs#L1-L3)).
  화면 상태는 저장 stage와 Herdr status를 한 함수에서 `idle/working/blocked/done/error`로 매핑한다
  ([state mapping](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/state.rs#L1-L23),
  [stage 분기](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/state.rs#L61-L84)).
- repo task는 별도 Herdr space·worktree·branch에서 실행되고, finish line이 잔여 변경 commit, main 동기화, check,
  draft PR, reviewer, 사람 승인, merge·cleanup을 순서대로 수행한다
  ([공식 lifecycle](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/how-work-gets-done.md#L1-L27),
  [finish line](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/how-work-gets-done.md#L88-L128)).
  `review.rounds` 기본값 3 안에서 check·review·conflict 실패를 worker에게 돌려보내며, 한도를 넘으면 사람 또는
  fallback으로 넘긴다
  ([review loop](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/finish/pipeline_end.rs#L84-L120)).
- 다만 “모든 완료 작업을 두 번째 agent가 새로 review한다”는 README 표현에는 예외가 있다. 이미 통과한 revision에
  board가 main만 깨끗하게 merge했고 check가 통과하면 기존 review를 유지할 수 있다
  ([공식 예외](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/how-work-gets-done.md#L238-L253)).

## review와 권한

- reviewer는 `verdict.json`에 reviewed commit, findings의 severity·proof, 실제 checks, limitations를 남긴다.
  다른 commit의 verdict는 무시한다
  ([review contract](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/finish/review_brief.rs#L38-L53),
  [commit 확인](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/finish/review_brief.rs#L56-L65)).
  reviewer가 merge 여부를 정하지 않고 board가 `review.mustFix` severity gate를 적용한다. PR merge 직전에도 remote
  head가 verified head와 다르면 거부하고 `gh pr merge --match-head-commit`을 사용한다
  ([merge 결속](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/land/merge.rs#L148-L173)).
- 기본 access는 worker·reviewer·Coordinator 모두 full이며 Codex는
  `--dangerously-bypass-approvals-and-sandbox`로 시작한다. 문서도 rules와 review가 sandbox가 아니라고 명시한다
  ([권한 문서](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/how-work-gets-done.md#L34-L47)).
  `*.access ask`를 선택할 수 있지만, 기본값 자체가 ThreadDock의 기존 승인·permission 상속 원칙보다 훨씬 넓다.
- README의 “자신의 agent settings에는 아무것도 쓰지 않는다”는 문구는 그대로 받아들이면 틀리다
  ([README 97행](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/README.md#L97)).
  상세 문서의 같은 문장은 turn-end hook을 사용자 설정에 영구 설치하지 않는다는 좁은 문맥이고, 바로 앞에서는
  full-access 역할을 위해 `~/.claude.json`과 `~/.codex/config.toml`에 repo trust를 기록한다고 밝힌다
  ([상세 문서](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/how-work-gets-done.md#L46-L59)).
  실제 adapter도 Codex config에 `[projects.<path>] trust_level = "trusted"`를 append한다
  ([구현](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/core/src/agents/adapter.rs#L242-L267)).

## 기본 Watcher의 정확한 의미

Watcher는 session/cache keepalive가 아니라 working agent의 visible screen을 모델에 보내 “doing now” 문장과 진행률을
추정하는 모니터다. 기본 `watcher.every`는 60초이며, 바뀐 화면만 대상으로 due agent 최대 8개를 한 호출에 묶고
machine 전체에서 한 호출만 실행한다. 부하가 높으면 최대 5배 느려지고 실패 시 backoff한다
([Watcher 문서](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/watcher.md#L1-L35)).
코드도 working stage만 고르고, 동일 screen words에는 호출하지 않으며, 늦은 응답이 새 turn을 덮지 못하게
stage·turn을 재확인한다
([구현](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/watcher.rs#L86-L125),
[late-result guard](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/crates/flow/src/watcher.rs#L128-L145)).
README의 “분당 working agent마다 작은 call”은 비용 요약으로는 이해되지만 실제 호출 topology는 agent별 개별 call이
아니다([README 125행](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/README.md#L125)).
표시되는 percent·ETA는 관측이 아니라 모델 추정이며, 문서도 이를 `guess`로 한정한다
([Watcher 40–57행](https://github.com/eliasstravik/herdr-projects/blob/963d81b4523f725ee028b893521811afd72d1369/docs/watcher.md#L40-L57)).

## ThreadDock에 좁게 빌릴 것과 채택 주의

사용자는 조사 후 **현재 ThreadDock 구조를 유지하며 필요한 요소만 차용하는 1번 방향**을 선택했다. Herdr Projects 설치·실행 관리 대체는 진행하지 않으며, 아래 차용 항목은 후속 후보이지 구현 완료 내역이 아니다. 권한 비교 설명 후에도 기존 실행 정책 보존을 유지하기로 했으며 full-access 기본값 변경은 하지 않았다.

빌릴 가치가 큰 것은 다음 네 가지다.

1. `needs you → ready → working`의 attention-first 요약과 원인별 blocked 표시.
2. observed Herdr status, model이 추정한 진행률, Git/check/review 증거를 서로 다른 신뢰 수준으로 표시.
3. review verdict를 정확한 candidate SHA와 결속하고 merge 직전 head drift를 거부하는 revision-bound proof.
4. check/review/conflict별 bounded retry 뒤 사람에게 올리는 명시적 종료 조건.

반대로 `task.json`을 새 업무 원본으로 삼는 state machine, 자체 Coordinator/Watcher model call, 자동 worktree·commit·push·PR·merge,
default full access와 agent 설정 trust 기록을 ThreadDock Monitor에 넣으면 현재 제품 경계를 뒤집는다. 유용한 아이디어는
기존 GitHub truth·Herdr execution·native leaf 계약과 Go/Wails read monitor 안에 좁게 표현해야 한다. 특히 progress 추정은
기본 기능으로 자동 호출하기보다 비용·provider·정확도·개인정보를 사용자가 선택하도록 하고, UI에서는 사실처럼 보이지
않게 해야 한다.

마지막으로 이 조사는 source와 문서의 구현 의도를 확인한 정적 조사다. upstream의 광범위한 tests가 존재한다는 사실은
확인했지만 suite를 실행하지 않았고, 실제 Herdr·GitHub·agent provider end-to-end 동작이나 보안 격리를 검증한 것은 아니다.
