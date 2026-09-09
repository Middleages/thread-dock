package githubpublication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/github"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

const (
	payloadRefPrefix              = "contract:v2:work="
	markerPrefixHead              = "<!-- threaddock:v2:parent_issue:work="
	publicationConflictDiagnostic = "parent issue publication outcome is ambiguous; operator reconciliation required"
)

type ProjectStore interface {
	Load(context.Context, contractv2.ProjectID) (registry.Project, error)
}

type WorkState interface {
	Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error)
	Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error)
}

type Remote interface {
	FindParentIssueMarkers(context.Context, github.Repository, string) ([]github.Issue, error)
	CreateParentIssue(context.Context, github.Repository, contractv2.IssueDraft, string) (github.Issue, error)
}

type Dispatcher interface {
	SubmitPublication(context.Context, contractv2.WorkID, statev2.PublicationIntentID) <-chan coordinator.CommandResult
}

type Publisher struct {
	state          WorkState
	projects       ProjectStore
	remote         Remote
	configuredHost string
	now            func() time.Time
}

func NewPublisher(state WorkState, projects ProjectStore, remote Remote, configuredHost string, now func() time.Time) *Publisher {
	if now == nil {
		now = time.Now
	}
	return &Publisher{state: state, projects: projects, remote: remote, configuredHost: configuredHost, now: now}
}

func (p *Publisher) Observe(ctx context.Context, publication statev2.PublicationState) (coordinator.PublicationObservation, error) {
	resolved, err := p.resolve(ctx, publication)
	if err != nil {
		return coordinator.PublicationObservation{}, err
	}
	issues, err := p.remote.FindParentIssueMarkers(ctx, resolved.repo, resolved.markerPrefix)
	if err != nil {
		return coordinator.PublicationObservation{}, err
	}
	if len(issues) == 0 {
		return coordinator.PublicationObservation{State: coordinator.PublicationObservationAbsent}, nil
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Body, resolved.marker) {
		return coordinator.PublicationObservation{State: coordinator.PublicationObservationUnknown, Diagnostic: publicationConflictDiagnostic}, nil
	}
	receipt, err := p.receiptFromIssue(issues[0])
	if err != nil {
		return coordinator.PublicationObservation{State: coordinator.PublicationObservationUnknown, Diagnostic: publicationConflictDiagnostic}, nil
	}
	return coordinator.PublicationObservation{State: coordinator.PublicationObservationMatch, Receipt: &receipt}, nil
}

func (p *Publisher) Publish(ctx context.Context, publication statev2.PublicationState) (statev2.PublicationReceipt, error) {
	resolved, err := p.resolve(ctx, publication)
	if err != nil {
		return statev2.PublicationReceipt{}, err
	}
	observation, err := p.Observe(ctx, publication)
	if err != nil {
		return statev2.PublicationReceipt{}, err
	}
	if observation.State != coordinator.PublicationObservationAbsent || observation.Receipt != nil {
		return statev2.PublicationReceipt{}, errors.New("parent issue publication requires a proven absent marker")
	}
	issue, postErr := p.remote.CreateParentIssue(ctx, resolved.repo, resolved.draft, resolved.marker)
	if postErr == nil {
		receipt, responseErr := p.receiptFromIssue(issue)
		if responseErr == nil {
			return receipt, nil
		}
	}
	// A lost response or an invalid response is reconciled by one exact scan;
	// neither case permits a second POST.
	recovered, observeErr := p.Observe(ctx, publication)
	if observeErr == nil && recovered.State == coordinator.PublicationObservationMatch && recovered.Receipt != nil {
		return *recovered.Receipt, nil
	}
	return statev2.PublicationReceipt{}, errors.New(publicationConflictDiagnostic)
}

type resolvedPublication struct {
	snapshot     statev2.WorkSnapshot
	draft        contractv2.IssueDraft
	repo         github.Repository
	markerPrefix string
	marker       string
}

