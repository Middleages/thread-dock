# 자연어 자동 진입 검증

## 범위와 Task packet

- `taskId`: `workflow-natural-entry`
- `baseSHA`: `f0ac862`
- `deps`: 없음. 기존 `coordinate-work`, `develop-feature`, development stance를 재사용한다.
- `ownedPaths`: `AGENTS.md`, `project-template/global/AGENTS.md`, `docs/operator/global-agent-setup.md`, `docs/operator/github-first-quickstart.md`, 이 검증 기록과 `HANDOFF.md`의 최신 절
- `worktree`: `/home/appuser/dev_system/.worktrees/workflow-natural-entry`
- `branch`: `agent/workflow-natural-entry`
- `forbiddenPaths`: Go/Wails·React 제품 코드와 테스트, 모델/provider/권한 설정, hooks, Herdr locator/session, 사용자 dirty checkout
- `interface`: `project-template/global/AGENTS.md`가 자동 진입의 canonical source다. Codex는 실제 Codex home의 `AGENTS.md`, OpenCode는 실제 config directory의 `AGENTS.md`에 같은 내용을 설치한다. 더 가까운 프로젝트 지침과 명시 역할이 우선하며 설치는 runtime model이나 역할을 선택하지 않는다.
- `acceptance`: 자연스러운 조정 요청은 `coordinate-work`, 배정된 단일 기능은 `develop-feature`로 진입한다. 작은 명확 작업과 일반 질문은 가볍게 유지한다. 제품 모호함만 좁게 묻고, 모호한 범위에서 외부 쓰기·Herdr 제어를 시작하지 않는다. 다른 컴퓨터 설치 경로와 override 관계를 문서화한다.
- `tests`: tracked/global parity, link·template parse, 네 개의 이름 없는 fresh Codex root 요청. 기존 UI/전체 suite는 범위 밖이며 반복하지 않는다.
- `result`: `changedFiles`는 위 owned paths, 구현 commit은 `07ceb8455faa28884b379dc428c4c31baadc674d`다. 아래 명령과 결과를 근거로 한다. `unverified`는 OpenCode 모델 실행, 실제 Herdr/GitHub 쓰기, runtime model/effort identity다. `blockers`는 독립 리뷰 전에는 없음이며 최종 리뷰 결과를 이 문서에 추가한다.

## 변경과 설치

12개 규칙의 짧은 global source를 추가했다. 모든 root를 Coordinator로 고정하지 않고 요청을 분류한다. 문서 오타처럼 작은 변경은 직접 처리하되 프로젝트가 제품 코드 child를 요구하면 한 bounded child를 사용한다. 일반 질문과 읽기 보고는 세션·수정·리뷰 ceremony 없이 답한다. 단일 기능의 계획 소유권은 Feature Leader에 유지한다.

저장소 root `AGENTS.md`의 무조건적인 여섯 문서 시작 읽기는 개발·조정 작업에서 최신 HANDOFF와 PRODUCT를 먼저 읽고, CONTEXT·quickstart·현재 Go/Wails 설계/계획은 관련될 때만 읽도록 좁혔다. 일반 질문·읽기 보고는 필요한 원본만 확인한다. 과거 설계나 기존 합의는 변경하지 않았다.

설치 직전 `/home/appuser/.codex/AGENTS.md`, `/home/appuser/.config/opencode/AGENTS.md`, `/home/appuser/.agents/AGENTS.md`는 모두 없었다. 따라서 교체된 파일과 backup은 없다. canonical source를 앞의 Codex/OpenCode 두 경로에 새로 설치했고 SHA-256은 모두 `74afe7b8aa284e7af76426fd419e8e84ea6c79f9c935b93e8affd818a678d918`로 같았다. 기존 Skill·role·config·hook은 수정하지 않았다. 설치 직후 상위 fresh turn에도 `ThreadDock natural entry`가 자동 주입된 것이 확인됐지만, 이는 지침 로딩 근거일 뿐 모델·역할 전환의 근거가 아니다.

## 이름 없는 fresh root 시험

