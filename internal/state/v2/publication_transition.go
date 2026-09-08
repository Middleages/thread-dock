package statev2

import (
	"encoding/hex"
	"math"
	"strings"
	"unicode/utf8"
)

func applyPublicationTransition(snapshot *WorkSnapshot, transition PublicationTransition) error {
	if !approvedSnapshot(snapshot) {
		return invalidTransition("work is not approved")
	}
	if strings.TrimSpace(string(transition.IntentID)) == "" {
		return invalidTransition("publication intent ID is required")
	}
	if transition.Action != PublicationBegin && transition.Action != PublicationActionSupersede {
		if transition.Generation == 0 {
			return invalidTransition("publication generation is required")
		}
	}
	switch transition.Action {
	case PublicationBegin:
		return beginPublication(snapshot, transition)
	case PublicationComplete:
		return completePublication(snapshot, transition)
	case PublicationFail:
		return failPublication(snapshot, transition)
	case PublicationActionConflict:
		return conflictPublication(snapshot, transition)
	case PublicationActionSupersede:
		return supersedePublication(snapshot, transition)
	default:
		return invalidTransition("unsupported publication action %q", transition.Action)
	}
}

func beginPublication(snapshot *WorkSnapshot, transition PublicationTransition) error {
	if transition.Generation == 0 {
		return invalidTransition("publication generation is required")
	}
	if _, ok := snapshot.Publications[transition.IntentID]; !ok && transition.Generation <= maxPublicationGeneration(snapshot, transition.Key) {
		return ErrStaleGeneration
	}
	if snapshot.Control.PauseRequested || snapshot.Control.Blocker != nil || snapshot.State == StateAwaitingApproval || snapshot.State == StateDraft || snapshot.State == StateNeedsOperator || snapshot.State == StateCompleted || snapshot.State == StatePaused {
		return invalidTransition("work cannot dispatch publication in state %q", snapshot.State)
	}
	if existing, ok := snapshot.Publications[transition.IntentID]; ok {
		if maxPublicationGeneration(snapshot, existing.Key) != existing.Generation {
			return ErrStaleGeneration
		}
		if transition.Generation != existing.Generation {
			return staleOrInvalidGeneration(snapshot, existing.Key, transition.Generation)
		}
		if existing.Status != PublicationFailed {
			return invalidTransition("publication intent cannot begin from status %q", existing.Status)
		}
		if transition.Key == "" || transition.Key != existing.Key {
			return invalidTransition("publication key is required and must match intent")
		}
		if transition.Key != "" && transition.Key != existing.Key || transition.Kind != "" && transition.Kind != existing.Kind || transition.PayloadHash != "" && transition.PayloadHash != existing.PayloadHash || transition.PayloadRef != "" && transition.PayloadRef != existing.PayloadRef || transition.Target != nil && *transition.Target != existing.Target || transition.CompletionRequired != nil && *transition.CompletionRequired != existing.CompletionRequired {
			return invalidTransition("publication immutable identity does not match")
		}
		if existing.Attempts == math.MaxUint32 {
			return invalidTransition("publication attempts exhausted")
		}
		existing.Status = PublicationPending
		existing.Attempts++
		existing.LastError = ""
		existing.Receipt = nil
		snapshot.Publications[existing.IntentID] = existing
		reduce(snapshot)
		return nil
	}
	if transition.CompletionRequired == nil {
		return invalidTransition("completion-required hint is required for a new publication")
	}
	max := maxPublicationGeneration(snapshot, transition.Key)
	if transition.Generation <= max {
		return ErrStaleGeneration
	}
	if max > 0 {
		current := publicationForGeneration(snapshot, transition.Key, max)
		if current.Status != PublicationCompleted {
			return invalidTransition("new publication generation requires completed prior generation")
		}
		if transition.Kind != current.Kind || transition.Target == nil || *transition.Target != current.Target || transition.CompletionRequired == nil || *transition.CompletionRequired != current.CompletionRequired {
			return invalidTransition("publication generation lineage does not match prior generation")
		}
	}
	if transition.Generation != max+1 {
		return staleOrInvalidGeneration(snapshot, transition.Key, transition.Generation)
	}
	if err := validateNewPublicationIdentity(transition); err != nil {
		return err
	}
	publication := PublicationState{IntentID: transition.IntentID, Key: transition.Key, Generation: transition.Generation, Kind: transition.Kind, Status: PublicationPending, PayloadHash: transition.PayloadHash, PayloadRef: transition.PayloadRef, Target: *transition.Target, Attempts: 1, CompletionRequired: *transition.CompletionRequired}
	if snapshot.Publications == nil {
		snapshot.Publications = map[PublicationIntentID]PublicationState{}
	}
	snapshot.Publications[publication.IntentID] = publication
	reduce(snapshot)
	return nil
}

