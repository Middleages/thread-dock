# PR #70 이후 운영 문서 정정 결과

## Task packet

- taskId: `post70-operations-docs`
- baseSHA: `22dcc6610b34582d9a3a4ee375e7661fde499e5e`
- deps: `[]`
- ownedPaths: `HANDOFF.md`, `docs/operator/github-first-quickstart.md`, 이 보고서
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice2-docs`
- branch: `agent/engine-retirement-slice2-docs`
- forbiddenPaths: 모든 코드·테스트·config·shared/module 파일, `AGENTS.md`, 기존 실험과 나머지 문서
- interface: 문서만 변경하며 제품 설정과 public/shared interface는 변경하지 않는다.
- acceptance: PR #70 병합, 현재 Project와 Wiki 상태를 반영한다. 2026-09-10의 Projects 0개와
  Wiki 비활성 근거는 과거 기록으로 보존한다. 실제 `THREADDOCK_PROJECTS` URL과 업무·실행 상태의
  원본 구분을 안내하고 native 검증 범위를 확대하지 않는다.
- tests: 상대 Markdown 링크와 `git diff --check`만 수행한다.

## Result

- changedFiles: `HANDOFF.md`, `docs/operator/github-first-quickstart.md`, 이 보고서
- commitSHA: 운영 문서 `34ea375c891e082063d2ea22401c2f70dcc060db`; 이 보고서는 다음
  ledger commit에 포함한다.
- executedCommands:
  - `git status`, `git rev-parse`, `git log`, `git show`: base와 PR #70 merge 확인
  - `rg`와 `sed`: `THREADDOCK_PROJECTS` parser와 현재 문서 근거 확인
  - owned 문서 상대 Markdown 링크 parser: exit 0, 2 files/7 links
  - `git diff --check`: exit 0
  - `git status --short`, `git diff --name-only`, `git diff --stat`: 승인된 2개 문서만 변경 확인
- outcomes:
  - PR #70의 main merge commit `22dcc66`, 열린 PR 0개, public 저장소와 Wiki 활성 상태를 기록했다.
  - 비공개 사용자 Project와 board view, 저장소 연결, Issue #42~48 `Todo`, PR #69·#70 `Done`,
    `정리 방향` 원문 field를 현재 상태로 기록했다.
  - `THREADDOCK_PROJECTS=https://github.com/users/Middleages/projects/1`를 실제 설정 예시로 추가하고
    `/views/2` URL은 Monitor 설정값이 아님을 명시했다.
  - GitHub Projects의 업무 상태와 Herdr session·Agent 상태/관찰 시각을 별도 원본으로 구분했다.
  - Wiki 활성화·사용자 접근 확인과 Wiki 페이지 발행 미수행을 구분했다.
  - 기존 `325db89` Windows healthy-path와 `ff977a2` tmpfs Linux gate 근거를 확장하지 않았다.
- unverified:
  - Wiki 페이지 실제 발행과 Wiki Git remote 접근
  - 현재 Project 설정을 사용한 새 Windows native Monitor 실행
  - native Windows 오류/degraded 상태의 실제 재현
  - 실제 Projects를 사용한 독립 기능 두 개의 end-to-end 운영 흐름
  - runtime model/effort identity
- blockers: docs-only 정정에는 없음. Wiki 페이지 발행은 수행하지 않았다.
