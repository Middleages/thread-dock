# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Go 기반 `agentctl`, Wails v2 기반 Windows 데스크톱 앱, React·TypeScript UI. Windows 앱은 `wsl.exe`를 통해 WSL의 구조화된 CLI 계약을 사용한다.

## Users

주 사용자는 깊은 Git·인프라 지식을 전제로 하지 않는 사내 개발 Operator다. 각 사용자는 자신의 Windows 11 PC와 WSL에서 한 번에 하나의 개발 요청을 감독한다.

## Product Purpose

개발자의 자연어 요청을 작업 인터뷰로 구체화하고, GitHub Issue 묶음부터 Agent 구현·독립 리뷰·CI·main 병합까지 연결해 사람이 사후 확인할 수 있게 한다. ThreadDock Monitor는 현재 상태와 사람이 해야 할 다음 행동을 즉시 이해시키고 production GitHub Actions workflow를 명시적으로 시작할 수 있게 한다.

성공은 실제 Issue 5~10개에서 사람의 반복 설명, 잘못된 변경 범위와 중단 복구 부담이 줄어드는 것으로 판단한다.

## Positioning

GitHub Enterprise Server를 영구 업무 기록으로 유지하면서, 개발자 PC의 OpenCode·Herdr·Worktree 실행을 하나의 쉬운 상태 흐름으로 연결한다. 중앙 멀티에이전트 플랫폼을 만들지 않고 로컬 실행과 GitHub 기록 사이의 간극만 메운다.

## Operating Context

- 작업 인터뷰와 Issue 묶음 승인은 OpenCode 대화에서 수행한다.
- WSL의 Local Orchestrator가 `agentctl` interface로 Herdr, Builder, Reviewer와 Worktree를 관리한다.
- Windows ThreadDock Monitor는 3~5초 간격으로 구조화된 CLI 상태를 읽고 중단·재개·재시도를 제공한다.
- 제품 저장소의 GitHub Actions가 CI와 production CD를 수행한다.
- 모든 workflow를 보여주되 `workflow_dispatch` workflow에만 실행 진입점을 제공한다.
- production dispatch 권한은 Windows 앱에만 보관하며 WSL Agent에는 제공하지 않는다.
- GHES Releases가 Windows 앱과 Linux `agentctl` binary를 함께 배포한다.

## Capabilities and Constraints

- Parent Issue 하나와 필요한 Child Issue를 묶음 미리보기 후 등록한다.
- Parent Issue 하나에서 Builder 최대 두 개를 Worktree로 병렬 실행한다.
- Reviewer는 별도 context에서 실행하며 Reviewer·CI 자동 수정은 합계 두 번까지다.
- 느리거나 중단된 Agent에는 연속 세 번까지 Recovery Attempt를 수행한다.
- 일반 변경은 Merge Gate 통과 후 main에 자동 병합하며 Protected Change는 사람 확인을 요구한다.
- production 배포는 Operator가 ThreadDock Monitor 또는 GHES에서 직접 시작한다.
- v0.1은 Windows 11과 WSL, 단일 Operator, 단일 저장소 실행만 지원한다.
- Worktree Isolation을 기본으로 사용하며 container sandbox는 프로젝트가 이미 지원할 때만 선택한다.
- 로컬 실행 상태는 사람이 읽을 수 있는 JSON snapshot과 event log로 보존한다.
- 한국어 UI만 제공하고 전체 Actions 로그와 production health metric은 앱에 저장하거나 포함하지 않는다.
- 앱과 CLI가 과거 버전이라는 이유만으로 실행을 차단하지 않으며 명령 계약 호환성으로 기능을 판단한다.
- 새 Release는 자동 다운로드 후 사용자가 적용 시점을 확인하고, 실패 시 직전 정상 버전으로 복구한다.

## Brand Commitments

제품명은 `ThreadDock`이다. Windows 앱은 `ThreadDock Monitor`, WSL CLI는 `agentctl`로 부른다. 화면 문구는 전문 용어보다 쉬운 한국어와 구체적인 다음 행동을 우선한다.

## Evidence on Hand

- `gitops-agent-system-design.md`: 기존 운영체계 초안
- `herdr-security-review.md`: Herdr v0.8.2 조건부 보안 검토
- `herdr-implementation-guide.html`: Herdr 구현·운영 가이드
- `CONTEXT.md`: 확정된 도메인 언어
- `docs/adr/0001-go-wails-for-local-tools.md`: 로컬 도구 기술 선택

실제 제품 UI, 브랜드 자산과 실제 저장소 pilot 결과는 아직 없다. 화면 설계에서 가상의 성과 지표나 운영 증거를 만들지 않는다.

## Product Principles

- 현재 상태와 다음 행동을 5초 안에 이해시킨다.
- GitHub를 영구 기록으로 두고 로컬 상태는 복구를 돕는다.
- 사람의 결정과 Agent의 자동 실행을 명확히 구분한다.
- 한 사용자의 실제 흐름을 먼저 완성하고 확장 기능은 측정된 필요가 있을 때 추가한다.
- 복잡한 자체 보안·플랫폼 기능은 만들지 않되 credential, 명령 실행과 production 승인 최소선은 유지한다.

## Accessibility & Inclusion

Windows 11의 100~200% 화면 배율, 키보드 탐색과 focus 표시를 지원한다. 상태는 색상에만 의존하지 않고 문구와 형태를 함께 사용하며, 기술 식별자는 기본 화면에서 숨기고 필요할 때 상세 보기로 제공한다.
