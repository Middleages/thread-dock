---
status: accepted
---

# 변경 범위와 최종 결과에 맞춰 검증한다

구현 중에는 관련 범위의 Focused Verification을 수행한다.
Task Gate는 Go가 후보 commit의 선언된 검증·static check를 실행한다.
Agent가 보고한 passed 결과만으로 검사 완료를 판단하지 않는다.

repository docs까지 포함한 저장소별 최종 HEAD에서 Full Suite를 수행하고
여러 저장소 업무의 연결 검증과 fresh 전체 리뷰를 수행한다.
이후 HEAD/base·계약이 바뀌면 영향받는 근거를 무효화한다.

2026-09-07 [ADR 0006](0006-project-workflow-mvp.md)에 적용한다.
매 Task마다 전체 suite를 반복하거나 파일럿 전용 sharding·장애 인증을 추가하지 않는다.
코드 변경 완료 시 make check를 실행하고 실패 원인을 확인해 해결한다.
문서만 변경하면 링크·정책 일관성을 검사하고 코드 실행 검사와 구분한다.

검증에는 명령, 대상 SHA, 결과, duration과 exit code를 남긴다.
GitHub에는 요약과 근거를 남기며 credential·raw transcript는 보존하지 않는다.
