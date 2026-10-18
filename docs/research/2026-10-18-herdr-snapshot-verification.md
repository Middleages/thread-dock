# Herdr snapshot query와 Monitor projection 검증

## 범위와 판정

Task `herdr-snapshot-probe`의 고정 base는 `437d4d2e84d60ae7892deb66b09f873cac22316d`이다. 이 문서는 현재 `monitor/herdr.go`의 정규화·캐시·부분 실패 의미를 바꾸지 않고, 설치된 Herdr의 read-only snapshot/list 응답을 현재 Monitor wire projection과 대조한 기록이다. 새 session, input/control, `report-metadata`, provider/network 호출, Wails 실행은 하지 않았다.

결론은 **현재 slice에서 production adapter를 전환하지 않는다**이다.

- workspace identity/label, agent status/location/cwd와 optional `name`의 의미가 현재 projection과 일치한다.
- Herdr의 `agent` 필드는 provider kind(예: `codex`)이고 explicit agent identity인 `name`과 다르다. default session의 6행 중 5행은 `name`이 없지만, 같은 행이 `agent list`와 `session_snapshot` 양쪽에서 동일하게 비어 있다. 이는 호환성 실패가 아니다.
- `session_snapshot`은 한 번의 read로 agent/workspace를 함께 주지만, 현재 adapter가 보존하는 per-session cache, workspace partial-failure notice, stale/offline 의미와 동일하다고 입증되지 않았다. 이 문서는 alias나 adapter switch를 구현하지 않는다.

## 실제 read-only 환경

먼저 설치본의 `herdr --help`, `herdr --skill`을 읽고 `HERDR_ENV=1`을 확인했다. 실행 중인 default session만 exact target으로 사용했다. `herdr session list --json`에서 default가 running임을 확인했으며 다른 session에는 attach하지 않았다.

| 항목 | 결과 |
|---|---|
| client | `herdr 0.9.5` |
| server | `0.9.5`, `endpoint_compatible: yes`, `private_protocol: 22` |
| endpoint | protocol `22`, endpoint protocol generation `1` |
| target | selected session `default`, `HERDR_ENV=1` |
| direct snapshot | `session_snapshot`, agents `6`, workspaces `8` |
| direct list | `agent_list` 6행, `workspace_list` 8행 |

실행한 live read 명령은 다음과 같다(원문 JSON은 `/tmp` 아래 임시 경로에만 저장했고 저장소에 넣지 않았다).

```text
herdr --version
herdr status client
herdr status server
herdr session list --json
herdr --session default api schema --json
herdr --session default api snapshot
herdr --session default agent list
herdr --session default workspace list
```

`api schema --json`의 protocol은 `22`, schema version은 `1`이며 `session.snapshot`, `agent.list`, `workspace.list` method가 존재한다. `api snapshot`의 snapshot key는 `agents`, `workspaces`, `panes`, `tabs`, `layouts`, `protocol`, `version` 등이다.

원문 JSON은 아래 임시 경로에만 보관했으며 저장소에는 넣지 않았다. 이 경로는 scratch evidence이며 재현 환경에서 사라질 수 있다.

```text
/tmp/herdr-snapshot-compare.1M2SbS/{snapshot-1.json,agents-1.json,workspaces-1.json}
/tmp/herdr-snapshot-compare.1M2SbS/compare_projection.py
/tmp/herdr-snapshot-drift.u6y3DT/{first.json,second.json}
/tmp/herdr-snapshot-schema.GrX5r1/schema.json
```

동일 target의 list/snapshot semantic projection은 다음 read-only Python probe로 비교했다.

```text
python3 /tmp/herdr-snapshot-compare.1M2SbS/compare_projection.py /tmp/herdr-snapshot-compare.1M2SbS
```

## Projection 비교

