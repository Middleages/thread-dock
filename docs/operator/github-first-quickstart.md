# GitHub와 Herdr로 바로 시작하기

ThreadDock은 실행 엔진이 아니다. GitHub Issue/PR/Projects를 업무 원본으로 사용하고, Herdr가 top-level session/worktree/Agent lifecycle을 맡는다. ThreadDock은 프로젝트별 Coordinator와 feature별 Feature Leader의 상태를 GitHub 업무와 한 화면에 연결한다.

새 프로젝트에서 기억할 역할은 두 개뿐이다.

```text
Coordinator -> Herdr Feature Leader -> transient planner/implementer/reviewer -> PR/handoff
```

Monitor와 `~/.threaddock/sessions.json`에는 Coordinator와 Feature Leader만 top-level 연결로 기록한다. 내부 native subagent는 기록하지 않는다.

## 1. 프로젝트에 ThreadDock 템플릿 넣기

ThreadDock checkout의 다음 폴더를 대상 저장소에 복사한다.

```text
project-template/.agents/skills  -> <target>/.agents/skills
project-template/.codex/agents  -> <target>/.codex/agents   # Codex 사용 시
project-template/.opencode/agents -> <target>/.opencode/agents # OpenCode 사용 시
```

Codex와 OpenCode를 함께 쓰면 세 경로를 모두 넣는다. 스킬 원본은 `.agents/skills` 하나이며 도구별로 복제할 필요가 없다. 기존 같은 이름의 파일은 통째로 덮어쓰지 말고 차이를 확인한다. 루트 `AGENTS.md`, 기존 모델 설정, CI 파일은 ThreadDock 템플릿 때문에 덮어쓰지 않는다.

Canonical Skill은 여섯 개다.

- `coordinate-work`: Coordinator의 GitHub 업무 분해, Herdr feature session 배정, 전역 locator 관리
- `develop-feature`: Feature Leader의 feature 전체 lifecycle
- `grill-plan`: 모호하거나 cross-cutting한 요청을 구현 가능하게 구체화
- `tdd-task`: bounded implementation subagent용 leaf 구현
- `review-change`: fresh independent reviewer용 read-only 검토
- `publish-work`: PR/Issue/Project/Wiki/locator 정리

`plan-work`, `open-agent-session`, `implement-task`, `record-work`는 기존 프롬프트 호환을 위한 짧은 migration shim으로만 남긴다.

### 전역 설치를 선택할 때

프로젝트별 설치 대신 모든 프로젝트에서 사용하려면 아래 대상으로 개별 파일·스킬 폴더를 복사한다. 전역 설치와 프로젝트 설치를 중복 유지하면 같은 이름의 지침이 달라질 수 있으므로 한 범위를 선택한다.

| 원본 | 전역 대상 |
|---|---|
| `project-template/.agents/skills/<skill>/` | `~/.agents/skills/<skill>/` (공용) |
| `project-template/.codex/agents/td_*.toml` | `~/.codex/agents/` (Codex) |
| `project-template/.opencode/agents/td_*.md` | `~/.config/opencode/agents/` (OpenCode) |

`~`는 도구를 실행하는 사용자의 홈이다. WSL에서 실행하면 Linux 홈(예: `/home/appuser`), Windows에서 직접 실행하면 그 Windows 사용자의 홈을 기준으로 한다. OpenCode의 XDG 설정이나 Codex의 설정 디렉터리를 바꿨다면 실제 경로를 따른다. 기존 전역 파일은 먼저 백업·비교하고, `AGENTS.md`와 `CONTEXT.md`는 전역 규칙으로 복사하지 않는다.

