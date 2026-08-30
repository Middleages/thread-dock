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
	if d.Kind != Continue || d.NextCount != 1 || d.Instruction != "Task packet과 현재 변경을 다시 확인하고, 완료되지 않은 수용 조건부터 계속 진행하세요. 이미 완료한 작업은 반복하지 마세요." {
		t.Fatal(d)
	}
}

func TestWorkingAgentAtWaitThresholdAsksOperator(t *testing.T) {
	base := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	d := Decide(base.Add(time.Hour), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{State: "working", LastProgress: base, Alive: true})
	if d.Kind != AskOperator {
		t.Fatal(d)
	}
}

func TestWorkingAgentWithoutProgressTimestampAsksOperator(t *testing.T) {
	d := Decide(time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{State: "working", Alive: true})
	if d.Kind != AskOperator {
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

func TestAbsentNativeSessionResumesWithIncrementedCount(t *testing.T) {
	d := Decide(time.Now(), Policy{WorkingWait: time.Hour, Limit: 3}, AgentSnapshot{Alive: false, CanNativeResume: true, RecoveryCount: 1})
	if d.Kind != ResumeSession || d.NextCount != 2 {
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

func TestRecoveryCapNeverExceedsThreeWhenPolicyLimitIsLarger(t *testing.T) {
	d := Decide(time.Now(), Policy{WorkingWait: time.Hour, Limit: 99}, AgentSnapshot{State: "idle", Alive: true, RecoveryCount: 3})
	if d.Kind != Block || d.NextCount != 3 {
		t.Fatal(d)
	}
}
