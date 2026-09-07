# Implementation Readiness Plan

> **For agentic workers:** 준비는 Sol medium이 수행하고, 코드 구현 계획 작성 후 superpowers:subagent-driven-development를 사용한다. 구현·수정은 Luna high에 배정한다. 사용자 지정 모델과 독립 Task 병렬 실행은 skill의 기본 모델 선택·순차 실행 지침보다 우선한다. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** GitHub 기반 1차 MVP의 실제 개발 환경을 확인하고, 단일 저장소 업무 하나를 끝까지 구현할 수 있는 착수 자료를 만든다.

**Architecture:** GitHub Issue·Projects·PR·Wiki가 업무 기록을 맡고 Go가 실행·검증·발행을 소유한다. Wails Monitor는 같은 Go 상태와 명령을 사용한다. 준비 단계에서는 외부 업무나 코드를 자동 실행하지 않는다.

**Tech Stack:** Go (`go.mod`: 1.27.0), Git, gh, Codex/OpenCode, Wails v2 Windows + React/TypeScript, WSL CLI.

**Spec:** [Project Workflow MVP](../specs/2026-09-07-project-workflow-mvp-design.md). 실행 전 함께 읽는다.

## Global Constraints

- 전역 Agent 호출 한도 기본 2, 같은 Project의 변경 업무 1개.
- Task별 자동 수정은 revision 안에서 누적 2회, 확실한 일시적 호출 실패 재시도는 1회.
- main 병합은 사람이 GitHub의 Create a merge commit으로 수행한다.
- GitHub 쓰기는 Go Publisher가 소유한다. Main Agent 대화가 닫혀도 승인된 진행은 유지한다.
- v1 자료는 보존하고 새 v2 state directory를 사용한다. v1 실행 재개·자동 migration은 제외한다.
- 첫 구현 runtime 기본값은 Codex이며 OpenCode 공통 계약은 유지한다.
- DXHub 프로젝트 메뉴·MCP 연동·다중 사용자 공유 실행은 후속 범위다.
- 현재 Wails는 선택된 설계 방향이다. 구현된 Monitor 앱이 이미 존재한다고 가정하지 않는다.
- 준비 단계의 명령은 실제 지원되는 명령만 사용한다. 설계상의 `agentctl project`와 `agentctl work`는 아직 구현 대상이다.
- 검증되지 않은 환경·권한은 `unverified`로 기록한다. 버전 확인만으로 runtime capability나 외부 쓰기가 검증됐다고 표시하지 않는다.

## 개발 세션의 역할

[저장소 실행 지침](../../../AGENTS.md)과 [Codex 설정](../../../.codex/config.toml)을 따른다.
Sol medium이 계획·분배·검토, Luna high가 구현·테스트·수정을 맡는다.
공유 타입·인터페이스를 먼저 정하고 독립 Task는 별도 worktree에서 병렬 구현한다.
동시 구현 worker는 전체 트리에서 최대 3개다. Sol 하위 조정자가 배정한 worker도 이 한도에 포함한다.
제품 내부의 전역 Agent 호출 한도 2와 이 개발 세션의 worker 한도를 혼동하지 않는다.

## 범위와 산출물

이 계획은 설계 §13의 **단계 0: 착수 확인**을 다룬다. 코드 구현 전체 계획이 아니다.
환경 확인 뒤 첫 단일 저장소 흐름의 구현 계획을 작성하고 실행한다.
제품 전체를 한 번에 구현하거나 독립된 fault-pilot 플랫폼을 만들지 않는다.

| 자료 | 책임 |
|---|---|
| 현재 spec·CONTEXT·ADR 0006 | 승인된 방향과 도메인·정책 |
| 이 계획 | 시작 명령, 실제 호스트 확인 항목, 첫 흐름의 범위와 인수 근거 |
| 개발 호스트의 비공개 readiness 결과 | binary 경로/버전, 로컬 바인딩과 확인 시각; 자격증명 값 제외 |
| 다음 코드 PR의 구현 계획 | 아래 첫 흐름을 작은 검증 가능한 Task로 나눈 파일·인터페이스·테스트 |

이번 문서 작성 환경에서는 실제 사용자 workstation과 Windows UI를 확인하지 않았다.
Go suite·runtime 호출·Projects/Wiki 쓰기·Wails 실행은 모두 `unverified`다.

