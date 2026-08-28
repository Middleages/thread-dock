# ThreadDock

ThreadDock은 Go `1.27.0` 표준 라이브러리로 동작하는 로컬 개발 오케스트레이터입니다.
현재 작업 계약 버전은 `1`이며, 한 Builder와 독립 Reviewer를 사용하는 복구 가능한
Single-run 단계까지 구현되어 있습니다.

Single-run은 GHES에 Parent/Child Issue를 등록하고 격리 Worktree에서 작업한 뒤
Reviewer에게 전달합니다. 아직 PR 생성, main 병합, CI/CD와 운영 배포는 수행하지
않습니다.

## 시작하기

계약 형식만 확인하려면 [Foundation 파일럿](docs/operator/foundation-pilot.md)을,
사내 GHES/OpenCode 복구 시험을 하려면
[Single-run 파일럿 빠른 시작](docs/operator/single-run-pilot.md)을 따르십시오.

## 문서

- [시스템 설계 명세](gitops-agent-system-design.md)
- [ThreadDock 구현 로드맵](docs/superpowers/plans/2026-08-28-threaddock-roadmap.md)
- [Foundation 파일럿 walkthrough](docs/operator/foundation-pilot.md)
- [Single-run 파일럿 빠른 시작](docs/operator/single-run-pilot.md)

검증과 미리보기의 현재 계약 구현은 `internal/contract`에 있으며, 반복 가능한
검사 게이트는 `make check`입니다.

## 검증 정책

구현 loop에서는 변경 범위에 맞는 focused 검증을 실행합니다. Task gate에서는 Task verification과 관련 static check를 실행하며, 전체 suite가 60초 이하면 이때 전체 suite도 실행합니다. 전체 suite가 60초를 초과하면 wave end, shared-interface 변경 후와 final PR에서 실행하고, final PR에서는 항상 실행합니다.

Makefile focused target은 package를 명시적으로 받아야 합니다.

```sh
make test-focused PKGS="./internal/contract"
make vet-focused PKGS="./internal/contract"
make check
```

`PKGS`가 비어 있으면 target은 설명과 함께 실패합니다. 검증 기록에는 command, outcome과 duration을 남깁니다. Reviewer는 근거가 부족하거나 이름이 명시된 의문이 있을 때만 재실행합니다.

Issue·PR audit comment는 `Decision`, `Dispatch`, `Review`, `Verification`, `Blocker`, `Integration` 중 하나로 분류하고 summary와 evidence만 남깁니다. transcript, token, secret과 긴 raw terminal output은 기록하지 않습니다.
