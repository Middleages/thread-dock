package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/contract"
)

func TestContractValidateJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"contract", "validate", "../../testdata/contracts/valid.json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, `"valid":true`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestContractPreviewContainsApprovalSections(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"contract", "preview", "../../testdata/contracts/valid.json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"전체 목표", "하위 작업", "제외 범위", "검증 방법", "승인 후 자동 진행"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestContractValidateUsesViolationShapeForSemanticAndStructuralFailures(t *testing.T) {
	malformedPath := writeContractFile(t, "malformed.json", []byte(`{"version":`))
	unknownPath := writeContractFile(t, "unknown.json", []byte(`{"version":1,"unexpected":true}`))
	validJSON, err := os.ReadFile("../../testdata/contracts/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	trailingJSONPath := writeContractFile(t, "trailing-document.json", append(append([]byte(nil), validJSON...), '\n', '{', '}'))
	trailingGarbagePath := writeContractFile(t, "trailing-garbage.json", append(append([]byte(nil), validJSON...), '\n', 'x'))

	invalidJSON := []contract.Violation{{
		Code:    contract.CodeInvalidJSON,
		Field:   "$",
		Message: contract.InvalidJSONMessage,
	}}
	tests := []struct {
		name       string
		path       string
		violations []contract.Violation
	}{
		{
			name: "semantic violation",
			path: "../../testdata/contracts/invalid-overlap.json",
			violations: []contract.Violation{{
				Code:    "path_overlap",
				Field:   "tasks[1].allowedPaths",
				Message: "경로 \"src/payments/**\"가 Task \"api\"의 경로 \"src/payments/**\"와 겹칩니다",
			}},
		},
		{name: "malformed JSON", path: malformedPath, violations: invalidJSON},
		{name: "unknown field", path: unknownPath, violations: invalidJSON},
		{name: "trailing JSON document", path: trailingJSONPath, violations: invalidJSON},
		{name: "trailing garbage", path: trailingGarbagePath, violations: invalidJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(context.Background(), []string{"contract", "validate", tt.path}, &out, &errOut)
			if code != 1 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
			}
			if errOut.Len() != 0 {
				t.Fatalf("stderr=%q", errOut.String())
			}
			assertValidationJSON(t, out.Bytes(), validationResult{Valid: false, Violations: tt.violations})
		})
	}
}

func TestContractValidateMapsUnreadableFilesToCodedViolation(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "file open failure", path: filepath.Join(t.TempDir(), "missing.json")},
		{name: "file read failure", path: t.TempDir()},
	}
	want := validationResult{Valid: false, Violations: []contract.Violation{{
		Code:    contract.CodeUnreadable,
		Field:   "$",
		Message: contract.UnreadableMessage,
	}}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(context.Background(), []string{"contract", "validate", tt.path}, &out, &errOut)
			if code != 1 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
			}
			if errOut.Len() != 0 {
				t.Fatalf("stderr=%q", errOut.String())
			}
			assertValidationJSON(t, out.Bytes(), want)
		})
	}
}

func TestContractPreviewReportsConciseCodedDiagnostics(t *testing.T) {
	malformedPath := writeContractFile(t, "malformed.json", []byte(`{"version":`))
	tests := []struct {
		name       string
		path       string
		wantStderr string
	}{
		{
			name:       "semantic violation",
			path:       "../../testdata/contracts/invalid-overlap.json",
			wantStderr: "[path_overlap] 경로 \"src/payments/**\"가 Task \"api\"의 경로 \"src/payments/**\"와 겹칩니다\n",
		},
		{name: "invalid JSON", path: malformedPath, wantStderr: "[invalid_json] 작업 계약 JSON 형식이 올바르지 않습니다\n"},
		{name: "unreadable file", path: filepath.Join(t.TempDir(), "missing.json"), wantStderr: "[unreadable] 작업 계약 파일을 읽을 수 없습니다\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(context.Background(), []string{"contract", "preview", tt.path}, &out, &errOut)
			if code != 1 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
			}
			if out.Len() != 0 {
				t.Fatalf("stdout=%q", out.String())
			}
			if got := errOut.String(); got != tt.wantStderr {
				t.Fatalf("stderr=%q, want %q", got, tt.wantStderr)
			}
		})
	}
}

func TestContractCommandsRejectMisuseWithExitCodeTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"contract", "validate"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

func writeContractFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertValidationJSON(t *testing.T, data []byte, want validationResult) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var got validationResult
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("decode validation JSON: %v; output=%s", err, data)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		t.Fatalf("extra validation JSON: %v; output=%s", err, data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("validation JSON = %#v, want %#v", got, want)
	}
}
