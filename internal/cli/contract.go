package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"thread-dock/internal/contract"
)

type validationResult struct {
	Valid      bool                 `json:"valid"`
	Violations []contract.Violation `json:"violations,omitempty"`
	Error      string               `json:"error,omitempty"`
}

func runContract(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "validate" && args[0] != "preview") {
		printUsage(stderr)
		return 2
	}

	file, err := os.Open(args[1])
	if err != nil {
		return reportContractReadError(args[0], stdout, stderr, err)
	}
	defer file.Close()

	c, err := contract.Read(file)
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
	if command == "validate" {
		var validationErr contract.ValidationError
		result := validationResult{Valid: false, Error: err.Error()}
		if errors.As(err, &validationErr) {
			result.Violations = validationErr.Violations
			result.Error = ""
		}
		if writeValidation(stdout, result) != 0 {
			return 1
		}
		return 1
	}
	fmt.Fprintln(stderr, err)
	return 1
}

func writeValidation(stdout io.Writer, result validationResult) int {
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}