func publicationForGeneration(snapshot *WorkSnapshot, key PublicationKey, generation uint32) PublicationState {
	for _, publication := range snapshot.Publications {
		if publication.Key == key && publication.Generation == generation {
			return publication
		}
	}
	return PublicationState{}
}

func validateNewPublicationIdentity(transition PublicationTransition) error {
	if transition.IntentID == "" || strings.TrimSpace(string(transition.IntentID)) != string(transition.IntentID) {
		return invalidTransition("publication intent ID is invalid")
	}
	if transition.Key == "" || strings.TrimSpace(string(transition.Key)) != string(transition.Key) {
		return invalidTransition("publication key is required")
	}
	if transition.Generation == 0 || !validPublicationKind(transition.Kind) {
		return invalidTransition("publication identity is invalid")
	}
	if transition.CompletionRequired == nil {
		return invalidTransition("completion-required hint is required for a new publication")
	}
	if !validPayloadHash(transition.PayloadHash) {
		return invalidTransition("publication payload hash must be lowercase SHA-256")
	}
	if !validBoundedNonempty(transition.PayloadRef) {
		return invalidTransition("publication payload reference is required")
	}
	if transition.Target == nil || strings.TrimSpace(transition.Target.Host) == "" || transition.Target.Key != transition.Key || strings.TrimSpace(transition.Target.Host) != transition.Target.Host || strings.TrimSpace(string(transition.Target.Repository)) != string(transition.Target.Repository) || strings.TrimSpace(transition.Target.Resource) != transition.Target.Resource || strings.TrimSpace(transition.Target.Base) != transition.Target.Base {
		return invalidTransition("publication target does not match key")
	}
	return nil
}

func validPayloadHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func completePublication(snapshot *WorkSnapshot, transition PublicationTransition) error {
	publication, err := currentPublication(snapshot, transition)
	if err != nil {
		return err
	}
	if publication.Status != PublicationPending {
		return invalidTransition("publication is not pending")
	}
	if err := validateImmutableHints(publication, transition); err != nil {
		return err
	}
	if transition.Receipt == nil || !validPublicationReceipt(*transition.Receipt) {
		return invalidTransition("valid publication receipt is required")
	}
	if transition.Key != "" && transition.Key != publication.Key {
		return invalidTransition("publication key does not match intent")
	}
	receipt := *transition.Receipt
	publication.Status = PublicationCompleted
	publication.Receipt = &receipt
	publication.LastError = ""
	snapshot.Publications[publication.IntentID] = publication
	reduce(snapshot)
	return nil
}

func failPublication(snapshot *WorkSnapshot, transition PublicationTransition) error {
	publication, err := currentPublication(snapshot, transition)
	if err != nil {
		return err
	}
	if publication.Status != PublicationPending {
		return invalidTransition("publication is not pending")
	}
	if err := validateImmutableHints(publication, transition); err != nil {
		return err
	}
	if err := validateDiagnostic(transition.Diagnostic); err != nil || strings.TrimSpace(transition.Diagnostic) == "" {
		return invalidTransition("bounded failure diagnostic is required")
	}
	publication.Status = PublicationFailed
	publication.LastError = boundedDiagnostic(transition.Diagnostic)
	snapshot.Publications[publication.IntentID] = publication
	reduce(snapshot)
	return nil
}

