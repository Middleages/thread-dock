# 두 번째 엔진 정리 ledger

## 기준과 보존

- baseSHA: `22dcc6610b34582d9a3a4ee375e7661fde499e5e`. PR #70 병합 후 local main과 origin/main 일치, main clean.
- 통합 worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice2`, branch `agent/engine-retirement-slice2`.
- 이전 slice는 importer 없는 `internal/monitorcli` 두 파일을 제거했다. [기존 inventory](2026-09-10-engine-dependency-inventory.md)의 나머지 v1/v2 군집을 이번 조사 입력으로 사용한다.
- 중단된 `.worktrees/codex-runtime`의 `cmd/agentctl/main.go`, `main_test.go`, `internal/config/config.go`, `config_test.go`는 기존 modified 상태로 보존한다. 해당 worktree를 수정·stage·reset·삭제·재개하지 않는다.
- 사용자에게 받은 PR #70 병합 지시는 완료했다. 이번 후속 code PR의 main 병합은 별도 사용자 작업으로 남긴다.

## 현재 GitHub 운영 상태

- [PR #70](https://github.com/Middleages/thread-dock/pull/70): merged, merge SHA `22dcc66`. 착수 시 열린 PR 0개.
- [ThreadDock Projects](https://github.com/users/Middleages/projects/1/views/2): 저장소에 연결된 비공개 보드. 착수 시 Issue #42~48은 Todo, 병합 PR #69·70은 Done, 총 9개 항목.
- `정리 방향`: #43·48은 새 방향으로 재작성, #42·44·45·46·47은 superseded 종료 후보. Issue의 열린 상태를 완료로 바꾸지 않았다.
- 저장소는 사용자가 공개 전환했다. 현재 REST는 `private=false`, `has_wiki=true`이며 사용자도 Wiki 활성화·접근을 확인했다.
- Wiki Git 조회는 아직 `Repository not found`를 반환했다. 활성화 여부와 페이지 게시 여부는 별개이며 이번 세션의 Wiki 페이지 발행을 주장하지 않는다.

## 역할·예약

root는 범위·공유 계약·통합·GitHub 기록을 담당한다. Sol medium은 의존성 조사와 문서,
Luna high는 제품 코드·테스트, fresh Sol medium은 고정 SHA의 독립 리뷰를 담당한다.
실제 runtime model/effort를 입증할 메타데이터는 노출되지 않아 unverified다.
모델 자동 대체·승격은 하지 않는다. 구현 worker 상한 3개 중 Luna 1개를 create-revert Task에 예약하고 reviewer 슬롯은 확보한다.

## Task packet: 다음 경계 조사

- taskId: `engine-retirement-slice2-inventory`
- baseSHA: `22dcc6610b34582d9a3a4ee375e7661fde499e5e`
- deps: PR #70의 첫 slice 완료
- ownedPaths: `[]` (읽기 전용)
- worktree: `/home/appuser/dev_system`
- branch: `main` (읽기 전용)
- forbiddenPaths: 모든 쓰기·사용자 실험
- interface: 다음 작은 v1 CLI 경계와 public/shared 영향부터 직렬 제안
- acceptance: import·호출·테스트·config·docs 의존성으로 다음 삭제 후보와 정확한 경로·focused 검사 제시
- tests: 기존 inventory 재사용, `rg`/`go list` 조회만. 제품/full 검사 없음.
- result: changedFiles `[]`, commitSHA 없음. executedCommands·outcomes·unverified·blockers는 조사 결과를 받아 아래에 기록한다.

## Task packet: 운영 상태 문서

- taskId: `post70-operations-docs`
- baseSHA: `22dcc6610b34582d9a3a4ee375e7661fde499e5e`
- deps: `[]`
- ownedPaths: `HANDOFF.md`, `docs/operator/github-first-quickstart.md`, `.superpowers/sdd/engine-retirement-slice2/docs-report.md`
- worktree: `/home/appuser/dev_system/.worktrees/engine-retirement-slice2-docs`
- branch: `agent/engine-retirement-slice2-docs`
- forbiddenPaths: 소유 밖 문서·제품 코드·테스트·AGENTS·config·shared/module·기존 실험
- interface: 문서만. Monitor의 실제 Projects 환경 형식을 읽어 실행 예시에 사용한다.
- acceptance: PR #70 병합, 보드·Wiki 현재 상태와 역사 관찰 구분, native 인수 범위 과장 없음
- tests: 상대 링크와 `git diff --check`만
- result: changedFiles·commitSHA·executedCommands·outcomes·unverified·blockers를 Task report와 통합 결과에 기록한다.

## 사전 검토와 검증 환경

| Task 조합 | 공유 범위 | 판단 |
|---|---|---|
| 조사 / 운영 문서 | 같은 base 읽기만 공유, 작성 경로 중복 없음 | 독립 진행 |
| 코드 / 운영 문서 | 코드 Task는 parallel-pilot, 문서 Task는 HANDOFF/quickstart | 작성 경로 중복 없음 |
| 코드 Task 내부 | cli.Dependencies.Reverter 제거와 cmd 구성 정리 | root/Sol이 사전 직렬 합의, Luna 한 명만 shared CLI 소유 |

## 다음 삭제 경계 확정

Sol 조사는 `create-revert`가 가장 작은 v1 CLI leaf라고 확인했다. production importer는
`cmd/agentctl/main.go`와 `internal/cli/revert.go`뿐이고 scripts/config/fixture 독점 의존은 없다.
route/run.go, Dependencies/run_commands.go, cmd 구성·repository/GHES 분류와 전용 service/test를
한 Task로 제거한다. [정확한 packet·계약·검사](../superpowers/plans/2026-09-11-engine-retirement-create-revert.md)를 참조한다.
제거된 명령은 기존 usage/exit 2이며 production 설정·인증·저장소 탐색을 시작하지 않는다.
negative test의 명령 문자열은 보존할 수 있고, production 참조 부재 검사에서 테스트는 제외한다.
공용 Git/worktree helper는 후속 call inventory로 남긴다. Monitor와 runner는 변경하지 않는다.

조사 result: changedFiles `[]`, commitSHA 없음. executedCommands는 기존 inventory, route/import/reference
`rg`, Go 1.27의 Imports/TestImports/XTestImports `go list`. outcomes는 위 leaf 경계와 단독 CLI 소유 확정,
unverified는 실제 runtime identity 및 아직 수행 전인 코드 검사, blockers는 없음이다.

이전 gate에서 ext4 `/tmp`의 누적 fsync 지연을 확인했으므로 이번 테스트 fixture는 처음부터
새 `/dev/shm` 임시 경로를 사용한다. 기존 성공 tuple을 다시 실행하지 않고, 이번 code PR의
최종 통합 `make check`만 한 번 수행한다. 실패가 있으면 원인과 소유 Task를 먼저 귀속한다.
Go 1.27.0은 `/home/appuser/.local/share/threaddock/toolchains/go1.27.0/bin/go`를 사용한다.
Linux fixture/build와 Windows native 실행, live gh/Herdr 결과를 구분한다. native 오류 상태와
Projects를 사용한 두 기능의 전체 운영 검증은 여전히 미검증이다.
