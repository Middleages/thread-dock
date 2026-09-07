// Package contractv2 defines the immutable, provider-neutral workflow contract.
package contractv2

const CurrentVersion = 2

type ProjectID string
type WorkID string
type RepoKey string
type TaskID string
type RequestID string
type Revision uint64

type RepositoryIdentity struct {
	Host          string `json:"host"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
}

type CommandSpec struct {
	Argv           []string `json:"argv,omitempty"`
	CwdRepoKey     RepoKey  `json:"cwdRepoKey"`
	TimeoutSeconds uint32   `json:"timeoutSeconds"`
	ShellScript    string   `json:"shellScript,omitempty"`
}

type RepositoryPlan struct {
	RepoKey      RepoKey       `json:"repoKey"`
	BaseSHA      string        `json:"baseSha"`
	TargetBranch string        `json:"targetBranch"`
	Verification []CommandSpec `json:"verification,omitempty"`
	PROrder      int           `json:"prOrder,omitempty"`
	MergeOrder   int           `json:"mergeOrder,omitempty"`
}

type Task struct {
	TaskID             TaskID        `json:"taskId"`
	RepoKey            RepoKey       `json:"repoKey"`
	Owner              string        `json:"owner,omitempty"`
	Role               string        `json:"role,omitempty"`
	Branch             string        `json:"branch,omitempty"`
	AllowedPaths       []string      `json:"allowedPaths"`
	DependsOn          []TaskID      `json:"dependsOn,omitempty"`
	AcceptanceCriteria []string      `json:"acceptanceCriteria"`
	Verification       []CommandSpec `json:"verification,omitempty"`
}

type InterfaceAgreement struct {
	AgreementID  string    `json:"agreementId,omitempty"`
	Summary      string    `json:"summary"`
	RepoKeys     []RepoKey `json:"repoKeys,omitempty"`
	DecisionRefs []string  `json:"decisionRefs,omitempty"`
}

type IssueDraft struct {
	Key                string   `json:"key"`
	Title              string   `json:"title"`
	Body               string   `json:"body"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Labels             []string `json:"labels,omitempty"`
	RepoKey            RepoKey  `json:"repoKey"`
}

type DocumentationPlan struct {
	Required     bool     `json:"required"`
	Repository   RepoKey  `json:"repository,omitempty"`
	WikiTargets  []string `json:"wikiTargets,omitempty"`
	AllowedPaths []string `json:"allowedPaths,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

type ExecutionProfiles struct {
	Builder    string `json:"builder"`
	Reviewer   string `json:"reviewer"`
	Documenter string `json:"documenter"`
}

type WorkItemContract struct {
	Version               int                  `json:"version"`
	WorkID                WorkID               `json:"workId"`
	ProjectID             ProjectID            `json:"projectId"`
	Revision              Revision             `json:"revision"`
	Request               string               `json:"request"`
	AcceptanceCriteria    []string             `json:"acceptanceCriteria"`
	RepositoryPlans       []RepositoryPlan     `json:"repositoryPlans"`
	Tasks                 []Task               `json:"tasks"`
	InterfaceAgreements   []InterfaceAgreement `json:"interfaceAgreements"`
	CrossRepoVerification []CommandSpec        `json:"crossRepoVerification"`
	IssueDrafts           []IssueDraft         `json:"issueDrafts"`
	Documentation         DocumentationPlan    `json:"documentation"`
	ExecutionProfiles     ExecutionProfiles    `json:"executionProfiles"`
	DecisionRefs          []string             `json:"decisionRefs"`
}

type Violation struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}
