.PHONY: help fmt test test-focused vet-focused check

GO_FILES := $(shell find cmd internal monitor -name '*.go' -print)
FRONTEND_DIR := monitor/frontend
FRONTEND_TESTS := src/bindings.test.ts src/monitor.test.tsx

help:
	@echo 'make check                         Run the repository-wide verification gate'
	@echo 'make test-focused PKGS="./path/..." Run focused Go tests for explicit packages'
	@echo 'make vet-focused PKGS="./path/..."  Run focused go vet for explicit packages'

fmt:
	gofmt -w $(GO_FILES)

test:
	bash -n scripts/single-run-pilot.sh
	go test ./...
	npm --prefix $(FRONTEND_DIR) test -- $(FRONTEND_TESTS)
	npm --prefix $(FRONTEND_DIR) run build

test-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make test-focused PKGS="./internal/contract"' >&2; exit 2)
	go test $(PKGS)

vet-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make vet-focused PKGS="./internal/contract"' >&2; exit 2)
	go vet $(PKGS)

check:
	@test -z "$$(gofmt -l $(GO_FILES))"
	bash -n scripts/single-run-pilot.sh
	go vet ./...
	go test ./...
	npm --prefix $(FRONTEND_DIR) test -- $(FRONTEND_TESTS)
	npm --prefix $(FRONTEND_DIR) run build
