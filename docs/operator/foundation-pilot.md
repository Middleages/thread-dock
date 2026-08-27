# Foundation contract-only pilot

이 절차는 ThreadDock Foundation의 첫 계약 전용 파일럿입니다. 승인 전에
계약을 검증하고 미리보기만 생성하며, GHES Issue·Project·PR에는 쓰지 않습니다.
미리보기는 stdout으로만 출력되고 저장소 파일을 변경하지 않습니다.

## 사전 조건

- 저장소 루트에서 실행합니다.
- Go `1.27.0`을 사용합니다. 아래 명령은 이 저장소에서 고정된 도구체인을
  우선 사용하도록 `PATH`를 설정합니다.
- 기준 입력은 저장소의 `testdata/contracts/valid.json`입니다.

## 실행

깨끗한 셸에서 다음 명령을 순서대로 실행합니다.

```bash
export PATH="/home/appuser/.local/share/threaddock-toolchains/go1.27.0/bin:$PATH"
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

이 Foundation increment에서는 GHES 쓰기를 수행하지 않습니다. 실행 전후
`git status --short`를 비교해 저장소 밖 파일과 저장소 파일이 변경되지 않았는지
확인하십시오.
