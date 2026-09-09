package statev2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidTransition = errors.New("invalid transition")
var ErrStaleGeneration = errors.New("stale publication generation")

type InvalidTransitionError struct{ Reason string }

func (e *InvalidTransitionError) Error() string {
	if e == nil || e.Reason == "" {
		return ErrInvalidTransition.Error()
	}
	return fmt.Sprintf("%s: %s", ErrInvalidTransition, e.Reason)
}

func (e *InvalidTransitionError) Unwrap() error { return ErrInvalidTransition }

func invalidTransition(format string, args ...any) error {
	return &InvalidTransitionError{Reason: fmt.Sprintf(format, args...)}
}

type transitionPayload struct {
	Work        *WorkTransition        `json:"work,omitempty"`
	Task        *TaskTransition        `json:"task,omitempty"`
	Publication *PublicationTransition `json:"publication,omitempty"`
}

// TransitionPayloadHash returns the canonical hash of the one typed transition.
// Request metadata is intentionally excluded from the hash.
func TransitionPayloadHash(request TransitionRequest) (string, error) {
	count := 0
	if request.Work != nil {
		count++
	}
	if request.Task != nil {
		count++
	}
	if request.Publication != nil {
		count++
	}
	if count != 1 {
		return "", invalidTransition("exactly one typed transition is required")
	}
	data, err := json.Marshal(transitionPayload{Work: request.Work, Task: request.Task, Publication: request.Publication})
	if err != nil {
		return "", invalidTransition("encode transition: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateApplyRequest(request TransitionRequest) error {
	if request.RequestID == "" || strings.TrimSpace(string(request.RequestID)) == "" {
		return invalidTransition("request ID is required")
	}
	if strings.TrimSpace(request.PayloadHash) == "" {
		return invalidTransition("payload hash is required")
	}
	hash, err := TransitionPayloadHash(request)
	if err != nil {
		return err
	}
	if request.PayloadHash != hash {
		return invalidTransition("payload hash does not match transition")
	}
	if request.Work == nil && request.Task == nil && request.Publication == nil {
		return invalidTransition("a typed transition is required")
	}
	if request.Work != nil {
		switch request.Work.Action {
		case WorkApprove, WorkPause, WorkResume:
		case WorkResolve:
			if request.Work.Resolve == nil {
				return invalidTransition("unsupported work resolve transition")
			}
			switch request.Work.Resolve.Kind {
			case ResolveExtendBudget, ResolveRuntimeNotStarted, ResolveRuntimeTerminated, ResolveRetryVerifiedStage, ResolvePublicationReconciled:
			default:
				return invalidTransition("unsupported work resolve transition")
			}
		default:
			return invalidTransition("unsupported work action %q", request.Work.Action)
		}
	}
	if request.Task != nil {
		switch request.Task.Action {
		case TaskReserveInvocation, TaskBeginLaunch, TaskMarkRunning, TaskRequestTermination,
			TaskConfirmTermination, TaskReconcileNotStarted, TaskRecordCandidate, TaskRecordGate,
			TaskRecordReview, TaskRecordIntegration, TaskNeedsOperatorAction,
			TaskBeginWorktreePreparation, TaskReconcileWorktreePreparation:
		default:
			return invalidTransition("unsupported task action %q", request.Task.Action)
		}
	}
	if request.Publication != nil {
		switch request.Publication.Action {
		case PublicationBegin, PublicationComplete, PublicationFail, PublicationActionConflict, PublicationActionSupersede:
		default:
			return invalidTransition("unsupported publication action %q", request.Publication.Action)
		}
	}
	return nil
}

func applyTransition(snapshot *WorkSnapshot, request TransitionRequest) error {
	if request.Work != nil {
		return applyWorkTransition(snapshot, *request.Work)
	}
	if request.Task != nil {
		return applyTaskTransition(snapshot, *request.Task, request.RequestID)
	}
	if request.Publication != nil {
		return applyPublicationTransition(snapshot, *request.Publication)
	}
	return invalidTransition("a typed transition is required")
}
