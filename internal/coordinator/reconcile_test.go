package coordinator

import (
	"context"
	"errors"
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
	snapshot              statev2.WorkSnapshot
	taskActions           int
	lastTaskAction        statev2.TaskAction
	lastPublicationAction statev2.PublicationAction
}

func (s *reconcileTestState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.snapshot, nil
}
func (s *reconcileTestState) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
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
		}
		s.snapshot.TaskStates[req.Task.TaskID] = t
	}
	if req.Publication != nil {
		s.lastPublicationAction = req.Publication.Action
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
	observes, launches, terminates int
}

func (r *reconcileTestRuntime) Observe(context.Context, statev2.InvocationState) (RuntimeObservation, error) {
	r.observes++
	return r.observation, nil
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
	observes, publishes int
}

func (p *reconcileTestPublisher) Observe(context.Context, statev2.PublicationState) (PublicationObservation, error) {
	p.observes++
	return p.observation, nil
}
func (p *reconcileTestPublisher) Publish(context.Context, statev2.PublicationState) (statev2.PublicationReceipt, error) {
	p.publishes++
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
