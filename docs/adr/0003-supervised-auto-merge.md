---
status: accepted
---

# 일반 변경은 자동 병합하고 Protected Change는 사람이 확인한다

Issue 묶음을 승인한 뒤 Agent는 구현, 독립 리뷰, CI와 일반 변경의 main 병합까지 자율 수행한다. 사후 확인만으로 조정 부담을 줄이되 데이터, 인증·권한, 배포, 공급망과 공개 계약을 바꾸는 Protected Change는 Merge Gate를 통과해도 사람 확인을 요구한다. production 배포는 별도 사용자 행동으로 유지하고 Reviewer·CI 차단을 Orchestrator가 임의로 우회하지 못하게 한다.
