package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	statev2 "thread-dock/internal/state/v2"
)

type testRuntime struct {
	mu         sync.Mutex
	launches   int
	observes   int
	terminates int
	ended      bool
	identity   string
}

func (r *testRuntime) Observe(context.Context, statev2.InvocationState) (RuntimeObservation, error) {
	r.mu.Lock()
	r.observes++
	ended := r.ended
	r.mu.Unlock()
	if ended {
		return RuntimeObservation{State: RuntimeObservationEnded, ProviderIdentity: r.identity}, nil
	}
	return RuntimeObservation{State: RuntimeObservationActive, ProviderIdentity: r.identity}, nil
}
func (r *testRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity) (string, error) {
	r.mu.Lock()
	r.launches++
	identity := r.identity
	r.mu.Unlock()
	return identity, nil
}
func (r *testRuntime) Terminate(context.Context, statev2.InvocationState) error {
	r.mu.Lock()
	r.terminates++
	r.mu.Unlock()
	return nil
}

func runtimeTestSnapshot() statev2.WorkSnapshot {
	workID := contractv2.WorkID("work-1")
	taskID := contractv2.TaskID("task-1")
	worktree := &statev2.WorktreeIdentity{CanonicalPath: "/work", GitCommonDir: "/repo/.git", Branch: "agent/task-1", BaseSHA: "0123456789012345678901234567890123456789"}
	logical := &statev2.LogicalWorkState{LogicalWorkID: "logical-1", Role: "builder", BuilderAttempt: 1, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1", Worktree: worktree}
	inv := &statev2.InvocationState{InvocationID: "inv-1", LogicalWorkID: "logical-1", Role: "builder", ReturnStage: statev2.TaskPending, LogicalProfile: "builder", RuntimeFingerprint: "runtime-v1"}
	return statev2.WorkSnapshot{WorkID: workID, Revision: 1, TaskStates: map[contractv2.TaskID]statev2.TaskExecutionState{taskID: {TaskID: taskID, Status: statev2.TaskInvocationReserved, BuilderAttempt: 1, RepairLimit: 2, RecoveryLimit: 1, LogicalWork: logical, Worktree: worktree, Invocation: inv, InvocationHistory: []statev2.InvocationID{"inv-1"}}}}
}

func TestRuntimeReserveAloneDoesNotLaunchAndSubmitBeginsBeforeOneLaunch(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	rt := &testRuntime{identity: "provider-1"}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	if result.Err != nil {
		t.Fatalf("submit runtime: %v", result.Err)
	}
	rt.mu.Lock()
	launches := rt.launches
	rt.mu.Unlock()
	if launches != 1 {
		t.Fatalf("launches = %d, want 1", launches)
	}
	if len(st.transitions) < 2 || st.transitions[0].Task == nil || st.transitions[0].Task.Action != statev2.TaskBeginLaunch || st.transitions[1].Task.Action != statev2.TaskMarkRunning {
		t.Fatalf("transitions = %#v", st.transitions)
	}
	_ = d.Close(context.Background())
}

func TestRuntimeLaunchRequestReplayObservesWithoutRelaunch(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	st.snapshot.TaskStates["task-1"].Invocation.LaunchRequested = true
	rt := &testRuntime{identity: "provider-1"}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	if result := <-d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1"); result.Err != nil {
		t.Fatalf("submit runtime: %v", result.Err)
	}
	rt.mu.Lock()
	launches, observes := rt.launches, rt.observes
	rt.mu.Unlock()
	if launches != 0 || observes != 1 {
		t.Fatalf("runtime calls launch=%d observe=%d", launches, observes)
	}
	_ = d.Close(context.Background())
}

func TestRuntimePauseRequestsTerminationAndConfirmsAfterTerminate(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	task := st.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	task.Invocation.LaunchRequested = true
	task.Invocation.ProviderIdentity = "provider-1"
	st.snapshot.TaskStates["task-1"] = task
	st.snapshot.Control.PauseRequested = true
	rt := &testRuntime{identity: "provider-1"}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	if result.Err != nil {
		t.Fatalf("submit runtime: %v", result.Err)
	}
	if got := st.snapshot.TaskStates["task-1"].Status; got != statev2.TaskTerminated {
		t.Fatalf("task status = %q, want terminated", got)
	}
	rt.mu.Lock()
	terminates := rt.terminates
	rt.mu.Unlock()
	if terminates != 1 {
		t.Fatalf("terminate calls = %d, want 1", terminates)
	}
	for _, transition := range st.transitions {
		if transition.Task == nil {
			continue
		}
		if transition.Task.Action == statev2.TaskRequestTermination && transition.Task.Reason == "" {
			t.Fatal("request termination reason is empty")
		}
		if transition.Task.Action == statev2.TaskConfirmTermination && transition.Task.Reason == "" {
			t.Fatal("confirm termination reason is empty")
		}
	}
	_ = d.Close(context.Background())
}

