# 하네스 후속 구현: CI와 PR 세션 연결

사용자는 [앞선 검토](2026-10-21-harness-review-and-simplification.md)의 후속 과제 검토 뒤 구현을 요청했다. 기준은 PR #105의 `f8d74671e526701796b8f476bb206968b94bcc9a`이며, 당시 main은 `d0af6f432f757312c2cc35551a9e497dc17af645`다. 두 revision의 제품 코드는 같았다. 이번 branch는 `agent/harness-followups-20261021`이며 main 병합은 사람에게 남긴다.

## 범위와 이유

- 저장소 CI를 실제로 연결한다. 하나의 workflow에서 환경 준비와 lockfile 설치 후 기존 `make check`를 실행한다. 경로 분류기, OS matrix, 배포·자동 병합은 추가하지 않는다.
- PR을 선택하면 관련 Issue의 기능 세션을 찾는다. 저장소 보기와 Projects-only 설정을 모두 다루며, Project 필드와 기존 GitHub·Herdr 관찰을 보존한다.
- 결정/댓글 자동 분류·요약, 새 상태 저장소·통신 엔진은 추가하지 않는다. 선택한 업무의 최근 기록 조회는 실제 재개 시험에서 탐색 부담이 확인된 뒤 판단한다.
- 실제 Windows/WSL/Herdr 운영 검증은 이 환경의 코드·fixture 검증과 분리한다. #43은 두 기능 협업·재개·오류 관찰, #48은 locator 정리, #91은 실제 앱 조작·migration의 각각 다른 완료 조건이다.

## 검토에서 확정한 계약

현재 `connectionsForWork`는 PR의 `relatedIssueUrls`를 소비하지 않았고, `mapProjectItem`은 관련 Issue·review·checks를 빈 값으로 만들었다. 기존 함수를 Node 24에서 실행한 재현은 Issue에 `feature-7`, 그 Issue를 닫는 PR에는 `repo-coordinator`, Projects PR에는 `board-coordinator`를 반환했다. 이는 실제 Windows 세션 시험이 아닌 순수 함수 재현이다.

새 상세 조회는 선택된 PR 하나에만 한정한다. `GetPullRequestDetail(url)`은 설정된 host의 canonical PR URL을 검증하고 기존 Go→WSL→gh 경로로 조회한다. 응답은 요청 URL, 기존 GitHubWork 형식의 상세와 필요한 조회 안내로 제한한다. 정상·캐시된 오래된 근거·최초 조회 실패를 구분하고, 실패해도 기본 snapshot과 Herdr 결과는 보존한다. 설정 변경이나 선택 전환 뒤 늦게 도착한 이전 응답을 현재 항목에 표시하지 않는다.

연결 identity는 host(필요한 port 포함), owner/repository, Issue/PR 종류와 번호를 구분한다. 유효한 대소문자·끝 slash 차이는 정규화하되 다른 host, 임의 본문 링크, repository 이름만으로 기능 연결을 추측하지 않는다. 반환된 여러 관련 Issue의 명시적인 locator를 보존하고 동일 binding만 중복 제거한다. 정확한 연결이 없을 때 기존 Project→repository Coordinator 안내를 사용한다. 반환된 cross-repository 참조는 보존하지만 GitHub/GHES가 모든 의미상의 관련 Issue를 발견한다고 보장하지 않는다.

Herdr 요약과 작업 정보 복사는 동일하게 해결된 선택 항목을 사용한다. 복사 결과는 근거 링크·실행 위치 안내이며, 실제로 수집하지 않은 현재 결정이나 handoff 본문을 만들어 넣지 않는다.

## 계획 검토와 작업 분리

독립 native `followup_plan_review`는 로드맵에 조건부 ACCEPT를 반환했다. 작성자는 Projects-only 상세 조회 계약을 구체화하고, 초기 CI의 변경 경로별 분기를 제거했다. 구체화한 계약만 재검토한 결과는 ACCEPT였으며 구현 착수가 가능하다고 판정했다. 추가 주의였던 SettingsShell→App의 설정 변경 전달은 같은 URL에서도 이전 WSL 환경 응답을 버리는 수용 조건에 포함했다. 중립 판정이 필요한 이견은 없었다.

CI와 Monitor는 같은 기준 commit에서 독립 worktree로 분리했다. CI worker는 workflow·Makefile·구조 검사만, PR 연결 worker는 Monitor Go·React·관련 focused test만 소유한다. 공유 모듈과 dependency lockfile은 변경하지 않는다. 제품 구현과 테스트는 지정 Luna native child, 독립 검토는 Sol native child를 요청했다. 실제 runtime model identity는 unverified다.

검증은 worker의 focused RED/GREEN 뒤 통합 후보의 `make check`, 실제 렌더 이미지 열람과 변경 경로 조작, 독립 후보 리뷰, GitHub PR의 실제 check 확인으로 이어진다. 같은 SHA·명령·환경의 근거는 재사용하며 테스트 삭제나 gate 하향으로 성공을 만들지 않는다.

## 구현·검증 결과

CI Task는 `.github/workflows/ci.yml` 하나에서 PR과 main push에 Node 24·go.mod의 Go 버전을 준비하고, `npm ci` 뒤 기존 `make check`를 실행한다. Actions는 확인된 공식 commit SHA로 고정했다. `scripts/check-template.py`는 실제 root 역할 TOML, template 역할 쌍과 canonical/legacy Skill의 이름·frontmatter·로컬 참조를 검사한다. OpenCode YAML 전체를 재구현하지 않고 현재 사용하는 top-level scalar와 `permission`의 평면 map 형식만 검사한다.

