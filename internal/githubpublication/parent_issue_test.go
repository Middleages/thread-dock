package githubpublication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/github"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

type publisherRemote struct {
	find                   func(context.Context, github.Repository, string) ([]github.Issue, error)
	create                 func(context.Context, github.Repository, contractv2.IssueDraft, string) (github.Issue, error)
	findCalls, createCalls int
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

func TestPublisherObserveClassifiesExactMismatchAndMultiplicityWithoutWrite(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	publication := publicationFor(snapshot, draft)
	hash, _ := issueDraftHash(draft)
	prefix := markerPrefix("work-1", "design")
	exact := prefix + hash + " -->"
	other := prefix + strings.Repeat("a", 64) + " -->"
	cases := []struct {
		name   string
		issues []github.Issue
		want   string
	}{
		{name: "exact", issues: []github.Issue{{Number: 7, NodeID: "node-7", HTMLURL: "https://ghes/issues/7", Body: "human prose\n\n" + exact}}, want: coordinator.PublicationObservationMatch},
		{name: "different payload", issues: []github.Issue{{Number: 8, NodeID: "node-8", HTMLURL: "https://ghes/issues/8", Body: "human prose\n" + other}}, want: coordinator.PublicationObservationUnknown},
		{name: "multiple", issues: []github.Issue{{Number: 7, NodeID: "node-7", HTMLURL: "https://ghes/issues/7", Body: exact}, {Number: 8, NodeID: "node-8", HTMLURL: "https://ghes/issues/8", Body: exact}}, want: coordinator.PublicationObservationUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remote := publisherRemote{find: func(context.Context, github.Repository, string) ([]github.Issue, error) { return tc.issues, nil }}
			publisher := NewPublisher(publisherState{snapshot: snapshot}, publisherProjects{project: publisherProject()}, remote, "ghes.example", func() time.Time { return time.Unix(10, 0).UTC() })
			observation, err := publisher.Observe(context.Background(), publication)
			if err != nil {
				t.Fatal(err)
			}
			if observation.State != tc.want {
				t.Fatalf("state=%q want=%q receipt=%+v", observation.State, tc.want, observation.Receipt)
			}
			if tc.name == "exact" && (observation.Receipt == nil || observation.Receipt.Number != 7 || observation.Receipt.NodeID != "node-7" || observation.Receipt.URL != "https://ghes/issues/7") {
				t.Fatalf("receipt=%+v", observation.Receipt)
			}
		})
	}
}

func TestPublisherPublishAdoptsOneLostResponseAndNeverPostsTwice(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	publication := publicationFor(snapshot, draft)
	hash, _ := issueDraftHash(draft)
	marker := markerPrefix("work-1", "design") + hash + " -->"
	findCalls := 0
	posts := 0
	remote := publisherRemote{find: func(context.Context, github.Repository, string) ([]github.Issue, error) {
		findCalls++
		if findCalls == 1 {
			return nil, nil
		}
		return []github.Issue{{Number: 19, NodeID: "node-19", HTMLURL: "https://ghes/issues/19", Body: "human prose\n\n" + marker}}, nil
	}, create: func(context.Context, github.Repository, contractv2.IssueDraft, string) (github.Issue, error) {
		posts++
		return github.Issue{}, errors.New("lost response")
	}}
	publisher := NewPublisher(publisherState{snapshot: snapshot}, publisherProjects{project: publisherProject()}, remote, "ghes.example", func() time.Time { return time.Unix(10, 0).UTC() })
	receipt, err := publisher.Publish(context.Background(), publication)
	if err != nil || receipt.Number != 19 || posts != 1 || findCalls != 2 {
		t.Fatalf("receipt=%+v err=%v posts=%d finds=%d", receipt, err, posts, findCalls)
	}
}