func TestRuntimeProviderMismatchNeedsOperatorAndDoesNotConfirm(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	task := st.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	task.Invocation.LaunchRequested = true
	task.Invocation.ProviderIdentity = "provider-1"
	st.snapshot.TaskStates["task-1"] = task
	rt := &testRuntime{identity: "provider-2"}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	if result.Err == nil || st.snapshot.TaskStates["task-1"].Status != statev2.TaskNeedsOperator {
		t.Fatalf("result=%#v task=%#v", result, st.snapshot.TaskStates["task-1"])
	}
	if got := st.snapshot.TaskStates["task-1"].Invocation.TerminationConfirmed; got {
		t.Fatal("provider mismatch confirmed termination")
	}
	blocker := st.snapshot.Control.Blocker
	if blocker == nil || blocker.Kind != statev2.BlockerKindRuntimeUnknown || blocker.OperatorRef != "owner-1" || blocker.TaskID != "task-1" || blocker.InvocationID != "inv-1" || blocker.Diagnostic == "" {
		t.Fatalf("runtime blocker=%#v", blocker)
	}
	_ = d.Close(context.Background())
}

func TestRuntimeUnknownNeedsOperatorWithoutRelaunch(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	st.snapshot.TaskStates["task-1"].Invocation.LaunchRequested = true
	rt := &testRuntime{identity: ""}
	rt.ended = false
	// An unknown observation must settle durably as operator attention; it must
	// not create a second launch attempt.
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	result := <-d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	if result.Err == nil || st.snapshot.TaskStates["task-1"].Status != statev2.TaskNeedsOperator {
		t.Fatalf("result=%#v task=%#v", result, st.snapshot.TaskStates["task-1"])
	}
	rt.mu.Lock()
	launches := rt.launches
	rt.mu.Unlock()
	if launches != 0 {
		t.Fatalf("launches = %d, want 0", launches)
	}
	_ = d.Close(context.Background())
}

func TestRuntimePositiveNoLaunchReconcileRequiresEvidence(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	d := NewRuntimeDispatcher(st, &testRuntime{identity: "provider-1"}, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC()).(*publicationDispatcher)
	bad := d.reconcileRuntimeNotStarted(context.Background(), st.snapshot, "task-1", "inv-1", statev2.ResolutionEvidence{OwnerTerminated: true})
	if bad.Err == nil {
		t.Fatal("incomplete no-launch evidence was accepted")
	}
	good := d.reconcileRuntimeNotStarted(context.Background(), st.snapshot, "task-1", "inv-1", statev2.ResolutionEvidence{OwnerTerminated: true, ProviderAbsent: true, Diagnostic: "owner exited before launch"})
	if good.Err != nil {
		t.Fatalf("positive no-launch reconcile: %v", good.Err)
	}
	if len(st.transitions) != 1 || st.transitions[0].Task.Action != statev2.TaskReconcileNotStarted {
		t.Fatalf("transitions=%#v", st.transitions)
	}
}

type barrierRuntime struct {
	launchEntered    chan struct{}
	launchRelease    chan struct{}
	observeEntered   chan struct{}
	observeRelease   chan struct{}
	terminateEntered chan struct{}
	terminateRelease chan struct{}
	mu               sync.Mutex
	launchCalls      int
	observeCalls     int
	terminateCalls   int
	launchOnce       sync.Once
	observeOnce      sync.Once
	terminateOnce    sync.Once
}

func (r *barrierRuntime) Launch(context.Context, statev2.InvocationState, statev2.WorktreeIdentity) (string, error) {
	r.mu.Lock()
	r.launchCalls++
	r.mu.Unlock()
	if r.launchEntered != nil {
		r.launchOnce.Do(func() { close(r.launchEntered) })
	}
	if r.launchRelease != nil {
		<-r.launchRelease
	}
	return "provider-1", nil
}
func (r *barrierRuntime) Observe(context.Context, statev2.InvocationState) (RuntimeObservation, error) {
	r.mu.Lock()
	r.observeCalls++
	r.mu.Unlock()
	if r.observeEntered != nil {
		r.observeOnce.Do(func() { close(r.observeEntered) })
	}
	if r.observeRelease != nil {
		<-r.observeRelease
	}
	return RuntimeObservation{State: RuntimeObservationActive, ProviderIdentity: "provider-1"}, nil
}
func (r *barrierRuntime) Terminate(context.Context, statev2.InvocationState) error {
	r.mu.Lock()
	r.terminateCalls++
	r.mu.Unlock()
	if r.terminateEntered != nil {
		r.terminateOnce.Do(func() { close(r.terminateEntered) })
	}
	if r.terminateRelease != nil {
		<-r.terminateRelease
	}
	return nil
}

