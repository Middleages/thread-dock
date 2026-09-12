# 옛 실행 엔진 제거 완료 계획

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Task별 Luna 구현·focused 검증·fresh Sol review를 통과한 뒤 통합한다.

**Goal:** 최신 main의 Go/Wails Monitor와 프로젝트 Skills를 보존하고 옛 agentctl 실행 엔진과 전용 설정·테스트·fixture·운영 진입점을 순차 제거한다.
**Architecture:** `monitor → internal/runner`를 유지한다. GitHub 업무 원본, Herdr 실행 lifecycle, Agent/Skills 개발 조정은 바꾸지 않는다. CLI root를 끊은 뒤 orphan internal 군집을 삭제하며 대체 엔진이나 compatibility runtime은 만들지 않는다.
**Tech Stack:** Go 1.27.0, Wails 2.15.0, React/Vite, Git/gh.
**Spec:** [ADR 0008](../../adr/0008-github-first-skills-before-engine.md), [현재 Agent/Skill 설계](../specs/2026-09-11-thread-dock-agent-orchestration-design.md).

## Global Constraints

- base main `2db8f776d22afd849fcb9a1328f7e8aff9b72e50`; 사용자 settings/GHES/Toolbox/Markdown/open-only/Skills 변경을 보존한다. compact UI 설계는 이번 구현 대상이 아니다.
- Sol medium은 조정·문서·리뷰, Luna high는 제품 코드·테스트 수정. 실제 runtime model/effort 미노출은 unverified다. 자동 승격 금지.
- 기존 worktree·중단된 codex-runtime 실험과 미커밋 4개 파일·사용자 runtime 데이터는 reset/삭제/commit/재개하지 않는다. tracked source 삭제와 사용자 데이터 삭제를 구분한다.
- 모든 구현은 독립 worktree/branch. shared interface/CLI/Makefile/go.mod/go.sum 소유자는 동시에 하나다. 구현 worker 최대 3, reviewer 슬롯 유지.
- worker full suite 금지. 직접 영향 focused 검사, 마지막 통합 make check 한 번. 실패 시 원인/Task 귀속 및 tuple 무효화 근거를 기록한다.
- Windows/native·live GHES/Herdr·Skill pressure scenario는 Linux fixture/gate와 별개다. HERDR_ENV 위조와 옛 실패 invocation 재사용 금지.
- 승인된 Issue/Project/PR/push 작업은 한국어로 진행한다. main 병합은 별도 사용자 지시에 남긴다.
- Report의 implementation commitSHA와 report-only commit을 포함한 review candidate SHA를 구분한다. 구현 파일 수와 report 포함 전체 변경 파일 수도 구분한다. final candidate SHA는 자기 commit 본문에 삽입하지 않고 root ledger·외부 review packet에 고정한다.

## 종료 기준

1. Monitor가 요구하는 runner와 제품 template 검증용 internal/projecttemplate 이외의 옛 실행 엔진 package와 CLI·전용 fixture·설정·scripts가 없다.
2. 남길 각 package와 asset에는 현재 소비자가 있거나 명시적 제품 목적이 있다. 모호한 것을 조용히 지우지 않는다.
3. legacy docs는 역사임을 표시하고 active quickstart/README/build가 제거된 경로를 요구하지 않는다. 사용자 데이터는 그대로 남는다.
4. Task reviews와 마지막 통합 gate/전체 review를 통과한 PR을 준비한다. native 미검증은 별도 Issue에 남긴다.

### Task 1: 옛 CLI와 pilot 진입점 제거