현재 Monitor가 읽는 정규화 필드는 `monitor/herdr.go:524-570`의 `normalizeHerdrAgents`와 `:573-610`의 `normalizeHerdrWorkspaces` 기준이다. agent `name`은 optional로 읽고, workspace/tab/pane ID는 required로 읽는다. workspace label은 별도 `workspace list` 결과로 enrich한다.

| 관심 필드 | 0.9.5 live 응답 | 현재 Monitor projection | 판정 |
|---|---|---|---|
| agent identity | `agent`는 provider kind이며 `name`은 optional; 6행 중 5행에 name 없음 | optional `name`을 읽고 없으면 빈 값 유지 | PASS |
| `agent_status` | 있음 | 보존 | PASS |
| `workspace_id`, `tab_id`, `pane_id` | 있음 | 보존·identity key 사용 | PASS |
| `cwd`, `foreground_cwd` | 있음 | 보존·worktree check에 사용 | PASS |
| workspace identity/label | `workspace_id`, `label` | 보존·agent에 enrich | PASS |
| terminal/session metadata | `terminal_id`, `agent_session`, title/seq 등이 live 응답에 포함 | wire projection에서 제외 | PASS (allowlist) |

실제 default session의 agent list와 session snapshot을 `(name, agent_status, workspace_id, tab_id, pane_id, cwd, foreground_cwd)`로 투영한 결과는 key와 값이 모두 같았다. 양쪽 모두 6행이며 `name` 부재는 5행에서 동일했다. workspace list/snapshot도 `(workspace_id, active_tab_id, label)` projection이 8행 모두 같았다. 현재 코드가 `name`을 optional로 처리하고 `agent` provider kind를 읽지 않는 것은 의도된 의미 분리와 일치한다. 이름이 없는 complete location agent를 unconnected로 취급하는 기존 경로는 이 live 응답에서도 그대로 의미가 있다.

### 가장 작은 synthetic probe

legacy `name`이 있는 행과 provider kind만 있고 `name`이 없는 0.9.5 행을 한 행씩 넣어 현재 allowlist projection을 재현했다. `agent`는 이름 alias로 사용하지 않는다.

```text
synthetic_rows=2
current_name_projection='','Sol'
agent_kind_not_identity=PASS
list_snapshot_semantic_parity=PASS
identity_status_cwd_parity=PASS
workspace_label_parity=PASS
projection_bytes=396 limit=524288 within_limit=True
partial_workspace_failure=source_contract_static:agents_retained=True labels_empty=True notice_required=True
malformed_agent_cold_cache=offline;warm_cache=cached (source/test contract; not executed here)
call_count_one_session=cat+agent_list+workspace_list=3; formula=1+2N sessions when uncached
```

이 probe는 production source에 추가되지 않았다. 실제 raw `session_snapshot` 응답은 13,002 bytes로 `maxHerdrOutput=512*1024` 한도 안에 있었고, raw `agent list`/`workspace list` 응답은 각각 2,918/2,946 bytes였다. sanitized projection은 1,554 bytes였다. 이는 현재 대상 규모의 단일 관찰일 뿐, 큰 workspace에 대한 coverage 주장이 아니다.

### Snapshot 대 per-session 호출과 drift

현재 Monitor의 uncached 호출 수는 연결 파일 `cat` 1회 + session마다 `agent list` 1회 + 성공 시 `workspace list` 1회로 `1+2N`이다. 기존 focused fixture가 한 session에서 3 calls를 확인한다. `session_snapshot`은 전체 topology를 한 번에 주지만, 현재 Monitor가 요구하는 session별 connection binding·worktree canonical check·cache semantics를 대체한다고 보지 않았다.

동일 default target의 `api snapshot`을 2초 간격으로 두 번 비교했다.

```text
agent_identity_sets=STABLE
agent_cwd_projection=STABLE
agent_status_projection=STABLE
workspace_identity_sets=STABLE
workspace_label_projection=STABLE
workspace_status_projection=STABLE
counts=6/6 agents, 8/8 workspaces
```

