package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

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

// Read decodes and validates a task contract from strict JSON.
func Read(r io.Reader) (TaskContract, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var c TaskContract
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("작업 계약 JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("작업 계약 JSON: 문서 뒤 추가 내용이 허용되지 않습니다")
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