func TestRuntimeBlockedWorkersDoNotBlockQueue(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	rt := &barrierRuntime{launchEntered: make(chan struct{}), launchRelease: make(chan struct{}), observeEntered: make(chan struct{}), observeRelease: make(chan struct{})}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	first := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.launchEntered:
	case <-time.After(time.Second):
		t.Fatal("launch did not enter barrier")
	}
	st.mu.Lock()
	st.snapshot.Control.PauseRequested = true
	st.mu.Unlock()
	second := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.observeEntered:
	case <-time.After(time.Second):
		t.Fatal("queue did not dispatch observe while launch was blocked")
	}
	close(rt.launchRelease)
	close(rt.observeRelease)
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("launch result stranded")
	}
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("observe result stranded")
	}
	_ = d.Close(context.Background())
}

func TestRuntimeBlockedObserveStillProcessesPauseTermination(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	task := st.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	task.Invocation.LaunchRequested = true
	task.Invocation.ProviderIdentity = "provider-1"
	st.snapshot.TaskStates["task-1"] = task
	rt := &barrierRuntime{observeEntered: make(chan struct{}), observeRelease: make(chan struct{}), terminateEntered: make(chan struct{}), terminateRelease: make(chan struct{})}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	first := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.observeEntered:
	case <-time.After(time.Second):
		t.Fatal("observe did not enter barrier")
	}
	st.mu.Lock()
	st.snapshot.Control.PauseRequested = true
	st.mu.Unlock()
	second := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.terminateEntered:
	case <-time.After(time.Second):
		t.Fatal("pause submission did not start termination while observe was blocked")
	}
	third := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	close(rt.terminateRelease)
	for name, resultCh := range map[string]<-chan CommandResult{"second": second, "third": third} {
		select {
		case result := <-resultCh:
			if result.Err != nil || result.Snapshot.TaskStates["task-1"].Status != statev2.TaskTerminated {
				t.Fatalf("%s termination result=%#v", name, result)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s termination result stranded", name)
		}
	}
	rt.mu.Lock()
	terminateCalls := rt.terminateCalls
	rt.mu.Unlock()
	if terminateCalls != 1 {
		t.Fatalf("terminate calls = %d, want 1", terminateCalls)
	}
	// The first Observe remains blocked until explicitly released; the queue
	// has already completed the pause/terminate lifecycle above.
	close(rt.observeRelease)
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("blocked observe result stranded")
	}
	_ = d.Close(context.Background())
}

func TestRuntimeDuplicateObserveSubmissionsDeduplicateProviderCall(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	task := st.snapshot.TaskStates["task-1"]
	task.Status = statev2.TaskRunning
	task.Invocation.LaunchRequested = true
	task.Invocation.ProviderIdentity = "provider-1"
	st.snapshot.TaskStates["task-1"] = task
	rt := &barrierRuntime{observeEntered: make(chan struct{}), observeRelease: make(chan struct{})}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	first := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	second := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.observeEntered:
	case <-time.After(time.Second):
		t.Fatal("observe did not enter barrier")
	}
	close(rt.observeRelease)
	rt.mu.Lock()
	observeCalls := rt.observeCalls
	rt.mu.Unlock()
	if observeCalls != 1 {
		t.Fatalf("observe calls = %d, want 1", observeCalls)
	}
	for name, result := range map[string]<-chan CommandResult{"first": first, "second": second} {
		select {
		case got := <-result:
			if got.Err != nil {
				t.Fatalf("%s result: %v", name, got.Err)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s result stranded", name)
		}
	}
	_ = d.Close(context.Background())
}

func TestRuntimePreIOIdentityDriftPreventsProviderCall(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot(), preIOEntered: make(chan struct{}), preIOAllow: make(chan struct{})}
	rt := &testRuntime{identity: "provider-1"}
	d := NewRuntimeDispatcher(st, rt, &fakeOwnerLocker{}, OwnerID("owner-1"), 42, time.Now().UTC())
	preIOEntered := st.preIOEntered
	resultCh := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-preIOEntered:
	case <-time.After(time.Second):
		t.Fatal("worker pre-I/O load did not enter barrier")
	}
	st.mu.Lock()
	st.snapshot.TaskStates["task-1"].Invocation.RuntimeFingerprint = "drifted-runtime"
	st.mu.Unlock()
	close(st.preIOAllow)
	result := <-resultCh
	if result.Err == nil || !errors.Is(result.Err, ErrRuntimeStale) {
		t.Fatalf("identity drift result = %v", result.Err)
	}
	rt.mu.Lock()
	launches := rt.launches
	rt.mu.Unlock()
	if launches != 0 {
		t.Fatalf("launches = %d after identity drift", launches)
	}
	_ = d.Close(context.Background())
}

