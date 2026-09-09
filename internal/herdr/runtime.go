package herdr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"thread-dock/internal/coordinator"
	"thread-dock/internal/opencodeagent"
	runtimecontract "thread-dock/internal/runtime"
	statev2 "thread-dock/internal/state/v2"
)

var (
	// ErrExactTerminationUnsupported is returned because Herdr 0.8.2 has no
	// operation that proves that one exact invocation stopped.
	ErrExactTerminationUnsupported = errors.New("exact Herdr invocation termination is unsupported")
	ErrRuntimeConfiguration        = errors.New("invalid Herdr Builder runtime configuration")
	ErrRuntimeIdentity             = errors.New("Herdr Builder provider identity mismatch")
	ErrRuntimeEvidence             = errors.New("Herdr Builder evidence is unavailable or malformed")
	ErrRuntimePrompt               = errors.New("Herdr Builder prompt delivery is unconfirmed")
)

// PromptError carries only a validated, bounded Herdr prompt failure code.
// Provider messages and command output are deliberately not retained.
type PromptError struct {
	code     string
	exitCode int
}

func (e *PromptError) Code() string {
	if e == nil {
		return ""
	}
	return e.code
}

func (e *PromptError) Error() string {
	if e == nil {
		return safeError("agent prompt", 0).Error()
	}
	return safeError("agent prompt", e.exitCode).Error()
}

// ProfileBinding pins a logical runtime profile to a native OpenCode agent and
// the fingerprint that was approved with the durable invocation.
type ProfileBinding struct {
	OpenCodeAgent      string
	RuntimeFingerprint string
}

type builderClient interface {
	OpenWorktree(context.Context, OpenWorktreeRequest) (Worktree, error)
	StartAgent(context.Context, StartAgentRequest) error
	Prompt(context.Context, string, string) error
	GetInfo(context.Context, string) (AgentInfo, error)
	ReadEvidence(context.Context, string) (Evidence, error)
}

// Runtime is a deliberately thin adapter over the existing Herdr CLI.
type Runtime struct {
	client   builderClient
	profiles map[string]ProfileBinding
}

// NewBuilderRuntime constructs a Builder-only Herdr runtime. The profile map
// is copied so a caller cannot change native routing during an invocation.
func NewBuilderRuntime(client builderClient, profiles map[string]ProfileBinding) *Runtime {
	copied := make(map[string]ProfileBinding, len(profiles))
	for profile, binding := range profiles {
		copied[profile] = binding
	}
	return &Runtime{client: client, profiles: copied}
}

func (r *Runtime) Launch(ctx context.Context, state statev2.InvocationState, worktree statev2.WorktreeIdentity, invocation runtimecontract.Invocation) (string, error) {
	if !state.LaunchRequested || state.ProviderIdentity != "" || state.ProviderSession != "" || state.ProviderPane != "" || state.ProviderProcess != "" {
		return "", ErrRuntimeConfiguration
	}
	binding, name, err := r.validate(state, worktree, invocation)
	if err != nil {
		return "", err
	}
	if r.client == nil {
		return "", ErrRuntimeConfiguration
	}
	if existing, probeErr := r.client.GetInfo(ctx, name); probeErr == nil {
		_ = existing
		return "", ErrRuntimeIdentity
	} else if !errors.Is(probeErr, ErrAgentNotFound) {
		return "", ErrRuntimeIdentity
	}
	cwd := filepath.Dir(worktree.GitCommonDir)
	if worktree.GitCommonDir == "" {
		cwd = filepath.Dir(worktree.CanonicalPath)
	}
	opened, err := r.client.OpenWorktree(ctx, OpenWorktreeRequest{
		Cwd: cwd, Path: worktree.CanonicalPath, Label: name,
	})
	if err != nil || !sameWorktree(opened, worktree) {
		return "", ErrRuntimeIdentity
	}
	if err := r.client.StartAgent(ctx, StartAgentRequest{Name: name, PaneID: opened.PaneID, OpenCodeAgent: binding.OpenCodeAgent}); err != nil {
		return "", ErrRuntimeIdentity
	}
	info, err := r.client.GetInfo(ctx, name)
	if err != nil {
		return "", ErrRuntimeIdentity
	}
	identity, err := exactIdentity(info, name, opened)
	if err != nil {
		return "", err
	}
	if err := r.client.Prompt(ctx, name, promptFor(invocation)); err != nil {
		// Prompt receipt is deliberately not retried: a second prompt could
		// start a second turn against the same invocation.
		var promptErr *PromptError
		if errors.As(err, &promptErr) && allowedPromptCode(promptErr.Code()) {
			return "", fmt.Errorf("%w: %w", ErrRuntimePrompt, promptErr)
		}
		return "", ErrRuntimePrompt
	}
	return identity, nil
}

