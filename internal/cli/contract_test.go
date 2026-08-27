package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
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

func TestContractCommandsReportInvalidFilesWithExitCodeOne(t *testing.T) {
	for _, command := range []string{"validate", "preview"} {
		t.Run(command, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(context.Background(), []string{"contract", command, "../../testdata/contracts/invalid-overlap.json"}, &out, &errOut)
			if code != 1 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
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
