package workrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	contractv2 "thread-dock/internal/contract/v2"
	"thread-dock/internal/runner"
	statev2 "thread-dock/internal/state/v2"
	"thread-dock/internal/worktree"
)

// CandidateVerifier runs the immutable, approved checks for a builder
// candidate before any later runtime or publication action.
type CandidateVerifier interface {
	VerifyCandidate(context.Context, statev2.WorkSnapshot, contractv2.TaskID) (statev2.WorkSnapshot, error)
}

// VerificationWorktreeGit is deliberately limited to the existing managed
// worktree identity and cleanliness inspection.
type VerificationWorktreeGit interface {
	InspectTaskWorktree(context.Context, string, string, string, string) (worktree.TaskWorktreeInspection, error)
}

type VerificationService struct {
	state       State
	process     runner.Runner
	git         VerificationWorktreeGit
	operatorRef string
	now         func() time.Time
}

func NewVerificationService(state State, process runner.Runner, git VerificationWorktreeGit, operatorRef string, now func() time.Time) *VerificationService {
	if now == nil {
		now = time.Now
	}
	return &VerificationService{state: state, process: process, git: git, operatorRef: operatorRef, now: now}
}

func (s *VerificationService) VerifyCandidate(ctx context.Context, supplied statev2.WorkSnapshot, taskID contractv2.TaskID) (statev2.WorkSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.state == nil || s.process == nil || s.git == nil {
		return statev2.WorkSnapshot{}, errors.New("verification dependencies are required")
	}
	if strings.TrimSpace(s.operatorRef) == "" {
		return statev2.WorkSnapshot{}, errors.New("operator reference is required")
	}
	if err := ctx.Err(); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	current, err := s.state.Load(ctx, supplied.WorkID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	if err := validateVerificationSnapshot(supplied, current, taskID); err != nil {
		return statev2.WorkSnapshot{}, err
	}
	task, commandSpecs, repositoryPath, repoKey, err := verificationInputs(current, taskID)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	labels := make([]string, len(commandSpecs))
	var bindingErr error
	for i, spec := range commandSpecs {
		labels[i], err = verificationCommandLabel(spec, repoKey)
		if err != nil {
			labels[i] = fmt.Sprintf("invalid-command:%d", i+1)
			if bindingErr == nil {
				bindingErr = err
			}
		}
	}
	if len(commandSpecs) == 0 {
		return statev2.WorkSnapshot{}, errors.New("verification commands are required")
	}

	inspection, inspectErr := s.git.InspectTaskWorktree(ctx, repositoryPath, task.Worktree.CanonicalPath, task.Worktree.Branch, task.Candidate.CandidateSHA)
	preOK := bindingErr == nil && inspectErr == nil && inspection.Exists && inspection.IdentityMatches
	outcomes := make([]string, len(labels))
	for i := range outcomes {
		outcomes[i] = "not_run"
	}
	passed := preOK
	var processErr error
	if preOK {
		for i, spec := range commandSpecs {
			result, runErr := runVerificationCommand(ctx, s.process, task.Worktree.CanonicalPath, spec)
			if runErr != nil {
				if result.ExitCode > 0 || errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, context.Canceled) {
					outcomes[i] = "failed"
				} else {
					outcomes[i] = "failed"
					processErr = runErr
				}
				passed = false
				break
			}
			if result.ExitCode != 0 {
				outcomes[i] = "failed"
				passed = false
				break
			}
			outcomes[i] = "passed"
		}
	}
	post, postErr := s.git.InspectTaskWorktree(ctx, repositoryPath, task.Worktree.CanonicalPath, task.Worktree.Branch, task.Candidate.CandidateSHA)
	if postErr != nil || !post.Exists || !post.IdentityMatches {
		passed = false
	}
	diagnostic := ""
	switch {
	case bindingErr != nil:
		diagnostic = "verification command binding invalid"
	case inspectErr != nil || !preOK:
		diagnostic = "verification worktree precondition failed"
	case postErr != nil || !post.Exists || !post.IdentityMatches:
		diagnostic = "verification worktree changed"
	case processErr != nil:
		diagnostic = "verification command execution failed"
	case !passed:
		diagnostic = "verification command failed"
	}
	resultSnapshot, applyErr := s.recordVerificationGate(ctx, current, taskID, labels, outcomes, passed, diagnostic)
	if applyErr != nil {
		return statev2.WorkSnapshot{}, applyErr
	}
	if processErr != nil {
		return resultSnapshot, errors.New("verification command execution failed")
	}
	return resultSnapshot, nil
}

