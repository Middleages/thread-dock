#!/usr/bin/env bash

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly THREADDOCK_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly CONFIG_PATH="${THREADDOCK_CONFIG:-$HOME/.config/threaddock/config.json}"
SIMULATION_CONFIG_PATH=""
START_STDOUT_PATH=""
START_STDERR_PATH=""

say() {
  printf '%s\n' "$*"
}

fail() {
  printf '오류: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Single-run 파일럿 도우미

처음 실행:
  ./scripts/single-run-pilot.sh OWNER REPOSITORY

GitHub.com 시험 (등록 장애 시뮬레이션):
  ./scripts/single-run-pilot.sh --simulate-outage OWNER REPOSITORY

WSL을 다시 시작한 다음:
  ./scripts/single-run-pilot.sh --continue

현재 결과만 다시 확인:
  ./scripts/single-run-pilot.sh --check

OWNER와 REPOSITORY에는 파일럿용 GitHub Enterprise 또는 GitHub.com 저장소의
조직명과 저장소명을 입력합니다. 제품 저장소나 main에서 직접 시험하지 마십시오.
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "'$1' 명령을 찾을 수 없습니다. 설치 후 다시 실행해 주세요."
}

read_json() {
  jq -er "$2" "$1"
}

cleanup_start_artifacts() {
  if [[ -n "${SIMULATION_CONFIG_PATH:-}" ]]; then
    rm -f -- "$SIMULATION_CONFIG_PATH"
    SIMULATION_CONFIG_PATH=""
  fi
  if [[ -n "${START_STDOUT_PATH:-}" ]]; then
    rm -f -- "$START_STDOUT_PATH"
    START_STDOUT_PATH=""
  fi
  if [[ -n "${START_STDERR_PATH:-}" ]]; then
    rm -f -- "$START_STDERR_PATH"
    START_STDERR_PATH=""
  fi
}

redact_text() {
  local value="$1"
  local secret="${THREADDOCK_GH_TOKEN:-}"
  if [[ -n "$secret" ]]; then
    value="${value//"$secret"/[REDACTED]}"
  fi
  printf '%s' "$value"
}

default_state_dir() {
  local configured
  configured="$(jq -r '.stateDir // empty' "$CONFIG_PATH")"
  if [[ -n "$configured" ]]; then
    printf '%s\n' "$configured"
  else
    printf '%s\n' "$HOME/.local/state/threaddock"
  fi
}

session_path() {
  if [[ -n "${THREADDOCK_PILOT_SESSION:-}" ]]; then
    printf '%s\n' "$THREADDOCK_PILOT_SESSION"
    return
  fi
  [[ -f "$CONFIG_PATH" ]] || fail "설정 파일이 없습니다: $CONFIG_PATH"
  printf '%s/pilot/session.json\n' "$(default_state_dir)"
}

load_session() {
  SESSION_FILE="$(session_path)"
  [[ -f "$SESSION_FILE" ]] || fail "이어갈 파일럿 기록이 없습니다. 먼저 OWNER와 REPOSITORY를 넣어 실행해 주세요."
  RUN="$(read_json "$SESSION_FILE" '.run')"
  STATE_DIR="$(read_json "$SESSION_FILE" '.stateDir')"
  MAIN_BEFORE="$(read_json "$SESSION_FILE" '.mainBefore')"
  CHECKOUT_BEFORE="$(jq -r '.checkoutBefore // ""' "$SESSION_FILE")"
  REPO_ROOT="$(read_json "$SESSION_FILE" '.repoRoot')"
  CONTRACT_PATH="$(read_json "$SESSION_FILE" '.contractPath')"
  SNAPSHOT="$STATE_DIR/runs/$RUN/run.json"
  [[ -f "$SNAPSHOT" ]] || fail "실행 기록을 찾을 수 없습니다: $SNAPSHOT"
  [[ -f "$CONTRACT_PATH" ]] || fail "파일럿 작업 계약을 찾을 수 없습니다: $CONTRACT_PATH"
}

set_session_value() {
  local key="$1"
  local value="$2"
  local temp_file="${SESSION_FILE}.tmp"
  jq --arg key "$key" --argjson value "$value" '.[$key] = $value' "$SESSION_FILE" >"$temp_file"
  chmod 600 "$temp_file"
  mv "$temp_file" "$SESSION_FILE"
}

