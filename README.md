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
