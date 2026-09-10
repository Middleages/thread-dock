# 옛 실행 엔진 첫 삭제 계획

> **For agentic workers:** Use superpowers:subagent-driven-development. 사용자 승인된 Luna 구현 → focused tests/self-review/commit → fresh Sol review → Luna fix 흐름으로 진행한다.

**Goal:** 현재 Monitor가 사용하지 않는 옛 조회 bridge 두 파일을 제거하고 후속 엔진 삭제 경계를 기록한다.
**Architecture:** Go/Wails Monitor의 `internal/runner` 직접 호출을 유지한다. 옛 `internal/monitorcli`는 `agentctl project status`를 읽던 별도 adapter이며 importer가 없으므로 제거한다.
**Tech Stack:** Go 1.27.0, Wails v2.15.0, React/Vite 유지.
**Spec:** [현재 Go/Wails 설계](../specs/2026-09-10-herdr-first-usable-workflow-design.md), [ADR 0008](../../adr/0008-github-first-skills-before-engine.md).

## Global Constraints

- 제품은 Windows Go/Wails 모니터와 기존 React 화면이다. Node/HTTP 서버·새 실행 엔진을 추가하지 않는다.
- public/shared interface 변경은 없음. Go/TS wire, `go.mod`, `go.sum`, runner, CLI 공유 파일은 변경하지 않는다.
- 중단된 codex-runtime과 기존 worktree·Windows staging은 수정·삭제·재개하지 않는다.
- 구현은 Luna high, fresh 리뷰는 Sol medium. 실제 runtime model/effort는 노출 없으면 unverified. 자동 승격 금지.
- focused 검증만 worker가 수행한다. 새 통합 code SHA의 마지막 `make check`는 root가 한 번 수행한다.
- main 병합은 사용자에게 남긴다. 승인된 push·Issue·PR 기록은 한국어로 수행한다.

## Task 1: 사용되지 않는 monitorcli bridge 제거

- taskId: `engine-retirement-slice1`
- baseSHA: `3f4bebf08bb099637d1b2bfff44a9ba56e91a5df`
- deps: `engine-inventory`의 importer 0개 조사와 root의 no-interface-change 결정
- ownedPaths: `internal/monitorcli/client.go`, `internal/monitorcli/client_test.go`, `.superpowers/sdd/engine-retirement-slice1/task-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice1`
- branch: `agent/engine-retirement-slice1`
- forbiddenPaths: 소유 밖 모든 경로. 특히 `monitor/**`, `internal/runner/**`, `internal/monitor/**`, `cmd/agentctl/**`, `internal/config/**`, 공유 타입, `go.mod`, `go.sum`, 다른 worktree.
- interface: 소비자 없는 internal package 삭제. 현재 Go/Wails runner·링크·경로 동작과 public/shared 계약 변경 없음.
- acceptance: 두 파일 제거, Go import 부재 증명, 현재 Monitor 테스트/vet 통과, 코드 삭제 외 변경 없음. 전용 테스트는 삭제된 bridge만 검사하므로 함께 삭제한다.
- tests: 삭제 전 import/reference 증거를 report에 남기고 삭제 후 `go test ./monitor`, `go vet ./monitor`, `git diff --check`, `test ! -e internal/monitorcli` 수행. Go 경로는 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go` 사용 가능.
- result: `changedFiles`, `commitSHA`, `executedCommands`, `outcomes`, `unverified`, `blockers`를 report에 기록한다. self-review 후 commit하고 fresh Sol review로 전달한다.

- [x] base 파일과 import/test/config/docs 참조 확인. Linux·Windows 그래프의 importer 0개 확인.
- [x] `apply_patch`로 `internal/monitorcli/client.go`, `internal/monitorcli/client_test.go` 삭제.
- [x] 영향받는 기존 Monitor 테스트와 vet 통과, self-review/commit 완료.
- [x] fresh Sol이 고정 SHA `3d89c93`에서 spec/code quality 모두 ACCEPT. 수정 요구 없음.

## 문서·통합

문서 Task는 PRODUCT/HANDOFF/현재 설계·계획·운영 문서의 병합 후 stale 문구를 정정한다.
과거 ADR 0002와 2026-09-07 계획은 역사 기록으로 보존한다. 현재 설계의 monitorcli 참고 문구는
직접 runner 사용으로 정정하고 이 삭제 계획은 제거 이유를 남긴다.

root는 별도 통합 branch에 문서/코드 commit을 결합하고 inventory·Issue 분류·ledger를 포함한다.
전체 branch 독립 리뷰와 최종 `make check` 후 PR을 준비한다. Windows 앱 실행이나 native 오류 상태는
이번 Linux 검사로 증명하지 않으며 기존 `325db89` 검증을 이번 SHA의 native 검증으로 재표기하지 않는다.

## 후속 후보

coordinator/scheduler/runtime/recovery/state 본체는 현재 Monitor의 의존성이 아니더라도 기존 CLI와
테스트가 소비한다. 이번에 함께 삭제하지 않는다. 각 consumer와 config/fixture/docs 경계를 inventory로
분리한 뒤 다음 작은 slice를 정한다.
