package cli

import (
	"context"
	"strings"
	"testing"
)

func TestRemovedCommandsRejectWithUsageAndNoStdout(t *testing.T) {
	for _, args := range [][]string{
		{"confirm", "run-184", "protected-change"},
		{"confirm", "run-184", "wrong"},
		{"confirm", "run-184", "protected-change", "extra"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, errOut strings.Builder
			code := RunWithDependencies(context.Background(), args, &out, &errOut, Dependencies{})
			if code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "사용법:") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
			}
		})
	}
}

func TestRemovedConfirmAndCreateRevertDoNotRequireProductionDependencies(t *testing.T) {
	for _, args := range [][]string{
		{"confirm", "run-184", "protected-change"},
		{"create-revert", "run-184", "--reason", "pilot regression"},
	} {
		if NeedsProductionDependencies(args) {
			t.Fatalf("removed command %v must not require production dependencies", args)
		}
	}
}
