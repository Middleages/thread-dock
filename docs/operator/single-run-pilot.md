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
  '.repository.owner=$owner | .repository.name=$repo | .repository.defaultBranch="main" | .baseCommit=$base |
   .parent={key:"parent",title:"Single-run pilot",body:"pilot result",acceptanceCriteria:["pilot-result.txt 한 파일만 추가한다"],labels:[]} |
   .children=[{key:"pilot",title:"Single-run pilot",body:"pilot result",acceptanceCriteria:["pilot-result.txt 한 파일만 추가한다"],labels:[]}] |
   .tasks=[{id:"pilot",issueKey:"pilot",owner:"pilot-builder",role:"builder",branch:"agent/pilot",allowedPaths:["pilot-result.txt"],dependsOn:[],acceptanceCriteria:["pilot-result.txt 한 파일만 추가한다"],verification:["test -f pilot-result.txt"]}] |
   .verification=["test -f pilot-result.txt"]' \
  /tmp/threaddock-pilot.json >/tmp/threaddock-pilot.json.tmp
mv /tmp/threaddock-pilot.json.tmp /tmp/threaddock-pilot.json
agentctl contract validate /tmp/threaddock-pilot.json
MAIN_BEFORE="$(git rev-parse HEAD)"
CHECKOUT_BEFORE="$(git status --short)"
REPO_ROOT="$(git rev-parse --show-toplevel)"
STATE_DIR="$(jq -r '.stateDir // empty' "$THREADDOCK_CONFIG")"
: "${STATE_DIR:=$HOME/.local/state/threaddock}"
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

resume은 paused면 intent append→active Save→Advance 1회, active면 Advance 1회입니다. CLI에는 영구 loop가 없으므로 다음 bounded loop를 사용합니다. reviewing phase만으로 종료하지 않습니다.

```bash
for attempt in $(seq 1 20); do
  SNAPSHOT="$STATE_DIR/runs/$RUN/run.json"
  if jq -e '(.phase == "reviewing") and ((.reviewer.name // "") != "") and
    ((.reviewerWorktree.path // "") != "") and
    ((.reviewerWorktree.workspaceId // "") != "") and ((.reviewerWorktree.paneId // "") != "") and
    ((.reviewerPrompt.requestId // "") != "") and (.pendingAction == "") and
    ((.reviewer.verification // []) | index("review schema sent") != null)' "$SNAPSHOT" >/dev/null; then
    break
  fi
  jq -e '(.phase != "completed") and (.phase != "blocked")' "$SNAPSHOT" >/dev/null
  agentctl resume "$RUN"
  sleep 2
done
jq -e '(.phase == "reviewing") and ((.reviewer.name // "") != "") and
  ((.reviewerWorktree.path // "") != "") and
  ((.reviewerWorktree.workspaceId // "") != "") and ((.reviewerWorktree.paneId // "") != "") and
  ((.reviewerPrompt.requestId // "") != "") and (.pendingAction == "") and
  ((.reviewer.verification // []) | index("review schema sent") != null)' "$STATE_DIR/runs/$RUN/run.json" >/dev/null
```

작업 중 한 번 stop하고 WSL을 재시작합니다. 종료 전 snapshot subset을 정렬 저장하고, Windows PowerShell에서 wsl --shutdown을 실행합니다.

```bash
agentctl stop "$RUN"
jq -S '{runId,parentIssue,integration,builderWorktree,reviewerWorktree,builderPrompt,reviewerPrompt,builderCommitSha:.builder.commitSha}' \
  "$STATE_DIR/runs/$RUN/run.json" >/tmp/threaddock-$RUN-before-wsl.json
umask 077
RESUME_ENV="$HOME/.local/state/threaddock/pilot-resume.env"
mkdir -p "$(dirname "$RESUME_ENV")"
{
  printf 'RUN=%q\n' "$RUN"
  printf 'STATE_DIR=%q\n' "$STATE_DIR"
  printf 'MAIN_BEFORE=%q\n' "$MAIN_BEFORE"
  printf 'CHECKOUT_BEFORE=%q\n' "$CHECKOUT_BEFORE"
  printf 'REPO_ROOT=%q\n' "$REPO_ROOT"
  printf 'THREADDOCK_CONFIG=%q\n' "$THREADDOCK_CONFIG"
} >"$RESUME_ENV"
chmod 600 "$RESUME_ENV"
printf 'source after WSL restart: %s\n' "$RESUME_ENV"
```

```powershell
wsl --shutdown
```

WSL/Herdr/OpenCode를 다시 시작해 WSL에 재진입한 뒤 종료 전에 출력한 env 경로를 source합니다. token은 저장하지 않았으므로 secret manager로 다시 주입합니다. resume 전 snapshot diff가 먼저 통과해야 합니다.

