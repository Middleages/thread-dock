# Project Toolbox verification scope

Date: 2026-09-11

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
