---
status: accepted
---

# Windows Monitor는 WSL CLI 계약을 사용한다

ThreadDock Monitor는 로컬 HTTP server나 WSL 상태 파일을 직접 사용하지 않고 `wsl.exe agentctl ... --json`을 호출한다. HTTP 인증·port·방화벽과 DB schema 결합을 피하면서 OpenCode, Windows 앱과 사람이 같은 작은 interface를 사용하기 위한 선택이다. 현재 한 active run과 3~5초 polling 규모에서는 process 시작 비용을 받아들이며, 실제 병목이 측정될 때만 같은 계약 뒤의 transport를 Unix socket으로 바꾼다.
