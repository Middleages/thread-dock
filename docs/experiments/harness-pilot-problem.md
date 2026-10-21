# 실험 문제: 프로젝트별 작업 현황 도구

이 문서는 개발 참가자에게 전달할 **최초 요구사항 R1**이다. 실행 위치·Issue·허용 범위는 진행자가 별도로 제공한다. 이 문제를 읽는 것만으로 실험을 시작하지 않는다.

## 사용자 요청

여러 프로젝트의 작업을 JSON 파일로 관리하고 있다. 필요한 작업을 찾고, 프로젝트별 진행 상황을 빠르게 확인할 작은 명령줄 도구 `tasklens`를 만들어 줘.

**A: 작업 목록 조회**, **B: 진행 상황 집계**를 독립 기능으로 나눠 병렬로 개발해 줘. 각 기능에는 별도 Issue, 작업 branch/worktree, Feature Leader가 있어야 한다. 공통 데이터 규칙과 명령 진입점은 먼저 합의하고 한 구현자가 소유해 줘. 구현 방법과 테스트 내부 구성은 담당자가 정하면 된다.

Python 3.11 이상과 표준 라이브러리만 사용한다. 웹 화면, DB, 인증, 설치 패키지, 배포 서버는 필요 없다. 입력 파일을 수정하거나 작업 상태를 변경하는 기능은 범위 밖이다.

## 공통 입력과 규칙

입력은 다음 네 필드를 가진 객체의 JSON 배열이다. 네 필드는 비어 있지 않은 문자열이고, `id`는 파일 전체에서 중복될 수 없다. 필드 값의 대소문자를 바꾸거나 공백을 자동 제거하지 않는다. 추가 필드가 있어도 입력 오류로 취급하지 않으며, 목록 출력은 아래 네 필드만 사용한다.

- `id`: 작업 식별자
- `title`: 작업 제목
- `project`: 프로젝트 이름
- `status`: `todo`, `doing`, `done` 중 하나

R1에서 **진행 중인 작업(active)**은 `todo` 또는 `doing`이다. `done`만 완료 작업이다. 프로젝트 이름은 정확히 일치하는 값으로 필터링한다.

파일 없음, 잘못된 JSON, 배열이 아닌 입력, 잘못된 항목·필드·상태, 중복 ID는 명확한 오류다. 파일 전체를 검증한 뒤 필터링한다. 오류가 있으면 정상 결과를 일부 출력하지 않고, stdout은 비우며 stderr에 원인을 설명하고 종료 코드 1을 반환한다. 내부 traceback을 사용자 오류 안내로 사용하지 않는다.

정상 실행은 종료 코드 0, stdout에 JSON 결과를 출력한다. JSON의 공백·객체 키 순서는 판정 대상이 아니다. 빈 배열은 정상 입력이다.

`data/sample.json`의 최초 데이터:

```json
[
  {"id":"t1","title":"백업 확인","project":"home","status":"todo"},
  {"id":"t2","title":"로그 정리","project":"home","status":"doing"},
  {"id":"t3","title":"인증서 확인","project":"home","status":"done"},
  {"id":"t4","title":"문서 작성","project":"lab","status":"todo"},
  {"id":"t5","title":"테스트 정리","project":"lab","status":"done"}
]
```

## 기능 A — 목록 조회

```bash
python -m tasklens list --input data/sample.json
python -m tasklens list --input data/sample.json --project home --active
```

- `--project`와 `--active`는 선택 사항이며 함께 쓰면 두 조건을 모두 만족해야 한다.
- 입력 순서를 유지한 작업 객체 배열을 출력한다. 네 필드 값은 입력 그대로 보존한다.
- 일치하는 프로젝트나 작업이 없으면 `[]`를 출력한다.

| 입력/옵션 | 예상 출력의 ID 순서 |
|---|---|
| sample, 옵션 없음 | `t1, t2, t3, t4, t5` |
| sample, `--active` | `t1, t2, t4` |
| sample, `--project home --active` | `t1, t2` |
| sample, `--project missing` | 빈 배열 |
| 빈 입력 배열 | 빈 배열 |

## 기능 B — 진행 상황 집계

```bash
python -m tasklens summary --input data/sample.json
python -m tasklens summary --input data/sample.json --project home
```

- `--project`는 선택 사항이다. 해당 범위의 `total`, `active`, `done` 개수를 정수로 출력한다.
- 존재하지 않는 프로젝트와 빈 배열은 세 값 모두 0이다.

| 입력/옵션 | 예상 JSON |
|---|---|
| sample, 옵션 없음 | `{"total":5,"active":3,"done":2}` |
| sample, `--project home` | `{"total":3,"active":2,"done":1}` |
| sample, `--project lab` | `{"total":2,"active":1,"done":1}` |
| 빈 입력 배열 | `{"total":0,"active":0,"done":0}` |

## 전달할 결과

- A와 B 각각 사용 가능한 명령, focused 테스트, 실제 후보 SHA의 독립 리뷰, 관련 Issue에 연결된 PR.
- 공통 상태 판정 규칙을 중복 구현하지 않는다. 공용 파일의 작성자는 하나로 유지한다.
- 각 Issue에서 현재 결정·진행 상황·남은 일과 주 검증 기록을 찾을 수 있어야 한다. PR이 있으면 상세 검증은 PR에 두고 Issue에서 링크한다.
- `python -m unittest discover -s tests -v`로 전체 검증할 수 있어야 한다. 각 worker는 자기 영향 범위를 검사하고, 최종 통합 후보에서 전체 검사를 한 번 수행한다.
- 사용법과 두 기능의 예시는 짧은 README에 담는다. 입력 파일을 수정하지 않는다는 조건도 확인한다.
- 첫 구현 후보와 handoff를 준비하면 진행자에게 알린다. 이번 실험에는 이후 요구 변경·인계 단계가 있으므로 그 전에는 전체 실험 완료나 main 병합으로 처리하지 않는다.
