// Package recovery contains the side-effect-free policy for recovering an
// incomplete agent run.
package recovery

import "time"

// ContinuationInstruction is sent when an incomplete agent can be prompted to
// continue its existing task.
const ContinuationInstruction = "Task packet과 현재 변경을 다시 확인하고, 완료되지 않은 수용 조건부터 계속 진행하세요. 이미 완료한 작업은 반복하지 마세요."

// Kind identifies the policy decision made for an agent snapshot.
type Kind string

const (
	Wait          Kind = "wait"
	Continue      Kind = "continue"
	ResumeSession Kind = "resume_session"
	AskOperator   Kind = "ask_operator"
	Block         Kind = "block"
)

// Policy bounds how long a live working agent may be left without progress
// and how many recovery actions may be attempted without progress.
type Policy struct {
	WorkingWait time.Duration
	Limit       int
}

// AgentSnapshot is the durable observation used by Decide. It intentionally
// has no Herdr or filesystem details so the policy cannot perform side effects.
type AgentSnapshot struct {
	State               string
	Alive               bool
	Complete            bool
	LastProgress        time.Time
	ProgressFingerprint string
	PreviousFingerprint string
	RecoveryCount       int
	CanNativeResume     bool
}

// Decision is the recovery action and the count to persist with it.
type Decision struct {
	Kind        Kind
	NextCount   int
	Instruction string
}

// Decide returns the next side-effect-free recovery action for agent.
func Decide(now time.Time, policy Policy, agent AgentSnapshot) Decision {
	count := agent.RecoveryCount
	if count < 0 {
		count = 0
	}

	// A non-empty changed fingerprint is evidence of progress. It must be
	// applied before any possible recovery action, including a cap decision.
	if agent.ProgressFingerprint != "" && agent.ProgressFingerprint != agent.PreviousFingerprint {
		return Decision{Kind: Wait, NextCount: 0}
	}

	// Completed work is terminal from the recovery supervisor's perspective.
	if agent.Complete {
		return Decision{Kind: Wait, NextCount: count}
	}

	// A blocked report is an operator decision, never an automatic prompt.
	if agent.State == "blocked" {
		return Decision{Kind: AskOperator, NextCount: count}
	}

	if agent.Alive {
		switch agent.State {
		case "working":
			if agent.LastProgress.IsZero() || now.Sub(agent.LastProgress) < policy.WorkingWait {
				return Decision{Kind: Wait, NextCount: count}
			}
			// Herdr v0.8.2 cannot prove foreground activity or exclude a
			// duplicate process, so stale live work is operator-owned.
			return Decision{Kind: AskOperator, NextCount: count}
		case "idle", "done":
			return continueDecision(policy, count)
		default:
			return Decision{Kind: AskOperator, NextCount: count}
		}
	}

	// An absent agent may be resumed only when orchestration established a
	// provider session identity. Terminal-only identities are excluded by the
	// CanNativeResume flag.
	if !agent.CanNativeResume {
		return Decision{Kind: AskOperator, NextCount: count}
	}
	if exhausted(policy, count) {
		return Decision{Kind: Block, NextCount: count}
	}
	return Decision{Kind: ResumeSession, NextCount: count + 1}
}

func continueDecision(policy Policy, count int) Decision {
	if exhausted(policy, count) {
		return Decision{Kind: Block, NextCount: count}
	}
	return Decision{Kind: Continue, NextCount: count + 1, Instruction: ContinuationInstruction}
}

func exhausted(policy Policy, count int) bool {
	return policy.Limit <= 0 || count >= policy.Limit
}
