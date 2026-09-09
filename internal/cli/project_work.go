package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/coordinator"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

var errWorkflowServiceMissing = errors.New("workflow service is not configured")

type workflowPauser interface {
	PauseWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
}

type workflowResumer interface {
	ResumeWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
}

type workflowReconcilerService interface {
	ReconcileWork(context.Context, contractv2.WorkID) (coordinator.ReconcileResult, error)
}

type workflowRunnerService interface {
	RunWork(context.Context, contractv2.WorkID, contractv2.Revision, contractv2.RequestID) (statev2.WorkSnapshot, error)
}

func runProjectWork(ctx context.Context, args []string, stdout, stderr io.Writer, service WorkflowService) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if !NeedsWorkflowDependencies(args) {
		printUsage(stderr)
		return 2
	}
	if service == nil {
		return reportWorkflowError(stderr, errWorkflowServiceMissing)
	}

	var (
		value any
		err   error
	)
	switch {
	case args[0] == "project" && args[1] == "register":
		path, revision, request, ok := parseWorkflowCreateArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		value, err = workflowRegister(ctx, path, revision, request, service)
	case args[0] == "project" && args[1] == "list":
		value, err = workflowList(ctx, service)
	case args[0] == "project" && args[1] == "status":
		value, err = service.Snapshot(ctx, time.Now().UTC())
	case args[0] == "work" && args[1] == "plan":
		path, revision, request, ok := parseWorkflowCreateArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		value, err = workflowPlan(ctx, path, revision, request, service)
	case args[0] == "work" && args[1] == "approve":
		id, revision, request, ok := parseWorkflowApproveArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		value, err = service.ApproveWork(ctx, id, revision, request)
	case args[0] == "work" && (args[1] == "pause" || args[1] == "resume"):
		id, revision, request, ok := parseWorkflowApproveArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		if args[1] == "pause" {
			control, ok := service.(workflowPauser)
			if !ok {
				return reportWorkflowError(stderr, errors.New("workflow pause is not configured"))
			}
			value, err = control.PauseWork(ctx, id, revision, request)
		} else {
			control, ok := service.(workflowResumer)
			if !ok {
				return reportWorkflowError(stderr, errors.New("workflow resume is not configured"))
			}
			value, err = control.ResumeWork(ctx, id, revision, request)
		}
	case args[0] == "work" && args[1] == "reconcile":
		id, ok := parseWorkflowReconcileArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		reconciler, ok := service.(workflowReconcilerService)
		if !ok {
			return reportWorkflowError(stderr, errors.New("workflow reconciler is not configured"))
		}
		if _, err = reconciler.ReconcileWork(ctx, id); err != nil {
			return reportWorkflowError(stderr, err)
		}
		value, err = service.Status(ctx, id)
	case args[0] == "work" && args[1] == "run":
		id, revision, request, ok := parseWorkflowRunArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		runner, ok := service.(workflowRunnerService)
		if !ok {
			return reportWorkflowError(stderr, errors.New("workflow runner is not configured"))
		}
		value, err = runner.RunWork(ctx, id, revision, request)
	case args[0] == "work" && args[1] == "status":
		value, err = service.Status(ctx, contractv2.WorkID(args[2]))
	}
	if err != nil {
		return reportWorkflowError(stderr, err)
	}
	if err := writeWorkflowJSON(stdout, value); err != nil {
		return reportWorkflowError(stderr, err)
	}
	return 0
}

func parseWorkflowRunArgs(args []string) (contractv2.WorkID, contractv2.Revision, contractv2.RequestID, bool) {
	if len(args) != 5 || !workflowIDArg(args[0]) || args[1] != "--expected-revision" || args[3] != "--request-id" || !workflowIDArg(args[4]) {
		return "", 0, "", false
	}
	revision, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil || revision == 0 {
		return "", 0, "", false
	}
	return contractv2.WorkID(args[0]), contractv2.Revision(revision), contractv2.RequestID(args[4]), true
}

func workflowIDArg(value string) bool {
	return nonFlagArg(value) && !strings.ContainsFunc(value, unicode.IsSpace)
}

func workflowRegister(ctx context.Context, path string, revision contractv2.Revision, request contractv2.RequestID, service WorkflowService) (registry.Project, error) {
	file, err := os.Open(path)
	if err != nil {
		return registry.Project{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return registry.Project{}, err
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return registry.Project{}, errors.New("project JSON must be an object")
	}
	projectDecoder := json.NewDecoder(bytes.NewReader(raw))
	projectDecoder.DisallowUnknownFields()
	var project registry.Project
	if err := projectDecoder.Decode(&project); err != nil {
		return registry.Project{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return registry.Project{}, errors.New("trailing JSON")
		}
		return registry.Project{}, err
	}
	return service.RegisterProject(ctx, project, revision, request)
}

func workflowList(ctx context.Context, service WorkflowService) (any, error) {
	projects, err := service.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if projects == nil {
		projects = []registry.Project{}
	}
	return struct {
		SchemaVersion int                `json:"schemaVersion"`
		Projects      []registry.Project `json:"projects"`
	}{SchemaVersion: 2, Projects: projects}, nil
}

func workflowPlan(ctx context.Context, path string, revision contractv2.Revision, request contractv2.RequestID, service WorkflowService) (statev2.WorkSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	defer file.Close()
	return service.PlanWork(ctx, path, file, revision, request)
}

func parseWorkflowCreateArgs(args []string) (string, contractv2.Revision, contractv2.RequestID, bool) {
	if len(args) != 5 || !nonFlagArg(args[0]) || args[1] != "--expected-revision" || args[3] != "--request-id" || !nonFlagArg(args[4]) {
		return "", 0, "", false
	}
	revision, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil || revision != 0 {
		return "", 0, "", false
	}
	return args[0], contractv2.Revision(revision), contractv2.RequestID(args[4]), true
}

func parseWorkflowApproveArgs(args []string) (contractv2.WorkID, contractv2.Revision, contractv2.RequestID, bool) {
	if len(args) != 5 || !nonFlagArg(args[0]) || args[1] != "--expected-revision" || args[3] != "--request-id" || !nonFlagArg(args[4]) {
		return "", 0, "", false
	}
	revision, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil || revision == 0 {
		return "", 0, "", false
	}
	return contractv2.WorkID(args[0]), contractv2.Revision(revision), contractv2.RequestID(args[4]), true
}

func parseWorkflowReconcileArgs(args []string) (contractv2.WorkID, bool) {
	if len(args) != 2 || !nonFlagArg(args[0]) || args[1] != "--json" {
		return "", false
	}
	return contractv2.WorkID(args[0]), true
}

func writeWorkflowJSON(stdout io.Writer, value any) error {
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(value); err != nil {
		return err
	}
	_, err := io.Copy(stdout, &encoded)
	return err
}

func reportWorkflowError(stderr io.Writer, _ error) int {
	// Do not expose provider, filesystem, or credential-shaped diagnostics from
	// the v2 boundary. Callers receive a bounded Korean message only.
	fmt.Fprintln(stderr, "프로젝트·워크플로 명령을 처리하지 못했습니다.")
	return 1
}