func (r *Runtime) Observe(ctx context.Context, state statev2.InvocationState, invocation runtimecontract.Invocation) (coordinator.RuntimeObservation, error) {
	if state.ProviderIdentity == "" && !state.LaunchRequested {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, Diagnostic: "provider identity is not persisted"}, ErrRuntimeIdentity
	}
	worktree := statev2.WorktreeIdentity{CanonicalPath: invocation.Worktree}
	binding, name, err := r.validate(state, worktree, invocation)
	if err != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, Diagnostic: "invalid runtime identity"}, err
	}
	_ = binding
	if r.client == nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, Diagnostic: "runtime is not configured"}, ErrRuntimeConfiguration
	}
	info, err := r.client.GetInfo(ctx, name)
	if err != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, Diagnostic: "provider identity could not be observed"}, nil
	}
	opened := Worktree{WorkspaceID: "", PaneID: info.PaneID, Path: invocation.Worktree}
	identity, err := exactIdentity(info, name, opened)
	if err != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, Diagnostic: "provider identity changed"}, nil
	}
	if state.ProviderIdentity != "" && state.ProviderIdentity != identity {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "provider identity changed"}, nil
	}

	evidence, evidenceErr := r.client.ReadEvidence(ctx, name)
	post, postErr := r.client.GetInfo(ctx, name)
	if postErr != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "provider identity could not be re-observed"}, nil
	}
	postIdentity, identityErr := exactIdentity(post, name, opened)
	if identityErr != nil || postIdentity != identity {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "provider identity changed during evidence read"}, nil
	}
	if evidenceErr != nil {
		if isEvidenceAbsent(evidenceErr) {
			if info.State == AgentStateWorking || info.State == AgentStateBlocked || post.State == AgentStateWorking || post.State == AgentStateBlocked {
				return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationActive, ProviderIdentity: identity}, nil
			}
			return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "current builder evidence is not available"}, nil
		}
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "builder evidence is malformed or stale"}, nil
	}
	if evidence.RequestID != string(invocation.RequestID) {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "builder evidence request does not match invocation"}, nil
	}
	if info.State == AgentStateWorking || info.State == AgentStateBlocked || post.State == AgentStateWorking || post.State == AgentStateBlocked {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationActive, ProviderIdentity: identity}, nil
	}
	if info.State != AgentStateIdle && info.State != AgentStateDone {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "provider state is unknown"}, nil
	}
	if post.State != AgentStateIdle && post.State != AgentStateDone {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "provider state is unknown"}, nil
	}
	result := runtimecontract.BuilderResult{CommitSHA: evidence.CommitSHA}
	for _, check := range evidence.Verification {
		result.Verification = append(result.Verification, runtimecontract.VerificationResult{Command: check.Command, Outcome: check.Outcome, Duration: check.Duration})
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity}, ErrRuntimeEvidence
	}
	if _, err := runtimecontract.DecodeBuilderResult(encoded); err != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "builder evidence is malformed"}, nil
	}
	artifact := &runtimecontract.ArtifactEnvelope{RequestID: invocation.RequestID, Role: runtimecontract.RoleBuilder, Status: "success", Result: encoded}
	if err := runtimecontract.ValidateEnvelope(invocation, *artifact); err != nil {
		return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationUnknown, ProviderIdentity: identity, Diagnostic: "builder artifact is invalid"}, nil
	}
	return coordinator.RuntimeObservation{State: coordinator.RuntimeObservationEnded, ProviderIdentity: identity, Artifact: artifact}, nil
}

