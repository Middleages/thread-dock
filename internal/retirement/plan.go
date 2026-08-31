// Package retirement contains the pure policy for retiring execution
// sessions. It deliberately has no filesystem, Herdr, Git, clock, or state
// store dependencies.
package retirement

import (
	"fmt"
	"path/filepath"
	"strings"

	"thread-dock/internal/contract"
	"thread-dock/internal/state"
)

// DecisionKind identifies the one action an orchestrator should perform for
// the current retirement target.
type DecisionKind string

// Kind is retained as a concise name for callers that use the other pure
// policy packages in this repository.
type Kind = DecisionKind

const (
	ObserveAgent   DecisionKind = "observe_agent"
	ProveGit       DecisionKind = "prove_git"
	CloseWorkspace DecisionKind = "close_workspace"
	Complete       DecisionKind = "complete"
	NeedsOperator  DecisionKind = "needs_operator"
)

// Decision is the pure result of applying one observation to a durable plan.
// TargetKey and WorkspaceID are copied from the durable target to make the
// adapter call explicit and prevent callers from reconstructing identity.
type Decision struct {
	Kind        DecisionKind
	TargetKey   string
	WorkspaceID string
	Reason      string
	Reasons     []string
}

// Observation contains one provider observation. The fields are intentionally
// provider-neutral so a caller can persist the observation separately while
// keeping this package free of Herdr and Git concerns.
type Observation struct {
	TargetKey           string
	AgentState          string
	WorkspaceFound      bool
	WorkspaceObserved   bool
	WorkspaceID         string
	PaneID              string
	Path                string
	GitProven           bool
	RepositoryCommonDir string
	Branch              string
	HeadSHA             string
}

// Build derives the deterministic retirement plan from a run snapshot.
// Reviewer is first, followed by builders in reverse contract order. Every
// candidate is validated before duplicate workspace IDs are collapsed.
func Build(snapshot state.RunSnapshot, automatic bool, targetPhase contract.RunPhase) (state.RetirementState, error) {
	plan := state.RetirementState{Status: "pending", TargetPhase: targetPhase, Automatic: automatic}
	type candidate struct {
		key, role, taskID string
		agent             state.AgentEvidence
		worktree          state.WorktreeState
		head              string
	}
	var candidates []candidate

	if snapshot.Reviewer.Name != "" || !isZeroWorktree(snapshot.ReviewerWorktree) || snapshot.FinalSHA != "" {
		candidates = append(candidates, candidate{
			key: "reviewer", role: "reviewer", agent: snapshot.Reviewer,
			worktree: snapshot.ReviewerWorktree, head: snapshot.FinalSHA,
		})
	}
	if len(snapshot.Tasks) > 0 || len(snapshot.TaskOrder) > 0 {
		for i := len(snapshot.TaskOrder) - 1; i >= 0; i-- {
			taskID := snapshot.TaskOrder[i]
			task, ok := snapshot.Tasks[taskID]
			if !ok {
				return state.RetirementState{}, fmt.Errorf("retirement target %q: task is missing", taskID)
			}
			candidates = append(candidates, candidate{
				key: "builder:" + taskID, role: "builder", taskID: taskID,
				agent: task.Agent, worktree: task.Worktree, head: task.Agent.CommitSHA,
			})
		}
	} else if snapshot.Builder.Name != "" || !isZeroWorktree(snapshot.BuilderWorktree) || snapshot.Builder.CommitSHA != "" {
		candidates = append(candidates, candidate{
			key: "builder", role: "builder", agent: snapshot.Builder,
			worktree: snapshot.BuilderWorktree, head: snapshot.Builder.CommitSHA,
		})
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if err := validateCandidate(candidate.key, candidate.agent, candidate.worktree, candidate.head); err != nil {
			return state.RetirementState{}, err
		}
		if _, ok := seen[candidate.worktree.WorkspaceID]; ok {
			continue
		}
		seen[candidate.worktree.WorkspaceID] = struct{}{}
		plan.Targets = append(plan.Targets, state.RetirementTarget{
			Key: candidate.key, Role: candidate.role, TaskID: candidate.taskID,
			WorkspaceID:         candidate.worktree.WorkspaceID,
			PaneID:              candidate.worktree.PaneID,
			Path:                filepath.Clean(candidate.worktree.Path),
			Branch:              candidate.worktree.Branch,
			HeadSHA:             candidate.head,
			Status:              "pending",
			AgentName:           candidate.agent.Name,
			RepositoryCommonDir: cleanOptional(snapshot.RepositoryPath),
		})
	}
	return plan, nil
}