- taskId: `retirement-root-cut`
- baseSHA: `91d37632723ffca8114700498e43efcdba7f0ec8`
- deps: Linux/Windows inventory에서 Monitor의 유일 internal dependency가 runner임을 확인, root 직렬 interface 결정
- ownedPaths: `cmd/agentctl/`, `internal/cli/`, `internal/config/`, `internal/pilot/`, `scripts/single-run-pilot.sh`, `Makefile`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-1-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-root-cut`
- branch: `agent/retirement-root-cut`
- forbiddenPaths: 소유 밖 전체, 특히 `monitor/`, `internal/runner/`, `project-template/`, `go.mod`, `go.sum`, docs, 다른 worktree와 사용자 데이터
- interface: agentctl binary/CLI와 옛 실행 설정·pilot을 제거한다. 나머지 내부 엔진은 이 단계에서 보존한다. Monitor·runner·새 settings/GHES/Toolbox·canonical Skills 계약은 불변이다.
- acceptance: owned legacy source/test와 single-run script 삭제; 남은 import 0 확인; Makefile Go discovery는 사라진 cmd를 요구하지 않으며 pilot bash 검사를 제거한다. focused 예시는 살아 있는 monitor/runner로 바꾼다. frontend/template/Go 전체 gate는 그대로 보존한다.
- tests: Go1.27 `go list ./...`; `go test ./monitor ./internal/projecttemplate`; `go vet ./monitor ./internal/runner`; `make template-check`; `make -n check`; refs/gofmt/diff. 별도 TMPDIR tmpfs 사용. full suite 금지.
- result: changedFiles, commitSHA, executedCommands(환경 포함), outcomes, unverified, blockers를 report에 기록한다.

- [x] production/test/config/docs caller 근거와 정확 삭제 파일을 report에 기록한다.
- [x] apply_patch로 owned 경로만 삭제하고 Makefile discovery/pilot/focused 예시를 정리한다.
- [x] focused 검증·self-review 후 commit한다. 삭제한 전용 동작의 대체 테스트를 만들지 않는다.
- [x] fresh Sol이 고정 SHA/diff를 검토하고 root가 통합한다.

### Task 2: v1 orchestrator root 제거

- taskId: `retirement-v1-root`
- baseSHA: Task 1 reviewed 통합 SHA(배정 시 ledger에 exact SHA 고정)
- deps: Task 1. 이후 production/test reverse importer 0.
- ownedPaths: `internal/orchestrator/`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-2-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v1-root`
- branch: `agent/retirement-v1-root`
- forbiddenPaths: 소유 밖 모든 경로, 특히 Task 3·Monitor·runner·project-template·Makefile·go.mod/go.sum·사용자 데이터
- interface: orphan orchestrator 전체 제거. v1 state/contract와 다른 downstream package는 이 단계에서 유지.
- acceptance: 17파일의 source/test 삭제, 외부 importer 0, 남은 graph 유효. 로직 이동·대체 엔진 없음.
- tests: `go list -deps ./...`, Linux/Windows `go list ./monitor`, `git diff --check`, 외부 import 검색. 단순 orphan 삭제이며 제품 테스트를 재실행하지 않는다.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] manifest/importer 증명 → apply_patch 삭제 → focused 정적 검사 → self-review/commit → fresh Task review.

### Task 3: v2 업무·발행·집계 root 제거

- taskId: `retirement-v2-entry-roots`
- baseSHA: Task 1 reviewed 통합 SHA(배정 시 ledger에 exact SHA 고정)
- deps: Task 1; Task 2와 파일·interface 독립.
- ownedPaths: `internal/githubpublication/`, `internal/workflow/`, `internal/workrun/`, `internal/monitor/`, `internal/coordinator/integration_test.go`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-3-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v2-entry-roots`
- branch: `agent/retirement-v2-entry-roots`
- forbiddenPaths: 소유 밖 모든 경로, 특히 Task 2·나머지 coordinator·제품 monitor·runner·project-template·Makefile·go.mod/go.sum
- interface: 옛 Work/Publication 및 aggregate 제거. internal/monitor는 옛 집계 package이며 제품 monitor/와 다르다. coordinator integration_test의 유일 workflow 외부-test import를 함께 제거.
- acceptance: source/test 삭제 후 유효 graph, 남은 coordinator 동작 유지. UI/server/대체 상태 모델 추가 금지.
- tests: `go list -deps ./...`; 별도 tmpfs `go test ./internal/coordinator`; importer·diff 검사. full suite 금지.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] manifest/importer 증명 → apply_patch 삭제 → focused 검사 → self-review/commit → fresh Task review.

### Task 4: coordinator와 Herdr 실행 adapter 제거

- taskId: `retirement-v2-adapters`
- baseSHA: Task 3 reviewed 통합 후 exact SHA를 ledger에 고정
- deps: Task 3; root 통합 직렬
- ownedPaths: `internal/herdr/`, `internal/coordinator/`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-4-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v2-adapters`
- branch: `agent/retirement-v2-adapters`
- forbiddenPaths: 소유 밖 전체, 특히 제품 monitor/herdr.go·runner·project-template·shared dependency 파일
- interface: Go runtime coordinator와 옛 Herdr 실행 adapter를 제거. 제품 Herdr 읽기 구현과 Skills의 Herdr 소유권은 보존.
- acceptance: coordinator의 유일 외부 소비자가 같은 Task herdr임을 증명하고 둘을 제거. session/worktree를 실제 종료·삭제하지 않음.
- tests: `go list -deps ./...`, Linux/Windows `go list ./monitor`, 외부 import/diff 확인.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] importer 증명 → source/test 삭제 → 정적 검사 → self-review/commit → fresh Task review.

### Task 5: 옛 GitHub 쓰기·worktree adapter 제거

