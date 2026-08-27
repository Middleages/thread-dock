package contract

import (
	"time"

	"thread-dock/internal/version"
)

const CurrentVersion = version.Contract

type TaskContract struct {
	Version      int           `json:"version"`
	Parent       IssueDraft    `json:"parent"`
	Children     []IssueDraft  `json:"children"`
	Repository   RepositoryRef `json:"repository"`
	BaseCommit   string        `json:"baseCommit"`
	Tasks        []Task        `json:"tasks"`
	Protected    []string      `json:"protectedPaths"`
	Verification []string      `json:"verification"`
}

type IssueDraft struct {
	Key                string   `json:"key"`
	Title              string   `json:"title"`
	Body               string   `json:"body"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Labels             []string `json:"labels"`
}

type RepositoryRef struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
}

type Task struct {
	ID                 string   `json:"id"`
	IssueKey           string   `json:"issueKey"`
	Role               string   `json:"role"`
	Branch             string   `json:"branch"`
	AllowedPaths       []string `json:"allowedPaths"`
	DependsOn          []string `json:"dependsOn"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Verification       []string `json:"verification"`
}

type Violation struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type RunID string
type RunPhase string

const (
	PhaseRegistered    RunPhase = "registered"
	PhaseAnalyzing     RunPhase = "analyzing"
	PhaseBuilding      RunPhase = "building"
	PhaseIntegrating   RunPhase = "integrating"
	PhaseReviewing     RunPhase = "reviewing"
	PhaseCI            RunPhase = "ci"
	PhaseNeedsOperator RunPhase = "needs_operator"
	PhaseMerging       RunPhase = "merging"
	PhaseCompleted     RunPhase = "completed"
	PhaseBlocked       RunPhase = "blocked"
	PhasePaused        RunPhase = "paused"
)

type NextAction struct{ Kind, Label string }
type AgentView struct{ Name, Role, State, Summary string }
type GitHubView struct {
	ParentIssue, PullRequest int
	URL, CI                  string
}
type StatusView struct {
	ContractVersion int         `json:"contractVersion"`
	RunID           RunID       `json:"runId"`
	Phase           RunPhase    `json:"phase"`
	Summary         string      `json:"summary"`
	NextAction      *NextAction `json:"nextAction,omitempty"`
	Agents          []AgentView `json:"agents"`
	GitHub          GitHubView  `json:"github"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}
