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

## 종료 기준

1. Monitor가 요구하는 runner 이외의 옛 실행 엔진 package와 CLI·전용 fixture·설정·scripts가 없다.
2. 남길 각 package와 asset에는 현재 소비자가 있거나 명시적 제품 목적이 있다. 모호한 것을 조용히 지우지 않는다.
3. legacy docs는 역사임을 표시하고 active quickstart/README/build가 제거된 경로를 요구하지 않는다. 사용자 데이터는 그대로 남는다.
4. Task reviews와 마지막 통합 gate/전체 review를 통과한 PR을 준비한다. native 미검증은 별도 Issue에 남긴다.

### Task 1: 옛 CLI와 pilot 진입점 제거

- taskId: `retirement-root-cut`
- baseSHA: `38a505a` (정확 SHA는 Task report와 ledger에 기록)
- deps: Linux/Windows inventory에서 Monitor의 유일 internal dependency가 runner임을 확인, root 직렬 interface 결정
- ownedPaths: `cmd/agentctl/`, `internal/cli/`, `internal/config/`, `internal/pilot/`, `scripts/single-run-pilot.sh`, `Makefile`; report `.superpowers/sdd/2026-09-12-engine-retirement-completion/task-1-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/retirement-root-cut`
- branch: `agent/retirement-root-cut`
- forbiddenPaths: 소유 밖 전체, 특히 `monitor/`, `internal/runner/`, `project-template/`, `go.mod`, `go.sum`, docs, 다른 worktree와 사용자 데이터
- interface: agentctl binary/CLI와 옛 실행 설정·pilot을 제거한다. 나머지 내부 엔진은 이 단계에서 보존한다. Monitor·runner·새 settings/GHES/Toolbox·canonical Skills 계약은 불변이다.
- acceptance: owned legacy source/test와 single-run script 삭제; 남은 import 0 확인; Makefile Go discovery는 사라진 cmd를 요구하지 않으며 pilot bash 검사를 제거한다. focused 예시는 살아 있는 monitor/runner로 바꾼다. frontend/template/Go 전체 gate는 그대로 보존한다.
- tests: Go1.27 `go list ./...`; `go test ./monitor ./internal/projecttemplate`; `go vet ./monitor ./internal/runner`; `make template-check`; `make -n check`; refs/gofmt/diff. 별도 TMPDIR tmpfs 사용. full suite 금지.
- result: changedFiles, commitSHA, executedCommands(환경 포함), outcomes, unverified, blockers를 report에 기록한다.

- [ ] production/test/config/docs caller 근거와 정확 삭제 파일을 report에 기록한다.
- [ ] apply_patch로 owned 경로만 삭제하고 Makefile discovery/pilot/focused 예시를 정리한다.
- [ ] focused 검증·self-review 후 commit한다. 삭제한 전용 동작의 대체 테스트를 만들지 않는다.
- [ ] fresh Sol이 고정 SHA/diff를 검토하고 root가 통합한다.

후속 Task의 정확 ownedPaths는 전체 inventory와 직전 reviewed SHA를 사용해 직렬 확정한다. CLI 제거만으로 전체 엔진 정리 완료를 주장하지 않는다.