func (p *Publisher) resolve(ctx context.Context, publication statev2.PublicationState) (resolvedPublication, error) {
	workID, draftKey, err := parsePayloadRef(publication.PayloadRef)
	if err != nil {
		return resolvedPublication{}, err
	}
	if publication.Kind != statev2.PublicationParentIssue || publication.Key != statev2.PublicationKey("parent_issue:"+string(workID)+":"+draftKey) || publication.Target.Key != publication.Key || publication.Target.Host != p.configuredHost {
		return resolvedPublication{}, errors.New("parent issue publication identity is invalid")
	}
	snapshot, err := p.state.Load(ctx, workID)
	if err != nil {
		return resolvedPublication{}, err
	}
	if snapshot.WorkID != workID || snapshot.Contract.WorkID != workID || !approved(snapshot) {
		return resolvedPublication{}, errors.New("parent issue publication requires the approved durable contract")
	}
	var draft contractv2.IssueDraft
	count := 0
	for _, candidate := range snapshot.Contract.IssueDrafts {
		if candidate.Key == draftKey {
			draft = candidate
			count++
		}
	}
	if count != 1 {
		return resolvedPublication{}, errors.New("parent issue publication draft key is missing or non-unique")
	}
	project, err := p.projects.Load(ctx, snapshot.ProjectID)
	if err != nil {
		return resolvedPublication{}, err
	}
	if project.PrimaryRepoKey == "" || draft.RepoKey != project.PrimaryRepoKey {
		return resolvedPublication{}, errors.New("parent issue draft must target the project primary repository")
	}
	if !repositoryPlanExists(snapshot.Contract, project.PrimaryRepoKey) {
		return resolvedPublication{}, errors.New("parent issue primary repository plan is missing")
	}
	repoIdentity, ok := project.Repositories[project.PrimaryRepoKey]
	if !ok || strings.TrimSpace(repoIdentity.Host) == "" || strings.TrimSpace(repoIdentity.Owner) == "" || strings.TrimSpace(repoIdentity.Name) == "" {
		return resolvedPublication{}, errors.New("parent issue primary repository is not registered")
	}
	if repoIdentity.Host != p.configuredHost || publication.Target.Repository != project.PrimaryRepoKey || publication.Target.Resource != repoIdentity.Owner+"/"+repoIdentity.Name || publication.Target.Base != "" {
		return resolvedPublication{}, errors.New("parent issue publication target does not match configured repository")
	}
	hash, err := issueDraftHash(draft)
	if err != nil || hash != publication.PayloadHash {
		return resolvedPublication{}, errors.New("parent issue publication payload hash does not match the approved draft")
	}
	prefix := markerPrefix(string(workID), draftKey)
	return resolvedPublication{snapshot: snapshot, draft: draft, repo: github.Repository{Owner: repoIdentity.Owner, Name: repoIdentity.Name}, markerPrefix: prefix, marker: prefix + hash + " -->"}, nil
}

func (p *Publisher) receiptFromIssue(issue github.Issue) (statev2.PublicationReceipt, error) {
	if issue.Number <= 0 || !validRemoteText(issue.NodeID) || !validRemoteText(issue.HTMLURL) {
		return statev2.PublicationReceipt{}, errors.New("parent issue response did not contain a complete identity")
	}
	at := p.now()
	if at.IsZero() {
		return statev2.PublicationReceipt{}, errors.New("parent issue publication time is invalid")
	}
	return statev2.PublicationReceipt{Number: uint64(issue.Number), NodeID: issue.NodeID, URL: issue.HTMLURL, PublishedAt: at.UTC()}, nil
}

type Service struct {
	state       WorkState
	projects    ProjectStore
	dispatcher  Dispatcher
	observer    coordinator.Publisher
	operatorRef string
}

func NewService(state WorkState, projects ProjectStore, dispatcher Dispatcher, observer coordinator.Publisher, operatorRef string) *Service {
	return &Service{state: state, projects: projects, dispatcher: dispatcher, observer: observer, operatorRef: operatorRef}
}

