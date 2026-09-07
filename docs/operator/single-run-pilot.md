> **Legacy v1 참고 자료.** 기존 구현·파일럿의 절차와 관찰 기록이며 새 MVP의 실행 계획이나 완료 조건이 아닙니다. [현재 설계](../superpowers/specs/2026-09-07-project-workflow-mvp-design.md)를 먼저 따르십시오.

# Single-run 파일럿 빠른 시작

## 이 시험은 무엇을 하나요?

ThreadDock이 사내 Windows 11·WSL·GitHub Enterprise·OpenCode 환경에서 작업을
잃지 않고 이어갈 수 있는지 확인하는 **일회성 시험 운전**입니다.

시험용 저장소에 `pilot-result.txt` 파일 하나를 만드는 작업을 맡기고 다음을
확인합니다.

- GitHub Enterprise 연결이 잠시 끊겨도 실행 기록이 남는다.
- 연결을 복구하면 같은 실행을 이어간다.
- WSL을 껐다 켜도 Issue와 Worktree가 중복 생성되지 않는다.
- Builder와 Reviewer가 서로 다른 작업 문맥을 사용한다.
- 원본 `main` 브랜치와 작업 폴더를 변경하지 않는다.

이 시험은 **PR 생성, main 병합, CI/CD, 운영 배포를 하지 않습니다.** 현재
Single-run 단계의 끝인 독립 Reviewer 전달까지만 확인합니다.

## 시작 전에 준비할 것

사내 환경을 아는 개발 담당자와 함께 처음 한 번만 준비하는 것을 권장합니다.

- 삭제해도 되는 **전용 시험 저장소**
- Windows 11의 WSL
- 설치된 `agentctl`, `herdr`, `opencode`, `git`, `gh`, `jq`
- 설정 파일 `~/.config/threaddock/config.json`
- 시험 저장소에 Issue를 만들 수 있는 `THREADDOCK_GH_TOKEN`
- GitHub CLI(`gh`)의 대상 저장소 로그인 (GHES 또는 GitHub.com)

GitHub Enterprise에서 실제 연결 중단을 시험하려면 승인된 GHES API 연결 차단·복구
방법도 준비합니다. GitHub.com 시험 모드에서는 이 방법이 필요하지 않습니다.

제품 저장소나 실제 업무용 `main`에서 처음 시험하지 마세요. 토큰은 파일에
적지 않고, 회사 비밀 관리 도구를 통해 현재 WSL 터미널에만 주입합니다.

준비 여부는 다음 명령으로 확인할 수 있습니다.

```bash
command -v agentctl herdr opencode git gh jq
test -f "${THREADDOCK_CONFIG:-$HOME/.config/threaddock/config.json}"
test -n "$THREADDOCK_GH_TOKEN"
```

세 명령 모두 별도 오류 없이 끝나야 합니다.

## 실행 방법

### 1. 시험 시작

깨끗한 파일럿 저장소의 ThreadDock 소스 루트에서 아래 명령 하나를 실행합니다.
`OWNER`와 `REPOSITORY`를 실제 시험 저장소 값으로 바꿉니다.

```bash
./scripts/single-run-pilot.sh OWNER REPOSITORY
```

예를 들어 조직명이 `PDX`, 시험 저장소가 `thread-dock-pilot`이면 다음과 같습니다.

```bash
./scripts/single-run-pilot.sh PDX thread-dock-pilot
```

스크립트가 한국어로 다음 행동을 하나씩 안내합니다.

1. GHES API 연결을 잠시 차단합니다.
2. Enter를 누르면 연결 중단 상태에서 실행 ID가 보존되는지 확인합니다.
3. 안내에 따라 GHES 연결을 복구합니다.
4. Builder 작업과 integration 반영이 끝날 때까지 기다립니다.

사내 모델이 느리거나 잠시 멈추면 스크립트가 같은 실행을 제한된 횟수만큼 다시
이어갑니다. 실패하더라도 새로운 실행을 만들지 않으며 기존 기록을 보존합니다.

### 2. WSL 재시작

첫 구간이 끝나면 스크립트가 멈추고 다음 명령을 보여줍니다. Windows
PowerShell에서 실행합니다.

```powershell
wsl --shutdown
```

그다음 WSL, Herdr, OpenCode를 다시 시작하고 토큰을 회사 비밀 관리 도구로 다시
주입합니다. 토큰은 파일럿 기록에 저장되지 않습니다.

### 3. 같은 시험 계속하기

원래 파일럿 저장소로 돌아와 다음 명령을 실행합니다.

```bash
./scripts/single-run-pilot.sh --continue
```

