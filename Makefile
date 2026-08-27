.PHONY: fmt test check

fmt:
	gofmt -w $$(find cmd internal -name '*.go')

test:
	go test ./...

check:
	@test -z "$$(gofmt -l $$(find cmd internal -name '*.go'))"
	go vet ./...
	go test ./...
