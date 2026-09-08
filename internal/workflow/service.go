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
	snapshot, err := s.works.Load(ctx, id)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	transition := statev2.WorkTransition{Action: statev2.WorkApprove, ApprovalRef: string(request), ContractHash: snapshot.ContractHash}
	apply := statev2.TransitionRequest{WorkID: id, ExpectedRevision: expected, RequestID: request, Work: &transition}
	apply.PayloadHash, err = statev2.TransitionPayloadHash(apply)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	return s.works.Apply(ctx, apply)
}
func (s *Service) Status(ctx context.Context, id contractv2.WorkID) (statev2.WorkSnapshot, error) {
	return s.works.Load(ctx, id)
}

func (s *Service) Snapshot(ctx context.Context, at time.Time) (monitor.Snapshot, error) {
	if at.IsZero() || at.Location() != time.UTC {
		return monitor.Snapshot{}, errors.New("snapshot observation time must be a nonzero UTC time")
	}
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
		if err := normalizeProjectionState(&work); err != nil {
			return monitor.Snapshot{}, fmt.Errorf("work %q: %w", work.WorkID, err)
		}
		tasks := make([]monitor.TaskDetail, 0, len(work.Contract.Tasks))
		for _, task := range work.Contract.Tasks {
			taskState, ok := work.TaskStates[task.TaskID]
			if !ok {
				return monitor.Snapshot{}, fmt.Errorf("work %q is missing task state %q", work.WorkID, task.TaskID)
			}
			tasks = append(tasks, taskDetail(task.TaskID, task.RepoKey, taskState))
		}
		publications := make([]monitor.PublicationDetail, 0, len(work.Publications))
		for _, publication := range work.Publications {
			detail := monitor.PublicationDetail{IntentID: string(publication.IntentID), Key: string(publication.Key), Generation: publication.Generation, Kind: string(publication.Kind), Status: string(publication.Status), Attempts: publication.Attempts}
			if publication.Receipt != nil {
				detail.URL = publication.Receipt.URL
			}
			publications = append(publications, detail)
		}
		sort.Slice(publications, func(i, j int) bool {
			if publications[i].Key != publications[j].Key {
				return publications[i].Key < publications[j].Key
			}
			if publications[i].Generation != publications[j].Generation {
				return publications[i].Generation < publications[j].Generation
			}
			return publications[i].IntentID < publications[j].IntentID
		})
		updatedAt := workActivity(work)
		item := monitor.WorkItem{
			WorkID:       work.WorkID,
			Title:        title,
			Request:      work.Contract.Request,
			State:        string(work.State),
			SyncStatus:   work.SyncStatus,
			NextAction:   work.NextAction,
			EvidenceRefs: append([]string{}, work.EvidenceRefs...),
			Tasks:        tasks,
			Blocker:      blockerKind(work.Control.Blocker),
			Publications: publications,
			Decisions:    []monitor.DecisionDetail{},
			Handoffs:     []monitor.HandoffDetail{},
			Links:        []monitor.Link{},
			UpdatedAt:    updatedAt,
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
	var representative *monitor.WorkItem
	for i := range resultProjects {
		project := &resultProjects[i]
		if len(project.WorkItems) == 0 {
			project.SyncStatus = "current"
			project.WorkItems = []monitor.WorkItem{}
			continue
		}
		sort.SliceStable(project.WorkItems, func(i, j int) bool { return workItemBefore(project.WorkItems[i], project.WorkItems[j]) })
		first := project.WorkItems[0]
		for _, item := range project.WorkItems {
			if item.UpdatedAt.After(project.UpdatedAt) {
				project.UpdatedAt = item.UpdatedAt
			}
		}
		project.State = first.State
		project.SyncStatus = first.SyncStatus
		project.NextAction = first.NextAction
		project.EvidenceRefs = append([]string{}, first.EvidenceRefs...)
		if representative == nil || workItemBefore(first, *representative) {
			copy := first
			representative = &copy
		}
	}
	if representative == nil {
		result.State = string(statev2.StateDraft)
		result.SyncStatus = "current"
		result.NextAction = ""
	} else {
		result.State = representative.State
		result.SyncStatus = representative.SyncStatus
		result.NextAction = representative.NextAction
		result.EvidenceRefs = append([]string{}, representative.EvidenceRefs...)
	}
	result.Freshness = monitor.Freshness{State: "fresh", SyncStatus: result.SyncStatus, ObservedAt: at}
	result.Projects = resultProjects
	return result, nil
}

func normalizeProjectionState(work *statev2.WorkSnapshot) error {
	if work.TaskStates == nil && work.Publications == nil && work.Control == (statev2.WorkControl{}) && (work.State == statev2.StateAwaitingApproval || work.State == statev2.StateQueued) {
		work.TaskStates = make(map[contractv2.TaskID]statev2.TaskExecutionState, len(work.Contract.Tasks))
		for _, task := range work.Contract.Tasks {
			work.TaskStates[task.TaskID] = statev2.TaskExecutionState{TaskID: task.TaskID, Status: statev2.TaskPending, RepairLimit: statev2.DefaultRepairLimit, RecoveryLimit: statev2.DefaultRecoveryLimit, PriorAttempts: []statev2.AttemptSummary{}, InvocationHistory: []statev2.InvocationID{}}
		}
		work.Publications = map[statev2.PublicationIntentID]statev2.PublicationState{}
	}
	if work.TaskStates == nil || work.Publications == nil {
		return errors.New("typed task, publication, and control state is required")
	}
	for _, task := range work.Contract.Tasks {
		if _, ok := work.TaskStates[task.TaskID]; !ok {
			return fmt.Errorf("is missing task state %q", task.TaskID)
		}
	}
	return nil
}

func taskDetail(id contractv2.TaskID, repo contractv2.RepoKey, task statev2.TaskExecutionState) monitor.TaskDetail {
	verification, review, merge := "pending", "pending", "pending"
	if task.Gate != nil {
		if task.Gate.Passed {
			verification = "passed"
		} else {
			verification = "failed"
		}
	} else if task.Status == statev2.TaskGateFailed {
		verification = "failed"
	} else if task.Status == statev2.TaskGatePassed || task.Status == statev2.TaskAccepted || task.Status == statev2.TaskIntegrated {
		verification = "passed"
	}
	if task.Review != nil {
		if task.Review.Accepted {
			review = "accepted"
		} else {
			review = "blocked"
		}
	} else if task.Status == statev2.TaskAccepted || task.Status == statev2.TaskIntegrated {
		review = "accepted"
	} else if task.Status == statev2.TaskReviewBlocked {
		review = "blocked"
	}
	if task.Integration != nil || task.Status == statev2.TaskIntegrated {
		merge = "integrated"
	}
	return monitor.TaskDetail{TaskID: id, RepoKey: repo, State: string(task.Status), Verification: verification, Review: review, Merge: merge}
}

func blockerKind(blocker *statev2.OperatorBlocker) string {
	if blocker == nil {
		return ""
	}
	return blocker.Kind
}

func workActivity(work statev2.WorkSnapshot) time.Time {
	var latest time.Time
	for _, task := range work.TaskStates {
		if task.Invocation != nil {
			if task.Invocation.StartedAt != nil && task.Invocation.StartedAt.After(latest) {
				latest = *task.Invocation.StartedAt
			}
			if task.Invocation.EndedAt != nil && task.Invocation.EndedAt.After(latest) {
				latest = *task.Invocation.EndedAt
			}
		}
		for _, observed := range []time.Time{gateObserved(task), reviewObserved(task), integrationObserved(task)} {
			if observed.After(latest) {
				latest = observed
			}
		}
	}
	for _, publication := range work.Publications {
		if publication.Receipt != nil && publication.Receipt.PublishedAt.After(latest) {
			latest = publication.Receipt.PublishedAt
		}
	}
	return latest
}

func gateObserved(task statev2.TaskExecutionState) time.Time {
	if task.Gate != nil {
		return task.Gate.ObservedAt
	}
	return time.Time{}
}
func reviewObserved(task statev2.TaskExecutionState) time.Time {
	if task.Review != nil {
		return task.Review.ObservedAt
	}
	return time.Time{}
}
func integrationObserved(task statev2.TaskExecutionState) time.Time {
	if task.Integration != nil {
		return task.Integration.ObservedAt
	}
	return time.Time{}
}

func workItemPriority(state string) int {
	switch state {
	case string(statev2.StateNeedsOperator):
		return 0
	case string(statev2.StatePaused):
		return 1
	case string(statev2.StatePublicationPending):
		return 2
	case string(statev2.StateReview):
		return 3
	case string(statev2.StateRunning):
		return 4
	case string(statev2.StateReadyForPR):
		return 5
	case string(statev2.StateQueued):
		return 6
	case string(statev2.StateCompleted):
		return 7
	case string(statev2.StateDraft), string(statev2.StateAwaitingApproval):
		return 8
	default:
		return 9
	}
}

func workItemBefore(a, b monitor.WorkItem) bool {
	if workItemPriority(a.State) != workItemPriority(b.State) {
		return workItemPriority(a.State) < workItemPriority(b.State)
	}
	if !a.UpdatedAt.Equal(b.UpdatedAt) {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	return a.WorkID < b.WorkID
}