func (r *Runtime) Terminate(context.Context, statev2.InvocationState) error {
	return ErrExactTerminationUnsupported
}

func (r *Runtime) validate(state statev2.InvocationState, worktree statev2.WorktreeIdentity, invocation runtimecontract.Invocation) (ProfileBinding, string, error) {
	if r == nil || r.profiles == nil || invocation.Role != runtimecontract.RoleBuilder || state.Role != string(runtimecontract.RoleBuilder) || invocation.ReadOnly || invocation.OutputSchema != runtimecontract.BuilderOutputSchema || invocation.RequestID == "" || string(state.InvocationID) != string(invocation.RequestID) || state.LogicalProfile != invocation.ProfileID || invocation.Worktree == "" || filepath.Clean(invocation.Worktree) != invocation.Worktree || !filepath.IsAbs(invocation.Worktree) || worktree.CanonicalPath != invocation.Worktree || len(invocation.Packet) == 0 || len(invocation.Packet) > 32*1024 || !json.Valid(invocation.Packet) {
		return ProfileBinding{}, "", ErrRuntimeConfiguration
	}
	binding, ok := r.profiles[invocation.ProfileID]
	if !ok || strings.TrimSpace(binding.OpenCodeAgent) == "" || strings.TrimSpace(binding.RuntimeFingerprint) == "" || binding.RuntimeFingerprint != state.RuntimeFingerprint || !opencodeagent.ValidName(binding.OpenCodeAgent) {
		return ProfileBinding{}, "", ErrRuntimeConfiguration
	}
	if worktree.GitCommonDir != "" && (filepath.Clean(worktree.GitCommonDir) != worktree.GitCommonDir || !filepath.IsAbs(worktree.GitCommonDir)) {
		return ProfileBinding{}, "", ErrRuntimeConfiguration
	}
	return binding, deterministicAgentName(string(invocation.RequestID), invocation.Worktree), nil
}

func deterministicAgentName(requestID, path string) string {
	hash := sha256.Sum256([]byte(string(requestID) + "\x00" + path))
	return "td-builder-" + hex.EncodeToString(hash[:])[:20]
}

func sameWorktree(got Worktree, want statev2.WorktreeIdentity) bool {
	return got.WorkspaceID != "" && got.PaneID != "" && got.Path == want.CanonicalPath
}

type providerIdentity struct {
	Name      string `json:"name"`
	SessionID string `json:"session"`
	PaneID    string `json:"pane"`
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
}

func exactIdentity(info AgentInfo, name string, worktree Worktree) (string, error) {
	if info.Name != name || info.SessionID == "" || info.PaneID == "" || info.WorkspaceID == "" || info.Path == "" || info.Path != worktree.Path || (worktree.PaneID != "" && info.PaneID != worktree.PaneID) || (worktree.WorkspaceID != "" && info.WorkspaceID != worktree.WorkspaceID) {
		return "", ErrRuntimeIdentity
	}
	identity, err := json.Marshal(providerIdentity{Name: info.Name, SessionID: info.SessionID, PaneID: info.PaneID, Workspace: info.WorkspaceID, Path: info.Path})
	if err != nil {
		return "", ErrRuntimeIdentity
	}
	return string(identity), nil
}

func promptFor(invocation runtimecontract.Invocation) string {
	packet := string(invocation.Packet)
	return fmt.Sprintf("ThreadDock Builder invocation requestId=%s. Return one strict marked evidence envelope.\n%s\n%s\nrequestId=%s\nInvocation packet:\n%s", invocation.RequestID, EvidenceBeginMarker+" ... "+EvidenceEndMarker, EvidenceSchemaExample, invocation.RequestID, packet)
}

func isEvidenceAbsent(err error) bool {
	return strings.Contains(err.Error(), "no complete envelope")
}
