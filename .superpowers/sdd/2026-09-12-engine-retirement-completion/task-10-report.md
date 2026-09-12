# Task 10 report: retirement-final-guidance-fix

## Packet

- taskId: `retirement-final-guidance-fix`
- baseSHA: `74868658277a32a018918c2521b2e8a2f703e895`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-final-guidance-fix`
- branch: `agent/retirement-final-guidance-fix`
- implementation commitSHA: `fe3a843f2dba7f2b6f1831f9f82a39f3f49188ca`
- external final candidate: worker scope에서는 알 수 없으며 root integration에서 기록할 값

## Change

`Makefile`의 두 PKGS 미지정 오류 안내 문자열만 수정했다.

- `test-focused`: `./internal/contract` → `./monitor`
- `vet-focused`: `./internal/contract` → `./internal/runner`

recipe, 변수, 제품 코드, 테스트, 설정, dependency는 변경하지 않았다.

## RED/GREEN evidence

수정 전 base Makefile에서 `unset PKGS; make test-focused`와 `make vet-focused`를
실행했다. 두 target 모두 exit 2였고 stale `PKGS="./internal/contract"` 안내를 출력했다.

수정 후 같은 cwd에서 동일 명령을 재실행했다. 두 target 모두 exit 2를 유지하면서 각각
`PKGS="./monitor"`, `PKGS="./internal/runner"`를 출력했다.

## Verification commands and outcomes

- `unset PKGS; make test-focused`: PASS; expected exit 2 and `./monitor` example
- `unset PKGS; make vet-focused`: PASS; expected exit 2 and `./internal/runner` example
- `git show 74868658277a32a018918c2521b2e8a2f703e895:Makefile | make -n -f - check`와
  `make -n check`: PASS; same cwd output byte-identical (`cmp -s`)
- `rg -n 'internal/contract' Makefile`: PASS; no matches
- `git diff --check`: PASS
- `git diff --unified=0 -- Makefile` changed-line assertion: PASS; exactly two added/two removed lines
- `git diff --name-only`: PASS; implementation diff only `Makefile`

Go/UI/full gate 및 build는 packet 지시에 따라 재실행하지 않았다. 기존 gate
`1e427ed`의 성공과 recipe 동등성은 별도 근거로 보존한다.

## Self-review

- acceptance의 두 예시만 바뀌었고 다른 Makefile 줄은 변경되지 않았다.
- PKGS 미지정 exit 2 계약은 유지됐다.
- `./internal/contract`는 Makefile에 남아 있지 않다.
- changedFiles는 implementation 1개이며 report를 포함한 총 changedFiles는 2개다.

## Result

- changedFiles: 2 (`Makefile`, report)
- commitSHA: `fe3a843f2dba7f2b6f1831f9f82a39f3f49188ca` (implementation)
- externalFinalCandidate: root integration 후 결정
- executedCommands: RED/GREEN 두 make target, base/current `make -n check` 비교,
  stale reference/source 범위 검사, `git diff --check`
- outcomes: 모든 focused acceptance 검증 PASS
- unverified: root external final candidate SHA, Go/UI/full gate 재실행 결과(의도적으로 생략),
  runtime model/effort identity
- blockers: 없음
