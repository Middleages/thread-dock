.PHONY: help fmt test test-focused vet-focused template-check check

GO_FILES := $(shell find internal monitor -name '*.go' -print)
FRONTEND_DIR := monitor/frontend
FRONTEND_TESTS := src/bindings.test.ts src/project-key.test.ts src/monitor.test.tsx src/WorkTable.test.tsx src/AppScope.test.tsx src/AppRefresh.test.tsx src/MarkdownBody.test.tsx src/ProjectReferences.test.tsx src/GlobalToolboxDrawer.test.tsx src/TopBar.test.tsx src/ProjectDetail.test.tsx src/HerdrSummary.test.tsx src/SettingsShell.test.tsx src/monitor-presentation.test.ts
CANONICAL_SKILLS := coordinate-work develop-feature grill-plan tdd-task review-change publish-work
HELPER_SKILLS := herdr-local explore-codebase write-project-docs
SPECIALIST_AGENTS := td_explorer td_docs_editor td_implementer td_reviewer

help:
	@echo 'make check                         Run the repository-wide verification gate'
	@echo 'make template-check                Validate ThreadDock project-template agents and skills'
	@echo 'make test-focused PKGS="./monitor" Run focused Go tests for explicit packages'
	@echo 'make vet-focused PKGS="./internal/runner"  Run focused go vet for explicit packages'

fmt:
	gofmt -w $(GO_FILES)

test:
	go test ./...
	npm --prefix $(FRONTEND_DIR) test -- $(FRONTEND_TESTS)
	npm --prefix $(FRONTEND_DIR) run build

test-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make test-focused PKGS="./monitor"' >&2; exit 2)
	go test $(PKGS)

vet-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make vet-focused PKGS="./internal/runner"' >&2; exit 2)
	go vet $(PKGS)

template-check:
	@test -f project-template/.codex/agents/td_coordinator.toml
	@test -f project-template/.codex/agents/td_feature_leader.toml
	@for agent in $(SPECIALIST_AGENTS); do \
		test -f "project-template/.codex/agents/$$agent.toml" || { echo "missing Codex agent: $$agent" >&2; exit 1; }; \
		test -f "project-template/.opencode/agents/$$agent.md" || { echo "missing OpenCode agent: $$agent" >&2; exit 1; }; \
	done
	@for skill in $(CANONICAL_SKILLS); do \
		test -f "project-template/.agents/skills/$$skill/SKILL.md" || { echo "missing skill: $$skill" >&2; exit 1; }; \
		grep -q '^description: Use when' "project-template/.agents/skills/$$skill/SKILL.md" || { echo "invalid skill description: $$skill" >&2; exit 1; }; \
	done
	@for skill in $(HELPER_SKILLS); do \
		test -f "project-template/.agents/skills/$$skill/SKILL.md" || { echo "missing helper skill: $$skill" >&2; exit 1; }; \
		grep -q '^description: Use when' "project-template/.agents/skills/$$skill/SKILL.md" || { echo "invalid helper description: $$skill" >&2; exit 1; }; \
	done

check: template-check
	@test -z "$$(gofmt -l $(GO_FILES))"
	go vet ./...
	go test ./...
	npm --prefix $(FRONTEND_DIR) test -- $(FRONTEND_TESTS)
	npm --prefix $(FRONTEND_DIR) run build
