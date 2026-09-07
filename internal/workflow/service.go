package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

type Service struct {
	projects registry.Store
	works    statev2.Store
}

func New(projects registry.Store, works statev2.Store) *Service {
	return &Service{projects: projects, works: works}
}
func (s *Service) RegisterProject(ctx context.Context, p registry.Project) (registry.Project, error) {
	return s.projects.Create(ctx, p)
}
func (s *Service) ListProjects(ctx context.Context) ([]registry.Project, error) {
	return s.projects.List(ctx)
}

type UnsupportedLegacyError struct {
	Code     string
	Source   string
	Location string
}

func (e *UnsupportedLegacyError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Location) }

func (s *Service) PlanWork(ctx context.Context, source string, r io.Reader) (statev2.WorkSnapshot, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	var envelope struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(data, &envelope) == nil && envelope.Version == 1 {
		return statev2.WorkSnapshot{}, &UnsupportedLegacyError{Code: "unsupported_legacy", Source: source, Location: source}
	}
	c, err := contractv2.Read(bytes.NewReader(data))
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if c.Revision != 1 {
		return statev2.WorkSnapshot{}, errors.New("plan contract revision must be 1")
	}
	p, err := s.projects.Load(ctx, c.ProjectID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	for _, plan := range c.RepositoryPlans {
		if _, ok := p.Repositories[plan.RepoKey]; !ok {
			return statev2.WorkSnapshot{}, fmt.Errorf("repository %q is not registered", plan.RepoKey)
		}
	}
	var canonical bytes.Buffer
	if err := contractv2.Write(&canonical, c); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	sum := sha256.Sum256(canonical.Bytes())
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: c.ProjectID, WorkID: c.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(sum[:]), Contract: c, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}}
	return s.works.CreatePlan(ctx, snapshot)
}

func (s *Service) ApproveWork(ctx context.Context, id contractv2.WorkID, expected contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	if request == "" {
		return statev2.WorkSnapshot{}, errors.New("request ID is required")
	}
	payload, err := json.Marshal(struct {
		WorkID   contractv2.WorkID   `json:"workId"`
		Expected contractv2.Revision `json:"expectedRevision"`
		Action   string              `json:"action"`
	}{id, expected, "approve"})
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	sum := sha256.Sum256(payload)
	return s.works.Mutate(ctx, statev2.Mutation{WorkID: id, ExpectedRevision: expected, RequestID: request, PayloadHash: hex.EncodeToString(sum[:]), Transition: func(s *statev2.WorkSnapshot) error {
		if s.State != statev2.StateAwaitingApproval {
			return errors.New("work is not awaiting approval")
		}
		s.State = statev2.StateQueued
		s.NextAction = "run"
		return nil
	}})
}
func (s *Service) Status(ctx context.Context, id contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.works.Load(ctx, id)
}
