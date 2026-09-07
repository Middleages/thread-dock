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
	if request.Work == nil {
		return invalidTransition("task and publication transitions are not supported")
	}
	switch request.Work.Action {
	case WorkApprove, WorkPause, WorkResume:
	case WorkResolve:
		if request.Work.Resolve == nil || request.Work.Resolve.Kind != ResolveExtendBudget {
			return invalidTransition("unsupported work resolve transition")
		}
	default:
		return invalidTransition("unsupported work action %q", request.Work.Action)
	}
	return nil
}

func applyTransition(snapshot *WorkSnapshot, request TransitionRequest) error {
	if request.Work != nil {
		return applyWorkTransition(snapshot, *request.Work)
	}
	return invalidTransition("task and publication transitions are not supported")
}