wait_for_enter() {
  local message="$1"
  printf '\n%s\n계속하려면 Enter를 누르세요. 취소하려면 Ctrl+C를 누르세요.\n' "$message"
  read -r
}

reviewer_ready() {
  jq -e '
    (.phase == "reviewing") and
    ((.reviewer.name // "") != "") and
    ((.reviewerWorktree.path // "") != "") and
    ((.reviewerWorktree.workspaceId // "") != "") and
    ((.reviewerWorktree.paneId // "") != "") and
    ((.reviewerPrompt.requestId // "") != "") and
    (.pendingAction == "") and
    ((.reviewer.verification // []) | index("review schema sent") != null)
  ' "$SNAPSHOT" >/dev/null
}

integration_ready() {
  jq -e '
    (.phase == "reviewing") and
    (.actionCursor == 0) and
    ((.integration.path // "") != "") and
    ((.builder.commitSha // "") != "") and
    (.pendingAction == "")
  ' "$SNAPSHOT" >/dev/null
}

resume_until() {
  local target="$1"
  local target_label="$2"
  local attempt phase
  for attempt in $(seq 1 30); do
    if "$target"; then
      say "[$attempt/30] $target_label: 준비됨"
      return 0
    fi
    phase="$(jq -r '.phase' "$SNAPSHOT")"
    if [[ "$phase" == "completed" || "$phase" == "blocked" ]]; then
      fail "실행이 '$phase' 상태라 자동으로 계속할 수 없습니다. agentctl status $RUN으로 원인을 확인해 주세요."
    fi
    say "[$attempt/30] 에이전트 작업을 이어갑니다. 사내 모델이 느리면 이 단계가 오래 걸릴 수 있습니다."
    set +e
    THREADDOCK_CONFIG="$CONFIG_PATH" agentctl resume "$RUN"
    local resume_code=$?
    set -e
    if [[ $resume_code -ne 0 ]]; then
      say "  이번 시도는 완료되지 않았습니다. 상태를 보존하고 다시 시도합니다."
    fi
    sleep 5
  done
  fail "$target_label 상태에 도달하지 못했습니다. 기록은 보존했습니다. agentctl status $RUN을 확인해 주세요."
}

snapshot_subset() {
  jq -S '{runId,parentIssue,integration,builderWorktree,reviewerWorktree,builderPrompt,reviewerPrompt,builderCommitSha:.builder.commitSha}' "$1"
}

write_initial_session() {
  local boot_id="$1"
  local simulate_outage="${2:-false}"
  mkdir -p "$(dirname "$SESSION_FILE")"
  umask 077
  jq -n \
    --arg run "$RUN" \
    --arg stateDir "$STATE_DIR" \
    --arg mainBefore "$MAIN_BEFORE" \
    --arg checkoutBefore "$CHECKOUT_BEFORE" \
    --arg repoRoot "$REPO_ROOT" \
    --arg contractPath "$CONTRACT_PATH" \
    --arg bootIdBefore "$boot_id" \
    --argjson simulateOutage "$simulate_outage" \
    '{run:$run,stateDir:$stateDir,mainBefore:$mainBefore,checkoutBefore:$checkoutBefore,repoRoot:$repoRoot,contractPath:$contractPath,bootIdBefore:$bootIdBefore,simulateOutage:$simulateOutage,outagePassed:true,wslRestartPassed:false,snapshotDiffPassed:false}' \
    >"$SESSION_FILE"
  chmod 600 "$SESSION_FILE"
}

prepare_contract() {
  local owner="$1"
  local repository="$2"
  local fixture="$THREADDOCK_ROOT/testdata/contracts/valid.json"
  [[ "$owner" =~ ^[A-Za-z0-9._-]+$ ]] || fail "OWNER 형식이 올바르지 않습니다."
  [[ "$repository" =~ ^[A-Za-z0-9._-]+$ ]] || fail "REPOSITORY 형식이 올바르지 않습니다."
  [[ -f "$fixture" ]] || fail "파일럿 원본 계약을 찾을 수 없습니다: $fixture"

  mkdir -p "$(dirname "$CONTRACT_PATH")"
  jq --arg owner "$owner" --arg repo "$repository" --arg base "$MAIN_BEFORE" '
    .repository.owner=$owner |
    .repository.name=$repo |
    .repository.defaultBranch="main" |
    .baseCommit=$base |
    .parent={key:"parent",title:"Single-run pilot",body:"ThreadDock 내부 시험",acceptanceCriteria:["pilot-result.txt 한 파일만 추가한다"],labels:[]} |
    .children=[{key:"pilot",title:"Single-run pilot 작업",body:"ThreadDock 내부 시험",acceptanceCriteria:["pilot-result.txt 한 파일만 추가한다"],labels:[]}] |
    .tasks=[{id:"pilot",issueKey:"pilot",owner:"pilot-builder",role:"builder",branch:"agent/pilot",allowedPaths:["pilot-result.txt"],dependsOn:[],acceptanceCriteria:["pilot-result.txt 한 파일만 추가한다"],verification:["test -f pilot-result.txt"]}] |
    .verification=["test -f pilot-result.txt"]
  ' "$fixture" >"${CONTRACT_PATH}.tmp"
  mv "${CONTRACT_PATH}.tmp" "$CONTRACT_PATH"
  agentctl contract validate "$CONTRACT_PATH" >/dev/null
}

start_pilot() {
  local owner="$1"
  local repository="$2"
  local simulate_outage="${3:-false}"
  local start_code start_output start_error boot_id

  for command in jq git agentctl herdr opencode gh; do
    require_command "$command"
  done
  [[ -f "$CONFIG_PATH" ]] || fail "설정 파일이 없습니다: $CONFIG_PATH"
  [[ -n "${THREADDOCK_GH_TOKEN:-}" ]] || fail "THREADDOCK_GH_TOKEN이 없습니다. 비밀 저장소에서 다시 주입해 주세요."

  REPO_ROOT="$(git rev-parse --show-toplevel)"
  cd "$REPO_ROOT"
  [[ -z "$(git status --short)" ]] || fail "현재 저장소에 커밋하지 않은 변경이 있습니다. 파일럿용 저장소를 깨끗하게 만든 뒤 다시 실행해 주세요."
  MAIN_BEFORE="$(git rev-parse HEAD)"
  CHECKOUT_BEFORE="$(git status --short)"
  STATE_DIR="$(default_state_dir)"
  SESSION_FILE="${THREADDOCK_PILOT_SESSION:-$STATE_DIR/pilot/session.json}"
  CONTRACT_PATH="$STATE_DIR/pilot/contract.json"
  [[ ! -f "$SESSION_FILE" ]] || fail "진행 중인 파일럿 기록이 있습니다: $SESSION_FILE (--continue 또는 --check를 사용하세요.)"

  prepare_contract "$owner" "$repository"
  say "준비 확인이 끝났습니다."
  say "- 시험 저장소: $owner/$repository"
  say "- 로컬 위치: $REPO_ROOT"
  say "- 작업 내용: pilot-result.txt 파일 하나 만들기"

  trap 'cleanup_start_artifacts' EXIT
  trap 'cleanup_start_artifacts; exit 130' INT
  trap 'cleanup_start_artifacts; exit 143' TERM
  if [[ "$simulate_outage" == true ]]; then
    SIMULATION_CONFIG_PATH="$(mktemp "$STATE_DIR/pilot/simulate-config.XXXXXX")"
    jq --arg apiBase "http://127.0.0.1:1" '{ghesHost,apiBase:$apiBase,apiVersion,stateDir,herdrBinary,gitBinary,workingWait,recoveryLimit,projectId,projectStatusFieldId,projectStatusOptions}' "$CONFIG_PATH" >"$SIMULATION_CONFIG_PATH"
    chmod 600 "$SIMULATION_CONFIG_PATH"
    say "GitHub.com 등록 장애를 로컬에서 시뮬레이션합니다. 네트워크 차단이나 복구는 필요하지 않습니다."
  else
    wait_for_enter "중요: 지금 GHES API 연결을 승인된 방법으로 잠시 차단해 주세요."
  fi

  START_STDOUT_PATH="$(mktemp "$STATE_DIR/pilot/start.out.XXXXXX")"
  START_STDERR_PATH="$(mktemp "$STATE_DIR/pilot/start.err.XXXXXX")"
  chmod 600 "$START_STDOUT_PATH" "$START_STDERR_PATH"

  set +e
  if [[ "$simulate_outage" == true ]]; then
    THREADDOCK_CONFIG="$SIMULATION_CONFIG_PATH" agentctl start "$CONTRACT_PATH" >"$START_STDOUT_PATH" 2>"$START_STDERR_PATH"
  else
    agentctl start "$CONTRACT_PATH" >"$START_STDOUT_PATH" 2>"$START_STDERR_PATH"
  fi
  start_code=$?
  set -e
  start_output="$(sed -n '1p' "$START_STDOUT_PATH")"
  start_error="$(redact_text "$(tail -n 1 "$START_STDERR_PATH" 2>/dev/null || true)")"
  cleanup_start_artifacts
  trap - EXIT INT TERM
  [[ -n "$start_output" ]] || fail "실행 ID를 받지 못했습니다. 연결 중단 방식과 start 오류를 확인해 주세요: $start_error"
  RUN="$start_output"
  [[ -z "${THREADDOCK_GH_TOKEN:-}" || "$RUN" != *"$THREADDOCK_GH_TOKEN"* ]] || fail "실행 ID에 허용되지 않은 값이 포함되었습니다."
  SNAPSHOT="$STATE_DIR/runs/$RUN/run.json"
  [[ -f "$SNAPSHOT" ]] || fail "실행 기록이 생성되지 않았습니다: $SNAPSHOT"
  if [[ $start_code -eq 0 ]] || ! jq -e '(.pendingAction == "register_issue_bundle") and (.registration.status == "pending") and ((.parentIssue // 0) == 0)' "$SNAPSHOT" >/dev/null; then
    fail "연결 중단 시험에 실패했습니다. Issue가 생성되기 전에 등록 대기 상태가 되어야 합니다."
  fi

  boot_id="$(cat /proc/sys/kernel/random/boot_id 2>/dev/null || printf 'unknown')"
  write_initial_session "$boot_id" "$simulate_outage"
  if [[ "$simulate_outage" == true ]]; then
    say "GitHub.com 등록 장애 시뮬레이션: PASS (실행 ID $RUN 보존)"
    say "원래 설정으로 같은 실행을 자동으로 이어갑니다."
  else
    say "GHES 연결 중단 시험: PASS (실행 ID $RUN 보존)"
    wait_for_enter "이제 GHES API 연결을 복구해 주세요."
  fi

  resume_until integration_ready "Builder 작업과 integration 반영"
  THREADDOCK_CONFIG="$CONFIG_PATH" agentctl stop "$RUN"
  snapshot_subset "$SNAPSHOT" >"$STATE_DIR/pilot/before-wsl.json"
  set_session_value "stage" '"wait_wsl_restart"'

  say ""
  say "첫 번째 구간이 끝났습니다."
  say "1. Windows PowerShell에서 다음 명령을 실행하세요: wsl --shutdown"
  say "2. WSL, Herdr, OpenCode를 다시 시작하세요."
  say "3. THREADDOCK_GH_TOKEN을 다시 주입하세요."
  say "4. 이 저장소에서 실행하세요: ./scripts/single-run-pilot.sh --continue"
}

continue_pilot() {
  for command in jq git agentctl herdr opencode gh; do
    require_command "$command"
  done
  load_session
  [[ "$(jq -r '.stage // ""' "$SESSION_FILE")" == "wait_wsl_restart" ]] || fail "이 파일럿은 아직 WSL 재시작 단계가 아닙니다."
  [[ -n "${THREADDOCK_GH_TOKEN:-}" ]] || fail "THREADDOCK_GH_TOKEN이 없습니다. 비밀 저장소에서 다시 주입해 주세요."
  [[ -d "$REPO_ROOT" ]] || fail "원래 저장소를 찾을 수 없습니다: $REPO_ROOT"
  cd "$REPO_ROOT"

  local before_boot current_boot
  before_boot="$(jq -r '.bootIdBefore // "unknown"' "$SESSION_FILE")"
  current_boot="$(cat /proc/sys/kernel/random/boot_id 2>/dev/null || printf 'unknown')"
  if [[ "$before_boot" == "unknown" || "$before_boot" == "$current_boot" ]]; then
    fail "WSL 재시작을 확인할 수 없습니다. Windows PowerShell에서 wsl --shutdown을 실행한 뒤 다시 시도해 주세요."
  fi
  set_session_value "wslRestartPassed" 'true'

  snapshot_subset "$SNAPSHOT" >"$STATE_DIR/pilot/after-wsl.json"
  if diff -u "$STATE_DIR/pilot/before-wsl.json" "$STATE_DIR/pilot/after-wsl.json" >/dev/null; then
    set_session_value "snapshotDiffPassed" 'true'
  else
    fail "WSL 재시작 전후에 실행 ID나 Worktree 정보가 바뀌었습니다. 자동 쓰기를 중단합니다."
  fi

  resume_until reviewer_ready "독립 Reviewer 시작"
  run_checks
}

result_line() {
  local label="$1"
  local passed="$2"
  local detail="$3"
  local status="FAIL"
  if [[ "$passed" == "true" ]]; then
    status="PASS"
  else
    CHECK_FAILURES=$((CHECK_FAILURES + 1))
  fi
  printf '%-30s %s  %s\n' "$label" "$status" "$detail"
  printf -- '- %s: %s (%s)\n' "$label" "$status" "$detail" >>"$EVIDENCE_PATH"
}

run_checks() {
  load_session
  cd "$REPO_ROOT"
  for command in jq git herdr gh; do
    require_command "$command"
  done

  local owner repository marker issue_output issue_count issue_links ghes_host ghes_hostname
  local outage_label="GHES 연결 중단 복구" issue_label="GHES Issue 중복 방지"
  local herdr_output duplicate_count current_main current_status reviewer_ok outage_ok wsl_ok diff_ok
  local builder_identity reviewer_identity reviewer_request_id
  owner="$(read_json "$CONTRACT_PATH" '.repository.owner')"
  repository="$(read_json "$CONTRACT_PATH" '.repository.name')"
  ghes_host="$(read_json "$CONFIG_PATH" '.ghesHost')"
  ghes_hostname="${ghes_host#*://}"
  ghes_hostname="${ghes_hostname%%/*}"
  if [[ "$(jq -r '.simulateOutage // false' "$SESSION_FILE")" == true ]]; then
    outage_label="GitHub 연결 중단 복구"
    issue_label="GitHub Issue 중복 방지"
  fi
  marker="<!-- threaddock:td:$RUN -->"
  CHECK_FAILURES=0
  mkdir -p "$STATE_DIR/pilot-results"
  EVIDENCE_PATH="$STATE_DIR/pilot-results/$RUN.md"
  umask 077
  {
    printf '# Single-run 파일럿 결과\n\n'
    printf -- '- 실행일: %s\n' "$(date -Iseconds)"
    printf -- '- 실행 ID: %s\n' "$RUN"
    printf -- '- 저장소: %s/%s\n' "$owner" "$repository"
    printf -- '- 기준 SHA: %s\n\n' "$MAIN_BEFORE"
  } >"$EVIDENCE_PATH"

  say ""
  say "Single-run 파일럿 결과"
  say "────────────────────────────────────────────────────────────────"

  reviewer_ok=false
  if reviewer_ready; then reviewer_ok=true; fi
  result_line "Reviewer 준비 상태" "$reviewer_ok" "별도 Reviewer와 요청 ID 확인"
  builder_identity="$(jq -r '[.builder.name // "", .builderWorktree.workspaceId // "", .builderWorktree.paneId // "", .builderWorktree.path // ""] | join("/")' "$SNAPSHOT")"
  reviewer_identity="$(jq -r '[.reviewer.name // "", .reviewerWorktree.workspaceId // "", .reviewerWorktree.paneId // "", .reviewerWorktree.path // ""] | join("/")' "$SNAPSHOT")"
  reviewer_request_id="$(jq -r '.reviewerPrompt.requestId // ""' "$SNAPSHOT")"
  printf -- '- Builder ID: %s\n- Reviewer ID: %s\n- Reviewer 요청 ID: %s\n' "$builder_identity" "$reviewer_identity" "$reviewer_request_id" >>"$EVIDENCE_PATH"

  outage_ok="$(jq -r '.outagePassed // false' "$SESSION_FILE")"
  result_line "$outage_label" "$outage_ok" "등록 대기 상태에서 같은 실행 ID 유지"
  wsl_ok="$(jq -r '.wslRestartPassed // false' "$SESSION_FILE")"
  result_line "WSL 재시작 복구" "$wsl_ok" "재시작 여부 확인"
  diff_ok="$(jq -r '.snapshotDiffPassed // false' "$SESSION_FILE")"
  result_line "재시작 전후 ID 보존" "$diff_ok" "snapshot 핵심 ID 비교"

  issue_output=""
  if issue_output="$(gh api --hostname "$ghes_hostname" "repos/$owner/$repository/issues" --paginate 2>/dev/null)"; then
    issue_count="$(printf '%s\n' "$issue_output" | jq -s --arg marker "$marker" 'map(.[]) | map(select((.body // "") | contains($marker))) | length')"
    issue_links="$(printf '%s\n' "$issue_output" | jq -rs --arg marker "$marker" 'map(.[]) | map(select((.body // "") | contains($marker)) | .html_url) | join(", ")')"
  else
    issue_count="-1"
    issue_links="조회 실패"
  fi
  result_line "$issue_label" "$([[ "$issue_count" == "2" ]] && printf true || printf false)" "Parent+Child=$issue_count"
  printf -- '- Issue 링크: %s\n' "$issue_links" >>"$EVIDENCE_PATH"

  herdr_output=""
  if herdr_output="$(herdr worktree list --cwd "$REPO_ROOT" 2>/dev/null)"; then
    duplicate_count="$(printf '%s\n' "$herdr_output" | jq '[.result.worktrees[]? | select((.path // "") != "" and (.branch // "") != "")] | group_by([.path,.branch]) | map(select(length > 1)) | length')"
  else
    duplicate_count="-1"
  fi
  result_line "Herdr Worktree 중복 방지" "$([[ "$duplicate_count" == "0" ]] && printf true || printf false)" "중복=$duplicate_count"

  current_main="$(git rev-parse HEAD)"
  result_line "main 브랜치 보호" "$([[ "$current_main" == "$MAIN_BEFORE" ]] && printf true || printf false)" "$MAIN_BEFORE → $current_main"
  current_status="$(git status --short)"
  result_line "원본 작업 폴더 보호" "$([[ "$current_status" == "$CHECKOUT_BEFORE" ]] && printf true || printf false)" "변경 상태 유지"

  say "────────────────────────────────────────────────────────────────"
  if [[ $CHECK_FAILURES -eq 0 ]]; then
    say "전체 결과: PASS"
    printf '\n전체 결과: PASS\n' >>"$EVIDENCE_PATH"
    say "결과 파일: $EVIDENCE_PATH"
    say "현재 버전은 Reviewer 전달까지만 검증하며 PR이나 main 병합은 수행하지 않았습니다."
    return 0
  fi

  say "전체 결과: FAIL ($CHECK_FAILURES개 항목)"
  printf '\n전체 결과: FAIL (%d개 항목)\n' "$CHECK_FAILURES" >>"$EVIDENCE_PATH"
  say "결과 파일: $EVIDENCE_PATH"
  say "외부 쓰기를 더 진행하지 말고 이 결과 파일과 agentctl status $RUN을 개발 담당자에게 전달해 주세요."
  return 1
}

case "${1:-}" in
  -h|--help)
    usage
    ;;
  --continue)
    [[ $# -eq 1 ]] || { usage >&2; exit 2; }
    continue_pilot
    ;;
  --check)
    [[ $# -eq 1 ]] || { usage >&2; exit 2; }
    run_checks
    ;;
  --simulate-outage)
    [[ $# -eq 3 ]] || { usage >&2; exit 2; }
    start_pilot "$2" "$3" true
    ;;
  "")
    usage >&2
    exit 2
    ;;
  *)
    [[ $# -eq 2 ]] || { usage >&2; exit 2; }
    start_pilot "$1" "$2" false
    ;;
esac
