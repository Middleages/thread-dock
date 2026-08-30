// Package mergegate contains the side-effect-free decision boundary for
// supervised pull-request merges.
package mergegate

import (
	"sort"
	"strings"

	"thread-dock/internal/contract"
	"thread-dock/internal/pathscope"
)

// Kind is the action permitted by the Merge Gate.
type Kind string

const (
	Wait          Kind = "wait"
	NeedsOperator Kind = "needs_operator"
	Merge         Kind = "merge"
	Block         Kind = "block"
)

// CheckState is the normalized state of one required check.
type CheckState struct {
	Name  string
	State string
}

// Input contains only evidence needed to make a Merge Gate decision.
type Input struct {
	AcceptanceMet     bool
	BuildersComplete  bool
	ReviewerApproved  bool
	Checks            []CheckState
	LatestMainTested  bool
	MergeabilityKnown bool
	Mergeable         bool
	ProtectedReasons  []string
	OperatorConfirmed bool
}

// Decision is the deterministic result of evaluating Input.
type Decision struct {
	Kind    Kind
	Reasons []string
}

// Evaluate applies the fail-closed Merge Gate precedence: incomplete Builders
// wait first; acceptance/reviewer rejection blocks; malformed or failed check
// evidence blocks before pending waits; known unmergeability blocks before
// stale-main waits; then absent/pending/unknown evidence waits; protected
// reasons finally require operator confirmation. A confirmation is meaningful
// only when at least one protected reason is present.
func Evaluate(input Input) Decision {
	reasons := canonicalReasons(input.ProtectedReasons)
	decision := func(kind Kind) Decision { return Decision{Kind: kind, Reasons: append([]string(nil), reasons...)} }

	if !input.BuildersComplete {
		return decision(Wait)
	}
	if !input.AcceptanceMet || !input.ReviewerApproved {
		return decision(Block)
	}
	if input.MergeabilityKnown && !input.Mergeable {
		return decision(Block)
	}
	if len(input.Checks) == 0 {
		return decision(Wait)
	}
	seen := make(map[string]struct{}, len(input.Checks))
	hasPending := false
	hasFailure := false
	for _, check := range input.Checks {
		name := strings.TrimSpace(check.Name)
		if name == "" || name != check.Name {
			return decision(Block)
		}
		if _, exists := seen[name]; exists {
			return decision(Block)
		}
		seen[name] = struct{}{}
	}
	for _, check := range input.Checks {
		switch check.State {
		case "pending":
			hasPending = true
		case "success":
		default:
			hasFailure = true
		}
	}
	if hasFailure {
		return decision(Block)
	}
	if hasPending {
		return decision(Wait)
	}
	if !input.LatestMainTested || !input.MergeabilityKnown {
		return decision(Wait)
	}
	if !input.Mergeable {
		return decision(Block)
	}
	if len(reasons) > 0 && !input.OperatorConfirmed {
		return decision(NeedsOperator)
	}
	return decision(Merge)
}

// ProtectedReasons classifies Git-derived changed paths and the explicit
// contract and Reviewer risk categories. Invalid entries are ignored because
// contract and evidence validators own rejection of malformed input.
func ProtectedReasons(changedFiles, contractRiskCategories, reviewerRiskCategories []string) []string {
	seen := make(map[string]struct{})
	for _, rawFile := range changedFiles {
		file := strings.TrimSpace(strings.ReplaceAll(rawFile, `\`, "/"))
		if file == "" {
			continue
		}
		for _, rawScope := range contract.DefaultProtectedPaths() {
			scope, err := pathscope.Normalize(rawScope)
			if err == nil && pathscope.Contains(scope, file) {
				seen[scope] = struct{}{}
			}
		}
	}
	for _, category := range append(append([]string(nil), contractRiskCategories...), reviewerRiskCategories...) {
		if contract.IsRiskCategory(category) {
			seen[category] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

func canonicalReasons(reasons []string) []string {
	seen := make(map[string]struct{}, len(reasons))
	for _, raw := range reasons {
		reason := strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
		if reason == "" {
			continue
		}
		if normalized, err := pathscope.Normalize(reason); err == nil {
			reason = normalized
		}
		seen[reason] = struct{}{}
	}
	return sortedKeys(seen)
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