func validateVerificationSnapshot(supplied, current statev2.WorkSnapshot, taskID contractv2.TaskID) error {
	if supplied.WorkID == "" || supplied.WorkID != current.WorkID || supplied.ProjectID != current.ProjectID || supplied.Revision != current.Revision || supplied.State != current.State || supplied.ContractHash != current.ContractHash || !reflect.DeepEqual(supplied.Contract, current.Contract) || !reflect.DeepEqual(supplied.Control, current.Control) {
		return errors.New("verification snapshot is stale")
	}
	provided, providedOK := supplied.TaskStates[taskID]
	actual, actualOK := current.TaskStates[taskID]
	if !providedOK || !actualOK || !reflect.DeepEqual(provided, actual) || provided.Candidate == nil || actual.Candidate == nil || provided.Worktree == nil || actual.Worktree == nil {
		return errors.New("verification candidate is not current")
	}
	if actual.Status != statev2.TaskCandidateReady || actual.Candidate.BuilderAttempt != actual.BuilderAttempt || actual.Candidate.CandidateSHA == "" || actual.Worktree.CanonicalPath == "" || actual.Worktree.Branch == "" {
		return errors.New("task is not ready for verification")
	}
	if current.Control.ApprovedContractHash == "" || current.Control.ApprovedContractHash != current.ContractHash || current.Control.ApprovalRef == "" {
		return errors.New("verification contract is not approved")
	}
	return nil
}

func verificationInputs(snapshot statev2.WorkSnapshot, taskID contractv2.TaskID) (statev2.TaskExecutionState, []contractv2.CommandSpec, string, contractv2.RepoKey, error) {
	taskContract, found := contractv2.Task{}, false
	for _, candidate := range snapshot.Contract.Tasks {
		if candidate.TaskID == taskID {
			taskContract, found = candidate, true
			break
		}
	}
	if !found {
		return statev2.TaskExecutionState{}, nil, "", "", errors.New("verification task is not in contract")
	}
	var plan *contractv2.RepositoryPlan
	for i := range snapshot.Contract.RepositoryPlans {
		if snapshot.Contract.RepositoryPlans[i].RepoKey == taskContract.RepoKey {
			plan = &snapshot.Contract.RepositoryPlans[i]
			break
		}
	}
	if plan == nil {
		return statev2.TaskExecutionState{}, nil, "", "", errors.New("verification repository is not in contract")
	}
	state := snapshot.TaskStates[taskID]
	return state, taskContract.Verification, filepath.Dir(state.Worktree.GitCommonDir), taskContract.RepoKey, nil
}

func verificationCommandLabel(spec contractv2.CommandSpec, repoKey contractv2.RepoKey) (string, error) {
	if spec.CwdRepoKey != repoKey || spec.TimeoutSeconds == 0 || (len(spec.Argv) == 0) == (strings.TrimSpace(spec.ShellScript) == "") {
		return "", errors.New("invalid command binding")
	}
	if len(spec.Argv) > 0 {
		if strings.TrimSpace(spec.Argv[0]) == "" {
			return "", errors.New("invalid executable")
		}
		encoded, err := json.Marshal(spec.Argv)
		if err != nil {
			return "", err
		}
		return "argv:" + string(encoded), nil
	}
	if strings.TrimSpace(spec.ShellScript) == "" || strings.TrimSpace(spec.ShellScript) != spec.ShellScript {
		return "", errors.New("invalid shell script")
	}
	encoded, err := json.Marshal(spec.ShellScript)
	if err != nil {
		return "", err
	}
	return "shell:" + string(encoded), nil
}

func runVerificationCommand(ctx context.Context, process runner.Runner, cwd string, spec contractv2.CommandSpec) (runner.Result, error) {
	child, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
	defer cancel()
	var result runner.Result
	var err error
	if len(spec.Argv) > 0 {
		result, err = process.Run(child, cwd, spec.Argv[0], spec.Argv[1:]...)
	} else {
		result, err = process.Run(child, cwd, "bash", "-lc", spec.ShellScript)
	}
	if childErr := child.Err(); childErr != nil {
		return result, childErr
	}
	return result, err
}

func (s *VerificationService) recordVerificationGate(ctx context.Context, snapshot statev2.WorkSnapshot, taskID contractv2.TaskID, commands, outcomes []string, passed bool, diagnostic string) (statev2.WorkSnapshot, error) {
	requestID, err := freshVerificationRequestID()
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	at := s.now().UTC()
	transition := statev2.TaskTransition{TaskID: taskID, Action: statev2.TaskRecordGate, Role: "builder", BuilderAttempt: snapshot.TaskStates[taskID].BuilderAttempt, At: at, Gate: &statev2.GateEvidence{BuilderAttempt: snapshot.TaskStates[taskID].BuilderAttempt, CandidateSHA: snapshot.TaskStates[taskID].Candidate.CandidateSHA, Commands: commands, Outcomes: outcomes, Passed: passed, ObservedAt: at, Diagnostic: boundedVerificationDiagnostic(diagnostic)}}
	request := statev2.TransitionRequest{WorkID: snapshot.WorkID, ExpectedRevision: snapshot.Revision, RequestID: requestID, Task: &transition}
	request.PayloadHash, err = statev2.TransitionPayloadHash(request)
	if err != nil {
		return statev2.WorkSnapshot{}, err
	}
	return s.state.Apply(ctx, request)
}

func freshVerificationRequestID() (contractv2.RequestID, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("create verification request ID: %w", err)
	}
	return contractv2.RequestID("verification-" + hex.EncodeToString(buf)), nil
}

func boundedVerificationDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "verification completed"
	}
	if len(value) > statev2.MaxDiagnosticBytes {
		return value[:statev2.MaxDiagnosticBytes]
	}
	return value
}
