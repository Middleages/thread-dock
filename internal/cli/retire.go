package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"thread-dock/internal/contract"
	"thread-dock/internal/orchestrator"
	"thread-dock/internal/state"
)

// RetirementCoordinator is the orchestration subset needed by the explicit
// retire command. It is separate from RunCoordinator so existing callers can
// continue injecting the older start/advance/stop seam.
type RetirementCoordinator interface {
	BeginRetirement(context.Context, contract.RunID, contract.RunPhase, bool) error
	Advance(context.Context, contract.RunID) error
}

var (
	errRetirementNeedsOperator = errors.New("retirement requires operator attention")
	errRetirementNotTerminal   = errors.New("retirement did not reach a terminal state")
)

// Retire validates the durable run phase, creates or resumes its exact
// retirement plan, and advances until the plan reaches its recorded terminal
// phase. Each coordinator Advance still performs at most one provider action;
// the bound merely prevents a broken adapter from creating an unbounded CLI
// loop.
func (s *OrchestratorRunService) Retire(ctx context.Context, id contract.RunID, blocked bool) error {
	if s == nil || s.store == nil || s.coordinator == nil {
		return errRunServiceMissing
	}
	if ctx == nil {
		ctx = context.Background()
	}
	coordinator, ok := s.coordinator.(RetirementCoordinator)
	if !ok {
		return errors.New("retirement service is not configured")
	}
	snapshot, err := s.store.Load(ctx, id)
	if err != nil {
		return err
	}
	targetPhase, err := retirementRequestPhase(snapshot, blocked)
	if err != nil {
		return err
	}
	if snapshot.Retirement.Status == "retired" {
		return nil
	}
	if snapshot.Retirement.Status == "needs_operator" || snapshot.Phase == contract.PhaseNeedsOperator {
		return errRetirementNeedsOperator
	}
	if snapshot.Phase != contract.PhaseRetiring {
		if err := coordinator.BeginRetirement(ctx, id, targetPhase, false); err != nil {
			return err
		}
		snapshot, err = s.store.Load(ctx, id)
		if err != nil {
			return err
		}
	}

	// Build's target order and the policy's fixed stages determine the maximum
	// work needed for one target: agent observation, Git proof, Workspace
	// observation, close, and close reconciliation. The extra step covers the
	// empty-plan/terminal transition and a persisted plan prepare.
	limit := retirementAdvanceLimit(len(snapshot.Retirement.Targets))
	for attempt := 0; attempt < limit; attempt++ {
		snapshot, err = s.store.Load(ctx, id)
		if err != nil {
			return err
		}
		if snapshot.Retirement.Status == "retired" {
			return nil
		}
		if snapshot.Retirement.Status == "needs_operator" || snapshot.Phase == contract.PhaseNeedsOperator {
			return errRetirementNeedsOperator
		}
		if snapshot.Phase != contract.PhaseRetiring {
			return errRetirementNotTerminal
		}
		if err := coordinator.Advance(ctx, id); err != nil {
			// The orchestrator uses ErrRunFinished to stop a needs_operator
			// retirement. Reloading above on the next iteration would be wasteful;
			// return a stable operator-facing error without exposing adapter text.
			if errors.Is(err, orchestrator.ErrRunFinished) {
				latest, loadErr := s.store.Load(ctx, id)
				if loadErr == nil && latest.Retirement.Status == "needs_operator" {
					return errRetirementNeedsOperator
				}
			}
			return err
		}
	}
	return errRetirementNotTerminal
}

func retirementRequestPhase(snapshot state.RunSnapshot, blocked bool) (contract.RunPhase, error) {
	if snapshot.Phase == contract.PhaseNeedsOperator {
		return "", errRetirementNeedsOperator
	}
	if snapshot.Phase == contract.PhasePaused {
		return "", errors.New("paused runs cannot be retired")
	}
	if snapshot.Phase == contract.PhaseRetiring {
		target := snapshot.Retirement.TargetPhase
		if target != contract.PhaseCompleted && target != contract.PhaseBlocked {
			return "", errors.New("retirement target phase is invalid")
		}
		if blocked && target != contract.PhaseBlocked {
			return "", errors.New("--blocked is only valid for blocked runs")
		}
		if !blocked && target == contract.PhaseBlocked {
			return "", errors.New("blocked retirement requires --blocked")
		}
		return target, nil
	}
	switch snapshot.Phase {
	case contract.PhaseCompleted:
		if blocked {
			return "", errors.New("--blocked is only valid for blocked runs")
		}
		return contract.PhaseCompleted, nil
	case contract.PhaseBlocked:
		if !blocked {
			return "", errors.New("blocked retirement requires --blocked")
		}
		return contract.PhaseBlocked, nil
	default:
		return "", fmt.Errorf("run phase %q cannot be retired", snapshot.Phase)
	}
}

func retirementAdvanceLimit(targets int) int {
	if targets < 1 {
		return 2
	}
	return targets*5 + 2
}

func sessionLifecycle(snapshot state.RunSnapshot) string {
	if snapshot.Retirement.Status == "retired" {
		return "retired"
	}
	if snapshot.Phase == contract.PhaseRetiring || (len(snapshot.Retirement.Targets) > 0 && (strings.TrimSpace(snapshot.Retirement.Status) == "pending" || snapshot.Retirement.Status == "needs_operator")) {
		return "retiring"
	}
	return "active"
}

var _ RetirementService = (*OrchestratorRunService)(nil)
