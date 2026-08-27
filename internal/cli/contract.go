package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"thread-dock/internal/contract"
)

type validationResult struct {
	Valid      bool                 `json:"valid"`
	Violations []contract.Violation `json:"violations,omitempty"`
}

func runContract(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "validate" && args[0] != "preview") {
		printUsage(stderr)
		return 2
	}

	data, err := os.ReadFile(args[1])
	if err != nil {
		return reportContractReadError(args[0], stdout, stderr, contract.NewUnreadableError(err))
	}

	c, err := contract.Read(bytes.NewReader(data))
	if err != nil {
		return reportContractReadError(args[0], stdout, stderr, err)
	}

	if args[0] == "validate" {
		return writeValidation(stdout, validationResult{Valid: true})
	}
	fmt.Fprint(stdout, contract.Preview(c))
	return 0
}

func reportContractReadError(command string, stdout, stderr io.Writer, err error) int {
	violations, ok := contract.ErrorViolations(err)
	if !ok {
		violations = []contract.Violation{contract.NewUnreadableError(err).Violation}
	}
	if command == "validate" {
		if writeValidation(stdout, validationResult{Valid: false, Violations: violations}) != 0 {
			return 1
		}
		return 1
	}
	diagnostic := violations[0]
	fmt.Fprintf(stderr, "[%s] %s\n", diagnostic.Code, diagnostic.Message)
	return 1
}

func writeValidation(stdout io.Writer, result validationResult) int {
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}