- taskId: `retirement-git-adapters`
- baseSHA: Task 2·3·4 reviewed 통합 후 exact SHA를 ledger에 고정
- deps: Task 2·3·4
- ownedPaths: `internal/github/`, `internal/worktree/`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-5-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-git-adapters`
- branch: `agent/retirement-git-adapters`
- forbiddenPaths: 소유 밖 전체, 실제 Git worktree·사용자 파일·제품 Monitor·runner
- interface: 두 orphan adapter package와 전용 테스트만 제거. 제품 GitHub 조회·WSL 경로/링크 열기는 monitor 내부에 보존.
- acceptance: 각 package 외부 importer 0, 실제 git/herdr cleanup 실행 0, source 삭제만 수행.
- tests: `go list -deps ./...`, Linux/Windows `go list ./monitor`, 외부 import/diff 확인.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] importer 증명 → source/test 삭제 → 정적 검사 → self-review/commit → fresh Task review.

### Task 6: v1 상태·계약·정책 foundations 제거

- taskId: `retirement-v1-foundations`
- baseSHA: Task 5 reviewed 통합 후 exact SHA를 ledger에 고정
- deps: Task 4·5; Task 7과 shared 부모 디렉터리가 있으므로 직렬
- ownedPaths: `internal/mergegate/`, `internal/recovery/`, `internal/retirement/`, `internal/review/`, `internal/scheduler/`, `internal/state/lock.go`, `internal/state/store.go`, `internal/state/store_test.go`, `internal/state/types.go`, `internal/contract/codec.go`, `internal/contract/codec_test.go`, `internal/contract/preview.go`, `internal/contract/preview_test.go`, `internal/contract/types.go`, `internal/contract/validate.go`, `internal/contract/validate_test.go`, `internal/testfixture/`, `internal/version/`, `testdata/contracts/valid.json`, `testdata/contracts/invalid-overlap.json`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-6-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v1-foundations`
- branch: `agent/retirement-v1-foundations`
- forbiddenPaths: 소유 밖 전체, 특히 internal/state/v2·internal/contract/v2·testdata/contracts/v2·사용자 snapshot/event/config 파일
- interface: v1 foundations만 제거; 남은 v2의 부모 경로 전체를 삭제하지 않는다.
- acceptance: 폐기 package/fixture의 외부 소비자 0 및 v2 graph 유지, 사용자 runtime 데이터 변경 없음.
- tests: `go list -deps ./...`, `go test ./internal/state/v2 ./internal/contract/v2`(고유 tmpfs), manifest/import/fixture/diff 검사.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] importer/fixture 증명 → exact source/test/fixture 삭제 → focused 검사 → self-review/commit → fresh Task review.

### Task 7: v2 상태·runtime foundations 제거

- taskId: `retirement-v2-foundations`
- baseSHA: Task 6 reviewed 통합 후 exact SHA를 ledger에 고정
- deps: Task 6
- ownedPaths: `internal/registry/`, `internal/runtime/`, `internal/state/v2/`, `internal/contract/v2/`, `testdata/contracts/v2/`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-7-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-v2-foundations`
- branch: `agent/retirement-v2-foundations`
- forbiddenPaths: 소유 밖 전체, 특히 실제 local Work store/registry와 사용자 데이터
- interface: v2 Work/Contract/Invocation/Artifact 모델·registry/runtime 코드를 제거하고 대체하지 않는다.
- acceptance: 외부 importer/fixture consumer 0, 소스와 전용 fixture만 삭제.
- tests: `go list -deps ./...`, Linux/Windows `go list ./monitor`, 외부 import/fixture/diff 검사.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] importer/fixture 증명 → source/test/fixture 삭제 → 정적 검사 → self-review/commit → fresh Task review.

### Task 8: 최종 orphan utility 정리

- taskId: `retirement-orphan-utilities`
- baseSHA: Task 7 reviewed 통합 후 exact SHA를 ledger에 고정
- deps: Task 7
- ownedPaths: `internal/dag/`, `internal/pathscope/`, `internal/opencodeagent/`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-8-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-orphan-utilities`
- branch: `agent/retirement-orphan-utilities`
- forbiddenPaths: 소유 밖 전체, 특히 runner·projecttemplate·제품 monitor·템플릿·go.mod/go.sum
- interface: orphan utility 삭제. internal/runner는 Monitor process boundary, internal/projecttemplate은 현재 template 검증 seam이므로 보존.
- acceptance: 남은 Go packages가 monitor/internal/runner/internal/projecttemplate뿐임을 확인한다. dependency/module 또는 새 구현 추가 없음.
- tests: `go list -deps ./...`, `go test ./internal/runner`, `go list ./...`, Windows `go list ./monitor`, refs/diff. template/Monitor tests는 최종 root gate에서 통합.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 report에 기록.
- [x] importer 증명 → source/test 삭제 → focused 검사 → self-review/commit → fresh Task review.