OpenCode는 공용 `~/.agents/skills`와 `.agents/skills`를 공식 지원한다. OpenCode 전용 `~/.config/opencode/skills`에도 설치할 수 있지만 두 도구를 함께 쓸 때는 공용 경로 하나를 권장한다. [OpenCode Skill 경로](https://opencode.ai/docs/skills/)
Codex도 같은 공용 Skill 경로를 읽으며, custom agent는 별도 TOML을 읽는다. [Codex Skill 경로](https://learn.chatgpt.com/docs/build-skills), [Codex custom agent](https://learn.chatgpt.com/docs/agent-configuration/subagents)

## 2. 도구별 Agent 설정

### Codex

대상 저장소의 `.codex/config.toml`에 이미 `[agents]` 설정이 있으면 그대로 사용한다. 없다면 기존 설정을 보존하면서 다음 stanza만 병합한다.

```toml
[agents]
enabled = true
max_concurrent_threads_per_session = 6
default_subagent_model = "gpt-5.6-sol"
default_subagent_reasoning_effort = "medium"
```

Agent 정의는 다음 두 개다.

```text
td_coordinator
td_feature_leader
```

기본 역할은 Coordinator/Feature Leader가 Sol medium, 구현 subagent가 Luna high 같은 구현 중심 모델을 우선하는 구조다. 실제 Codex 설치에서 모델 이름이나 custom agent 기능이 지원되지 않으면 임의 대체를 성공으로 보고하지 않는다.

위 모델 값은 저장소의 Codex 예시다. 이미 선택한 모델을 자동으로 바꾸는 설치 지시가 아니다. 배포된 TOML에도 모델 값이 있으므로 복사 전에 대상 환경의 지원 모델·정책에 맞는지 확인한다.

### OpenCode

`td_coordinator.md`와 `td_feature_leader.md`는 둘 다 `mode: primary`다. 대상 저장소에서 OpenCode를 열고 Tab으로 역할을 선택한다. Feature Leader는 독립 feature의 Herdr 세션에서 사용하고, 내부 구현·리뷰만 native subagent로 분배한다. 내부 subagent를 별도 Herdr pane이나 locator 항목으로 만들지 않는다.

Markdown 에이전트는 기존 Codex TOML과 별개 형식이다. OpenCode 템플릿은 `model`·effort·권한 override를 넣지 않아 사용자의 기존 설정을 따른다. primary agent의 모델을 생략하면 전역 모델을 사용한다. [OpenCode Agent 설정](https://opencode.ai/docs/agents/)

설치 후 대상 저장소에서 다음으로 인식 여부를 확인한다.

```bash
opencode agent list
opencode debug skill
```

두 역할과 여섯 canonical Skill이 보이는지 확인한다. Skill 또는 native task 권한이 제한됐다면 해당 기능은 사용할 수 없다. Codex 설정 stanza를 OpenCode에 복사하거나 권한을 자동 완화하지 말고 실제 설치의 설정을 확인한다. 목록 조회 성공은 모델 호출·Herdr session 생성·GitHub 쓰기 성공의 근거가 아니다.

## 3. 전역 sessions 파일 한 번만 만들기

프로젝트별 locator 파일을 만들지 않는다. 선택한 WSL 사용자 기준으로 하나만 사용한다.

```bash
mkdir -p ~/.threaddock
cat > ~/.threaddock/sessions.json <<'EOF'
{
  "version": 1,
  "bindings": []
}
EOF
```

Monitor 설정에는 WSL 내부 절대 경로를 넣는다.

```text
/home/appuser/.threaddock/sessions.json
```

실제 `$HOME`이 다르면 그 값을 사용한다.

여러 프로젝트의 Coordinator가 같은 파일을 쓸 수 있으므로 Skill은 `~/.threaddock/sessions.lock` 같은 user-level exclusive lock 아래에서 locator를 다시 읽고, 자기 exact binding만 upsert/cleanup하고, temporary file을 atomic rename하도록 요구한다. lock을 안전하게 얻지 못하면 파일을 수정하지 않고 pending으로 남긴다.

Feature binding은 다음 세 조건이 모두 확인될 때만 삭제한다.

1. Issue closed
2. 관련 PR 작업 finished
3. Herdr top-level Feature Leader session absent

`idle`, `done`, 일시적인 관찰 실패만으로 삭제하지 않는다.

## 4. Monitor 설정

Windows ThreadDock Monitor 설정에서 다음을 입력한다.

```text
GitHub Host: github.samsungds.net
GitHub 저장소: FDYPhotoDX/jmj
GitHub Projects: https://github.samsungds.net/orgs/FDYPhotoDX/projects/4
WSL 배포판: Ubuntu
Herdr 연결 파일: /home/appuser/.threaddock/sessions.json
```

GitHub.com이면 Host는 `github.com`이다. GitHub Enterprise Server는 `https://` 없이 hostname만 넣는다. Project URL은 `/views/2`가 아닌 `/orgs/.../projects/N` 또는 `/users/.../projects/N` 루트 URL을 사용한다.

선택한 WSL 안의 `gh`도 같은 host에 인증되어 있어야 한다.

```bash
gh auth status --hostname github.samsungds.net
```

## 5. 프로젝트 Coordinator 시작

대상 저장소를 연 Herdr pane에서 `td_coordinator`를 사용한다. 시작 프롬프트는 길게 쓰지 않는다.

```text
ThreadDock coordinator로 이 프로젝트를 맡아줘.
GitHub Issue/Project를 업무 원본으로 사용하고 coordinate-work로 열린 일을 정리해.
```

Coordinator는 다음을 맡는다.

- 열린 Issue/PR/Project 상태 확인
- 독립 feature와 dependency 구분
- 이미 존재하는 exact Feature Leader session 재사용
- 필요한 feature만 새 Herdr top-level session/worktree로 배정
- shared interface/file 충돌만 직렬화
- 전역 sessions locator exact upsert
- blocker/PR/review 결과를 GitHub 근거와 함께 보고

Coordinator는 일반적인 제품 구현자가 아니다. 작은 docs/config 변경처럼 feature session 비용이 더 큰 경우를 제외하면 구현은 Feature Leader에 맡긴다.

## 6. Feature Leader 흐름

Coordinator가 Feature Leader에게 넘기는 packet은 최소한 다음만 포함한다.

```text
Issue URL
repository / target branch
feature 목적과 acceptance criteria
feature worktree
선행 dependency
shared interface/file 주의사항
```

Feature Leader는 `develop-feature`를 entrypoint로 사용한다.

```text
Issue -> develop-feature
      -> 필요할 때만 grill-plan
      -> 독립 task별 tdd-task implementation subagent
      -> 통합 / focused verification
      -> fresh review-change reviewer
      -> publish-work
      -> PR / Issue handoff / Project / 필요 시 Wiki
```

구현 worker는 독립 task가 있을 때만 1~3개 정도 사용한다. 항상 최대치를 채우지 않는다. worker별 full suite는 기본 경로가 아니며, affected focused test와 final integration gate를 구분한다.

Reviewer는 구현자의 자기평가를 대신하는 Agent가 아니다. exact SHA/diff와 acceptance evidence를 fresh context에서 읽고 `accept` 또는 구체적인 `block` finding을 반환한다. 직접 코드를 수정하거나 merge하지 않는다.

## 7. 여러 프로젝트를 동시에 쓸 때

각 프로젝트는 자기 Coordinator를 가진다.

```text
JMJ Coordinator -----┐
  |- Feature #12     |
  `- Feature #18     |
                     |
PIECE Coordinator ---+--> ~/.threaddock/sessions.json
  `- Feature #31     |
                     |
LONA Coordinator ----┘
  `- Feature #7
```

한 Coordinator가 여러 codebase의 전체 문맥을 장기간 유지하지 않는다. ThreadDock Monitor가 여러 프로젝트 Coordinator와 Feature Leader를 한 화면에 모은다.

## 8. GitHub와 Herdr의 완료 의미를 섞지 않기

GitHub가 업무 상태의 원본이다.

- Issue `OPEN/CLOSED`
- PR `OPEN/CLOSED/MERGED`
- Project Status/Priority/Waiting
- checks/review 결과

Herdr는 실행 관찰이다.

- session 존재 여부
- Agent `working/blocked/idle/done`
- workspace/tab/pane/cwd

Agent `done`은 Issue 완료가 아니고, GitHub Project `Done`도 Herdr session 종료 명령이 아니다.

## 9. Monitor 빌드와 검증

제품은 Windows Go/Wails 앱이다. Vite/Node는 화면 개발/빌드에 사용하지만 별도 Node HTTP monitor server는 제품 실행 조건이 아니다.

저장소 전체 검증:

```bash
make template-check
make check
```

Windows package:

```powershell
cd monitor
wails build
.\build\bin\ThreadDockMonitor.exe
```

실제 Windows -> WSL, GHES, Herdr, Markdown 화면은 packaged app에서 별도 smoke evidence로 확인한다. component test나 계획만으로 live 통합 성공을 주장하지 않는다.
