---
status: accepted
---

# 실행 종료와 실행 근거 삭제를 분리한다

Agent를 종료해도 Worktree·검증·리뷰·업무 기록을 함께 삭제하지 않는다.
미완료 업무 재개와 완료 결과 검토에 필요한 정보이기 때문이다.
불명확한 실행 상태의 프로세스나 Worktree를 강제로 정리하지 않는다.

2026-09-07 [ADR 0006](0006-project-workflow-mvp.md)에 따라 원칙은 유지하되,
v1 retirement 상태 기계·session 복구를 새 MVP에 이식할 의무는 없다.
Codex ephemeral 호출도 Runtime으로 지원하며 계약·Git·상태가 연속성을 제공한다.
OpenCode/Herdr 정리는 adapter가 정확히 소유한 실행에 대해서만 수행한다.

근거 삭제는 완료·clean 상태를 확인한 명시적 정리 작업이다.
이전 세부 절차는 legacy 운영 문서와 Git 이력을 참고한다.
