package review

import (
	"strings"
	"testing"

	"thread-dock/internal/state"
)

func TestReviewerThenCIFailureConsumesTwoTotalRounds(t *testing.T) {
	s := state.RunSnapshot{RepairCount: 0}
	first := Decide(s, ReviewResult{Source: "reviewer", Blocking: true})
	if first.Kind != Repair || first.NextCount != 1 {
		t.Fatal(first)
	}
	s.RepairCount = first.NextCount
	second := Decide(s, ReviewResult{Source: "ci", Blocking: true})
	if second.Kind != Repair || second.NextCount != 2 {
		t.Fatal(second)
	}
	s.RepairCount = second.NextCount
	third := Decide(s, ReviewResult{Source: "ci", Blocking: true})
	if third.Kind != Block || third.NextCount != 2 {
		t.Fatal(third)
	}
}

func TestDecideAcceptPreservesCountAndRejectsInvalidSources(t *testing.T) {
	accepted := Decide(state.RunSnapshot{RepairCount: 1}, ReviewResult{Source: "reviewer"})
	if accepted.Kind != Accept || accepted.NextCount != 1 {
		t.Fatal(accepted)
	}
	for _, source := range []string{"", "Reviewer", "reviewer ", "ci ", "github"} {
		got := Decide(state.RunSnapshot{RepairCount: 0}, ReviewResult{Source: source, Blocking: true})
		if got.Kind != Block || got.NextCount != 0 {
			t.Fatalf("source %q: %#v", source, got)
		}
	}
}

func TestDecideClampsCountersAndHardCapsBudget(t *testing.T) {
	if got := Decide(state.RunSnapshot{RepairCount: -4}, ReviewResult{Source: "ci", Blocking: true}); got.Kind != Repair || got.NextCount != 1 {
		t.Fatal(got)
	}
	if got := Decide(state.RunSnapshot{RepairCount: 9}, ReviewResult{Source: "ci", Blocking: true}); got.Kind != Block || got.NextCount != 2 {
		t.Fatal(got)
	}
}

func TestBuildRepairPacketIsDeterministicAndContainsBlockingDataOnly(t *testing.T) {
	input := RepairPacketInput{
		AcceptanceCriteria: []string{"criteria one", "criteria two"},
		IntegrationSHA:     "0123456789abcdef0123456789abcdef01234567",
		BlockingFindings:   []Finding{{ID: "F-1", Summary: "fix this", Paths: []string{"src/api.go"}}},
		AllowedPaths:       []string{"src/**", "README.md"},
		RemainingBudget:    1,
	}
	first, err := BuildRepairPacket(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildRepairPacket(input)
	if err != nil || first != second {
		t.Fatalf("second=%q err=%v first=%q", second, err, first)
	}
	for _, want := range []string{"1. F-1: fix this", "src/api.go", "src/**", "README.md", "criteria one", "0123456789abcdef0123456789abcdef01234567", "Remaining repair budget: 1"} {
		if !strings.Contains(first, want) {
			t.Fatalf("packet %q missing %q", first, want)
		}
	}
	if strings.Contains(first, "recommend") {
		t.Fatalf("packet leaks recommendations: %q", first)
	}
}

func TestBuildRepairPacketRejectsInvalidInputs(t *testing.T) {
	base := RepairPacketInput{AcceptanceCriteria: []string{"criteria"}, IntegrationSHA: "0123456789abcdef0123456789abcdef01234567", BlockingFindings: []Finding{{ID: "F-1", Summary: "fix", Paths: []string{"src/api.go"}}}, AllowedPaths: []string{"src/**"}, RemainingBudget: 1}
	cases := []struct {
		name   string
		mutate func(*RepairPacketInput)
	}{
		{"criteria empty", func(i *RepairPacketInput) { i.AcceptanceCriteria = nil }},
		{"sha invalid", func(i *RepairPacketInput) { i.IntegrationSHA = "bad" }},
		{"findings empty", func(i *RepairPacketInput) { i.BlockingFindings = nil }},
		{"finding id empty", func(i *RepairPacketInput) { i.BlockingFindings[0].ID = " " }},
		{"finding path invalid", func(i *RepairPacketInput) { i.BlockingFindings[0].Paths = []string{"../secret"} }},
		{"allowed path invalid", func(i *RepairPacketInput) { i.AllowedPaths = []string{"/src"} }},
		{"budget invalid", func(i *RepairPacketInput) { i.RemainingBudget = 3 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.mutate(&input)
			if _, err := BuildRepairPacket(input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
