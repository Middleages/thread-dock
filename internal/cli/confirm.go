package cli

import (
	"context"
	"errors"

	"thread-dock/internal/contract"
)

// ProtectedChangeConfirmer is the narrow injected service behind the
// operator-only confirmation command.
type ProtectedChangeConfirmer interface {
	ConfirmProtectedChange(context.Context, contract.RunID) error
}

func runConfirm(ctx context.Context, args []string, stdout, stderr interface{ Write([]byte) (int, error) }, confirmer ProtectedChangeConfirmer) int {
	if len(args) != 2 || args[0] == "" || args[1] != "protected-change" {
		printUsage(stderr)
		return 2
	}
	if confirmer == nil {
		return reportRunError(stderr, errors.New("보호 변경 확인 서비스가 구성되지 않았습니다"))
	}
	if err := confirmer.ConfirmProtectedChange(ctx, contract.RunID(args[0])); err != nil {
		return reportRunError(stderr, err)
	}
	return 0
}
