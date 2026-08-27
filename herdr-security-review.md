# Herdr v0.8.2 보안·개인정보 검토

| 항목 | 값 |
|---|---|
| 판정 | **조건부 승인** |
| 대상 | Herdr v0.8.2, commit `9eb521456ac0d19d3ab3d9d7cea3cca10baa8a4c` |
| 사용 환경 | 사내망 Linux 개발 호스트, Codex CLI·OpenCode CLI, Worktree 병렬 구현 |
| 검토 방식 | 공식 문서와 고정 tag 소스의 정적 검토 |
| 제외 | 동적 침투 테스트, 패킷 캡처, Codex/OpenCode 자체의 모델 전송 정책 |

## 1. 결론

Herdr 핵심이 prompt, 소스 코드, pane 출력 또는 session 내용을 Herdr 운영사로 전송하는 telemetry·analytics·crash-report 기능은 이번 v0.8.2 정적 검토에서 발견되지 않았다. 기본 외부 통신은 버전 확인과 agent-detection manifest 확인을 위한 정적 URL의 HTTP GET이다. Codex와 OpenCode 공식 integration도 session ID와 lifecycle state를 로컬 Herdr socket으로 보고하며 prompt나 transcript 본문을 Herdr 운영사에 보내지 않는다.

그러나 Herdr는 보안 격리 도구가 아니다. 같은 Herdr session에서 실행되는 Agent와 Plugin은 local socket을 통해 다른 pane의 출력을 읽고 입력을 보내며 프로세스 정보를 볼 수 있다. Plugin은 sandbox 없이 현재 사용자 권한과 환경을 상속한다. 따라서 다음 조건을 모두 만족할 때만 사내망 사용을 승인한다.

1. 외부 version·manifest check를 비활성화한다.
2. 검토한 v0.8.2 바이너리와 Skill을 내부 mirror에서 설치한다.
3. v0.1에서는 Herdr Plugin을 설치하지 않는다.
4. 하나의 OS 사용자와 Herdr session을 하나의 신뢰 경계로 취급한다.
5. production credential과 장기 secret을 Herdr 실행 환경에 상시 노출하지 않는다.
6. pane history를 비활성화하고 Herdr 상태 디렉터리 권한을 사용자 전용으로 만든다.

## 2. 확인된 외부 통신

| 동작 | 목적지 | 시점 | 전송되는 애플리케이션 데이터 | 조치 |
|---|---|---|---|---|
| version check | `https://herdr.dev/latest.json` 또는 preview URL | 기본 활성, 시작 시와 약 30분마다 | 코드·prompt 없음. 일반적인 IP, TLS, `curl` 요청 정보는 서버·프록시에 보임 | `version_check = false` |
| agent manifest check | `https://herdr.dev/agent-detection/index.toml`과 하위 manifest | 기본 활성, 시작 시와 약 30분마다 | pane snapshot을 업로드하지 않고 탐지 규칙을 내려받음 | `manifest_check = false` |
| 수동 update | manifest가 지정한 release asset | 사용자가 `herdr update` 실행 | 다운로드 요청 | 내부 패키지로만 수동 갱신 |
| `herdr --remote` bootstrap | Herdr manifest·release asset 또는 SSH 대상 | 호환 binary가 remote에 없을 때 | 플랫폼·다운로드 요청; 이후 terminal frame은 SSH로 대상 host와 교환 | 일반 SSH 후 remote host에서 Herdr 실행 |
| Plugin 설치 | GitHub 및 Plugin build가 접근하는 임의 목적지 | 사용자가 설치할 때 | Git clone 정보; build·runtime은 임의 데이터 전송 가능 | v0.1 금지 |
| Codex/OpenCode 모델 호출 | 각 agent가 설정한 provider | agent 사용 중 | provider 정책에 따라 prompt·코드·도구 결과 | Herdr와 별도의 보안검토 필요 |

