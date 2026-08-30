# ThreadDock 다음 세션 Handoff

이 문서는 다음 개발 세션의 시작점이다. 먼저 현재 상태를 검증한 뒤 후속 계획을
실행한다. 완료된 Single-run 작업을 다시 구현하지 않는다.

## 현재 기준선

- Repository: `https://github.com/Middleages/thread-dock`
- Branch: `main`
- Single-run merge commit: `2f6ccb06791a0e6b2579af4fc13022b4bbf0062e`
- Parent Issue: `#15` closed
- Parent PR: `#36` merged
- Open Issue / PR: `0 / 0`
- Working tree: clean, `main...origin/main`
- Post-merge gate: Go 1.27 + jq `make check` PASS

Single-run은 GitHub.com과 GHES endpoint profile, registration outage 복구,
중단 전·WSL 후 같은 RUN 재개, Builder/Reviewer Worktree 분리와 strict evidence
검증까지 구현했다. 현재 단계는 Reviewer packet 전달에서 끝난다. 제품 PR 생성,
CI, main 자동 병합과 배포는 아직 수행하지 않는다.

## 최종 파일럿 근거

- Pilot repository: `https://github.com/middleages-dev-system/thread-dock-pilot`
- RUN: `run-1787939711953381828-1`
- Frozen source HEAD: `4ea5ca00ed1f788280ea1aab3f53b4a9dcc880c7`
- Result: 전체 PASS
- Evidence:
  `/home/appuser/.local/state/threaddock-github-com-final-pilot/pilot-results/run-1787939711953381828-1.md`
- Frozen checkout: `/home/appuser/thread-dock-final-pilot`
- Config: `/home/appuser/.config/threaddock/github-com-final-pilot.json`

확인된 항목은 simulated GitHub outage, 같은 RUN 회수, 실제 WSL boot 변경,
snapshot identity 보존, Parent/Child marker count `2`, Worktree duplicate `0`,
Builder/Reviewer terminal·workspace 분리, source SHA·checkout 불변과 evidence secret
scan이다.

파일럿 RUN과 Worktree는 증거다. 현재 phase가 `reviewing`이므로 `agentctl cleanup`,
강제 Worktree 삭제, Issue 삭제를 실행하지 않는다. 개발용 `.worktrees/issue-*`만
이미 safe removal했다.

## 환경 결정

- OpenCode `1.18.25`, Herdr `0.8.2`, jq `1.8.2`가 사용자 영역에 설치돼 있다.
- OpenCode 기본 모델은
  `/home/appuser/.config/opencode/opencode.json`에서 `opencode/big-pickle`이다.
- MiMo 무료 provider가 live pilot 중 HTTP 404를 반환했다. 같은 terminal identity와
  RUN에서 Big Pickle로 바꿔 작업을 회수했다.
- GitHub.com public config는 canonical
  `https://github.com` + `https://api.github.com`만 허용한다.
- `--simulate-outage`는 첫 start에만 양 endpoint가
  `http://127.0.0.1:1`인 secret-free 임시 config를 사용한다.
- Single-run에서 ProjectV2는 사용하지 않는다. 파일럿 config의 Project ID 값은
  validation placeholder이므로 Project 자동화를 검증한 근거로 사용하지 않는다.

Herdr 0.8.2 OpenCode 응답에는 provider session ID가 없다. 이 경우
`herdr-terminal:<terminal_id>`를 execution identity로 사용한다. 같은 terminal에서
프로세스를 수동 교체하면 provider session보다 구분이 약해지는 trade-off는
`herdr-security-review.md`와 Issue `#23`에 기록돼 있다.

## 다음 목표

다음 increment는
[`docs/superpowers/plans/2026-08-28-threaddock-parallel-merge.md`](docs/superpowers/plans/2026-08-28-threaddock-parallel-merge.md)다.
두 Builder 병렬 실행, bounded repair/recovery, Draft PR, Merge Gate와 감독형 main
자동 병합을 구현한다.

첫 우선순위는 slow/stopped model recovery supervisor다. live pilot에서 provider
404 후 사람이 모델을 바꾸고 같은 RUN을 이어야 했기 때문이다. 기존 계획의 정확한
계속 지시, 진전 감지, 연속 3회 recovery cap을 현재 Herdr terminal identity와
evidence parser interface에 맞춰 먼저 재검증한다.

## 다음 세션 실행 순서

1. `git status --short --branch`, `git rev-parse HEAD`, GitHub open Issue/PR을 확인한다.
2. 이 문서, `CONTEXT.md`, parallel-merge 계획, ADR
   `docs/adr/0004-tiered-test-cycle.md`를 읽는다.
3. 기존 parallel-merge 계획이 이번 increment에서 변경된 GitHub endpoint,
   agent naming, terminal identity, evidence normalization interface와 일치하는지
   점검하고 필요한 계획 수정만 한다.
4. Main Sol이 Parent Issue와 독립 Child Issue를 등록한다. 병렬 가능한 작업만
   별도 Worktree로 분리한다.
5. 구현은 5.6 Luna, 각 Child와 Parent 전체 리뷰는 구현 문맥과 분리된 fresh Sol을
   사용한다. Decision, Blocker, Review, Verification은 Issue/PR comment로 남긴다.
6. focused test → Task gate → wave-end/full suite 순서로 실행한다. 매 Task마다 full
   suite를 반복하지 않는다.
7. final pilot 전에는 고정 checkout을 사용한다. 개발 중 움직인 checkout의 pilot은
   진단 근거일 뿐 Merge Gate PASS로 사용하지 않는다.

## 시작 검증

호스트에 Go가 없으면 기존 방식대로 Go 1.27 Docker image에 repository를 mount하고
jq를 임시 설치해 `make check`를 실행한다. 토큰, raw OpenCode transcript와 자격
증명은 문서, Issue, PR 또는 evidence에 복사하지 않는다.
