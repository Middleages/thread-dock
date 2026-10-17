# GitHub와 Herdr로 바로 시작하기

ThreadDock은 실행 엔진이 아니다. GitHub Issue/PR/Projects를 업무 원본으로 사용하고, Herdr가 top-level session/worktree/Agent lifecycle을 맡는다. ThreadDock은 프로젝트별 Coordinator와 feature별 Feature Leader의 상태를 GitHub 업무와 한 화면에 연결한다.

역할 소유권, 요청별 lifecycle, 근거 경계와 현재 개선 검토는 [Agent workflow 아키텍처](../architecture/agent-workflow.md)를 함께 본다.

새 프로젝트에서 상주 역할은 두 개다. 세부 작업은 필요할 때만 Feature Leader가 native leaf에 맡긴다.

```text
Coordinator -> Herdr Feature Leader -> transient planner/implementer/reviewer -> PR/handoff
```

Monitor와 `~/.threaddock/sessions.json`에는 Coordinator와 Feature Leader만 top-level 연결로 기록한다. 내부 native subagent는 별도 Herdr pane이나 locator 항목으로 만들지 않는다.

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
- `grill-plan`: 중요한 기능 계획의 독립 반대 검토와 남은 중요 이견의 조건부 중립 판정
- `tdd-task`: bounded implementation subagent용 leaf 구현
- `review-change`: fresh independent reviewer용 read-only 검토
- `publish-work`: PR/Issue/Project/Wiki/locator 정리

`plan-work`, `open-agent-session`, `implement-task`, `record-work`는 기존 프롬프트 호환을 위한 짧은 migration shim으로만 남긴다.

Herdr를 조작할 때는 공용 보조 Skill `herdr-local`을 함께 사용한다. 여섯 업무 Skill의 역할을 바꾸지 않고, 설치된 CLI 문법·실제 실행 위치·대상 확인을 담당한다.

필요한 세부 작업에는 다음 native leaf와 공용 Skill을 대응시킨다.

| native leaf | 공용 Skill | 책임 |
|---|---|---|
| `td_explorer` | `explore-codebase` | 코드 흐름·재사용 지점·영향 테스트 탐색 |
| `td_docs_editor` | `write-project-docs` | 독자·목적에 맞는 문서 편집과 근거 보존 |
| `td_implementer` | `tdd-task` | 직접 구현·focused test·진단 가능한 로그 |
| `td_reviewer` | `review-change` | exact SHA 독립 검토 |

이 leaf들은 고정 pipeline이 아니다. 필요한 경우에만 선택하며, 대상 도구가 named leaf를 지원하지 않으면 같은 Skill과 범위를 허용된 native agent에 전달한다. 권한 거부를 우회하거나 모델 fallback을 추가하지 않는다.

### Herdr 사용과 실제 네트워크 제한

`project-template/.agents/skills/herdr-local/`을 다른 공용 Skill과 함께 복사한다. Codex와 OpenCode가 같은 파일을 사용하며, 전역 설치도 아래 공용 경로를 따른다.

- 설치된 `herdr --help`와 `herdr --skill`을 문법 근거로 사용한다. 웹 문서 조회·업데이트·다운로드는 하지 않는다.
- 관리 pane 안에서 확인한 실제 ID로 로컬 상태·출력을 조회한다. 세션·pane 생성과 Agent 입력은 승인된 대상·작업에 한정한다. `HERDR_ENV`를 임의로 설정하거나 관리 pane 밖에서 대상 세션을 추측해 조작하지 않는다.
- 기존에 승인된 작업은 설정된 GitHub 서비스와 모델 provider를 사용해 진행한다. 모든 환경에 내부 전용 통신 증명이나 반복 승인을 요구하지 않는다. 이 helper는 로컬 Herdr 조작용이며 설치·업데이트·SSH 원격 운영은 제공하지 않는다.
- 로컬 CLI라고 모델 통신까지 로컬인 것은 아니다. 사용자가 폐쇄망·오프라인·허용 endpoint 제한을 명시했다면 해당 작업의 실제 통신 경로에 그 제한을 적용하고 하위 Agent에도 전달한다. 제한과 충돌하거나 필요한 경로가 불명확할 때만 그 동작을 pending으로 남기며, 허용된 독립 작업은 계속한다. hostname 추측이나 금지 endpoint 접속으로 검증하지 않는다.
- GitHub 기록은 기존 업무 Skill과 기존 승인 범위를 따른다. 명시된 제한 때문에 게시할 수 없으면 로컬 handoff를 남기고 미게시로 표시한다.

