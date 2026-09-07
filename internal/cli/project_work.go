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
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/registry"
	statev2 "thread-dock/internal/state/v2"
)

var errWorkflowServiceMissing = errors.New("workflow service is not configured")

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
		value, err = workflowRegister(ctx, args[2], service)
	case args[0] == "project" && args[1] == "list":
		value, err = workflowList(ctx, service)
	case args[0] == "project" && args[1] == "status":
		value, err = service.Snapshot(ctx, time.Now().UTC())
	case args[0] == "work" && args[1] == "plan":
		value, err = workflowPlan(ctx, args[2], service)
	case args[0] == "work" && args[1] == "approve":
		id, revision, request, ok := parseWorkflowApproveArgs(args[2:])
		if !ok {
			printUsage(stderr)
			return 2
		}
		value, err = service.ApproveWork(ctx, id, revision, request)
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

func workflowRegister(ctx context.Context, path string, service WorkflowService) (registry.Project, error) {
	file, err := os.Open(path)
	if err != nil {
		return registry.Project{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var project registry.Project
	if err := decoder.Decode(&project); err != nil {
		return registry.Project{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return registry.Project{}, errors.New("trailing JSON")
		}
		return registry.Project{}, err
	}
	return service.RegisterProject(ctx, project)
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

func workflowPlan(ctx context.Context, path string, service WorkflowService) (statev2.WorkSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	defer file.Close()
	return service.PlanWork(ctx, path, file)
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
