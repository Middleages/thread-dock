.PHONY: help fmt test test-focused vet-focused template-check check

GO_FILES := $(shell find internal monitor -name '*.go' -print)
FRONTEND_DIR := monitor/frontend
FRONTEND_TESTS := src/bindings.test.ts src/monitor.test.tsx src/WorkTable.test.tsx src/AppScope.test.tsx src/MarkdownBody.test.tsx src/ProjectToolbox.test.tsx
CANONICAL_SKILLS := coordinate-work develop-feature grill-plan tdd-task review-change publish-work

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
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make test-focused PKGS="./internal/contract"' >&2; exit 2)
	go test $(PKGS)

vet-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make vet-focused PKGS="./internal/contract"' >&2; exit 2)
	go vet $(PKGS)

template-check:
	@test -f project-template/.codex/agents/td_coordinator.toml
	@test -f project-template/.codex/agents/td_feature_leader.toml
	@for skill in $(CANONICAL_SKILLS); do \
		test -f "project-template/.agents/skills/$$skill/SKILL.md" || { echo "missing skill: $$skill" >&2; exit 1; }; \
		grep -q '^description: Use when' "project-template/.agents/skills/$$skill/SKILL.md" || { echo "invalid skill description: $$skill" >&2; exit 1; }; \
	done

check: template-check
	@test -z "$$(gofmt -l $(GO_FILES))"
	go vet ./...
	go test ./...
	npm --prefix $(FRONTEND_DIR) test -- $(FRONTEND_TESTS)
	npm --prefix $(FRONTEND_DIR) run build
