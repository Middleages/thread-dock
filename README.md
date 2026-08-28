# ThreadDock Foundation

ThreadDock은 Go `1.27.0` 표준 라이브러리로 동작하는 계약 검증·미리보기 CLI입니다.
현재 작업 계약 버전은 `1`입니다.

Foundation increment의 파일럿은 계약을 읽고 검증·미리보기만 수행하며 GHES에
쓰지 않습니다. 승인과 GHES Issue·Project·PR 반영은 후속 increment의 범위입니다.

## 시작하기

저장소 루트에서 [Foundation contract-only pilot](docs/operator/foundation-pilot.md)의
명령을 실행하십시오. 기준 입력은 `testdata/contracts/valid.json`입니다.

## 문서

- [시스템 설계 명세](gitops-agent-system-design.md)
- [ThreadDock 구현 로드맵](docs/superpowers/plans/2026-08-28-threaddock-roadmap.md)
- [Foundation 파일럿 walkthrough](docs/operator/foundation-pilot.md)

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
