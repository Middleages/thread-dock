# ThreadDock

여러 프로젝트를 에이전트와 진행하면서 요청, 결정, 구현, 검증과 문서를 Git/GitHub에
연결하는 로컬 개발 운영 도구입니다. 프로젝트를 다시 열었을 때 현재 상태와 다음
행동을 빠르게 회수하는 것을 목표로 합니다.

## 현재 설계와 구현 상태

2026-09-07부터 프로젝트 중심 MVP를 기준으로 개발합니다.
프로젝트는 저장소 하나 또는 여러 개를 묶고, 업무는 저장소별 실행·PR을 연결합니다.
ThreadDock Monitor, GitHub Projects와 Wiki가 MVP에 포함됩니다.

이번 변경은 설계 전환과 문서 정리입니다. 현재 Go 코드는 기존 v1 구현을 포함하며
새 다중 프로젝트 흐름·UI·Contract v2의 구현 완료를 뜻하지 않습니다.
기존 Run을 새 MVP에서 실행하는 호환성은 목표가 아닙니다.

## 시작 문서

- [현재 MVP 설계](docs/superpowers/specs/2026-09-07-project-workflow-mvp-design.md)
- [제품 정의](PRODUCT.md)
- [용어 모델](CONTEXT.md)
- [현재 handoff와 구현 순서](HANDOFF.md)
- [설계 전환 결정 ADR 0006](docs/adr/0006-project-workflow-mvp.md)

충돌하는 이전 설계·계획은 현재 tree에서 삭제했고 Git 이력에서 확인할 수 있습니다.
docs/operator와 project-template은 v1 참고 자료입니다. 새 기능은 현재 설계를 따릅니다.

## 기존 구현 참고

- [Foundation 파일럿](docs/operator/foundation-pilot.md)
- [Single-run 운영 절차](docs/operator/single-run-pilot.md)
- [OpenCode 역할 routing](docs/operator/opencode-role-agents.md)

위 절차는 기존 binary 설명이며 새 MVP의 실행 계획이나 완료 조건이 아닙니다.

## 검증

구현 중에는 focused 검증을, 최종 코드 변경에는 make check를 수행합니다.
문서 변경은 링크·내용 일관성 검사와 실제 Go 검사를 구분해 보고합니다.

```sh
make test-focused PKGS="./internal/contract"
make vet-focused PKGS="./internal/contract"
make check
```

PKGS가 비어 있으면 focused target은 실패합니다.
검증 근거는 명령, 대상 commit, 결과와 소요 시간을 남깁니다.
Issue·PR에는 결정과 근거를 요약하고 credential·환경 dump·전체 transcript는 남기지 않습니다.
