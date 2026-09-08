package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

func TestCoordinatorReconcileReservedFalseProvesNotStartedWithoutRuntimeIO(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcileSnapshot(statev2.TaskInvocationReserved)}
	lease := &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}
	locker := &reconcileLocker{lease: lease}
	rt := &reconcileTestRuntime{}
	c := NewCoordinator(st, rt, nil, locker, "owner-1", 41, reconcileAt)

	got, err := c.Reconcile(context.Background(), "work-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != st.snapshot.State || got.NextAction != st.snapshot.NextAction {
		t.Fatalf("result projection = %#v snapshot = %#v", got, st.snapshot)
	}
	if rt.observes != 0 || rt.launches != 0 || rt.terminates != 0 {
		t.Fatalf("runtime calls = observe %d launch %d terminate %d", rt.observes, rt.launches, rt.terminates)
	}
	if st.taskActions != 1 || st.lastTaskAction != statev2.TaskReconcileNotStarted {
		t.Fatalf("task actions = %d/%q", st.taskActions, st.lastTaskAction)
	}
	if locker.acquires != 1 || lease.releases != 1 {
		t.Fatalf("owner calls = acquire %d release %d", locker.acquires, lease.releases)
	}
}

func TestCoordinatorReconcileLaunchRequestedObservesAndSettlesExactIdentity(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcileSnapshot(statev2.TaskInvocationReserved)}
	task := st.snapshot.TaskStates["task-1"]
	inv := task.Invocation
	inv.LaunchRequested = true
	task.Invocation = inv
	st.snapshot.TaskStates["task-1"] = task
	rt := &reconcileTestRuntime{observation: RuntimeObservation{State: RuntimeObservationActive, ProviderIdentity: "provider-1"}}
	c := NewCoordinator(st, rt, nil, &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}, "owner-1", 41, reconcileAt)

	if _, err := c.Reconcile(context.Background(), "work-1"); err != nil {
		t.Fatal(err)
	}
	if rt.launches != 0 || rt.observes != 1 {
		t.Fatalf("runtime calls = launch %d observe %d", rt.launches, rt.observes)
	}
	if st.lastTaskAction != statev2.TaskMarkRunning {
		t.Fatalf("last task action = %q", st.lastTaskAction)
	}
}

func TestCoordinatorReconcilePublicationMatchAdoptsReceipt(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcilePublicationSnapshot()}
	pub := &reconcileTestPublisher{observation: PublicationObservation{State: PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: reconcileAt}}}
	c := NewCoordinator(st, nil, pub, &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}, "owner-1", 41, reconcileAt)

	result, err := c.Reconcile(context.Background(), "work-1")
	if err != nil {
		t.Fatal(err)
	}
	if pub.observes != 1 || pub.publishes != 0 {
		t.Fatalf("publisher calls = observe %d publish %d", pub.observes, pub.publishes)
	}
	if result.State != st.snapshot.State || st.lastPublicationAction != statev2.PublicationComplete {
		t.Fatalf("result/state = %#v/%#v action=%q", result, st.snapshot, st.lastPublicationAction)
	}
}

func TestCoordinatorRunningUnknownObservationsNeverTerminate(t *testing.T) {
	for _, observation := range []RuntimeObservation{
		{State: RuntimeObservationUnknown},
		{State: RuntimeObservationNotStarted},
		{},
		{State: RuntimeObservationActive, ProviderIdentity: "provider-other"},
	} {
		st := &reconcileTestState{snapshot: reconcileSnapshot(statev2.TaskRunning)}
		task := st.snapshot.TaskStates["task-1"]
		task.Invocation.LaunchRequested = true
		task.Invocation.ProviderIdentity = "provider-1"
		st.snapshot.TaskStates["task-1"] = task
		rt := &reconcileTestRuntime{observation: observation}
		c := NewCoordinator(st, rt, nil, &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}, "owner-1", 41, reconcileAt)
		if _, err := c.Reconcile(context.Background(), "work-1"); err == nil {
			t.Fatalf("observation %#v unexpectedly succeeded", observation)
		}
		if rt.terminates != 0 || st.lastTaskAction != statev2.TaskNeedsOperatorAction || st.snapshot.State != statev2.StateNeedsOperator {
			t.Fatalf("observation %#v terminate=%d action=%q state=%q", observation, rt.terminates, st.lastTaskAction, st.snapshot.State)
		}
		blocker := st.snapshot.Control.Blocker
		if blocker == nil || blocker.Kind != statev2.BlockerKindRuntimeUnknown || blocker.TaskID != "task-1" || blocker.InvocationID != "inv-1" || blocker.OperatorRef != "owner-1" || blocker.Diagnostic == "" {
			t.Fatalf("runtime blocker = %#v", blocker)
		}
	}
}

