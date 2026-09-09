package githubpublication

import (
	"context"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/github"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

type publisherRemote struct {
	find   func(context.Context, github.Repository, string) ([]github.Issue, error)
	create func(context.Context, github.Repository, contractv2.IssueDraft, string) (github.Issue, error)
}

func (r publisherRemote) FindParentIssueMarkers(ctx context.Context, repo github.Repository, prefix string) ([]github.Issue, error) {
	return r.find(ctx, repo, prefix)
}
func (r publisherRemote) CreateParentIssue(ctx context.Context, repo github.Repository, draft contractv2.IssueDraft, marker string) (github.Issue, error) {
	return r.create(ctx, repo, draft, marker)
}

type publisherState struct{ snapshot statev2.WorkSnapshot }

func (s publisherState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}
func (s publisherState) Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}

type publisherProjects struct{ project registry.Project }

func (p publisherProjects) Load(context.Context, contractv2.ProjectID) (registry.Project, error) {
	return p.project, nil
}

func TestPublisherObserveAbsentUsesCanonicalPayloadAndTarget(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	var gotRepo github.Repository
	var gotPrefix string
	remote := publisherRemote{find: func(_ context.Context, repo github.Repository, prefix string) ([]github.Issue, error) {
		gotRepo, gotPrefix = repo, prefix
		return nil, nil
	}}
	publisher := NewPublisher(publisherState{snapshot: snapshot}, publisherProjects{project: publisherProject()}, remote, "ghes.example", func() time.Time { return time.Unix(10, 0).UTC() })
	observation, err := publisher.Observe(context.Background(), publicationFor(snapshot, draft))
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != "absent" {
		t.Fatalf("observation=%+v", observation)
	}
	if gotRepo != (github.Repository{Owner: "acme", Name: "app"}) || gotPrefix != "<!-- threaddock:v2:parent_issue:work=work-1:draft=design:sha256=" {
		t.Fatalf("repo=%+v prefix=%q", gotRepo, gotPrefix)
	}
}

func publisherProject() registry.Project {
	return registry.Project{ProjectID: "project-1", Name: "Project", PrimaryRepoKey: "primary", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"primary": {Host: "ghes.example", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
}

func publisherSnapshot(draft contractv2.IssueDraft) statev2.WorkSnapshot {
	contract := contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "primary"}}, IssueDrafts: []contractv2.IssueDraft{draft}}
	return statev2.WorkSnapshot{WorkID: "work-1", ProjectID: "project-1", Contract: contract, ContractHash: "contract-hash", Control: statev2.WorkControl{ApprovedContractHash: "contract-hash", ApprovalRef: "approval"}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
}

func publicationFor(snapshot statev2.WorkSnapshot, draft contractv2.IssueDraft) statev2.PublicationState {
	hash, _ := issueDraftHash(draft)
	key := statev2.PublicationKey("parent_issue:work-1:" + draft.Key)
	return statev2.PublicationState{IntentID: "parent-issue:req-1", Key: key, Generation: 1, Kind: statev2.PublicationParentIssue, Status: statev2.PublicationPending, PayloadHash: hash, PayloadRef: payloadRef(snapshot.WorkID, draft.Key), Target: statev2.PublicationTarget{Host: "ghes.example", Repository: "primary", Resource: "acme/app", Key: key}, CompletionRequired: true}
}

type serviceState struct {
	snapshot statev2.WorkSnapshot
	request  statev2.TransitionRequest
}

func (s *serviceState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}
func (s *serviceState) Apply(_ context.Context, request statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	s.request = request
	if request.Publication != nil {
		s.snapshot.Publications = map[statev2.PublicationIntentID]statev2.PublicationState{request.Publication.IntentID: {
			IntentID: request.Publication.IntentID, Key: request.Publication.Key, Generation: request.Publication.Generation, Kind: request.Publication.Kind, Status: statev2.PublicationPending, PayloadHash: request.Publication.PayloadHash, PayloadRef: request.Publication.PayloadRef, Target: *request.Publication.Target, Attempts: 1, CompletionRequired: *request.Publication.CompletionRequired,
		}}
	}
	return s.snapshot, nil
}

type serviceDispatcher struct {
	snapshot statev2.WorkSnapshot
	workID   contractv2.WorkID
	intentID statev2.PublicationIntentID
}

func (d *serviceDispatcher) SubmitPublication(_ context.Context, workID contractv2.WorkID, intentID statev2.PublicationIntentID) <-chan coordinator.CommandResult {
	d.workID, d.intentID = workID, intentID
	result := make(chan coordinator.CommandResult, 1)
	result <- coordinator.CommandResult{Snapshot: d.snapshot}
	return result
}

func TestServiceBeginsParentIssueWithFrozenIdentityBeforeDispatch(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	snapshot.Revision = 4
	state := &serviceState{snapshot: snapshot}
	dispatcher := &serviceDispatcher{}
	service := NewService(state, publisherProjects{project: publisherProject()}, dispatcher, nil, "operator")
	got, err := service.PublishParentIssue(context.Background(), snapshot, draft.Key, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.request.Publication == nil || state.request.Publication.Generation != 1 || state.request.Publication.Kind != statev2.PublicationParentIssue || state.request.Publication.CompletionRequired == nil || !*state.request.Publication.CompletionRequired {
		t.Fatalf("request=%+v", state.request.Publication)
	}
	if state.request.Publication.PayloadRef != "contract:v2:work=work-1:parent-issue=design" || state.request.Publication.Target.Resource != "acme/app" {
		t.Fatalf("request=%+v", state.request.Publication)
	}
	if dispatcher.workID != got.WorkID || dispatcher.intentID != "parent-issue:request-1" {
		t.Fatalf("dispatch work=%q intent=%q", dispatcher.workID, dispatcher.intentID)
	}
}
