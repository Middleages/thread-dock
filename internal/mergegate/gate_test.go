package mergegate

import (
	"reflect"
	"testing"
)

func TestGateDecisionTable(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want Kind
	}{
		{"ordinary", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Merge},
		{"protected", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true, ProtectedReasons: []string{"authentication/**"}}, NeedsOperator},
		{"protected confirmed", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true, ProtectedReasons: []string{"authentication/**"}, OperatorConfirmed: true}, Merge},
		{"builder incomplete", Input{AcceptanceMet: true, BuildersComplete: false, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Wait},
		{"acceptance unmet", Input{BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Block},
		{"review rejected", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: false, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Block},
		{"check pending", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "pending"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Wait},
		{"checks absent", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Wait},
		{"mergeability pending", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true}, Wait},
		{"latest main stale", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, MergeabilityKnown: true, Mergeable: true}, Wait},
		{"check failed", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "failure"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Block},
		{"check unknown", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "other"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}, Block},
		{"merge conflict", Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, Checks: []CheckState{{Name: "ci", State: "success"}}, LatestMainTested: true, MergeabilityKnown: true, Mergeable: false}, Block},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(tc.in); got.Kind != tc.want {
				t.Fatalf("got=%v want=%v reasons=%v", got.Kind, tc.want, got.Reasons)
			}
		})
	}
}

func TestGateFailsClosedForEmptyAndDuplicateChecks(t *testing.T) {
	base := Input{AcceptanceMet: true, BuildersComplete: true, ReviewerApproved: true, LatestMainTested: true, MergeabilityKnown: true, Mergeable: true}
	for _, checks := range [][]CheckState{
		{{Name: "", State: "success"}},
		{{Name: "ci", State: "success"}, {Name: "ci", State: "success"}},
	} {
		base.Checks = checks
		if got := Evaluate(base); got.Kind != Block {
			t.Fatalf("checks=%#v got=%v", checks, got)
		}
	}
}

func TestProtectedReasonsClassifiesPathsAndValidRiskCategories(t *testing.T) {
	got := ProtectedReasons(
		[]string{"src/main.go", "authentication/policy.go", "migrations/001.sql", ".github/workflows/test.yml", "deployment/app.yaml", "authentication/policy.go"},
		[]string{"data", "public_contract", "ignored"},
		[]string{"supply_chain", "data", "ignored"},
	)
	want := []string{".github/workflows/**", "authentication/**", "data", "deployment/**", "migrations/**", "public_contract", "supply_chain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestProtectedReasonsIgnoresInvalidPathsAndRiskCategories(t *testing.T) {
	got := ProtectedReasons([]string{"../authentication/policy.go", "src/main.go"}, []string{"DATA", "unknown"}, []string{" data ", "unknown"})
	if len(got) != 0 {
		t.Fatalf("got=%v want empty", got)
	}
}
