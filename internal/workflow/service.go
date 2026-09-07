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
	"sort"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/monitor"
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
func (s *Service) RegisterProject(ctx context.Context, p registry.Project, expected contractv2.Revision, request contractv2.RequestID) (registry.Project, error) {
	if expected != 0 {
		return registry.Project{}, errors.New("project creation expected revision must be 0")
	}
	if request == "" {
		return registry.Project{}, errors.New("request ID is required")
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return registry.Project{}, err
	}
	sum := sha256.Sum256(payload)
	return s.projects.Create(ctx, p, expected, request, hex.EncodeToString(sum[:]))
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

func (s *Service) PlanWork(ctx context.Context, source string, r io.Reader, expected contractv2.Revision, request contractv2.RequestID) (statev2.WorkSnapshot, error) {
	if expected != 0 {
		return statev2.WorkSnapshot{}, errors.New("plan creation expected revision must be 0")
	}
	if request == "" {
		return statev2.WorkSnapshot{}, errors.New("request ID is required")
	}
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
	taskStates := make(map[contractv2.TaskID]statev2.TaskExecutionState, len(c.Tasks))
	for _, task := range c.Tasks {
		taskStates[task.TaskID] = statev2.TaskExecutionState{
			TaskID:        task.TaskID,
			Status:        statev2.TaskPending,
			RepairLimit:   statev2.DefaultRepairLimit,
			RecoveryLimit: statev2.DefaultRecoveryLimit,
			PriorAttempts: []statev2.AttemptSummary{},
		}
	}
	snapshot := statev2.WorkSnapshot{SchemaVersion: 2, ProjectID: c.ProjectID, WorkID: c.WorkID, Revision: 1, State: statev2.StateAwaitingApproval, ContractHash: hex.EncodeToString(sum[:]), Contract: c, SyncStatus: "local", NextAction: "approve", EvidenceRefs: []string{}, Receipts: map[contractv2.RequestID]statev2.Receipt{}, Control: statev2.WorkControl{}, TaskStates: taskStates, Publications: map[statev2.PublicationIntentID]statev2.PublicationState{}}
	action, err := json.Marshal(struct {
		WorkID           contractv2.WorkID   `json:"workId"`
		ExpectedRevision contractv2.Revision `json:"expectedRevision"`
		Action           string              `json:"action"`
		ContractHash     string              `json:"contractHash"`
	}{c.WorkID, expected, "plan", hex.EncodeToString(sum[:])})
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	actionSum := sha256.Sum256(action)
	return s.works.CreatePlan(ctx, snapshot, request, hex.EncodeToString(actionSum[:]))
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

func (s *Service) Snapshot(ctx context.Context, at time.Time) (monitor.Snapshot, error) {
	projects, err := s.projects.List(ctx)
	if err != nil {
		return monitor.Snapshot{}, err
	}
	works, err := s.works.List(ctx)
	if err != nil {
		return monitor.Snapshot{}, err
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ProjectID < projects[j].ProjectID })
	sort.Slice(works, func(i, j int) bool { return works[i].WorkID < works[j].WorkID })

	projectIndex := make(map[contractv2.ProjectID]int, len(projects))
	resultProjects := make([]monitor.Project, len(projects))
	for i, project := range projects {
		projectIndex[project.ProjectID] = i
		resultProjects[i] = monitor.Project{
			ProjectID:    project.ProjectID,
			Name:         project.Name,
			State:        string(statev2.StateDraft),
			EvidenceRefs: []string{},
			WorkItems:    []monitor.WorkItem{},
		}
	}

	var revision contractv2.Revision
	for _, work := range works {
		projectPosition, ok := projectIndex[work.ProjectID]
		if !ok {
			return monitor.Snapshot{}, fmt.Errorf("work %q references unregistered project %q", work.WorkID, work.ProjectID)
		}
		if work.Revision > revision {
			revision = work.Revision
		}
		title := string(work.WorkID)
		if len(work.Contract.IssueDrafts) > 0 && strings.TrimSpace(work.Contract.IssueDrafts[0].Title) != "" {
			title = work.Contract.IssueDrafts[0].Title
		}
		tasks := make([]monitor.TaskDetail, 0, len(work.Contract.Tasks))
		for _, task := range work.Contract.Tasks {
			tasks = append(tasks, monitor.TaskDetail{TaskID: task.TaskID, RepoKey: task.RepoKey})
		}
		item := monitor.WorkItem{
			WorkID:       work.WorkID,
			Title:        title,
			Request:      work.Contract.Request,
			State:        string(work.State),
			SyncStatus:   work.SyncStatus,
			NextAction:   work.NextAction,
			EvidenceRefs: append([]string{}, work.EvidenceRefs...),
			Tasks:        tasks,
		}
		resultProjects[projectPosition].WorkItems = append(resultProjects[projectPosition].WorkItems, item)
	}

	result := monitor.Snapshot{
		SchemaVersion: 2,
		Revision:      revision,
		ObservedAt:    at,
		EvidenceRefs:  []string{},
		Projects:      resultProjects,
	}
	for i := range resultProjects {
		project := &resultProjects[i]
		if len(project.WorkItems) == 0 {
			continue
		}
		first := project.WorkItems[0]
		project.State = first.State
		project.SyncStatus = first.SyncStatus
		project.NextAction = first.NextAction
		project.EvidenceRefs = append([]string{}, first.EvidenceRefs...)
		if result.State == "" {
			result.State = first.State
			result.SyncStatus = first.SyncStatus
			result.NextAction = first.NextAction
			result.EvidenceRefs = append([]string{}, first.EvidenceRefs...)
		}
	}
	if result.State == "" {
		result.State = string(statev2.StateDraft)
	}
	result.Freshness = monitor.Freshness{State: result.State, SyncStatus: result.SyncStatus, ObservedAt: at}
	result.Projects = resultProjects
	return result, nil
}
