# ThreadDock Agent Orchestration Design

Date: 2026-09-11
Status: proposed after user-approved direction; written review required before implementation

## 1. 목적

ThreadDock을 또 하나의 실행 엔진으로 만들지 않으면서, 사용자가 여러 개발 프로젝트를 동시에 운영할 때 GitHub 업무와 Herdr 실행 상태를 자연스럽게 연결한다.

고정 Agent 종류는 최소화하고, 세부 역할은 Skill과 native subagent로 제어한다. 사용자는 Agent 종류를 기억하거나 매번 긴 역할 프롬프트를 작성하지 않아도 되어야 한다.

고정 경계는 다음과 같다.

- GitHub Issue / PR / Projects: durable 업무 원본
- Herdr: top-level session, pane, worktree, Agent lifecycle
- native subagent: 계획, 구현, 독립 리뷰 등 feature 내부 작업
- ThreadDock Monitor: GitHub 업무와 top-level Herdr 상태를 한 화면에 연결
- `~/.threaddock/sessions.json`: 여러 프로젝트가 공유하는 로컬 locator

ThreadDock은 scheduler, lease/recovery engine, 자체 task database, 별도 execution protocol을 소유하지 않는다.

## 2. 고정 Agent는 2종만 둔다

### 2.1 Coordinator

프로젝트 단위의 장기 실행 Agent다. 한 Coordinator가 모든 회사 프로젝트를 한 컨텍스트에서 관리하지 않는다. 프로젝트/repository별 Coordinator를 두고 ThreadDock Monitor가 여러 Coordinator를 한 화면에 모은다.

책임:

- GitHub의 열린 Issue, PR, Projects 상태를 업무 기준으로 읽는다.
- 사용자 요청을 독립 feature로 나누고 의존성을 판단한다.
- 이미 살아 있는 feature session을 재사용하거나 새 Herdr top-level feature session을 연다.
- feature별 GitHub Issue와 Herdr locator를 `~/.threaddock/sessions.json`에 exact upsert한다.
- 여러 feature가 같은 shared interface/file을 수정해야 하면 해당 경계만 직렬화한다.
- blocker, 주요 결정, PR, review 결과를 모아 사용자에게 보고한다.
- 종료 조건을 만족한 locator만 정리한다.

Coordinator는 제품 코드를 직접 구현하는 기본 경로가 아니다. 작은 docs/config 변경처럼 feature session을 만드는 비용이 더 큰 작업은 예외적으로 직접 처리할 수 있지만, 구현과 독립 리뷰의 역할을 한 Agent가 동시에 수행했다고 표시하면 안 된다.

기본 모델 역할은 Sol medium 수준의 계획/조정/검토 능력을 전제로 한다. 실제 설치 환경이 해당 모델 이름을 지원하지 않으면 임의 대체를 성공으로 보고하지 않는다.

### 2.2 Feature Leader

하나의 Issue 또는 독립 feature를 처음부터 PR 준비까지 책임지는 Herdr top-level Agent다.

책임:

- Issue와 acceptance criteria를 읽고 feature scope를 고정한다.
- 필요할 때만 상세 planning subagent를 사용한다.
- 독립 구현 task를 찾아 native implementation subagent에 병렬 분배한다.
- 결과를 feature worktree에 통합한다.
- focused 검증을 수행한다.
- 구현자와 분리된 fresh reviewer subagent의 검토를 받는다.
- blocking review를 수정하고 재검토한다.
- 최종 PR, Issue handoff, Project, Wiki 필요사항을 정리한다.

Feature Leader 내부의 native subagent는 ThreadDock locator에 기록하지 않는다. Monitor에는 Coordinator와 Feature Leader만 top-level 실행으로 표시한다.

기본 역할은 Sol medium 수준의 계획/통합을 전제로 하고, 실제 코드 구현 subagent는 Luna high 같은 구현 중심 모델을 우선한다.

## 3. transient subagent 역할

다음 역할은 별도의 상주 Agent 종류나 Herdr top-level session으로 만들지 않는다.

### Planner

요구사항이 모호하거나 여러 컴포넌트/인터페이스를 건드리는 경우에만 호출한다. `grill-me`처럼 가정, edge case, 완료 조건, 데이터/상태 전이를 집요하게 확인한 뒤 구현 가능한 계획으로 반환한다.

단순 버그, 작은 UI 수정, 명확한 acceptance criteria에는 호출하지 않는다.

### Implementer

Feature Leader가 나눈 작은 task를 구현한다. test seam이 있는 제품 동작은 TDD를 기본으로 하고, 독립 task는 병렬 실행할 수 있다. 구현 subagent가 다른 implementer를 다시 spawn하지 않는다.