func TestCoordinatorProvenAbsentPublishesExactlyOnce(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcilePublicationSnapshot()}
	pub := &reconcileTestPublisher{observation: PublicationObservation{State: PublicationObservationAbsent}, publishReceipt: &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: reconcileAt}}
	c := NewCoordinator(st, nil, pub, &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}, "owner-1", 41, reconcileAt)
	if _, err := c.Reconcile(context.Background(), "work-1"); err != nil {
		t.Fatal(err)
	}
	if pub.observes != 1 || pub.publishes != 1 || st.snapshot.Publications["intent-1"].Status != statev2.PublicationCompleted {
		t.Fatalf("calls=%d/%d status=%q", pub.observes, pub.publishes, st.snapshot.Publications["intent-1"].Status)
	}
}

func TestCoordinatorReleasesLeaseOnLoadRuntimePublicationAndCancelReturns(t *testing.T) {
	tests := []struct {
		name      string
		state     State
		runtime   Runtime
		publisher Publisher
		ctx       context.Context
	}{
		{name: "load error", state: &reconcileErrorState{snapshot: reconcileSnapshot(statev2.TaskPending), loadErr: errors.New("load failed")}},
		{name: "runtime observation error", state: &reconcileTestState{snapshot: func() statev2.WorkSnapshot {
			s := reconcileSnapshot(statev2.TaskInvocationReserved)
			task := s.TaskStates["task-1"]
			task.Invocation.LaunchRequested = true
			s.TaskStates["task-1"] = task
			return s
		}()}, runtime: &reconcileTestRuntime{observeErr: errors.New("provider unavailable")}},
		{name: "publication settlement apply error", state: &reconcileTestState{snapshot: reconcilePublicationSnapshot(), applyErr: errors.New("settlement failed")}, publisher: &reconcileTestPublisher{observation: PublicationObservation{State: PublicationObservationMatch, Receipt: &statev2.PublicationReceipt{NodeID: "node-1", PublishedAt: reconcileAt}}}},
		{name: "canceled context", state: &reconcileTestState{snapshot: reconcileSnapshot(statev2.TaskPending)}, ctx: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lease := &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}
			c := NewCoordinator(tc.state, tc.runtime, tc.publisher, &reconcileLocker{lease: lease}, "owner-1", 41, reconcileAt)
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			_, _ = c.Reconcile(ctx, "work-1")
			if lease.releases != 1 {
				t.Fatalf("lease releases = %d, want 1", lease.releases)
			}
		})
	}
}

type reconcileErrorState struct {
	snapshot statev2.WorkSnapshot
	loadErr  error
}

func (s *reconcileErrorState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	if s.loadErr != nil {
		return statev2.WorkSnapshot{}, s.loadErr
	}
	return s.snapshot, nil
}
func (s *reconcileErrorState) Apply(context.Context, statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	return s.snapshot, errors.New("apply failed")
}

func TestCoordinatorPublicationObserveErrorIsConflictWithoutPublish(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcilePublicationSnapshot()}
	pub := &reconcileTestPublisher{observeErr: errors.New("provider secret should not persist")}
	c := NewCoordinator(st, nil, pub, &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}, "owner-1", 41, reconcileAt)
	if _, err := c.Reconcile(context.Background(), "work-1"); err == nil {
		t.Fatal("observe error unexpectedly succeeded")
	}
	if pub.publishes != 0 || st.lastPublicationAction != statev2.PublicationActionConflict || st.lastPublicationDiagnostic == "" || strings.Contains(st.lastPublicationDiagnostic, "provider secret") {
		t.Fatalf("publication calls=%d action=%q diagnostic=%q", pub.publishes, st.lastPublicationAction, st.lastPublicationDiagnostic)
	}
}

func TestCoordinatorPublicationUnknownIsConflictWithoutPublish(t *testing.T) {
	st := &reconcileTestState{snapshot: reconcilePublicationSnapshot()}
	pub := &reconcileTestPublisher{observation: PublicationObservation{State: PublicationObservationUnknown}, publishReceipt: &statev2.PublicationReceipt{NodeID: "should-not-publish", PublishedAt: reconcileAt}}
	c := NewCoordinator(st, nil, pub, &reconcileLocker{lease: &reconcileLease{record: OwnerRecord{WorkID: "work-1", OwnerID: "owner-1", PID: 41, StartedAt: reconcileAt}}}, "owner-1", 41, reconcileAt)
	if _, err := c.Reconcile(context.Background(), "work-1"); err == nil {
		t.Fatal("unknown observation unexpectedly succeeded")
	}
	if pub.publishes != 0 || st.lastPublicationAction != statev2.PublicationActionConflict {
		t.Fatalf("publication calls=%d action=%q", pub.publishes, st.lastPublicationAction)
	}
}

var reconcileAt = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