`version_check`와 `manifest_check`의 기본값이 모두 `true`이며, source는 각각 `herdr.dev`의 고정 URL을 `curl`로 조회한다. [Update config source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/config/model.rs#L31-L45), [version check source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/update.rs#L1-L27), [manifest check source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/detect/manifest_update.rs#L15-L18)

정적 검토에서 Herdr core의 Sentry, analytics, OpenTelemetry exporter 같은 client telemetry 의존성이나 전송 코드는 발견하지 못했다. 이는 “검토한 v0.8.2 소스에서 발견하지 못했다”는 결과이며 이후 버전에 대한 보증은 아니다.

## 3. 로컬에서 처리·저장되는 데이터

### Live pane buffer

Herdr terminal은 화면에 표시된 prompt, agent 응답, 명령 출력과 secret을 메모리에 보유한다. Agent 상태 탐지는 최근 bottom-buffer snapshot을 로컬에서 읽는다. 이 snapshot을 manifest 서버에 업로드하는 코드는 발견하지 못했다.

local socket 접근자는 `pane.read`, `agent.read`, `pane.process_info`를 호출할 수 있다. 공식 Socket API는 pane 출력 읽기뿐 아니라 입력 전송, Agent prompt, integration 설치, server stop까지 제공한다. [Herdr Socket API](https://herdr.dev/docs/socket-api/)

### `session.json`

기본 session snapshot에는 다음 정보가 저장된다.

- Workspace·tab 이름과 전체 작업 경로
- Worktree 정보
- pane label과 agent 종류
- agent session ID 또는 session 경로
- pane launch argv

prompt와 일반 pane 출력은 기본 `session.json`에 저장되지 않는다. 다만 명령행 인자로 secret을 전달하면 `launch_argv`에 남을 수 있다. [Session snapshot source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/persist/snapshot.rs#L14-L20), [Pane snapshot source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/persist/snapshot.rs#L97-L109)

Linux 기본 위치는 일반적으로 `~/.config/herdr/session.json`이며 named session은 그 아래 `sessions/<name>/`에 저장된다.

### `session-history.json`

`pane_history = true`이면 최근 terminal 내용이 별도 `session-history.json`에 저장된다. 공식 문서는 이 파일에 secret, token, prompt와 command output이 포함될 수 있어 기본값을 꺼 두었다고 명시한다. [Session state and restore](https://herdr.dev/docs/session-state/)

`pane_history = false`를 유지한다. v0.8.2 source는 history가 비활성화되면 session save 시 기존 history 파일을 제거한다. [Persistence source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/persist/io.rs#L63-L74)

### 로그

`herdr.log`, `herdr-client.log`, `herdr-server.log`가 session directory에 생성된다. 기본 log level은 `info`, 파일당 최대 크기는 5 MiB이며 source의 기본 retained sibling 수는 0이다. 로그는 API method, pane·workspace ID, 경로와 오류 문자열을 포함할 수 있으므로 support에 전달하기 전에 검열한다. `HERDR_LOG=debug` 또는 `trace`에서는 raw terminal input byte가 기록될 수 있으므로 운영 환경에서 사용하지 않는다. [Configuration — Logs](https://herdr.dev/docs/configuration/), [logging source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/logging.rs#L9-L30), [raw input logging source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/raw_input.rs#L200-L211)

### 파일 권한

Unix socket 자체는 v0.8.2에서 `0600`으로 설정한다. [API socket source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/api/server.rs#L27-L27), [socket binding source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/api/server.rs#L88-L94)

반면 일반 session JSON, log, Plugin config/state directory 생성 경로에서는 사용자 전용 mode를 명시적으로 설정하는 코드를 확인하지 못했다. 실제 mode는 OS의 umask와 ACL에 의존한다는 소스 기반 추론이다. 공유 Linux host에서는 Herdr를 처음 시작하기 전 `umask 077`을 적용하고 다음 상태를 유지한다.

```text
~/.config/herdr       directory 0700
~/.local/state/herdr  directory 0700
files                 0600
```

## 4. Codex·OpenCode integration

### Codex

설치 시 `~/.codex` 또는 `CODEX_HOME` 아래에 hook script를 쓰고 `hooks.json`, `config.toml`을 수정한다. Hook은 Codex의 hook JSON 전체를 임시 파일에 받아 읽지만, local Herdr socket에 보내는 값은 pane ID, agent 이름, sequence, Codex session ID와 session start source다. transcript path는 유효성 판단에만 사용되며 요청에 포함되지 않는다. 임시 파일은 종료 trap으로 삭제한다. [Official integration docs](https://herdr.dev/docs/integrations/#codex), [v0.8.2 Codex hook source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/integration/assets/codex/herdr-agent-state.sh#L10-L97)

### OpenCode

설치된 JavaScript Plugin은 session ID와 `working`, `blocked`, `idle` lifecycle state를 local socket으로 보고한다. 검토한 source는 prompt 내용, tool argument 또는 tool 결과를 report payload에 넣지 않는다. [Official integration docs](https://herdr.dev/docs/integrations/#opencode), [v0.8.2 OpenCode integration source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/integration/assets/opencode/herdr-agent-state.js#L61-L118)

### 잔여 위험

보고된 session ID 또는 session path는 `session.json`에 남고 같은 사용자 권한을 가진 프로세스가 native resume에 사용할 수 있다. 이는 인증 credential은 아니지만 대화 session의 식별자다. 민감도가 높은 환경에서는 integration을 설치하지 않고 bundled/local screen detection만 사용한다. `[session] resume_agents_on_restore = false`는 자동 재개를 막지만 integration이 session reference를 보고·저장하는 것 자체를 보안 경계로 차단하지는 않는다.

## 5. 가장 중요한 위험

### H-1 — Plugin은 신뢰된 코드와 동일한 권한을 가짐

Herdr Plugin은 sandbox되지 않는다. build와 runtime command는 현재 사용자 권한으로 실행되고 환경을 상속하며 full Herdr CLI와 socket path를 받는다. invocation context에는 작업 경로, Worktree, agent 상태, 선택한 terminal text와 클릭한 URL이 포함될 수 있다. Plugin command의 stdout·stderr는 각각 최대 64 KiB씩 최근 200개 command에 대해 server memory log로 남을 수 있다. [Herdr Plugins — Trust and security](https://herdr.dev/docs/plugins/#trust-and-security), [Plugin runtime source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/app/api/plugins/runtime.rs#L11-L15), [Plugin context source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/api/schema/plugins.rs#L361-L393)

악성 또는 침해된 Plugin은 환경변수의 token, 소스 파일, 다른 pane 출력, 선택 텍스트를 외부로 전송할 수 있다. 설치 preview가 있어도 build command 자체가 설치 과정에서 실행되므로 `--yes` 자동 설치와 floating branch 설치를 금지한다.

**v0.1 조치:** Plugin 기능 전체를 사용하지 않는다. 이후 필요할 때 source, manifest, build dependency를 검토하고 정확한 commit을 내부 mirror에 고정한 자체 Plugin만 허용한다.

### H-2 — 같은 사용자·session의 Agent 사이에 권한 분리가 없음

Herdr는 각 managed pane에 `HERDR_SOCKET_PATH`를 주입한다. Socket API에는 per-agent ACL이나 capability token이 없으며 하나의 제어면을 공유한다. 따라서 Builder 하나가 다른 pane을 읽거나 입력을 보내고 server를 중지할 수 있다. [Integration environment](https://herdr.dev/docs/integrations/#integrate-your-own-agent), [Socket control surface](https://herdr.dev/docs/socket-api/#what-you-can-control)

**조치:**

- 같은 회사 프로젝트와 같은 민감도 작업만 한 OS 사용자·Herdr session에 둔다.
- 서로 다른 고객, 개인 데이터, 보안 등급은 OS 사용자·컨테이너·개발 VM을 분리한다.
- production shell과 autonomous Builder를 같은 Herdr session에 두지 않는다.
- named session은 정리 단위일 뿐 보안 격리 경계로 간주하지 않는다.

### M-1 — 기본 외부 통신

기본 설정으로 Herdr와 agent manifest 서버에 주기적으로 접속한다. 요청 body에 프로젝트 데이터는 없지만 폐쇄망 정책과 egress audit 요구에는 어긋난다.

**조치:** 두 check를 설정과 방화벽에서 함께 차단한다.

### M-2 — 상속된 환경변수

Herdr pane과 Plugin process는 실행 사용자의 환경을 상속한다. Herdr core가 이를 업로드하는 것은 아니지만, Agent가 실행한 명령이나 Plugin은 값을 읽을 수 있다.

**조치:** Herdr를 secret이 export되지 않은 전용 계정·sanitized environment에서 시작한다. 필요한 credential은 작업별 최소 범위 helper 또는 secret store를 통해 짧게 주입하고 terminal 명령행에 직접 넣지 않는다.

### M-3 — Windows local API pipe 검증 필요

v0.8.2의 일반 local listener는 Windows에서 명시적인 private security descriptor 없이 named pipe를 생성하고, 이후 permission 제한 함수는 Windows에서 no-op이다. 같은 source에는 owner·SYSTEM만 허용하는 별도의 private-listener 구현이 있지만 일반 API server 경로는 이를 호출하지 않는다. [Local listener source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/ipc.rs#L54-L75), [Private listener source](https://github.com/herdrdev/herdr/blob/v0.8.2/src/ipc.rs#L141-L168)

Windows의 기본 named-pipe ACL은 LocalSystem·관리자·creator owner에 full control, Everyone·anonymous에 read를 허용한다. [Microsoft Named Pipe Security](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)

이 정적 검토만으로 read-only 계정이 duplex Herdr API 연결을 실제 성립시킬 수 있는지는 확인하지 못했다. 따라서 Windows multi-user host의 사용은 보류하고, 예정대로 Linux host를 사용한다. Windows 사용이 필요하면 실제 pipe DACL과 비소유 사용자 연결 실패를 동적 시험한다.

### L-1 — Clipboard·notification·remote bridge

terminal 선택을 OS clipboard로 복사하면 다른 로컬 application이 접근할 수 있다. system notification에는 작업 이름이나 오류 요약이 노출될 수 있다. `herdr --remote`는 local image clipboard를 remote temp file로 복사할 수 있다. [Remote access docs](https://herdr.dev/docs/persistence-remote/)

**조치:** `ui.toast.delivery = "herdr"`를 유지하고, 일반 SSH 후 remote host에서 Herdr를 실행한다. `keys.remote_image_paste = ""`로 remote image 단축키를 비활성화하고 민감 텍스트를 system clipboard로 복사하지 않는다.

## 6. 폐쇄망 권장 설정

```toml
onboarding = false

[update]
version_check = false
manifest_check = false

[experimental]
pane_history = false

[keys]
remote_image_paste = ""

[ui.toast]
delivery = "herdr"
```

이 설정만으로 보안이 완성되지는 않는다. 방화벽 egress deny, 내부 binary mirror, 사용자·프로젝트 신뢰 경계 분리와 함께 적용한다.

## 7. 배포 전 통과 조건

- [ ] v0.8.2 binary, source tag, LICENSE, SHA-256을 내부 저장소에 보관했다.
- [ ] `version_check = false`, `manifest_check = false`를 적용했다.
- [ ] 방화벽 또는 proxy log에서 Herdr core의 외부 접속이 없음을 확인했다.
- [ ] `pane_history = false`이며 기존 `session-history.json`이 없다.
- [ ] `HERDR_LOG`를 `debug` 또는 `trace`로 설정하지 않았다.
- [ ] Herdr config·state directory가 사용자 전용 권한이다.
- [ ] Plugin 목록이 비어 있다.
- [ ] Codex/OpenCode integration source와 변경되는 config diff를 검토했다.
- [ ] production credential이 Herdr process 환경에 없다.
- [ ] 민감도가 다른 프로젝트와 개인 데이터가 별도 OS 경계에 있다.
- [ ] incident 발생 시 session·log 파일을 안전하게 수집·폐기하는 절차가 있다.

## 8. 최종 판정

권장 v0.1 구조인 **Linux 전용 개발 계정 + 외부 check 차단 + pane history off + Plugin 없음 + production credential 분리**에서는 Herdr를 사내 구현 런타임으로 사용할 수 있다.

다음 중 하나라도 필요하면 재검토한다.

- Herdr 버전 변경
- Marketplace 또는 자체 Plugin 도입
- Windows multi-user host 사용
- 서로 다른 보안 등급의 Agent를 같은 사용자로 실행
- Herdr session에 production credential 또는 개인정보 처리 terminal 포함
- 외부에서 Herdr socket 또는 terminal frame으로 연결하는 bridge 도입
