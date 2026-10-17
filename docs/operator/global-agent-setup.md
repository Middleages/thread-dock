# 다른 컴퓨터에 ThreadDock 전역 Skill·Agent 설치하기

이 문서는 설치를 수행할 Agent에게 전달하는 지침이다. 저장소 URL과 사용할 branch/ref를 받아 **그 컴퓨터에서 도구를 실행하는 사용자**의 전역 설정에 ThreadDock 지침을 설치한다. Monitor 앱 빌드나 ThreadDock 제품 개발은 필요하지 않다. 저장소의 HANDOFF에 적힌 과거 작업을 재개하는 요청도 아니다.

## 사용자 전달 프롬프트

아래 문구를 다른 컴퓨터의 Agent에게 보내면 `main` 기준으로 설치한다. 특정 버전을 설치하려는 경우에만 `main`을 전달받은 branch 또는 tag로 바꾼다.

```text
https://github.com/Middleages/thread-dock 의 main 브랜치를 새 폴더에 받아
docs/operator/global-agent-setup.md를 읽고 이 컴퓨터의 현재 사용자에 전역 Skill과 Agent를 설치해줘.
설치된 Codex/OpenCode를 확인해 사용하는 도구에 적용하고, 둘 다 있으면 둘 다 적용해줘.
기존 설정·모델·provider·권한은 보존하고 같은 이름의 파일은 백업·비교 후 병합해줘.
제품 빌드, 이전 세션 복원, GitHub 쓰기는 하지 말고 설치 경로·원본 commit·확인 결과를 알려줘.
```

## 1. 원본과 실행 환경 확인

1. 전달받은 URL/ref를 새 clone 또는 별도 worktree에서 연다. 기존 dirty checkout을 reset하거나 branch를 바꾸지 않는다. 새 clone 예시는 다음과 같으며 placeholder를 실제 ref와 사용하지 않는 경로로 바꾼다.

   ```bash
   git clone --branch <ref> --single-branch https://github.com/Middleages/thread-dock.git <new-directory>
   git -C <new-directory> rev-parse HEAD
   git -C <new-directory> status --short
   ```

   정확한 commit SHA를 받았다면 해당 commit을 포함하는 ref를 받아 별도 checkout에서 그 SHA를 확인한다. 설치 파일은 이 checkout의 **tracked `project-template/`**에서만 가져오고, 사용한 ref와 SHA를 결과에 기록한다.
2. 현재 shell이 Linux/WSL인지 native Windows인지, Codex/OpenCode 실행 파일이 어느 환경 것인지 확인한다. Bash에서는 `command -v codex opencode`, PowerShell에서는 `Get-Command codex,opencode -ErrorAction SilentlyContinue`를 사용할 수 있다. 설치된 도구의 `--version`·`--help`로 지원 명령을 확인한다. 없는 도구를 자동 설치하거나 도구 버전을 바꾸지 않는다.
3. 사용자 홈과 실제 설정 디렉터리를 확인한다. Linux/WSL의 홈과 Windows의 사용자 프로필은 별도 설치 범위다. WSL에서 Windows 실행 파일을 호출하는 등 혼합 환경이면 **해당 실행 파일이 실제 읽는 경로**를 먼저 확인하며 두 홈에 추측으로 중복 설치하지 않는다.

| 설치 항목 | 기본 대상 | 사용자 지정이 있을 때 |
|---|---|---|
| 공용 Skill | 사용자 홈의 `.agents/skills/` | 두 도구의 실제 발견 경로를 확인. `CODEX_HOME` 변경만으로 공용 Skill 경로도 옮겨졌다고 가정하지 않음 |
| Codex Agent | 사용자 홈의 `.codex/agents/` | `CODEX_HOME`을 사용한다면 실제 Codex home과 그 `agents/` 경로 확인 |
| OpenCode Agent | 사용자 홈의 `.config/opencode/agents/` | `XDG_CONFIG_HOME`, `OPENCODE_CONFIG_DIR`, 실행 옵션과 실제 config 경로 확인. `OPENCODE_CONFIG`는 config **파일** 지정이므로 곧바로 Agent 디렉터리로 쓰지 않음 |