func conflictPublication(snapshot *WorkSnapshot, transition PublicationTransition) error {
	if snapshot.Control.Blocker != nil {
		return invalidTransition("work has an operator blocker")
	}
	publication, err := currentPublication(snapshot, transition)
	if err != nil {
		return err
	}
	if publication.Status != PublicationPending && publication.Status != PublicationFailed {
		return invalidTransition("publication cannot enter conflict from status %q", publication.Status)
	}
	if err := validateImmutableHints(publication, transition); err != nil {
		return err
	}
	if err := validateDiagnostic(transition.Diagnostic); err != nil || strings.TrimSpace(transition.Diagnostic) == "" || transition.Blocker == nil || transition.Blocker.Kind != BlockerKindPublicationConflict || transition.Blocker.IntentID != publication.IntentID || strings.TrimSpace(transition.Blocker.OperatorRef) == "" || strings.TrimSpace(transition.Blocker.Diagnostic) == "" {
		return invalidTransition("publication conflict evidence is incomplete")
	}
	if err := validateDiagnostic(transition.Blocker.Diagnostic); err != nil {
		return invalidTransition("publication conflict blocker: %v", err)
	}
	blocker := *transition.Blocker
	blocker.Diagnostic = boundedDiagnostic(blocker.Diagnostic)
	publication.Status = PublicationConflict
	publication.LastError = boundedDiagnostic(transition.Diagnostic)
	publication.Receipt = nil
	snapshot.Publications[publication.IntentID] = publication
	snapshot.Control.Blocker = &blocker
	reduce(snapshot)
	return nil
}

func supersedePublication(snapshot *WorkSnapshot, transition PublicationTransition) error {
	if snapshot.Control.PauseRequested || snapshot.Control.Blocker != nil || snapshot.State == StateNeedsOperator || snapshot.State == StatePaused || snapshot.State == StateCompleted {
		return invalidTransition("work cannot supersede publication in state %q", snapshot.State)
	}
	if transition.Supersedes == "" || transition.Supersedes == transition.IntentID {
		return invalidTransition("superseded intent is required")
	}
	old, ok := snapshot.Publications[transition.Supersedes]
	if !ok {
		return invalidTransition("superseded publication intent is unknown")
	}
	if maxPublicationGeneration(snapshot, old.Key) != old.Generation {
		return ErrStaleGeneration
	}
	if old.Status != PublicationFailed || transition.Resolution == nil || !transition.Resolution.NotPublished || transition.Resolution.RemoteMatch || strings.TrimSpace(transition.Resolution.Diagnostic) == "" {
		return invalidTransition("publication supersede requires proven not-published failure")
	}
	if err := validateDiagnostic(transition.Resolution.Diagnostic); err != nil {
		return invalidTransition("supersede resolution: %v", err)
	}
	if transition.Generation != old.Generation+1 || transition.Key != old.Key || transition.Kind != old.Kind || transition.CompletionRequired == nil || *transition.CompletionRequired != old.CompletionRequired || transition.Target == nil || *transition.Target != old.Target {
		return invalidTransition("publication supersede identity does not match prior generation")
	}
	if _, exists := snapshot.Publications[transition.IntentID]; exists {
		return invalidTransition("publication intent ID already exists")
	}
	if err := validateNewPublicationIdentity(transition); err != nil {
		return err
	}
	old.Status = PublicationSuperseded
	snapshot.Publications[old.IntentID] = old
	snapshot.Publications[transition.IntentID] = PublicationState{IntentID: transition.IntentID, Key: transition.Key, Generation: transition.Generation, Kind: transition.Kind, Status: PublicationPending, PayloadHash: transition.PayloadHash, PayloadRef: transition.PayloadRef, Target: *transition.Target, Attempts: 1, CompletionRequired: *transition.CompletionRequired}
	reduce(snapshot)
	return nil
}

