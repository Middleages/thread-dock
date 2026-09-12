# 최종 옛 실행 엔진 dependency inventory

기준 SHA: `2db8f776d22afd849fcb9a1328f7e8aff9b72e50`. 2026-09-10 inventory를 최신 사용자 Monitor/Skills 변경에 맞춰 재조사했다.

## 보존

| 경로 | 실제 소비자·목적 |
|---|---|
| `monitor/` | 제품 Go/Wails·React, GitHub/Herdr 읽기·settings·GHES·Toolbox·Markdown·링크/경로/clipboard |
| `internal/runner/` | Linux·Windows Monitor의 유일한 repo-internal production import; 명령 인자/timeout/Windows console 경계 |
| `internal/projecttemplate/` | 제품 template 검증 seam. canonical 여섯 Skill·두 Agent의 Makefile template-check와 함께 유지 |
| `project-template/` | Coordinator/Feature Leader와 여섯 canonical Skill·네 migration shim. 옛 Go engine import 없음 |
| `go.mod`, `go.sum` | direct dependency는 Wails; 옛 엔진이 별도 외부 모듈을 요구하지 않으므로 변경 불필요 |

Toolbox/settings/GHES는 monitor 내부와 표준 라이브러리를 사용하며 agentctl·legacy state/config에 의존하지 않는다. 이름이 같은 `internal/monitor`는 옛 집계 엔진으로 제품 `monitor/`와 구별한다.

## 제거 순서·역의존 증명

| Task | 제거 군집 | 삭제 직전 소비자 상태 |
|---|---|---|
| 1 | cmd/agentctl, internal/cli/config/pilot, pilot script 및 Makefile 진입점 | config 소비자는 함께 제거할 cmd/pilot뿐. Monitor 경로와 독립 |
| 2 | internal/orchestrator | Task 1 뒤 production/test importer 0 |
| 3 | internal/githubpublication/workflow/workrun/monitor와 coordinator integration_test | Task 1 뒤 production importer 0. workflow 외부-test 소비자 하나를 함께 제거 |
| 4 | internal/herdr/coordinator | Task 3 뒤 coordinator 외부 소비자는 같은 Task의 herdr뿐 |
| 5 | internal/github/worktree | Task 2·3·4 뒤 각각 외부 importer 0 |
| 6 | v1 mergegate/recovery/retirement/review/scheduler/state/contract/testfixture/version | 상위 소비자 제거 후 v1 군집 제거. state/v2와 contract/v2를 건드리지 않음 |
| 7 | registry/runtime/state-v2/contract-v2 | v2 상위 소비자 제거 후 제거, shared 부모 디렉터리 때문에 Task 6 뒤 직렬 |
| 8 | dag/pathscope/opencodeagent | foundations 삭제 후 최종 orphan |

정확 ownedPaths와 focused 검사는 [계획](../superpowers/plans/2026-09-12-engine-retirement-completion.md)에 있다. Task 2·3만 동일 reviewed base에서 독립 병렬화할 수 있다. 나머지는 직전 reviewed 통합 SHA에 의존한다.

## 설정·fixture·문서

- legacy 설정은 internal/config이고 사용자 Monitor settings.json/config.json 또는 locator와 동일한 것이 아니다. 사용자 파일을 삭제하지 않는다.
- tracked 전용 fixture는 internal/herdr/testdata와 testdata/contracts의 v1 두 JSON·v2 JSON 하나다. 해당 consumer package와 같은 Task에서 제거한다.
- scripts/single-run-pilot.sh와 internal/pilot은 옛 실행기의 운영/검증 harness다. 삭제 후 Makefile은 cmd 디렉터리와 pilot script를 요구하지 않아야 한다.
- README·quickstart·현재 설계/계획을 active로 유지한다. 과거 2026-09-07~09 설계·pilot·이전 slice ledger는 기록으로 남기며 재실행 지시가 아님을 명시한다.
- 실제 worktree, 런타임 snapshot/event/config/registry 데이터, Windows staging, 중단된 codex-runtime은 삭제 대상이 아니다.

## 조사 근거와 한계

Sol read-only inventory: git SHA, Linux Go 1.27 go list Imports/TestImports/XTestImports, GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go list Monitor/runner, repo-wide rg와 exact 파일 manifest 조회. 테스트 suite 미실행.
Herdr 0.8.2의 현재 WSL status/agent list 읽기는 exit 0이었다. 이 환경에는 wsl.exe가 없어 이번 Windows→WSL 호출은 검증하지 않았다. 실제 runtime model/effort identity는 unverified.
삭제 완료 여부와 Task fixed SHA·검증은 [ledger](2026-09-12-engine-retirement-completion-ledger.md)에 기록한다. 이 inventory 자체는 삭제 완료 증거가 아니다.
