# Product

<!-- impeccable:product-schema 1 -->

## Platform

Windows desktop with WSL execution.

## Stack

Herdr 기반 persistent terminal session, Go 기반 agentctl, Wails v2 Windows 앱, React·TypeScript UI.
Windows 앱은 wsl.exe를 통해 구조화된 CLI를 호출한다.
GitHub.com/GHES의 Issues, Projects, PR과 Wiki를 업무 기록으로 사용한다.

## Users

주 사용자는 Windows 11과 WSL에서 여러 프로젝트를 동시에 진행하는 단일 Operator다.
프로젝트는 단일 저장소이거나 frontend/backend처럼 여러 저장소로 구성된다.
Agent가 구현하는 동안 다른 프로젝트로 이동하고, 돌아올 때 문맥 회수에 시간과 힘을 쓴다.

## Product Purpose

어느 프로젝트로 돌아와도 무엇을 왜 시작했고, 어떤 결정을 했으며,
어디까지 완료했고 지금 무엇을 해야 하는지 근거와 함께 알 수 있게 한다.
에이전트의 구현·수정·검증과 프로젝트의 의사결정·문서를 연결한다.

성공은 실제 업무 하나의 요청부터 문서 발행까지 이어지고,
여러 프로젝트를 오가거나 새 대화를 시작해도 같은 설명을 반복하지 않는 것으로 판단한다.
현재 설계는 목표이며 다중 프로젝트 UI와 새 v2 구현 완료를 주장하지 않는다.

## Positioning

멀티 세션과 native subagent가 잘 잡힌 Herdr 구조 위에 놓는 GitHub 중심 Monitor다.
GitHub가 업무·결정·PR·문서의 영구 기록을 소유하고 Herdr가 대화형 Session·Workspace·Tab·Pane을 소유한다.
ThreadDock은 두 상태를 연결해 문맥 회수와 근거 탐색을 돕고, Top-level Agent가 결정과 결과를 이해할 수 있는 문장으로 남긴다.

## Operating Context

1차는 GitHub Projects·Issue·PR·Wiki와 기존 Wails Monitor를 사용한다.
DXHub 프로젝트 메뉴·MCP 연동과 여러 사용자의 공유 실행 제어는 후속 범위다.

- Herdr Pane의 Top-level Codex/OpenCode 대화에서 요청, 범위와 중요한 결정을 승인한다.
- 각 Top-level Agent는 구현·조사·검토를 위한 native Internal Subagent를 직접 조정한다.
- Project는 여러 Repository를 묶고 대표 저장소에 Parent Issue와 기본 Wiki를 둔다.
- Work Item은 저장소별 Task·PR과 전체 완료 조건을 연결한다.
- 공용 GitHub Projects 보드의 프로젝트별 보기에서 업무 진행과 우선순위를 관리한다.
- Monitor는 GitHub 중심 프로젝트 목록·상세·다음 행동·결정·근거 링크와 연결된 live Herdr Session 상태를 제공한다.
- Monitor는 Work/repository/canonical Worktree의 exact binding으로 두 원본을 연결하고, 업무 기록 상태와 로컬 실행 상태를 별도 label·관찰 시각으로 보여준다.
- Herdr persistent Session은 Monitor UI 종료와 독립적이며 사용자는 Herdr에서 다시 attach한다.
- GitHub PR은 사람이 Create a merge commit 방식으로 병합한다.
- 사용법·구조·런북은 Wiki에, 코드와 함께 검토할 상세 설계는 해당 저장소에 둔다.

## Capabilities and Constraints

