# Task 9 report: latest WorkTable row selector correction

## Packet

- taskId: `retirement-gate-selector-fix`
- baseSHA: `e7ce6e6a9c7dc7d7665f8882e989ae34a442146e`
- deps: `e630c5a` cache 환경 focused 실행에서 startup 오류 없이 28 passed/1 failed를 확인한 RED 로그
- ownedPaths: `monitor/frontend/src/monitor.test.tsx`의 `uses the selected work identity for detail and action links after a project shrinks` 사례 선택자 한 곳; 이 report
- worktree: `/home/appuser/dev_system/.worktrees/retirement-gate-selector-fix`
- branch: `agent/retirement-gate-selector-fix`
- forbiddenPaths: 모든 production 코드·나머지 테스트·config/dependency/timeout/isolation·사용자 데이터
- interface: 새 `WorkTable` row의 accessible name은 번호·종류·제목·상태·시각을 포함하며, 선택 전환과 후속 링크 계약은 불변이다.
- acceptance: `getByRole('tab', { name: 'Second work' })`를 해당 제목을 포함하는 `name: /Second work/` 탐색으로만 수정하고 role·후속 click·heading/link/assertion은 보존한다.
- tests: 기존 `e630c5a` cached focused 로그를 RED로 채택하고, 지정된 테스트 이름만 Node 26.8.1/Vitest 단일 worker/tmpfs 환경에서 실행한다. full suite/browser/devserver/new test는 실행하지 않는다.

## Root cause and change

기존 RED는 `Second work`를 exact accessible name으로 찾지 못했다. 현재
`WorkTable.tsx`는 GitHub metadata가 없는 legacy row에만 `role="tab"`을 부여하고,
그 row의 accessible name은 제목뿐 아니라 번호·종류·상태·수정시각을 함께 포함한다.
따라서 production/component/mock은 건드리지 않고 해당 test의 name matcher 한 곳만
`/Second work/`로 넓혔다. role, 선택 후 project click, heading/link/button assertion은
그대로 보존했다.

## Verification

기존 RED 증거는 재실행하지 않았다.

```text
e630c5a RED log: monitor.test.tsx 24 tests, 1 failed; the selected-work test failed with
TestingLibraryElementError: Unable to find an accessible element with role "tab" and name
"Second work". Startup completed without an error.
```

첫 focused GREEN 호출은 brief의 cache 경로와 단일 worker를 사용했으나 실행 래퍼가
Vitest의 최종 요약/종료 상태를 출력하지 않아 통과 근거로 채택하지 않았다. 프로세스가
남아 있지 않음은 확인했지만 그 호출의 종료 상태는 unverified로 남겼다.

동일 테스트 이름을 새 전용 tmpfs/cache tuple로 한 번 재검증했다.

```sh
TMPDIR=$(mktemp -d /dev/shm/threaddock-retirement-selector-retry.XXXXXX) \
NODE_COMPILE_CACHE=/dev/shm/threaddock-node-compile.retirement-selector-retry \
VITEST_MAX_WORKERS=1 timeout 90s \
npm --prefix monitor/frontend test -- src/monitor.test.tsx -t \
'uses the selected work identity for detail and action links after a project shrinks'
```

PASS, explicit `__TEST_EXIT_STATUS=0`: 1 test file passed, 1 test passed and 23 skipped,
Vitest duration 6.56s. Node `v26.8.1`; `monitor/frontend/node_modules` was the prepared
symlink to the shared identical-lock dependency tree and was not staged or modified.

```text
git diff --check e7ce6e6a9c7dc7d7665f8882e989ae34a442146e..HEAD
```

PASS (exit 0).

## Self-review

- [x] The implementation diff is exactly one selector replacement in the assigned test.
- [x] No production/component/mock/other assertion/timeout/isolation/dependency path changed.
- [x] The existing role, subsequent project selection click, heading assertion, link assertion,
  and GitHub action assertion remain unchanged.
- [x] No test was added, skipped, removed, or broadened beyond the requested test name.
- [x] The prepared untracked `node_modules` symlink was not included in either commit.
- [x] Focused GREEN evidence is explicit and the earlier ambiguous wrapper result is not
  represented as a pass.

## Result

- changedFiles: `monitor/frontend/src/monitor.test.tsx` (1 selector line); this report (1 metadata file)
- implementationSHA: `4bfa9653b42886be32dae40a00d2a83385019b8a`
- result.commitSHA: `4bfa9653b42886be32dae40a00d2a83385019b8a` (test-only implementation commit; report is a follow-up metadata commit)
- finalCandidateSHA: root ledger and external review packet should fix the SHA after this report-only commit; this report avoids a self-referential SHA.
- executedCommands: brief/AGENTS/skill reads; source and RED-log inspection; `apply_patch`; focused GREEN retry above; process-state check; `git diff --check`; exact owned diff/status review; implementation `git commit`.
- outcomes: accessible-name root cause confirmed; one test selector corrected; focused selected test passed with explicit exit 0; implementation commit contains only the owned test file.
- unverified: first wrapper invocation's final exit status; fresh Sol `td_reviewer` task-review; root's final integrated `make check`; Windows native Wails execution; runtime model/effort identity.
- blockers: none.
