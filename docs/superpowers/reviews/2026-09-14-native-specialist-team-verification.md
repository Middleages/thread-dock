# Native 전문가 팀 검증

상태: 정적/Task 검토·로컬 역할 probe·최종 통합 리뷰 완료. main 병합은 사람에게 남긴다.

## 구현과 Task 검토

- 기준 d57beeb5e1218ec39d5053b74e6ae5d1c4b6b5b8, 구현7f5e2a4656695ab9cfe270591526b6a617b6402c.
- 첫 fresh 리뷰는 review-change의 단순성·진단 가능성 보강 누락으로 BLOCK. adf1a585a401d7644e42ca8498dc216865a426c3에서 해당 Skill만 보강했고 scoped ACCEPT.
- 통합 점검에서 tdd-task의 구체 단순성 기준과 Quickstart의 권한 설명을 수정했다. d1b826e04b54978f0b8bf9da1722b72dfa665f6b에서 두 파일 focused validate 후 scoped ACCEPT.
- 통합 source a2b2d914bcc1603d46348a1074d0ea1ebd7200a7. 구현자는 Luna high, task reviewer는 fresh Sol medium 지정; 별도 실제 runtime identity는 unverified.
- 실제 probe에서 원래 edge-case가 잘못 위임된 것을 확인해 develop-feature의 원본 조건/경로/dependency 전달을 복원했다. f4220cfb3d5e248a48b106be39b4db1af75f6a09는 해당 Skill만 변경, focused validate와 scoped ACCEPT. 최종 통합 source는2781c09d763ee325252e4e34ea44388f6acadcf5다.

## 검증 범위