## Task 1: 올바른 기준선과 개발 도구 확인

**Files:** Read `HANDOFF.md`, `CONTEXT.md`, `PRODUCT.md`, `go.mod`, `Makefile`, 현재 spec.
Modify 없음. 로컬 사용자 변경과 v1 상태를 보존한다.

**Interfaces:** 입력은 현재 checkout/remote 상태, 출력은 기준 commit과 도구 확인 결과다.

- [ ] checkout에서 작업 경로와 변경을 확인한다.

```bash
pwd
git status --short
git remote -v
git branch --show-current
git rev-parse HEAD
```

결과에서 remote가 의도한 `Middleages/thread-dock`인지 확인한다.
출력에 credential을 포함한 remote URL이 있으면 공유 보고서에 복사하지 않는다.

- [ ] remote refs를 갱신하고 main과 설계 branch 관계를 확인한다.

```bash
git fetch origin
git log --oneline -5 origin/agent/runtime-adapters-mvp
git log --oneline -3 origin/main
git diff --stat origin/main...origin/agent/runtime-adapters-mvp
```

설계 기준 main은 `ce42f403b0a758acf3e647394b1c04a5f2c447c7`이다.
실제 main이 앞서 있으면 차이를 읽고 구현 branch에 필요한 변경을 통합한다.
사용자 변경에 reset/clean을 실행하지 않는다. 구현 branch는 최신 설계 문서를 포함해야 한다.

- [ ] WSL 개발 호스트의 도구와 설치된 CLI 도움말을 읽는다.

```bash
go version
git --version
gh --version
codex --version
codex exec --help
opencode --version
opencode --help
```

Go는 go.mod 요구와 호환되어야 한다. Codex 첫 흐름의 진행 여부는 OpenCode 설치 누락과
분리한다. OpenCode adapter 착수 시 Herdr의 실제 binary·도움말도 확인한다.
CLI capability는 해당 설치 버전으로 판단하고 미지원 flag를 추측해 사용하지 않는다.

- [ ] Windows 쪽 Go·Node·Wails 환경과 WSL 호출 가능 여부를 확인한다.

```powershell
go version
node --version
wails version
wails doctor
wsl.exe --status
wsl.exe --exec git --version
```

앱 bootstrap 전에 선택한 Wails 버전의 실제 요구사항을 확인한다.
WSL distribution을 명시할 로컬 binding을 정하고 경로 인용·종료 코드 전달을 검증한다.

- [ ] 결과를 `passed / failed / unverified`, 확인 시각, 다음 조치로 정리한다.
이 단계에는 소스 수정과 commit이 필요 없다. 준비 확인을 전체 기능 검증으로 표시하지 않는다.

## Task 2: 첫 업무의 연결과 실행 환경 확인

**Files:** Read spec §3·4·6·8·11·12. 아직 없는 v2 설정을 기존 v1 config에 끼워 넣지 않는다.

**Interfaces:** 입력은 실제 사용할 저장소·host·기존 board와 Wiki, 출력은 비밀 없는
논리 매핑 및 workstation 전용 binding이다.

- [ ] 첫 흐름의 고정 범위를 아래와 같이 설정한다.

| 항목 | 첫 흐름 |
|---|---|
| Project / Repository / Work Item | 각각 하나 |
| runtime | 설치 capability를 확인한 Codex |
| Task | 작고 되돌릴 수 있는 코드 변경 하나, 명시된 수용 조건 |
| GitHub | 기존 대상 저장소·보드·대표 Wiki에 연결 |
| 문서 | 관련 repository docs와 Wiki 변경안 |
| 검토 | 후보 Task 리뷰와 문서 통합 후 전체 리뷰 |
| UI | 현재 목적·완료·대기 이유·다음 행동·링크를 보는 최소 목록/상세 |
| 병합 | 사용자가 GitHub에서 merge commit 생성 |

실제 외부 쓰기 테스트 대상이 지정되지 않았다면 코드 구현은 fake GitHub로 진행할 수 있다.
특정 실사용 업무의 Issue/PR/Wiki를 만드는 시점에만 대상과 범위를 확정한다.

- [ ] 다음 연결 항목을 read-only로 확인한다.

