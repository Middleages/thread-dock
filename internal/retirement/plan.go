// Package retirement contains the pure policy for retiring execution
// sessions. It deliberately has no filesystem, Herdr, Git, clock, or state
// store dependencies.
package retirement

import (
	"errors"
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
	ObserveAgent     DecisionKind = "observe_agent"
	ProveGit         DecisionKind = "prove_git"
	ObserveWorkspace DecisionKind = "observe_workspace"
	RecordWorkspace  DecisionKind = "record_workspace"
	MarkRetired      DecisionKind = "mark_retired"
	CloseWorkspace   DecisionKind = "close_workspace"
	Complete         DecisionKind = "complete"
	NeedsOperator    DecisionKind = "needs_operator"
)

// Decision is the pure result of applying one observation to a durable plan.
// TargetKey and WorkspaceID are copied from the durable target to make the
// adapter call explicit and prevent callers from reconstructing identity.
type Decision struct {
	Kind        DecisionKind
	TargetKey   string
	WorkspaceID string
	NextStatus  string
	Reason      string
	Reasons     []string
}

// Observation contains one provider observation. The fields are intentionally
// provider-neutral so a caller can persist the observation separately while
// keeping this package free of Herdr and Git concerns.
type Observation struct {
	TargetKey         string
	AgentState        string
	WorkspaceFound    bool
	WorkspaceObserved bool
	// WorkspaceState is the lifecycle state of the exact workspace/pane
	// observation. It is distinct from AgentState because an Agent can report
	// done while the latest pane/workspace has become active again.
	WorkspaceState      string
	WorkspaceID         string
	PaneID              string
	Path                string
	GitProven           bool
	RepositoryCommonDir string
	Branch              string
	HeadSHA             string
}

type targetCandidate struct {
	key, role, taskID string
	agent             state.AgentEvidence
	worktree          state.WorktreeState
	head              string
}

// Build derives the deterministic retirement plan from a run snapshot.
// Reviewer is first, followed by builders in reverse contract order. Every
// candidate is validated and every Workspace must have exactly one owner.
// Aliases fail closed because later cleanup is keyed by the durable task.
func Build(snapshot state.RunSnapshot, automatic bool, targetPhase contract.RunPhase) (state.RetirementState, error) {
	plan := state.RetirementState{Status: "pending", TargetPhase: targetPhase, Automatic: automatic}
	var candidates []targetCandidate

	if snapshot.Reviewer.Name != "" || !isZeroWorktree(snapshot.ReviewerWorktree) || snapshot.FinalSHA != "" {
		candidates = append(candidates, targetCandidate{
			key: "reviewer", role: "reviewer", agent: snapshot.Reviewer,
			worktree: snapshot.ReviewerWorktree, head: snapshot.FinalSHA,
		})
	}
	if len(snapshot.Tasks) > 0 || len(snapshot.TaskOrder) > 0 {
		if err := validateTaskOrder(snapshot.Tasks, snapshot.TaskOrder); err != nil {
			return state.RetirementState{}, err
		}
		for i := len(snapshot.TaskOrder) - 1; i >= 0; i-- {
			taskID := snapshot.TaskOrder[i]
			task, ok := snapshot.Tasks[taskID]
			if !ok {
				return state.RetirementState{}, fmt.Errorf("retirement target %q: task is missing", taskID)
			}
			candidates = append(candidates, targetCandidate{
				key: "builder:" + taskID, role: "builder", taskID: taskID,
				agent: task.Agent, worktree: task.Worktree, head: task.Agent.CommitSHA,
			})
		}
	} else if snapshot.Builder.Name != "" || !isZeroWorktree(snapshot.BuilderWorktree) || snapshot.Builder.CommitSHA != "" {
		candidates = append(candidates, targetCandidate{
			key: "builder", role: "builder", agent: snapshot.Builder,
			worktree: snapshot.BuilderWorktree, head: snapshot.Builder.CommitSHA,
		})
	}

	seen := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		if err := validateCandidate(candidate.key, candidate.agent, candidate.worktree, candidate.head); err != nil {
			return state.RetirementState{}, err
		}
		if previousKey, ok := seen[candidate.worktree.WorkspaceID]; ok {
			return state.RetirementState{}, fmt.Errorf("retirement target %q: workspace is already owned by %q", candidate.key, previousKey)
		}
		seen[candidate.worktree.WorkspaceID] = candidate.key
		plan.Targets = append(plan.Targets, state.RetirementTarget{
			Key: candidate.key, Role: candidate.role, TaskID: candidate.taskID,
			WorkspaceID:         candidate.worktree.WorkspaceID,
			PaneID:              candidate.worktree.PaneID,
			Path:                filepath.Clean(candidate.worktree.Path),
			Branch:              candidate.worktree.Branch,
			HeadSHA:             candidate.head,
			Status:              "pending",
			AgentName:           candidate.agent.Name,
			RepositoryCommonDir: "",
		})
	}
	return plan, nil
}

