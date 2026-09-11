# ThreadDock 사용성 체크리스트

ThreadDock을 실제 프로젝트에 사용하면서 확인할 항목이다. 기능 완료 체크리스트가 아니라 **사용자가 계속 쓰고 싶은 도구인지** 확인하는 dogfooding 목록으로 유지한다.

## Monitor 기본 사용

- [ ] 앱 첫 화면이 이해하기 쉽고 불필요한 내부 용어가 보이지 않는다.
- [ ] 초기 저장소 조회는 열린 Issue/PR만 가져오며 체감 대기 시간이 과하지 않다.
- [ ] `전체 보기`는 사용자가 요청했을 때만 닫힌 이력을 가져온다.
- [ ] `전체 보기` 상태에서 닫힌 이력을 주기적으로 불필요하게 재조회하지 않는다.
- [ ] 여러 repository/Project가 있어도 프로젝트 선택이 자연스럽다.
- [ ] 긴 Issue/PR 제목이 목록을 과도하게 늘리지 않는다.
- [ ] `조회 상태`가 실제 업무 진행 단계처럼 오해되지 않는다.

## Issue / PR 상세

- [ ] Issue/PR 실제 open/closed/merged 상태가 정확하게 보인다.
- [ ] GitHub Project field가 있을 때 읽기 쉽게 보인다.
- [ ] 제목, 목록, 체크리스트, 강조, 코드, 인용, 링크, 표 등 자주 쓰는 Markdown이 읽기 좋게 보인다.
- [ ] Markdown 안의 위험한 링크나 raw HTML이 임의 실행되지 않는다.
- [ ] 관련 Issue/PR/Wiki 링크를 쉽게 열 수 있다.
- [ ] `작업 정보 복사`가 선택한 업무와 Herdr 위치를 전달하기에 충분하다.

## GitHub Enterprise / 설정

- [ ] GitHub Enterprise Host를 설정하고 앱을 재시작해도 유지된다.
- [ ] 저장소와 GitHub Project root URL 설정이 재시작 후 유지된다.
- [ ] GHES Issue/PR/Project 링크가 github.com으로 잘못 바뀌지 않는다.
- [ ] WSL 배포판과 Herdr 연결 파일 설정 오류가 이해 가능한 문구로 표시된다.

## Herdr / 여러 프로젝트

- [ ] `~/.threaddock/sessions.json` 하나로 여러 프로젝트 binding이 함께 유지된다.
- [ ] 프로젝트별 Coordinator가 정확한 repository/Project에 연결된다.
- [ ] Feature Leader가 정확한 Issue/worktree/session/pane에 연결된다.
- [ ] native subagent는 top-level ThreadDock session처럼 노출되지 않는다.
- [ ] Agent `working / blocked / idle / done` 상태가 GitHub 업무 완료와 혼동되지 않는다.
- [ ] session이 사라졌지만 Issue가 열려 있으면 recovery 정보가 유지된다.
- [ ] `Issue closed + 관련 PR 작업 종료 + top-level session 없음` 조건에서만 feature binding이 정리된다.
- [ ] 여러 Coordinator가 locator를 갱신해도 다른 프로젝트 binding이 유실되지 않는다.

## Agent orchestration

- [ ] 짧은 Coordinator 시작 프롬프트만으로 현재 GitHub 업무를 파악한다.
- [ ] 기존 Feature Leader session이 있으면 중복 생성하지 않고 재사용한다.
- [ ] feature를 필요 이상으로 잘게 나누지 않는다.
- [ ] 독립적인 feature만 별도 Herdr top-level session으로 분리한다.
- [ ] `grill-plan`은 모호하거나 큰 작업에만 사용하고 단순 작업에서는 생략한다.
- [ ] implementation subagent를 무조건 최대 개수까지 생성하지 않는다.
- [ ] 공유 interface/file 변경은 한 owner에게 직렬화한다.
- [ ] 구현자와 분리된 fresh reviewer가 exact SHA/diff를 검토한다.
- [ ] worker마다 full suite를 반복하지 않고 focused 검증을 사용한다.
- [ ] PR/Issue/Project/Wiki 기록은 실제 수행 결과와 일치한다.
- [ ] main 병합은 사용자에게 남는다.

## Project Toolbox

- [ ] 프로젝트마다 Toolbox 내용이 서로 섞이지 않는다.
- [ ] Web 자료 링크를 등록하고 열 수 있다.
- [ ] Windows absolute file path를 등록하고 열 수 있다.
- [ ] WSL absolute file path를 등록하고 Windows 앱에서 열 수 있다.
- [ ] 잘못된 상대경로나 위험한 URL을 등록하지 못한다.
- [ ] 자주 쓰는 `psql`, `docker`, `kubectl`, `uv` 등의 명령어를 저장하고 한 번에 복사할 수 있다.
- [ ] Toolbox의 명령어는 복사만 가능하고 ThreadDock이 임의 실행하지 않는다.
- [ ] 프로젝트별 개인 체크리스트를 추가/완료/삭제할 수 있다.
- [ ] Toolbox 내용이 앱 재시작 후에도 유지된다.
- [ ] Toolbox 자료/명령어/체크리스트가 Coordinator나 Feature Leader에게 자동 주입되지 않는다.
- [ ] 필요할 때 사용자가 자료를 직접 Agent에게 전달하는 흐름이 불편하지 않다.

## 장애 / 복구

- [ ] GitHub 연결 실패 시 마지막 성공 결과와 현재 오류가 구분된다.
- [ ] Herdr 조회 실패가 GitHub 정보 표시까지 막지 않는다.
- [ ] 잘못된 sessions 파일이나 toolbox 파일이 앱 전체를 종료시키지 않는다.
- [ ] 저장 실패 시 성공으로 표시하지 않는다.
- [ ] 앱 종료 후 불필요한 child process가 남지 않는다.

## 메모

발견한 불편은 이 문서를 억지로 세분화하기보다 GitHub Issue로 올리고, 여기에는 반복해서 확인할 사용성 기준만 남긴다.