### 최종 문서·통합

root는 역사 문서를 명시적으로 분류하고 active docs의 legacy 실행 안내를 제거한다. go.mod/go.sum은 Wails graph가 소비하므로 이유 없이 tidy/업그레이드하지 않는다. 전체 internal graph와 Git diff·링크 검증, 마지막 `make check`, fresh 전체 리뷰를 완료한 후 한국어 PR과 Issue/보드 기록을 갱신한다. main 병합은 별도 사용자 지시를 기다린다.

### Task 9: 최신 main 업무 표에 맞춰 기존 선택자 회귀 테스트 정정

- taskId: `retirement-gate-selector-fix`
- baseSHA: 계획 commit 후 배정 시 ledger/외부 packet에 exact SHA 고정
- deps: e630c5a의 cache 환경 focused 실행에서 실제 assertion 실패 확인(28 passed/1 failed, startup 오류 없음)
- ownedPaths: `monitor/frontend/src/monitor.test.tsx`의 `uses the selected work identity for detail and action links after a project shrinks` 사례 선택자 한 곳; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-9-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-gate-selector-fix`
- branch: `agent/retirement-gate-selector-fix`
- forbiddenPaths: 모든 production 코드·나머지 테스트·config/dependency/timeout/isolation·사용자 데이터
- interface: 새 WorkTable은 row의 번호·종류·제목·상태·시각을 accessible name에 포함한다. 선택 전환과 링크 결과의 계약은 불변이다.
- acceptance: `getByRole('tab', { name: 'Second work' })`를 해당 제목을 포함하는 row 탐색인 `name: /Second work/`로 한정 수정. role·후속 click·heading/link/assertion은 그대로 보존한다. 테스트 skip·제거·timeout 변경·생산 코드 변경 없음.
- tests: 기존 e630c5a cached focused 로그를 RED로 채택한다. 새 source에서 같은 failing test 이름만 `npm --prefix monitor/frontend test -- src/monitor.test.tsx -t 'uses the selected work identity for detail and action links after a project shrinks'`로 검증한다. Node26.8.1, VITEST_MAX_WORKERS=1, dedicated NODE_COMPILE_CACHE, tmpfs 사용. 나머지 테스트는 최종 gate에 포함하며 worker full suite 금지.
- result: implementation commitSHA와 root가 고정할 final candidate를 구분한다. changedFiles에는 test1+report1을 구분하고 commands/환경/outcomes/unverified/blockers 및 기존 failing proof를 기록한다.
- [x] 기존 실패 원인/새 accessible name 근거를 읽고 apply_patch로 선택자 한 곳만 수정한다.
- [x] focused GREEN·diff/self-review·commit 후 fresh Sol에게 고정 diff를 검토받는다.

### Task 10: 전체 리뷰의 Makefile 오류 안내 정정

- taskId: `retirement-final-guidance-fix`
- baseSHA: 배정 시 ledger·외부 packet에 exact SHA 고정
- deps: 전체 리뷰 ed12903의 blocking finding, Task1 acceptance의 누락
- ownedPaths: `Makefile`의 test-focused/vet-focused PKGS 미지정 오류 안내 두 문자열; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-10-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-final-guidance-fix`
- branch: `agent/retirement-final-guidance-fix`
- forbiddenPaths: 나머지 Makefile recipe/변수, 모든 제품·테스트·설정·dependency, 다른 worktree/사용자 데이터
- interface: PKGS 미지정은 기존 exit2로 거절하되 존재하는 package 예시를 보여준다. 실제 test/check 실행 계약은 불변이다.
- acceptance: test-focused의 오류 예시는 `./monitor`, vet-focused는 `./internal/runner`로 변경. `./internal/contract` 안내는 Makefile에0개. 다른 줄은 변경하지 않는다.
- tests: 수정 전후 실제 PKGS 없는 두 make target의 exit2와 출력 예시 검증. 같은 cwd에서 base Makefile과 수정 Makefile의 `make -n check` 출력을 비교해 동일함을 증명. source/diff checks만 수행하며 Go/UI/full gate 재실행 금지.
- result: implementation commitSHA와 외부 final candidate를 구분하고 총changedFiles2(Makefile+report), commands/exit/outcomes/unverified/blockers를 기록.
- [ ] 실제 오류 안내 RED → 두 문자열 apply_patch 정정 → 오류 안내 GREEN 및 check dry-run 동등성 확인.
- [ ] self-review/commit 후 동일 전체 reviewer에게 fix diff만 재검토받는다.
