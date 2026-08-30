package recovery

import (
	"testing"
	"time"
)

func TestWorkingAgentWaitsForSixtyMinutes(t *testing.T) {
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	d := Decide(base.Add(59*time.Minute), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{State: "working", LastProgress: base, Alive: true})
	if d.Kind != Wait {
		t.Fatal(d)
	}
}

func TestIncompleteIdleGetsContinueInstruction(t *testing.T) {
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	d := Decide(base.Add(10*time.Minute), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{State: "idle", Alive: true, RecoveryCount: 0})
	if d.Kind != Continue || d.NextCount != 1 || d.Instruction != ContinuationInstruction {
		t.Fatal(d)
	}
}

func TestBlockedNeverReceivesAutomaticInput(t *testing.T) {
	d := Decide(time.Now(), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{State: "blocked", Alive: true})
	if d.Kind != AskOperator {
		t.Fatal(d)
	}
}

func TestTerminalFallbackCannotClaimNativeResume(t *testing.T) {
	d := Decide(time.Now(), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{Alive: false, CanNativeResume: false})
	if d.Kind != AskOperator || d.NextCount != 0 {
		t.Fatal(d)
	}
}

func TestProgressResetAndThirdConsecutiveAttemptBlocks(t *testing.T) {
	policy := Policy{WorkingWait: time.Hour, Limit: 3}
	progressed := Decide(time.Now(), policy, AgentSnapshot{State: "idle", Alive: true, RecoveryCount: 2, PreviousFingerprint: "old", ProgressFingerprint: "new"})
	if progressed.Kind != Wait || progressed.NextCount != 0 {
		t.Fatal(progressed)
	}
	blocked := Decide(time.Now(), policy, AgentSnapshot{State: "done", Alive: true, RecoveryCount: 3, PreviousFingerprint: "same", ProgressFingerprint: "same"})
	if blocked.Kind != Block {
		t.Fatal(blocked)
	}
}
