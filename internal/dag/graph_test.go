package dag

import (
	"reflect"
	"testing"
)

func TestReadyReturnsSatisfiedIDsInContractOrder(t *testing.T) {
	graph, violations := Build([]Node{{ID: "tests", DependsOn: []string{"api"}}, {ID: "api"}})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	if got := graph.Ready(nil); !reflect.DeepEqual(got, []string{"api"}) {
		t.Fatalf("ready=%v", got)
	}
	if got := graph.Ready(map[string]bool{"api": true}); !reflect.DeepEqual(got, []string{"tests"}) {
		t.Fatalf("ready=%v", got)
	}
}

func TestBuildReportsMissingDependencyAndCycle(t *testing.T) {
	_, got := Build([]Node{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a", "missing"}}})
	if !hasCodes(got, "dependency_cycle", "missing_dependency") {
		t.Fatal(got)
	}
}

func TestReadyExcludesCompletedNodesAndRequiresEveryDependency(t *testing.T) {
	graph, violations := Build([]Node{
		{ID: "release", DependsOn: []string{"tests", "review"}},
		{ID: "tests", DependsOn: []string{"api"}},
		{ID: "api"},
		{ID: "review"},
	})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	completed := map[string]bool{"api": true, "review": true}
	if got := graph.Ready(completed); !reflect.DeepEqual(got, []string{"tests"}) {
		t.Fatalf("ready=%v", got)
	}
	completed["tests"] = true
	if got := graph.Ready(completed); !reflect.DeepEqual(got, []string{"release"}) {
		t.Fatalf("ready=%v", got)
	}
}

func hasCodes(violations []Violation, want ...string) bool {
	seen := make(map[string]bool, len(violations))
	for _, violation := range violations {
		seen[violation.Code] = true
	}
	for _, code := range want {
		if !seen[code] {
			return false
		}
	}
	return true
}