이 Skill은 행동 지침이며 방화벽·네트워크 격리를 설치하거나 보장하지 않는다. 운영 환경의 외부 통신 차단은 별도 정책으로 유지한다. Monitor의 기존 Windows→WSL 읽기 경로를 변경하는 기능도 아니다.

### 전역 설치를 선택할 때

다른 컴퓨터의 Agent에게 설치를 맡길 때는 [전역 Agent 설치 지침](global-agent-setup.md)을 전달한다. 설치 원본 ref, 백업·병합, 운영체제별 실제 경로와 설치 후 확인을 함께 다룬다.

자연어 요청에서 매번 Skill 이름이나 primary 역할을 먼저 지정하지 않아도 되도록, tracked 자동 진입 원본 `project-template/global/AGENTS.md`도 함께 설치한다. 이 짧은 지침은 프로젝트 조정 요청을 `coordinate-work`, 배정된 단일 기능을 `develop-feature`로 연결하되, 작은 명확 작업과 일반 질문은 가볍게 유지한다. 모든 root session을 Coordinator로 고정하거나 runtime model·권한을 바꾸지 않으며, 더 가까운 프로젝트 `AGENTS.md`와 명시 역할이 우선한다.

프로젝트별 설치 대신 모든 프로젝트에서 사용하려면 아래 대상으로 개별 파일·스킬 폴더를 복사한다. 전역 설치와 프로젝트 설치를 중복 유지하면 같은 이름의 지침이 달라질 수 있으므로 한 범위를 선택한다.

| 원본 | 전역 대상 |
|---|---|
| `project-template/global/AGENTS.md` | Codex: `~/.codex/AGENTS.md`, OpenCode: `~/.config/opencode/AGENTS.md` (실제 custom home/config 사용 시 그 경로) |
| `project-template/.agents/skills/<skill>/` | `~/.agents/skills/<skill>/` (공용) |
| `project-template/.codex/agents/td_*.toml` | `~/.codex/agents/` (Codex) |
| `project-template/.opencode/agents/td_*.md` | `~/.config/opencode/agents/` (OpenCode) |

`~`는 도구를 실행하는 사용자의 홈이다. WSL에서 실행하면 Linux 홈(예: `/home/appuser`), Windows에서 직접 실행하면 그 Windows 사용자의 홈을 기준으로 한다. OpenCode의 XDG 설정이나 Codex의 설정 디렉터리를 바꿨다면 실제 경로를 따른다. 기존 전역 파일은 먼저 백업·비교한다. 저장소 root나 `project-template/AGENTS.md`를 전역으로 복사하지 말고 위의 짧은 global 원본만 사용하며, `CONTEXT.md`는 전역 규칙으로 복사하지 않는다.

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

상주 Agent 정의는 다음 두 개다.

```text
td_coordinator
td_feature_leader
```

native leaf 정의도 함께 복사한다: `td_explorer`, `td_docs_editor`, `td_implementer`, `td_reviewer`. leaf는 model/effort를 고정하지 않고 대상 Codex 설정과 권한·인증·sandbox를 상속한다. 실제 Codex 설치에서 모델 이름이나 custom agent 기능이 지원되지 않으면 임의 대체를 성공으로 보고하지 않는다.

위 모델 값은 저장소의 Codex 예시다. 이미 선택한 모델을 자동으로 바꾸는 설치 지시가 아니다. 배포된 TOML에도 모델 값이 있으므로 복사 전에 대상 환경의 지원 모델·정책에 맞는지 확인한다.

### OpenCode

`td_coordinator.md`와 `td_feature_leader.md`는 둘 다 `mode: primary`다. `td_explorer`, `td_docs_editor`, `td_implementer`, `td_reviewer`는 `mode: subagent`인 세부 역할이다. 대상 저장소에서 OpenCode를 열고 primary를 선택한다. Feature Leader는 독립 feature의 Herdr 세션에서 사용하고, 필요한 세부 작업만 native subagent로 분배한다.

기본 `build` 역할이 custom primary를 자동 선택하는 것으로 가정하지 않는다. 설치된 `opencode --help`가 지원하면 `opencode --agent td_coordinator` 또는 `opencode --agent td_feature_leader`로 시작한다. Feature Leader는 제품 코드·테스트·리뷰 후 수정까지 native `task` child에 맡기고, native task가 없거나 거부되면 해당 작업을 pending으로 보고한다. 기존 권한·모델을 바꾸어 우회하지 않는다.

