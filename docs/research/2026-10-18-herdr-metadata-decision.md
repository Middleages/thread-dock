# Herdr metadata의 ThreadDock locator 대체 여부

조사일은 2026-10-18이다. 설치된 `herdr 0.9.5`의 help, protocol 22 API schema, read-only snapshot,
설치 integration source만 조사했다. 외부 문서는 사용하지 않았다. metadata report, session restart/생성,
locator·global config 변경은 하지 않았다.

## 결정

Herdr metadata는 `sessions.json`을 대체하지 않는다. 현재 확인된 계약은 **display-only이며 유효기간 입력을 받는
실행 annotation**이다. source별 update ordering과 TTL 만료를 위한 schema·구현 흔적은 있지만 실제 동작은 이번
범위에서 검증하지 않았다. 특히 다음을 증명하지 못한다.

- server/session restart 뒤 persistence
- 같은 token을 여러 source가 쓸 때의 우선순위와 provenance
- clear가 source의 contribution만 지우는지, 합쳐진 effective value를 지우는지
- Issue/repository/worktree와 Herdr identity를 원자적으로 묶는 durable binding

ThreadDock에 유용한 범위는 workspace/pane 화면에 잠깐 보이는 역할·Issue 번호·관찰 상태 같은 보조 표시다.
업무 원본이나 locator identity로 읽어서는 안 된다. 이 범위에서도 현재 token 사용자가 없고 dual-write는 구조를
단순화하지 않으므로 **지금 새 metadata writer를 추가하지 않는 것**이 최소 권고다.

## 조사 기준과 설치 상태

- `herdr status`: client/server `0.9.5`, stable, protocol 22, endpoint protocol generation 1,
  `endpoint_compatible: yes`, `private_protocol_compatible: yes`, restart 불필요.
- 실제 환경은 `HERDR_ENV=1`이었다. topology live state는 `herdr api snapshot` 한 번의 read-only 확인에 한정했다.
- `herdr api schema --json`: `schema_version: 1`, SHA-256
  `9e2af207e9aa8183d4aeca5fde9cc48e7909bb40cdbd7cf21608a6d3ea78075b`.
- `herdr integration status`: Codex v8은 내장 v10보다 outdated, OpenCode v10은 내장 v13보다 outdated였다.
  설치 source는 metadata token을 쓰지 않고 Agent state/session만 보고하므로 metadata persistence 근거가 아니다.

## schema와 help가 보장하는 것

`herdr workspace report-metadata --help`와 `herdr pane report-metadata --help`는 둘 다
“Report display-only ... metadata”라고 정의한다.

| 항목 | 확인된 입력 계약 | 확인된 출력 계약 | 판정 |
|---|---|---|---|
| `source` | workspace/pane 모두 필수 string. schema에는 길이·pattern이 없음 | `WorkspaceInfo`/`PaneInfo`에 source가 없음 | reporter 구분 입력이지만 consumer가 provenance를 복원할 수 없음 |
| `seq` | optional nullable `uint64`, 최소 0 | 출력되지 않음 | source별 freshness/order guard 용도. accepted seq 조회나 재시작 유지 여부는 미확인 |
| `ttl_ms` | optional nullable `uint64`, 1–86,400,000 ms | expiry 시각·남은 TTL이 출력되지 않음 | 최대 24시간의 만료 요청. 생략 시 수명과 restart 의미는 미확인 |
| token patch | 한 요청 최대 16개, name은 `^[A-Za-z0-9_-]{1,32}$`, value는 string 또는 null | effective token map은 최대 32개 string 값 | null/`--clear-token`으로 개별 clear 요청 가능. source collision/clear 범위는 미확인 |
| workspace metadata | `workspace_id`, `source`, `tokens` 필수; CLI는 `--token`/`--clear-token` 제공 | `WorkspaceInfo.tokens` 외 source/seq/TTL 없음 | flat display annotation |
| pane metadata | `pane_id`, `source` 필수; `agent`, `applies_to_source`, title/display-agent/state-label, tokens와 각 clear 제공 | pane에는 agent/session/title/display-agent/state-label/tokens가 보이지만 source/seq/TTL 없음 | presentation override와 annotation; durable identity가 아님 |

설치 binary에는 `metadata_report_sequence_is_fresh`, `accept_sequence`, `expire_metadata_tokens`,
`next_expiry`, `expire_at` symbol과 source별 sequenced-report 수 제한 오류 문자열이 있다. 이는 seq freshness 검사와
TTL expiry 구현이 존재한다는 보조 근거다. 하지만 binary symbol 이름은 equality 처리, source collision,
restart persistence 같은 세부 계약을 제공하지 않는다. 그런 동작은 확인된 것으로 쓰지 않는다.

## 실제 read-only 상태