Codex CLI `0.163.0`의 기존 config/auth를 그대로 사용했다. `/tmp/threaddock-natural-entry-20261016` 아래 격리 fixture에서 `codex exec --ephemeral --json -C <fixture>`를 실행했고 model override, `--ignore-user-config`, trust/hook 우회, GitHub 쓰기와 Herdr 제어는 사용하지 않았다. 최초 plain 실행은 fixture가 Git 저장소가 아니어서 `Not inside a trusted directory`로 종료했다. 우회 옵션을 쓰지 않고 fixture 자체만 `git init`한 뒤 실행했다.

| case / thread | 자연어 요청 | 관찰 결과 | per-run usage |
|---|---|---|---|
| 일반 읽기 / `01a144d2-7456-70b1-9cfa-d7abab095383` | 폴더 목적 설명, 읽기 전용 | README와 파일 목록만 확인하고 답변. 수정·session·review 없음 | input 55,836 / cached 43,520 / output 291 |
| 작은 typo / `01a144d3-3383-76c1-adfe-a31f5ae94f27` | README의 명백한 오타 하나 수정 | child나 feature ceremony 없이 `teh`를 `the`로 직접 수정하고 재확인 | input 75,604 / cached 62,976 / output 313 |
| 여러 일 우선순위 / `01a144d3-fa66-7730-982f-0b4858b7fd80` | 폴더 근거로 local recommendation | 이름 없이 `coordinate-work`를 선택. 영향·미합의 dependency를 구분하고 외부 쓰기·session 없이 종료 | input 60,049 / cached 44,800 / output 570 |
| 배정된 단일 migration 계획 / `01a144d4-a4ee-7340-8873-d242b27b3e2f` | JSON→SQLite planning-only | 이름 없이 `develop-feature`를 선택. 파일·session을 바꾸지 않고 계획과 실제 repo 부재/독립 plan review 미검증을 명시 | input 92,200 / cached 62,976 / output 1,740 |

네 실행 모두 별도 전역 `superpowers:using-superpowers` 문서를 실제로 읽었다. 큰 per-run input에는 항상 로드되는 skill/plugin metadata와 이 문서가 기여했을 가능성이 있지만, usage만으로 전체 비용의 원인을 하나로 확정할 수는 없다. 이번 승인 범위는 ThreadDock 자동 진입이며 다른 plugin/skill을 disable하거나 설정을 바꾸지 않았다. OpenCode `1.19.0`의 설치 경로와 source parity는 확인했지만 provider 모델 실행은 비용과 중복을 늘리지 않기 위해 수행하지 않았다.

## 정적 검증과 도구화 판단

실행 명령:

```text
git diff --check
cmp -s project-template/global/AGENTS.md /home/appuser/.codex/AGENTS.md
cmp -s project-template/global/AGENTS.md /home/appuser/.config/opencode/AGENTS.md
test -f project-template/.agents/skills/{coordinate-work,develop-feature}/SKILL.md
test -f project-template/.agents/skills/develop-feature/references/development-stance.md
make template-check
```

모두 exit 0이었다. 실제 반복은 source parity·링크 확인용 짧은 shell 명령뿐이었다. 새 helper는 유지비에 비해 이득이 없고 hook은 모든 invocation의 비용과 trust 표면을 늘리므로 만들지 않았다. 기존 `/home/appuser/.codex/hooks.json`도 보존했다. 설치 drift가 여러 호스트에서 실제 반복 문제로 확인될 때 read-only 검사만 별도 검토하면 된다.

## 한계와 리뷰

- 실제 runtime model/effort identity는 CLI event에서 확인할 수 없어 `unverified`다. 역할 파일이나 설치 지침을 근거로 실행 model을 주장하지 않는다.
- OpenCode fresh model 행동, Windows/native startup, 실제 GitHub/Herdr 동작은 이번 범위에서 실행하지 않았다.
- 제품 코드/UI 변경이 없어 `make check`, browser fixture, Wails 검증은 실행하지 않았다.
- fresh `td_reviewer`가 최종 고정 SHA와 `d3b2b1e` 이후 범위를 독립 검토한 결과를 아래에 기록한다.
