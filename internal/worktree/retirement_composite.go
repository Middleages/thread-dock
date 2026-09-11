package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// CompositeRetirementInspector routes retirement proof and removal to the
// adapter whose trusted root owns the canonical checkout. State-managed
// Reviewer/Integration paths use managedGit; Builder paths use herdrGit.
// Keeping this choice in one adapter prevents a caller from accidentally
// proving a path with one root and removing it through another.
type CompositeRetirementInspector struct {
	managedGit  *Git
	herdrGit    *Git
	managedRoot string
	herdrRoot   string
}

func NewCompositeRetirementInspector(managedGit, herdrGit *Git, managedRoot, herdrRoot string) *CompositeRetirementInspector {
	return &CompositeRetirementInspector{managedGit: managedGit, herdrGit: herdrGit, managedRoot: managedRoot, herdrRoot: herdrRoot}
}

// InspectRetirementTarget implements the orchestrator's provider-neutral
// inspection port. With no role in that port, ownership is selected only when
// the exact canonical path belongs to exactly one trusted root.
func (c *CompositeRetirementInspector) InspectRetirementTarget(ctx context.Context, repositoryPath, worktreePath, expectedBranch, expectedSHA string) (RetirementProof, error) {
	adapter, canonicalPath, _, err := c.selectAdapter(ctx, worktreePath, "", false)
	if err != nil {
		return RetirementProof{}, err
	}
	return adapter.InspectRetirementTarget(ctx, repositoryPath, canonicalPath, expectedBranch, expectedSHA)
}

// InspectRetirementTargetForRole is used by role-aware callers that need the
// stronger Reviewer/Integration-versus-Builder root contract.
func (c *CompositeRetirementInspector) InspectRetirementTargetForRole(ctx context.Context, repositoryPath, role, worktreePath, expectedBranch, expectedSHA string) (RetirementProof, error) {
	adapter, canonicalPath, selectedRoot, err := c.selectAdapter(ctx, worktreePath, role, false)
	if err != nil {
		return RetirementProof{}, err
	}
	if !c.roleOwnsRoot(role, selectedRoot) {
		return RetirementProof{}, ErrUnsafeTarget
	}
	return adapter.InspectRetirementTarget(ctx, repositoryPath, canonicalPath, expectedBranch, expectedSHA)
}

// RemoveRetired performs the same root selection before invoking the Git
// adapter's final proof-and-remove operation. A missing leaf is permitted for
// idempotent response-loss reconciliation, but symlinked parents are not.
func (c *CompositeRetirementInspector) RemoveRetired(ctx context.Context, repositoryPath, trustedRoot string, proof RetirementProof) error {
	adapter, canonicalPath, selectedRoot, err := c.selectAdapter(ctx, proof.Path, "", true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(trustedRoot) != "" {
		canonicalTrusted, trustedErr := canonicalRetirementRoot(trustedRoot)
		if trustedErr != nil || canonicalTrusted != selectedRoot {
			return ErrUnsafeTarget
		}
	}
	proof.Path = canonicalPath
	proof.RepositoryCommonDir = filepath.Clean(proof.RepositoryCommonDir)
	return adapter.RemoveRetired(ctx, repositoryPath, selectedRoot, proof)
}

func (c *CompositeRetirementInspector) selectAdapter(_ context.Context, worktreePath, role string, allowMissing bool) (*Git, string, string, error) {
	if c == nil {
		return nil, "", "", ErrUnsafeTarget
	}
	managedRoot := strings.TrimSpace(c.managedRoot)
	if managedRoot == "" && c.managedGit != nil {
		managedRoot = c.managedGit.ManagedRoot
	}
	herdrRoot := strings.TrimSpace(c.herdrRoot)
	if herdrRoot == "" && c.herdrGit != nil {
		herdrRoot = c.herdrGit.ManagedRoot
	}
	managedCanonical, err := canonicalRetirementRoot(managedRoot)
	if err != nil {
		return nil, "", "", err
	}
	herdrCanonical, err := canonicalRetirementRoot(herdrRoot)
	if err != nil {
		return nil, "", "", err
	}
	if managedCanonical == herdrCanonical || strictlyContained(managedCanonical, herdrCanonical) || strictlyContained(herdrCanonical, managedCanonical) {
		return nil, "", "", ErrUnsafeTarget
	}
	canonicalTarget, err := canonicalRetirementSelectionPath(worktreePath, allowMissing)
	if err != nil {
		return nil, "", "", err
	}
	inManaged := strictlyContained(managedCanonical, canonicalTarget)
	inHerdr := strictlyContained(herdrCanonical, canonicalTarget)
	if inManaged == inHerdr {
		return nil, "", "", ErrUnsafeTarget
	}
	selectedRoot := herdrCanonical
	adapter := c.herdrGit
	if inManaged {
		selectedRoot = managedCanonical
		adapter = c.managedGit
	}
	if adapter == nil || adapter.Runner == nil {
		return nil, "", "", ErrUnsafeTarget
	}
	configuredRoot, configuredErr := canonicalRetirementRoot(adapter.ManagedRoot)
	if configuredErr != nil || configuredRoot != selectedRoot {
		return nil, "", "", ErrUnsafeTarget
	}
	if role != "" && !c.roleOwnsRoot(role, selectedRoot) {
		return nil, "", "", ErrUnsafeTarget
	}
	return adapter, canonicalTarget, selectedRoot, nil
}

func (c *CompositeRetirementInspector) roleOwnsRoot(role, selectedRoot string) bool {
	managedCanonical, managedErr := canonicalRetirementRoot(c.managedRoot)
	if managedErr != nil && c.managedGit != nil {
		managedCanonical, managedErr = canonicalRetirementRoot(c.managedGit.ManagedRoot)
	}
	herdrCanonical, herdrErr := canonicalRetirementRoot(c.herdrRoot)
	if herdrErr != nil && c.herdrGit != nil {
		herdrCanonical, herdrErr = canonicalRetirementRoot(c.herdrGit.ManagedRoot)
	}
	if managedErr != nil || herdrErr != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "reviewer", "integration":
		return selectedRoot == managedCanonical
	case "builder":
		return selectedRoot == herdrCanonical
	default:
		return false
	}
}

func canonicalRetirementRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path {
		return "", ErrUnsafeTarget
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil || filepath.Clean(path) != path {
		return "", ErrUnsafeTarget
	}
	resolved, err := resolvePath(path)
	if err != nil || resolved != filepath.Clean(absPath) || isFilesystemRoot(resolved) {
		return "", ErrUnsafeTarget
	}
	if validateTrustedManagedRoot(resolved) != nil {
		return "", ErrUnsafeTarget
	}
	return resolved, nil
}

func canonicalRetirementSelectionPath(path string, allowMissing bool) (string, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path || filepath.Clean(path) != path {
		return "", ErrUnsafeTarget
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", ErrUnsafeTarget
	}
	canonical, exists, err := resolveRetirementTargetPath(path, allowMissing)
	if err != nil || (!allowMissing && !exists) || (exists && canonical != filepath.Clean(absPath)) {
		return "", ErrUnsafeTarget
	}
	if !exists && canonical != filepath.Clean(absPath) {
		return "", ErrUnsafeTarget
	}
	if info, statErr := os.Stat(canonical); statErr == nil && !info.IsDir() {
		return "", ErrUnsafeTarget
	}
	return filepath.Clean(canonical), nil
}

var _ interface {
	InspectRetirementTarget(context.Context, string, string, string, string) (RetirementProof, error)
} = (*CompositeRetirementInspector)(nil)