Bash의 `$HOME`, PowerShell의 `$HOME`/`$env:USERPROFILE`는 확인용으로 읽고 다른 값으로 덮어쓰지 않는다. native Windows에서도 홈 기준 경로를 실제로 해석하며 Linux의 `/home/...`를 복사하지 않는다. 경로가 충돌하면 도구의 실제 로드 결과를 기준으로 목적지를 정한다. Codex의 사용자 Skill과 Agent 경로, config home은 [Skills](https://learn.chatgpt.com/docs/build-skills), [Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents), [Config locations](https://learn.chatgpt.com/docs/config-file/config-advanced)를 참고한다. OpenCode의 custom 경로는 [Config](https://opencode.ai/docs/config/)를 따른다.

## 2. 백업하고 필요한 지침만 병합

원본 inventory는 `git ls-files project-template/global/AGENTS.md project-template/.agents/skills project-template/.codex/agents project-template/.opencode/agents`로 확인한다. 현재 배포본은 짧은 자동 진입 지침, canonical 6개와 helper·호환 Skill, harness별 역할 6개를 포함한다. 숫자만 맞추지 말고 선택한 source commit의 실제 파일을 기준으로 설치한다.

| 원본 | 설치 내용 |
|---|---|
| `project-template/global/AGENTS.md` | Codex는 실제 Codex home의 `AGENTS.md`, OpenCode는 실제 config directory의 `AGENTS.md`. 두 도구를 쓰면 같은 원본을 각각 설치 |
| `project-template/.agents/skills/` | **각 Skill 디렉터리 전체**. `SKILL.md`뿐 아니라 `references/`, `scripts/`, `assets/` 등 해당 commit에 있는 supporting files를 포함 |
| `project-template/.codex/agents/*.toml` | Codex를 사용할 때 역할 6개 |
| `project-template/.opencode/agents/*.md` | OpenCode를 사용할 때 역할 6개 |

역할은 `td_coordinator`, `td_feature_leader`, `td_explorer`, `td_docs_editor`, `td_implementer`, `td_reviewer`다. 공용 Skill은 한 벌만 설치하고 두 도구에 동일 폴더를 중복 복제하지 않는다. `brainstorm-design`, `agent-wait` 같은 helper와 legacy shim도 source inventory에 있으면 함께 보존한다.

자동 진입 지침은 모든 root session을 Coordinator로 바꾸는 기본 역할 설정이 아니다. 자연스러운 요청을 현재 요청·명시 역할·더 가까운 프로젝트 규칙에 맞는 Skill로 연결하는 짧은 fallback이다. Codex와 OpenCode의 전역 시작 파일 위치는 다르지만 같은 tracked 원본을 사용한다. 기존 전역 `AGENTS.md`가 있으면 다른 설정과 마찬가지로 전체 덮어쓰기 대신 백업·비교·병합하며, 프로젝트의 더 가까운 `AGENTS.md`가 구체적인 구현·검증 정책을 추가하거나 우선할 수 있다.

- 기존 목적지의 같은 이름 파일·폴더와 symlink 실제 대상을 확인하고, 변경할 파일을 사용자 소유의 별도 백업 디렉터리에 원래 상대 경로대로 보관한다. 없는 원본과 신규 설치 파일 목록도 기록해 되돌릴 대상을 구별한다. 사용자 홈이나 전체 설정 디렉터리를 통째로 덮어쓰거나 지우지 않는다.
- 같은 이름 파일이 없으면 source를 설치한다. 있으면 diff를 읽고 ThreadDock의 workflow·reference 연결을 병합하면서 사용자 고유 지침과 설정을 보존한다. 변경 없는 파일은 그대로 둔다. 오래된 파일을 자동 삭제하지 말고 이번 source와의 차이로 보고한다.
- **모델 설정은 대상 컴퓨터가 기준이다.** 기존 role의 `model`, effort, provider, permission, sandbox/approval 값은 보존한다. 신규 Codex primary 파일에 source의 예시 `model`/`model_reasoning_effort`가 들어 있더라도 자동으로 적용하지 않는다. 사용자가 지정한 role별 매핑이 없으면 신규 파일에서는 그 예시 두 필드를 제외해 현재 도구의 모델 설정을 상속하고, 이 차이를 기록한다. 기존 매핑이 있으면 그것을 유지하며 지원되지 않는 모델은 임의 대체하지 않는다.
- OpenCode 신규 leaf에는 source의 `mode`와 `task: deny`, explorer/reviewer의 `edit: deny`를 유지한다. 기존 역할 병합에서는 현재 권한을 완화하지 않는다. Codex TOML을 OpenCode config로 변환하거나 기존 전체 config를 교체하지 않는다. [OpenCode Agents](https://opencode.ai/docs/agents/)
- 저장소 **루트**의 `.codex/`, `AGENTS.md`, `CONTEXT.md`는 ThreadDock 자체 개발용이다. 이를 전역에 복사하지 않는다. 전역 자동 진입 원본은 `project-template/global/AGENTS.md` 하나다. `project-template/AGENTS.md`, `.github/`, CI 설정도 이번 전역 설치 대상이 아니다. 모델-profile/OMO 설계 문서는 실행 설정이나 설치 명령이 아니므로 자동 적용하지 않는다.
- 다른 컴퓨터의 `auth.json`, API key·token, provider credential, shell 환경, history/session, `.threaddock/sessions.json`·lock, Herdr locator와 worktree state를 이식하지 않는다. 기존 인증과 명시된 네트워크 제약을 사용한다. 일반 설치에 별도 내부망 인증 gate를 만들지 않으며, 실제 제한으로 막히는 동작만 구체적으로 보고한다.

`impeccable`, `codebase-design`, `domain-modeling`, `writing-for-agents` 등 선택 보조 Skill이 이미 있으면 해당 작업에서 재사용한다. 없다는 이유로 core 설치를 막거나 비슷한 plugin을 중복 설치하지 않는다.

## 3. 전역 설치와 실제 적용을 나누어 확인

1. 설치한 자동 진입 지침과 Skill의 frontmatter·상대 reference가 유효하고 supporting files가 빠지지 않았는지 확인한다. Codex TOML과 OpenCode Markdown frontmatter를 사용할 도구의 parser/발견 결과로 확인한다. 원본과 다른 파일은 사용자 설정 보존 때문에 남긴 차이인지 설명한다.
2. 전역 확인은 ThreadDock clone이나 기존 프로젝트 밖의 빈 임시 폴더에서 수행한다. **이 저장소의 루트 `.codex/agents`와 `AGENTS.md`는 개발용 설정이라 전역 검증을 가릴 수 있다.** OpenCode는 설치 버전이 지원하는 `opencode agent list`, `opencode debug skill`로 실제 이름과 경로를 확인한다. Codex는 provider 호출 없이 볼 수 있는 새 세션의 Skill/Agent 목록으로 확인한다. 해당 버전이 세션 시작만으로 모델을 호출한다면 로컬 parse까지만 수행하고 발견 검증은 미검증으로 남긴다. `codex agents`는 설치 버전에 따라 실행 세션 목록이므로 custom role 설치 검증이라고 단정하지 않는다. 실제 자동 진입은 Skill 이름을 프롬프트에 쓰지 않은 작은 격리 요청으로 별도 확인하며, 설치/parse 성공만으로 routing 성공을 주장하지 않는다.
3. 실제 작업할 프로젝트에서도 동명 `.codex/agents`, `.opencode/agents`, `.agents/skills`와 config override를 확인한다. local Agent가 global을 가리거나 같은 이름 Skill이 함께 보일 수 있다. 임의 삭제 대신 유효 경로와 충돌을 보고하고, 사용자가 해당 프로젝트 병합까지 요청했다면 같은 백업/병합 방식으로 그 범위만 맞춘다. [Codex Skill discovery](https://learn.chatgpt.com/docs/build-skills), [OpenCode Skill discovery](https://opencode.ai/docs/skills/)
4. Herdr를 사용할 환경이면 실행 파일 존재와 `herdr --help`, `herdr --skill`을 확인한다. 실제 상태/제어는 [herdr-local](../../project-template/.agents/skills/herdr-local/SKILL.md)의 managed context·실제 ID·기존 승인 범위를 따른다. `HERDR_ENV`를 주입하거나 설치 확인을 위해 새 세션을 임의 시작하지 않는다. Herdr가 없어도 Skill 파일 설치 결과는 보존하고 Herdr 실행만 미검증으로 남긴다.
5. UI 검증 도구는 이미 있는 browser/image 경로, Playwright MCP/CLI, `agent-browser`를 확인한다. 실행 파일 존재와 `--help`는 실제 화면 검증이 아니다. 실제 UI 작업에서는 image-capable Agent의 screenshot 직접 열람과 관련 interaction 결과가 있어야 한다. 모델 자동 교체나 도구 중복 설치로 보완하지 말고 없는 능력은 미검증으로 보고한다.

이 설치 요청만으로 실제 모델 호출, GitHub 쓰기, Herdr session 생성, 과거 작업 재개를 수행하지 않는다. 사용자가 동작 smoke까지 요청했다면 기존 승인·설정 안에서 작은 격리 작업으로 확인하고 그 결과를 별도로 기록한다. 파일 발견 성공을 native task/vision 또는 Wails 앱 동작 성공으로 표현하지 않는다.

## 4. 설치 결과 전달

설치 Agent는 다음을 짧게 보고한다.

- source URL, 전달받은 ref, 실제 commit SHA와 설치한 도구/OS·사용자 범위
- 실제 Skill/Agent 목적지와 백업 위치, 설치·병합·보존한 파일, source와 남은 차이
- 유지한 모델·권한 정책, 프로젝트 override가 있으면 그 유효 경로
- 수행한 parse/발견 검사와 결과, 실제 실행을 하지 않은 항목·누락된 도구
- 설치로 무엇이 달라졌는지, 필요한 경우에만 다음 행동

설치 후 사용 흐름은 [GitHub-first 빠른 시작](github-first-quickstart.md)을 따른다. root 개발 설정을 복사하지 않아도 `coordinate-work` → `develop-feature`의 업무 흐름과 공용 품질 기준을 사용할 수 있다.