func reconcileSnapshot(status statev2.TaskStatus) statev2.WorkSnapshot {
	worktree := &statev2.WorktreeIdentity{CanonicalPath: "/work", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: "0123456789012345678901234567890123456789"}
	logical := &statev2.LogicalWorkState{LogicalWorkID: "logical-1", Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1", Worktree: worktree}
	inv := &statev2.InvocationState{InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}
	return statev2.WorkSnapshot{WorkID: "work-1", Revision: 1, State: statev2.StateRunning, ContractHash: "hash", Contract: contractv2.WorkItemContract{WorkID: "work-1", Tasks: []contractv2.Task{{TaskID: "task-1"}}}, Control: statev2.WorkControl{ApprovedContractHash: "hash", ApprovalRef: "approval"}, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{"task-1": {TaskID: "task-1", Status: status, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1, LogicalWork: logical, Worktree: worktree, Invocation: inv, InvocationHistory: []statev2.InvocationID{"inv-1"}}}, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
}

func reconcilePublicationSnapshot() statev2.WorkSnapshot {
	s := reconcileSnapshot(statev2.TaskIntegrated)
	s.TaskStates["task-1"] = statev2.TaskExecutionState{TaskID: "task-1", Status: statev2.TaskIntegrated, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1, InvocationHistory: []statev2.InvocationID{}}
	s.Publications["intent-1"] = statev2.PublicationState{IntentID: "intent-1", Key: "issue:1", Generation: 1, Kind: statev2.PublicationParentIssue, Status: statev2.PublicationPending, PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PayloadRef: "artifact://one", Target: statev2.PublicationTarget{Host: "github.com", Key: "issue:1"}, CompletionRequired: true}
	s.State = statev2.StatePublicationPending
	return s
}

type reconcileTestState struct {
	snapshot                  statev2.WorkSnapshot
	applyErr                  error
	taskActions               int
	lastTaskAction            statev2.TaskAction
	lastPublicationAction     statev2.PublicationAction
	lastPublicationDiagnostic string
}

func (s *reconcileTestState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}
func (s *reconcileTestState) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	if s.applyErr != nil {
		return s.snapshot, s.applyErr
	}
	if req.Task != nil {
		s.taskActions++
		s.lastTaskAction = req.Task.Action
		t := s.snapshot.TaskStates[req.Task.TaskID]
		switch req.Task.Action {
		case statev2.TaskReconcileNotStarted:
			t.Status = statev2.TaskPending
			t.Invocation = nil
		case statev2.TaskMarkRunning:
			t.Status = statev2.TaskRunning
			t.Invocation.ProviderIdentity = req.Task.Invocation.ProviderIdentity
		case statev2.TaskRequestTermination:
			t.Status = statev2.TaskTerminationPending
		case statev2.TaskConfirmTermination:
			t.Status = statev2.TaskTerminated
		case statev2.TaskNeedsOperatorAction:
			t.Status = statev2.TaskNeedsOperator
			s.snapshot.Control.Blocker = req.Task.Blocker
		}
		s.snapshot.TaskStates[req.Task.TaskID] = t
	}
	if req.Publication != nil {
		s.lastPublicationAction = req.Publication.Action
		s.lastPublicationDiagnostic = req.Publication.Diagnostic
		p := s.snapshot.Publications[req.Publication.IntentID]
		if req.Publication.Action == statev2.PublicationComplete {
			p.Status = statev2.PublicationCompleted
			p.Receipt = req.Publication.Receipt
		}
		s.snapshot.Publications[req.Publication.IntentID] = p
	}
	statev2.Reduce(&s.snapshot)
	return s.snapshot, nil
}

type reconcileTestRuntime struct {
	observation                    RuntimeObservation
	observeErr                     error
	observes, launches, terminates int
}

func (r *reconcileTestRuntime) Observe(context.Context, statev2.InvocationState) (RuntimeObservation, error) {
	r.observes++
	return r.observation, r.observeErr
}
func (r *reconcileTestRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity) (string, error) {
	r.launches++
	return "provider-1", nil
}
func (r *reconcileTestRuntime) Terminate(context.Context, statev2.InvocationState) error {
	r.terminates++
	return nil
}

type reconcileTestPublisher struct {
	observation         PublicationObservation
	observeErr          error
	publishReceipt      *statev2.PublicationReceipt
	observes, publishes int
}

func (p *reconcileTestPublisher) Observe(context.Context, statev2.PublicationState) (PublicationObservation, error) {
	p.observes++
	return p.observation, p.observeErr
}
func (p *reconcileTestPublisher) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	p.publishes++
	if p.publishReceipt != nil {
		return *p.publishReceipt, nil
	}
	return statev2.PublicationReceipt{}, errors.New("unexpected publish")
}

type reconcileLocker struct {
	lease    OwnerLease
	acquires int
}

func (l *reconcileLocker) Acquire(context.Context, contractv2.WorkID, OwnerID, int, time.Time) (OwnerLease, error) {
	l.acquires++
	return l.lease, nil
}

type reconcileLease struct {
	record   OwnerRecord
	releases int
}

func (l *reconcileLease) Record() OwnerRecord { return l.record }
func (l *reconcileLease) Release() error      { l.releases++; return nil }