func validateTaskOrder(tasks map[string]state.TaskRunState, order []string) error {
	if len(tasks) != len(order) {
		return errors.New("retirement task map and order must contain the same tasks")
	}
	seen := make(map[string]struct{}, len(order))
	for _, taskID := range order {
		if strings.TrimSpace(taskID) == "" {
			return errors.New("retirement task order contains an empty task ID")
		}
		if _, duplicate := seen[taskID]; duplicate {
			return fmt.Errorf("retirement task order contains duplicate task %q", taskID)
		}
		if _, ok := tasks[taskID]; !ok {
			return fmt.Errorf("retirement target %q: task is missing", taskID)
		}
		seen[taskID] = struct{}{}
	}
	return nil
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
	if !canonicalAgentName(agent.Name) {
		return fmt.Errorf("retirement target %q: agent name is not canonical", key)
	}
	if !canonicalID(worktree.WorkspaceID) {
		return fmt.Errorf("retirement target %q: workspace ID is not canonical", key)
	}
	if !canonicalID(worktree.PaneID) {
		return fmt.Errorf("retirement target %q: pane ID is not canonical", key)
	}
	if !canonicalPath(worktree.Path) {
		return fmt.Errorf("retirement target %q: path is not canonical", key)
	}
	return nil
}

func canonicalAgentName(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 32 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func canonicalID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "..") {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '.' && char != '_' && char != ':' && char != '-' {
			return false
		}
	}
	return true
}

func canonicalPath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && filepath.Clean(value) == value
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
		if observation != nil && observation.TargetKey != target.Key {
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
				base.NextStatus = "agent_observed"
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
			if strings.TrimSpace(observation.RepositoryCommonDir) == "" {
				return needs(base, "git repository identity is required")
			}
			base.Kind = ObserveWorkspace
			base.NextStatus = "git_proven"
			return base
		case "git_proven":
			if strings.TrimSpace(target.RepositoryCommonDir) == "" {
				return needs(base, "git repository identity is not persisted")
			}
			if observation == nil || !observation.WorkspaceObserved {
				base.Kind = ObserveWorkspace
				return base
			}
			if !observation.WorkspaceFound {
				base.Kind = MarkRetired
				base.NextStatus = "retired"
				return base
			}
			if !safeWorkspaceObservation(observation) {
				return needs(base, "workspace lifecycle state is not safe to retire")
			}
			if reason := workspaceIdentityMismatch(target, observation); reason != "" {
				return needs(base, reason)
			}
			base.Kind = RecordWorkspace
			base.NextStatus = "workspace_observed"
			return base
		case "workspace_observed":
			if strings.TrimSpace(target.RepositoryCommonDir) == "" {
				return needs(base, "git repository identity is not persisted")
			}
			if observation == nil {
				base.Kind = CloseWorkspace
				base.NextStatus = "closing"
				return base
			}
			if !observation.WorkspaceObserved || !observation.WorkspaceFound {
				return needs(base, "workspace observation is not an exact existing identity")
			}
			if reason := workspaceIdentityMismatch(target, observation); reason != "" {
				return needs(base, reason)
			}
			if !safeWorkspaceObservation(observation) {
				return needs(base, "workspace lifecycle state is not safe to retire")
			}
			base.Kind = CloseWorkspace
			base.NextStatus = "closing"
			return base
		case "closing":
			if observation == nil {
				base.Kind = ObserveWorkspace
				return base
			}
			if !observation.WorkspaceObserved {
				return needs(base, "workspace observation is required")
			}
			if !observation.WorkspaceFound {
				base.Kind = Complete
				base.NextStatus = "retired"
				return base
			}
			if reason := workspaceIdentityMismatch(target, observation); reason != "" {
				return needs(base, reason)
			}
			if !safeWorkspaceObservation(observation) {
				return needs(base, "workspace lifecycle state is not safe to retire")
			}
			base.Kind = CloseWorkspace
			base.NextStatus = "closing"
			return base
		default:
			return needs(base, "unknown retirement target status")
		}
	}
	return Decision{Kind: Complete}
}

func safeWorkspaceLifecycle(value string) bool {
	switch strings.ToLower(value) {
	case "idle", "done", "complete", "completed":
		return true
	default:
		return false
	}
}

// safeWorkspaceObservation accepts either modeled lifecycle field when the
// other is not supplied, but every supplied field must be safe. Herdr's
// adapter currently exposes the exact pane state as WorkspaceState; tests and
// alternate adapters may also provide AgentState for the same observation.
func safeWorkspaceObservation(observation *Observation) bool {
	if observation == nil {
		return false
	}
	seen := false
	for _, value := range []string{observation.AgentState, observation.WorkspaceState} {
		if value == "" {
			continue
		}
		seen = true
		if !safeWorkspaceLifecycle(value) {
			return false
		}
	}
	return seen
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
	if !observation.WorkspaceObserved || !observation.WorkspaceFound || !canonicalID(observation.WorkspaceID) || !canonicalID(observation.PaneID) || !canonicalPath(observation.Path) || observation.WorkspaceID != target.WorkspaceID || observation.PaneID != target.PaneID || observation.Path != target.Path {
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
