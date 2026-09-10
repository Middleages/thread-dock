# PR #69 병합 후 업무 분류

## 실제 조회

2026-09-10 현재 `gh auth status`는 Middleages 활성 인증과 `read:project`, `repo` scope를 확인했다.
`gh project list --owner Middleages --format json`은 성공했고 보드는 0개였다.
GraphQL repository `projectsV2`도 0개, 열린 Issue #42~48의 `projectItems`도 모두 0개다.
권한 오류와 빈 조회 결과를 구분한다. `read:project`는 쓰기 권한의 증거가 아니며, 갱신 대상 보드가 없어 Projects 쓰기는 수행하지 않았다.
`hasWikiEnabled=false`이므로 이 문서는 저장소 문서이며 Wiki 반영이 아니다.

로컬 main과 fetch한 origin/main은 `3f4bebf08bb099637d1b2bfff44a9ba56e91a5df`다.
[PR #69](https://github.com/Middleages/thread-dock/pull/69)는 2026-09-10T13:08:14Z에 병합됐으며
최종 head는 `f35e24885ea95379559dd5cb07f4992f69915ac0`이다. 착수 시 열린 PR은 0개였다.

## ADR 0008에 따른 분류

기준: [ADR 0008](../adr/0008-github-first-skills-before-engine.md).
실행은 Herdr, 개발 판단·분배·기록은 Agent/Skills, 업무 원본은 GitHub, 제품은 Go/Wails Monitor다.
main 병합은 사람이 수행한다. 기존 요구사항을 그대로 유지할 Issue는 없다.

| Issue | 분류 | 근거 및 다음 범위 |
|---|---|---|
| [#42](https://github.com/Middleages/thread-dock/issues/42) | superseded 종료 후보 | Parent의 scheduler·recovery·repair·감독형 자동 병합 목표 전체가 ADR 0008과 충돌한다. 완료로 닫지 않고 대체 결정에 따른 종료 후보로 남긴다. |
| [#43](https://github.com/Middleages/thread-dock/issues/43) | 새 방향으로 재작성 | 중단 후 이어가기 목적은 유효하다. 자동 continuation·3회 recovery cap을 제거하고 위치·관찰 시각·GitHub handoff로 안전하게 돌아가는 운영 검증으로 바꾼다. #69의 구현을 중복하지 않고 실제 native 오류 상태 등 남은 검증만 추적한다. |
| [#44](https://github.com/Middleages/thread-dock/issues/44) | superseded 종료 후보 | Task DAG·두 슬롯 dispatch·RUN snapshot은 Agent/Herdr 책임을 복제한다. 소유 경로·공유 변경 규칙은 Task packet과 개발 지침에 이미 남아 있다. |
| [#45](https://github.com/Middleages/thread-dock/issues/45) | superseded 종료 후보 | one-action-per-advance·intent reconciliation·자동 병합 pilot은 폐기한 Orchestrator 요구다. 독립 기능 두 개의 실제 사용 검증은 현재 2026-09-10 계획의 미완료 항목으로 별도 유지한다. |
| [#46](https://github.com/Middleages/thread-dock/issues/46) | superseded 종료 후보 | exact-SHA merge API·agentctl confirm·자동 Merge Gate는 현재 사람 병합 경계와 충돌한다. Draft PR와 검증 근거 기록은 Agent가 gh/Git으로 수행한다. |
| [#47](https://github.com/Middleages/thread-dock/issues/47) | superseded 종료 후보 | immutable SHA·경로 소유권 검토 원칙은 유지하되 별도 Builder 통합 엔진·공유 repair budget은 만들지 않는다. 원칙은 개발 정책에서 적용한다. |
| [#48](https://github.com/Middleages/thread-dock/issues/48) | 새 방향으로 재작성 | Git 변경·handoff 보존 목적은 유지한다. completed RUN 자동 retirement와 workspace close 엔진을 제거하고 Herdr에서 사용자가 세션을 정리할 때의 보존·재개 안내로 바꾼다. idle/done이나 Issue 닫힘을 자동 종료 조건으로 쓰지 않는다. |

이 표는 분류 제안이다. 기존 Issue 본문과 열린 상태를 보존하고 각 Issue에 한국어 근거 댓글을 기록한다.
종료 후보를 구현 완료로 처리하지 않으며, 새 방향의 완료 조건이 확정되지 않은 Issue 본문을 완료된 기능처럼 바꾸지 않는다.

## 검증 제한

- 이번 live GitHub CLI 결과는 현재 Linux 세션의 인증·원격 기록 조회 근거다. Windows 앱의 Projects 표시 검증이 아니다.
- 기존 Windows healthy-path 근거는 reviewed SHA `325db89`의 기록을 인용한다. 이번 삭제 slice의 Windows 실행 결과로 확대하지 않는다.
- native Windows error/degraded 상태와 중앙의 독립 기능 둘·Projects까지 잇는 사용 검증은 여전히 미검증이다.
- 현재 도구 목록에 native computer-use가 없으므로 새 Windows 화면 검증이 필요하면 Windows GPT app 검증을 요청해야 한다.
