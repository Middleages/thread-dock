---
status: accepted
---

# 계층형 검증 cycle로 feedback과 회귀 검증을 균형화한다

구현 loop에서는 변경 범위에 맞는 Focused Verification을 실행하고, Task gate에서는 Task verification과 관련 static check를 실행한다. Full Suite가 60초 이하면 Task gate에서도 실행한다. 60초를 초과하면 Wave End Verification, shared-interface 변경, final PR에서 실행하며 final PR은 항상 Full Suite를 실행한다. Reviewer는 근거가 부족하거나 이름이 명시된 의문이 있을 때만 다시 실행한다.

이 선택은 빠른 feedback과 전체 회귀 검증 사이의 되돌리기 어려운 운영 tradeoff다. 매 Task마다 Full Suite를 강제하면 느린 저장소에서 구현 cycle이 길어지고 병렬 실행의 이점이 줄어든다. 반대로 Full Suite를 final PR까지 미루면 통합 시점의 결함 발견이 늦어진다. 60초 기준과 shared-interface·wave·final 시점을 고정해 두 비용을 예측 가능하게 분리한다.

각 검증 evidence에는 command, outcome과 duration을 남긴다. Issue·PR audit comment는 Decision, Dispatch, Review, Verification, Blocker, Integration 범주로 요약하며 transcript, token, secret과 긴 raw terminal output은 보존하지 않는다.
