package cli

import (
	"context"
	"fmt"
	"io"

	"thread-dock/internal/version"
)

func Run(_ context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "thread-dock %s contract=%d\n", version.Build, version.Contract)
		return 0
	}
	if len(args) > 0 && args[0] == "contract" {
		return runContract(args[1:], stdout, stderr)
	}
	printUsage(stderr)
	return 2
}

func printUsage(stderr io.Writer) {
	fmt.Fprintln(stderr, "사용법: agentctl version | contract <validate|preview> <file>")
}