Markdown 에이전트는 기존 Codex TOML과 별개 형식이다. 두 primary 정의는 `model`·effort·권한 override를 넣지 않아 사용자의 기존 설정을 따른다. 네 leaf 정의도 model/effort는 생략해 상속하지만, native 위임을 막는 `task: deny`를 명시하고 explorer/reviewer에는 `edit: deny`를 추가한다. 이는 도구 권한 설정이며 OS 격리를 보장한다는 뜻이 아니다. [OpenCode Agent 설정](https://opencode.ai/docs/agents/)

설치 후 대상 저장소에서 다음으로 인식 여부를 확인한다.

```bash
opencode agent list
opencode debug skill
```

두 primary, 네 leaf와 `project-template/.agents/skills`의 전체 Skill inventory가 보이는지 확인한다. leaf는 task 위임을 하지 않으며 explorer/reviewer는 edit가 거부된다. Skill 또는 native task 권한이 제한됐다면 해당 기능은 사용할 수 없다. Codex 설정 stanza를 OpenCode에 복사하거나 권한을 자동 완화하지 말고 실제 설치의 설정을 확인한다. 목록 조회 성공은 모델 호출·Herdr session 생성·GitHub 쓰기 성공의 근거가 아니다.

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
- 목표의 모호함·범위·우선순위·의존성을 정리한 brief 인계
- 독립 feature와 dependency 구분
- 이미 존재하는 exact Feature Leader session 재사용
- 필요한 feature만 새 Herdr top-level session/worktree로 배정
- shared interface/file 충돌만 직렬화
- 전역 sessions locator exact upsert
- blocker/PR/review 결과를 GitHub 근거와 함께 보고

Coordinator는 일반적인 제품 구현자가 아니다. 작은 docs/config 변경처럼 feature session 비용이 더 큰 경우를 제외하면 구현은 Feature Leader에 맡긴다.

Coordinator는 `coordinate-work`로 목표·범위·우선순위·의존성·공유 제약을 정리한 brief를 넘긴다. 완성된 아키텍처 전체계획을 먼저 수렴시키는 gate는 없다. 기능 계획의 단일 소유자는 Feature Leader다.

질문 전에 원본 요청·Issue·문서·코드의 관련 근거를 읽는다. 사용자/흐름/범위/화면 의미/완료조건/데이터 손실/외부 효과가 달라지는 제품 선택만 구체적 선택지와 영향으로 묻는다. 자료로 확인 가능한 사실은 직접 조사하고, 구조·파일·테스트 내부는 Feature Leader에게 맡기며, 무해하고 되돌릴 수 있는 가정은 밝히고 진행한다. ‘좋게·빠르게·자동화’도 문맥이 뜻을 정할 수 있으며, 숫자 목표가 없다는 이유로 질문하거나 임의 수치를 만들지 않는다. 위험도와 모호함은 별개다.

작은 명확한 수정은 직접 brief로 넘긴다. 사용자가 이미 합의한 계획은 Feature Leader가 인수·실행가능성을 확인해 재사용하며 재작성·재승인을 반복하지 않는다. 미결정에 의존하는 배정만 기다리고 독립 승인 업무·기존 세션·초안은 유지한다. 계획-only 요청은 구현·새 기능 세션·GitHub 쓰기 허가가 아니다.

## 6. Feature Leader 흐름

Coordinator가 Feature Leader에게 넘기는 packet은 최소한 다음만 포함한다.

```text
Issue URL
repository / target branch
원본 요청 / 확인된 목표·acceptance·non-goals
사실 / 가정 / 열린 제품 결정
이미 합의한 계획 reference / revision (있을 때)
권한 범위 / 담당 Feature Leader / shared constraints
feature worktree
선행 dependency
shared interface/file 주의사항
```

Feature Leader는 `develop-feature`를 entrypoint로 사용한다.

Feature Leader가 기능 계획과 파일·Task·테스트를 소유한다. 합의된 계획의 유효한 결정/검토 근거는 재사용하고 구현 세부는 이유를 남겨 조정할 수 있다. 기능 간 계약·의존성은 Coordinator가 영향분만 조정하며, 위임받지 않은 제품 범위/동작 변경은 사용자에게 선택을 돌린다. 계획만 맡았다면 구현·게시 없이 계획 결과까지만 전달하되, 중요한 계획에 필요한 독립 plan review와 owner resolution은 완료한다. 명시적 아이디어/초안 요청만 `unreviewed draft`로 끝낼 수 있다.

```text
Issue -> develop-feature
      -> 대안 탐색이 필요하면 brainstorm-design
      -> 중요한 새 기능 계획/실질적 위험 변경은 grill-plan
      -> 독립 task별 tdd-task implementation subagent
      -> 통합 / focused verification
      -> fresh review-change reviewer
      -> publish-work
      -> PR / Issue handoff / Project / 필요 시 Wiki
```

구현 child는 모든 제품 코드·테스트 편집에 사용한다. 작은 단일 task는 child 하나로 처리하고, 독립적으로 쓸 수 있는 여러 task만 1~3개 정도의 별도 worktree로 나눈다. 항상 최대치를 채우지 않는다. worker별 full suite는 기본 경로가 아니며, affected focused test와 final integration gate를 구분한다.

Reviewer는 구현자의 자기평가를 대신하는 Agent가 아니다. exact SHA/diff와 acceptance evidence를 fresh context에서 읽고 `accept` 또는 구체적인 `block` finding을 반환한다. 직접 코드를 수정하거나 merge하지 않는다.

### 계획·화면 품질·완료 설명

공통 개발 기준은 [development-stance](../../project-template/.agents/skills/develop-feature/references/development-stance.md)에 있다. 현재 구조를 존중하고 필요한 책임만 계층으로 나누며, 빈 계층·중복 상태·근거 없는 추상화와 gate를 추가하지 않는다. 이 기준을 계획·구현 packet부터 공유한다.

중요한 새 기능 계획이나 실질적 위험 변경은 `grill-plan`에서 고정 revision의 독립 반대 검토 한 번을 실제 native reviewer에게 받고 작성자가 근거로 해결한다. 합의되면 중립 판정 없이 끝낸다. 중요한 이견이 남을 때만 다른 판정자를 한 번 호출하며, 그래도 미해결이면 구체적 근거 부족/사람의 선택을 명시하고 의존 작업만 기다린다. 계획 검토는 실행가능성과 source 근거를 평가하며 아직 구현하지 않은 screenshot·성공 tests를 요구하지 않는다. 실제 코드 리뷰의 UI/테스트 근거 요구는 그대로 유지한다. 작은 수정과 변하지 않은 합의는 절차를 반복하지 않는다. 계획 응답/handoff의 한 줄에는 plan revision, 실제 reviewer 결과 참조와 verdict, 남은 중요 항목, implementation readiness를 남기며 별도 상태 저장소를 만들지 않는다.

UI는 [UI quality](../../project-template/.agents/skills/develop-feature/references/ui-quality.md)를 따른다. 중요한 문구·화면/flow 변경은 신규 사용자·반복 작업자·편집/접근성 관점의 독립 제안을 비교하고, 작성자를 가린 평가와 한 방향의 최종 편집을 거친다. 오타 수준은 직접 수정한다. 모든 UI 변경은 실제 렌더링 screenshot을 이미지 입력 가능한 Agent가 열어 시각 검사하고 관련 동작을 실행해야 한다. screenshot 저장이나 DOM snapshot만으로 시각 검토를 통과시키지 않는다.

Codex는 사용 가능한 browser/image 도구, OpenCode는 기존 Playwright MCP 또는 CLI를 쓰며 설치된 `agent-browser`도 대안이다. 모델을 자동 교체하거나 도구를 중복 설치하지 않는다. 이미지 입력·렌더링·동작 도구가 없으면 해당 검증을 미검증으로 남긴다. 브라우저 React fixture와 실제 packaged Wails 검증은 별개다.

완료·대기·차단 결과는 `publish-work`에서 문제/요청, 사용자에게 달라진 점, 선택 이유와 tradeoff, 실제 검증과 한계, 필요한 다음 행동을 짧게 설명한다. 기존 `impeccable`은 UI 작업, `codebase-design`은 어려운 interface, `domain-modeling`은 용어·상태 의미 충돌, `writing-for-agents`는 Agent 지침 편집 때만 선택해 재사용한다. 새로운 필수 plugin이나 항상 실행할 추가 단계는 아니다.

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
