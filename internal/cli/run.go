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
		case "project", "work":
			return runProjectWork(ctx, args, stdout, stderr, deps.Workflow)
		case "start", "status", "stop", "resume", "cleanup":
			return runCommand(ctx, args, stdout, stderr, deps.Runs)
		case "retire":
			service := deps.Retirement
			if service == nil {
				if candidate, ok := deps.Runs.(RetirementService); ok {
					service = candidate
				}
			}
			return runRetire(ctx, args[1:], stdout, stderr, service)
		case "confirm":
			return runConfirm(ctx, args[1:], stdout, stderr, deps.Confirmer)
		case "create-revert":
			return runCreateRevert(ctx, args[1:], stdout, stderr, deps.Reverter)
		}
	}
	printUsage(stderr)
	return 2
}

func printUsage(stderr io.Writer) {
	fmt.Fprintln(stderr, "사용법: agentctl version | contract <validate|preview> <file> | project register PROJECT.json --expected-revision 0 --request-id ID | project list --json | project status --all --json | work plan CONTRACT.json --expected-revision 0 --request-id ID | work approve WORK --expected-revision N --request-id ID | work run WORK --expected-revision N --request-id ID | work publish-issues WORK PARENT_DRAFT_KEY --expected-revision N --request-id ID | work pause WORK --expected-revision N --request-id ID | work resume WORK --expected-revision N --request-id ID | work reconcile WORK --json | work status WORK --json | start CONTRACT | status [RUN] [--json] | stop RUN | resume RUN | retire RUN [--blocked] | cleanup RUN | confirm RUN protected-change | create-revert RUN --reason TEXT")
}
