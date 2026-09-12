# ThreadDock Agent/Skill Verification Scope

Date: 2026-09-11

## 2026-09-12 Linux 검증 후속

`1e427eda673a997245c58420df83dd2f6070d06a`의 최종 `make check`는 template-check·전체 Go3 packages·UI37tests·frontend build까지 통과했다.
아래 connector 환경의 미실행 설명은 당시 기록이다. canonical Skill의 pressure scenario와 native Codex/Herdr/Windows 실사용을 실행한 것은 아니며 #43/#48에 남긴다.
검증 환경·초기 실패·정정·제한은 [ledger](../../operator/2026-09-12-engine-retirement-completion-ledger.md)를 따른다.

## What is verified in this chat/tool environment

- `project-template/.codex/agents` contains exactly the two intended ThreadDock top-level agent definitions: `td_coordinator.toml`, `td_feature_leader.toml`.
- `project-template/.agents/skills` contains the six canonical skills: `coordinate-work`, `develop-feature`, `grill-plan`, `tdd-task`, `review-change`, `publish-work`.
- Legacy `plan-work`, `open-agent-session`, `implement-task`, `record-work` remain as compatibility shims.
- Canonical skill frontmatter descriptions use trigger-only `Use when...` wording.
- Quickstart and README teach the two-agent model, global `~/.threaddock/sessions.json`, and canonical skill names.
- Makefile has `template-check` and includes it in `make check`.

## Not verified here

This ChatGPT connector environment cannot resolve `github.com` from the local container, so it cannot clone the repository or install/run the repo toolchain. It also has no native Codex/Herdr subagent dispatcher.

Therefore these remain required before merge:

1. `make template-check`
2. `make check`
3. Windows `wails build`
4. packaged Monitor smoke against the actual GHES + WSL + Herdr environment
5. writing-skills pressure-scenario validation in a subagent-capable Codex/Herdr environment, especially:
   - Coordinator does not create duplicate feature sessions when an exact live locator exists.
   - Feature Leader skips `grill-plan` for small well-specified changes.
   - `tdd-task` does not widen scope or run a worker full suite without cause.
   - reviewer stays read-only and returns concrete accept/block evidence.
   - `publish-work` never deletes unrelated project bindings or treats `idle/done` as completion.

No unexecuted check above is claimed as passing.