protocol 22 snapshot은 workspace 8개와 pane 8개를 반환했다. token value는 출력하지 않고 key만 검사했다.
모든 `WorkspaceInfo.tokens`, `PaneInfo.tokens`, pane `state_labels`가 비어 있었고 title/display-agent도 null이었다.
따라서 이 환경에는 metadata lifecycle을 관찰할 기존 표본이 없다. 쓰기가 금지된 이번 범위에서는 set→stale seq,
TTL expiry, clear, collision, restart를 행동 검증할 수 없다.

snapshot에서 consumer가 받는 것은 합쳐진 flat map이다. token별 source, accepted seq, expiry가 없으므로 같은 key의
값이 보인다는 사실만으로 누가 썼는지, 최신인지, 재시작 가능한 binding인지 판단할 수 없다.

## 설치 integration source가 보여주는 seq 관례

설치 Codex integration `/home/appuser/.codex/herdr-agent-state.sh`는 managed v8 파일이며, SessionStart에서
`source = "herdr:codex"`, `seq = time.time_ns()`, Agent session ID를 `pane.report_agent_session`으로 보낸다
(32–84행). 설치 OpenCode integration
`/home/appuser/.config/opencode/plugins/herdr-agent-state.js` v10은 process-local counter를 만들고(9–29행),
모든 report에 고정 source와 증가 seq를 붙인다(55–85행).

이는 source가 reporter namespace이고 seq가 그 reporter 안에서 단조 증가하도록 설계된 관례를 보여준다. 하지만 두
설치본은 metadata report를 호출하지 않으며 최신 내장 integration보다 오래됐다. 따라서 다음을 추론할 수 없다.

- metadata seq가 server restart 후에도 기억되는지
- reporter process가 재시작해 작은 seq로 돌아갈 때 reset 규칙이 있는지
- 서로 다른 source가 같은 token key를 쓸 때 어느 값이 effective한지
- integration 설치가 metadata token을 자동 복원하는지

## `sessions.json`과의 책임 비교

| 필요한 책임 | Herdr metadata 근거 | 결정 |
|---|---|---|
| 현재 workspace/pane에 짧은 operator hint 표시 | display-only help, title/state-label/token 출력, TTL | 적합하나 optional |
| stale/out-of-order reporter update 억제 | source+seq schema와 binary freshness symbol | 지원이 추정되나 runtime과 exact edge semantics는 미검증 |
| 시간이 지난 annotation 제거 | TTL 1 ms–24 h와 expiry symbol | 지원이 추정되나 runtime은 미검증 |
| GitHub Issue/repository/worktree ↔ exact Herdr 위치 binding | flat token map뿐이며 provenance/transaction 없음 | 부적합 |
| restart 후 재접속 locator | persistence 계약 없음 | 부적합 |
| Agent state를 업무 완료로 판정 | metadata와 무관하며 Herdr lifecycle도 GitHub 완료가 아님 | 금지 |

따라서 metadata를 채택해도 GitHub가 durable work truth라는 경계와 exact live ID 재확인은 그대로 남는다.
`sessions.json`을 없애기 위해 annotation을 mandatory dual-write하면 복잡도만 늘어난다.

## 최소 후속 검증

locator 대체를 다시 검토하려면 별도 승인된 disposable session에서 아래만 확인하면 된다. 현재 작업의 blocker는
아니며 이번에는 실행하지 않았다.

1. source A의 seq 2 뒤 seq 1·동일 seq가 error/no-op/update 중 무엇인지 read back한다.
2. TTL 생략, 짧은 TTL, TTL 후 새 seq를 비교하고 server restart 전후를 구분한다.
3. source A/B가 같은 token을 쓰고 각 source에서 `--clear-token` 했을 때 effective value와 event를 확인한다.
4. pane move/close, workspace close, named session detach/restart에서 metadata가 어떻게 정리되는지 확인한다.
5. API response/event가 source·accepted seq·expiry를 제공하지 않는다면 durable locator 대체 검토를 종료한다.

이 검증이 성공하더라도 첫 적용은 화면 annotation 한두 개로 제한한다. GitHub URL 전체, 권한/sandbox 값,
credential, review verdict나 완료 판정은 token으로 옮기지 않는다.

## Task 결과

- `taskId`: `herdr-metadata-review`
- `baseSHA`: `437d4d2e84d60ae7892deb66b09f873cac22316d`
- `changedFiles`: 이 보고서 한 파일
- `executedCommands`: `herdr --version`, `herdr status`, `herdr --help`, `herdr --skill`, 관련 group/nested
  `--help`, `herdr integration status`, `herdr api schema --json`의 `jq` projection/hash,
  sanitized `herdr api snapshot`, 설치 Codex/OpenCode integration source read, installed binary string/symbol 확인
- `outcomes`: metadata는 optional transient display annotation으로만 적합하며 locator 대체 근거는 부족하다고 판정
- `unverified`: metadata write, stale/equal seq, TTL expiry/default, collision/clear, event, restart/move/close persistence
- `blockers`: 없음. locator 대체 주장은 위 행동 근거가 생길 때까지 보류