func TestPublisherPublishAmbiguousLossReturnsErrorWithoutSecondPost(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	publication := publicationFor(snapshot, draft)
	for _, issues := range [][]github.Issue{nil, {{Number: 1, NodeID: "n1", HTMLURL: "https://ghes/1", Body: "prefix mismatch"}, {Number: 2, NodeID: "n2", HTMLURL: "https://ghes/2", Body: "prefix mismatch"}}} {
		posts, finds := 0, 0
		remote := publisherRemote{find: func(context.Context, github.Repository, string) ([]github.Issue, error) {
			finds++
			if finds == 1 {
				return nil, nil
			}
			return issues, nil
		}, create: func(context.Context, github.Repository, contractv2.IssueDraft, string) (github.Issue, error) {
			posts++
			return github.Issue{}, errors.New("lost")
		}}
		publisher := NewPublisher(publisherState{snapshot: snapshot}, publisherProjects{project: publisherProject()}, remote, "ghes.example", time.Now)
		if _, err := publisher.Publish(context.Background(), publication); err == nil || posts != 1 || finds != 2 {
			t.Fatalf("issues=%v err=%v posts=%d finds=%d", issues, err, posts, finds)
		}
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
		current := s.snapshot.Publications[request.Publication.IntentID]
		if request.Publication.Action == statev2.PublicationBegin {
			current = statev2.PublicationState{IntentID: request.Publication.IntentID, Key: request.Publication.Key, Generation: request.Publication.Generation, Kind: request.Publication.Kind, Status: statev2.PublicationPending, PayloadHash: request.Publication.PayloadHash, PayloadRef: request.Publication.PayloadRef, Target: *request.Publication.Target, Attempts: 1, CompletionRequired: *request.Publication.CompletionRequired}
		} else if request.Publication.Action == statev2.PublicationComplete {
			current.Status = statev2.PublicationCompleted
			current.Receipt = request.Publication.Receipt
		} else if request.Publication.Action == statev2.PublicationActionConflict {
			current.Status = statev2.PublicationConflict
			current.LastError = request.Publication.Diagnostic
			s.snapshot.Control.Blocker = request.Publication.Blocker
		}
		s.snapshot.Publications = map[statev2.PublicationIntentID]statev2.PublicationState{request.Publication.IntentID: current}
	}
	return s.snapshot, nil
}

type serviceObserver struct {
	observation  coordinator.PublicationObservation
	err          error
	publishCalls *int
}

func (o serviceObserver) Observe(context.Context, statev2.PublicationState) (coordinator.PublicationObservation, error) {
	return o.observation, o.err
}
func (o serviceObserver) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	if o.publishCalls != nil {
		*o.publishCalls = *o.publishCalls + 1
	}
	return statev2.PublicationReceipt{}, errors.New("unexpected publish")
}

type serviceDispatcher struct {
	snapshot statev2.WorkSnapshot
	workID   contractv2.WorkID
	intentID statev2.PublicationIntentID
	calls    int
}

