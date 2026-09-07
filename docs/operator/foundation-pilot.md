> **Legacy v1 참고 자료.** 기존 구현·파일럿의 절차와 관찰 기록이며 새 MVP의 실행 계획이나 완료 조건이 아닙니다. [현재 설계](../superpowers/specs/2026-09-07-project-workflow-mvp-design.md)를 먼저 따르십시오.

# Foundation contract-only pilot

이 절차는 ThreadDock Foundation의 첫 계약 전용 파일럿입니다. 승인 전에
계약을 검증하고 미리보기만 생성하며, GHES Issue·Project·PR에는 쓰지 않습니다.
미리보기는 stdout으로만 출력되고 저장소 파일을 변경하지 않습니다.

## 사전 조건

- 저장소 루트에서 실행합니다.
- 실행 전에 `go version`이 Go `1.27.0`을 보고하는지 확인합니다(예:
  `go version go1.27.0 linux/amd64`). 다른 버전이면 실행을 거부하고 Go
  `1.27.0`을 먼저 설치하거나 선택합니다.
- 기준 입력은 저장소의 `testdata/contracts/valid.json`입니다.

## 실행

깨끗한 셸에서 다음 명령을 순서대로 실행합니다.

```bash
make check
go run ./cmd/agentctl contract validate testdata/contracts/valid.json
go run ./cmd/agentctl contract preview testdata/contracts/valid.json
```

## 확인 기준

`make check`는 `go vet ./...`와 전체 Go 테스트를 통과해야 합니다.
검증 명령은 다음 JSON을 출력해야 합니다.

```json
{"valid":true}
```

미리보기에는 저장소와 기준 커밋, 전체 목표, 하위 작업, 작업 소유와 의존성,
제외 범위, 검증 방법, 승인 후 자동 진행 문맥이 포함되어야 합니다. 마지막
문장은 다음과 같습니다.

```text
승인 후 Agent가 구현·독립 확인·자동 검사·일반 변경의 기본 브랜치 반영까지 진행합니다.
```

이 Foundation increment의 세 명령에는 GHES/API 쓰기 경로가 설계되어 있지
않습니다. 실행 전후 `git status --short`를 비교하면 저장소의 작업 트리가
깨끗한지 확인할 수 있지만, 이 확인만으로 저장소 밖 파일 시스템 상태를
검증할 수는 없습니다.
