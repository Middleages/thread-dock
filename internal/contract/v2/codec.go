package contractv2

import (
	"encoding/json"
	"fmt"
	"io"
)

// Read decodes one complete v2 contract and rejects unknown or trailing JSON.
func Read(r io.Reader) (WorkItemContract, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var contract WorkItemContract
	if err := decoder.Decode(&contract); err != nil {
		return contract, fmt.Errorf("decode contract: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return contract, fmt.Errorf("decode contract: trailing JSON value")
		}
		return contract, fmt.Errorf("decode contract: trailing data: %w", err)
	}
	if violations := Validate(contract); len(violations) > 0 {
		return contract, ValidationError{Violations: violations}
	}
	return contract, nil
}

// Write validates and emits one indented v2 contract.
func Write(w io.Writer, contract WorkItemContract) error {
	if violations := Validate(contract); len(violations) > 0 {
		return ValidationError{Violations: violations}
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(contract)
}

// Decode and Encode are convenient aliases for callers that use codec naming.
func Decode(r io.Reader) (WorkItemContract, error) { return Read(r) }
func Encode(w io.Writer, c WorkItemContract) error { return Write(w, c) }

type ValidationError struct{ Violations []Violation }

func (e ValidationError) Error() string {
	return fmt.Sprintf("contract validation failed: %d violation(s)", len(e.Violations))
}
