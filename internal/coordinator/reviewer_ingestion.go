package coordinator

import (
	"context"
	"errors"
	"fmt"

	contractv2 "thread-dock/internal/contract/v2"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
)

var (
	errMalformedReviewerArtifact = errors.New("malformed reviewer artifact")
	errReviewerEvidenceMismatch  = errors.New("reviewer evidence mismatch")
)

func (d *publicationDispatcher) blockActiveArtifact(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState) (CommandResult, error) {
	if invocation != nil && invocation.Role == string(runtimecontract.RoleReviewer) {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, errReviewerEvidenceMismatch)
	}
	return d.blockBuilderArtifact(ctx, snapshot, task, invocation, errors.New("active runtime returned an artifact before termination"))
}

// ingestReviewerArtifact is the only path that records Reviewer evidence. It
// is called after durable termination confirmation and never trusts provider
// prose for candidate identity.
func (d *publicationDispatcher) ingestReviewerArtifact(ctx context.Context, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, artifact *runtimecontract.ArtifactEnvelope) (CommandResult, error) {
	if artifact == nil {
		return CommandResult{Snapshot: snapshot}, nil
	}
	task, ok := snapshot.TaskStates[taskID]
	if !ok {
		return CommandResult{Snapshot: snapshot, Err: errReviewerEvidenceMismatch}, errReviewerEvidenceMismatch
	}
	invocation := task.Invocation
	if invocation == nil || invocation.InvocationID != statev2.InvocationID(artifact.RequestID) {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, errReviewerEvidenceMismatch)
	}
	_, validatedInvocation, err := d.validateRuntimeIdentity(snapshot, &publicationQueue{workID: snapshot.WorkID, owner: OwnerRecord{WorkID: snapshot.WorkID, OwnerID: d.ownerID, PID: d.pid, StartedAt: d.startedAt}}, runtimeKey{taskID: taskID, invocationID: invocation.InvocationID})
	if err != nil {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, errReviewerEvidenceMismatch)
	}
	invocation = validatedInvocation
	if task.Status != statev2.TaskTerminated || invocation.Role != string(runtimecontract.RoleReviewer) || !invocation.TerminationConfirmed || invocation.EndedAt == nil {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, errReviewerEvidenceMismatch)
	}
	runtimeInvocation, buildErr := d.buildRuntimeInvocation(ctx, snapshot, taskID, *invocation, *task.Worktree)
	if buildErr != nil {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: %v", errReviewerEvidenceMismatch, buildErr))
	}
	if err := runtimecontract.ValidateEnvelope(runtimeInvocation, *artifact); err != nil || artifact.Status != "success" {
		if err == nil {
			err = errors.New("reviewer artifact status is not success")
		}
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: %v", errMalformedReviewerArtifact, err))
	}
	result, decodeErr := runtimecontract.DecodeReviewerResult(artifact.Result)
	if decodeErr != nil {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: %v", errMalformedReviewerArtifact, decodeErr))
	}
	if result.ReviewedSHA != task.Candidate.CandidateSHA {
		return d.blockReviewerArtifact(ctx, snapshot, task, invocation, errReviewerEvidenceMismatch)
	}
	findings := make([]statev2.ReviewFinding, len(result.BlockingFindings))
	for i, finding := range result.BlockingFindings {
		findings[i] = statev2.ReviewFinding{Code: finding.Code, Severity: "blocking", Diagnostic: finding.Diagnostic}
	}
	observedAt := *invocation.EndedAt
	transition := runtimeTransitionAt(task, invocation, statev2.TaskRecordReview, "", &observedAt)
	transition.Review = &statev2.ReviewEvidence{
		ReviewerInvocationID: invocation.InvocationID,
		BuilderAttempt:       task.BuilderAttempt,
		CandidateSHA:         task.Candidate.CandidateSHA,
		ReviewSHA:            result.ReviewedSHA,
		Accepted:             result.Decision == "accept",
		Findings:             findings,
		ObservedAt:           observedAt,
	}
	request, requestErr := d.runtimeRequest(snapshot, transition)
	if requestErr != nil {
		return CommandResult{Snapshot: snapshot, Err: requestErr}, requestErr
	}
	updated, applyErr := d.state.Apply(ctx, request)
	if applyErr != nil {
		return CommandResult{Snapshot: snapshot, Err: applyErr}, applyErr
	}
	return CommandResult{Snapshot: updated}, nil
}

func (d *publicationDispatcher) blockReviewerArtifact(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState, cause error) (CommandResult, error) {
	if cause == nil {
		cause = errReviewerEvidenceMismatch
	}
	if invocation == nil {
		return CommandResult{Snapshot: snapshot, Err: cause}, cause
	}
	kind := statev2.BlockerKindEvidenceMismatch
	safeErr := errReviewerEvidenceMismatch
	if errors.Is(cause, errMalformedReviewerArtifact) {
		kind = statev2.BlockerKindMalformedArtifact
		safeErr = errMalformedReviewerArtifact
	}
	transition := runtimeTransitionAt(task, invocation, statev2.TaskNeedsOperatorAction, "reviewer artifact requires operator inspection", nil)
	transition.Blocker = &statev2.OperatorBlocker{Kind: kind, OperatorRef: string(d.ownerID), TaskID: task.TaskID, InvocationID: invocation.InvocationID, Diagnostic: "reviewer artifact did not match the current invocation"}
	request, err := d.runtimeRequest(snapshot, transition)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}, err
	}
	updated, err := d.state.Apply(ctx, request)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}, err
	}
	return CommandResult{Snapshot: updated, Err: safeErr}, safeErr
}
