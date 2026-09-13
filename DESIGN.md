---
{
  "name": "ThreadDock Monitor",
  "description": "GitHub 업무와 Herdr 실행 상태를 읽고 사람용 자료와 복사 전용 도구를 다루는 compact desktop monitor",
  "colors": {
    "cobalt-950": "#172136",
    "cobalt-600": "#365bc8",
    "cobalt-100": "#dde3ef",
    "cloud-50": "#fbfcff",
    "cloud-100": "#edf0f6",
    "slate-600": "#596379",
    "slate-300": "#b5bdcc",
    "amber-700": "#714900",
    "green-700": "#257151",
    "white": "#ffffff"
  },
  "typography": {
    "display": {
      "fontFamily": "Segoe UI Variable, Pretendard, Noto Sans KR, sans-serif",
      "fontSize": "1.9375rem",
      "lineHeight": 1.08,
      "letterSpacing": "-0.04em"
    },
    "title": {
      "fontFamily": "Segoe UI Variable, Pretendard, Noto Sans KR, sans-serif",
      "fontSize": "1.125rem",
      "lineHeight": 1.25,
      "letterSpacing": "-0.03em"
    },
    "body": {
      "fontFamily": "Segoe UI Variable, Pretendard, Noto Sans KR, sans-serif",
      "fontSize": "0.875rem",
      "lineHeight": 1.55
    },
    "label": {
      "fontFamily": "Segoe UI Variable, Pretendard, Noto Sans KR, sans-serif",
      "fontSize": "0.6875rem"
    }
  },
  "rounded": {
    "control": "0.375rem"
  },
  "components": {
    "button-primary": {
      "backgroundColor": "{colors.cobalt-950}",
      "textColor": "{colors.cloud-50}",
      "rounded": "{rounded.control}",
      "height": "43px"
    },
    "button-secondary": {
      "backgroundColor": "{colors.cloud-50}",
      "textColor": "{colors.cobalt-950}",
      "rounded": "{rounded.control}",
      "height": "43px"
    },
    "input": {
      "backgroundColor": "{colors.cloud-50}",
      "textColor": "{colors.cobalt-950}",
      "rounded": "{rounded.control}",
      "padding": "8px 10px"
    },
    "toolbox-drawer": {
      "backgroundColor": "{colors.cloud-50}",
      "textColor": "{colors.cobalt-950}",
      "width": "480px"
    }
  }
}
---

# Design System: ThreadDock Monitor

## Overview

**Creative North Star: "Operate"**

ThreadDock Monitor는 장식보다 현재 업무, 관찰 상태, 다음 조작을 또렷하게 드러내는 운영 화면이다. 차분한 cobalt·slate·cloud 계열과 조밀한 목록, 얇은 구분선으로 GitHub 원본과 Herdr 관찰을 한 흐름에서 읽게 한다. Toolbox는 Agent 자동화가 아니라 사람이 자료를 열고 명령을 복사하는 보조 표면이다.

고정 좌우 rail 없이 상단 bar와 최대 폭 본문을 사용하고, 전역 Toolbox만 우측 overlay로 잠시 겹친다. 이 문서는 완성된 Linux fixture UI와 소스에서 추출했으며 Windows native 렌더링은 아직 검증되지 않았다.

**Key Characteristics:**

- 정보 밀도가 높지만 선과 여백으로 묶음을 구분한다.
- 상태색은 의미 전달에만 쓰고 본문은 짙은 cobalt와 slate로 유지한다.
- 명령은 복사 전용이며 실행 affordance를 만들지 않는다.

## Colors

Cobalt는 본문과 행동, cloud는 표면, slate는 보조 정보와 경계, amber와 green은 상태에 한정한다.

### Primary

- **Deep Cobalt:** 주요 텍스트, primary action, 강한 구분선에 사용한다.
- **Action Cobalt:** 링크, 선택 탭, 진행 상태와 focus ring에 사용한다.
- **Pale Cobalt:** 선택 영역과 skeleton 같은 낮은 강조에 사용한다.

### Neutral

- **Cloud Surface:** 기본 본문과 dialog·drawer 표면이다.
- **Cloud Canvas:** hover, 입력 보조 영역, 상태 안내 배경이다.
- **Slate Copy:** 설명, 시각, 보조 상태에 사용한다.
- **Slate Border:** 표, 입력, 행과 overlay의 얇은 경계다.

### Tertiary

- **Attention Amber:** 오류, 차단, 확인이 필요한 상태다.
- **Success Green:** 정상 연결과 성공 상태다.

**The Semantic Status Rule.** Amber와 green은 장식용 accent가 아니라 실제 상태 의미에만 사용한다.

## Typography

