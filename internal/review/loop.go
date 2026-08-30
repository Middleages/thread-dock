// Package review contains the pure policy for Reviewer and CI review rounds.
package review

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"thread-dock/internal/pathscope"
	"thread-dock/internal/state"
)

const maxRepairRounds = 2

// ReviewResult is the normalized result of a Reviewer or CI check.
type ReviewResult struct {
	Source         string
	Blocking       bool
	Findings       []Finding
	RiskCategories []string
}

// Finding identifies one concrete blocking review issue.
type Finding struct {
	ID      string
	Summary string
	Paths   []string
}

// DecisionKind is the next action selected by Decide.
type DecisionKind string

// Kind is retained as a concise alias for callers following the other loop
// packages' decision API.
type Kind = DecisionKind

const (
	Accept DecisionKind = "accept"
	Repair DecisionKind = "repair"
	Block  DecisionKind = "block"
)

// Decision is the review loop action and the counter value to persist.
type Decision struct {
	Kind           DecisionKind
	NextCount      int
	Findings       []Finding
	RiskCategories []string
	Reason         string
}

// Decide shares one hard two-round repair budget between Reviewer and CI.
// Invalid sources are blocked without consuming budget. Negative counters
// are treated as zero and counters above the hard maximum are exhausted.
func Decide(snapshot state.RunSnapshot, result ReviewResult) Decision {
	count := snapshot.RepairCount
	if count < 0 {
		count = 0
	}
	if count > maxRepairRounds {
		count = maxRepairRounds
	}
	base := Decision{NextCount: count, Findings: result.Findings, RiskCategories: result.RiskCategories}
	if result.Source != "reviewer" && result.Source != "ci" {
		base.Kind = Block
		base.Reason = "invalid review source"
		return base
	}
	if !result.Blocking {
		base.Kind = Accept
		base.Reason = "review accepted"
		return base
	}
	if count >= maxRepairRounds {
		base.Kind = Block
		base.Reason = "repair budget exhausted"
		return base
	}
	base.Kind = Repair
	base.NextCount = count + 1
	base.Reason = "blocking findings require repair"
	return base
}

// RepairPacketInput supplies all data needed to construct a bounded repair
// prompt.
type RepairPacketInput struct {
	AcceptanceCriteria []string
	IntegrationSHA     string
	BlockingFindings   []Finding
	AllowedPaths       []string
	RemainingBudget    int
}

// BuildRepairPacket deterministically renders a repair packet containing only
// acceptance criteria, blocking findings, allowed paths, integration SHA, and
// the remaining shared repair budget.
func BuildRepairPacket(input RepairPacketInput) (string, error) {
	criteria := input.AcceptanceCriteria
	sha := input.IntegrationSHA
	if input.RemainingBudget < 0 || input.RemainingBudget > maxRepairRounds {
		return "", errors.New("remaining repair budget must be between 0 and 2")
	}
	if len(criteria) == 0 {
		return "", errors.New("acceptance criteria are required")
	}
	for _, criterion := range criteria {
		if !canonicalNonEmpty(criterion) {
			return "", errors.New("acceptance criteria must be canonical and nonempty")
		}
	}
	if !validSHA(sha) {
		return "", errors.New("integration SHA must be 40 lowercase hexadecimal characters")
	}
	if len(input.BlockingFindings) == 0 {
		return "", errors.New("blocking findings are required")
	}
	for _, finding := range input.BlockingFindings {
		if !canonicalNonEmpty(finding.ID) || !canonicalNonEmpty(finding.Summary) {
			return "", errors.New("finding ID and summary must be canonical and nonempty")
		}
		if len(finding.Paths) == 0 {
			return "", errors.New("finding paths are required")
		}
		for _, path := range finding.Paths {
			if !canonicalPath(path) {
				return "", errors.New("finding path must be canonical and repository-relative")
			}
		}
	}
	if len(input.AllowedPaths) == 0 {
		return "", errors.New("allowed paths are required")
	}
	for _, path := range input.AllowedPaths {
		if !canonicalPath(path) {
			return "", errors.New("allowed path must be canonical and repository-relative")
		}
	}

	var b strings.Builder
	b.WriteString("Repair task packet\n\nAcceptance criteria:\n")
	for _, criterion := range criteria {
		fmt.Fprintf(&b, "- %s\n", strconv.Quote(criterion))
	}
	b.WriteString("\nIntegration SHA: ")
	b.WriteString(strconv.Quote(sha))
	b.WriteString("\n\nBlocking findings:\n")
	for i, finding := range input.BlockingFindings {
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, strconv.Quote(finding.ID), strconv.Quote(finding.Summary))
		b.WriteString("   Paths: ")
		for pathIndex, path := range finding.Paths {
			if pathIndex > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(path))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nAllowed paths:\n")
	for _, path := range input.AllowedPaths {
		fmt.Fprintf(&b, "- %s\n", strconv.Quote(path))
	}
	fmt.Fprintf(&b, "\nRemaining repair budget: %d\n", input.RemainingBudget)
	return b.String(), nil
}

func canonicalNonEmpty(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func canonicalPath(value string) bool {
	if !canonicalNonEmpty(value) {
		return false
	}
	normalized, err := pathscope.Normalize(value)
	return err == nil && normalized == value
}

func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
