# Single-run GHES/OpenCode pilot

이 문서는 사내 GHES/OpenCode disposable 저장소에서 한 번 실행하는 production prerequisite입니다.
현재 Builder 환경에서는 실제 pilot을 실행하지 않았습니다. Go 1.27.0, Herdr 0.8.2, OpenCode, Git, jq와 GHES 쓰기 권한을 준비하고, THREADDOCK_GH_TOKEN은 agentctl 프로세스에만 주입합니다.

## 1. 준비와 registration outage + RUN 회수

저장소 root에서 원본 fixture를 복사하고 실제 owner/name/base로 수정합니다.

```bash
set -euo pipefail
export THREADDOCK_CONFIG="$HOME/.config/threaddock/config.json"
test -n "$PILOT_OWNER" && test -n "$PILOT_REPO"
cp testdata/contracts/valid.json /tmp/threaddock-pilot.json
jq --arg owner "$PILOT_OWNER" --arg repo "$PILOT_REPO" \
  --arg base "$(git rev-parse HEAD)" \
  '.repository.owner=$owner | .repository.name=$repo | .baseCommit=$base' \
  /tmp/threaddock-pilot.json >/tmp/threaddock-pilot.json.tmp
mv /tmp/threaddock-pilot.json.tmp /tmp/threaddock-pilot.json
MAIN_BEFORE="$(git rev-parse HEAD)"
CHECKOUT_BEFORE="$(git status --short)"
STATE_DIR="$(jq -r '.stateDir' "$THREADDOCK_CONFIG")"
```

승인된 네트워크 장비로 GHES API를 차단한 뒤 start를 실행합니다. (RUN,error)여도 CLI stdout 첫 줄을 회수하고 stderr/exit code를 별도로 기록합니다.

```bash
set +e
agentctl start /tmp/threaddock-pilot.json >/tmp/threaddock-start.out 2>/tmp/threaddock-start.err
START_CODE=$?
set -e
RUN="$(sed -n '1p' /tmp/threaddock-start.out)"
test -n "$RUN"
printf 'start_exit=%s run=%s\n' "$START_CODE" "$RUN"
```

차단 중 새 Issue가 없어야 하고 snapshot의 pending action이 registration이어야 합니다. GHES를 복구한 뒤 같은 RUN만 사용합니다.

## 2. Bounded resume loop와 WSL restart

resume은 paused면 intent append→active Save→Advance 1회, active면 Advance 1회입니다. CLI에는 영구 loop가 없으므로 다음 bounded loop를 사용합니다.

```bash
for attempt in $(seq 1 20); do
  PHASE="$(agentctl status "$RUN" --json | jq -r '.phase')"
  case "$PHASE" in
    reviewing) break ;;
    completed|blocked) exit 1 ;;
    *) agentctl resume "$RUN"; sleep 2 ;;
  esac
done
test "$PHASE" = reviewing
```

작업 중 한 번 agentctl stop "$RUN"을 실행하고 snapshot의 RUN, Parent Issue, Worktree IDs, prompt receipts와 commit IDs를 기록합니다. Windows PowerShell에서 wsl --shutdown을 한 번 실행한 뒤 WSL/Herdr/OpenCode를 다시 시작하고 같은 bounded resume loop를 반복합니다. stop은 evidence와 Worktree를 삭제하지 않습니다.

## 3. Invariants와 evidence

GHES marker td:<RUN>의 Parent/Child bundle은 각각 한 번만 존재해야 하고, Herdr branch/path/workspace도 중복되지 않아야 합니다. 다음 invariant를 확인합니다.

```bash
test "$(jq -r '.runId' "$STATE_DIR/runs/$RUN/run.json")" = "$RUN"
test "$MAIN_BEFORE" = "$(git rev-parse HEAD)"
test "$CHECKOUT_BEFORE" = "$(git status --short)"
```

completed 후 7일 전에는 cleanup하지 않습니다. evidence에는 date, copied contract/base SHA, RUN/Issue links, Builder·Reviewer IDs, outage/WSL 결과, duplicate count, main before/after와 cleanup preflight를 기록합니다. token, raw transcript와 자격 증명은 기록하지 않습니다.

## 4. 중단 기준

stdout 첫 줄이 비어 있거나 RUN/marker/Worktree ID/receipt/commit SHA가 바뀌거나, Issue·Worktree가 중복되거나, main이 변경되거나, credential이 evidence에 나타나면 즉시 외부 쓰기를 중단하고 snapshot/event만 보존합니다. dirty 또는 identity mismatch Worktree cleanup, 강제 삭제, 수동 main merge를 하지 말고 Operator에게 escalation합니다.