```bash
RESUME_ENV="$HOME/.local/state/threaddock/pilot-resume.env"
source "$RESUME_ENV"
test -d "$REPO_ROOT"; cd "$REPO_ROOT"
# Herdr duplicate checks and git main/status invariants below run from the restored repository root.
test -f "$RESUME_ENV"
jq -S '{runId,parentIssue,integration,builderWorktree,reviewerWorktree,builderPrompt,reviewerPrompt,builderCommitSha:.builder.commitSha}' \
  "$STATE_DIR/runs/$RUN/run.json" >/tmp/threaddock-$RUN-after-wsl.json
diff -u /tmp/threaddock-$RUN-before-wsl.json /tmp/threaddock-$RUN-after-wsl.json
herdr worktree list --cwd "$REPO_ROOT" | jq .
for attempt in $(seq 1 20); do
  SNAPSHOT="$STATE_DIR/runs/$RUN/run.json"
  if jq -e '(.phase == "reviewing") and ((.reviewer.name // "") != "") and ((.reviewerWorktree.path // "") != "") and ((.reviewerWorktree.workspaceId // "") != "") and ((.reviewerWorktree.paneId // "") != "") and ((.reviewerPrompt.requestId // "") != "") and (.pendingAction == "") and ((.reviewer.verification // []) | index("review schema sent") != null)' "$SNAPSHOT" >/dev/null; then break; fi
  jq -e '(.phase != "completed") and (.phase != "blocked")' "$SNAPSHOT" >/dev/null
  agentctl resume "$RUN"
  sleep 2
done
```

## 3. Invariants와 evidence

GHES marker td:<RUN>의 Parent/Child bundle은 각각 한 번만 존재해야 하고, Herdr branch/path/workspace도 중복되지 않아야 합니다. 다음 invariant를 확인합니다.

```bash
OWNER="$(jq -r '.repository.owner' /tmp/threaddock-pilot.json)"
REPO="$(jq -r '.repository.name' /tmp/threaddock-pilot.json)"
MARKER="<!-- threaddock:td:$RUN -->"
PARENT_ACCEPTANCE='pilot-result.txt 한 파일만 추가한다'
jq -e '.parent == {key:"parent",title:"Single-run pilot",body:"pilot result",acceptanceCriteria:[$acceptance],labels:[]}' \
  --arg acceptance "$PARENT_ACCEPTANCE" /tmp/threaddock-pilot.json >/dev/null
ISSUE_COUNT="$(gh api "repos/$OWNER/$REPO/issues" --paginate | jq -s --arg marker "$MARKER" 'map(.[]) | map(select((.body // "") | contains($marker))) | length')"
test "$ISSUE_COUNT" -eq 2
HERDR_DUPLICATES="$(herdr worktree list --cwd "$PWD" | jq '[.result.worktrees[]? | select((.path // "") != "" and (.branch // "") != "")] | group_by([.path,.branch]) | map(select(length > 1)) | length')"
test "$HERDR_DUPLICATES" -eq 0
test "$(jq -r '.runId' "$STATE_DIR/runs/$RUN/run.json")" = "$RUN"
test -n "$(jq -r '.reviewerPrompt.requestId // empty' "$STATE_DIR/runs/$RUN/run.json")"
test "$MAIN_BEFORE" = "$(git rev-parse HEAD)"
test "$CHECKOUT_BEFORE" = "$(git status --short)"
```

completed 후 7일 전에는 cleanup하지 않습니다. evidence에는 date, copied contract/base SHA, RUN/Issue links, Builder·Reviewer IDs, outage/WSL 결과, duplicate count, main before/after와 cleanup preflight를 기록합니다. token, raw transcript와 자격 증명은 기록하지 않습니다.

```text
Single-run pilot: <date>
Contract/base: /tmp/threaddock-pilot.json, <base SHA>
Run/Parent/Child: <RUN>, <links>; marker count=2
Builder/Reviewer IDs: <workspace/pane/path IDs>
Reviewer prompt request ID: <reviewerPrompt.requestId>
GHES outage: <pass/fail>; WSL restart: <pass/fail>; snapshot diff: <pass/fail>
Duplicate Issue/Worktree: <0>
main before/after: <SHA>/<SHA; unchanged>
Cleanup preflight: <completed, age, every Worktree clean>
Blocker/decision: <none or Korean action>
```

## 4. 중단 기준

stdout 첫 줄이 비어 있거나 RUN/marker/Worktree ID/receipt/commit SHA가 바뀌거나, Issue·Worktree가 중복되거나, main이 변경되거나, credential이 evidence에 나타나면 즉시 외부 쓰기를 중단하고 snapshot/event만 보존합니다. dirty 또는 identity mismatch Worktree cleanup, 강제 삭제, 수동 main merge를 하지 말고 Operator에게 escalation합니다.