func (d *serviceDispatcher) SubmitPublication(_ context.Context, workID contractv2.WorkID, intentID statev2.PublicationIntentID) <-chan coordinator.CommandResult {
	d.calls++
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

func TestServiceReplayCompletedAndConflictingDraftDoNotDispatch(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	snapshot.Revision = 4
	hash, _ := issueDraftHash(draft)
	key := statev2.PublicationKey("parent_issue:work-1:design")
	snapshot.Publications["parent-issue:request-1"] = statev2.PublicationState{IntentID: "parent-issue:request-1", Key: key, Generation: 1, Kind: statev2.PublicationParentIssue, Status: statev2.PublicationCompleted, PayloadHash: hash, PayloadRef: payloadRef("work-1", "design"), Target: statev2.PublicationTarget{Host: "ghes.example", Repository: "primary", Resource: "acme/app", Key: key}, Attempts: 1, CompletionRequired: true, Receipt: &statev2.PublicationReceipt{Number: 9, NodeID: "node-9", URL: "https://ghes/9", PublishedAt: time.Unix(10, 0).UTC()}}
	state := &serviceState{snapshot: snapshot}
	dispatcher := &serviceDispatcher{}
	service := NewService(state, publisherProjects{project: publisherProject()}, dispatcher, nil, "operator")
	got, err := service.PublishParentIssue(context.Background(), snapshot, "design", "request-1")
	if err != nil || got.Publications["parent-issue:request-1"].Receipt.Number != 9 || dispatcher.intentID != "" {
		t.Fatalf("got=%+v err=%v dispatcher=%q", got.Publications["parent-issue:request-1"], err, dispatcher.intentID)
	}
	other := draft
	other.Key = "other"
	snapshot.Contract.IssueDrafts = append(snapshot.Contract.IssueDrafts, other)
	state.snapshot = snapshot
	if _, err := service.PublishParentIssue(context.Background(), snapshot, "other", "request-1"); err == nil {
		t.Fatal("expected request conflict")
	} else {
		var conflict *statev2.RequestConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestServiceInterruptedPendingRecoveryPersistsMatchOrConflictWithoutDispatch(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	base := publisherSnapshot(draft)
	base.Revision = 4
	pending := publicationFor(base, draft)
	base.Publications[pending.IntentID] = pending
	for _, tc := range []struct {
		name        string
		observation coordinator.PublicationObservation
		err         error
		wantStatus  statev2.PublicationStatus
		wantBlocker bool
	}{
		{name: "match", observation: coordinator.PublicationObservation{State: coordinator.PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{Number: 11, NodeID: "node-11", URL: "https://ghes/11", PublishedAt: time.Unix(10, 0).UTC()}}, wantStatus: statev2.PublicationCompleted},
		{name: "invalid receipt", observation: coordinator.PublicationObservation{State: coordinator.PublicationObservationMatch, Diagnostic: "provider-secret", Receipt: &statev2.PublicationReceipt{Number: 11, NodeID: "receipt-secret", PublishedAt: time.Unix(10, 0).UTC()}}, wantStatus: statev2.PublicationConflict, wantBlocker: true},
		{name: "absent", observation: coordinator.PublicationObservation{State: coordinator.PublicationObservationAbsent}, wantStatus: statev2.PublicationConflict, wantBlocker: true},
		{name: "unknown", observation: coordinator.PublicationObservation{State: coordinator.PublicationObservationUnknown}, wantStatus: statev2.PublicationConflict, wantBlocker: true},
		{name: "error", err: errors.New("observation failed"), wantStatus: statev2.PublicationConflict, wantBlocker: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed := base
			seed.Publications = map[statev2.PublicationIntentID]statev2.PublicationState{pending.IntentID: pending}
			state := &serviceState{snapshot: seed}
			dispatcher := &serviceDispatcher{}
			publishCalls := 0
			service := NewService(state, publisherProjects{project: publisherProject()}, dispatcher, serviceObserver{observation: tc.observation, err: tc.err, publishCalls: &publishCalls}, "operator")
			got, err := service.PublishParentIssue(context.Background(), seed, "design", "req-1")
			if err != nil || got.Publications[pending.IntentID].Status != tc.wantStatus || (got.Control.Blocker != nil) != tc.wantBlocker {
				t.Fatalf("got=%+v blocker=%+v err=%v", got.Publications[pending.IntentID], got.Control.Blocker, err)
			}
			if tc.name == "invalid receipt" {
				publication := got.Publications[pending.IntentID]
				blocker := got.Control.Blocker
				if blocker == nil || blocker.Kind != statev2.BlockerKindPublicationConflict || blocker.IntentID != pending.IntentID || blocker.OperatorRef != "operator" {
					t.Fatalf("blocker=%+v", blocker)
				}
				for name, diagnostic := range map[string]string{"blocker": blocker.Diagnostic, "lastError": publication.LastError} {
					if strings.TrimSpace(diagnostic) == "" || len([]byte(diagnostic)) > statev2.MaxDiagnosticBytes || strings.Contains(diagnostic, "provider-secret") || strings.Contains(diagnostic, "receipt-secret") {
						t.Fatalf("unsafe %s diagnostic=%q", name, diagnostic)
					}
				}
				if dispatcher.calls != 0 || publishCalls != 0 {
					t.Fatalf("dispatch calls=%d publish calls=%d", dispatcher.calls, publishCalls)
				}
			}
		})
	}
}

func TestServiceRejectsInvalidInputsBeforeDispatchOrProvider(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	base := publisherSnapshot(draft)
	base.Revision = 4
	cases := []struct {
		name   string
		mutate func(*statev2.WorkSnapshot, *statev2.WorkSnapshot, *registry.Project)
		key    string
	}{
		{name: "stale revision", mutate: func(supplied, _ *statev2.WorkSnapshot, _ *registry.Project) { supplied.Revision = 3 }, key: "design"},
		{name: "mismatched work identity", mutate: func(supplied, _ *statev2.WorkSnapshot, _ *registry.Project) { supplied.WorkID = "other" }, key: "design"},
		{name: "unapproved", mutate: func(_, authoritative *statev2.WorkSnapshot, _ *registry.Project) {
			authoritative.Control = statev2.WorkControl{}
		}, key: "design"},
		{name: "missing draft", mutate: func(_, _ *statev2.WorkSnapshot, _ *registry.Project) {}, key: "missing"},
		{name: "nonunique draft", mutate: func(_, authoritative *statev2.WorkSnapshot, _ *registry.Project) {
			authoritative.Contract.IssueDrafts = append(authoritative.Contract.IssueDrafts, draft)
		}, key: "design"},
		{name: "non-primary draft", mutate: func(_, authoritative *statev2.WorkSnapshot, _ *registry.Project) {
			authoritative.Contract.IssueDrafts[0].RepoKey = "secondary"
		}, key: "design"},
		{name: "missing repository plan", mutate: func(_, authoritative *statev2.WorkSnapshot, _ *registry.Project) {
			authoritative.Contract.RepositoryPlans = nil
		}, key: "design"},
		{name: "missing registration", mutate: func(_, _ *statev2.WorkSnapshot, project *registry.Project) { project.Repositories = nil }, key: "design"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authoritative := base
			supplied := base
			project := publisherProject()
			tc.mutate(&supplied, &authoritative, &project)
			state := &serviceState{snapshot: authoritative}
			dispatcher := &serviceDispatcher{}
			service := NewService(state, publisherProjects{project: project}, dispatcher, nil, "operator")
			if _, err := service.PublishParentIssue(context.Background(), supplied, tc.key, "request-reject"); err == nil {
				t.Fatal("expected rejection")
			}
			if dispatcher.intentID != "" {
				t.Fatalf("unexpected dispatch %q", dispatcher.intentID)
			}
		})
	}
}

func TestPublisherRejectsConfiguredHostOrResourceMismatchBeforeRemote(t *testing.T) {
	draft := contractv2.IssueDraft{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}
	snapshot := publisherSnapshot(draft)
	publication := publicationFor(snapshot, draft)
	finds := 0
	remote := publisherRemote{find: func(context.Context, github.Repository, string) ([]github.Issue, error) { finds++; return nil, nil }}
	for _, mutate := range []func(*statev2.PublicationState){
		func(p *statev2.PublicationState) { p.Target.Host = "other.example" },
		func(p *statev2.PublicationState) { p.Target.Resource = "acme/other" },
	} {
		candidate := publication
		mutate(&candidate)
		publisher := NewPublisher(publisherState{snapshot: snapshot}, publisherProjects{project: publisherProject()}, remote, "ghes.example", time.Now)
		if _, err := publisher.Observe(context.Background(), candidate); err == nil {
			t.Fatal("expected target rejection")
		}
	}
	if finds != 0 {
		t.Fatalf("remote observations=%d", finds)
	}
}

func TestIntegrationRealStoresAndCoordinatorPersistPendingBeforeFakePostAndReceiptAfterRelease(t *testing.T) {
	root := t.TempDir()
	workStore := statev2.NewStore(filepath.Join(root, "state"))
	projectStore := registry.NewStore(filepath.Join(root, "registry"))
	contract := integrationContract()
	var canonical bytes.Buffer
	if err := contractv2.Write(&canonical, contract); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: contract.ProjectID, WorkID: contract.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, Contract: contract, ContractHash: hex.EncodeToString(sum[:]), SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, Control: statev2.WorkControl{}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	if _, err := workStore.CreatePlan(context.Background(), snapshot, "create-work", "payload-work"); err != nil {
		t.Fatal(err)
	}
	project := registry.Project{ProjectID: contract.ProjectID, Name: "Project", PrimaryRepoKey: "primary", Repositories: map[contractv2.RepoKey]contractv2.RepositoryIdentity{"primary": {Host: "127.0.0.1", Owner: "acme", Name: "app", DefaultBranch: "main"}}}
	before, err := workStore.Load(context.Background(), snapshot.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	approve := statev2.TransitionRequest{WorkID: before.WorkID, ExpectedRevision: before.Revision, RequestID: "approve-work", Work: &statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: "operator", ContractHash: before.ContractHash}}
	approve.PayloadHash, err = statev2.TransitionPayloadHash(approve)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := workStore.Apply(context.Background(), approve)
	if err != nil {
		t.Fatal(err)
	}
	postEntered := make(chan struct{})
	releasePost := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]github.Issue{})
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
			return
		}
		var body struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode post: %v", err)
			return
		}
		close(postEntered)
		<-releasePost
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(github.Issue{Number: 31, NodeID: "node-31", Title: body.Title, Body: body.Body, HTMLURL: "http://127.0.0.1/issues/31"})
	}))
	defer server.Close()
	realProject := project
	realProject.Repositories["primary"] = contractv2.RepositoryIdentity{Host: server.URL, Owner: "acme", Name: "app", DefaultBranch: "main"}
	if _, err := projectStore.Create(context.Background(), realProject, 0, "create-project-server", "payload-project-server"); err != nil {
		t.Fatal(err)
	}
	publisher := NewPublisher(workStore, projectStore, github.NewRESTClient(server.URL, "token", "2022-11-28", server.Client()), server.URL, func() time.Time { return time.Unix(10, 0).UTC() })
	coordinatorInstance := coordinator.NewCoordinator(workStore, nil, publisher, nil, coordinator.NewOwnerLocker(filepath.Join(root, "owners")), "owner", os.Getpid(), time.Now().UTC())
	if _, err := coordinatorInstance.Activate(context.Background(), approved.WorkID); err != nil {
		t.Fatal(err)
	}
	service := NewService(workStore, projectStore, coordinatorInstance, publisher, "operator")
	resultCh := make(chan struct {
		snapshot statev2.WorkSnapshot
		err      error
	}, 1)
	go func() {
		got, callErr := service.PublishParentIssue(context.Background(), approved, "design", "request-integration")
		resultCh <- struct {
			snapshot statev2.WorkSnapshot
			err      error
		}{got, callErr}
	}()
	select {
	case <-postEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("fake POST was not reached")
	}
	pending, err := workStore.Load(context.Background(), approved.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	intent := pending.Publications["parent-issue:request-integration"]
	if intent.Status != statev2.PublicationPending || intent.PayloadRef != "contract:v2:work=work-1:parent-issue=design" {
		t.Fatalf("pending intent=%+v", intent)
	}
	close(releasePost)
	result := <-resultCh
	if result.err != nil || result.snapshot.Publications[intent.IntentID].Status != statev2.PublicationCompleted {
		t.Fatalf("result=%+v err=%v", result.snapshot.Publications[intent.IntentID], result.err)
	}
	persisted, err := workStore.Load(context.Background(), approved.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	receipt := persisted.Publications[intent.IntentID].Receipt
	if receipt == nil || receipt.Number != 31 || receipt.NodeID != "node-31" || receipt.URL != "http://127.0.0.1/issues/31" {
		t.Fatalf("receipt=%+v", receipt)
	}
	_ = coordinatorInstance.Close(context.Background())
}

func integrationContract() contractv2.WorkItemContract {
	return contractv2.WorkItemContract{Version: 2, WorkID: "work-1", ProjectID: "project-1", Revision: 1, Request: "publish", AcceptanceCriteria: []string{"works"}, RepositoryPlans: []contractv2.RepositoryPlan{{RepoKey: "primary", BaseSHA: strings.Repeat("0", 40), TargetBranch: "main"}}, Tasks: []contractv2.Task{{TaskID: "task-1", RepoKey: "primary", AllowedPaths: []string{"internal"}, AcceptanceCriteria: []string{"works"}}}, IssueDrafts: []contractv2.IssueDraft{{Key: "design", Title: "Design", Body: "Approved body", RepoKey: "primary"}}, Documentation: contractv2.DocumentationPlan{Required: false, Reason: "none"}, ExecutionProfiles: contractv2.ExecutionProfiles{Builder: "builder", Reviewer: "reviewer", Documenter: "documenter"}}
}