func (s *Service) PublishParentIssue(ctx context.Context, supplied statev2.WorkSnapshot, draftKey string, requestID contractv2.RequestID) (statev2.WorkSnapshot, error) {
	authoritative, err := s.state.Load(ctx, supplied.WorkID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if supplied.WorkID == "" || supplied.ProjectID != authoritative.ProjectID || supplied.WorkID != authoritative.WorkID || supplied.Contract.WorkID != authoritative.Contract.WorkID {
		return statev2.WorkSnapshot{}, errors.New("supplied work identity does not match authoritative state")
	}
	if supplied.Revision != authoritative.Revision {
		return statev2.WorkSnapshot{}, &statev2.StaleRevisionError{CurrentRevision: authoritative.Revision, CurrentState: authoritative.State}
	}
	if !approved(authoritative) {
		return statev2.WorkSnapshot{}, errors.New("parent issue publication requires an approved contract")
	}
	if !validMarkerID(draftKey) || !validRequestID(requestID) {
		return statev2.WorkSnapshot{}, errors.New("parent issue publication identity is invalid")
	}
	project, err := s.projects.Load(ctx, authoritative.ProjectID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	draft, err := selectDraft(authoritative.Contract, draftKey)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if draft.RepoKey != project.PrimaryRepoKey || !repositoryPlanExists(authoritative.Contract, project.PrimaryRepoKey) {
		return statev2.WorkSnapshot{}, errors.New("parent issue draft must target a planned primary repository")
	}
	repoIdentity, ok := project.Repositories[project.PrimaryRepoKey]
	if !ok || strings.TrimSpace(repoIdentity.Host) == "" || strings.TrimSpace(repoIdentity.Owner) == "" || strings.TrimSpace(repoIdentity.Name) == "" {
		return statev2.WorkSnapshot{}, errors.New("parent issue primary repository is not registered")
	}
	hash, err := issueDraftHash(draft)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	key := statev2.PublicationKey("parent_issue:" + string(authoritative.WorkID) + ":" + draftKey)
	intentID := statev2.PublicationIntentID("parent-issue:" + string(requestID))
	ref := payloadRef(authoritative.WorkID, draftKey)
	target := statev2.PublicationTarget{Host: repoIdentity.Host, Repository: project.PrimaryRepoKey, Resource: repoIdentity.Owner + "/" + repoIdentity.Name, Key: key}
	if existing, exists := authoritative.Publications[intentID]; exists {
		if !samePublicationIdentity(existing, key, hash, ref, target) {
			return statev2.WorkSnapshot{}, &statev2.RequestConflictError{RequestID: requestID, ExistingHash: existing.PayloadHash, PayloadHash: hash}
		}
		if existing.Status == statev2.PublicationCompleted {
			return authoritative, nil
		}
		if existing.Status == statev2.PublicationPending {
			return s.recoverPending(ctx, authoritative, existing)
		}
		return s.submit(ctx, authoritative.WorkID, intentID, authoritative)
	}
	generation := nextGeneration(authoritative, key)
	if generation > 1 {
		prior := publicationAtGeneration(authoritative, key, generation-1)
		if prior.Status != statev2.PublicationCompleted {
			return statev2.WorkSnapshot{}, errors.New("parent issue publication lineage is not completed")
		}
	}
	completionRequired := true
	transition := statev2.PublicationTransition{Action: statev2.PublicationBegin, IntentID: intentID, Key: key, Generation: generation, Kind: statev2.PublicationParentIssue, PayloadHash: hash, PayloadRef: ref, Target: &target, CompletionRequired: &completionRequired}
	request := statev2.TransitionRequest{WorkID: authoritative.WorkID, ExpectedRevision: authoritative.Revision, RequestID: contractv2.RequestID(intentID), Publication: &transition}
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	started, err := s.state.Apply(ctx, request)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	return s.submit(ctx, started.WorkID, intentID, started)
}

func (s *Service) submit(ctx context.Context, workID contractv2.WorkID, intentID statev2.PublicationIntentID, fallback statev2.WorkSnapshot) (statev2.WorkSnapshot, error) {
	if s.dispatcher == nil {
		return fallback, errors.New("publication dispatcher is unavailable")
	}
	result := <-s.dispatcher.SubmitPublication(ctx, workID, intentID)
	if result.Snapshot.WorkID == "" {
		result.Snapshot = fallback
	}
	return result.Snapshot, result.Err
}

func (s *Service) recoverPending(ctx context.Context, snapshot statev2.WorkSnapshot, publication statev2.PublicationState) (statev2.WorkSnapshot, error) {
	if s.observer == nil {
		return snapshot, errors.New(publicationConflictDiagnostic)
	}
	observation, err := s.observer.Observe(ctx, publication)
	match := err == nil && observation.State == coordinator.PublicationObservationMatch && validReceipt(observation.Receipt)
	transition := statev2.PublicationTransition{IntentID: publication.IntentID, Key: publication.Key, Generation: publication.Generation, Kind: publication.Kind, PayloadHash: publication.PayloadHash, PayloadRef: publication.PayloadRef, Target: &publication.Target, CompletionRequired: boolPointer(publication.CompletionRequired)}
	if match {
		transition.Action = statev2.PublicationComplete
		transition.Receipt = observation.Receipt
	} else {
		transition.Action = statev2.PublicationActionConflict
		transition.Diagnostic = publicationConflictDiagnostic
		transition.Blocker = &statev2.OperatorBlocker{Kind: statev2.BlockerKindPublicationConflict, OperatorRef: s.operatorRef, IntentID: publication.IntentID, Diagnostic: publicationConflictDiagnostic}
	}
	request := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: recoveryRequestID(publication.IntentID), Publication: &transition}
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		return snapshot, err
	}
	return s.state.Apply(ctx, request)
}

