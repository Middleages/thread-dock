---
status: accepted
---

# Go와 Wails v2로 ThreadDock 로컬 도구 구현

WSL의 Orchestrator CLI와 Windows ThreadDock Monitor를 Go로 구현하고, 데스크톱 UI에는 안정된 Wails v2와 React·TypeScript를 사용한다. Electron은 한 언어로 개발하기 쉽지만 별도 Chromium binary 배포 부담이 있고, Tauri는 Rust와 C++ toolchain을 추가하므로 비전공 Operator가 사용하는 사내 도구를 단순하게 유지하려는 목표에 맞지 않는다.

GHES Releases가 Windows 앱과 Linux `agentctl` binary를 함께 배포한다. 앱은 새 Release를 자동 다운로드하되 사용자가 적용 시점을 확인하며, 업데이트 실패 시 직전 정상 버전으로 복구한다. 앱과 CLI는 제품 버전이 아니라 구조화된 명령 계약의 호환성으로 동작 가능 여부를 판단한다.
