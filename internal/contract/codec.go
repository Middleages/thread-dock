package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	CodeInvalidJSON    = "invalid_json"
	CodeUnreadable     = "unreadable"
	InvalidJSONMessage = "작업 계약 JSON 형식이 올바르지 않습니다"
	UnreadableMessage  = "작업 계약 파일을 읽을 수 없습니다"
)

// DiagnosticError reports one stable, user-facing contract diagnostic while
// retaining its internal cause for callers that need it.
type DiagnosticError struct {
	Violation Violation
	cause     error
}

func (e DiagnosticError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Violation.Code, e.Violation.Message)
}

func (e DiagnosticError) Unwrap() error {
	return e.cause
}

func invalidJSONError(cause error) DiagnosticError {
	return DiagnosticError{
		Violation: Violation{Code: CodeInvalidJSON, Field: "$", Message: InvalidJSONMessage},
		cause:     cause,
	}
}

// NewUnreadableError returns the stable diagnostic used when a contract file
// cannot be opened or read.
func NewUnreadableError(cause error) DiagnosticError {
	return DiagnosticError{
		Violation: Violation{Code: CodeUnreadable, Field: "$", Message: UnreadableMessage},
		cause:     cause,
	}
}

// ValidationError reports violations found while decoding or encoding a task
// contract.
type ValidationError struct {
	Violations []Violation
}

func (e ValidationError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, violation := range e.Violations {
		parts[i] = fmt.Sprintf("%s (%s): %s", violation.Code, violation.Field, violation.Message)
	}
	return "작업 계약 검증 실패: " + strings.Join(parts, "; ")
}

// ErrorViolations returns the stable user-facing diagnostics carried by a
// semantic or structural contract error.
func ErrorViolations(err error) ([]Violation, bool) {
	var validationErr ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Violations, true
	}
	var diagnosticErr DiagnosticError
	if errors.As(err, &diagnosticErr) {
		return []Violation{diagnosticErr.Violation}, true
	}
	return nil, false
}

// Read decodes and validates a task contract from strict JSON.
func Read(r io.Reader) (TaskContract, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var c TaskContract
	if err := dec.Decode(&c); err != nil {
		return c, invalidJSONError(err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return c, invalidJSONError(err)
	}
	if v := Validate(c); len(v) > 0 {
		return c, ValidationError{Violations: v}
	}
	return c, nil
}

// Write validates and encodes a task contract as indented JSON.
func Write(w io.Writer, c TaskContract) error {
	if v := Validate(c); len(v) > 0 {
		return ValidationError{Violations: v}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}
