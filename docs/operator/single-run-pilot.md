# Single-run GHES/OpenCode one-time pilot

이 절차는 사내 GHES와 OpenCode가 연결된 disposable 저장소에서 단일
Builder·독립 Reviewer 실행을 한 번 검증하기 위한 운영 절차입니다. 실제
GHES 자격 증명, 네트워크 단절, WSL 중단·재시작이 필요하므로 현재 Builder
환경에서는 실행하지 않았습니다. 이 문서는 production 배포 전 Operator가
실행하고 evidence를 Parent Issue에 남겨야 하는 prerequisite입니다.

## 사전 조건

- 삭제해도 되는 사내 disposable GHES 저장소와 테스트용 Issue 권한이 있습니다.
- WSL 안에 Go `1.27.0`, Herdr `0.8.2`, OpenCode와 Git이 설치되어 있습니다.
- `THREADDOCK_GH_TOKEN`은 사내 secret manager에서 현재 `agentctl` 프로세스에만
  주입하고, 토큰 값 자체를 문서·로그·Issue·evidence에 기록하거나 shell
  history에 남기지 않습니다.
- GHES의 기준 저장소에서 `main`은 보호되어 있고, 테스트 변경은 `main`에 직접 쓰지 않습니다.
- 시작 전후 canonical checkout에서 `git status --short`가 동일해야 합니다.

## 실행 순서

저장소 root에서 disposable contract의 repository와 `baseCommit`을 확인한 뒤,
깨끗한 WSL shell에서 실행합니다.

```bash
export THREADDOCK_CONFIG="$HOME/.config/threaddock/config.json"
# 사내 secret manager가 현재 agentctl 프로세스에만 THREADDOCK_GH_TOKEN을 주입

git status --short
agentctl start testdata/contracts/pilot.json
# 출력된 RUN 값을 아래 P로 기록
agentctl status "$RUN" --json
agentctl status "$RUN"
```

`start`가 반환한 RUN은 이후 모든 명령에서 같은 값을 사용합니다. Builder가
작업을 마치고 evidence를 남길 때까지 상태를 관찰합니다.

```bash
agentctl status "$RUN" --json > /tmp/threaddock-status.json
agentctl stop "$RUN"
agentctl status "$RUN" --json
agentctl resume "$RUN"
agentctl status "$RUN" --json
```

## 복구 드릴

1. GHES 연결을 한 번 끊고 `status` 또는 `resume`을 실행합니다. 연결을 복구한
   뒤 `resume`하고, 같은 RUN의 marker를 가진 Parent/Child Issue가 새로 생기지
   않았는지 확인합니다.
2. 실행 중 WSL을 한 번 중단·재시작합니다. 재시작 후 같은 `agentctl status`
   와 `agentctl resume`을 사용합니다. snapshot의 RUN, Parent Issue, Builder와
   Reviewer Worktree ID, prompt receipt ID와 commit SHA가 유지되어야 합니다.
3. Herdr worktree 목록에서 동일 branch/path가 두 번 생기지 않았는지 확인하고,
   GHES Issue 목록에서 `td:<RUN>` marker가 붙은 bundle을 대조합니다.
4. 최종 상태가 `reviewing`이고 Builder·Reviewer evidence가 있으며 `main`의
   HEAD가 변하지 않았는지 확인합니다.

중단·재시작은 실행 중 Agent를 강제 종료하거나 Worktree를 삭제하는 절차가
아닙니다. `stop`은 local orchestration만 pause하며 evidence와 Worktree를
그대로 둡니다.

## 정리

completed 상태가 된 뒤 7일이 지나기 전에는 cleanup을 실행하지 않습니다.
모든 managed Worktree가 clean인지 먼저 확인해야 하며, 하나라도 dirty하거나
경로·상태를 확인할 수 없으면 정리는 거부되어야 합니다. 경계 조건을 만족한
후에만 다음 명령을 실행합니다.

```bash
agentctl cleanup "$RUN"
```

## Evidence template

아래 template을 Parent Issue의 짧은 Verification/Blocker comment에 복사하되,
token, raw terminal transcript와 자격 증명은 넣지 않습니다.

```text
Single-run pilot: <YYYY-MM-DD>
Environment: GHES <host redacted>, WSL <distro>, Herdr <version>, OpenCode <version>
Contract/base: <contract path>, <base SHA>
Run ID: <RUN>
Parent Issue: <number/link>
Child Issues: <numbers/links>
Builder Worktree: <workspace/pane/path IDs>
Reviewer Worktree: <workspace/pane/path IDs>
Builder commit/evidence: <SHA and verification command + duration>
Reviewer evidence: <decision and verification summary>
GHES outage drill: <pass/fail; duplicate marker count>
WSL stop/restart drill: <pass/fail; IDs unchanged yes/no>
Duplicate Issue/Worktree: <none or count>
Main before/after: <SHA>/<SHA; unchanged yes/no>
Cleanup preflight: <completed, age, every Worktree clean>
Evidence links: <redacted paths/Issue comments>
Blocker/decision: <none or concise Korean action>
```

## 중단 기준

다음 중 하나라도 발생하면 즉시 더 이상의 외부 쓰기를 중단하고 Run ID와
현재 snapshot/event evidence만 보존합니다. 강제 삭제나 수동 main merge를
하지 말고 Operator에게 escalation합니다.

- 같은 RUN에서 Parent/Child Issue 또는 Worktree가 중복 생성됩니다.
- resume 후 RUN, marker, Worktree ID, prompt receipt ID 또는 commit SHA가 바뀝니다.
- GHES outage 뒤 marker reconcile 없이 새 Issue가 생성됩니다.
- `main`이 변경되거나 single-run 흐름이 main merge를 시도합니다.
- Builder patch/evidence에 token, secret, password 또는 private key가 보입니다.
- Worktree가 dirty인데 cleanup이 진행되거나, 상태 확인 없이 state가 삭제됩니다.
- 기대한 한국어 오류·exit code·`status --json` JSON-only 계약이 깨집니다.

이 세션에서는 실제 disposable GHES pilot을 실행하지 않았습니다. 이번 변경의
검증 근거는 fake/local E2E와 focused CLI gate이며, production 배포 전에 위
절차를 실제 환경에서 수행해야 합니다.
