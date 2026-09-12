# Project Toolbox verification scope

Date: 2026-09-11

## 2026-09-12 Linux 검증 후속

옛 엔진 정리 branch의 `1e427eda673a997245c58420df83dd2f6070d06a`에서 최종 `make check`가 통과했다.
Go3 packages, UI6파일/37tests(기존 Toolbox·scope·bindings 포함), TypeScript/Vite build를 검증했다.
아래 최초 connector 환경의 미실행 기록은 역사다. 이번 결과도 Windows packaged app의 실제 파일 열기·GHES/WSL/Herdr smoke를 대신하지 않는다.
초기 실패·test selector 정정·tmpfs/cache 환경과 성공 근거는 [ledger](../../operator/2026-09-12-engine-retirement-completion-ledger.md)에 있다.

## Intended behavior

- Project Toolbox is human-only local state and is not automatically injected into Coordinator, Feature Leader, or Skills.
- Data is stored separately from Monitor settings and Herdr locator in `%APPDATA%\ThreadDock\projects.json` on Windows.
- Toolbox sections are references, copy-only commands, and personal checklist items.
- Commands have no execution binding.
- Web references accept only HTTP(S); local and WSL references require absolute paths.
- WSL file opening converts a registered WSL path with the configured distribution before opening it as a local file.

## Added test coverage

- Go store keeps project entries independent.
- Invalid executable/relative references are rejected.
- Unknown projects return non-nil empty collections.
- App Get/Save bindings use the local Toolbox store.
- Unsafe local targets are rejected before launch.
- Frontend Wails binding coverage includes Toolbox Get/Save/OpenReference.
- Project Toolbox UI covers load, copy-only command behavior, and immediate checklist persistence.
- App scope coverage verifies the human-only Toolbox tab.

## Verification limitation

The current ChatGPT container cannot resolve `github.com`, so a repository checkout and current-SHA `make check` / Windows `wails build` could not be executed here. These tests are source-level coverage until run in the user's development environment. Do not treat their presence as passing evidence.