- 여러 프로젝트의 Top-level Agent를 Herdr의 여러 Session·Workspace·Tab·Pane에서 동시에 유지한다.
- Top-level Agent 하나가 native Internal Subagent의 분배와 호출 한도를 소유한다. ThreadDock은 별도 중첩 scheduler를 만들지 않는다.
- 모니터는 목적, 최근 검증된 완료 내용, 현재 단계, 최근 결정과 필요한 다음 행동을 보여준다.
- 모니터는 Top-level Session·Pane과 agent 상태를 보여주되 Internal Subagent 개별 생명주기는 표시하지 않는다. 필요한 결과는 요약 evidence로 연결한다.
- 실제 실행 상태와 GitHub 동기화 상태를 분리하고 관찰·발행 시각 및 stale 표시를 제공한다.
- binding이 없거나 중복되면 `missing` 또는 `conflict`로 표시하고 running/completed를 한쪽 상태에서 추정하지 않는다.
- Task 검증·독립 리뷰와 최종 통합 검증·전체 업무 리뷰를 수행한다.
- 같은 모델 세션 복원에만 의존하지 않고 GitHub·Git·검증·handoff를 통해 완료 작업을 회수한다.
- Codex와 OpenCode를 Top-level Agent로 사용한다. GitHub 쓰기는 Operator가 승인한 skill 또는 멱등 Go Publisher를 통한다.
- 제안과 승인된 결정을 구분하고 새 결정이 이전 결정을 대체한 이유를 남긴다.
- repository docs를 포함한 최종 HEAD와 Wiki 변경안을 검토한다.
- 필수 PR 일부만 병합된 상태를 표시하고 전체 완료로 처리하지 않는다.
- 필수 Wiki·Issue·Projects 반영 실패는 코드 재실행 없이 발행만 재시도한다.
- 문서만 정리하는 업무도 승인·검토·Wiki 발행 과정을 지원한다.
- 여러 저장소 원자적 병합, 자동 main 병합과 자동 rollback을 제공하지 않는다.
- 기존 v1 Run은 새 실행 호환 대상이 아니다. 기록은 보존하고 상태 파일을 자동 변환하지 않는다.
- 기존 Go state·verification·publisher는 선택적 Work Evidence module로 유지할 수 있지만 Session·Pane·Agent 생명주기를 소유하지 않는다.
- Session/Workspace/Tab/Pane 생성·열기·초점·Top-level Agent 시작은 Herdr CLI를 따르는 project skill의 명시적 행동으로만 제공한다.
- 작업 재개는 기존 live Agent의 focus/attach 또는 최신 handoff prompt, blocked Agent의 inspect-first, missing/ended Agent의 승인된 새 start로 나눈다. 완료 Task는 새 prompt에 다시 넣지 않는다.
- `agent_prompt_stalled`는 판단 필요 상태로 보이고 자동 재전송하지 않는다. 사용자가 직접 attach 또는 prompt를 선택할 수 있다.
- production 배포·관측, 자동 업데이트, 전체 workflow catalog와 새 채팅 UI는 후속 범위다.
- 파일럿 전용 장애 인증·대규모 fault matrix를 이번 제품 완료 조건으로 두지 않는다.

## Brand Commitments

제품명은 ThreadDock, Windows 앱은 ThreadDock Monitor, WSL CLI는 agentctl이다.
한국어로 목적·상태·다음 행동을 먼저 설명하고 technical identity는 상세 근거에 둔다.
진행률을 추정해서 만들거나 오래된 상태를 현재 실행 중이라고 표시하지 않는다.

## Evidence on Hand

- [Project Workflow MVP 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md)
- [Herdr-first 첫 사용 설계](docs/superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)
- [Herdr 실행 topology 소유 결정](docs/adr/0007-herdr-owns-interactive-execution-topology.md)
- [설계 전환 결정](docs/adr/0006-project-workflow-mvp.md)
- [용어 모델](CONTEXT.md)
- 기존 Go 구현과 legacy 운영 문서는 재사용 검토 자료이며 새 MVP 완료 근거가 아니다.
- 예전 파일럿 결과를 다중 프로젝트·다중 저장소·Monitor 기능의 검증 결과로 표시하지 않는다.

## Product Principles

- 프로젝트를 다시 열었을 때 다음 행동을 5초 안에 찾게 한다.
- 목적·결정·코드·검증·문서를 서로 연결한다.
- 업무 상태, 실행 상태와 동기화 상태를 구분한다.
- 일상적인 실패는 제한된 자동 수정으로 처리하고 판단이 필요한 지점에서 근거와 함께 멈춘다.
- 실제 업무에 필요한 기능과 검증을 우선하고 검증 플랫폼 자체를 제품으로 키우지 않는다.
- GitHub의 Issue·PR·Projects·Wiki 역할을 활용하고 동일 본문의 중복 원본을 만들지 않는다.
- Herdr의 persistent topology와 Agent의 native subagent 기능을 재구현하지 않는다.

## Accessibility & Inclusion

Windows의 100~200% 배율, 키보드 탐색과 focus 표시를 지원한다.
상태는 색상만으로 구분하지 않고 텍스트를 함께 표시한다.
기본 화면에는 업무와 다음 행동을, 상세에는 commit·검증·동기화 근거를 제공한다.