### Reviewer

구현 대화와 분리된 fresh context에서 고정 SHA/diff와 acceptance criteria를 검토한다. 직접 수정하지 않고 accept/block과 구체적인 finding만 반환한다.

### Publish / Docs helper

PR 설명, Issue handoff, Project field, Wiki/운영 문서처럼 외부 기록이 많을 때만 필요에 따라 호출한다. 이를 PR Agent, Wiki Agent, Project Agent로 각각 고정하지 않는다.

## 4. Skill 구성

고정 Skill은 약 6개로 제한한다.

### `coordinate-work`

사용자: Coordinator

기존 `plan-work`의 coordinator 부분과 `open-agent-session`을 통합한다.

- GitHub 업무/Project 읽기
- feature 분해와 dependency 판단
- 기존 Herdr session 재사용 / 새 feature session 시작
- feature leader 초기 packet 작성
- shared locator exact upsert
- 프로젝트 간 unrelated binding 보존
- 동시 locator 변경 lock 규칙

### `develop-feature`

사용자: Feature Leader

feature 전체 lifecycle을 지휘하는 상위 Skill이다.

1. Issue/acceptance 읽기
2. 범위/의존성 확인
3. 필요 시 `grill-plan`
4. 독립 task 분해
5. native implementation subagent 병렬 분배
6. 결과 통합
7. focused verification
8. fresh reviewer에 `review-change` 요청
9. blocking finding 수정 및 재검토
10. `publish-work`

이 Skill이 SDD(subagent-driven development)의 기본 orchestration entrypoint가 된다.

### `grill-plan`

사용자: Planner subagent 또는 Feature Leader

- 숨은 가정과 모호한 요구 찾기
- edge case / failure mode 질문
- acceptance criteria 보강
- interface와 dependency 고정
- 병렬 가능한 task와 직렬 경계 도출

결과는 구현 계획이지 별도 orchestration state가 아니다.

### `tdd-task`

사용자: implementation subagent

기존 `implement-task`를 더 명확한 leaf 역할로 정리한다.

- 할당 task만 수정
- applicable test seam에서는 red -> green -> refactor
- docs/config-only는 적절한 parser/check 사용
- focused test만 실행
- changed files / command / outcome / blocker 반환
- public/shared interface 변경 필요 시 Feature Leader로 되돌림

worker별 full suite는 금지한다.

### `review-change`

사용자: Reviewer subagent

현재 Skill을 유지하되 Feature Leader orchestration에 맞춘다.

- exact SHA/diff 검토
- acceptance별 evidence 확인
- scope regression, error handling, test evidence 확인
- accept 또는 block
- 직접 코드 수정/merge 금지

### `publish-work`

사용자: Feature Leader 또는 Coordinator

기존 `record-work`를 통합한다.

- PR create/update
- Issue handoff / decision / blocker
- GitHub Project status/priority/waiting
- Wiki 또는 운영문서 갱신이 필요한 경우에만 반영
- shared ThreadDock locator maintenance / cleanup

PR, Wiki, Project 각각을 별도 Agent 종류로 만들지 않는다.

## 5. 기존 Skill migration

현재 프로젝트 템플릿의 Skill은 다음처럼 재편한다.

| 현재 | 새 구조 |
| --- | --- |
| `plan-work` | coordinator 부분 -> `coordinate-work`, feature 상세계획 -> `develop-feature` / `grill-plan` |
| `open-agent-session` | `coordinate-work`에 흡수 |
| `implement-task` | `tdd-task`로 대체 |
| `review-change` | 유지/정리 |
| `record-work` | `publish-work`로 대체 |

기존 이름은 한동안 compatibility shim으로 둘 수 있지만, 문서와 새 설치에서는 새 Skill 이름만 가르친다. shim은 중복 구현이 아니라 새 Skill을 가리키는 짧은 migration 안내로 제한한다.

## 6. 사용자 진입점

사용자가 긴 orchestration 프롬프트를 외우지 않게 한다.

### Coordinator 시작

목표 형태:

```text
ThreadDock coordinator로 이 프로젝트를 맡아줘.
GitHub Issue/Project를 업무 원본으로 사용하고 coordinate-work로 현재 열린 일을 정리해.
```

Coordinator agent definition 자체가 GitHub/Herdr/ThreadDock 경계를 알고 있으므로 사용자가 model 역할, session locator schema, review workflow를 반복 설명하지 않는다.

### Feature Leader 시작

Coordinator가 자동으로 다음 최소 packet만 전달한다.