스크립트가 WSL이 실제로 재시작됐는지 확인하고, 재시작 전후 실행 ID를 비교한 뒤
독립 Reviewer를 시작합니다. 마지막에는 아래와 같은 한국어 결과표가 표시됩니다.

```text
Single-run 파일럿 결과
────────────────────────────────────────────────────────────────
Reviewer 준비 상태             PASS  별도 Reviewer와 요청 ID 확인
GHES 연결 중단 복구             PASS  등록 대기 상태에서 같은 실행 ID 유지
WSL 재시작 복구                 PASS  재시작 여부 확인
재시작 전후 ID 보존             PASS  snapshot 핵심 ID 비교
GHES Issue 중복 방지            PASS  Parent+Child=2
Herdr Worktree 중복 방지        PASS  중복=0
main 브랜치 보호                PASS  변경 전 SHA → 변경 후 SHA
원본 작업 폴더 보호             PASS  변경 상태 유지
────────────────────────────────────────────────────────────────
전체 결과: PASS
```

결과표를 다시 보고 싶으면 다음 명령을 사용합니다.

```bash
./scripts/single-run-pilot.sh --check
```

### GitHub.com 시험 모드

GitHub.com 저장소에서 첫 등록 장애 구간만 안전하게 재현하려면 다음 명령을
사용합니다.

```bash
./scripts/single-run-pilot.sh --simulate-outage OWNER REPOSITORY
```

이 모드는 방화벽이나 네트워크를 차단하라고 묻지 않습니다. `CONFIG_PATH`에서
비밀이 없는 임시 설정을 만들고, 첫 `agentctl start`에만
`apiBase=http://127.0.0.1:1`을 주입해 등록을 실패시킵니다. 장애가 기록된 뒤
임시 설정은 삭제하며, 이후에는 원래 설정으로 같은 실행 ID를 `resume`합니다.
토큰은 설정 파일이나 출력에 기록하지 않습니다. 이후 WSL 재시작과 `--continue`,
`--check` 절차는 위와 같습니다.

## PASS 이후

`전체 결과: PASS`이면 현재 Single-run 단계가 사내 환경에서도 동작한다고 판단할
수 있습니다. 결과는 아래 위치에 Markdown 파일로 자동 저장됩니다.

```text
~/.local/state/threaddock/pilot-results/<실행-ID>.md
```

설정에서 `stateDir`을 바꿨다면 그 디렉터리 아래 `pilot-results`에 저장됩니다.
이 파일에는 실행 ID, Issue 링크, Builder·Reviewer 식별자, 중복 검사와 main 보호
결과만 들어갑니다. 토큰과 OpenCode 원문 대화는 기록하지 않습니다.

현재 Single-run 실행은 Reviewer 전달 상태에서 끝나므로 `agentctl cleanup`을
실행하지 마세요. 파일럿 저장소와 로컬 Worktree 정리는 후속 단계에서 안전한 정리
절차를 제공한 뒤 수행합니다.

## FAIL이 나오면

새 파일럿을 반복하거나 Issue·Worktree를 수동 삭제하지 마세요. `main`을 직접
병합하거나 Worktree를 강제로 지우지도 마세요. 다음 두 가지를 개발 담당자에게
전달하면 됩니다.

```bash
agentctl status <실행-ID>
```

- 화면에 표시된 `pilot-results/<실행-ID>.md` 파일
- 위 명령의 요약 결과

다음 상황에서는 스크립트가 즉시 외부 쓰기를 중단합니다.

- 실행 ID 또는 Builder·Reviewer 식별자가 재시작 전후 달라짐
- Parent/Child Issue가 두 개보다 많이 생성됨
- 같은 경로와 브랜치의 Worktree가 중복됨
- 원본 `main` SHA 또는 작업 폴더 상태가 변경됨
- WSL 재시작 여부를 확인할 수 없음

## 개발 담당자를 위한 저장 위치

문제 조사에 필요한 로컬 파일은 기본적으로 아래에 있습니다.

```text
~/.local/state/threaddock/pilot/session.json
~/.local/state/threaddock/pilot/contract.json
~/.local/state/threaddock/pilot/before-wsl.json
~/.local/state/threaddock/pilot/after-wsl.json
~/.local/state/threaddock/runs/<실행-ID>/run.json
~/.local/state/threaddock/pilot-results/<실행-ID>.md
```

`session.json`에는 토큰을 저장하지 않습니다. 장애 조사 중에도 토큰, 자격 증명,
OpenCode 원문 대화를 Issue나 결과 파일에 복사하지 마세요.
