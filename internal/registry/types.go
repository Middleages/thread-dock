package registry

import (
	"context"

	contractv2 "thread-dock/internal/contract/v2"
)

// Project is the non-secret, versioned identity of one product or system.
type Project struct {
	ProjectID      contractv2.ProjectID                                 `json:"projectId"`
	Name           string                                               `json:"name"`
	PrimaryRepoKey contractv2.RepoKey                                   `json:"primaryRepoKey"`
	Repositories   map[contractv2.RepoKey]contractv2.RepositoryIdentity `json:"repositories"`
}

type Store interface {
	Create(context.Context, Project) (Project, error)
	Load(context.Context, contractv2.ProjectID) (Project, error)
	List(context.Context) ([]Project, error)
}
