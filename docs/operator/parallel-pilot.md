> **역사 문서.** 이 문서는 옛 v1 parallel orchestrator pilot의 기록으로 보존되며
> 현재 ThreadDock의 실행 계획·운영 절차·완료 조건이 아닙니다. 현재 제품은 Go/Wails
> Monitor가 GitHub와 Herdr 관찰을 결합하고, 실행·세션 관리는 Agent와 Herdr가 맡습니다.

# Parallel Orchestrator Pilot 기록

이 문서는 ADR 0008에서 대체한 자체 실행 엔진이 존재하던 시기의 pilot 범위와
검증 항목을 역사적으로 보존합니다. 기록에 남은 `agentctl create-revert`를 포함한
옛 명령과 절차는 현재 CLI에서 지원되지 않으며 실행 지침으로 사용해서는 안 됩니다.

현재 구조와 사용 범위는 다음 문서를 기준으로 확인합니다.

- [현재 제품 설계](../superpowers/specs/2026-09-10-herdr-first-usable-workflow-design.md)
- [GitHub-first quickstart](github-first-quickstart.md)
- [ADR 0008](../adr/0008-github-first-skills-before-engine.md)
