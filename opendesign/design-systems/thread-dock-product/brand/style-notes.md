# Visual Foundations

## World

News Desk의 작업 원장처럼 한 행이 하나의 사실을 가진다. 장식용 dashboard card보다 ruled row, open region과 정렬된 숫자를 사용한다.

## Color

- 회청색 canvas와 거의 흰 primary surface가 장시간 관찰의 배경이다.
- Cobalt는 현재 단계, focus와 primary action에만 사용한다.
- Green은 완료·정상, amber는 확인 필요에만 사용한다.
- 상태는 항상 한국어 문구와 함께 표시한다.

## Typography

- Windows 기본 품질과 한글 지원을 위해 Segoe UI Variable을 우선하고 Pretendard, Noto Sans KR로 fallback한다.
- Heading은 굵기와 크기로 구분하며 장식용 uppercase label을 만들지 않는다.
- 시간, 횟수와 commit은 tabular numeral을 사용한다.

## Layout

- 기본 화면은 navigation, work ledger, action rail 세 영역이다.
- 1100px 미만에서는 action rail을 ledger 아래로 내린다.
- 820px 미만에서는 navigation을 상단 가로 목록으로 바꾸고 표의 Issue 열을 숨긴다.
- 200% 배율에서도 가로 page scroll을 만들지 않는다.

## Components

- Control radius는 6px, panel radius는 최대 8px다.
- Shadow는 선택된 실행 행 하나에만 사용한다.
- 행 구분은 1px border를 사용하고 card를 중첩하지 않는다.
- Primary action은 짙은 foreground 또는 cobalt fill, secondary action은 1px outline이다.

## Motion

- 상태 갱신은 180ms ease-out으로 background만 전환한다.
- 진행 중 상태를 무한 pulse하지 않는다.
- `prefers-reduced-motion`에서는 모든 transition을 제거한다.

## Iconography

v0.1은 아이콘 없이 text label과 status dot을 사용한다. 향후 아이콘이 필요하면 하나의 검토된 SVG library를 고정하며 Unicode glyph와 emoji로 대체하지 않는다.
