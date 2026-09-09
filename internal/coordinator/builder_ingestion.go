package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/pathscope"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

var (
	errMalformedBuilderArtifact = errors.New("malformed builder artifact")
	errBuilderEvidenceMismatch  = errors.New("builder evidence mismatch")
)

// ingestBuilderArtifact is the only path that records Builder candidate
// evidence. It expects termination to have been durably confirmed first and
// derives all Git evidence from the inspector rather than the agent result.
func (d *publicationDispatcher) ingestBuilderArtifact(ctx context.Context, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, artifact *runtimecontract.ArtifactEnvelope) (CommandResult, error) {
	if artifact == nil {
		return CommandResult{Snapshot: snapshot}, nil
	}
	task, invocation, err := d.validateRuntimeIdentity(snapshot, &publicationQueue{workID: snapshot.WorkID, owner: OwnerRecord{WorkID: snapshot.WorkID, OwnerID: d.ownerID, PID: d.pid, StartedAt: d.startedAt}}, runtimeKey{taskID: taskID, invocationID: statev2.InvocationID(artifact.RequestID)})
	if err != nil {
		// Late results from an old invocation are ignored. In particular they
		// must not turn a newer invocation into a false current success.
		return CommandResult{Snapshot: snapshot, Err: errBuilderEvidenceMismatch}, errBuilderEvidenceMismatch
	}
	if task.Status == statev2.TaskCandidateReady && task.Candidate != nil {
		return CommandResult{Snapshot: snapshot}, nil
	}
	if task.Status != statev2.TaskTerminated || invocation.Role != "builder" || !invocation.TerminationConfirmed || invocation.EndedAt == nil || task.Worktree == nil {
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, errBuilderEvidenceMismatch)
	}
	runtimeInvocation, buildErr := buildRuntimeInvocation(snapshot, taskID, *invocation, *task.Worktree)
	if buildErr != nil {
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, buildErr)
	}
	if err := runtimecontract.ValidateEnvelope(runtimeInvocation, *artifact); err != nil || artifact.Status != "success" {
		if err == nil {
			err = fmt.Errorf("builder artifact status is %q", artifact.Status)
		}
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: %v", errMalformedBuilderArtifact, err))
	}
	result, err := runtimecontract.DecodeBuilderResult(artifact.Result)
	if err != nil {
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: %v", errMalformedBuilderArtifact, err))
	}
	if snapshot.Control.Blocker != nil || snapshot.Control.PauseRequested || strings.TrimSpace(snapshot.Control.ApprovedContractHash) == "" || snapshot.Control.ApprovedContractHash != snapshot.ContractHash || strings.TrimSpace(snapshot.Control.ApprovalRef) == "" {
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: work approval or execution policy is not current", errBuilderEvidenceMismatch))
	}
	if d.inspector == nil || task.Worktree == nil {
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: candidate inspector is not configured", errBuilderEvidenceMismatch))
	}
	inspection, err := d.inspector.InspectCommit(ctx, task.Worktree.CanonicalPath, task.Worktree.BaseSHA, task.Worktree.Branch, result.CommitSHA)
	if err != nil || inspection.CommitSHA != result.CommitSHA || inspection.Branch != task.Worktree.Branch || inspection.TreeSHA == "" {
		if err == nil {
			err = errors.New("git inspection identity mismatch")
		}
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: %v", errBuilderEvidenceMismatch, err))
	}
	contractTask, found := contractTask(snapshot.Contract, taskID)
	if !found {
		return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: task is absent from contract", errBuilderEvidenceMismatch))
	}
	for _, changed := range inspection.ChangedFiles {
		if strings.Contains(changed, "\\") {
			return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: Git returned a non-canonical path", errBuilderEvidenceMismatch))
		}
		allowed := false
		for _, pattern := range contractTask.AllowedPaths {
			if pathscope.Contains(pattern, changed) {
				allowed = true
				break
			}
		}
		if !allowed {
			return d.blockBuilderArtifact(ctx, snapshot, task, invocation, fmt.Errorf("%w: changed path %q is outside allowed paths", errBuilderEvidenceMismatch, changed))
		}
	}
	transition := runtimeTransitionAt(task, invocation, statev2.TaskRecordCandidate, "", nil)
	transition.Candidate = &statev2.CandidateEvidence{BuilderAttempt: task.BuilderAttempt, CandidateSHA: inspection.CommitSHA, TreeSHA: inspection.TreeSHA, ChangedFiles: append([]string(nil), inspection.ChangedFiles...)}
	resultRequest, err := d.runtimeRequest(snapshot, transition)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}, err
	}
	updated, err := d.state.Apply(ctx, resultRequest)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}, err
	}
	return CommandResult{Snapshot: updated}, nil
}

func contractTask(contract contractv2.WorkItemContract, taskID contractv2.TaskID) (contractv2.Task, bool) {
	for _, task := range contract.Tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return contractv2.Task{}, false
}

func (d *publicationDispatcher) blockBuilderArtifact(ctx context.Context, snapshot statev2.WorkSnapshot, task statev2.TaskExecutionState, invocation *statev2.InvocationState, cause error) (CommandResult, error) {
	if invocation == nil {
		return CommandResult{Snapshot: snapshot, Err: cause}, cause
	}
	diagnostic := cause.Error()
	if len([]byte(diagnostic)) > statev2.MaxDiagnosticBytes {
		diagnostic = string([]byte(diagnostic)[:statev2.MaxDiagnosticBytes])
	}
	kind := statev2.BlockerKindEvidenceMismatch
	if errors.Is(cause, errMalformedBuilderArtifact) {
		kind = statev2.BlockerKindMalformedArtifact
	}
	transition := runtimeTransitionAt(task, invocation, statev2.TaskNeedsOperatorAction, diagnostic, nil)
	transition.Blocker = &statev2.OperatorBlocker{Kind: kind, OperatorRef: string(d.ownerID), TaskID: task.TaskID, InvocationID: invocation.InvocationID, Diagnostic: diagnostic}
	request, err := d.runtimeRequest(snapshot, transition)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}, err
	}
	updated, err := d.state.Apply(ctx, request)
	if err != nil {
		return CommandResult{Snapshot: snapshot, Err: err}, err
	}
	return CommandResult{Snapshot: updated, Err: cause}, cause
}

// Keep the worktree package in this file's dependency graph as part of the
// fixed CandidateInspector contract, without introducing another inspector.
var _ CandidateInspector = (*worktree.Git)(nil)
