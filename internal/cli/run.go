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
	fmt.Fprintln(stderr, "사용법: agentctl version | contract <validate|preview> <file>")
	return 2
}
