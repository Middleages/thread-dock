package main

import (
	"context"
	"errors"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/githubpublication"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/workflow"
)

// productionParentIssuePublisher is the private CLI-only bridge to the
// provider-backed publication service. WorkflowService remains provider
// neutral and unchanged.
type productionParentIssuePublisher struct {
	*workflow.Service
	publication *githubpublication.Service
	dispatcher  coordinator.Dispatcher
}

func (s *productionParentIssuePublisher) PublishParentIssue(ctx context.Context, workID contractv2.WorkID, draftKey string, revision contractv2.Revision, requestID contractv2.RequestID) (statev2.WorkSnapshot, error) {
	if s == nil || s.Service == nil || s.publication == nil || s.dispatcher == nil {
		return statev2.WorkSnapshot{}, errors.New("parent issue publication is not configured")
	}
	authoritative, err := s.Service.Status(ctx, workID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	// The merged service performs the authoritative revision check. Replacing
	// only the caller-supplied revision preserves that exact check while the
	// embedded workflow service supplies the authoritative identity/contract.
	supplied := authoritative
	supplied.Revision = revision
	snapshot, publishErr := s.publication.PublishParentIssue(ctx, supplied, draftKey, requestID)
	closeErr := s.dispatcher.Close(ctx)
	if publishErr != nil {
		return snapshot, publishErr
	}
	if closeErr != nil {
		return snapshot, closeErr
	}
	return snapshot, nil
}
