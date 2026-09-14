# Native Specialist Team Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development. 저장소의 Luna 구현/fresh Sol 리뷰·모델 승격 금지·focused 검증·사용자 데이터 보존 정책이 우선한다.

**Goal:** OMO 없이 문서 편집·탐색·직접적인 구현·독립 리뷰를 수행하는 재사용 가능한 팀 템플릿을 제공한다.
**Architecture:** 기존 두 top-level Agent + 네 native leaf + 공용 Skill. 실행과 권한은 기존 harness, 업무 원본은 GitHub다.
**Tech Stack:** Markdown Skill/OpenCode Agent, Codex TOML, 기존 Makefile template-check.
**Spec:** [승인 범위 설계](../specs/2026-09-14-native-specialist-team-design.md).

## Global Constraints

- 새 엔진/bridge/plugin/global install 및 제품 Go/React 변경 없음.
- canonical6개 유지, helper는 herdr-local + explore-codebase + write-project-docs.
- 새 leaf4개 이름: td_explorer, td_docs_editor, td_implementer, td_reviewer. model/effort override 없음.
- 단순성·로그의 진단 역할·문서 의미 보존은 판단 기준이며 새 실행 차단/checklist pipeline이 아니다.
- root에 남은 기존 dirty 문서·실험은 보존한다. worker는 소유 밖 파일이나 다른 worktree를 수정하지 않는다.

### Task 1: 팀 템플릿·공용 Skill·설치 안내

- taskId: native-specialist-team-templates
- baseSHA: d57beeb5e1218ec39d5053b74e6ae5d1c4b6b5b8
- deps: 위 spec의 이름/책임/공유 참조 직렬 고정 완료
- ownedPaths: project-template/.agents/skills/**, project-template/.codex/agents/**, project-template/.opencode/agents/**, project-template/AGENTS.md, Makefile, docs/operator/github-first-quickstart.md
- worktree: /home/appuser/dev_system/.worktrees/native-specialist-team-impl
- branch: agent/native-specialist-team-impl
- forbiddenPaths: 소유 밖 전체. 특히 root AGENTS.md, PRODUCT.md, monitor/**, internal/**, go.mod/go.sum, 사용자 전역 설정, 다른 worktree, 외부 게시
- interface: spec의 leaf→Skill 매핑. 새 helper frontmatter는 name과 description: Use when으로 기존 형식 유지. 새 도구별 정의는 같은 Skill을 가리킨다.
- acceptance: 두 primary 유지, leaf4개×두harness 정의, helper2개, 짧은 라우팅, tdd 자급성, 합의된 단순성/로그/문서 책임, GitHub 원본·사람병합·leaf 무재위임/게시 없음, 실제 옵션 없는 Project pending, 불명확 쓰기의 중복 방지. Quickstart는 설치·호출 방법과 모델/권한 상속·선택적인 위임을 설명한다. Makefile은 새 역할·helper 누락을 기존 template-check에서 찾되 별도 validation engine이나 runtime version gate를 만들지 않는다.
- tests: make template-check; Python tomllib/YAML 등 이용 가능한 parser로 전체 Agent frontmatter/name/mode와 Skill 참조 확인; quick_validate.py로 신규/수정 Skill 검증; 수정 문서 로컬 링크; git diff --check. OpenCode --pure 실제 인식과 행동 probe는 root가 통합 후 수행한다. docs/config-only이므로 전체 make check/Go/UI suite를 반복하지 않는다.
- result: changedFiles, commitSHA, executedCommands, outcomes, unverified, blockers를 배정된 report에 기록한다.

- [ ] 기존 wrapper/Skill/설치 문서의 연결점을 읽는다.
- [ ] leaf4개를 두 harness에 추가하고 helper2개를 구현한다.
- [ ] tdd-task/review-change/develop-feature/publish-work와 primary 진입점을 한정 보강한다. 공통 의미는 Skill에 두고 wrapper에 복제하지 않는다.
- [ ] template-check 목록과 Quickstart를 실제 구성을 설명하도록 갱신한다.
- [ ] focused parse/link 검사와 self-review 후 소유 파일만 commit한다. controller가 fresh task-review를 배정한다.

## 통합·동작 확인

root는 reviewed Task commit을 agent/native-specialist-team에 통합한다. 실제 설치 환경에서 source/Skill 이름 인식과 OMO 없는 leaf 위임을 확인하고, 탐색은 영향 경로와 근거, 문서 편집은 사실·명령 보존, 리뷰는 불필요한 검사와 진단 정보 손실을 식별하는지 작은 fixture에서 관찰한다. 같은 검증 tuple은 반복하지 않는다. 최종 whole-branch 리뷰 후 결과/제한/사용법을 handoff에 기록한다. 기본 역할 파일 추가가 현재 세션의 Agent registry를 자동 변경한다고 주장하지 않는다.