| 대상 | 확인할 근거 |
|---|---|
| 저장소 | host, owner/name, 기본 branch, 원격 base SHA |
| Projects | 기존 board node ID, Project/Status/Priority field 및 option ID, 현재 접근 가능 여부 |
| Wiki | enabled와 실제 초기화 여부, 읽을 수 있는 Wiki Git base |
| 사용자 | 해당 repository와 board 접근; 한 운영자가 실행기를 소유 |
| 병합 | merge commit 허용, 필수 CI와 보호 규칙의 확인 가능한 범위 |

GitHub Project 접근과 private repository 접근은 별개다.
board 상태 변경 권한이 있다고 runtime 제어 권한을 얻는 것으로 취급하지 않는다.
read-only 조회로 쓰기 권한을 확정할 수 없으면 `unverified`를 유지한다.

- [ ] repository 실행 profile과 local binding의 필드를 다음 계약으로 정리한다.

| 버전 관리되는 executionProfile | workstation 전용 binding |
|---|---|
| profileId, 비밀 없는 profile hash | repoKey → canonical local path |
| setup/checks의 argv·cwdRepoKey·timeoutSeconds | requiredEnvNames → 로컬 secret 공급 경로 |
| requiredEnvNames: 이름만 | runtime binary/profile의 실제 위치 |
| services의 시작·readiness·timeout·managed 여부 | Run별 할당 port·endpoint·프로세스 identity |
| 수용 조건과 검증 target | host별 publication 인증 수단 |

비밀 값과 전체 환경을 보고서·계약·Agent packet에 기록하지 않는다.
setup도 검토된 명령만 실행하며 먼저 저장소의 실제 설치·테스트 지침을 읽는다.
관리되는 service는 자기 소유 프로세스만 종료하며 외부 공유 service는 종료하지 않는다.

- [ ] 작은 capability 확인은 adapter 구현 이후 첫 실제 호출에서 수행하도록 인수 항목으로 남긴다.
read-only 리뷰, 구조화 결과, timeout/종료 확인을 실제 설치 버전에서 검증한다.
새로운 live-fault 플랫폼이나 영구 시험용 board를 준비 작업으로 만들지 않는다.

## Task 3: 첫 구현 계획의 경계 고정

**Files:** 아래는 현재 확인한 접점이다. 새 package 경로는 후속 구현 계획에서 고정하며,
기존 v1 타입 이름에 억지로 v2 의미를 넣지 않는다.

| 현재 접점 | 구현 시 판단 |
|---|---|
| `cmd/agentctl/main.go` | 새 명령을 기존 v1 GHES/Herdr dependency 초기화와 분리 |
| `internal/cli/run.go` | project/work 명령과 구조화 오류·상태 출력 경계 |
| `internal/config/config.go` | host·프로젝트·로컬 binding 분리; v1 필수 필드를 v2 요구로 재사용하지 않음 |
| `internal/contract/` | WorkItem 계약 v2, immutable revision·검증된 명령 |
| `internal/state/` | 새 v2 저장 공간, 단일 writer·revision·멱등성 receipt |
| `internal/worktree/`, `internal/integration/`, `internal/pathscope/` | 범위 검사와 Go 소유 Git 조작의 좁은 기능 재사용 검토 |
| `internal/herdr/` | OpenCode adapter 내부에서 필요한 기능만 사용 |
| `internal/orchestrator/parallel.go` | 기존 자동 병합·recovery 정책을 그대로 연결하지 않음 |
| Monitor | 실제 앱 구현 여부부터 확인하고 선택한 Wails 구조로 최소 화면 구성 |

- [ ] 단일 저장소 흐름을 아래 순서로 나누고 각 Task의 입력/출력 타입과 실제 파일을 고정한다.

1. registry·contract v2·revision 상태와 승인/handoff 조회.
2. 승인된 Issue/Projects 발행과 receipt; 대화 종료에도 작동하는 Go Publisher.
3. Codex 호출 → Go 후보 commit·검증 → fresh Reviewer → Integration.
4. Documenter → repository docs 포함 최종 검사·전체 리뷰 → Final Manifest.
5. PR 발행 → 사람 병합 관계 확인 → Wiki 발행·업무 완료.
6. 위 상태를 사용하는 최소 Wails 목록·상세·근거 링크.