func currentPublication(snapshot *WorkSnapshot, transition PublicationTransition) (PublicationState, error) {
	if transition.Key == "" {
		return PublicationState{}, invalidTransition("publication key is required")
	}
	publication, ok := snapshot.Publications[transition.IntentID]
	if !ok {
		if transition.Generation > 0 && transition.Generation <= maxPublicationGeneration(snapshot, transition.Key) {
			return PublicationState{}, ErrStaleGeneration
		}
		return PublicationState{}, invalidTransition("publication intent is unknown")
	}
	if transition.Generation < maxPublicationGeneration(snapshot, publication.Key) {
		return PublicationState{}, ErrStaleGeneration
	}
	if transition.Generation != publication.Generation {
		return PublicationState{}, invalidTransition("publication generation does not match intent")
	}
	if transition.Key != "" && transition.Key != publication.Key {
		return PublicationState{}, invalidTransition("publication key does not match intent")
	}
	return publication, nil
}

func validateImmutableHints(publication PublicationState, transition PublicationTransition) error {
	if transition.Key != "" && transition.Key != publication.Key || transition.Kind != "" && transition.Kind != publication.Kind || transition.PayloadHash != "" && transition.PayloadHash != publication.PayloadHash || transition.PayloadRef != "" && transition.PayloadRef != publication.PayloadRef || transition.Target != nil && *transition.Target != publication.Target || transition.CompletionRequired != nil && *transition.CompletionRequired != publication.CompletionRequired {
		return invalidTransition("publication immutable identity does not match")
	}
	return nil
}

func staleOrInvalidGeneration(snapshot *WorkSnapshot, key PublicationKey, generation uint32) error {
	if generation <= maxPublicationGeneration(snapshot, key) {
		return ErrStaleGeneration
	}
	return invalidTransition("publication generation must follow current generation")
}

func maxPublicationGeneration(snapshot *WorkSnapshot, key PublicationKey) uint32 {
	var max uint32
	for _, publication := range snapshot.Publications {
		if publication.Key == key && publication.Generation > max {
			max = publication.Generation
		}
	}
	return max
}

func validBoundedNonempty(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len([]byte(value)) <= MaxDiagnosticBytes && utf8.ValidString(value)
}

func boundedDiagnostic(value string) string {
	if len([]byte(value)) <= MaxDiagnosticBytes {
		return value
	}
	b := []byte(value)[:MaxDiagnosticBytes]
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}

func applyPublicationReconcile(snapshot *WorkSnapshot, transition WorkTransition) error {
	payload := transition.Resolve
	if payload == nil || strings.TrimSpace(payload.OperatorRef) == "" || payload.IntentID == "" || payload.Evidence == nil {
		return invalidTransition("invalid publication reconciliation")
	}
	blocker := snapshot.Control.Blocker
	if blocker == nil || blocker.Kind != BlockerKindPublicationConflict || blocker.OperatorRef != payload.OperatorRef || blocker.IntentID != payload.IntentID {
		return invalidTransition("publication reconciliation does not match blocker")
	}
	publication, ok := snapshot.Publications[payload.IntentID]
	if !ok || publication.Generation != maxPublicationGeneration(snapshot, publication.Key) || publication.Status != PublicationConflict {
		return invalidTransition("publication reconciliation intent is not current conflict")
	}
	remote, absent := payload.Evidence.RemoteMatch, payload.Evidence.NotPublished
	if remote == absent {
		return invalidTransition("publication reconciliation evidence is ambiguous")
	}
	if remote {
		if payload.PublicationReceipt == nil || !validPublicationReceipt(*payload.PublicationReceipt) {
			return invalidTransition("publication reconciliation receipt is invalid")
		}
		receipt := *payload.PublicationReceipt
		publication.Status = PublicationCompleted
		publication.Receipt = &receipt
		publication.LastError = ""
	} else {
		if strings.TrimSpace(payload.Evidence.Diagnostic) == "" {
			return invalidTransition("not-published diagnostic is required")
		}
		if err := validateDiagnostic(payload.Evidence.Diagnostic); err != nil {
			return invalidTransition("reconciliation diagnostic: %v", err)
		}
		publication.Status = PublicationFailed
		publication.LastError = boundedDiagnostic(payload.Evidence.Diagnostic)
		publication.Receipt = nil
	}
	snapshot.Publications[publication.IntentID] = publication
	snapshot.Control.Blocker = nil
	reduce(snapshot)
	return nil
}