func selectDraft(contract contractv2.WorkItemContract, key string) (contractv2.IssueDraft, error) {
	var draft contractv2.IssueDraft
	count := 0
	for _, candidate := range contract.IssueDrafts {
		if candidate.Key == key {
			draft, count = candidate, count+1
		}
	}
	if count != 1 {
		return contractv2.IssueDraft{}, errors.New("parent issue draft key is missing or non-unique")
	}
	return draft, nil
}

func issueDraftHash(draft contractv2.IssueDraft) (string, error) {
	data, err := json.Marshal(draft)
	if err != nil {
		return "", fmt.Errorf("encode parent issue draft: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func parsePayloadRef(ref string) (contractv2.WorkID, string, error) {
	if !strings.HasPrefix(ref, payloadRefPrefix) || strings.TrimSpace(ref) != ref {
		return "", "", errors.New("parent issue payload reference is invalid")
	}
	rest := strings.TrimPrefix(ref, payloadRefPrefix)
	separator := strings.Index(rest, ":parent-issue=")
	if separator <= 0 {
		return "", "", errors.New("parent issue payload reference is invalid")
	}
	workID, draftKey := rest[:separator], rest[separator+len(":parent-issue="):]
	if !validMarkerID(workID) || !validMarkerID(draftKey) || payloadRef(contractv2.WorkID(workID), draftKey) != ref {
		return "", "", errors.New("parent issue payload reference is invalid")
	}
	return contractv2.WorkID(workID), draftKey, nil
}

func payloadRef(workID contractv2.WorkID, draftKey string) string {
	return payloadRefPrefix + string(workID) + ":parent-issue=" + draftKey
}

func markerPrefix(workID, draftKey string) string {
	return markerPrefixHead + workID + ":draft=" + draftKey + ":sha256="
}

func approved(snapshot statev2.WorkSnapshot) bool {
	return snapshot.Control.ApprovedContractHash != "" && snapshot.Control.ApprovalRef != "" && snapshot.Control.ApprovedContractHash == snapshot.ContractHash
}

func repositoryPlanExists(contract contractv2.WorkItemContract, key contractv2.RepoKey) bool {
	for _, plan := range contract.RepositoryPlans {
		if plan.RepoKey == key {
			return true
		}
	}
	return false
}

func validStableID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\:\r\n\t ") && utf8.ValidString(value)
}

func validMarkerID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, b := range []byte(value) {
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '.' || b == '_' || b == '-' {
			continue
		}
		return false
	}
	return true
}

func validRequestID(value contractv2.RequestID) bool { return validStableID(string(value)) }

func samePublicationIdentity(existing statev2.PublicationState, key statev2.PublicationKey, hash, ref string, target statev2.PublicationTarget) bool {
	return existing.Key == key && existing.Kind == statev2.PublicationParentIssue && existing.PayloadHash == hash && existing.PayloadRef == ref && existing.Target == target && existing.CompletionRequired
}

func nextGeneration(snapshot statev2.WorkSnapshot, key statev2.PublicationKey) uint32 {
	var max uint32
	for _, candidate := range snapshot.Publications {
		if candidate.Key == key && candidate.Generation > max {
			max = candidate.Generation
		}
	}
	return max + 1
}

func publicationAtGeneration(snapshot statev2.WorkSnapshot, key statev2.PublicationKey, generation uint32) statev2.PublicationState {
	for _, candidate := range snapshot.Publications {
		if candidate.Key == key && candidate.Generation == generation {
			return candidate
		}
	}
	return statev2.PublicationState{}
}

func recoveryRequestID(intentID statev2.PublicationIntentID) contractv2.RequestID {
	return contractv2.RequestID("parent-issue-recovery:" + string(intentID))
}

func boolPointer(value bool) *bool { return &value }

func validReceipt(receipt *statev2.PublicationReceipt) bool {
	return receipt != nil && receipt.Number > 0 && receipt.NodeID != "" && receipt.URL != "" && !receipt.PublishedAt.IsZero() && receipt.PublishedAt.Location() == time.UTC
}

func validRemoteText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value)
}