독립 `ci_review`가 필수 root 역할 누락과 잘못된 permission 들여쓰기의 false-green을 발견했다. Luna가 수정한 `ff99a9b7a0ed009b73aacb6290b745d315b07a3a`에 대해 마지막 scoped review는 ACCEPT다. 정상 입력, 역할 누락, 잘못된 top-level 아래 들여쓰기, 지나치게 깊은 자식과 탭 들여쓰기의 focused positive/negative 검증을 확인했다. 통합 branch의 CI 변경은 `3884660`, `ad1a136`, `85ce1e8`로 옮겼다. 실제 GitHub Actions 실행 결과는 게시 후 별도로 확인한다.

PR 연결 초안 `037efc8fc9d14c14362c95e7f83cb6c1f037f0af`의 task-review는 BLOCK이었다. GHES와 github.com 참조가 혼동되는 host 변환, owner/repository 대소문자 동등성 누락, port가 있는 coordinator fallback, 최초 실패의 negative cache를 수정 대상으로 배정했다. 기존 frontend 116개 테스트 통과는 회귀 근거이며 새 연결·비동기 경합의 수용 근거를 대신하지 않는다. 신규 행동 테스트와 선택된 PR의 명시적 재조회 경로도 같은 구현자에게 배정했다. 구현 전 RED 로그를 보존하지 않은 초안은 TDD 검증 완료로 기록하지 않는다.

화면 검증 환경은 현재 차단돼 있다. 공식 Chrome 151.0.7922.34 archive의 크기·체크섬과 실행 파일 버전은 확인했지만 로컬 실행은 socket `Operation not permitted`로 실패했다. 필요한 실행 권한 확대 요청은 환경의 `sandbox_approval: false` 정책으로 자동 거절됐다. 별도 cloud browser도 실행 중인 로컬 Vite URL에 `ERR_CONNECTION_REFUSED`를 반환했다. 이미지 열람·실제 UI 조작, Windows Wails·WSL·Herdr 운영 성공을 주장하지 않는다. 코드·단위 테스트·빌드는 이 누락과 별도로 기록한다.

첫 수정 `2f5f2aca62bd55ab74e588f749931eaf985becd7`은 case/port identity, GitHub 응답 host 분리, negative cache와 singleflight, 선택/설정 응답 폐기, 요약·복사의 같은 관계 사용을 focused tests로 확인했다. Go 및 race 검사, frontend focused 3 files/35 tests와 기존 회귀 14 files/122 tests, build가 통과했다. scoped review는 GitHub 출력의 host 보존 뒤에도 Herdr parser가 github.com만 허용하는 잔여 P1을 발견해 BLOCK했다. 실제 hosted Herdr 입력부터 출력까지의 검증과 오래된 상세가 새 snapshot을 덮지 않는 갱신, 설정 저장과 수동 refresh가 겹칠 때의 복구를 보완한다.

수동 새로고침은 기존 60초 TTL 안에서 상세를 다시 읽는 의미다. 최초 실패도 TTL 동안 재사용하며, 즉시 캐시를 우회하는 새 API는 추가하지 않는다. 독립 리뷰의 조건부 cache-bypass 제안은 이 계약과 [운영 안내](../operator/github-first-quickstart.md)의 제한 명시로 해결했다.

최종 PR 연결 Task 후보는 `95a3ae3b38ed6c71de648bb64b753ac49a639916`다. `pr_link_review`의 마지막 scoped code verdict는 ACCEPT이며 blocking finding은 없다. 실제 hosted Herdr 입력에서 configured host를 보존하고 다른 host를 거절했다. 새 GitHub 관찰에 맞춘 상세 갱신과 설정/수동 refresh 경합도 보완했다. 수정 전 두 경로의 RED 로그를 보존했고, 수정 후 `go test ./monitor -count=1`, 관련 3개 case의 `-race`, frontend 4 files/39 tests가 통과했다. 이 수정에서는 frontend 전체와 build를 반복하지 않았다.

CI와 Monitor를 통합한 코드 후보는 `24892a180b8048dac15e5ed7033704b258664597`이며 Monitor subtree가 Task 후보와 동일함을 확인했다. 운영 안내와 이 기록을 포함한 `f33c5c93c0f0732b51e0ea1365dd4538e9d61b4b`에서 root의 마지막 `make check`가 exit 0으로 완료됐다. Linux, Go 1.27.0, Node 24.19.0, npm 11.9.0 환경에서 구조 검사·gofmt·go vet, Go 3 packages, frontend 14 files/124 tests와 TypeScript/Vite build가 통과했다. dependency/lockfile 변경은 없다. 이후 변경은 이 결과의 문서 기록뿐이며 제품 gate는 반복하지 않았다.

최종 독립 통합 검토 판정, 실제 GitHub Actions 결과와 최신 게시 SHA는 PR에 기록한다. 시각 검증이 남아 있어 PR은 draft로 유지한다. 이후 운영 검증에서는 실제 앱에서 PR 선택·설정 전환·복사와 GHES/Herdr 연결을 확인하고, #43의 두 기능 병렬 작업·재개 시험으로 이어간다.