- Issue URL
- repository / target branch
- feature 목적 / acceptance
- feature worktree
- 선행 dependency / shared interface 주의사항

Feature Leader는 `develop-feature`를 entrypoint로 실행한다.

## 7. 여러 프로젝트 사용

사용자는 여러 repository/project를 동시에 작업할 수 있다.

- 각 프로젝트는 자기 Coordinator를 가진다.
- feature마다 별도 top-level Herdr Feature Leader를 가진다.
- 모든 Coordinator/Feature Leader locator는 하나의 `~/.threaddock/sessions.json`에 저장한다.
- 각 writer는 user-level lock을 얻은 후 locator를 다시 읽고 exact binding만 upsert/cleanup한다.
- 다른 repository/project binding을 추측하거나 prune하지 않는다.
- Monitor는 전역 locator와 설정된 GitHub sources를 결합해 여러 프로젝트를 한 화면에 보여준다.

이 구조는 한 Coordinator가 여러 codebase의 전체 문맥을 오래 들고 있는 것보다 context drift와 잘못된 cross-project 수정 위험을 줄인다.

## 8. 세션과 완료 lifecycle

Feature lifecycle:

```text
GitHub Issue
  -> Coordinator selects feature
  -> Herdr Feature Leader session
  -> develop-feature
       -> optional grill-plan
       -> tdd-task subagents
       -> integrate
       -> review-change subagent
       -> publish-work
  -> PR / handoff ready
  -> human merge
  -> session can end
  -> safe locator cleanup when completion conditions hold
```

Agent의 `idle` 또는 `done`은 GitHub 업무 완료가 아니다.

Feature locator 삭제 조건은 모두 만족해야 한다.

1. Issue closed
2. 관련 PR 작업 finished (merged 또는 명시적 종료 + 남은 handoff 없음)
3. Herdr top-level feature session absent

Coordinator locator는 해당 프로젝트를 더 이상 ThreadDock으로 관리하지 않을 때만 제거한다.

## 9. 병렬성과 검증

- Feature Leader가 독립 task를 찾은 경우에만 implementation subagent를 병렬화한다.
- 기본 구현 worker 목표 상한은 2~3개이며 항상 채우지 않는다.
- shared interface / dependency 변경은 single owner 또는 선행 task로 직렬화한다.
- worker는 focused test만 수행한다.
- Feature Leader 통합 후 affected verification을 수행한다.
- fresh reviewer는 테스트를 무조건 재실행하지 않고 exact SHA evidence를 검토한다.
- 전체 suite는 최종 통합 gate에서 필요한 경우 한 번 실행한다.

## 10. ThreadDock Monitor에 보여줄 것

Monitor는 사용자가 판단할 수 있는 factual state만 보여준다.

표시:

- GitHub Issue / PR 실제 상태
- GitHub Project field
- Coordinator / Feature Leader top-level session 상태
- session / pane / cwd locator
- blocker / handoff / PR / checks / review evidence

표시하지 않음:

- 내부 planner/implementer/reviewer subagent를 top-level session처럼 표시
- generic wire state를 실제 workflow 단계처럼 번역
- `done`을 Issue 완료로 추정
- ThreadDock이 자체 scheduler처럼 task 상태를 발명

## 11. 구현 범위

첫 구현은 다음으로 제한한다.

1. project-template에 Coordinator와 Feature Leader용 Agent 정의 추가
2. 6개 Skill 구조 도입
3. 기존 5개 Skill migration/shim 정리
4. quickstart를 짧은 coordinator 시작 흐름으로 수정
5. 전역 sessions locator 계약 유지
6. 필요 최소한의 테스트/정적 검증 추가

처음 구현에서는 별도 installer, marketplace, 자동 모델 선택기, 별도 daemon, scheduler, queue, lease/recovery engine을 만들지 않는다.

## 12. 성공 기준

- 사용자가 프로젝트에서 Coordinator를 한 문장 수준으로 시작할 수 있다.
- Coordinator가 Issue를 feature session에 분배하고 전역 locator를 기록한다.
- Feature Leader가 `develop-feature` 하나로 planning -> implementation -> review -> publish 흐름을 수행한다.
- 구현/review/docs 같은 세부 역할 때문에 Herdr top-level session 수가 폭증하지 않는다.
- 여러 프로젝트의 locator가 하나의 sessions 파일에 안전하게 공존한다.
- ThreadDock Monitor에는 top-level Agent와 factual GitHub/Herdr evidence만 보인다.
- 사용자는 내부 orchestration protocol을 외울 필요가 없다.
