package scheduler

import (
	"reflect"
	"testing"

	"thread-dock/internal/dag"
)

func TestNextNeverExceedsTwo(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	got := Next(graph, map[string]TaskState{}, 2)
	if len(got) != 2 {
		t.Fatalf("got=%d", len(got))
	}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got=%v", got)
	}
}

func TestNextDoesNotDispatchDependentTask(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{{ID: "api"}, {ID: "tests", DependsOn: []string{"api"}}})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	got := Next(graph, map[string]TaskState{"api": Running}, 2)
	if len(got) != 0 {
		t.Fatalf("got=%v", got)
	}
}

func TestNextDispatchesCompletedDependenciesInGraphOrder(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{
		{ID: "release", DependsOn: []string{"tests", "review"}},
		{ID: "tests", DependsOn: []string{"api"}},
		{ID: "api"},
		{ID: "review"},
	})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	states := map[string]TaskState{"api": Completed, "review": Completed}
	if got := Next(graph, states, 2); !reflect.DeepEqual(got, []string{"tests"}) {
		t.Fatalf("got=%v", got)
	}
	states["tests"] = Completed
	if got := Next(graph, states, 2); !reflect.DeepEqual(got, []string{"release"}) {
		t.Fatalf("got=%v", got)
	}
}

func TestNextCountsRunningAndRepairingAgainstCapacity(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	states := map[string]TaskState{"a": Running, "b": Repairing}
	if got := Next(graph, states, 5); len(got) != 0 {
		t.Fatalf("got=%v", got)
	}
	states["b"] = Completed
	if got := Next(graph, states, 5); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("got=%v", got)
	}
}

func TestNextOnlyDispatchesPendingOrMissingTasks(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{
		{ID: "pending"},
		{ID: "missing"},
		{ID: "paused"},
		{ID: "failed"},
		{ID: "blocked"},
		{ID: "completed"},
	})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	states := map[string]TaskState{
		"paused":    Paused,
		"failed":    Failed,
		"blocked":   Blocked,
		"completed": Completed,
	}
	if got := Next(graph, states, 2); !reflect.DeepEqual(got, []string{"pending", "missing"}) {
		t.Fatalf("got=%v", got)
	}
}

func TestNextUnknownStateFailsClosedForThatTask(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{{ID: "unknown"}, {ID: "pending"}})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	states := map[string]TaskState{"unknown": TaskState("future")}
	if got := Next(graph, states, 2); !reflect.DeepEqual(got, []string{"pending"}) {
		t.Fatalf("got=%v", got)
	}
}

func TestNextRejectsNonPositiveLimit(t *testing.T) {
	graph, violations := dag.Build([]dag.Node{{ID: "a"}})
	if len(violations) != 0 {
		t.Fatal(violations)
	}
	for _, limit := range []int{0, -1} {
		if got := Next(graph, nil, limit); got != nil {
			t.Fatalf("limit=%d got=%v", limit, got)
		}
	}
}
