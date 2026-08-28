package runner

import (
	"context"
	"strings"
	"testing"
)

func TestRunnerPreservesArgumentBoundaries(t *testing.T) {
	got := captureArgs(t, []string{"value with spaces", "$(not-run)"})
	if diff := strings.Join(got, "|"); diff != "value with spaces|$(not-run)" {
		t.Fatal(diff)
	}
}

func TestOSRunnerReturnsOutputAndExitCode(t *testing.T) {
	runner := OSRunner{}
	result, err := runner.Run(context.Background(), "", "/bin/sh", "-c", "printf 'output\\n'; exit 7")
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	if result.Stdout != "output\n" || result.Stderr != "" || result.ExitCode != 7 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestOSRunnerUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	result, err := (OSRunner{}).Run(context.Background(), dir, "/bin/pwd")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Stdout) != dir {
		t.Fatalf("pwd=%q want=%q", strings.TrimSpace(result.Stdout), dir)
	}
}

func captureArgs(t *testing.T, args []string) []string {
	t.Helper()
	commandArgs := append([]string{"%s\\n"}, args...)
	result, err := (OSRunner{}).Run(context.Background(), "", "/usr/bin/printf", commandArgs...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(result.Stdout, "\n"), "\n")
}
