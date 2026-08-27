# ThreadDock Product Design System

ThreadDock Monitor의 승인된 News Desk 구조와 Civic Cobalt 팔레트를 재사용하기 위한 제품 디자인 시스템이다. 비전공 Operator가 Agent 개발과 CI/CD 상태를 빠르게 파악하도록 문장, 행, 진행선과 행동 패널을 우선한다.

## Sources

- `PRODUCT.md` — 사용자, 목적, 기능과 제약
- `CONTEXT.md` — 도메인 용어
- `opendesign/mockups/thread-dock/news-desk-palettes.html` — 선택된 화면과 팔레트
- `opendesign/mockups/thread-dock/run-monitor-directions.html` — 검토한 대안

## Contents

- `tokens/colors_and_type.css` — Civic Cobalt 색상, type, spacing, shape와 motion token
- `brand/voice-and-tone.md` — 한국어 문구 원칙
- `brand/style-notes.md` — 시각·반응형·상태 원칙
- `ui-kit-thread-dock/components/` — 선택된 화면에서 추출한 React component
- `ui-kit-thread-dock/index.html` — 핵심 component 정적 검토 화면

## Confidence

색상, 기본 화면의 정보 위계와 상태 표현은 사용자가 직접 선택했다. 아이콘 체계와 실제 production 데이터의 최소·최대 범위는 아직 검증되지 않았으므로 확장 전에 pilot에서 확인한다.