func TestRuntimeCloseCancelsWorkersResolvesResultsAndReleasesLease(t *testing.T) {
	st := &runtimeTestState{snapshot: runtimeTestSnapshot()}
	rt := &barrierRuntime{launchEntered: make(chan struct{}), launchRelease: make(chan struct{})}
	locker := &fakeOwnerLocker{}
	d := NewRuntimeDispatcher(st, rt, locker, OwnerID("owner-1"), 42, time.Now().UTC())
	result := d.SubmitRuntime(context.Background(), "work-1", "task-1", "inv-1")
	select {
	case <-rt.launchEntered:
	case <-time.After(time.Second):
		t.Fatal("launch did not enter barrier")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- d.Close(context.Background()) }()
	q := d.(*publicationDispatcher).queues["work-1"]
	select {
	case <-q.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel runtime queue")
	}
	close(rt.launchRelease)
	if err := <-closeDone; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := <-result; !errors.Is(got.Err, ErrDispatcherClosed) {
		t.Fatalf("worker result error = %v, want dispatcher closed", got.Err)
	}
	if locker.releases != 1 {
		t.Fatalf("owner releases = %d, want 1", locker.releases)
	}
}

// runtimeTestState intentionally records typed transitions while applying the
// small lifecycle mutations needed by dispatcher tests.
type runtimeTestState struct {
	mu           sync.Mutex
	snapshot     statev2.WorkSnapshot
	transitions  []statev2.TransitionRequest
	preIOEntered chan struct{}
	preIOAllow   chan struct{}
}

func (s *runtimeTestState) Load(context.Context, contractv2.WorkID) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var allow chan struct{}
	if s.preIOEntered != nil {
		task := s.snapshot.TaskStates["task-1"]
		if task.Invocation != nil && task.Invocation.LaunchRequested {
			close(s.preIOEntered)
			s.preIOEntered = nil
			allow = s.preIOAllow
		}
	}
	if allow != nil {
		s.mu.Unlock()
		<-allow
		s.mu.Lock()
	}
	data, err := json.Marshal(s.snapshot)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	var copy statev2.WorkSnapshot
	if err := json.Unmarshal(data, &copy); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	return copy, nil
}
func (s *runtimeTestState) Apply(_ context.Context, req statev2.TransitionRequest) (statev2.WorkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ExpectedRevision != s.snapshot.Revision {
		return s.snapshot, statev2.ErrStaleRevision
	}
	hash, err := statev2.TransitionPayloadHash(req)
	if err != nil || hash != req.PayloadHash {
		return s.snapshot, errors.New("invalid transition hash")
	}
	if req.Task == nil {
		return s.snapshot, errors.New("task transition required")
	}
	t := *req.Task
	s.transitions = append(s.transitions, req)
	task := s.snapshot.TaskStates[t.TaskID]
	if task.Invocation == nil || task.Invocation.InvocationID != t.InvocationID {
		return s.snapshot, errors.New("invocation mismatch")
	}
	switch t.Action {
	case statev2.TaskBeginLaunch:
		task.Invocation.LaunchRequested = true
	case statev2.TaskMarkRunning:
		task.Status = statev2.TaskRunning
		task.Invocation.ProviderIdentity = t.Invocation.ProviderIdentity
		task.Invocation.ProviderSession = t.Invocation.ProviderSession
		started := t.At
		task.Invocation.StartedAt = &started
	case statev2.TaskRequestTermination:
		if t.Reason == "" {
			return s.snapshot, errors.New("termination reason required")
		}
		task.Status = statev2.TaskTerminationPending
		task.Invocation.TerminationReason = t.Reason
	case statev2.TaskConfirmTermination:
		if t.Reason == "" {
			return s.snapshot, errors.New("termination reason required")
		}
		task.Status = statev2.TaskTerminated
		task.Invocation.TerminationConfirmed = true
		ended := t.At
		task.Invocation.EndedAt = &ended
	case statev2.TaskNeedsOperatorAction:
		if t.Blocker == nil || t.Blocker.Kind != statev2.BlockerKindRuntimeUnknown || t.Blocker.OperatorRef == "" || t.Blocker.TaskID != t.TaskID || t.Blocker.InvocationID != t.InvocationID || t.Blocker.Diagnostic == "" || len([]byte(t.Blocker.Diagnostic)) > statev2.MaxDiagnosticBytes {
			return s.snapshot, errors.New("runtime unknown blocker required")
		}
		task.Status = statev2.TaskNeedsOperator
		s.snapshot.Control.Blocker = t.Blocker
	}
	s.snapshot.TaskStates[t.TaskID] = task
	s.snapshot.Revision++
	return s.snapshot, nil
}
