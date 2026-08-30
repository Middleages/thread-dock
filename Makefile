.PHONY: help fmt test test-focused vet-focused check

help:
	@echo 'make check                         Run the repository-wide verification gate'
	@echo 'make test-focused PKGS="./path/..." Run focused Go tests for explicit packages'
	@echo 'make vet-focused PKGS="./path/..."  Run focused go vet for explicit packages'

fmt:
	gofmt -w $$(find cmd internal -name '*.go')

test:
	bash -n scripts/single-run-pilot.sh
	go test ./...

test-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make test-focused PKGS="./internal/contract"' >&2; exit 2)
	go test $(PKGS)

vet-focused:
	@test -n "$(strip $(PKGS))" || (echo 'PKGS is required, for example: make vet-focused PKGS="./internal/contract"' >&2; exit 2)
	go vet $(PKGS)

check:
	@test -z "$$(gofmt -l $$(find cmd internal -name '*.go'))"
	bash -n scripts/single-run-pilot.sh
	go vet ./...
	go test ./...