- make template-check, 신규 helper와 수정 Skill quick_validate, TOML/YAML/frontmatter/reference와 Quickstart 링크, git diff --check PASS. 초기 template-check 성공 뒤 구조가 바뀌지 않은 문구 fix에는 그 근거를 재사용했다.
- OpenCode1.18.30 --pure debug config/skill에서 두 primary, 네 subagent, helper2개+tdd-task 실제 경로를 확인했다. leaf taskdeny, explorer/reviewereditdeny, model override 없음, plugin[]. 로그는 /tmp/threaddock-native-team.pJWGKz/resolved-config.json 및 resolved-skills.json.
- Codex name/description/developer_instructions 및 optional model/effort 조건은 실제 TOML parse와 [공식 schema 설명](https://learn.chatgpt.com/docs/agent-configuration/subagents)에 근거한다. 실제 Codex child 모델 호출/정책 적용은 이번 확인 범위가 아니다.
- docs/config-only 변경이라 Go/UI 전체 make check와 Windows/Wails build는 실행하지 않는다. 기존 제품 코드 변경 없음.

## 로컬 동작 probe

원문/결과는 /tmp/threaddock-native-team.pJWGKz에 보존한다. baseline CSV는 이전에 검토된 stdlib fixture를 재사용했고 operator-notes/proposal은 합의한 단순성·로그·문서 의미 판단을 위한 가상 자료다. 실제 DB BU 코드나 운영 배포를 검증한 것이 아니다.

설정은 독립 XDG config/data/cache/state, model opencode/big-pickle, plugin[], --pure이며 새 전역 설치는 없다. 탐색/문서 편집/CSV 구현/제안 검토를 실제 named task로 요청한다. 이 네 단계는 시험용이며 제품의 필수 pipeline이 아니다.

- 초기 CLI 호출은 variadic --file 위치 때문에 message를 파일명으로 해석해 실패(exit1, 모델 이벤트0).
- 두 번째는 작업 폴더 밖 첨부 경로의 external_directory 요청이 headless에서 거부됐다. read 이벤트만 있었고 task0, CLI exit0이었다. 역할 성공으로 세지 않는다.
- 세 번째는 요청을 worktree 내부 .probe-request.md로 옮겼다. source d1b826e, fixture ba5595ba06a5ae5beb40f8ede84426ce12eeb0b9, parent ses_f5fd52e41ffeKMrjWiNGCWwPkG. probe-r3.jsonl/stderr에 실제 결과를 남긴다.

### 첫 실제 probe 결과와 발견한 실패

| 역할 | 실제 child ID | 관찰 |
|---|---|---|
| td_explorer | ses_f5fd4a472ffe3p8KHaCgllKf0y | CLI 진입점/행 검증·출력 경로와 교체할 기존 테스트를 찾고 미확정 의미를 구분 |
| td_docs_editor | ses_f5fd3b98effemQgNh5EbRbGWaC | operator-notes.md만 편집. 명령·release/build 값·공용 대상·로그 예·실행 미검증 사실 보존 |
| td_implementer | ses_f5fd19fbfffe265d6olBJMM2th | 두 소유 파일만 commit했으나 원래 no-match 요구와 다른 구현/테스트를 만들어 이 probe는 FAIL |
| td_reviewer | ses_f5fcf1b06ffem3geIGQ8g9s7zl | 제안 문서의 정보용 버전 차단·공용값 재입력·진단 필드 제거를 source notes와 대조해 block. 문서/코드 수정 없음 |

모든 호출은 실제 named native task였고 metadata는 opencode/big-pickle이었다. 새 helper/기존 tdd-task/review-change를 가리키는 역할로 수행했다. Explorer/문서 편집/제안 검토는 확인됐지만 첫 구현 결과는 성공으로 세지 않는다.

실패 원인: 원래 요청은 no-match에도 header-only 출력인데, parent가 child packet을 “matching row가 있을 때만 header, no-match는 empty stdout”으로 재작성했다. 구현자와 그 테스트가 함께 변경된 조건을 따라 7 tests OK를 보고했다. root의 `python3 csv_filter.py --status nonexistent < sample.csv | wc -c` 확인은0바이트였다. 실패 후보c0640a0b9aacab27685f7ab04d82d88610c65f67은 별도 fixture에 보존했고 원래 요청을 맞춘 것으로 표기하지 않는다.

제품 template의 develop-feature에서 기존 bounded paths/acceptance/dependency 입력 전달이 fallback 설명으로 대체되어 약해진 부분도 복원했다. 새 자동 검증 gate 대신 원본 Issue/request link와 조건, 특히 edge-case 출력·오류 의미를 그대로 전달하는 문장으로 수정했다.

### 원문 보존을 포함한 focused 코드 재확인

- 새 worktree `/tmp/threaddock-native-team.pJWGKz/code-recheck`, source f4220cf, fixture baseline00a9a0e3dc2e887c85a153f0371f249cd5d39a56.
- 원래 의미를 바꾸지 않고 “zero matches → header line alone, NOT empty stdout”을 명확히 적은 requirements.md를 원본으로 삼았다. instruction 수정과 fixture 문구 명료화가 함께 있었으므로 개선 효과를 Skill 변경 하나의 인과로 단정하지 않는다.
- parent `ses_f5fc9e792ffeSYRfWd0E56OnwO`가 실제 td_implementer `ses_f5fc85e9cffe7O7fdUACz7KQsx`에 원본 파일/조건을 전달했다.
- 구현 candidate `0ec22d3d9b9bbb5a34a6b7ee779ecb56480c9eb8`, owned csv_filter.py/test_csv_filter.py만 변경. focused8tests PASS. no-match·header-only 입력(with/without filter)에서 `id,title,status\n`, missing header는exit2/stderr/partial stdout없음을 실제 명령으로 확인했다.
- fresh td_reviewer `ses_f5fc3595fffej3Dx59zQyx73HM`가 원본 requirements.md와 exact candidate를 대조해 accept를 반환했다. 새 모듈·의존성·설정이 없고 추가 len(row) 방어는 새 인덱스 접근의 실제 crash 위험과 관련된 것으로 평가했다.
- CLI exit0 및 parent 최종 보고까지 회수했다. 로그 code-recheck.jsonl/stderr, 원문과 출력은 같은 temp root에 보존한다. root는 전체 fixture suite를 추가 재실행하지 않았다.
- 코드 탐색/문서 편집 Skill과 wrapper는 재확인 전후 불변이므로 해당 역할 전체 probe는 반복하지 않았다. 실제 새 helper 이름/권한/Skill 참조는 정적 인식과 최초 named 호출 근거를 함께 사용한다.

### 해석과 한계

이 결과는 짧은 역할 지침과 공용 Skill이 실제 native subagent에서 사용 가능하며, 소유 경로·문서 의미·단순성/로그 판단과 올바른 원문 전달을 확인한 제한 사례다. 프롬프트가 모든 모델/업무에서 올바른 결과를 보장한다는 뜻은 아니다. 테스트가 통과해도 원래 요구와 다른 테스트면 완료가 아니라는 실패를 보존한다.

모델 실행에는 일부 중복된 focused 검증과 과장된 부연도 있어, 최소 작업량이나 항상 정확한 문장을 보증하지 않는다. 조건·검증 근거를 비교하는 리뷰를 유지한다. 코드/설치 형식 검증과 실제 역할 행동, metadata상의 모델명과 실제 backend identity도 구분한다.

이번 변경의 live GitHub/Herdr 제품 E2E, Codex 실제 child 실행, Windows/Wails, optional LSP/Playwright/AST 도구는 미검증이다. 전역 설치·사용자 모델 설정 변경·OMO 사용은 없고, 현재 root 세션의 Agent registry가 자동 교체됐다고 주장하지 않는다.

## 최종 통합 검토

fresh native_team_final_review는0bf44611557a3ea8dc0dcd1edc52e35e7593c0f2에서 ACCEPT, blocking/non-blocking finding 없음으로 판정했다. source diff22파일의 범위, 두 harness 정의, 짧은 shared Skill, 원문 전달, 실패/재확인 구분과 실제 native metadata를 읽었다. 기존 checks/model 실행을 재실행하지 않았다.

같은 후보에서 root의 최종 make template-check와 TOML/YAML의 paired6역할/권한/새leaf model미지정/Skill참조 확인도 통과했다. 이후 변경은 검토·게시 metadata뿐이다. 이 승인은 제한된 구현의 코드/문서 검토이며 모든 모델과 실서비스에서의 완전한 행동 보장이 아니다.
