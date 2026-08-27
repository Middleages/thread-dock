# Task 3 report: strict JSON codec and compatibility fixtures

## RED

Added codec tests first, then ran:

```text
PATH=/home/appuser/.local/share/threaddock-toolchains/go1.27.0/bin:$PATH go test ./internal/contract -run 'Test(Read|Write)' -v
```

The package failed because `contract.Read` and `contract.Write` were undefined.

## GREEN

Implemented strict unknown-field decoding, validation on read/write, indented JSON encoding, and `ValidationError`. Added canonical valid and invalid-overlap fixtures and tests for fixture drift and validation rejection.

Commands and results:

```text
PATH=/home/appuser/.local/share/threaddock-toolchains/go1.27.0/bin:$PATH go test ./internal/contract -v
PASS

PATH=/home/appuser/.local/share/threaddock-toolchains/go1.27.0/bin:$PATH make check
go vet ./...
go test ./...
PASS
```

## Files

- `internal/contract/codec.go`
- `internal/contract/codec_test.go`
- `testdata/contracts/valid.json`
- `testdata/contracts/invalid-overlap.json`

## Self-review

- `Read` uses `json.Decoder.DisallowUnknownFields` and validates decoded contracts.
- `Write` validates before creating/writing through the encoder, so invalid contracts do not partially encode.
- Fixture bytes are asserted against the canonical fixture serialization.
- `git diff --check` passed.

## Concerns

- `make check` requires the repository-specified Go 1.27 toolchain on `PATH`; the initial invocation without that `PATH` failed because `go` is not globally installed. The mandated toolchain invocation passed.

## Commit

`e552f55 feat: 작업 계약 JSON 호환성 고정`
