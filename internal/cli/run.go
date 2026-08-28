package cli

import (
	"context"
	"fmt"
	"io"

	"thread-dock/internal/version"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunWithDependencies(ctx, args, stdout, stderr, Dependencies{})
}

// RunWithDependencies is the stable command boundary used by agentctl and
// by the Windows monitor. Run services are injected so command routing and
// output remain testable without production adapters or credentials.
func RunWithDependencies(ctx context.Context, args []string, stdout, stderr io.Writer, deps Dependencies) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "thread-dock %s contract=%d\n", version.Build, version.Contract)
		return 0
	}
	if len(args) > 0 && args[0] == "contract" {
		return runContract(args[1:], stdout, stderr)
	}
	if len(args) > 0 {
		switch args[0] {
		case "start", "status", "stop", "resume", "cleanup":
			return runCommand(ctx, args, stdout, stderr, deps.Runs)
		}
	}
	printUsage(stderr)
	return 2
}

func printUsage(stderr io.Writer) {
	fmt.Fprintln(stderr, "사용법: agentctl version | contract <validate|preview> <file> | start CONTRACT | status [RUN] [--json] | stop RUN | resume RUN | cleanup RUN")
}