func validateCandidate(key string, agent state.AgentEvidence, worktree state.WorktreeState, head string) error {
	fields := map[string]string{
		"agent name": agent.Name, "workspace ID": worktree.WorkspaceID,
		"pane ID": worktree.PaneID, "path": worktree.Path,
		"branch": worktree.Branch, "expected HEAD": head,
	}
	for label, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("retirement target %q: %s is required", key, label)
		}
	}
	return nil
}

func isZeroWorktree(worktree state.WorktreeState) bool {
	return worktree.Path == "" && worktree.WorkspaceID == "" && worktree.PaneID == "" && worktree.Branch == ""
}

func cleanOptional(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Clean(path)
}

// Next chooses one pure retirement action. The caller is responsible for
// persisting the observation and applying the resulting lifecycle mutation.
func Next(plan state.RetirementState, observation *Observation) Decision {
	if len(plan.Targets) == 0 {
		return Decision{Kind: Complete}
	}
	for i := range plan.Targets {
		target := plan.Targets[i]
		if target.Status == "retired" {
			continue
		}
		base := Decision{TargetKey: target.Key, WorkspaceID: target.WorkspaceID}
		if observation != nil && observation.TargetKey != "" && observation.TargetKey != target.Key {
			return needs(base, "observation target identity mismatch")
		}
		switch target.Status {
		case "pending", "":
			if observation == nil {
				base.Kind = ObserveAgent
				return base
			}
			switch strings.ToLower(strings.TrimSpace(observation.AgentState)) {
			case "idle", "done", "complete", "completed":
				base.Kind = ProveGit
				return base
			default:
				return needs(base, "agent state is not safe to retire")
			}
		case "agent_observed":
			if observation == nil || !observation.GitProven {
				base.Kind = ProveGit
				return base
			}
			if reason := gitIdentityMismatch(target, observation); reason != "" {
				return needs(base, reason)
			}
			if observation.WorkspaceObserved && !observation.WorkspaceFound {
				base.Kind = Complete
				return base
			}
			base.Kind = CloseWorkspace
			return base
		case "git_proven":
			base.Kind = CloseWorkspace
			return base
		case "closing":
			if observation == nil {
				base.Kind = CloseWorkspace
				return base
			}
			if !observation.WorkspaceFound {
				base.Kind = Complete
				return base
			}
			if reason := workspaceIdentityMismatch(target, observation); reason != "" {
				return needs(base, reason)
			}
			base.Kind = CloseWorkspace
			return base
		default:
			return needs(base, "unknown retirement target status")
		}
	}
	return Decision{Kind: Complete}
}

func gitIdentityMismatch(target state.RetirementTarget, observation *Observation) string {
	if observation.Path != target.Path || observation.Branch != target.Branch || observation.HeadSHA != target.HeadSHA {
		return "git identity mismatch"
	}
	if target.RepositoryCommonDir != "" && observation.RepositoryCommonDir != target.RepositoryCommonDir {
		return "git repository identity mismatch"
	}
	return ""
}

func workspaceIdentityMismatch(target state.RetirementTarget, observation *Observation) string {
	if observation.WorkspaceID != target.WorkspaceID || observation.PaneID != target.PaneID || observation.Path != target.Path {
		return "workspace identity mismatch"
	}
	return ""
}

func needs(base Decision, reason string) Decision {
	base.Kind = NeedsOperator
	base.Reason = reason
	base.Reasons = []string{reason}
	return base
}