이 관찰 구간에서는 drift가 없었다. 다만 `agent_status`, focused/status, revision/sequence는 실행 상태이므로 안정 identity/label/cwd와 같은 계약으로 캐시하거나 완료로 해석해서는 안 된다. 이 문서는 장기 polling이나 large-scale drift coverage를 검증하지 않는다.

## 실패·메타데이터 의미

- **workspace partial failure:** 기존 source와 fixture는 agent list가 성공하고 workspace list가 실패하거나 malformed일 때 agent 관찰을 fresh로 유지하고 label을 비운 뒤 session별 notice를 남기는 계약을 정의한다. Go 실행 파일 부재로 이 환경에서는 실행하지 못했지만, workspace metadata failure가 agent observation을 숨기지 않는 의미는 보존 대상이다.
- **malformed agent row:** 기존 source와 fixture는 cold cache에서 offline/missing, valid prior cache에서만 cached stale fallback을 정의한다. malformed payload를 빈 목록으로 성공 처리하지 않는다. 이 환경에서는 Go focused test를 실행하지 못해 동작 PASS로 주장하지 않는다.
- **size:** current output limit은 command stdout 512 KiB이다. 실측 단일 payload와 1,554-byte sanitized wire는 한도 안에 있지만, 8 workspace/6 agent 한 번의 관찰만으로 규모 확장을 보장하지 않는다.
- **metadata:** direct snapshot의 `terminal_id`, `agent_session`, title, completion/state sequence는 현재 ThreadDock wire의 stable identity가 아니다. adapter를 검토하더라도 workspace/tab/pane/agent name/status/cwd와 label만 allowlist하고 이 metadata를 그대로 노출하지 않아야 한다. 이 probe는 `report-metadata`나 secrets를 요청하지 않았다.
- **partial snapshot trade-off:** one-call snapshot은 call count를 줄일 가능성이 있지만, 어떤 session/workspace subsection이 실패했는지와 cache fallback을 분리 표시하는 현재 semantics가 사라질 수 있다. fresh snapshot, cached session, offline session을 같은 aggregate status로 합치기 전에 실제 endpoint 오류 응답과 per-session target fixture가 더 필요하다.

## 권고와 한계

1. **지금은 production adapter switch를 보류한다.** 현재 code path와 stale/offline/partial semantics를 그대로 유지한다.
2. 후속 구현이 승인되면 `session.snapshot` response fixture와 stable semantic allowlist를 별도 bounded change로 합의하고, list/snapshot의 optional name 부재, binding match/duplicate/conflict, cache semantics를 기존 fixture에 추가해 focused 검증한다. `agent` provider kind를 `name` alias로 매핑하지 않는다.
3. 이 검증의 live 근거는 현재 default session의 read-only 6 agent/8 workspace와 2초 drift window뿐이다. Wails/native 화면, WSL에서 Go adapter가 실제로 동일 응답을 받는지, 다중 session·large payload·endpoint failure matrix는 **미검증**이다.

## 검증 결과

실행 명령과 결과:

| 명령 | 결과 |
|---|---|
| `herdr --help`, `herdr --skill` | PASS |
| `herdr status client/server`, `--version` | PASS: 0.9.5 / protocol 22 / endpoint 1 |
| read-only default `api snapshot`, `agent list`, `workspace list` | PASS: JSON read, 6/8 rows |
| synthetic projection/size probe | PASS: provider kind와 optional name을 분리하고 list/snapshot semantic parity 재현 |
| partial-failure/malformed semantics | source/test contract만 확인; Go 실행 불가로 runtime 결과는 UNVERIFIED |
| `go test ./monitor -run 'TestHerdrMonitor\|TestCombinedMonitor' -count=1` | **UNVERIFIED/BLOCKED**: this environment has no `go` executable |

No product Go/React file, shared interface, global setting, locator, live session, or external record was changed. 변경 파일은 이 research report 하나뿐이다.