**Display Font:** Segoe UI Variable, Pretendard, Noto Sans KR, sans-serif
**Body Font:** Segoe UI Variable, Pretendard, Noto Sans KR, sans-serif
**Label/Mono Font:** ui-monospace, SFMono-Regular, Consolas, monospace는 명령 내용에만 사용한다.

운영 화면에 맞는 작은 본문과 강한 숫자·상태 label을 사용한다. 큰 제목도 같은 UI family를 유지해 별도 브랜드 서체를 추가하지 않는다.

### Hierarchy

- **Display:** 화면의 최상위 작업 현황 제목에 사용한다.
- **Title:** 프로젝트와 주요 section 제목에 사용한다.
- **Body:** 표, 상세 내용, 안내 문구의 기본 크기다.
- **Label:** 개수, 시각, 필드 설명처럼 보조적인 메타데이터에 사용한다.

## Layout

상단 bar와 본문은 모두 `min(1440px, 100%)` 안에서 정렬된다. 본문은 넓은 화면에서 유동적인 좌우 padding을 사용하며 프로젝트 목록과 상세를 같은 세로 흐름에 둔다. 820px 이하에서는 표의 덜 중요한 열을 줄이고, 620px 이하에서는 top bar가 줄바꿈한다.

Toolbox는 본문을 재배치하지 않는 우측 overlay다. 폭은 최대 480px이며 좁은 화면에서는 viewport의 92%를 넘지 않는다. 설정은 중앙 dialog로 표시한다.

## Elevation & Depth

기본 화면은 평평하고 선과 tonal surface로 깊이를 만든다. 그림자는 선택된 프로젝트 행과 활성 overlay처럼 현재 상호작용 계층이 바뀔 때만 사용한다.

**The Flat-at-Rest Rule.** 평상시 목록과 section에 card shadow를 반복하지 않고 border와 surface 차이로 구조를 만든다.

## Shapes

대부분의 control은 작고 일관된 0.375rem radius와 1px border를 사용한다. 상태는 8px 원형 mark로 표시한다. 설정 dialog의 14px radius는 큰 modal 표면에만 쓰며 일반 control로 확장하지 않는다.

Alert와 notice는 기존의 3px 좌측 상태 bar를 유지한다. 이는 검토된 운영 상태 표현이며 장식적인 강조선으로 복제하지 않는다.

## Components

### Buttons

- **Primary:** deep cobalt 배경과 cloud 글자로 중요한 한 단계 행동을 표시한다.
- **Secondary:** 투명 또는 cloud 배경과 deep cobalt border를 사용한다.
- **Hover / Focus:** hover는 기존 surface 또는 밝기 변화만 사용하고, keyboard focus는 3px cobalt outline과 offset으로 표시한다.

### Inputs / Fields

- **Style:** cloud 표면, slate border, control radius를 사용한다.
- **Focus:** 3px cobalt outline을 유지한다.
- **Error / Disabled:** 오류는 inline alert로 보이고 disabled 상태는 opacity와 cursor로 구분한다. 저장 실패 시 입력을 지우지 않는다.

### Navigation

Top bar에는 브랜드, 로컬 연결, Toolbox, 설정만 둔다. 프로젝트 내부 탭은 `업무 / 자료`, Drawer 탭은 `명령어 / 할 일`로 제한하며 선택 탭은 cobalt 하단선으로 표시한다.

### Lists and status rows

프로젝트·업무·근거 행은 얇은 horizontal rule과 작은 metadata를 공유한다. 선택 행은 cloud canvas와 제한된 그림자 또는 inset cobalt bar로 구분한다. Herdr 세부 정보는 기본적으로 접고 한 줄 상태 summary를 항상 남긴다.

### Toolbox overlays

Drawer는 backdrop, Escape, 닫기 버튼을 제공하고 본문 폭을 영구 축소하지 않는다. 명령어는 복사만 제공한다. Todo는 미완료가 기본이며 완료 포함과 프로젝트 필터를 명시적으로 선택한다.

## Do's and Don'ts

### Do:

- **Do** 기존 cobalt/slate/cloud token과 Segoe UI 계열을 재사용한다.
- **Do** 상태, 실패, stale 여부를 텍스트와 mark로 함께 전달한다.
- **Do** 좁은 화면에서 열을 줄이고 control을 줄바꿈하되 고정 rail을 되살리지 않는다.
- **Do** motion을 180ms ease-out 안에서 사용하고 reduced-motion 설정을 존중한다.

### Don't:

- **Don't** 새 브랜드 색, 폰트, 이미지 또는 장식적 gradient를 추가하지 않는다.
- **Don't** 모든 section을 card와 shadow로 감싸 정보 밀도를 낮추지 않는다.
- **Don't** Toolbox 명령에 실행 버튼이나 terminal affordance를 추가하지 않는다.
- **Don't** 3px alert bar를 일반 장식이나 모든 container의 accent로 확장하지 않는다.