각 Task 계획에 실제 테스트 코드와 구현 인터페이스를 작성하고 실행한다.
계획만으로 검증·상태·발행이 구현됐다고 보고하지 않는다.

- [ ] 다음 실패 위험을 첫 흐름부터 검사 대상으로 포함한다.

| 위험 | 필요한 관찰 |
|---|---|
| 중복 요청·중단된 외부 발행 | 동일 requestId는 중복 Issue/PR을 만들지 않음; 다른 payload는 거부 |
| Agent가 종료되지 않음 | commit·검증·같은 Worktree의 새 호출이 시작되지 않음 |
| docs 통합 후 코드 변경 | 최종 검증/리뷰는 실제 최종 HEAD를 대상으로 함 |
| 사람이 PR HEAD/base 수정 | 이전 readiness 무효화 |
| 사람이 완료 조건 변경 | 새 Task·PR 발행 중단, 계약 revision 승인 대기 |
| Wiki 발행 실패 | 완료로 표시하지 않고 코드 재실행 없이 발행만 복구 |
| stale snapshot | 현재 실행으로 표시하지 않고 관찰 시각·stale 상태 제공 |

- [ ] focused 검사와 최종 검사를 구분한다.

```bash
make test-focused PKGS="./internal/contract ./internal/state ./internal/cli"
make vet-focused PKGS="./internal/contract ./internal/state ./internal/cli"
```

위 package 집합은 착수 시 현재 코드를 확인하는 예다. 실제 변경한 package를 명시하여
focused 검사한다. 수정 후에는 영향받는 covering 검사만 실행한다.
Sol 검토자는 동일 SHA·명령·환경에서 이미 통과한 테스트를 재실행하지 않는다.
각 worker의 full suite 실행은 금지한다. 실패는 재현 가능한 원인과 함께 기록한다.
통합된 코드 PR의 최종 검증 때 `make check`를 한 번 수행하고 결과를 보고한다.
전체 재실행은 필수 gate 또는 이전 결과를 무효화한 구체적 변경 근거가 있을 때만 한다.
Sol은 병렬 worker의 중복 검증을 제거하고 무거운 검사는 동시 실행 수를 낮춘다.
문서·설정만 바뀐 경우에는 링크·구문 검증을 수행하며 Go 전체 suite를 실행하지 않는다. 기존 suite가 무거워도 성공으로 보이게
검사를 제외하지 않으며, 더 이상 제품에 맞지 않는 검사는 해당 변경 이유를 설명해 정리한다.

- [ ] 첫 실제 흐름을 시연하고 아래 근거를 기록한다.

Parent Issue와 결정 링크, 후보/최종 SHA, 검증 결과, Reviewer 결과,
PR과 실제 merge commit, Wiki commit, 최종 handoff, Monitor에서 확인한 다음 행동.
이를 만족해야 단일 저장소 흐름 완료다. 전체 MVP 완료와 구분한다.

## 후속 단계와 spec 범위 점검

| spec 범위 | 이번 준비에서 고정한 것 | 코드 구현 인수 시점 |
|---|---|---|
| §1–4 목적·기록·식별자 | 원본과 Publisher 책임, 기존 GitHub 연결 | 단일 저장소 흐름 |
| §5 Monitor·동시 실행 | Wails 유지, UI와 실행 분리 | 최소 UI는 첫 흐름; 프로젝트 동시성은 다음 단계 |
| §6–8 계약·환경·역할 | profile 필드, Codex부터 시작 | 첫 흐름; OpenCode는 adapter 확장 |
| §9 복구·수정 | 처음부터 revision·receipt·종료 확인 | 첫 흐름 뒤 실제 중단·수정 시나리오 확장 |
| §10–12 상태·병합·Wiki | 상태 분리와 최종 gate | 단일 저장소 흐름에 포함 |
| 다중 repository 조건 | Project→Repositories 모델 유지 | 단일 흐름과 복구 확인 후 추가 |
| §14 전체 인수 | 아래 전체 완료 기준 유지 | 모든 단계 종료 시 |

전체 MVP에서는 두 프로젝트 전환, frontend/backend 두 저장소, 제한 수정·재검토,
완료 Task 보존 재개, 부분 병합과 Wiki 발행 복구까지 확인한다.
공유 포털이나 두 번째 UI를 앞 단계에 추가하지 않는다.
